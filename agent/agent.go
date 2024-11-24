package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"search_engine/browser"
	"search_engine/primitives/api"
	"search_engine/search"
	"search_engine/utils/slicesx"
	"strings"
)

type BrowserAgent interface {
	Solve(ctx context.Context, query string, b browser.Browser) (string, error)
}

type BrowserAction struct {
	Address  int
	Endpoint string
	Body     map[string]any
}

type LLMBrowserAgent struct {
	api *api.ModelAPI
}

func (a *LLMBrowserAgent) determinePageToVisit(ctx context.Context, query string, results []*search.SearchResult) (foundPage bool, pageIdxToVisit int, err error) {
	noneOfTheAbove := "None of the above"
	options := slicesx.Map(results, func(r *search.SearchResult, _ int) string {
		return fmt.Sprintf("'%s'", r.WebPageTitle)
	})
	options = append(options, noneOfTheAbove)
	pageIdx, err := a.api.Classify(ctx, fmt.Sprintf("You will be given a set of titles for Open API specs and a query from a user. Determine the best page to visit. If no page is relevant, choose '%s'.", noneOfTheAbove), query, options, nil)
	if err != nil {
		return false, 0, err
	}
	if pageIdx == len(results) {
		return false, 0, nil
	}
	return true, pageIdx, nil
}

func (a *LLMBrowserAgent) act(ctx context.Context, query string, browserDisplay string) (*BrowserAction, error) {
	ba := &BrowserAction{}
	err := a.api.ParseForce(ctx, "You will be given an Open API spec and a query from a user. Build the correct request.", fmt.Sprintf("# Browser display\n%s\n\nUser query:\n%s", browserDisplay, query), &ba, nil)
	if err != nil {
		return nil, err
	}
	return ba, nil
}

func (a *LLMBrowserAgent) Solve(ctx context.Context, query string, b browser.Browser) (string, error) {
	results, err := b.Search(ctx, query, nil)
	if err != nil {
		return "", err
	}
	foundPage, pageIdxToVisit, err := a.determinePageToVisit(ctx, query, results)
	if err != nil {
		return "", err
	}
	if foundPage {
		userGeoLocation, err := b.GetLocation(ctx)
		if err != nil {
			return "", err
		}
		pageToVisit, err := b.Navigate(ctx, results[pageIdxToVisit].Endpoint)
		if err != nil {
			return "", err
		}
		// TODO: move this to the browser
		browserDisplay := fmt.Sprintf("## User location:\n\nLatitude: %f\nLongitude: %f\nCity: %s\nCountry: %s\n\n## Page display:\n\n%s\n", userGeoLocation.Latitude, userGeoLocation.Longitude, userGeoLocation.City, userGeoLocation.Country, pageToVisit)
		action, err := a.act(ctx, query, browserDisplay)
		if err != nil {
			return "", err
		}
		// for now, json serialize the action to take
		actionJSON, err := json.Marshal(action)
		if err != nil {
			return "", err
		}
		return string(actionJSON), nil
	} else {
		return "", fmt.Errorf("no page selected to visit from pages: %v", strings.Join(slicesx.Map(results, func(r *search.SearchResult, _ int) string {
			return r.WebPageTitle
		}), ", "))
	}
}

type LLMBrowserAgentOptions struct {
	ModelAPI *api.ModelAPI
}

func NewLLMBrowserAgent(options *LLMBrowserAgentOptions) BrowserAgent {
	modelAPI := api.DefaultModelAPI()
	if options != nil {
		if options.ModelAPI != nil {
			modelAPI = options.ModelAPI
		}
	}
	return &LLMBrowserAgent{api: modelAPI}
}
