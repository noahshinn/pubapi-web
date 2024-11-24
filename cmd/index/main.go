package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"search_engine/index"
	"search_engine/www"
)

func main() {
	ctx := context.Background()
	endpointsPath := flag.String("endpoints-path", "", "Path to a json file containing endpoints")
	maxConcurrency := flag.Int("max-concurrency", 8, "Maximum number of concurrent processes")
	outputPath := flag.String("output-path", "", "File to output indexed docs to")
	flag.Parse()

	if *endpointsPath == "" {
		log.Fatal("Please provide a path to endpoints.txt using the -endpoints-path flag")
	}
	if *outputPath == "" {
		log.Fatal("Please provide a path to output the indexed docs to using the -output-path flag")
	}
	endpoints := []*www.Endpoint{}
	endpointsBytes, err := os.ReadFile(*endpointsPath)
	if err != nil {
		log.Fatalf("Error reading endpoints: %v", err)
	}
	if err := json.Unmarshal(endpointsBytes, &endpoints); err != nil {
		log.Fatalf("Error unmarshalling endpoints: %v", err)
	}
	endpointToWebPage := []*index.EndpointAndWebPage{}
	for _, endpoint := range endpoints {
		client := &http.Client{}
		req, err := http.NewRequest("GET", endpoint.URL(), nil)
		if err != nil {
			log.Fatalf("Error creating request: %v", err)
		}
		resp, err := client.Do(req)
		if err != nil {
			log.Fatalf("Error making request: %v", err)
		}
		defer resp.Body.Close()
		var specMap map[string]any
		if err := json.NewDecoder(resp.Body).Decode(&specMap); err != nil {
			log.Fatalf("Error decoding response: %v", err)
		}
		title, ok := specMap["info"].(map[string]any)["title"]
		if !ok {
			log.Fatalf("Error getting title from spec: %v", specMap)
		}
		if title == nil {
			log.Fatalf("Title is nil for spec: %v", specMap)
		}
		if titleStr, ok := title.(string); !ok {
			log.Fatalf("Title is not a string: %v", title)
		} else {
			endpointToWebPage = append(endpointToWebPage, &index.EndpointAndWebPage{Endpoint: endpoint, WebPage: www.NewWebPage(titleStr, specMap)})
		}
	}

	indexedDocs, err := index.IndexWebPages(ctx, endpointToWebPage, &index.IndexOptions{
		MaxConcurrency: *maxConcurrency,
	})
	if err != nil {
		log.Fatalf("Error indexing docs: %v", err)
	}
	f, err := os.Create(*outputPath)
	if err != nil {
		log.Fatalf("Error creating indexed docs file: %v", err)
	}
	defer f.Close()

	encoder := json.NewEncoder(f)
	err = encoder.Encode(indexedDocs)
	if err != nil {
		log.Fatalf("Error encoding indexed docs: %v", err)
	}
	fmt.Printf("Indexed %d docs and saved to %s\n", len(indexedDocs), *outputPath)
}
