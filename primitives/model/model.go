package model

import (
	"search_engine/primitives/model/models"
	"search_engine/primitives/model/models/chat"
)

func DefaultGeneralModel() (models.GeneralModel, error) {
	return chat.NewOpenAIModel(chat.DEFAULT_OPENAI_MODEL, nil)
}
