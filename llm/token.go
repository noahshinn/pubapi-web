package llm

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/tiktoken-go/tokenizer/codec"
)

var tokenizer = codec.NewCl100kBase()

func ApproxNumTokens(text string) int {
	tokens, _, err := tokenizer.Encode(text)
	if err != nil {
		// approximation
		wc := len(strings.Split(text, " ")) * 4 / 3
		cc := len(text) / 4
		return (wc + cc) / 2
	}
	return len(tokens)
}

func ApproxNumTokensFast(text string) int {
	return len(text) / 4
}

func ApproxNumTokensInMessages(messages []*Message) int {
	numTokens := 0
	for _, message := range messages {
		if message.FunctionCall != nil {
			numTokens += ApproxNumTokens(fmt.Sprintf("%s(%s)", message.FunctionCall.Name, message.FunctionCall.Arguments))
		} else {
			numTokens += ApproxNumTokens(message.Content)
		}
	}
	return numTokens
}

func ApproxNumTokensInFunctionDefs(functionDefs []*FunctionDef) int {
	numTokens := 0
	for _, functionDef := range functionDefs {
		b, err := json.Marshal(functionDef)
		if err != nil {
			b = []byte(fmt.Sprintf("%v", functionDef))
		}
		numTokens += ApproxNumTokens(string(b))
	}
	return numTokens
}
