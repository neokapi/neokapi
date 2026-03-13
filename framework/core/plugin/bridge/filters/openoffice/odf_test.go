//go:build integration

package openoffice

import (
	"os"
	"testing"

	"github.com/neokapi/neokapi/core/plugin/bridge/filters/bridgetest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// okapi: ODFFilterTest#testFirstTextUnit
func TestODF_FirstTextUnit(t *testing.T) {
	parts := readODFFile(t, "okapi/filters/openoffice/src/test/resources/TestDocument01.odt_content.xml", nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "should extract translatable blocks from content.xml")

	// The first text unit in TestDocument01.odt_content.xml is "Heading 1".
	assert.Equal(t, "Heading 1", blocks[0].SourceText())
}

// okapi: ODFFilterTest#testITSMarkup
func TestODF_ITSMarkup(t *testing.T) {
	parts := readODFFile(t, "okapi/filters/openoffice/src/test/resources/Content_WithITS.xml", nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "should extract translatable blocks from ITS content")

	texts := bridgetest.BlockTexts(blocks)

	// The Java test checks specific text units at indices 2, 3, 5, 6.
	// Text unit 2: translate='no' spans with "To translate ... but translate this."
	foundTranslate := false
	for _, text := range texts {
		if containsAll(text, "To translate", "translate this") {
			foundTranslate = true
			break
		}
	}
	assert.True(t, foundTranslate, "should find ITS translate text unit")

	// Text unit 3: localization notes with "Text with a note."
	foundNotes := false
	for _, text := range texts {
		if containsAll(text, "Text with", "note") {
			foundNotes = true
			break
		}
	}
	assert.True(t, foundNotes, "should find ITS localization notes text unit")

	// Text unit 5: terminology with "a very long term made of several words"
	foundTerm := false
	for _, text := range texts {
		if containsAll(text, "very long term", "several words") {
			foundTerm = true
			break
		}
	}
	assert.True(t, foundTerm, "should find ITS terminology text unit")

	// Text unit 6: locale filter with "Locale filter: for FR and Not for FR."
	foundLocale := false
	for _, text := range texts {
		if containsAll(text, "Locale filter", "FR") {
			foundLocale = true
			break
		}
	}
	assert.True(t, foundLocale, "should find ITS locale filter text unit")

	// Verify some blocks have inline codes (spans) from ITS annotations.
	hasSpans := false
	for _, b := range blocks {
		frag := b.FirstFragment()
		if frag != nil && len(frag.Spans) > 0 {
			hasSpans = true
			break
		}
	}
	assert.True(t, hasSpans, "ITS markup should produce inline codes (spans)")
}

// okapi: ODFFilterTest#testDefaultInfo
func TestODF_DefaultInfo(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, odfFilterClass)

	// Verify the filter is available and can provide info.
	b, err := pool.Acquire(cfg)
	require.NoError(t, err)
	defer pool.Release(b)

	info, err := b.Info(odfFilterClass)
	require.NoError(t, err)
	assert.NotEmpty(t, info.Name, "filter should have a name")
	assert.NotEmpty(t, info.DisplayName, "filter should have a display name")
}

// okapi: ODFFilterTest#testDoubleExtraction
func TestODF_DoubleExtraction(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, odfFilterClass)

	// The Java test roundtrips several ODF XML files.
	files := []string{
		"okapi/filters/openoffice/src/test/resources/TestDocument01.odt_content.xml",
		"okapi/filters/openoffice/src/test/resources/TestDocument01.odt_meta.xml",
		"okapi/filters/openoffice/src/test/resources/TestDocument01.odt_styles.xml",
		"okapi/filters/openoffice/src/test/resources/TestDocument02.odt_content.xml",
		"okapi/filters/openoffice/src/test/resources/ODFTest_footnote.xml",
		"okapi/filters/openoffice/src/test/resources/TestSpreadsheet01.ods_content.xml",
	}

	for _, f := range files {
		t.Run(f, func(t *testing.T) {
			path := bridgetest.TestdataFile(t, f)
			content, err := os.ReadFile(path)
			require.NoError(t, err)
			bridgetest.AssertRoundTripEvents(t, pool, cfg, odfFilterClass, content, path, odfMimeType, nil)
		})
	}
}
