package search

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"sync"

	"search_engine/index"
	"search_engine/primitives/api"
	"search_engine/utils/slicesx"
	"search_engine/www"
	"sort"

	"gonum.org/v1/gonum/mat"
)

type SearchResult struct {
	WebPageTitle string
	Endpoint     *www.Endpoint
	Score        float64
	Summary      string
}

func LoadEmbeddedSpecs(filePath string) ([]*index.Document, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	var specs []*index.Document
	err = json.Unmarshal(data, &specs)
	if err != nil {
		return nil, err
	}

	return specs, nil
}

func GetQueryEmbedding(ctx context.Context, query string, api *api.ModelAPI) ([]float64, error) {
	embeddings, err := api.Embedding(ctx, query)
	if err != nil {
		return nil, err
	}
	return embeddings, nil
}

func similaritySearch(queryEmbedding []float64, docs []*index.Document, n int, scoreFunc func(a, b []float64) float64) []*SearchResult {
	similarities := make([]float64, len(docs))
	for i, doc := range docs {
		similarities[i] = scoreFunc(queryEmbedding, doc.Embedding)
	}
	indices := make([]int, len(similarities))
	for i := range indices {
		indices[i] = i
	}
	sort.Slice(indices, func(i, j int) bool {
		return similarities[indices[i]] > similarities[indices[j]]
	})
	results := make([]*SearchResult, min(n, len(docs)))
	for i := range results {
		idx := indices[i]
		results[i] = &SearchResult{
			WebPageTitle: docs[idx].WebPage.Title,
			Endpoint:     docs[idx].Endpoint,
			Score:        similarities[idx],
			Summary:      docs[idx].Summary,
		}
	}
	return results
}

func cosineSimilarity(a, b []float64) float64 {
	va := mat.NewVecDense(len(a), a)
	vb := mat.NewVecDense(len(b), b)

	dotProduct := mat.Dot(va, vb)
	normA := mat.Norm(va, 2)
	normB := mat.Norm(vb, 2)

	return dotProduct / (normA * normB)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func verifySearchResults(ctx context.Context, query string, results []*SearchResult, api *api.ModelAPI, maxConcurrency int) ([]*SearchResult, error) {
	var wg sync.WaitGroup
	semaphore := make(chan struct{}, maxConcurrency)
	verifiedResults := make([]*SearchResult, len(results))
	for i, result := range results {
		wg.Add(1)
		go func(i int, result *SearchResult) {
			defer wg.Done()
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			response, err := api.BinaryClassify(ctx, "Determine if the search result is relevant to the query. The query is searching an index of public API specs for a set of public APIs that will at least partially satisfy the desired behavior.", fmt.Sprintf("Query:\n%s\nPublic API spec summary:\n%s", query, result.Summary), nil)
			if err != nil {
				log.Printf("Error verifying search result, skipping: %v", err)
				return
			}
			if response {
				verifiedResults[i] = result
			}
		}(i, result)
	}
	wg.Wait()
	filteredResults := slicesx.Filter(verifiedResults, func(result *SearchResult) bool {
		return result != nil
	})
	return filteredResults, nil
}

type SearchOptions struct {
	MaxConcurrency  int
	MaxNumResults   int
	UseVerification *bool
	ModelAPI        *api.ModelAPI
}

const defaultMaxConcurrency = 8
const defaultMaxNumResults = 5

func Search(ctx context.Context, docs []*index.Document, query string, options *SearchOptions) ([]*SearchResult, error) {
	maxConcurrency := defaultMaxConcurrency
	maxNumResults := defaultMaxNumResults
	useVerification := true
	modelAPI := api.DefaultModelAPI()
	if options != nil {
		if options.MaxConcurrency != 0 {
			maxConcurrency = options.MaxConcurrency
		}
		if options.MaxNumResults != 0 {
			maxNumResults = options.MaxNumResults
		}
		if options.UseVerification != nil {
			useVerification = *options.UseVerification
		}
		if options.ModelAPI != nil {
			modelAPI = options.ModelAPI
		}
	}
	queryEmbedding, err := GetQueryEmbedding(ctx, query, modelAPI)
	if err != nil {
		return nil, err
	}
	results := similaritySearch(queryEmbedding, docs, maxNumResults, cosineSimilarity)
	if useVerification {
		return verifySearchResults(ctx, query, results, modelAPI, maxConcurrency)
	}
	return results, nil
}
