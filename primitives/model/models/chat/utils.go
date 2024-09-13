package chat

import (
	"encoding/json"
	"fmt"
	"search_engine/primitives/datapoint"
	"search_engine/primitives/model/models"
	"search_engine/utils/jsonx"
	"strings"
)

func indexToAlpha(index int) string {
	alpha := ""
	for index >= 0 {
		alpha = string(rune(index%26+int('A'))) + alpha
		index = index/26 - 1
	}
	return alpha
}

func displayChoices(choices []string) (string, map[string]int) {
	choiceDisplays := []string{}
	decodeMap := map[string]int{}
	for i, choice := range choices {
		label := indexToAlpha(i)
		choiceDisplay := fmt.Sprintf("%s. %s", label, choice)
		choiceDisplays = append(choiceDisplays, choiceDisplay)
		decodeMap[label] = i
	}
	return strings.Join(choiceDisplays, "\n"), decodeMap
}

func BuildClassifyState(instruction string, text string, options []string, examples []*datapoint.ClassifyDatapoint) ([]*models.Message, map[string]int, error) {
	displaySample := func(instr string, t string, opts []string, response *int) ([]*models.Message, map[string]int, error) {
		choicesDisplay, decodeMap := displayChoices(opts)
		inputText := fmt.Sprintf("Instruction:\n%s\n\nText:\n%s\n\nChoices:\n%s", instr, t, choicesDisplay)
		sampleMsgs := []*models.Message{
			{
				Role:    models.MessageRoleUser,
				Content: inputText,
			},
		}
		if response != nil {
			var responseLabel string
			for label, index := range decodeMap {
				if *response == index {
					responseLabel = label
				}
			}
			if responseLabel == "" {
				return nil, nil, fmt.Errorf("response %d not found in choices", *response)
			}
			sampleMsgs = append(sampleMsgs, &models.Message{
				Role:    models.MessageRoleAssistant,
				Content: fmt.Sprintf("{\"classification\": \"%s\"}", responseLabel),
			})
		}
		return sampleMsgs, decodeMap, nil
	}
	msgs := []*models.Message{
		{
			Role:    models.MessageRoleSystem,
			Content: "Classify the following text with the provided instruction and choices. To classify, provide the key of the choice:\n{\"classification\": string}\n\nFor example, if the correct choice is 'Z. description of choice Z', then provide 'Z' as the classification as valid JSON:\n{\"classification\": \"Z\"}",
		},
	}
	for _, example := range examples {
		exampleMsgs, _, err := displaySample(instruction, example.Text, example.Options, example.Response)
		if err != nil {
			return nil, nil, err
		}
		msgs = append(msgs, exampleMsgs...)
	}
	msg, decodeMap, err := displaySample(instruction, text, options, nil)
	if err != nil {
		return nil, nil, err
	}
	return append(msgs, msg...), decodeMap, nil
}

type ClassifyResponse struct {
	Classification string `json:"classification"`
}

func HandleClassifyResponse(response *models.Message, decodeMap map[string]int) (int, error) {
	content := response.Content
	var classifyResponse ClassifyResponse
	if err := json.Unmarshal([]byte(content), &classifyResponse); err != nil {
		return 0, err
	}
	classification := classifyResponse.Classification
	index, ok := decodeMap[classification]
	if !ok {
		return 0, fmt.Errorf("classification %s not found in choices", classification)
	}
	return index, nil
}

func BuildParseForceState(instruction string, text string, v any, examples []*datapoint.ParseForceDatapoint) ([]*models.Message, error) {
	displaySample := func(instr string, t string, value any, response any) ([]*models.Message, error) {
		jsonSchemaStr, err := jsonx.ValueToJsonSchemaStr(value)
		if err != nil {
			return nil, fmt.Errorf("failed to generate JSON schema: %w", err)
		}
		inputText := fmt.Sprintf("Instruction:\n%s\n\nText:\n%s\n\nJSON Schema:\n%s", instr, t, jsonSchemaStr)
		sampleMsgs := []*models.Message{
			{
				Role:    models.MessageRoleUser,
				Content: inputText,
			},
		}
		if response != nil {
			jsonResponseData, err := json.Marshal(response)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal response: %w", err)
			}
			jsonResponseStr := string(jsonResponseData)
			sampleMsgs = append(sampleMsgs, &models.Message{
				Role:    models.MessageRoleAssistant,
				Content: jsonResponseStr,
			})
		}
		return sampleMsgs, nil
	}
	msgs := []*models.Message{
		{
			Role:    models.MessageRoleSystem,
			Content: "Parse the following text with the provided JSON schema.",
		},
	}
	for _, example := range examples {
		exampleMsgs, err := displaySample(instruction, example.Text, v, example.Response)
		if err != nil {
			return nil, err
		}
		msgs = append(msgs, exampleMsgs...)
	}
	msg, err := displaySample(instruction, text, v, nil)
	if err != nil {
		return nil, err
	}
	return append(msgs, msg...), nil
}

func HandleParseForceResponse(response *models.Message, v any) error {
	content := response.Content
	if err := json.Unmarshal([]byte(content), v); err != nil {
		return err
	}
	return nil
}

func BuildScoreState(instruction string, text string, min int, max int, examples []*datapoint.ScoreDatapoint) ([]*models.Message, error) {
	displaySample := func(instr string, t string, min int, max int, response *int) ([]*models.Message, error) {
		inputText := fmt.Sprintf("Instruction:\n%s\n\nText:\n%s\n\nRange:\n[%d, %d]", instr, t, min, max)
		sampleMsgs := []*models.Message{
			{
				Role:    models.MessageRoleUser,
				Content: inputText,
			},
		}
		if response != nil {
			sampleMsgs = append(sampleMsgs, &models.Message{
				Role:    models.MessageRoleAssistant,
				Content: fmt.Sprintf("{\"score\": %d}", *response),
			})
		}
		return sampleMsgs, nil
	}
	msgs := []*models.Message{
		{
			Role:    models.MessageRoleSystem,
			Content: "Score the following text with the provided instruction and range as an integer value in valid JSON:\n{\"score\": number}",
		},
	}
	for _, example := range examples {
		exampleMsgs, err := displaySample(instruction, example.Text, example.Min, example.Max, example.Response)
		if err != nil {
			return nil, err
		}
		msgs = append(msgs, exampleMsgs...)
	}
	msg, err := displaySample(instruction, text, min, max, nil)
	if err != nil {
		return nil, err
	}
	return append(msgs, msg...), nil
}

type ScoreResponse struct {
	Score int `json:"score"`
}

func HandleScoreResponse(response *models.Message) (int, error) {
	content := response.Content
	var scoreResponse ScoreResponse
	if err := json.Unmarshal([]byte(content), &scoreResponse); err != nil {
		return 0, fmt.Errorf("failed to unmarshal score response: %w", err)
	}
	return scoreResponse.Score, nil
}

func BuildGenerateState(instruction string, text string, examples []*datapoint.GenerateDatapoint) ([]*models.Message, error) {
	displaySample := func(instr string, t string, response *string) ([]*models.Message, error) {
		inputText := fmt.Sprintf("Instruction:\n%s\n\nText:\n%s", instr, t)
		sampleMsgs := []*models.Message{
			{
				Role:    models.MessageRoleUser,
				Content: inputText,
			},
		}
		if response != nil {
			sampleMsgs = append(sampleMsgs, &models.Message{
				Role:    models.MessageRoleAssistant,
				Content: *response,
			})
		}
		return sampleMsgs, nil
	}
	msgs := []*models.Message{}
	for _, example := range examples {
		exampleMsgs, err := displaySample(instruction, example.Text, example.Response)
		if err != nil {
			return nil, err
		}
		msgs = append(msgs, exampleMsgs...)
	}
	msg, err := displaySample(instruction, text, nil)
	if err != nil {
		return nil, err
	}
	return append(msgs, msg...), nil
}
