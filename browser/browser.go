package browser

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"search_engine/search"
	"search_engine/www"
)

type Browser interface {
	Search(ctx context.Context, query string, options *search.SearchOptions) ([]*search.SearchResult, error)
	Navigate(ctx context.Context, endpoint *www.Endpoint) (string, error)
	Execute(ctx context.Context, endpoint *www.Endpoint, body map[string]any) (string, error)
	GetLocation(ctx context.Context) (*GeoLocation, error)
}

type BaseBrowser struct {
	searchEngine   search.SearchEngine
	maxConcurrency int
}

func (b *BaseBrowser) Search(ctx context.Context, query string, options *search.SearchOptions) ([]*search.SearchResult, error) {
	return b.searchEngine.Search(ctx, query, options)
}

func (b *BaseBrowser) Navigate(ctx context.Context, endpoint *www.Endpoint) (string, error) {
	client := &http.Client{}
	req, err := http.NewRequestWithContext(ctx, "GET", fmt.Sprintf("http://%s:%d/", endpoint.IpAddress, endpoint.Port), nil)
	if err != nil {
		return "", err
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var content map[string]any
	err = json.NewDecoder(resp.Body).Decode(&content)
	if err != nil {
		return "", err
	}
	jsonBytes, err := json.Marshal(content)
	if err != nil {
		return "", err
	}
	return string(jsonBytes), nil
}

type BrowserOptions struct {
	MaxConcurrency int
}

const defaultMaxConcurrency = 1

func NewBaseBrowser(searchEngine search.SearchEngine, options *BrowserOptions) (Browser, error) {
	maxConcurrency := defaultMaxConcurrency
	if options != nil && options.MaxConcurrency > 0 {
		maxConcurrency = options.MaxConcurrency
	}
	return &BaseBrowser{searchEngine: searchEngine, maxConcurrency: maxConcurrency}, nil
}

func (b *BaseBrowser) Execute(ctx context.Context, endpoint *www.Endpoint, body map[string]any) (string, error) {
	// TODO: implement
	return "", nil
}

func (b *BaseBrowser) GetLocation(ctx context.Context) (*GeoLocation, error) {
	return getLocationFromIP()
}
