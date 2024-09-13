package models

import (
	"context"
	"search_engine/primitives/datapoint"
)

type BinaryClassifyModel interface {
	BinaryClassify(ctx context.Context, instruction string, text string, examples []*datapoint.BinaryClassifyDatapoint) (bool, error)
}

type ClassifyModel interface {
	Classify(ctx context.Context, instruction string, text string, options []string, examples []*datapoint.ClassifyDatapoint) (int, error)
}

type ParseForceModel interface {
	ParseForce(ctx context.Context, instruction string, text string, v any, examples []*datapoint.ParseForceDatapoint) error
}

type ScoreModel interface {
	Score(ctx context.Context, instruction string, text string, min int, max int, examples []*datapoint.ScoreDatapoint) (int, error)
}

type GenerateModel interface {
	Generate(ctx context.Context, instruction string, text string, examples []*datapoint.GenerateDatapoint) (string, error)
}

type GeneralModel interface {
	BinaryClassifyModel
	ClassifyModel
	ParseForceModel
	ScoreModel
	GenerateModel
}
