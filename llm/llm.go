package llm

import (
	"context"
)

type ChatModelID string
type EmbeddingModelID string

const (
	ChatModelGPT4o            ChatModelID      = "gpt-4o"
	EmbeddingModelOpenAILarge EmbeddingModelID = "text-embedding-3-large"
)

type Models struct {
	DefaultChatModel      ChatModel
	DefaultEmbeddingModel EmbeddingModel
	ChatModels            map[ChatModelID]ChatModel
	EmbeddingModels       map[EmbeddingModelID]EmbeddingModel
}

func AllModels(api_key string) *Models {
	return &Models{
		DefaultChatModel:      NewOpenAIChatModel(ChatModelGPT4o, api_key),
		DefaultEmbeddingModel: NewOpenAIEmbeddingModel(EmbeddingModelOpenAILarge, api_key),
		ChatModels: map[ChatModelID]ChatModel{
			ChatModelGPT4o: NewOpenAIChatModel(ChatModelGPT4o, api_key),
		},
		EmbeddingModels: map[EmbeddingModelID]EmbeddingModel{
			EmbeddingModelOpenAILarge: NewOpenAIEmbeddingModel(EmbeddingModelOpenAILarge, api_key),
		},
	}
}

type FunctionDef struct {
	Name        string     `json:"name"`
	Description string     `json:"description"`
	Parameters  Parameters `json:"parameters"`
}

type Parameters struct {
	Type       string              `json:"type"`
	Properties map[string]Property `json:"properties"`
	Required   []string            `json:"required"`
}

type Property struct {
	Type        string      `json:"type"`
	Description string      `json:"description"`
	Items       *ArrayItems `json:"items,omitempty"`
}

type ArrayItems struct {
	Type string `json:"type"`
}

type FunctionCall struct {
	Name      string
	Arguments string
}

type Message struct {
	Role         MessageRole   `json:"role"`
	Content      string        `json:"content"`
	Name         string        `json:"name"`
	FunctionCall *FunctionCall `json:"function_call"`
}

type MessageRole string

const (
	MessageRoleSystem    MessageRole = "system"
	MessageRoleUser      MessageRole = "user"
	MessageRoleAssistant MessageRole = "assistant"
)

type StreamEvent struct {
	Text  string
	Error error
}

type MessageOptions struct {
	Temperature   float32  `json:"temperature"`
	MaxTokens     int      `json:"max_tokens"`
	StopSequences []string `json:"stop_sequences"`
}

type ChatModel interface {
	Message(ctx context.Context, messages []*Message, options *MessageOptions) (*Message, error)
	ContextLength() int
}

type EmbeddingModel interface {
	Embedding(ctx context.Context, texts []string) ([][]float64, error)
}
