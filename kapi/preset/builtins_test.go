package preset

import (
	"testing"

	"github.com/neokapi/neokapi/core/preset"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// kapi/preset.RegisterBuiltins is a deprecated shim over core/preset. These
// tests verify it delegates faithfully; the authoritative preset contents are
// owned and tested by core/preset (see core/preset/builtins_test.go), so we do
// not re-assert the (evolving) list here.

func TestRegisterBuiltinsDelegatesToCore(t *testing.T) {
	viaShim := preset.NewPresetRegistry()
	RegisterBuiltins(viaShim)

	viaCore := preset.NewPresetRegistry()
	preset.RegisterBuiltins(viaCore)

	assert.Equal(t, frameworkNames(viaCore), frameworkNames(viaShim),
		"shim must register the same framework presets as core/preset")
	assert.NotEmpty(t, frameworkNames(viaShim))
}

func TestRegisterBuiltinsRegistersCommonStacks(t *testing.T) {
	reg := preset.NewPresetRegistry()
	RegisterBuiltins(reg)
	for _, name := range []string{"nextjs", "react-intl", "angular"} {
		require.NotNilf(t, reg.GetFrameworkPreset(name), "preset %q should be registered", name)
	}
}

func frameworkNames(reg *preset.PresetRegistry) []string {
	var names []string
	for _, p := range reg.ListFrameworkPresets() {
		names = append(names, p.Name)
	}
	return names
}
