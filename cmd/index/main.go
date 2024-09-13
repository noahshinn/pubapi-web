package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"search_engine/index"
	"search_engine/primitives/api"
	"search_engine/www"
)

func main() {
	ctx := context.Background()
	specsPath := flag.String("specs-path", "", "Directory containing OpenAPI spec JSON files")
	maxConcurrency := flag.Int("max-concurrency", 8, "Maximum number of concurrent processes")
	outputPath := flag.String("output-path", "", "File to output indexed docs to")
	flag.Parse()

	if *specsPath == "" {
		log.Fatal("Please provide a directory containing OpenAPI spec JSON files using the -specs-path flag")
	}
	if *outputPath == "" {
		log.Fatal("Please provide a path to output the indexed docs to using the -output-path flag")
	}
	api := api.DefaultAPI()

	paths, err := os.ReadDir(*specsPath)
	if err != nil {
		log.Fatalf("Error reading specs directory: %v", err)
	}
	webPages := []*www.WebPage{}
	for _, path := range paths {
		if path.IsDir() {
			continue
		}
		specPath := filepath.Join(*specsPath, path.Name())
		spec, err := os.ReadFile(specPath)
		if err != nil {
			log.Fatalf("Error reading doc file: %v", err)
		}
		var specMap map[string]any
		err = json.Unmarshal(spec, &specMap)
		if err != nil {
			log.Fatalf("Error unmarshalling doc file: %v", err)
		}
		webPages = append(webPages, www.NewWebPage(path.Name(), specMap))
	}

	indexedDocs, err := index.IndexWebPages(ctx, webPages, api, *maxConcurrency)
	if err != nil {
		log.Fatalf("Error indexing docs: %v", err)
	}
	f, err := os.Create(*outputPath)
	if err != nil {
		log.Fatalf("Error creating indexed docs file: %v", err)
	}
	defer f.Close()

	encoder := json.NewEncoder(f)
	err = encoder.Encode(indexedDocs)
	if err != nil {
		log.Fatalf("Error encoding indexed docs: %v", err)
	}
	fmt.Printf("Indexed %d docs and saved to %s\n", len(indexedDocs), *outputPath)
}
