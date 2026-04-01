package schema

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestComponentSchema_Validate(t *testing.T) {
	s := &ComponentSchema{
		Properties: map[string]PropertySchema{
			"enabled":   {Type: "boolean"},
			"name":      {Type: "string"},
			"count":     {Type: "integer"},
			"threshold": {Type: "number"},
			"mode":      {Type: "string", Enum: []any{"fast", "slow"}},
		},
	}

	// Valid params
	errs := s.Validate(map[string]any{
		"enabled":   true,
		"name":      "test",
		"count":     42,
		"threshold": 0.5,
		"mode":      "fast",
	})
	assert.Empty(t, errs)

	// Type mismatches
	errs = s.Validate(map[string]any{
		"enabled": "not-a-bool",
		"name":    123,
		"count":   "not-a-number",
	})
	assert.Len(t, errs, 3)

	// Unknown parameter
	errs = s.Validate(map[string]any{
		"unknown": "value",
	})
	require.Len(t, errs, 1)
	assert.Equal(t, "unknown", errs[0].Field)
	assert.Contains(t, errs[0].Message, "unknown parameter")

	// Enum violation
	errs = s.Validate(map[string]any{
		"mode": "invalid",
	})
	require.Len(t, errs, 1)
	assert.Contains(t, errs[0].Message, "enum")
}

func TestComponentSchema_Validate_NilSchema(t *testing.T) {
	var s *ComponentSchema
	errs := s.Validate(map[string]any{"foo": "bar"})
	assert.Empty(t, errs)
}

func TestComponentSchema_JSON_Roundtrip(t *testing.T) {
	s := &ComponentSchema{
		ID:          "pseudo-translate",
		Title:       "Pseudo Translate",
		Description: "Generate pseudo-translations",
		Type:        "object",
		ToolMeta: &ToolMeta{
			ID:          "pseudo-translate",
			Category:    "transform",
			DisplayName: "Pseudo Translate",
			Inputs:      []string{PartTypeBlock},
			Tags:        []string{"translation"},
			Requires:    []string{RequiresTargetLanguage},
		},
		Groups: []ParameterGroup{
			{ID: "output", Label: "Output", Fields: []string{"prefix", "suffix"}},
		},
		Properties: map[string]PropertySchema{
			"expansionPercent": {Type: "integer", Description: "Extra padding", Default: 0},
			"prefix":           {Type: "string", Default: "["},
			"suffix":           {Type: "string", Default: "]"},
		},
	}

	data, err := json.Marshal(s)
	require.NoError(t, err)

	var decoded ComponentSchema
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Equal(t, s.ID, decoded.ID)
	assert.Equal(t, s.Title, decoded.Title)
	assert.Equal(t, s.ToolMeta.Category, decoded.ToolMeta.Category)
	assert.Equal(t, []string{"block"}, decoded.ToolMeta.Inputs)
	assert.Empty(t, decoded.ToolMeta.Outputs)
	assert.Equal(t, []string{"translation"}, decoded.ToolMeta.Tags)
	assert.Equal(t, []string{"target-language"}, decoded.ToolMeta.Requires)
	assert.Len(t, decoded.Properties, 3)
	assert.Equal(t, "integer", decoded.Properties["expansionPercent"].Type)
}

func TestValidationError_Error(t *testing.T) {
	e := ValidationError{Field: "name", Message: "expected string"}
	assert.Equal(t, "name: expected string", e.Error())
}
