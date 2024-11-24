package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"search_engine/agent"
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
		panic(fmt.Errorf("error creating search engine: %w", err))
	}
	log.Println("Search engine is up and running")
	br, err := browser.NewBaseBrowser(searchEngine, &browser.BrowserOptions{
		MaxConcurrency: *maxConcurrency,
	})
	if err != nil {
		panic(fmt.Errorf("error creating browser: %w", err))
	}
	ag := agent.NewLLMBrowserAgent(nil)
	res, err := ag.Solve(ctx, *query, br)
	if err != nil {
		panic(fmt.Errorf("error solving: %w", err))
	}
	log.Println(res)
}
