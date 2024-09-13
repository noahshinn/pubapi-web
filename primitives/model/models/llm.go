package models

type Message struct {
	Role    MessageRole `json:"role"`
	Content string      `json:"content"`
}

type MessageRole string

const (
	MessageRoleSystem    MessageRole = "system"
	MessageRoleUser      MessageRole = "user"
	MessageRoleAssistant MessageRole = "assistant"
)

type ResponseFormatType string

const (
	ResponseFormatTypeText       ResponseFormatType = "text"
	ResponseFormatTypeJSONObject ResponseFormatType = "json_object"
)

type ResponseFormat struct {
	Type ResponseFormatType
}

type MessageOptions struct {
	Temperature    float64         `json:"temperature"`
	MaxTokens      int             `json:"max_tokens"`
	StopSequences  []string        `json:"stop_sequences"`
	ResponseFormat *ResponseFormat `json:"response_format"`
}
