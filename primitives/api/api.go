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
	binaryClassifyModels []models.BinaryClassifyModel
	classifyModels       []models.ClassifyModel
	scoreModels          []models.ScoreModel
	generateModels       []models.GenerateModel
	parseForceModels     []models.ParseForceModel
	embeddingModels      []models.EmbeddingModel
	requestRouter        router.RequestRouter
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
	models := a.binaryClassifyModels
	var examples []*datapoint.BinaryClassifyDatapoint
	if requestOptions != nil {
		dp.Examples = requestOptions.Examples
		examples = requestOptions.Examples
		if len(requestOptions.Models) > 0 {
			models = requestOptions.Models
		}
	}
	model, err := a.requestRouter.RouteBinaryClassify(ctx, *dp, models)
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
	models := a.classifyModels
	var examples []*datapoint.ClassifyDatapoint
	if requestOptions != nil {
		dp.Examples = requestOptions.Examples
		examples = requestOptions.Examples
		if len(requestOptions.Models) > 0 {
			models = requestOptions.Models
		}
	}
	model, err := a.requestRouter.RouteClassify(ctx, *dp, models)
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
	models := a.parseForceModels
	var examples []*datapoint.ParseForceDatapoint
	if requestOptions != nil {
		examples = requestOptions.Examples
		dp.Examples = examples
		if len(requestOptions.Models) > 0 {
			models = requestOptions.Models
		}
	}
	model, err := a.requestRouter.RouteParseForce(ctx, *dp, models)
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
	models := a.scoreModels
	var examples []*datapoint.ScoreDatapoint
	if requestOptions != nil {
		dp.Examples = requestOptions.Examples
		examples = requestOptions.Examples
		if len(requestOptions.Models) > 0 {
			models = requestOptions.Models
		}
	}
	model, err := a.requestRouter.RouteScore(ctx, *dp, models)
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
	models := a.generateModels
	var examples []*datapoint.GenerateDatapoint
	if requestOptions != nil {
		dp.Examples = requestOptions.Examples
		examples = requestOptions.Examples
		if len(requestOptions.Models) > 0 {
			models = requestOptions.Models
		}
	}
	model, err := a.requestRouter.RouteGenerate(ctx, *dp, models)
	if err != nil {
		return "", err
	}
	return model.Generate(ctx, instruction, text, examples)
}

func (a *API) Embedding(ctx context.Context, text string) ([]float64, error) {
	embeddings, err := a.embeddingModels[0].GetEmbedding(ctx, text)
	if err != nil {
		return nil, err
	}
	return embeddings, nil
}

func DefaultAPI() *API {
	generalModel, err := model.DefaultGeneralModel()
	if err != nil {
		panic(fmt.Errorf("failed to create default general model: %v", err))
	}
	embeddingModel, err := model.DefaultEmbeddingModel()
	if err != nil {
		panic(fmt.Errorf("failed to create default embedding model: %v", err))
	}
	return &API{
		binaryClassifyModels: []models.BinaryClassifyModel{generalModel},
		classifyModels:       []models.ClassifyModel{generalModel},
		scoreModels:          []models.ScoreModel{generalModel},
		generateModels:       []models.GenerateModel{generalModel},
		parseForceModels:     []models.ParseForceModel{generalModel},
		embeddingModels:      []models.EmbeddingModel{embeddingModel},
		requestRouter:        router.DefaultRequestRouter(),
	}
}
