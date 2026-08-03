package reconcile_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/formats/markdown"
	"github.com/neokapi/neokapi/core/formats/plaintext"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/reconcile"
)

func readBlocks(t *testing.T, r format.DataFormatReader, content string) []*model.Block {
	t.Helper()
	ctx := context.Background()
	require.NoError(t, r.Open(ctx, testutil.RawDocFromString(content, model.LocaleEnglish)))

	var out []*model.Block
	for pr := range r.Read(ctx) {
		require.NoError(t, pr.Error)
		if pr.Part == nil || pr.Part.Type != model.PartBlock {
			continue
		}
		if b, ok := pr.Part.Resource.(*model.Block); ok && b != nil && b.SourceText() != "" {
			out = append(out, b)
		}
	}
	return out
}

// keysByText reconciles a read and indexes the outcome by the words of each
// block, which is how these tests refer to blocks across revisions.
func keysByText(blocks []*model.Block, prior []reconcile.Unit) map[string]reconcile.Result {
	out := map[string]reconcile.Result{}
	for _, r := range reconcile.Blocks(blocks, prior) {
		out[r.Block.SourceText()] = r
	}
	return out
}

func unitsOf(blocks []*model.Block, res map[string]reconcile.Result) []reconcile.Unit {
	var out []reconcile.Unit
	for _, b := range blocks {
		id := model.ComputeIdentity(b)
		out = append(out, reconcile.Unit{
			Key:         res[b.SourceText()].Key,
			ContentHash: id.ContentHash,
			ContextHash: id.ContextHash,
		})
	}
	return out
}

// The end-to-end claim, against real readers rather than hand-built blocks.
//
// Prose readers name blocks by position — markdown "para3", plaintext "line5" —
// so deleting a paragraph renames every block below it. That is fine, and is
// left alone: the name is a context signal, not an identity. What must hold is
// that identity survives the rename anyway.
func TestReaders_IdentitySurvivesAnUnrelatedDeletion(t *testing.T) {
	for _, tc := range []struct {
		name   string
		reader func() format.DataFormatReader
		v1, v2 string
	}{
		{"markdown", func() format.DataFormatReader { return markdown.NewReader() },
			"Alpha\n\nBravo\n\nCharlie\n", "Alpha\n\nCharlie\n"},
		{"plaintext", func() format.DataFormatReader { return plaintext.NewReader() },
			"Alpha\n\nBravo\n\nCharlie\n", "Alpha\n\nCharlie\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			v1Blocks := readBlocks(t, tc.reader(), tc.v1)
			v1 := keysByText(v1Blocks, nil)
			prior := unitsOf(v1Blocks, v1)

			// The reader's own name for Charlie shifts. That is the hazard.
			v2Blocks := readBlocks(t, tc.reader(), tc.v2)
			var before, after string
			for _, b := range v1Blocks {
				if b.SourceText() == "Charlie" {
					before = b.Name
				}
			}
			for _, b := range v2Blocks {
				if b.SourceText() == "Charlie" {
					after = b.Name
				}
			}
			require.NotEqual(t, before, after, "precondition: the reader renames Charlie")

			v2 := keysByText(v2Blocks, prior)

			assert.Equal(t, v1["Charlie"].Key, v2["Charlie"].Key,
				"Charlie's text never changed, so its decisions and translation must follow it")
			assert.Equal(t, reconcile.Moved, v2["Charlie"].Kind,
				"and the move is reported, not hidden")
			assert.Equal(t, reconcile.Unchanged, v2["Alpha"].Kind)
		})
	}
}

// Editing a paragraph keeps it the same unit, with a translation now stale.
// A content-derived name would report this as a deletion plus an addition.
func TestReaders_EditKeepsTheUnit(t *testing.T) {
	blocks := readBlocks(t, markdown.NewReader(), "Alpha\n\nBravo\n")
	v1 := keysByText(blocks, nil)
	prior := unitsOf(blocks, v1)

	v2 := keysByText(readBlocks(t, markdown.NewReader(), "Alpha\n\nBravo, revised\n"), prior)

	assert.Equal(t, v1["Bravo"].Key, v2["Bravo, revised"].Key,
		"the paragraph was rewritten, not replaced")
	assert.Equal(t, reconcile.Edited, v2["Bravo, revised"].Kind)
}

// The walk that motivates keeping units for blocks no longer present: gone in
// one revision, back in a later one, and its earlier translation is reusable.
func TestReaders_SurvivesRemovalAndReturn(t *testing.T) {
	v1Blocks := readBlocks(t, markdown.NewReader(), "Alpha\n\nBravo\n\nCharlie\n")
	v1 := keysByText(v1Blocks, nil)
	prior := unitsOf(v1Blocks, v1)

	// v2 drops Bravo; priors are carried forward untouched.
	keysByText(readBlocks(t, markdown.NewReader(), "Alpha\n\nCharlie\n"), prior)

	// v3 brings Bravo back lower down, after text that did not exist in v1.
	v3 := keysByText(readBlocks(t, markdown.NewReader(), "Alpha\n\nDelta\n\nBravo\n"), prior)

	assert.Equal(t, v1["Bravo"].Key, v3["Bravo"].Key, "it returns to its own history")
	assert.Equal(t, reconcile.New, v3["Delta"].Kind)
}
