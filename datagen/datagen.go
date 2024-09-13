package datagen

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"search_engine/primitives/api"
	"strings"
)

func GenerateOpenAPISpec(ctx context.Context, company string, api *api.API) (string, error) {
	instruction := "Generate an Open API spec for the company. It should be complete and comprehensive. Put the entire spec in a JSON code block."
	response, err := api.Generate(ctx, instruction, fmt.Sprintf("Company: %s", company), nil)
	if err != nil {
		return "", err
	}

	jsonSpec, err := extractJSON(response)
	if err != nil {
		return "", err
	}

	return jsonSpec, nil
}

func extractJSON(text string) (string, error) {
	re := regexp.MustCompile("```json\n(.*?)\n```")
	match := re.FindStringSubmatch(text)
	if len(match) > 1 {
		return match[1], nil
	}
	return "", errors.New("no JSON code block found in the response")
}

func ProcessCompany(ctx context.Context, company string, outputDir string, api *api.API) (string, error) {
	fmt.Printf("Generating OpenAPI spec for %s...\n", company)
	response, err := GenerateOpenAPISpec(ctx, company, api)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return "", err
	}

	jsonSpec, err := extractJSON(response)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return "", err
	}
	var specDict map[string]interface{}
	err = json.Unmarshal([]byte(jsonSpec), &specDict)
	if err != nil {
		fmt.Printf("Error: Invalid JSON received for %s\n", company)
		fmt.Printf("Response content: %s...\n", jsonSpec[:500])
		return "", err
	}

	re := regexp.MustCompile("[^a-z0-9]")
	filename := re.ReplaceAllString(strings.ToLower(company), "") + ".json"
	fp := filepath.Join(outputDir, filename)

	file, err := os.Create(fp)
	if err != nil {
		fmt.Printf("Error creating file for %s: %v\n", company, err)
		return "", err
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	err = encoder.Encode(specDict)
	if err != nil {
		fmt.Printf("Error writing JSON to file for %s: %v\n", company, err)
		return "", err
	}
	fmt.Printf("Spec for %s saved to %s\n", company, fp)
	return fp, nil
}
