package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"search_engine/browser"
	"search_engine/index"
	"search_engine/search"
)

func main() {
	ctx := context.Background()
	query := flag.String("query", "", "query to search for")
	searchIndexPath := flag.String("search-index", "", "path to search index")
	maxConcurrency := flag.Int("max-concurrency", 8, "max concurrency")
	flag.Parse()
	if *query == "" {
		log.Fatal("query is required")
	}
	var searchIndex []*index.Document
	if *searchIndexPath != "" {
		bytes, err := os.ReadFile(*searchIndexPath)
		if err != nil {
			panic(fmt.Errorf("error reading search index: %w", err))
		}
		if err := json.Unmarshal(bytes, &searchIndex); err != nil {
			panic(fmt.Errorf("error unmarshalling search index: %w", err))
		}
	}
	searchEngine, err := search.NewDenseEmbeddingSearchEngine(searchIndex, nil)
	if err != nil {
		log.Fatal(err)
	}
	log.Println("Search engine is up and running")
	b, err := browser.NewBaseBrowser(searchEngine, &browser.BrowserOptions{
		MaxConcurrency: *maxConcurrency,
	})
	if err != nil {
		log.Fatal(err)
	}
	log.Println("Browser is up and running")
	useVerification := true
	results, err := b.Search(ctx, *query, &search.SearchOptions{
		MaxNumResults:   10,
		MaxConcurrency:  *maxConcurrency,
		UseVerification: &useVerification,
	})
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("\nGot %d results", len(results))
	topResult := results[0]
	fmt.Printf("\n%s\n", topResult.WebPageTitle)
}
