//go:build integration

// Package filters contains integration tests that verify the bridge works
// end-to-end with multiple Okapi filter types.
package filters

import (
	"testing"

	"github.com/neokapi/neokapi/core/plugin/bridge/filters/bridgetest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- cascadingfilter surefire: CascadingFilterTest ---
//
// okapi-filter: cascadingfilter
// okapi-unmapped: CascadingFilterTest#extractFeastWithStream — Java-internal cascading filter extraction test
// okapi-unmapped: CascadingFilterTest#extractMinimalNormally — Java-internal cascading filter extraction test
// okapi-unmapped: CascadingFilterTest#extractMinimalWithStream — Java-internal cascading filter extraction test
// okapi-unmapped: CascadingFilterTest#extractNormally — Java-internal cascading filter extraction test
// okapi-unmapped: CascadingFilterTest#extractWithStream — Java-internal cascading filter extraction test

// TestBridgeSmoke_ListFilters verifies the bridge can start and list all
// available Okapi filters. This is the most basic sanity check.
func TestBridgeSmoke_ListFilters(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	b, err := pool.Acquire(cfg)
	require.NoError(t, err)
	defer pool.Release(b)

	filters, err := b.ListFilters()
	require.NoError(t, err)
	require.NotNil(t, filters)

	// The shaded JAR discovers 10+ filter classes from ~9 filter JARs.
	assert.GreaterOrEqual(t, len(filters.Filters), 8,
		"should discover Okapi filters")

	// Spot-check a few well-known filters.
	filterNames := make(map[string]bool)
	for _, f := range filters.Filters {
		filterNames[f.FilterClass] = true
	}
	assert.True(t, filterNames["net.sf.okapi.filters.html.HtmlFilter"], "HTML filter should be available")
	assert.True(t, filterNames["net.sf.okapi.filters.json.JSONFilter"], "JSON filter should be available")
	assert.True(t, filterNames["net.sf.okapi.filters.properties.PropertiesFilter"], "Properties filter should be available")
}

// TestBridgeSmoke_HTMLExtraction verifies HTML content extraction through
// the full bridge pipeline (open → read → close).
func TestBridgeSmoke_HTMLExtraction(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	parts := bridgetest.ReadString(t, pool, cfg,
		"net.sf.okapi.filters.html.HtmlFilter",
		`<html><body><h1>Title</h1><p>Hello <b>world</b></p></body></html>`,
		"test.html", "text/html", nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.GreaterOrEqual(t, len(blocks), 2, "should extract title and paragraph blocks")

	texts := bridgetest.BlockTexts(blocks)
	assert.Contains(t, texts, "Title")
}

// TestBridgeSmoke_JSONExtraction verifies JSON content extraction.
func TestBridgeSmoke_JSONExtraction(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	parts := bridgetest.ReadString(t, pool, cfg,
		"net.sf.okapi.filters.json.JSONFilter",
		`{"greeting": "Hello", "farewell": "Goodbye"}`,
		"test.json", "application/json", nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.Len(t, blocks, 2, "should extract two translatable values")

	texts := bridgetest.BlockTexts(blocks)
	assert.Contains(t, texts, "Hello")
	assert.Contains(t, texts, "Goodbye")
}

// TestBridgeSmoke_PropertiesExtraction verifies Java properties extraction.
func TestBridgeSmoke_PropertiesExtraction(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	parts := bridgetest.ReadString(t, pool, cfg,
		"net.sf.okapi.filters.properties.PropertiesFilter",
		"greeting=Hello World\nfarewell=Goodbye\n",
		"test.properties", "text/x-java-properties", nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.Len(t, blocks, 2, "should extract two translatable properties")

	texts := bridgetest.BlockTexts(blocks)
	assert.Contains(t, texts, "Hello World")
	assert.Contains(t, texts, "Goodbye")
}

// TestBridgeSmoke_YAMLExtraction verifies YAML content extraction.
func TestBridgeSmoke_YAMLExtraction(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	parts := bridgetest.ReadString(t, pool, cfg,
		"net.sf.okapi.filters.yaml.YamlFilter",
		"greeting: Hello\nfarewell: Goodbye\n",
		"test.yaml", "application/x-yaml", nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.Len(t, blocks, 2, "should extract two translatable YAML values")

	texts := bridgetest.BlockTexts(blocks)
	assert.Contains(t, texts, "Hello")
	assert.Contains(t, texts, "Goodbye")
}

// TestBridgeSmoke_MarkdownExtraction verifies Markdown content extraction.
// Skipped: MarkdownFilter is not included in the current shaded JAR.
func TestBridgeSmoke_MarkdownExtraction(t *testing.T) {
	t.Skip("MarkdownFilter not included in shaded JAR")
}

// TestBridgeSmoke_POExtraction verifies PO file extraction.
func TestBridgeSmoke_POExtraction(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	parts := bridgetest.ReadString(t, pool, cfg,
		"net.sf.okapi.filters.po.POFilter",
		`msgid "Hello"
msgstr ""

msgid "Goodbye"
msgstr ""
`,
		"test.po", "application/x-gettext", nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "should extract translatable msgid entries")
	texts := bridgetest.BlockTexts(blocks)
	assert.Contains(t, texts, "Hello")
}

// TestBridgeSmoke_MultipleSequentialOperations verifies the bridge handles
// multiple open → read → close cycles sequentially without protocol corruption.
func TestBridgeSmoke_MultipleSequentialOperations(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	// Run three consecutive extraction operations through the same bridge.
	for i, tc := range []struct {
		filter  string
		content string
		uri     string
		mime    string
		want    string
	}{
		{
			filter:  "net.sf.okapi.filters.html.HtmlFilter",
			content: `<p>First</p>`,
			uri:     "a.html", mime: "text/html",
			want: "First",
		},
		{
			filter:  "net.sf.okapi.filters.json.JSONFilter",
			content: `{"key":"Second"}`,
			uri:     "b.json", mime: "application/json",
			want: "Second",
		},
		{
			filter:  "net.sf.okapi.filters.properties.PropertiesFilter",
			content: "key=Third\n",
			uri:     "c.properties", mime: "text/x-java-properties",
			want: "Third",
		},
	} {
		parts := bridgetest.ReadString(t, pool, cfg, tc.filter, tc.content, tc.uri, tc.mime, nil)
		blocks := bridgetest.TranslatableBlocks(parts)
		require.NotEmpty(t, blocks, "operation %d: should extract blocks", i)
		texts := bridgetest.BlockTexts(blocks)
		assert.Contains(t, texts, tc.want, "operation %d", i)
	}
}
