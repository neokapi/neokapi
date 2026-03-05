//go:build integration

package pdf

import (
	"strings"
	"testing"

	"github.com/gokapi/gokapi/core/model"
	"github.com/gokapi/gokapi/core/plugin/bridge/filters/bridgetest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// okapi: PdfFilterTest#testStartDocument
func TestExtract_StartDocument(t *testing.T) {
	parts := readPDF(t, "OmegaT_documentation_en.PDF", nil)

	// Should produce a LayerStart at the beginning.
	require.NotEmpty(t, parts)
	assert.Equal(t, model.PartLayerStart, parts[0].Type, "first part should be LayerStart")
}

// okapi-unmapped: PdfFilterTest#testDefaultInfo — Java-only API test (IFilter.getParameters/getName/getConfigurations)

// okapi: PdfFilterTest#firstTextUnit
func TestExtract_FirstTextUnit(t *testing.T) {
	t.Run("PALC_2011_LT", func(t *testing.T) {
		// Java test uses lineSeparator="" and paragraphSeparator="\n\n"
		parts := readPDF(t, "PALC_2011_LT.pdf", map[string]any{
			"lineSeparator":      "",
			"paragraphSeparator": "\n\n",
		})

		blocks := bridgetest.TranslatableBlocks(parts)
		require.NotEmpty(t, blocks, "should extract translatable blocks from PALC_2011_LT.pdf")

		// The first text unit should be the title.
		assert.Equal(t, "Translation Quality Checking in LanguageTool", blocks[0].SourceText())
	})

	t.Run("OmegaT_documentation_en", func(t *testing.T) {
		// Default parameters (no custom lineSeparator/paragraphSeparator).
		parts := readPDF(t, "OmegaT_documentation_en.PDF", nil)

		blocks := bridgetest.TranslatableBlocks(parts)
		require.NotEmpty(t, blocks, "should extract translatable blocks from OmegaT_documentation_en.PDF")

		// The first text unit should be the title line.
		assert.Equal(t, "OmegaT 3.1 - User's Guide Vito Smolej", blocks[0].SourceText())
	})

	t.Run("TAUS_QualityDashboard", func(t *testing.T) {
		// Default parameters; the second text unit should be "TAUS Quality Dashboard".
		parts := readPDF(t, "TAUS-QualityDashboard-September.pdf", nil)

		blocks := bridgetest.TranslatableBlocks(parts)
		require.GreaterOrEqual(t, len(blocks), 2, "should extract at least 2 translatable blocks from TAUS PDF")

		// Java gets the 2nd text unit (1-based index 2).
		assert.Equal(t, "TAUS Quality Dashboard", blocks[1].SourceText())
	})
}

// okapi: PdfFilterTest#firstParagraphTextUnit
func TestExtract_FirstParagraphTextUnit(t *testing.T) {
	t.Run("PALC_2011_LT", func(t *testing.T) {
		// Java test uses lineSeparator="\n" and paragraphSeparator="\n"
		parts := readPDF(t, "PALC_2011_LT.pdf", map[string]any{
			"lineSeparator":      "\n",
			"paragraphSeparator": "\n",
		})

		blocks := bridgetest.TranslatableBlocks(parts)
		require.GreaterOrEqual(t, len(blocks), 3, "should extract at least 3 blocks from PALC_2011_LT.pdf")

		// Java gets the 3rd text unit (1-based index 3).
		text := blocks[2].SourceText()
		assert.True(t, strings.HasPrefix(text, "Abstract: In large computer-aided translation"),
			"3rd block should start with 'Abstract: In large computer-aided translation', got: %s", truncate(text, 80))
	})

	t.Run("OmegaT_documentation_en", func(t *testing.T) {
		// Default parameters; the 5th text unit starts with "This document is the official user's guide to OmegaT".
		parts := readPDF(t, "OmegaT_documentation_en.PDF", nil)

		blocks := bridgetest.TranslatableBlocks(parts)
		require.GreaterOrEqual(t, len(blocks), 5, "should extract at least 5 blocks from OmegaT PDF")

		// Java gets the 5th text unit (1-based index 5).
		text := blocks[4].SourceText()
		assert.True(t, strings.HasPrefix(text, "This document is the official user's guide to OmegaT"),
			"5th block should start with 'This document is the official...', got: %s", truncate(text, 80))
	})

	t.Run("TAUS_QualityDashboard", func(t *testing.T) {
		// Default parameters; the 6th text unit starts with "This document describes how the TAUS Dynamic Quality Framework".
		parts := readPDF(t, "TAUS-QualityDashboard-September.pdf", nil)

		blocks := bridgetest.TranslatableBlocks(parts)
		require.GreaterOrEqual(t, len(blocks), 6, "should extract at least 6 blocks from TAUS PDF")

		// Java gets the 6th text unit (1-based index 6).
		text := blocks[5].SourceText()
		assert.True(t, strings.HasPrefix(text, "This document describes how the TAUS Dynamic Quality Framework"),
			"6th block should start with 'This document describes...', got: %s", truncate(text, 80))
	})
}

// truncate returns the first n characters of s, appending "..." if truncated.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
