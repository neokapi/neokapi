package ner_test

import (
	"testing"

	"github.com/gokapi/gokapi/core/ai/ner"
	"github.com/gokapi/gokapi/core/model"
	"github.com/stretchr/testify/assert"
)

func TestDetectedEntityFields(t *testing.T) {
	e := ner.DetectedEntity{
		Text:       "John Smith",
		Type:       model.EntityPerson,
		Confidence: 0.95,
		Offset:     0,
		Length:     10,
	}
	assert.Equal(t, "John Smith", e.Text)
	assert.Equal(t, model.EntityPerson, e.Type)
	assert.Equal(t, 0.95, e.Confidence)
	assert.Equal(t, 0, e.Offset)
	assert.Equal(t, 10, e.Length)
}

func TestResponseEntitiesSlice(t *testing.T) {
	resp := ner.Response{
		Entities: []ner.DetectedEntity{
			{Text: "March 15", Type: model.EntityDate, Confidence: 0.9, Offset: 20, Length: 8},
			{Text: "$49.99", Type: model.EntityCurrency, Confidence: 0.98, Offset: 30, Length: 6},
		},
	}
	assert.Len(t, resp.Entities, 2)
	assert.Equal(t, model.EntityDate, resp.Entities[0].Type)
	assert.Equal(t, model.EntityCurrency, resp.Entities[1].Type)
}

func TestConfigFields(t *testing.T) {
	cfg := ner.Config{
		Endpoint: "https://example.cognitiveservices.azure.com",
		APIKey:   "test-key",
		Options:  map[string]string{"model_version": "latest"},
	}
	assert.Equal(t, "https://example.cognitiveservices.azure.com", cfg.Endpoint)
	assert.Equal(t, "test-key", cfg.APIKey)
	assert.Equal(t, "latest", cfg.Options["model_version"])
}
