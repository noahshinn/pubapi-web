package search

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"path/filepath"
	"search_engine/cache"
	"search_engine/index"
	"search_engine/primitives/api"
	"search_engine/www"
)

type SearchEngine interface {
	Search(ctx context.Context, query string, options *SearchOptions) ([]*SearchResult, error)
	RefreshIndex(ctx context.Context, web *www.WWW, addresses []int, options *RefreshIndexOptions) error
}

type PubAPISearchEngine struct {
	api   *api.API
	index []*index.Document
	// a cache of json-serialized web pages to documents
	cache cache.DiskCache
}

func (e *PubAPISearchEngine) Search(ctx context.Context, query string, options *SearchOptions) ([]*SearchResult, error) {
	if e.index == nil {
		return nil, errors.New("index is not initialized")
	}
	if len(e.index) == 0 {
		return nil, errors.New("index is empty")
	}
	return Search(ctx, e.index, query, e.api, options)
}

type RefreshIndexOptions struct {
	MaxConcurrency int
}

func (e *PubAPISearchEngine) RefreshIndex(ctx context.Context, web *www.WWW, addresses []int, options *RefreshIndexOptions) error {
	maxConcurrency := defaultMaxConcurrency
	if options != nil && options.MaxConcurrency != 0 {
		maxConcurrency = options.MaxConcurrency
	}

	webIndex := make([]*index.Document, 0, len(addresses))
	errChan := make(chan error, len(addresses))
	resultChan := make(chan *index.Document, len(addresses))

	semaphore := make(chan struct{}, maxConcurrency)

	for _, address := range addresses {
		go func(addr int) {
			semaphore <- struct{}{}
			defer func() { <-semaphore }()
			machine, err := web.Get(addr)
			if err != nil {
				errChan <- err
				return
			}
			webPage, err := machine.Request()
			if err != nil {
				errChan <- err
				return
			}
			webPageJsonBytes, err := json.Marshal(webPage.Content)
			if err != nil {
				errChan <- err
				return
			}
			cacheKey := fmt.Sprintf("web-page-content-%s", string(webPageJsonBytes))
			if doc, err := e.cache.Get(cacheKey); err == nil {
				docBytes, err := json.Marshal(doc)
				if err != nil {
					errChan <- err
					return
				}
				var d index.Document
				err = json.Unmarshal(docBytes, &d)
				if err != nil {
					errChan <- err
					return
				}
				resultChan <- &d
			} else {
				docs, err := index.IndexWebPages(ctx, []*index.AddressAndWebPage{{Address: addr, WebPage: webPage}}, e.api, 1)
				if err != nil {
					errChan <- err
					return
				}
				doc := docs[0]
				e.cache.Set(cacheKey, doc)
				resultChan <- doc
			}
		}(address)
	}
	for i := 0; i < len(addresses); i++ {
		select {
		case err := <-errChan:
			return err
		case doc := <-resultChan:
			webIndex = append(webIndex, doc)
		}
	}
	e.index = webIndex
	err := e.cache.SaveToDisk()
	if err != nil {
		return err
	}
	log.Println("Cached recent index")
	return nil
}

func NewPubAPISearchEngine(a *api.API, web *www.WWW) (SearchEngine, error) {
	se := &PubAPISearchEngine{api: a}
	cachePath, err := se.getRootCachePath()
	if err != nil {
		return nil, err
	}
	c, err := cache.NewDiskCacheFromPath(cachePath)
	if err != nil {
		return nil, err
	}
	return &PubAPISearchEngine{api: a, cache: c}, nil
}

func (e *PubAPISearchEngine) getRootCachePath() (string, error) {
	cacheRoot, err := cache.GetCacheRootPath()
	if err != nil {
		return "", err
	}
	return filepath.Join(cacheRoot, "search-engine"), nil
}
