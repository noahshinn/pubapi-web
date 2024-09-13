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
}

type PubAPISpecBrowser struct {
	searchEngine search.SearchEngine
	web          *www.WWW
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

func NewPubAPISpecBrowser(searchEngine search.SearchEngine, web *www.WWW) (Browser, error) {
	return &PubAPISpecBrowser{searchEngine: searchEngine, web: web}, nil
}
