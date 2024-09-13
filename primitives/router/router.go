package router

import (
	"context"
	"fmt"
	"search_engine/primitives/datapoint"
	"search_engine/primitives/model/models"
)

type RequestRouter interface {
	RouteBinaryClassify(ctx context.Context, dp datapoint.BinaryClassifyDatapoint, models []models.BinaryClassifyModel) (models.BinaryClassifyModel, error)
	RouteClassify(ctx context.Context, dp datapoint.ClassifyDatapoint, models []models.ClassifyModel) (models.ClassifyModel, error)
	RouteParseForce(ctx context.Context, dp datapoint.ParseForceDatapoint, models []models.ParseForceModel) (models.ParseForceModel, error)
	RouteScore(ctx context.Context, dp datapoint.ScoreDatapoint, models []models.ScoreModel) (models.ScoreModel, error)
	RouteGenerate(ctx context.Context, dp datapoint.GenerateDatapoint, models []models.GenerateModel) (models.GenerateModel, error)
}

type FirstModelRequestRouter struct{}

func (r *FirstModelRequestRouter) RouteBinaryClassify(ctx context.Context, dp datapoint.BinaryClassifyDatapoint, models []models.BinaryClassifyModel) (models.BinaryClassifyModel, error) {
	if len(models) == 0 {
		return nil, fmt.Errorf("no models provided")
	}
	return models[0], nil
}

func (r *FirstModelRequestRouter) RouteClassify(ctx context.Context, dp datapoint.ClassifyDatapoint, models []models.ClassifyModel) (models.ClassifyModel, error) {
	if len(models) == 0 {
		return nil, fmt.Errorf("no models provided")
	}
	return models[0], nil
}

func (r *FirstModelRequestRouter) RouteParseForce(ctx context.Context, dp datapoint.ParseForceDatapoint, models []models.ParseForceModel) (models.ParseForceModel, error) {
	if len(models) == 0 {
		return nil, fmt.Errorf("no models provided")
	}
	return models[0], nil
}

func (r *FirstModelRequestRouter) RouteScore(ctx context.Context, dp datapoint.ScoreDatapoint, models []models.ScoreModel) (models.ScoreModel, error) {
	if len(models) == 0 {
		return nil, fmt.Errorf("no models provided")
	}
	return models[0], nil
}

func (r *FirstModelRequestRouter) RouteGenerate(ctx context.Context, dp datapoint.GenerateDatapoint, models []models.GenerateModel) (models.GenerateModel, error) {
	if len(models) == 0 {
		return nil, fmt.Errorf("no models provided")
	}
	return models[0], nil
}

func DefaultRequestRouter() RequestRouter {
	return &FirstModelRequestRouter{}
}
