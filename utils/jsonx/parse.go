package jsonx

import (
	"encoding/json"

	"github.com/invopop/jsonschema"
)

func ValueToJsonSchemaStr(v any) (string, error) {
	jsonSchema := jsonschema.Reflect(v)
	jsonSchemaData, err := json.MarshalIndent(jsonSchema, "", "  ")
	if err != nil {
		return "", err
	}
	return string(jsonSchemaData), nil
}
