package preset

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockSchemaValidator implements SchemaValidator for testing.
type mockSchemaValidator struct {
	schemas map[string]map[string]string // format -> param -> type
}

func (m *mockSchemaValidator) ValidateParams(filterID string, params map[string]any) error {
	schema, ok := m.schemas[filterID]
	if !ok {
		return nil // no schema = no validation
	}
	var errors []string
	for name := range params {
		if _, ok := schema[name]; !ok {
			errors = append(errors, name+": unknown parameter")
		}
	}
	if len(errors) > 0 {
		return fmt.Errorf("invalid filter parameters for %s:\n  %s", filterID, strings.Join(errors, "\n  "))
	}
	return nil
}

func TestResolveFormatConfig_NoPreset(t *testing.T) {
	reg := NewPresetRegistry()
	resolver := NewConfigResolver(reg, nil)

	result, err := resolver.ResolveFormatConfig("json", "", nil, map[string]any{
		"extractArrayStrings": false,
	})
	require.NoError(t, err)
	assert.Equal(t, false, result["extractArrayStrings"])
}

func TestResolveFormatConfig_RegistryPreset(t *testing.T) {
	reg := NewPresetRegistry()
	reg.RegisterFormatPreset("okf_html", "wellFormed", &FormatPreset{
		Name:   "wellFormed",
		Format: "okf_html",
		Config: map[string]any{
			"assumeWellformed":   true,
			"preserveWhitespace": true,
		},
	})

	resolver := NewConfigResolver(reg, nil)
	result, err := resolver.ResolveFormatConfig("okf_html", "wellFormed", nil, nil)
	require.NoError(t, err)
	assert.Equal(t, true, result["assumeWellformed"])
	assert.Equal(t, true, result["preserveWhitespace"])
}

func TestResolveFormatConfig_LocalPresetOverridesRegistry(t *testing.T) {
	reg := NewPresetRegistry()
	reg.RegisterFormatPreset("json", "strict", &FormatPreset{
		Name:   "strict",
		Format: "json",
		Config: map[string]any{"extractArrayStrings": true},
	})

	local := map[string]LocalFormatPreset{
		"strict": {Config: map[string]any{"extractArrayStrings": false}},
	}

	resolver := NewConfigResolver(reg, nil)
	result, err := resolver.ResolveFormatConfig("json", "strict", local, nil)
	require.NoError(t, err)
	// Local preset takes precedence over registry
	assert.Equal(t, false, result["extractArrayStrings"])
}

func TestResolveFormatConfig_OverridesApplied(t *testing.T) {
	reg := NewPresetRegistry()
	reg.RegisterFormatPreset("okf_html", "wellFormed", &FormatPreset{
		Name:   "wellFormed",
		Format: "okf_html",
		Config: map[string]any{
			"assumeWellformed":   true,
			"preserveWhitespace": true,
		},
	})

	resolver := NewConfigResolver(reg, nil)
	result, err := resolver.ResolveFormatConfig("okf_html", "wellFormed", nil, map[string]any{
		"preserveWhitespace": false,
	})
	require.NoError(t, err)
	assert.Equal(t, true, result["assumeWellformed"])
	assert.Equal(t, false, result["preserveWhitespace"]) // overridden
}

func TestResolveFormatConfig_PresetNotFound(t *testing.T) {
	reg := NewPresetRegistry()
	resolver := NewConfigResolver(reg, nil)

	_, err := resolver.ResolveFormatConfig("json", "nonexistent", nil, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestResolveFormatConfig_SchemaValidation(t *testing.T) {
	reg := NewPresetRegistry()
	validator := &mockSchemaValidator{
		schemas: map[string]map[string]string{
			"okf_html": {"assumeWellformed": "boolean", "preserveWhitespace": "boolean"},
		},
	}
	resolver := NewConfigResolver(reg, validator)

	// Valid params
	result, err := resolver.ResolveFormatConfig("okf_html", "", nil, map[string]any{
		"assumeWellformed": true,
	})
	require.NoError(t, err)
	assert.Equal(t, true, result["assumeWellformed"])

	// Invalid param (typo)
	_, err = resolver.ResolveFormatConfig("okf_html", "", nil, map[string]any{
		"preservWhitespace": true,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown parameter")
}

func TestValidatePreset(t *testing.T) {
	reg := NewPresetRegistry()
	validator := &mockSchemaValidator{
		schemas: map[string]map[string]string{
			"okf_html": {"assumeWellformed": "boolean"},
		},
	}
	resolver := NewConfigResolver(reg, validator)

	// Valid preset
	err := resolver.ValidatePreset(&FormatPreset{
		Format: "okf_html",
		Config: map[string]any{"assumeWellformed": true},
	})
	require.NoError(t, err)

	// Invalid preset (unknown param)
	err = resolver.ValidatePreset(&FormatPreset{
		Format: "okf_html",
		Config: map[string]any{"badParam": true},
	})
	require.Error(t, err)
}

func TestValidateAllPresets(t *testing.T) {
	reg := NewPresetRegistry()
	validator := &mockSchemaValidator{
		schemas: map[string]map[string]string{
			"okf_html": {"assumeWellformed": "boolean"},
		},
	}
	resolver := NewConfigResolver(reg, validator)

	locals := map[string]LocalFormatPreset{
		"my-html": {Base: "okf_html", Config: map[string]any{"badParam": true}},
		"my-json": {Base: "json", Config: map[string]any{"anything": true}},
	}

	// All formats
	errs := resolver.ValidateAllPresets(locals, "")
	assert.Len(t, errs, 1) // only okf_html has a schema

	// Filtered to okf_html
	errs = resolver.ValidateAllPresets(locals, "okf_html")
	assert.Len(t, errs, 1)

	// Filtered to json (no schema)
	errs = resolver.ValidateAllPresets(locals, "json")
	assert.Empty(t, errs)
}

func TestIsConfigFilePath(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"wellFormed", false},
		{"strict-mode", false},
		{"./my-config.yaml", true},
		{"../configs/openxml.yml", true},
		{"/absolute/path.json", true},
		{"config.yaml", true},
		{"config.yml", true},
		{"config.json", true},
		{"config.JSON", true},
		{"path/to/file", true},
		{"my-preset", false},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.want, IsConfigFilePath(tt.input))
		})
	}
}

func TestLoadConfigFile_YAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	err := os.WriteFile(path, []byte("extractStyles: true\ntranslateComments: false\n"), 0o644)
	require.NoError(t, err)

	cfg, err := LoadConfigFile(path)
	require.NoError(t, err)
	assert.Equal(t, true, cfg["extractStyles"])
	assert.Equal(t, false, cfg["translateComments"])
}

func TestLoadConfigFile_JSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	err := os.WriteFile(path, []byte(`{"extractStyles": true, "maxDepth": 5}`), 0o644)
	require.NoError(t, err)

	cfg, err := LoadConfigFile(path)
	require.NoError(t, err)
	assert.Equal(t, true, cfg["extractStyles"])
	assert.Equal(t, float64(5), cfg["maxDepth"]) // JSON numbers are float64
}

func TestLoadConfigFile_NotFound(t *testing.T) {
	_, err := LoadConfigFile("/nonexistent/config.yaml")
	require.Error(t, err)
}

func TestLoadConfigFile_EnvelopedYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	err := os.WriteFile(path, []byte(`apiVersion: v1
kind: HtmlFormatConfig
metadata:
  name: my-html
spec:
  parser:
    preserveWhitespace: true
  useCodeFinder: false
`), 0o644)
	require.NoError(t, err)

	cfg, err := LoadConfigFile(path)
	require.NoError(t, err)
	parser, ok := cfg["parser"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, true, parser["preserveWhitespace"])
	assert.Equal(t, false, cfg["useCodeFinder"])
}

func TestLoadConfigFile_EnvelopedJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	err := os.WriteFile(path, []byte(`{
		"apiVersion": "v1",
		"kind": "JsonFormatConfig",
		"metadata": {"name": "test"},
		"spec": {"extractAllPairs": false, "useFullKeyPath": true}
	}`), 0o644)
	require.NoError(t, err)

	cfg, err := LoadConfigFile(path)
	require.NoError(t, err)
	assert.Equal(t, false, cfg["extractAllPairs"])
	assert.Equal(t, true, cfg["useFullKeyPath"])
}

func TestLoadConfigFile_BareYAMLStillWorks(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	err := os.WriteFile(path, []byte("extractStyles: true\ntranslateComments: false\n"), 0o644)
	require.NoError(t, err)

	cfg, err := LoadConfigFile(path)
	require.NoError(t, err)
	assert.Equal(t, true, cfg["extractStyles"])
	assert.Equal(t, false, cfg["translateComments"])
}

func TestLoadConfigFile_InvalidEnvelope(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	err := os.WriteFile(path, []byte(`apiVersion: bad-version
kind: HtmlFormatConfig
metadata:
  name: test
spec: {}
`), 0o644)
	require.NoError(t, err)

	_, err = LoadConfigFile(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "envelope")
}

func TestTransformConfigSpec(t *testing.T) {
	// No transform registered for this pair - spec returned unchanged
	spec := map[string]any{"foo": "bar"}
	result, err := TransformConfigSpec(config.Kind("CustomFormatConfig"), config.Kind("TestFormatConfig"), spec)
	require.NoError(t, err)
	assert.Equal(t, spec, result)
}

func TestResolveFormatConfig_ConfigFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "openxml.yaml")
	err := os.WriteFile(path, []byte("extractStyles: true\ntranslateComments: false\n"), 0o644)
	require.NoError(t, err)

	reg := NewPresetRegistry()
	resolver := NewConfigResolver(reg, nil)

	result, err := resolver.ResolveFormatConfig("okf_openxml", path, nil, nil)
	require.NoError(t, err)
	assert.Equal(t, true, result["extractStyles"])
	assert.Equal(t, false, result["translateComments"])
}

func TestResolveFormatConfig_ConfigFileSchemaValidation(t *testing.T) {
	dir := t.TempDir()

	validator := &mockSchemaValidator{
		schemas: map[string]map[string]string{
			"okf_html": {"assumeWellformed": "boolean", "preserveWhitespace": "boolean"},
		},
	}

	// Valid config file
	validPath := filepath.Join(dir, "valid.yaml")
	err := os.WriteFile(validPath, []byte("assumeWellformed: true\n"), 0o644)
	require.NoError(t, err)

	reg := NewPresetRegistry()
	resolver := NewConfigResolver(reg, validator)

	result, err := resolver.ResolveFormatConfig("okf_html", validPath, nil, nil)
	require.NoError(t, err)
	assert.Equal(t, true, result["assumeWellformed"])

	// Invalid config file (unknown parameter)
	invalidPath := filepath.Join(dir, "invalid.yaml")
	err = os.WriteFile(invalidPath, []byte("badParam: true\n"), 0o644)
	require.NoError(t, err)

	_, err = resolver.ResolveFormatConfig("okf_html", invalidPath, nil, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown parameter")
}

func TestResolveFormatConfig_ConfigFileWithOverrides(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "openxml.yaml")
	err := os.WriteFile(path, []byte("extractStyles: true\ntranslateComments: false\n"), 0o644)
	require.NoError(t, err)

	reg := NewPresetRegistry()
	resolver := NewConfigResolver(reg, nil)

	result, err := resolver.ResolveFormatConfig("okf_openxml", path, nil, map[string]any{
		"translateComments": true,
	})
	require.NoError(t, err)
	assert.Equal(t, true, result["extractStyles"])
	assert.Equal(t, true, result["translateComments"]) // overridden
}
