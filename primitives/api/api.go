package api

import (
	"context"
	"fmt"
	"search_engine/primitives/datapoint"
	"search_engine/primitives/model"
	"search_engine/primitives/model/models"
	"search_engine/primitives/router"
)

type API struct {
	BinaryClassifyModels []models.BinaryClassifyModel
	ClassifyModels       []models.ClassifyModel
	ScoreModels          []models.ScoreModel
	GenerateModels       []models.GenerateModel
	ParseForceModels     []models.ParseForceModel
	RequestRouter        router.RequestRouter
}

type RequestOptions struct {
	RequestRouter router.RequestRouter
}

type BinaryClassifyRequestOptions struct {
	RequestOptions
	Examples []*datapoint.BinaryClassifyDatapoint
	Models   []models.BinaryClassifyModel
}

type ClassifyRequestOptions struct {
	RequestOptions
	Examples []*datapoint.ClassifyDatapoint
	Models   []models.ClassifyModel
}

type ParseForceRequestOptions struct {
	RequestOptions
	Examples []*datapoint.ParseForceDatapoint
	Models   []models.ParseForceModel
}

type ScoreRequestOptions struct {
	RequestOptions
	Examples []*datapoint.ScoreDatapoint
	Models   []models.ScoreModel
}

type GenerateRequestOptions struct {
	RequestOptions
	Examples []*datapoint.GenerateDatapoint
	Models   []models.GenerateModel
}

func (a *API) BinaryClassify(ctx context.Context, instruction string, text string, requestOptions *BinaryClassifyRequestOptions) (bool, error) {
	dp := &datapoint.BinaryClassifyDatapoint{
		Instruction: instruction,
		Text:        text,
	}
	models := a.BinaryClassifyModels
	var examples []*datapoint.BinaryClassifyDatapoint
	if requestOptions != nil {
		dp.Examples = requestOptions.Examples
		examples = requestOptions.Examples
		if len(requestOptions.Models) > 0 {
			models = requestOptions.Models
		}
	}
	model, err := a.RequestRouter.RouteBinaryClassify(ctx, *dp, models)
	if err != nil {
		return false, err
	}
	return model.BinaryClassify(ctx, instruction, text, examples)
}

func (a *API) Classify(ctx context.Context, instruction string, text string, options []string, requestOptions *ClassifyRequestOptions) (int, error) {
	dp := &datapoint.ClassifyDatapoint{
		Instruction: instruction,
		Text:        text,
		Options:     options,
	}
	models := a.ClassifyModels
	var examples []*datapoint.ClassifyDatapoint
	if requestOptions != nil {
		dp.Examples = requestOptions.Examples
		examples = requestOptions.Examples
		if len(requestOptions.Models) > 0 {
			models = requestOptions.Models
		}
	}
	model, err := a.RequestRouter.RouteClassify(ctx, *dp, models)
	if err != nil {
		return 0, err
	}
	return model.Classify(ctx, instruction, text, options, examples)
}

func (a *API) ParseForce(ctx context.Context, instruction string, text string, v any, requestOptions *ParseForceRequestOptions) error {
	dp := &datapoint.ParseForceDatapoint{
		Instruction: instruction,
		Text:        text,
		V:           v,
	}
	models := a.ParseForceModels
	var examples []*datapoint.ParseForceDatapoint
	if requestOptions != nil {
		examples = requestOptions.Examples
		dp.Examples = examples
		if len(requestOptions.Models) > 0 {
			models = requestOptions.Models
		}
	}
	model, err := a.RequestRouter.RouteParseForce(ctx, *dp, models)
	if err != nil {
		return err
	}
	return model.ParseForce(ctx, instruction, text, v, examples)
}

func (a *API) Score(ctx context.Context, instruction string, text string, min int, max int, requestOptions *ScoreRequestOptions) (int, error) {
	dp := &datapoint.ScoreDatapoint{
		Instruction: instruction,
		Text:        text,
		Min:         min,
		Max:         max,
	}
	models := a.ScoreModels
	var examples []*datapoint.ScoreDatapoint
	if requestOptions != nil {
		dp.Examples = requestOptions.Examples
		examples = requestOptions.Examples
		if len(requestOptions.Models) > 0 {
			models = requestOptions.Models
		}
	}
	model, err := a.RequestRouter.RouteScore(ctx, *dp, models)
	if err != nil {
		return 0, err
	}
	return model.Score(ctx, instruction, text, min, max, examples)
}

func (a *API) Generate(ctx context.Context, instruction string, text string, requestOptions *GenerateRequestOptions) (string, error) {
	dp := &datapoint.GenerateDatapoint{
		Instruction: instruction,
		Text:        text,
	}
	models := a.GenerateModels
	var examples []*datapoint.GenerateDatapoint
	if requestOptions != nil {
		dp.Examples = requestOptions.Examples
		examples = requestOptions.Examples
		if len(requestOptions.Models) > 0 {
			models = requestOptions.Models
		}
	}
	model, err := a.RequestRouter.RouteGenerate(ctx, *dp, models)
	if err != nil {
		return "", err
	}
	return model.Generate(ctx, instruction, text, examples)
}

func DefaultAPI() *API {
	generalModel, err := model.DefaultGeneralModel()
	if err != nil {
		panic(fmt.Errorf("failed to create default general model: %v", err))
	}
	return &API{
		BinaryClassifyModels: []models.BinaryClassifyModel{generalModel},
		ClassifyModels:       []models.ClassifyModel{generalModel},
		ScoreModels:          []models.ScoreModel{generalModel},
		GenerateModels:       []models.GenerateModel{generalModel},
		ParseForceModels:     []models.ParseForceModel{generalModel},
		RequestRouter:        router.DefaultRequestRouter(),
	}
}
