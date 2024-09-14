package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"

	"search_engine/index"
	"search_engine/primitives/api"
	"search_engine/search"
)

func main() {
	ctx := context.Background()
	query := flag.String("query", "", "Search query")
	indexPath := flag.String("index", "", "Path to the embedded specs JSON file")
	maxConcurrency := flag.Int("max-concurrency", 8, "Maximum number of concurrent requests to make to the LLM")
	disableVerification := flag.Bool("disable-verification", false, "Disable verification of search results")
	n := flag.Int("n", 5, "Number of search results to return")
	flag.Parse()

	if *indexPath == "" || *query == "" {
		flag.Usage()
		os.Exit(1)
	}

	docs, err := index.LoadIndexedDocs(*indexPath)
	if err != nil {
		log.Fatalf("Error loading embedded docs: %v", err)
	}

	api := api.DefaultAPI()
	useVerification := !*disableVerification
	results, err := search.Search(ctx, docs, *query, api, &search.SearchOptions{
		MaxConcurrency:  *maxConcurrency,
		MaxNumResults:   *n,
		UseVerification: &useVerification,
	})
	if err != nil {
		log.Fatalf("Error performing search: %v", err)
	}
	if len(results) == 0 {
		fmt.Printf("No results found for query: '%s'\n", *query)
	} else {
		fmt.Printf("Found %d results for query: '%s'\n", len(results), *query)
		for i, result := range results {
			if useVerification {
				fmt.Printf("%d. %s\n", i+1, result.WebPageTitle)
			} else {
				fmt.Printf("%d. %s (score=%.4f)\n", i+1, result.WebPageTitle, result.Score)
			}
		}
	}
}
