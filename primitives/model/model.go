package model

import (
	"search_engine/primitives/model/models"
	"search_engine/primitives/model/models/chat"
	"search_engine/primitives/model/models/embedding"
)

func DefaultGeneralModel() (models.GeneralModel, error) {
	return chat.NewOpenAIModel(chat.DEFAULT_OPENAI_MODEL, nil)
}

func DefaultEmbeddingModel() (models.EmbeddingModel, error) {
	return embedding.NewOpenAIEmbeddingModel(nil)
}
