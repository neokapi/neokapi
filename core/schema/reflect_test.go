package schema

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type simpleConfig struct {
	Enabled    bool
	Name       string
	MaxRetries int
	Threshold  float64
}

func TestFromStruct_SimpleTypes(t *testing.T) {
	cfg := simpleConfig{
		Enabled:    true,
		Name:       "test",
		MaxRetries: 3,
		Threshold:  0.75,
	}
	meta := ToolMeta{
		ID:          "simple-tool",
		Category:    "transform",
		DisplayName: "Simple Tool",
	}
	s := FromStruct(&cfg, meta)

	require.NotNil(t, s)
	assert.Equal(t, "simple-tool", s.ID)
	assert.Equal(t, "Simple Tool", s.Title)
	assert.Equal(t, "object", s.Type)
	assert.Equal(t, "simple-tool", s.ToolMeta.ID)
	assert.Len(t, s.Properties, 4)

	// Boolean
	p := s.Properties["enabled"]
	assert.Equal(t, "boolean", p.Type)
	assert.Equal(t, true, p.Default)

	// String
	p = s.Properties["name"]
	assert.Equal(t, "string", p.Type)
	assert.Equal(t, "test", p.Default)

	// Integer
	p = s.Properties["maxRetries"]
	assert.Equal(t, "integer", p.Type)
	assert.Equal(t, 3, p.Default)

	// Float
	p = s.Properties["threshold"]
	assert.Equal(t, "number", p.Type)
	assert.Equal(t, 0.75, p.Default)
}

type taggedConfig struct {
	Mode      string `schema:"title=Transformation Mode,description=Processing mode,enum=upper|lower|title,default=lower"`
	Expansion int    `schema:"description=Text expansion percentage,min=0,max=200,default=0"`
	Secret    string `schema:"widget=password"`
}

func TestFromStruct_SchemaTags(t *testing.T) {
	s := FromStruct(&taggedConfig{}, ToolMeta{ID: "tagged"})

	mode := s.Properties["mode"]
	assert.Equal(t, "Transformation Mode", mode.Title)
	assert.Equal(t, "Processing mode", mode.Description)
	require.Len(t, mode.Options, 3)
	assert.Equal(t, "upper", mode.Options[0].Value)
	assert.Equal(t, "upper", mode.Options[0].Label)
	assert.Equal(t, "lower", mode.Default)

	expansion := s.Properties["expansion"]
	assert.Equal(t, "Text expansion percentage", expansion.Description)
	require.NotNil(t, expansion.Min)
	assert.Equal(t, float64(0), *expansion.Min)
	require.NotNil(t, expansion.Max)
	assert.Equal(t, float64(200), *expansion.Max)
	assert.Equal(t, 0, expansion.Default)

	secret := s.Properties["secret"]
	assert.Equal(t, "password", secret.Widget)
}

type nestedConfig struct {
	Rules []ruleEntry
	Extra map[string]string
}

type ruleEntry struct {
	Pattern string
	IsRegex bool
}

func TestFromStruct_NestedTypes(t *testing.T) {
	s := FromStruct(&nestedConfig{}, ToolMeta{ID: "nested"})

	// Slice of structs
	rules := s.Properties["rules"]
	assert.Equal(t, "array", rules.Type)
	require.NotNil(t, rules.Items)
	assert.Equal(t, "object", rules.Items.Type)
	assert.Contains(t, rules.Items.Properties, "pattern")
	assert.Contains(t, rules.Items.Properties, "isRegex")

	// Map
	extra := s.Properties["extra"]
	assert.Equal(t, "object", extra.Type)
}

type withInterface struct {
	Name     string
	Provider any //nolint:revive // intentionally empty interface to test schema reflection skipping
	Callback func() error
}

func TestFromStruct_SkipsInterfaceAndFunc(t *testing.T) {
	s := FromStruct(&withInterface{Name: "test"}, ToolMeta{ID: "iface"})
	assert.Len(t, s.Properties, 1)
	assert.Contains(t, s.Properties, "name")
}

type groupedConfig struct {
	Host    string `schema:"group=connection,description=Server hostname"`
	Port    int    `schema:"group=connection,description=Server port"`
	Verbose bool   `schema:"group=advanced,description=Enable verbose output"`
}

func TestFromStruct_Groups(t *testing.T) {
	s := FromStruct(&groupedConfig{}, ToolMeta{ID: "grouped"})
	require.Len(t, s.Groups, 2)
	assert.Equal(t, "connection", s.Groups[0].ID)
	assert.Equal(t, "Connection", s.Groups[0].Label)
	assert.Equal(t, []string{"host", "port"}, s.Groups[0].Fields)
	assert.Equal(t, "advanced", s.Groups[1].ID)
	assert.Equal(t, []string{"verbose"}, s.Groups[1].Fields)
}

func TestFromStruct_NonPointer(t *testing.T) {
	cfg := simpleConfig{Enabled: true}
	s := FromStruct(cfg, ToolMeta{ID: "val"})
	assert.Len(t, s.Properties, 4)
}

func TestFromStruct_EmptyStruct(t *testing.T) {
	type empty struct{}
	s := FromStruct(&empty{}, ToolMeta{ID: "empty"})
	assert.Empty(t, s.Properties)
}

func TestToCamelCase(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"FuzzyThreshold", "fuzzyThreshold"},
		{"TargetLocale", "targetLocale"},
		{"XMLParser", "xmlParser"},
		{"ID", "id"},
		{"ApplyTarget", "applyTarget"},
		{"CheckEmptyTarget", "checkEmptyTarget"},
		{"A", "a"},
		{"", ""},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.expected, toCamelCase(tt.input), "toCamelCase(%q)", tt.input)
	}
}

func TestFromStruct_RawJSON(t *testing.T) {
	s := FromStruct(&simpleConfig{Enabled: true}, ToolMeta{ID: "json-test"})
	require.NotNil(t, s.RawJSON)
	assert.Contains(t, string(s.RawJSON), `"enabled"`)
	assert.Contains(t, string(s.RawJSON), `"boolean"`)
}
