package chat

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"

	"search_engine/primitives/datapoint"
	"search_engine/primitives/model/models"
	"search_engine/utils/slicesx"
)

const OPENAI_API_URL = "https://api.openai.com/v1"
const OPENAI_API_KEY_ENV_VAR = "OPENAI_API_KEY"
const DEFAULT_OPENAI_MODEL = "gpt-4o-2024-08-06"

type OpenAIModel struct {
	ApiKey      string
	Model       string
	Temperature float64
}

type OpenAIModelOptions struct {
	ApiKey      string
	Temperature float64
}

func NewOpenAIModel(model string, options *OpenAIModelOptions) (models.GeneralModel, error) {
	var key string
	var temperature float64 = 0.0
	if options != nil {
		if options.ApiKey != "" {
			key = options.ApiKey
		}
		if options.Temperature > 0 {
			temperature = options.Temperature
		}
	}
	if key == "" {
		key = os.Getenv(OPENAI_API_KEY_ENV_VAR)
		if key == "" {
			return nil, fmt.Errorf("API key not provided, set %s or pass in OpenAIModelOptions", OPENAI_API_KEY_ENV_VAR)
		}
	}
	return &OpenAIModel{
		ApiKey:      key,
		Model:       model,
		Temperature: temperature,
	}, nil
}

func (m *OpenAIModel) BinaryClassify(ctx context.Context, instruction string, text string, examples []*datapoint.BinaryClassifyDatapoint) (bool, error) {
	classifyExamples := slicesx.Map(examples, func(dp *datapoint.BinaryClassifyDatapoint, _ int) *datapoint.ClassifyDatapoint {
		var response int
		if dp.Response != nil {
			if *dp.Response {
				response = 0
			} else {
				response = 1
			}
		}
		return &datapoint.ClassifyDatapoint{
			Instruction: dp.Instruction,
			Text:        dp.Text,
			Options:     []string{"true", "false"},
			Response:    &response,
		}
	})
	res, err := m.Classify(ctx, instruction, text, []string{"true", "false"}, classifyExamples)
	if err != nil {
		return false, err
	}
	return res == 0, nil
}

func (m *OpenAIModel) Classify(ctx context.Context, instruction string, text string, options []string, examples []*datapoint.ClassifyDatapoint) (int, error) {
	messages, decodeMap, err := BuildClassifyState(instruction, text, options, examples)
	if err != nil {
		return 0, err
	}
	if response, err := m.message(ctx, messages, &models.MessageOptions{
		Temperature: m.Temperature,
		MaxTokens:   128,
		ResponseFormat: &models.ResponseFormat{
			Type: models.ResponseFormatTypeJSONObject,
		},
	}); err != nil {
		return 0, err
	} else {
		return HandleClassifyResponse(response, decodeMap)
	}
}

func (m *OpenAIModel) ParseForce(ctx context.Context, instruction string, text string, v any, examples []*datapoint.ParseForceDatapoint) error {
	messages, err := BuildParseForceState(instruction, text, v, examples)
	if err != nil {
		return err
	}
	if response, err := m.message(ctx, messages, &models.MessageOptions{
		Temperature: m.Temperature,
		ResponseFormat: &models.ResponseFormat{
			Type: models.ResponseFormatTypeJSONObject,
		},
	}); err != nil {
		return err
	} else {
		return HandleParseForceResponse(response, v)
	}
}

func (m *OpenAIModel) Score(ctx context.Context, instruction string, text string, min int, max int, examples []*datapoint.ScoreDatapoint) (int, error) {
	messages, err := BuildScoreState(instruction, text, min, max, examples)
	if err != nil {
		return 0, err
	}
	if response, err := m.message(ctx, messages, &models.MessageOptions{
		Temperature: m.Temperature,
		ResponseFormat: &models.ResponseFormat{
			Type: models.ResponseFormatTypeJSONObject,
		},
	}); err != nil {
		return 0, err
	} else {
		return HandleScoreResponse(response)
	}
}

func (m *OpenAIModel) Generate(ctx context.Context, instruction string, text string, examples []*datapoint.GenerateDatapoint) (string, error) {
	messages, err := BuildGenerateState(instruction, text, examples)
	if err != nil {
		return "", err
	}
	if response, err := m.message(ctx, messages, &models.MessageOptions{
		Temperature: m.Temperature,
		ResponseFormat: &models.ResponseFormat{
			Type: models.ResponseFormatTypeText,
		},
	}); err != nil {
		return "", err
	} else {
		return response.Content, nil
	}
}

func (m *OpenAIModel) message(ctx context.Context, messages []*models.Message, options *models.MessageOptions) (*models.Message, error) {
	args := m.buildArgs(messages, options)
	if response, err := apiRequest(ctx, m.ApiKey, "/chat/completions", args); err != nil {
		return nil, err
	} else {
		return parseMessageResponse(response)
	}
}

func (m *OpenAIModel) buildArgs(messages []*models.Message, options *models.MessageOptions) map[string]any {
	jsonMessages := []map[string]string{}
	for _, message := range messages {
		jsonMessage := map[string]string{
			"role":    string(message.Role),
			"content": message.Content,
		}
		jsonMessages = append(jsonMessages, jsonMessage)
	}
	args := map[string]any{
		"model":    m.Model,
		"messages": jsonMessages,
	}
	if options != nil {
		if options.Temperature > 0 {
			args["temperature"] = options.Temperature
		}
		if options.MaxTokens > 0 {
			args["max_tokens"] = options.MaxTokens
		}
		if len(options.StopSequences) > 0 {
			args["stop"] = options.StopSequences
		}
		if options.ResponseFormat != nil {
			args["response_format"] = map[string]string{
				"type": string(options.ResponseFormat.Type),
			}
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

func parseMessageResponse(response map[string]any) (*models.Message, error) {
	if choices, ok := response["choices"].([]any); !ok {
		return nil, &Error{Message: "invalid response, no choices"}
	} else if len(choices) != 1 {
		return nil, &Error{Message: "invalid response, expected 1 choice"}
	} else if choice, ok := choices[0].(map[string]any); !ok {
		return nil, &Error{Message: "invalid response, choice is not a map"}
	} else if message, ok := choice["message"].(map[string]any); !ok {
		return nil, &Error{Message: "invalid response, message is not a map"}
	} else if content, ok := message["content"].(string); ok {
		return &models.Message{
			Role:    models.MessageRole(message["role"].(string)),
			Content: content,
		}, nil
	}
	return nil, &Error{Message: "invalid response, no content or function call"}
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
