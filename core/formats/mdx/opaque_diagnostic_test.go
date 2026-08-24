package mdx

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A span that cannot be rebuilt byte-for-byte is emitted opaque, which costs it
// every translatable block it had. That is the right trade and it must not be a
// quiet one: a whole document can go opaque for one construct, and the only
// symptom is a page still in its source language.
//
// `See [Ship gates &amp; CI](/x).` is the shape that does it. The same entity
// outside a link reconstructs fine.

func readAll(t *testing.T, src string) ([]*model.Block, []format.Diagnostic) {
	t.Helper()
	r := NewReader()
	store, err := format.NewSkeletonStore()
	require.NoError(t, err)
	defer func() { _ = store.Close() }()
	r.SetSkeletonStore(store)

	doc := &model.RawDocument{
		URI:          "case.mdx",
		Reader:       io.NopCloser(bytes.NewReader([]byte(src))),
		SourceLocale: model.LocaleID("en"),
	}
	ctx := context.Background()
	require.NoError(t, r.Open(ctx, doc))

	var blocks []*model.Block
	for pr := range r.Read(ctx) {
		require.NoError(t, pr.Error)
		if pr.Part.Type == model.PartBlock {
			if b, ok := pr.Part.Resource.(*model.Block); ok && b.Translatable {
				blocks = append(blocks, b)
			}
		}
	}
	return blocks, r.Diagnostics()
}

func TestOpaqueFallbackIsReported(t *testing.T) {
	blocks, diags := readAll(t, "See [Ship gates &amp; CI](/x).\n")

	assert.Empty(t, blocks, "this is the shape that loses its blocks")
	require.NotEmpty(t, diags,
		"the span went opaque and said nothing; that silence is the defect")

	d := diags[0]
	assert.Equal(t, "structure.markdown-span-opaque", d.Category)
	assert.Equal(t, format.SeverityMajor, d.Severity)
	assert.Contains(t, d.Message, "source language",
		"the message must say what the reader will do, not just that a check failed")
	assert.NotEmpty(t, d.Snippet, "a diagnostic with no excerpt is another bisect")
}

func TestFaithfulSpanIsSilent(t *testing.T) {
	blocks, diags := readAll(t, "See [Ship gates & CI](/x).\n")

	assert.NotEmpty(t, blocks, "a plain ampersand reconstructs")
	assert.Empty(t, diags, "nothing was lost, so there is nothing to report")
}

// The diagnostic exists to end a bisect, so it has to name a position and show
// the bytes that diverged.
func TestDiagnosticLocatesTheDivergence(t *testing.T) {
	_, diags := readAll(t, "First paragraph.\n\nSee [Ship gates &amp; CI](/x).\n")
	require.NotEmpty(t, diags)

	d := diags[0]
	assert.Positive(t, d.Line, "a diagnostic without a line is a bisect with extra steps")
	assert.Positive(t, d.ByteOffset)
	assert.True(t,
		strings.Contains(d.Message, "source has") || strings.Contains(d.Message, "skeleton references"),
		"the message must show the divergence, got: %s", d.Message)
}

// The two shapes that cost this repo's own docs their translations. Both were
// found by reading the diagnostic above; three rounds of guessing had missed
// them, because the ingredient is what wraps across the newline, not the
// blockquote or the list.
func TestKnownRoundTripDivergences(t *testing.T) {
	tests := []struct {
		name   string
		src    string
		opaque bool
		want   string
	}{
		{
			name:   "code span wrapping a blockquote continuation",
			src:    "> A quoted line with `code that\n> wraps` across the break.\n",
			opaque: true,
			want:   "the > prefix is dropped on the continuation line",
		},
		{
			name:   "code span wrapping a list continuation",
			src:    "- An item with `code that\n  wraps` across the break.\n",
			opaque: true,
			want:   "the two-space indent is dropped on the continuation line",
		},
		{
			name:   "entity in link text",
			src:    "See [Ship gates &amp; CI](/x).\n",
			opaque: true,
			want:   "&amp; is decoded on read and not restored on render",
		},
		// The controls: same constructs without the wrapping code span, and
		// the same entity outside a link. If these ever go opaque the fix
		// above has over-reached.
		{name: "plain wrapped blockquote", src: "> A quoted line that wraps onto\n> a second line here.\n"},
		{name: "plain wrapped list item", src: "- An item that wraps onto\n  a second line here.\n"},
		{name: "entity outside a link", src: "Ship gates &amp; CI here.\n"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			blocks, diags := readAll(t, tc.src)
			if tc.opaque {
				assert.Empty(t, blocks, "known divergence: %s", tc.want)
				assert.NotEmpty(t, diags, "and it must be reported, not taken in silence")
				return
			}
			assert.NotEmpty(t, blocks, "this shape reconstructs and must stay translatable")
			assert.Empty(t, diags)
		})
	}
}
