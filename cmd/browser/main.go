package main

import (
	"context"
	"flag"
	"fmt"
	"log"
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
		log.Fatal(err)
	}
	log.Println("Search engine is up and running")
	log.Println("Refreshing index...")
	err = searchEngine.RefreshIndex(ctx, www, www.AllAddresses(), &search.RefreshIndexOptions{
		MaxConcurrency: *maxConcurrency,
	})
	if err != nil {
		log.Fatal(err)
	}
	log.Println("Index is refreshed")
	b, err := browser.NewPubAPISpecBrowser(searchEngine, www)
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
	fmt.Printf("\nGot %d results", len(results))
	for _, result := range results {
		fmt.Printf("\n%s (score: %f)", result.WebPage.Title, result.Score)
	}
	fmt.Println()
}
