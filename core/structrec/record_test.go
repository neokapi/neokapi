package structrec

import (
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
)

// TestFromBlock_RendersPlaceholders locks decision C: the record Text renders
// inline codes as <x id="…"/> placeholders so the read leg is symmetric with
// the write-back leg, and decision D: ContentHash stays the canonical identity
// over the block's PLAIN source text (not the placeholder rendering), so it
// matches the hash the sync engine and stores use.
func TestFromBlock_RendersPlaceholders(t *testing.T) {
	b := &model.Block{
		ID:           "p1",
		Translatable: true,
		Source: []model.Run{
			{Text: &model.TextRun{Text: "Click "}},
			{PcOpen: &model.PcOpenRun{ID: "1"}},
			{Text: &model.TextRun{Text: "here"}},
			{PcClose: &model.PcCloseRun{ID: "1"}},
			{Text: &model.TextRun{Text: " now"}},
		},
	}

	rec := FromBlock(1, b, b.Source)

	// Text shows the codes as placeholders, not dropped.
	assert.Equal(t, `Click <x id="1"/>here<x id="/1"/> now`, rec.Text)
	// ContentHash is over the plain source text ("Click here now"), NOT the Text.
	assert.Equal(t, model.ComputeContentHash(b.SourceText()), rec.ContentHash)
	assert.NotEqual(t, model.ComputeContentHash(rec.Text), rec.ContentHash,
		"hash must be canonical (plain source), not a hash of the placeholder rendering")
}
