package sievepen

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestBulkAdd_PreservesVariantTextVerbatim is the regression test for the
// bulk fast path storing NormalizeText-collapsed text as the variant
// CONTENT: multi-line targets (CLI Long help, docs paragraphs) came back as
// a single line after any bulk import (TMX, klftm). Normalization is for
// the derived matching keys only — the stored text must round-trip
// verbatim, exactly like the non-bulk AddWithStream path.
func TestBulkAdd_PreservesVariantTextVerbatim(t *testing.T) {
	ctx := context.Background()
	tm, err := NewSQLiteTM(filepath.Join(t.TempDir(), "tm.db"))
	require.NoError(t, err)
	defer tm.Close()

	multiline := "First line.\nSecond line.\n\n  Indented third."
	bracketed := "[note] leading bracket survives"

	entries := []TMEntry{
		{
			ID: "e-multiline",
			Variants: map[model.LocaleID][]model.Run{
				"en": {{Text: &model.TextRun{Text: "Source one"}}},
				"nb": {{Text: &model.TextRun{Text: multiline}}},
			},
		},
		{
			ID: "e-bracket",
			Variants: map[model.LocaleID][]model.Run{
				"en": {{Text: &model.TextRun{Text: bracketed}}},
			},
		},
	}
	require.NoError(t, tm.BulkAddWithStream(ctx, entries, ""))

	got, err := tm.Entries(ctx)
	require.NoError(t, err)
	require.Len(t, got, 2)
	byID := map[string]*TMEntry{}
	for i := range got {
		byID[got[i].ID] = &got[i]
	}

	assert.Equal(t, multiline, byID["e-multiline"].VariantText("nb"),
		"bulk-imported variant text must round-trip verbatim, line structure intact")
	assert.Equal(t, bracketed, byID["e-bracket"].VariantText("en"),
		"plain text with a leading bracket must not be misread as coded runs")

	// Matching still works on the normalized key: a single-line query must
	// find the multi-line entry's source.
	matches, err := tm.LookupText(ctx, "Source one", "en", "nb", LookupOptions{})
	require.NoError(t, err)
	require.NotEmpty(t, matches, "normalized matching keys must be unaffected")
	assert.Equal(t, multiline, matches[0].Entry.VariantText("nb"))
}
