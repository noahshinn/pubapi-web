package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
)

const OPENAI_API_URL = "https://api.openai.com/v1"

type OpenAIModel struct {
	modelID ChatModelID
	apiKey  string
}

type OpenAIEmbeddingModel struct {
	modelID EmbeddingModelID
	apiKey  string
}

func NewOpenAIChatModel(modelID ChatModelID, apiKey string) ChatModel {
	return &OpenAIModel{modelID: modelID, apiKey: apiKey}
}

func NewOpenAIEmbeddingModel(embeddingModelID EmbeddingModelID, apiKey string) EmbeddingModel {
	return &OpenAIEmbeddingModel{modelID: embeddingModelID, apiKey: apiKey}
}

func (m *OpenAIModel) Message(ctx context.Context, messages []*Message, options *MessageOptions) (*Message, error) {
	args := m.buildArgs(messages, options)
	if response, err := apiRequest(ctx, m.apiKey, "/chat/completions", args); err != nil {
		return nil, err
	} else {
		return parseMessageResponse(response)
	}
}

func (m *OpenAIModel) ContextLength() int {
	switch m.modelID {
	case ChatModelGPT4o:
		return 128000
	default:
		return 128000
	}
}

func (m *OpenAIModel) buildArgs(messages []*Message, options *MessageOptions) map[string]any {
	jsonMessages := []map[string]string{}
	for _, message := range messages {
		jsonMessage := map[string]string{
			"role":    string(message.Role),
			"content": message.Content,
		}
		if message.Name != "" {
			jsonMessage["name"] = message.Name
		}
		jsonMessages = append(jsonMessages, jsonMessage)
	}
	args := map[string]any{
		"model":    m.modelID,
		"messages": jsonMessages,
	}
	if options != nil {
		if options.MaxTokens > 0 {
			args["max_tokens"] = options.MaxTokens
		}
		if len(options.StopSequences) > 0 {
			args["stop"] = options.StopSequences
		}
		if options.Temperature != 0 {
			args["temperature"] = options.Temperature
		}
	}
	return args
}

type Error struct {
	Code    string
	Message string
}

func (e *Error) Error() string {
	return e.Message
}

func parseMessageResponse(response map[string]any) (*Message, error) {
	if choices, ok := response["choices"].([]any); !ok {
		return nil, &Error{Message: "invalid response, no choices"}
	} else if len(choices) != 1 {
		return nil, &Error{Message: "invalid response, expected 1 choice"}
	} else if choice, ok := choices[0].(map[string]any); !ok {
		return nil, &Error{Message: "invalid response, choice is not a map"}
	} else if message, ok := choice["message"].(map[string]any); !ok {
		return nil, &Error{Message: "invalid response, message is not a map"}
	} else if content, ok := message["content"].(string); ok {
		return &Message{
			Role:    MessageRole(message["role"].(string)),
			Content: content,
		}, nil
	}
	return nil, &Error{Message: "invalid response, no content"}
}

func parseEmbeddingsResponse(texts []string, response map[string]any) ([][]float64, error) {
	embeddings := make([][]float64, len(texts))
	if data, ok := response["data"].([]any); !ok {
		return nil, &Error{Message: "invalid embeddings response; missing choices"}
	} else if len(data) != len(texts) {
		return nil, &Error{Message: "invalid embeddings response; number of embeddings does not match input"}
	} else {
		for i, body := range data {
			if object, ok := body.(map[string]any); !ok {
				return nil, &Error{Message: "invalid embedding; embedding is not a JSON object"}
			} else if values, ok := object["embedding"].([]any); !ok {
				return nil, &Error{Message: "invalid embedding; missing embedding array"}
			} else {
				embedding := make([]float64, len(values))
				for j, value := range values {
					if number, ok := value.(float64); !ok {
						return nil, &Error{Message: "invalid embedding; number is not a float"}
					} else {
						embedding[j] = number
					}
				}
				embeddings[i] = embedding
			}
		}
		return embeddings, nil
	}
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
				response := Error{Message: "OpenAI error"}
				if value, ok := err["code"].(string); ok {
					response.Code = value
				}
				if value, ok := err["message"].(string); ok {
					response.Message = value
				}
				return nil, &response
			}
			return result, nil
		}
	}
}

func (m *OpenAIEmbeddingModel) Embedding(ctx context.Context, texts []string) ([][]float64, error) {
	args := map[string]any{
		"model": m.modelID,
		"input": texts,
	}
	if response, err := apiRequest(ctx, m.apiKey, "/embeddings", args); err != nil {
		return nil, err
	} else if embeddings, err := parseEmbeddingsResponse(texts, response); err != nil {
		return nil, err
	} else {
		return embeddings, nil
	}
}
