package bridge

import (
	"testing"

	"github.com/neokapi/neokapi/core/schema"
	"github.com/stretchr/testify/assert"
)

func TestEncodeFilterParams_NilMap(t *testing.T) {
	result := encodeFilterParams(nil)
	assert.Nil(t, result)
}

func TestEncodeFilterParams_EmptyMap(t *testing.T) {
	result := encodeFilterParams(map[string]any{})
	assert.Nil(t, result)
}

func TestEncodeFilterParams_StringValues(t *testing.T) {
	result := encodeFilterParams(map[string]any{
		"key": "value",
	})
	assert.Equal(t, "value", result["key"])
}

func TestEncodeFilterParams_BooleanValues(t *testing.T) {
	result := encodeFilterParams(map[string]any{
		"flag": true,
	})
	assert.Equal(t, "true", result["flag"])
}

func TestEncodeFilterParams_IntValues(t *testing.T) {
	result := encodeFilterParams(map[string]any{
		"count": 42,
	})
	assert.Equal(t, "42", result["count"])
}

func TestEncodeFilterParams_ComplexValues(t *testing.T) {
	result := encodeFilterParams(map[string]any{
		"codeFinderRules": map[string]any{
			"rules": []map[string]string{
				{"pattern": "<[^>]+>"},
			},
		},
	})
	assert.Contains(t, result["codeFinderRules"], "rules")
	assert.Contains(t, result["codeFinderRules"], "pattern")
}

func TestEncodeFilterParams_HierarchicalParams(t *testing.T) {
	// Params should be passed in hierarchical schema format.
	// Section keys like "parser" map to JSON objects with their properties.
	result := encodeFilterParams(map[string]any{
		"elements": map[string]any{
			"pre": map[string]any{"ruleTypes": []string{"EXCLUDE"}},
		},
		"parser": map[string]any{
			"assumeWellformed": true,
		},
	})
	assert.Contains(t, result["elements"], `"pre"`)
	assert.Contains(t, result["parser"], `"assumeWellformed"`)
}

func TestExtractParamTypes_NilSchema(t *testing.T) {
	result := extractParamTypes(nil, map[string]any{"key": "val"})
	assert.Nil(t, result)
}

func TestExtractParamTypes_EmptyParams(t *testing.T) {
	s := &schema.ComponentSchema{
		Properties: map[string]schema.PropertySchema{
			"regEx": {Type: "boolean"},
		},
	}
	result := extractParamTypes(s, nil)
	assert.Nil(t, result)
}

func TestExtractParamTypes_MatchesSchemaTypes(t *testing.T) {
	s := &schema.ComponentSchema{
		Properties: map[string]schema.PropertySchema{
			"regEx":   {Type: "boolean"},
			"count":   {Type: "integer"},
			"search0": {Type: "string"},
			"logPath": {Type: "string"},
		},
	}
	params := map[string]any{
		"regEx":   true,
		"count":   1,
		"search0": "hello",
		"unknown": "extra",
	}
	result := extractParamTypes(s, params)
	assert.Equal(t, "boolean", result["regEx"])
	assert.Equal(t, "integer", result["count"])
	assert.Equal(t, "string", result["search0"])
	// "unknown" is not in schema, so not in result.
	_, hasUnknown := result["unknown"]
	assert.False(t, hasUnknown)
	// "logPath" is in schema but not in params, so not in result.
	_, hasLogPath := result["logPath"]
	assert.False(t, hasLogPath)
}
