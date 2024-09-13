package embedding

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"search_engine/primitives/model/models"
)

const OPENAI_API_URL = "https://api.openai.com/v1"
const OPENAI_API_KEY_ENV_VAR = "OPENAI_API_KEY"
const DEFAULT_OPENAI_MODEL = "text-embedding-3-large"

type OpenAIEmbeddingModel struct {
	ApiKey string
	Model  string
}

type OpenAIEmbeddingModelOptions struct {
	Model  string
	ApiKey string
}

func NewOpenAIEmbeddingModel(options *OpenAIEmbeddingModelOptions) (models.EmbeddingModel, error) {
	var key string
	model := DEFAULT_OPENAI_MODEL
	if options != nil {
		if options.Model != "" {
			model = options.Model
		}
		if options.ApiKey != "" {
			key = options.ApiKey
		}
	}
	if key == "" {
		key = os.Getenv(OPENAI_API_KEY_ENV_VAR)
		if key == "" {
			return nil, fmt.Errorf("API key not provided, set %s or pass in OpenAIEmbeddingModelOptions", OPENAI_API_KEY_ENV_VAR)
		}
	}
	return &OpenAIEmbeddingModel{
		ApiKey: key,
		Model:  model,
	}, nil
}

func apiRequest(ctx context.Context, apiKey string, endpoint string, args map[string]any) (map[string]any, error) {
	if encoded, err := json.Marshal(args); err != nil {
		return nil, err
	} else if request, err := http.NewRequestWithContext(ctx, "POST", OPENAI_API_URL+endpoint, bytes.NewBuffer(encoded)); err != nil {
		return nil, err
	} else {
		request.Header.Set("Content-Type", "application/json; charset=utf-8")
		request.Header.Set("Authorization", "Bearer "+apiKey)
		client := &http.Client{}
		response, err := client.Do(request)
		if err != nil {
			return nil, err
		} else if responseBody, err := io.ReadAll(response.Body); err != nil {
			return nil, err
		} else {
			result := map[string]any{}
			if err := json.Unmarshal(responseBody, &result); err != nil {
				return nil, err
			}
			if err, ok := result["error"].(map[string]any); ok {
				return nil, fmt.Errorf("OpenAI error: %s", err["message"].(string))
			}
			return result, nil
		}
	}
}

func (m *OpenAIEmbeddingModel) GetEmbedding(ctx context.Context, text string) ([]float64, error) {
	args := map[string]any{
		"model": m.Model,
		"input": text,
	}
	if response, err := apiRequest(ctx, m.ApiKey, "/embeddings", args); err != nil {
		return nil, err
	} else if embedding, err := parseEmbeddingsResponse(response); err != nil {
		return nil, err
	} else {
		return embedding, nil
	}
}

func parseEmbeddingsResponse(response map[string]any) ([]float64, error) {
	if data, ok := response["data"].([]any); !ok {
		return nil, fmt.Errorf("invalid embeddings response; missing choices")
	} else if len(data) != 1 {
		return nil, fmt.Errorf("invalid embeddings response; number of embeddings does not match input")
	} else if object, ok := data[0].(map[string]any); !ok {
		return nil, fmt.Errorf("invalid embedding; embedding is not a JSON object")
	} else if values, ok := object["embedding"].([]any); !ok {
		return nil, fmt.Errorf("invalid embedding; missing embedding array")
	} else {
		embedding := make([]float64, len(values))
		for j, value := range values {
			if number, ok := value.(float64); !ok {
				return nil, fmt.Errorf("invalid embedding; number is not a float")
			} else {
				embedding[j] = number
			}
		}
		return embedding, nil
	}
}
