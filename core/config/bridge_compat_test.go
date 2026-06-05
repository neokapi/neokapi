package config

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These tests verify that neokapi's config envelope system produces Kind and
// apiVersion values compatible with okapi-bridge v2.13.0.
//
// The bridge's FilterRegistry.toKind() maps "okf_{format}" → "Okf{Format}FilterConfig"
// and FilterRegistry.toApiVersion(n) → "vN". The bridge's resolveByKind() does the
// reverse: "Okf{Format}FilterConfig" → "okf_{format}" → filter class lookup.
//
// If these tests fail, configs written by neokapi won't be understood by the bridge
// (or vice versa).

// TestBridgeKindNaming verifies that OkapiFilterConfigKind produces the same
// Kind strings as okapi-bridge's FilterRegistry.toKind() for all bridge filters
// that have corresponding neokapi native formats.
func TestBridgeKindNaming(t *testing.T) {
	t.Parallel()
	// Map from Okapi filter ID suffix to the expected Kind from versions.json.
	// These are the exact values produced by the bridge's toKind() method.
	bridgeKinds := map[string]string{
		"html":       "OkfHtmlFilterConfig",
		"json":       "OkfJsonFilterConfig",
		"xml":        "OkfXmlFilterConfig",
		"yaml":       "OkfYamlFilterConfig",
		"properties": "OkfPropertiesFilterConfig",
		"po":         "OkfPoFilterConfig",
		"xmlstream":  "OkfXmlstreamFilterConfig",
		"openxml":    "OkfOpenxmlFilterConfig",
		"idml":       "OkfIdmlFilterConfig",
		"regex":      "OkfRegexFilterConfig",
		"archive":    "OkfArchiveFilterConfig",
	}

	for format, expectedKind := range bridgeKinds {
		t.Run(format, func(t *testing.T) {
			t.Parallel()
			neokapiKind := OkapiFilterConfigKind(format)
			assert.Equal(t, Kind(expectedKind), neokapiKind,
				"neokapi OkapiFilterConfigKind(%q) must match bridge toKind(\"okf_%s\")", format, format)
		})
	}
}

// TestBridgeAPIVersionFormat verifies that FormatAPIVersion produces the same
// format as okapi-bridge's FilterRegistry.toApiVersion().
func TestBridgeAPIVersionFormat(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "v1", FormatAPIVersion(1))
	assert.Equal(t, "v2", FormatAPIVersion(2))
	assert.Equal(t, "v3", FormatAPIVersion(3))

	// Verify round-trip: format → parse
	for i := 1; i <= 5; i++ {
		s := FormatAPIVersion(i)
		n, err := ParseAPIVersion(s)
		require.NoError(t, err)
		assert.Equal(t, i, n)
	}
}

// TestBridgeEnvelopeParsing verifies that configs emitted in bridge envelope
// format can be parsed by neokapi's config.Parse().
func TestBridgeEnvelopeParsing(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name         string
		yaml         string
		expectedKind Kind
	}{
		{
			name: "OkfHtmlFilterConfig",
			yaml: `apiVersion: v1
kind: OkfHtmlFilterConfig
metadata:
  name: well-formed
spec:
  assumeWellformed: true
  preserveWhitespace: true
`,
			expectedKind: OkapiFilterConfigKind("html"),
		},
		{
			name: "OkfJsonFilterConfig",
			yaml: `apiVersion: v1
kind: OkfJsonFilterConfig
metadata:
  name: extract-all
spec:
  extractAllPairs: true
  useFullKeyPath: true
`,
			expectedKind: OkapiFilterConfigKind("json"),
		},
		{
			name: "OkfXmlFilterConfig v2",
			yaml: `apiVersion: v2
kind: OkfXmlFilterConfig
metadata:
  name: custom-xml
spec:
  preserveWhitespace: false
`,
			expectedKind: OkapiFilterConfigKind("xml"),
		},
		{
			name: "OkfPropertiesFilterConfig",
			yaml: `apiVersion: v1
kind: OkfPropertiesFilterConfig
metadata:
  name: java-props
spec:
  useKeyAsName: true
`,
			expectedKind: OkapiFilterConfigKind("properties"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			env, err := Parse([]byte(tt.yaml), ".yaml")
			require.NoError(t, err)
			assert.Equal(t, tt.expectedKind, env.Kind)
			assert.True(t, IsFormatConfigKind(env.Kind))
			assert.True(t, env.Kind.IsValid())
			assert.NotEmpty(t, env.Spec)
		})
	}
}

// TestBridgeEnvelopeJSON verifies JSON envelope parsing matches bridge output.
// The bridge can emit configs as JSON (e.g., from schema generator).
func TestBridgeEnvelopeJSON(t *testing.T) {
	t.Parallel()
	jsonData := []byte(`{
		"apiVersion": "v1",
		"kind": "OkfHtmlFilterConfig",
		"metadata": {"name": "bridge-html"},
		"spec": {
			"assumeWellformed": true,
			"useCodeFinder": false,
			"preserveWhitespace": true
		}
	}`)

	env, err := Parse(jsonData, ".json")
	require.NoError(t, err)
	assert.Equal(t, OkapiFilterConfigKind("html"), env.Kind)
	assert.Equal(t, "v1", env.APIVersion)
	assert.Equal(t, true, env.Spec["assumeWellformed"])
	assert.Equal(t, false, env.Spec["useCodeFinder"])
}

// TestBridgeResolveByKindCompatibility verifies that neokapi's OkapiFilterConfigKind
// produces kinds that the bridge's resolveByKind() would accept.
// Bridge validation: kind must start with "Okf" and end with "FilterConfig".
func TestBridgeResolveByKindCompatibility(t *testing.T) {
	t.Parallel()
	formats := []string{
		"html", "json", "xml", "yaml", "properties",
		"po", "xmlstream", "openxml", "idml", "regex",
		"archive", "autoxliff", "dtd", "ts",
	}

	for _, format := range formats {
		t.Run(format, func(t *testing.T) {
			t.Parallel()
			kind := OkapiFilterConfigKind(format)
			s := string(kind)
			// Bridge validates: starts with "Okf" and ends with "FilterConfig"
			assert.Greater(t, len(s), len("Okf")+len("FilterConfig"),
				"kind %q too short", s)
			assert.Equal(t, "Okf", s[:3],
				"kind %q must start with Okf", s)
			assert.Equal(t, "FilterConfig", s[len(s)-12:],
				"kind %q must end with FilterConfig", s)

			// Bridge extracts format: strip "Okf" prefix and "FilterConfig" suffix,
			// then lowercases. Verify round-trip.
			extracted := s[3 : len(s)-12]
			assert.NotEmpty(t, extracted)
			// First char should be uppercase (PascalCase)
			assert.Equal(t, strings.ToUpper(extracted[:1]), extracted[:1])
		})
	}
}

// TestBridgeTransformPipeline verifies the full pipeline: bridge envelope →
// parse → transform → native format config.
func TestBridgeTransformPipeline(t *testing.T) {
	t.Parallel()
	reg := NewTransformRegistry()

	// Register a mock Okf→native transform (like the real html/json ones)
	okapiHTMLKind := OkapiFilterConfigKind("html")
	nativeHTMLKind := FormatConfigKind("html")
	reg.Register(okapiHTMLKind, nativeHTMLKind,
		TransformerFunc(func(spec map[string]any) (map[string]any, error) {
			result := make(map[string]any)
			for k, v := range spec {
				// Drop okapi-only params
				switch k {
				case "quoteMode", "quoteModeDefined", "assumeWellformed":
					continue
				}
				result[k] = v
			}
			return result, nil
		}))

	// Parse a bridge-style envelope
	yaml := `apiVersion: v1
kind: OkfHtmlFilterConfig
metadata:
  name: bridge-config
spec:
  preserveWhitespace: true
  assumeWellformed: true
  quoteMode: 3
  quoteModeDefined: true
  useCodeFinder: false
`
	env, err := Parse([]byte(yaml), ".yaml")
	require.NoError(t, err)

	// Transform to native
	result, err := reg.Transform(env.Kind, nativeHTMLKind, env.Spec)
	require.NoError(t, err)

	// Shared params kept
	assert.Equal(t, true, result["preserveWhitespace"])
	assert.Equal(t, false, result["useCodeFinder"])

	// Okapi-only params dropped
	assert.Nil(t, result["quoteMode"])
	assert.Nil(t, result["quoteModeDefined"])
	assert.Nil(t, result["assumeWellformed"])
}

// TestBridgeEnvelopeUnwrapParams verifies the envelope unwrapping that the
// bridge performs in applyFilterParams. When the bridge receives kind + spec
// in filter_params, it unwraps spec as the actual params. This test ensures
// neokapi can construct such an envelope correctly.
func TestBridgeEnvelopeUnwrapParams(t *testing.T) {
	t.Parallel()
	// Simulate what neokapi sends to the bridge as filter_params
	kind := OkapiFilterConfigKind("html")
	apiVersion := FormatAPIVersion(1)

	// These would be serialized as map<string, string> in gRPC
	filterParams := map[string]string{
		"kind":       string(kind),
		"apiVersion": apiVersion,
		"spec":       `{"assumeWellformed":true,"preserveWhitespace":true}`,
	}

	// Bridge validates kind format
	assert.True(t, strings.HasPrefix(filterParams["kind"], "Okf"))
	assert.True(t, strings.HasSuffix(filterParams["kind"], "FilterConfig"))

	// Bridge validates apiVersion format
	_, err := ParseAPIVersion(filterParams["apiVersion"])
	require.NoError(t, err)
}

// TestBridgeVersionsJsonKindConsistency verifies naming for all filters in
// the bridge's versions.json. The bridge uses toKind("okf_{format}") which
// does: strip "okf_" prefix, PascalCase first char, append "FilterConfig".
// neokapi's OkapiFilterConfigKind does: "Okf" + PascalCase(format) + "FilterConfig".
//
// This test verifies both algorithms agree for various filter names.
func TestBridgeVersionsJsonKindConsistency(t *testing.T) {
	t.Parallel()
	// These are real entries from versions.json in okapi-bridge v2.13.0
	versionsEntries := []struct {
		filterID   string // e.g., "okf_html"
		bridgeKind string // exact value from versions.json
	}{
		{"okf_html", "OkfHtmlFilterConfig"},
		{"okf_json", "OkfJsonFilterConfig"},
		{"okf_xml", "OkfXmlFilterConfig"},
		{"okf_yaml", "OkfYamlFilterConfig"},
		{"okf_properties", "OkfPropertiesFilterConfig"},
		{"okf_po", "OkfPoFilterConfig"},
		{"okf_xmlstream", "OkfXmlstreamFilterConfig"},
		{"okf_openxml", "OkfOpenxmlFilterConfig"},
		{"okf_idml", "OkfIdmlFilterConfig"},
		{"okf_regex", "OkfRegexFilterConfig"},
		{"okf_archive", "OkfArchiveFilterConfig"},
		{"okf_autoxliff", "OkfAutoxliffFilterConfig"},
		{"okf_baseplaintext", "OkfBaseplaintextFilterConfig"},
		{"okf_basetable", "OkfBasetableFilterConfig"},
		{"okf_commaseparatedvalues", "OkfCommaseparatedvaluesFilterConfig"},
	}

	for _, entry := range versionsEntries {
		t.Run(entry.filterID, func(t *testing.T) {
			t.Parallel()
			// Strip "okf_" prefix to get the format name
			format := strings.TrimPrefix(entry.filterID, "okf_")
			neokapiKind := OkapiFilterConfigKind(format)
			assert.Equal(t, Kind(entry.bridgeKind), neokapiKind,
				"OkapiFilterConfigKind(%q) must produce %q to match bridge versions.json",
				format, entry.bridgeKind)
		})
	}
}

// TestNativeKindNotConfusedWithBridgeKind verifies that native format kinds
// (HtmlFormatConfig) are distinct from bridge filter kinds (OkfHtmlFilterConfig).
func TestNativeKindNotConfusedWithBridgeKind(t *testing.T) {
	t.Parallel()
	formats := []string{"html", "json", "xml", "yaml"}
	for _, f := range formats {
		native := FormatConfigKind(f)
		bridge := OkapiFilterConfigKind(f)
		assert.NotEqual(t, native, bridge,
			"native and bridge kinds must be distinct for %q", f)
		assert.True(t, IsFormatConfigKind(native))
		assert.True(t, IsFormatConfigKind(bridge))
	}
}
