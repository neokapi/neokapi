//go:build integration

package json

import (
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/plugin/bridge/filters/bridgetest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Tests translated from JSONFilterTest.java — configuration/metadata tests.
// These test metadata-related extraction rules, subfilters with metadata,
// and combined feature scenarios.
// ---------------------------------------------------------------------------

// okapi: JSONFilterTest#metaDataAndExtractionRulesWithSubfilter
func TestConfig_MetaDataAndExtractionRulesWithSubfilter(t *testing.T) {
	// Metadata + extraction rules + HTML subfilter combined.
	// Uses metadata.json and metadata.fprm.
	pool, cfg := bridgetest.SharedBridge(t)
	path := bridgetest.TestdataFile(t, "okf_json/metadata.json")

	parts := bridgetest.ReadFile(t, pool, cfg, filterClass, path, mimeType, map[string]any{
		"extractAllPairs":          false,
		"useFullKeyPath":           true,
		"useLeadingSlashOnKeyPath": true,
		"useKeyAsName":             false,
		"subfilter":                "okf_html",
		"extractionRules":          `/widgets/body.*`,
		"noteRules":                `/widgets/name.*`,
		"idRules":                  `/widgets/id.*`,
		"genericMetaRules":         `/widgets/image.*`,
	})

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	// Body keys should be extracted (they match extractionRules).
	var foundBody bool
	for _, b := range blocks {
		if strings.Contains(b.SourceText(), "blurb") {
			foundBody = true
			break
		}
	}
	assert.True(t, foundBody, "body values should be extracted via extractionRules")

	// Name keys should NOT be extracted as translatable (they are noteRules).
	texts := bridgetest.BlockTexts(blocks)
	for _, text := range texts {
		assert.NotEqual(t, "The Year of the Tiger", text, "name values should be notes, not translatable text")
	}

	// ID keys should not be translatable text (they are idRules).
	for _, text := range texts {
		assert.NotEqual(t, "115013866768", text, "id values should be used as TU names, not translatable text")
	}
}

// okapi: JSONFilterTest#metaDataAndExtractionRulesNestedNotes
func TestConfig_MetaDataAndExtractionRulesNestedNotes(t *testing.T) {
	// Nested note rules with metadata from file.
	pool, cfg := bridgetest.SharedBridge(t)
	path := bridgetest.TestdataFile(t, "okf_json/metadata-nested.json")

	parts := bridgetest.ReadFile(t, pool, cfg, filterClass, path, mimeType, map[string]any{
		"extractAllPairs":          false,
		"useFullKeyPath":           true,
		"useLeadingSlashOnKeyPath": true,
		"useKeyAsName":             false,
		"subfilter":                "okf_html",
		"extractionRules":          `/widgets/body.*`,
		"noteRules":                `/widgets/name.*`,
		"idRules":                  `/widgets/id.*`,
		"genericMetaRules":         `/widgets/image.*`,
	})

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	// Body value should be extracted.
	var foundBody bool
	for _, b := range blocks {
		if strings.Contains(b.SourceText(), "blurb") {
			foundBody = true
			break
		}
	}
	assert.True(t, foundBody, "body value should be extracted")

	// Nested name values (prefix, animal) should be notes.
	texts := bridgetest.BlockTexts(blocks)
	for _, text := range texts {
		assert.NotEqual(t, "Tiger", text, "nested name values should be notes")
		assert.NotContains(t, text, "The Year of the ", "nested name prefix should be a note")
	}
}
