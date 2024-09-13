package chat

import (
	"reflect"
	"search_engine/primitives/datapoint"
)

type MistralModel struct {
	Temperature float64
}

func (m *MistralModel) BinaryClassify(instruction string, text string, examples []*datapoint.BinaryClassifyDatapoint) (bool, error) {
	// TODO: implement
	return false, nil
}

func (m *MistralModel) Classify(instruction string, text string, options []string, examples []*datapoint.ClassifyDatapoint) (int, error) {
	// TODO: implement
	return 0, nil
}

func (m *MistralModel) Parse(instruction string, text string, typ reflect.Type, examples []*datapoint.ParseForceDatapoint) error {
	// TODO: implement
	return nil
}

func (m *MistralModel) Score(instruction string, text string, min int, max int, examples []*datapoint.ScoreDatapoint) (int, error) {
	// TODO: implement
	return 0, nil
}

func (m *MistralModel) Generate(instruction string, text string, examples []*datapoint.GenerateDatapoint) (string, error) {
	// TODO: implement
	return "", nil
}