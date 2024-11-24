package search

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"path/filepath"
	"search_engine/cache"
	"search_engine/index"
	"search_engine/primitives/api"
	"search_engine/www"
)

type SearchEngine interface {
	Search(ctx context.Context, query string, options *SearchOptions) ([]*SearchResult, error)
	RefreshIndex(ctx context.Context, endpoints []*www.Endpoint, options *RefreshIndexOptions) error
}

type DenseEmbeddingSearchEngine struct {
	modelAPI *api.ModelAPI
	index    []*index.Document
	// a cache of json-serialized web pages to documents
	cache cache.DiskCache
}

func (e *DenseEmbeddingSearchEngine) Search(ctx context.Context, query string, options *SearchOptions) ([]*SearchResult, error) {
	if e.index == nil {
		return nil, errors.New("index is not initialized")
	}
	if len(e.index) == 0 {
		return nil, errors.New("index is empty")
	}
	return Search(ctx, e.index, query, options)
}

type RefreshIndexOptions struct {
	MaxConcurrency int
}

func (e *DenseEmbeddingSearchEngine) RefreshIndex(ctx context.Context, endpoints []*www.Endpoint, options *RefreshIndexOptions) error {
	maxConcurrency := defaultMaxConcurrency
	if options != nil && options.MaxConcurrency != 0 {
		maxConcurrency = options.MaxConcurrency
	}

	webIndex := make([]*index.Document, 0, len(endpoints))
	errChan := make(chan error, len(endpoints))
	resultChan := make(chan *index.Document, len(endpoints))

	semaphore := make(chan struct{}, maxConcurrency)

	for _, endpoint := range endpoints {
		go func(endpoint *www.Endpoint) {
			semaphore <- struct{}{}
			defer func() { <-semaphore }()
			client := &http.Client{}
			req, err := http.NewRequest("GET", fmt.Sprintf("http://%s:%d/", endpoint.IpAddress, endpoint.Port), nil)
			if err != nil {
				errChan <- err
				return
			}

			resp, err := client.Do(req)
			if err != nil {
				errChan <- err
				return
			}
			defer resp.Body.Close()

			var webPage www.WebPage
			decoder := json.NewDecoder(resp.Body)
			if err := decoder.Decode(&webPage.Content); err != nil {
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
				docs, err := index.IndexWebPages(ctx, []*index.EndpointAndWebPage{{Endpoint: endpoint, WebPage: &webPage}}, &index.IndexOptions{
					MaxConcurrency: maxConcurrency,
					ModelAPI:       e.modelAPI,
				})
				if err != nil {
					errChan <- err
					return
				}
				doc := docs[0]
				e.cache.Set(cacheKey, doc)
				resultChan <- doc
			}
		}(endpoint)
	}
	for i := 0; i < len(endpoints); i++ {
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

type DenseEmbeddingSearchEngineOptions struct {
	ModelAPI *api.ModelAPI
}

func NewDenseEmbeddingSearchEngine(searchIndex []*index.Document, options *DenseEmbeddingSearchEngineOptions) (SearchEngine, error) {
	modelAPI := api.DefaultModelAPI()
	if options != nil {
		if options.ModelAPI != nil {
			modelAPI = options.ModelAPI
		}
	}
	se := &DenseEmbeddingSearchEngine{modelAPI: modelAPI, index: searchIndex}
	cachePath, err := se.getRootCachePath()
	if err != nil {
		return nil, err
	}
	c, err := cache.NewDiskCacheFromPath(cachePath)
	if err != nil {
		return nil, err
	}
	se.cache = c
	return se, nil
}

func (e *DenseEmbeddingSearchEngine) getRootCachePath() (string, error) {
	cacheRoot, err := cache.GetCacheRootPath()
	if err != nil {
		return "", err
	}
	return filepath.Join(cacheRoot, "search-engine"), nil
}
