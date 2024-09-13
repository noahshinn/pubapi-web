package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"sync"

	"search_engine/datagen"
	"search_engine/primitives/api"
)

func main() {
	ctx := context.Background()
	companiesFile := flag.String("companies-file", "", "Path to the file containing company names")
	n := flag.Int("n", 0, "Maximum number of companies to process")
	maxConcurrency := flag.Int("max-concurrency", 5, "Maximum number of concurrent requests")
	output := flag.String("output", "", "Output directory for generated specs")
	flag.Parse()

	api := api.DefaultAPI()
	if *companiesFile == "" || *output == "" {
		log.Fatal("Please provide both companies_file and output arguments")
	}
	companies, err := readCompanies(*companiesFile)
	if err != nil {
		log.Fatalf("Error reading companies file: %v", err)
	}
	if *n > 0 && *n < len(companies) {
		companies = companies[:*n]
	}
	if err := os.MkdirAll(*output, 0755); err != nil {
		log.Fatalf("Error creating output directory: %v", err)
	}
	var wg sync.WaitGroup
	semaphore := make(chan struct{}, *maxConcurrency)

	for _, company := range companies {
		wg.Add(1)
		go func(company string) {
			defer wg.Done()
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			path, err := datagen.ProcessCompany(ctx, company, *output, api)
			if err != nil {
				log.Printf("Error processing company %s: %v", company, err)
			}
			fmt.Printf("Generated spec for %s at %s\n", company, path)
		}(company)
	}
	wg.Wait()
	fmt.Println("All companies processed")
}

func readCompanies(filename string) ([]string, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var companies []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		companies = append(companies, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return companies, nil
}
