package preset

import (
	"testing"

	"github.com/neokapi/neokapi/core/preset"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegisterBuiltins(t *testing.T) {
	reg := preset.NewPresetRegistry()
	RegisterBuiltins(reg)

	// nextjs
	fp := reg.GetFrameworkPreset("nextjs")
	require.NotNil(t, fp)
	assert.Equal(t, "Next.js App Router with next-intl", fp.Description)
	assert.Len(t, fp.Mappings, 1)
	assert.Equal(t, "messages/*.json", fp.Mappings[0].Local)
	assert.Contains(t, fp.Exclude, "node_modules/**")
	assert.Contains(t, fp.Exclude, ".next/**")
	assert.Equal(t, registry.SourceBuiltIn, fp.Source)

	// react-intl
	fp = reg.GetFrameworkPreset("react-intl")
	require.NotNil(t, fp)
	assert.Equal(t, "React with react-intl (FormatJS)", fp.Description)

	// angular
	fp = reg.GetFrameworkPreset("angular")
	require.NotNil(t, fp)
	assert.Equal(t, "Angular with @angular/localize", fp.Description)
	assert.Len(t, fp.Mappings, 1)
	assert.Equal(t, "xliff", fp.Mappings[0].Format)
}

func TestBuiltinPresetsList(t *testing.T) {
	reg := preset.NewPresetRegistry()
	RegisterBuiltins(reg)

	presets := reg.ListFrameworkPresets()
	assert.Len(t, presets, 3)
	// Should be sorted by name
	assert.Equal(t, "angular", presets[0].Name)
	assert.Equal(t, "nextjs", presets[1].Name)
	assert.Equal(t, "react-intl", presets[2].Name)
}
