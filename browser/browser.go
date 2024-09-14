package browser

import (
	"context"
	"encoding/json"
	"search_engine/search"
	"search_engine/www"
)

type Browser interface {
	Search(ctx context.Context, query string, options *search.SearchOptions) ([]*search.SearchResult, error)
	Navigate(ctx context.Context, address int) (string, error)
	Execute(ctx context.Context, address int, endpoint string, body map[string]any) (string, error)
}

type PubAPISpecBrowser struct {
	searchEngine   search.SearchEngine
	web            *www.WWW
	maxConcurrency int
}

func (b *PubAPISpecBrowser) Search(ctx context.Context, query string, options *search.SearchOptions) ([]*search.SearchResult, error) {
	return b.searchEngine.Search(ctx, query, options)
}

func (b *PubAPISpecBrowser) Navigate(ctx context.Context, address int) (string, error) {
	machine, err := b.web.Get(address)
	if err != nil {
		return "", err
	}
	webPage, err := machine.Request()
	if err != nil {
		return "", err
	}
	jsonBytes, err := json.Marshal(webPage.Content)
	if err != nil {
		return "", err
	}
	return string(jsonBytes), nil
}

type BrowserOptions struct {
	MaxConcurrency int
}

const defaultMaxConcurrency = 1

func NewPubAPISpecBrowser(searchEngine search.SearchEngine, web *www.WWW, options *BrowserOptions) (Browser, error) {
	maxConcurrency := defaultMaxConcurrency
	if options != nil && options.MaxConcurrency > 0 {
		maxConcurrency = options.MaxConcurrency
	}
	return &PubAPISpecBrowser{searchEngine: searchEngine, web: web, maxConcurrency: maxConcurrency}, nil
}

func (b *PubAPISpecBrowser) Execute(ctx context.Context, address int, endpoint string, body map[string]any) (string, error) {
	// TODO: implement
	return "", nil
}
