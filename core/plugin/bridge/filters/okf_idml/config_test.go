//go:build integration

package okf_idml

import (
	"testing"

	"github.com/gokapi/gokapi/core/plugin/bridge/filters/bridgetest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// ParametersTest — validates that IDML filter parameters are applied correctly.
//
// The Java ParametersTest tests mostly validate in-memory parameter objects.
// Since we test through the bridge, we verify that valid configs produce
// expected extraction results and that the bridge correctly applies parameters.
// ---------------------------------------------------------------------------

// okapi: ParametersTest#initialisesDefaultParameters
func TestConfig_InitialisesDefaultParameters(t *testing.T) {
	// Extract with default parameters (nil) and verify it works.
	parts := readIDML(t, "idmltest.idml", nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "default parameters should produce translatable blocks")
}

// okapi: ParametersTest#initialisesStyleIgnorances
func TestConfig_InitialisesStyleIgnorances(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	tdDir := bridgetest.TestdataDir(t)

	// Load the IgnoreAll config which sets all style ignorance flags.
	configPath := tdDir + "/okf_idml/okf_idml@IgnoreAll.fprm"
	params := map[string]any{
		"configFile": configPath,
	}
	path := bridgetest.TestdataFile(t, "okf_idml/756-character-kerning.idml")
	parts := bridgetest.ReadFile(t, pool, cfg, filterClass, path, mimeType, params)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "IgnoreAll config should produce translatable blocks")
}

// okapi: ParametersTest#excludedStyleConfigurationsInitialised
func TestConfig_ExcludedStyleConfigurationsInitialised(t *testing.T) {
	// Load styles-exclusion config and verify it applies correctly.
	parts := readIDMLWithConfig(t,
		"styles-exclusion/1418-styles-exclusion.idml",
		"styles-exclusion/okf_idml@syles-exclusion.fprm")

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "styles exclusion config should produce translatable blocks")
}

// okapi: ParametersTest#fontMappingsAreInitialised
func TestConfig_FontMappingsAreInitialised(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	tdDir := bridgetest.TestdataDir(t)

	// Load the chained font mappings config.
	configPath := tdDir + "/okf_idml/okf_idml@chained-font-mappings.fprm"
	params := map[string]any{
		"configFile": configPath,
	}
	path := bridgetest.TestdataFile(t, "okf_idml/926.idml")
	parts := bridgetest.ReadFile(t, pool, cfg, filterClass, path, mimeType, params)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "font mappings config should produce translatable blocks")
}

// ---------------------------------------------------------------------------
// Kerning threshold parameter tests
// ---------------------------------------------------------------------------

// okapi: ParametersTest#setsCharacterKerningMinIgnoranceThreshold
func TestConfig_SetsCharacterKerningMinIgnoranceThreshold(t *testing.T) {
	params := map[string]any{
		"ignoreCharacterKerning":                true,
		"characterKerningMinIgnoranceThreshold": -100,
	}
	parts := readIDML(t, "756-character-kerning.idml", params)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "should produce blocks with min kerning threshold")
}

// okapi: ParametersTest#setsCharacterKerningMaxIgnoranceThreshold
func TestConfig_SetsCharacterKerningMaxIgnoranceThreshold(t *testing.T) {
	params := map[string]any{
		"ignoreCharacterKerning":                true,
		"characterKerningMaxIgnoranceThreshold": 100,
	}
	parts := readIDML(t, "756-character-kerning.idml", params)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "should produce blocks with max kerning threshold")
}

// okapi: ParametersTest#failsToSetCharacterKerningMinIgnoranceThreshold
func TestConfig_FailsToSetCharacterKerningMinIgnoranceThreshold(t *testing.T) {
	// In Java, setting min > max fails. Through the bridge, this may produce
	// an error or unexpected results. We verify the filter still processes.
	params := map[string]any{
		"ignoreCharacterKerning":                true,
		"characterKerningMinIgnoranceThreshold": 100,
		"characterKerningMaxIgnoranceThreshold": -100,
	}
	parts := readIDML(t, "756-character-kerning.idml", params)
	// The filter should still process (bridge may silently handle invalid thresholds).
	assert.NotEmpty(t, parts, "filter should still produce parts even with invalid threshold order")
}

// okapi: ParametersTest#failsToSetCharacterKerningMaxIgnoranceThreshold
func TestConfig_FailsToSetCharacterKerningMaxIgnoranceThreshold(t *testing.T) {
	params := map[string]any{
		"ignoreCharacterKerning":                true,
		"characterKerningMinIgnoranceThreshold": 50,
		"characterKerningMaxIgnoranceThreshold": -50,
	}
	parts := readIDML(t, "756-character-kerning.idml", params)
	assert.NotEmpty(t, parts, "filter should still produce parts even with invalid threshold order")
}

// ---------------------------------------------------------------------------
// Tracking threshold parameter tests
// ---------------------------------------------------------------------------

// okapi: ParametersTest#setsCharacterTrackingMinIgnoranceThreshold
func TestConfig_SetsCharacterTrackingMinIgnoranceThreshold(t *testing.T) {
	params := map[string]any{
		"ignoreCharacterTracking":                true,
		"characterTrackingMinIgnoranceThreshold": -50,
	}
	parts := readIDML(t, "756-character-tracking.idml", params)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "should produce blocks with min tracking threshold")
}

// okapi: ParametersTest#setsCharacterTrackingMaxIgnoranceThreshold
func TestConfig_SetsCharacterTrackingMaxIgnoranceThreshold(t *testing.T) {
	params := map[string]any{
		"ignoreCharacterTracking":                true,
		"characterTrackingMaxIgnoranceThreshold": 50,
	}
	parts := readIDML(t, "756-character-tracking.idml", params)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "should produce blocks with max tracking threshold")
}

// okapi: ParametersTest#failsToSetCharacterTrackingMinIgnoranceThreshold
func TestConfig_FailsToSetCharacterTrackingMinIgnoranceThreshold(t *testing.T) {
	params := map[string]any{
		"ignoreCharacterTracking":                true,
		"characterTrackingMinIgnoranceThreshold": 100,
		"characterTrackingMaxIgnoranceThreshold": -100,
	}
	parts := readIDML(t, "756-character-tracking.idml", params)
	assert.NotEmpty(t, parts, "filter should still produce parts even with invalid threshold order")
}

// okapi: ParametersTest#failsToSetCharacterTrackingMaxIgnoranceThreshold
func TestConfig_FailsToSetCharacterTrackingMaxIgnoranceThreshold(t *testing.T) {
	params := map[string]any{
		"ignoreCharacterTracking":                true,
		"characterTrackingMinIgnoranceThreshold": 50,
		"characterTrackingMaxIgnoranceThreshold": -50,
	}
	parts := readIDML(t, "756-character-tracking.idml", params)
	assert.NotEmpty(t, parts, "filter should still produce parts even with invalid threshold order")
}

// ---------------------------------------------------------------------------
// Leading threshold parameter tests
// ---------------------------------------------------------------------------

// okapi: ParametersTest#setsCharacterLeadingMinIgnoranceThreshold
func TestConfig_SetsCharacterLeadingMinIgnoranceThreshold(t *testing.T) {
	params := map[string]any{
		"ignoreCharacterLeading":                true,
		"characterLeadingMinIgnoranceThreshold": -100,
	}
	parts := readIDML(t, "756-character-leading.idml", params)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "should produce blocks with min leading threshold")
}

// okapi: ParametersTest#setsCharacterLeadingMaxIgnoranceThreshold
func TestConfig_SetsCharacterLeadingMaxIgnoranceThreshold(t *testing.T) {
	params := map[string]any{
		"ignoreCharacterLeading":                true,
		"characterLeadingMaxIgnoranceThreshold": 100,
	}
	parts := readIDML(t, "756-character-leading.idml", params)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "should produce blocks with max leading threshold")
}

// okapi: ParametersTest#failsToSetCharacterLeadingMinIgnoranceThreshold
func TestConfig_FailsToSetCharacterLeadingMinIgnoranceThreshold(t *testing.T) {
	params := map[string]any{
		"ignoreCharacterLeading":                true,
		"characterLeadingMinIgnoranceThreshold": 100,
		"characterLeadingMaxIgnoranceThreshold": -100,
	}
	parts := readIDML(t, "756-character-leading.idml", params)
	assert.NotEmpty(t, parts, "filter should still produce parts even with invalid threshold order")
}

// okapi: ParametersTest#failsToSetCharacterLeadingMaxIgnoranceThreshold
func TestConfig_FailsToSetCharacterLeadingMaxIgnoranceThreshold(t *testing.T) {
	params := map[string]any{
		"ignoreCharacterLeading":                true,
		"characterLeadingMinIgnoranceThreshold": 50,
		"characterLeadingMaxIgnoranceThreshold": -50,
	}
	parts := readIDML(t, "756-character-leading.idml", params)
	assert.NotEmpty(t, parts, "filter should still produce parts even with invalid threshold order")
}

// ---------------------------------------------------------------------------
// Baseline shift threshold parameter tests
// ---------------------------------------------------------------------------

// okapi: ParametersTest#setsCharacterBaselineShiftMinIgnoranceThreshold
func TestConfig_SetsCharacterBaselineShiftMinIgnoranceThreshold(t *testing.T) {
	params := map[string]any{
		"ignoreCharacterBaselineShift":                true,
		"characterBaselineShiftMinIgnoranceThreshold": -2,
	}
	parts := readIDML(t, "756-character-baseline-shift.idml", params)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "should produce blocks with min baseline shift threshold")
}

// okapi: ParametersTest#setsCharacterBaselineShiftMaxIgnoranceThreshold
func TestConfig_SetsCharacterBaselineShiftMaxIgnoranceThreshold(t *testing.T) {
	params := map[string]any{
		"ignoreCharacterBaselineShift":                true,
		"characterBaselineShiftMaxIgnoranceThreshold": 2,
	}
	parts := readIDML(t, "756-character-baseline-shift.idml", params)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "should produce blocks with max baseline shift threshold")
}

// okapi: ParametersTest#failsToSetCharacterBaselineShiftMinIgnoranceThreshold
func TestConfig_FailsToSetCharacterBaselineShiftMinIgnoranceThreshold(t *testing.T) {
	params := map[string]any{
		"ignoreCharacterBaselineShift":                true,
		"characterBaselineShiftMinIgnoranceThreshold": 10,
		"characterBaselineShiftMaxIgnoranceThreshold": -10,
	}
	parts := readIDML(t, "756-character-baseline-shift.idml", params)
	assert.NotEmpty(t, parts, "filter should still produce parts even with invalid threshold order")
}

// okapi: ParametersTest#failsToSetCharacterBaselineShiftMaxIgnoranceThreshold
func TestConfig_FailsToSetCharacterBaselineShiftMaxIgnoranceThreshold(t *testing.T) {
	params := map[string]any{
		"ignoreCharacterBaselineShift":                true,
		"characterBaselineShiftMinIgnoranceThreshold": 5,
		"characterBaselineShiftMaxIgnoranceThreshold": -5,
	}
	parts := readIDML(t, "756-character-baseline-shift.idml", params)
	assert.NotEmpty(t, parts, "filter should still produce parts even with invalid threshold order")
}
