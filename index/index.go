package index

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"search_engine/primitives/api"
	"strings"
	"sync"
)

type Document struct {
	Title     string          `json:"title"`
	Summary   string          `json:"summary"`
	Embedding []float64       `json:"embedding"`
	Spec      json.RawMessage `json:"spec"`
}

func LoadIndexedSpecs(filePath string) ([]*Document, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	var specs []*Document
	decoder := json.NewDecoder(file)
	err = decoder.Decode(&specs)
	if err != nil {
		return nil, err
	}
	return specs, nil
}

func SummarizeSpec(ctx context.Context, spec *Document, api *api.API) (string, error) {
	var openAPISpec map[string]interface{}
	err := json.Unmarshal(spec.Spec, &openAPISpec)
	if err != nil {
		return "", err
	}
	info, _ := openAPISpec["info"].(map[string]interface{})
	title := ""
	description := ""
	if info != nil {
		title, _ = info["title"].(string)
		description, _ = info["description"].(string)
	}
	paths, _ := openAPISpec["paths"].(map[string]interface{})
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

func IndexSpecs(ctx context.Context, specsPath string, api *api.API, maxConcurrency int) ([]*Document, error) {
	var documents []*Document
	var mu sync.Mutex
	var wg sync.WaitGroup

	files, err := os.ReadDir(specsPath)
	if err != nil {
		return nil, fmt.Errorf("error reading directory: %w", err)
	}
	fmt.Printf("Indexing %d specs from %s\n", len(files), specsPath)

	semaphore := make(chan struct{}, maxConcurrency)

	for _, file := range files {
		if !file.IsDir() && strings.HasSuffix(file.Name(), ".json") {
			wg.Add(1)
			go func(fileName string) {
				defer wg.Done()
				semaphore <- struct{}{}
				defer func() { <-semaphore }()

				doc, err := processSpec(ctx, fileName, specsPath, api)
				if err != nil {
					log.Printf("Error processing %s: %v", fileName, err)
					return
				}

				mu.Lock()
				documents = append(documents, doc)
				mu.Unlock()
			}(file.Name())
		}
	}
	wg.Wait()
	return documents, nil
}

func processSpec(ctx context.Context, fileName, dirPath string, api *api.API) (*Document, error) {
	filePath := filepath.Join(dirPath, fileName)
	specData, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("error reading file %s: %w", fileName, err)
	}

	var spec map[string]interface{}
	if err := json.Unmarshal(specData, &spec); err != nil {
		return nil, fmt.Errorf("error unmarshaling JSON from %s: %w", fileName, err)
	}
	title := ""
	if info, ok := spec["info"].(map[string]interface{}); ok {
		if t, ok := info["title"].(string); ok {
			title = t
		}
	}
	summary, err := SummarizeSpec(ctx, &Document{Spec: specData}, api)
	if err != nil {
		return nil, fmt.Errorf("error summarizing spec %s: %w", fileName, err)
	}
	embeddings, err := api.Embedding(ctx, summary)
	if err != nil {
		return nil, fmt.Errorf("error getting embedding for %s: %w", fileName, err)
	}
	return &Document{
		Title:     title,
		Summary:   summary,
		Embedding: embeddings,
		Spec:      specData,
	}, nil
}
