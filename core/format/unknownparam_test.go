package format

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	fschema "github.com/neokapi/neokapi/core/format/schema"
)

type schemaConfig struct{}

func (schemaConfig) Schema() *fschema.FormatSchema {
	return &fschema.FormatSchema{Properties: map[string]fschema.PropertySchema{
		"extract_all_pairs": {Type: "boolean"},
	}}
}

func TestUnknownParameterErrorNamesTheSnakeCaseKey(t *testing.T) {
	require.EqualError(t, UnknownParameterError(schemaConfig{}, "json", "extractAllPairs"),
		"json: extractAllPairs is now extract_all_pairs")
	require.EqualError(t, UnknownParameterError(schemaConfig{}, "json", "useKeyAsName"),
		"json: unknown parameter: useKeyAsName", "a camelCase key the schema does not hold is unknown")
	require.EqualError(t, UnknownParameterError(schemaConfig{}, "json", "bogus"),
		"json: unknown parameter: bogus")
	assert.NoError(t, RenamedParameterError(schemaConfig{}, "json", "extract_all_pairs"))
}

type viaJSONConfig struct {
	MaxCharsPerLine int `json:"max_chars_per_line"`
}

func (*viaJSONConfig) FormatName() string { return "vtt" }

func TestApplyMapViaJSONRefusesCamelCaseKeys(t *testing.T) {
	var c viaJSONConfig
	require.EqualError(t, ApplyMapViaJSON(&c, map[string]any{"maxCharsPerLine": 40}),
		"vtt: maxCharsPerLine is now max_chars_per_line")
	require.NoError(t, ApplyMapViaJSON(&c, map[string]any{"max_chars_per_line": 40}))
	assert.Equal(t, 40, c.MaxCharsPerLine)
}
