package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"

	"search_engine/index"
	"search_engine/llm"
	"search_engine/primitives/api"
)

func main() {
	ctx := context.Background()
	specsPath := flag.String("specs-path", "", "Directory containing OpenAPI spec JSON files")
	maxConcurrency := flag.Int("max-concurrency", 8, "Maximum number of concurrent processes")
	outputPath := flag.String("output-path", "", "File to output indexed specs to")
	flag.Parse()

	if *specsPath == "" {
		log.Fatal("Please provide a directory containing OpenAPI spec JSON files using the -specs-path flag")
	}
	if *outputPath == "" {
		log.Fatal("Please provide a path to output the indexed specs to using the -output-path flag")
	}

	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		log.Fatal("Please provide an OpenAI API key using the OPENAI_API_KEY environment variable")
	}
	models := llm.AllModels(apiKey)
	api := api.DefaultAPI()

	indexedSpecs, err := index.IndexSpecs(ctx, *specsPath, models.DefaultEmbeddingModel, api, *maxConcurrency)
	if err != nil {
		log.Fatalf("Error indexing specs: %v", err)
	}
	f, err := os.Create(*outputPath)
	if err != nil {
		log.Fatalf("Error creating indexed specs file: %v", err)
	}
	defer f.Close()

	encoder := json.NewEncoder(f)
	err = encoder.Encode(indexedSpecs)
	if err != nil {
		log.Fatalf("Error encoding indexed specs: %v", err)
	}
	fmt.Printf("Indexed %d specs and saved to %s\n", len(indexedSpecs), *outputPath)
}
