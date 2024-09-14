package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"search_engine/agent"
	"search_engine/browser"
	"search_engine/primitives/api"
	"search_engine/search"
	"search_engine/www"
)

func main() {
	ctx := context.Background()
	content := flag.String("content", "", "content to index")
	query := flag.String("query", "", "query to search for")
	maxConcurrency := flag.Int("max-concurrency", 8, "max concurrency")
	flag.Parse()
	if *content == "" {
		log.Fatal("content is required")
	}
	if *query == "" {
		log.Fatal("query is required")
	}
	www, err := www.NewWWWFromPath(*content)
	if err != nil {
		log.Fatal(err)
	}
	log.Println("WWW is up and running")
	a := api.DefaultAPI()
	searchEngine, err := search.NewPubAPISearchEngine(a, www)
	if err != nil {
		panic(fmt.Errorf("error creating search engine: %w", err))
	}
	log.Println("Search engine is up and running")
	log.Println("Refreshing index...")
	err = searchEngine.RefreshIndex(ctx, www, www.AllAddresses(), &search.RefreshIndexOptions{
		MaxConcurrency: *maxConcurrency,
	})
	if err != nil {
		panic(fmt.Errorf("error refreshing index: %w", err))
	}
	log.Println("Index is refreshed")
	br, err := browser.NewPubAPISpecBrowser(searchEngine, www, &browser.BrowserOptions{
		MaxConcurrency: *maxConcurrency,
	})
	if err != nil {
		panic(fmt.Errorf("error creating browser: %w", err))
	}
	ag := agent.NewLLMBrowserAgent(a)
	res, err := ag.Solve(ctx, *query, br)
	if err != nil {
		panic(fmt.Errorf("error solving: %w", err))
	}
	log.Println(res)
}
