package bridge_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gokapi/gokapi/core/plugin/bridge"
	"github.com/gokapi/gokapi/core/plugin/loader"
	"github.com/gokapi/gokapi/core/preset"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// bridgeJAR returns the JAR path from GOKAPI_BRIDGE_JAR or skips the test.
func bridgeJAR(t *testing.T) string {
	t.Helper()
	jar := os.Getenv("GOKAPI_BRIDGE_JAR")
	if jar == "" {
		t.Skip("GOKAPI_BRIDGE_JAR not set — skipping bridge integration test")
	}
	return jar
}

// javaPath returns the Java binary to use. Respects JAVA_HOME if set,
// otherwise defaults to "java".
func javaPath(t *testing.T) string {
	t.Helper()
	if home := os.Getenv("JAVA_HOME"); home != "" {
		return filepath.Join(home, "bin", "java")
	}
	return "java"
}

// bridgeSchemasDir returns the schemas directory adjacent to the JAR.
func bridgeSchemasDir(t *testing.T, jar string) string {
	t.Helper()
	dir := filepath.Join(filepath.Dir(jar), "schemas")
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		t.Skipf("schemas directory not found at %s", dir)
	}
	return dir
}

func TestIntegrationListFilters(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping bridge integration test in short mode")
	}
	jar := bridgeJAR(t)

	b := bridge.NewJavaBridge(bridge.BridgeConfig{
		Command: javaPath(t),
		Args:    []string{"-jar", jar},
	}, nil)
	require.NoError(t, b.Start())
	defer func() { _ = b.Stop() }()

	lf, err := b.ListFilters()
	require.NoError(t, err)

	// The shaded JAR includes 9 filter JARs producing ~10 filter classes.
	assert.GreaterOrEqual(t, len(lf.Filters), 8,
		"expected at least 8 filters, got %d", len(lf.Filters))

	// Spot-check a few well-known filters.
	filterNames := make(map[string]bool)
	for _, f := range lf.Filters {
		filterNames[f.Name] = true
	}
	assert.True(t, filterNames["html"], "expected html filter")
	assert.True(t, filterNames["json"], "expected json filter")
	assert.True(t, filterNames["xml"], "expected xml filter")
	assert.True(t, filterNames["xliff"], "expected xliff filter")
}

func TestIntegrationFilterParamsApplied(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping bridge integration test in short mode")
	}
	jar := bridgeJAR(t)

	b := bridge.NewJavaBridge(bridge.BridgeConfig{
		Command: javaPath(t),
		Args:    []string{"-jar", jar},
	}, nil)
	require.NoError(t, b.Start())
	defer func() { _ = b.Stop() }()

	// Use a JSON document with filter_params to test parameter application.
	jsonDoc := `{"greeting": "Hello World", "count": 42}`

	err := b.Open(bridge.OpenParams{
		FilterClass:  "net.sf.okapi.filters.json.JSONFilter",
		URI:          "test.json",
		SourceLocale: "en",
		Encoding:     "UTF-8",
		Content:      []byte(jsonDoc),
		MimeType:     "application/json",
		FilterParams: map[string]any{
			"extractAllPairs": true,
			"useFullKeyPath":  true,
			"useCodeFinder":   false,
		},
	})
	require.NoError(t, err)

	parts, err := b.Read()
	require.NoError(t, err)
	assert.NotEmpty(t, parts, "should have extracted parts from JSON")

	require.NoError(t, b.CloseFilter())
}

func TestIntegrationSchemaLoading(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping bridge integration test in short mode")
	}
	jar := bridgeJAR(t)
	schemasDir := bridgeSchemasDir(t, jar)

	reg := loader.NewSchemaRegistry()
	require.NoError(t, reg.LoadFromDirectory(schemasDir))

	// okapi-bridge includes 45-57 schema files depending on version.
	assert.GreaterOrEqual(t, reg.Count(), 40,
		"expected at least 40 schemas, got %d", reg.Count())

	// Spot-check known filter schemas.
	for _, filterID := range []string{"okf_html", "okf_json", "okf_xliff", "okf_properties"} {
		s, ok := reg.GetSchema(filterID)
		require.True(t, ok, "expected schema for %s", filterID)
		assert.NotEmpty(t, s.FilterMeta.ID, "schema %s should have x-filter.id", filterID)
		assert.NotEmpty(t, s.FilterMeta.Class, "schema %s should have x-filter.class", filterID)
	}

	// Schemas with rich properties should have them parsed correctly.
	// Properties contains the raw schema sections; FlatProperties has flattened names.
	jsonSchema, ok := reg.GetSchema("okf_json")
	require.True(t, ok)
	assert.NotEmpty(t, jsonSchema.Properties, "okf_json should have properties")
	assert.Contains(t, jsonSchema.Properties, "extraction",
		"okf_json should have 'extraction' section in Properties")
	assert.NotEmpty(t, jsonSchema.FlatProperties, "okf_json should have FlatProperties")
	assert.Contains(t, jsonSchema.FlatProperties, "extractAllPairs",
		"okf_json FlatProperties should contain 'extractAllPairs'")
	assert.Equal(t, "boolean", jsonSchema.FlatProperties["extractAllPairs"].Type)

	// SectionMap should map flat names to section keys.
	assert.Equal(t, "extraction", jsonSchema.SectionMap["extractAllPairs"])

	// GetSectionMap should work by filter class.
	sm := reg.GetSectionMap("net.sf.okapi.filters.json.JSONFilter")
	assert.NotNil(t, sm, "should have section map for JSON filter class")
	assert.Equal(t, "extraction", sm["extractAllPairs"])

	// Validate known-good params pass validation (uses FlatProperties).
	err := reg.ValidateParams("okf_json", map[string]any{
		"extractAllPairs": true,
	})
	assert.NoError(t, err)

	// Validate unknown param is rejected.
	err = reg.ValidateParams("okf_json", map[string]any{
		"nonexistentParam": "hello",
	})
	assert.Error(t, err)
}

func TestIntegrationExtractPresets(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping bridge integration test in short mode")
	}
	jar := bridgeJAR(t)
	schemasDir := bridgeSchemasDir(t, jar)

	schemaReg := loader.NewSchemaRegistry()
	require.NoError(t, schemaReg.LoadFromDirectory(schemasDir))

	presetReg := preset.NewPresetRegistry()
	schemaReg.ExtractPresets(presetReg)

	formats := presetReg.FormatNames()
	t.Logf("Extracted presets for %d formats: %v", len(formats), formats)

	assert.GreaterOrEqual(t, schemaReg.Count(), 40,
		"schemas loaded correctly for preset extraction pipeline")
}
