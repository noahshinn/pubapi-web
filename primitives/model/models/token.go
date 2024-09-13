package models

import (
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
		numTokens += ApproxNumTokens(message.Content)
	}
	return numTokens
}
