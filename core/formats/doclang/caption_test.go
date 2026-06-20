package doclang_test

import (
	"testing"

	doclangfmt "github.com/neokapi/neokapi/core/formats/doclang"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const captionDoc = `<?xml version="1.0" encoding="UTF-8"?>
<doclang xmlns="https://www.doclang.ai/ns/v0" version="0.6">
  <table>
    <caption>Table 1: Sales by region</caption>
    <ched/>Region<ched/>Revenue<nl/>
    <fcel/>North<fcel/>1200<nl/>
  </table>
  <picture>
    <caption>Figure 2: Revenue trend</caption>
  </picture>
</doclang>`

func readCaptionBlocks(t *testing.T, configure func(*doclangfmt.Config)) []*model.Block {
	t.Helper()
	ctx := t.Context()
	r := doclangfmt.NewReader()
	if configure != nil {
		if c, ok := r.Config().(*doclangfmt.Config); ok {
			configure(c)
		}
	}
	require.NoError(t, r.Open(ctx, testutil.RawDocFromString(captionDoc, model.LocaleEnglish)))
	defer r.Close()
	return testutil.CollectBlocks(t, r.Read(ctx))
}

// By default, table and picture <caption> text surfaces as non-translatable
// RoleCaption content blocks — visible to ingestion, skipped by MT.
func TestCaptionsSurfacedAsContent(t *testing.T) {
	var captions []*model.Block
	for _, b := range readCaptionBlocks(t, nil) {
		if b.SemanticRole() == model.RoleCaption {
			captions = append(captions, b)
		}
	}
	require.Len(t, captions, 2, "table + picture captions both surface")
	texts := testutil.BlockTexts(captions)
	assert.Contains(t, texts, "Table 1: Sales by region")
	assert.Contains(t, texts, "Figure 2: Revenue trend")
	for _, c := range captions {
		assert.False(t, c.Translatable, "captions surface as non-translatable content")
	}
}

// With ExtractNonTranslatableContent disabled (the Okapi-faithful config), no
// caption blocks are produced — captions stay in skeleton.
func TestCaptionsDisabledStaySkeleton(t *testing.T) {
	for _, b := range readCaptionBlocks(t, func(c *doclangfmt.Config) {
		c.SetExtractNonTranslatableContent(false)
	}) {
		assert.NotEqual(t, model.RoleCaption, b.SemanticRole(), "no caption blocks when surfacing is off")
	}
}
