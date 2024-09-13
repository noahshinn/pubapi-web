package chat

import (
	"reflect"

	"search_engine/primitives/datapoint"
)

type VLLMChatModel struct {
	Temperature float64
}

func (m *VLLMChatModel) BinaryClassify(instruction string, text string, examples []*datapoint.BinaryClassifyDatapoint) (bool, error) {
	// TODO: implement
	return false, nil
}

func (m *VLLMChatModel) Classify(instruction string, text string, options []string, examples []*datapoint.ClassifyDatapoint) (int, error) {
	// TODO: implement
	return 0, nil
}

func (m *VLLMChatModel) ParseForce(instruction string, text string, typ reflect.Type, examples []*datapoint.ParseForceDatapoint) error {
	// TODO: implement
	return nil
}

func (m *VLLMChatModel) Score(instruction string, text string, min int, max int, examples []*datapoint.ScoreDatapoint) (int, error) {
	// TODO: implement
	return 0, nil
}

func (m *VLLMChatModel) Generate(instruction string, text string, examples []*datapoint.GenerateDatapoint) (string, error) {
	// TODO: implement
	return "", nil
}
