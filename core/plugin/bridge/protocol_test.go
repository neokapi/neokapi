package bridge

import (
	"testing"

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
