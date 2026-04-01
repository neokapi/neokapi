package cli

import (
	"testing"

	"github.com/neokapi/neokapi/core/schema"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testSchema() *schema.ComponentSchema {
	return &schema.ComponentSchema{
		Properties: map[string]schema.PropertySchema{
			"enabled":          {Type: "boolean", Default: true, Description: "Enable feature"},
			"mode":             {Type: "string", Default: "fast", Enum: []any{"fast", "slow"}, Description: "Processing mode"},
			"maxRetries":       {Type: "integer", Default: 3, Description: "Max retry count"},
			"threshold":        {Type: "number", Default: 0.75, Description: "Score threshold"},
			"expansionPercent": {Type: "integer", Default: 0, Description: "Expansion percent"},
		},
	}
}

func TestRegisterSchemaFlags(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	s := testSchema()
	RegisterSchemaFlags(cmd, s)

	// Check boolean flag
	f := cmd.Flags().Lookup("enabled")
	require.NotNil(t, f, "expected 'enabled' flag")
	assert.Equal(t, "true", f.DefValue)

	// Check string flag with enum in description
	f = cmd.Flags().Lookup("mode")
	require.NotNil(t, f)
	assert.Equal(t, "fast", f.DefValue)
	assert.Contains(t, f.Usage, "fast, slow")

	// Check integer flag (camelCase -> kebab-case)
	f = cmd.Flags().Lookup("max-retries")
	require.NotNil(t, f)
	assert.Equal(t, "3", f.DefValue)

	// Check number flag
	f = cmd.Flags().Lookup("threshold")
	require.NotNil(t, f)
	assert.Equal(t, "0.75", f.DefValue)

	// Check kebab-case conversion
	f = cmd.Flags().Lookup("expansion-percent")
	require.NotNil(t, f)
	assert.Equal(t, "0", f.DefValue)
}

func TestReadSchemaFlags_OnlyChanged(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	s := testSchema()
	RegisterSchemaFlags(cmd, s)

	// Simulate setting only --mode flag
	err := cmd.Flags().Set("mode", "slow")
	require.NoError(t, err)

	result := ReadSchemaFlags(cmd, s)
	assert.Len(t, result, 1)
	assert.Equal(t, "slow", result["mode"])
	// Other flags should not appear since they weren't changed
	assert.NotContains(t, result, "enabled")
}

func TestReadAllSchemaFlags(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	s := testSchema()
	RegisterSchemaFlags(cmd, s)

	result := ReadAllSchemaFlags(cmd, s)
	assert.Equal(t, true, result["enabled"])
	assert.Equal(t, "fast", result["mode"])
	assert.Equal(t, 3, result["maxRetries"])
	assert.Equal(t, 0.75, result["threshold"])
	assert.Equal(t, 0, result["expansionPercent"])
}

func TestReadSchemaFlags_NilSchema(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	result := ReadSchemaFlags(cmd, nil)
	assert.Nil(t, result)
}

func TestToKebabCase(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"fuzzyThreshold", "fuzzy-threshold"},
		{"applyTarget", "apply-target"},
		{"enabled", "enabled"},
		{"XMLParser", "x-m-l-parser"},
		{"", ""},
		{"a", "a"},
		{"checkEmptyTarget", "check-empty-target"},
		{"expansionPercent", "expansion-percent"},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.expected, toKebabCase(tt.input), "toKebabCase(%q)", tt.input)
	}
}
