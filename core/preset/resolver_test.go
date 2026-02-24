package preset

import (
	"fmt"
	"strings"
	"testing"

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
			errors = append(errors, fmt.Sprintf("%s: unknown parameter", name))
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
			"assumeWellformed":    true,
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
			"assumeWellformed":    true,
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
	assert.Error(t, err)
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
	assert.Error(t, err)
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
	assert.NoError(t, err)

	// Invalid preset (unknown param)
	err = resolver.ValidatePreset(&FormatPreset{
		Format: "okf_html",
		Config: map[string]any{"badParam": true},
	})
	assert.Error(t, err)
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
	assert.Len(t, errs, 0)
}
