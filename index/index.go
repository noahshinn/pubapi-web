package index

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"search_engine/primitives/api"
	"search_engine/www"
	"strings"
	"sync"
)

type Document struct {
	WebPage   *www.WebPage  `json:"web_page"`
	Summary   string        `json:"summary"`
	Embedding []float64     `json:"embedding"`
	Endpoint  *www.Endpoint `json:"endpoint"`
}

func LoadIndexedDocs(filePath string) ([]*Document, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	var docs []*Document
	decoder := json.NewDecoder(file)
	err = decoder.Decode(&docs)
	if err != nil {
		return nil, err
	}
	return docs, nil
}

func summarizeDoc(ctx context.Context, doc *Document, api *api.ModelAPI) (string, error) {
	info, _ := doc.WebPage.Content["info"].(map[string]any)
	title := ""
	description := ""
	if info != nil {
		title, _ = info["title"].(string)
		description, _ = info["description"].(string)
	}
	paths, _ := doc.WebPage.Content["paths"].(map[string]any)
	var sampleEndpoints []string
	for path := range paths {
		sampleEndpoints = append(sampleEndpoints, path)
		if len(sampleEndpoints) == 5 {
			break
		}
	}
	instruction := "Summarize the following API specification. Provide a concise summary that captures the key features and purpose of this API:"
	text := ""
	if title != "" {
		text += "Title: " + title + "\n"
	}
	if description != "" {
		text += "Description: " + description + "\n"
	}
	text += "Sample endpoints:\n" + strings.Join(sampleEndpoints, ", ") + "\n\n-----\n\n"
	return api.Generate(ctx, instruction, text, nil)
}

type EndpointAndWebPage struct {
	Endpoint *www.Endpoint
	WebPage  *www.WebPage
}

type IndexOptions struct {
	MaxConcurrency int
	ModelAPI       *api.ModelAPI
}

const DefaultMaxConcurrency = 8

func IndexWebPages(ctx context.Context, endpointsAndWebPages []*EndpointAndWebPage, opts *IndexOptions) ([]*Document, error) {
	var documents []*Document
	var mu sync.Mutex
	var wg sync.WaitGroup

	maxConcurrency := DefaultMaxConcurrency
	modelAPI := api.DefaultModelAPI()
	if opts != nil {
		maxConcurrency = opts.MaxConcurrency
		modelAPI = opts.ModelAPI
	}

	semaphore := make(chan struct{}, maxConcurrency)

	for _, endpointAndWebPage := range endpointsAndWebPages {
		wg.Add(1)
		go func(endpointAndWebPage *EndpointAndWebPage) {
			defer wg.Done()
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			doc, err := processWebPage(ctx, endpointAndWebPage.Endpoint, endpointAndWebPage.WebPage, modelAPI)
			if err != nil {
				log.Printf("Error processing doc: %v", err)
				return
			}

			mu.Lock()
			documents = append(documents, doc)
			mu.Unlock()
		}(endpointAndWebPage)
	}
	wg.Wait()
	return documents, nil
}

func processWebPage(ctx context.Context, endpoint *www.Endpoint, webPage *www.WebPage, api *api.ModelAPI) (*Document, error) {
	summary, err := summarizeDoc(ctx, &Document{WebPage: webPage}, api)
	if err != nil {
		return nil, fmt.Errorf("error summarizing doc: %w", err)
	}
	embeddings, err := api.Embedding(ctx, summary)
	if err != nil {
		return nil, fmt.Errorf("error getting embedding for doc: %w", err)
	}
	return &Document{
		Summary:   summary,
		Embedding: embeddings,
		WebPage:   webPage,
		Endpoint:  endpoint,
	}, nil
}
