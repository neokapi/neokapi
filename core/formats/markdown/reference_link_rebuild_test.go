package markdown_test

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/neokapi/neokapi/core/formats/markdown"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestReferenceLinkRebuildsAsReference is the #2635 reproducer. The rebuild
// path (no skeleton store: a cross-format export, or a document with no
// skeleton) spelled a reference link inline with the destination and title its
// definition resolved, and wrote the definition's translatable title back as a
// bare paragraph, so one block became two on re-read. The link is written as
// the reference it was and the definition as a block of its own.
func TestReferenceLinkRebuildsAsReference(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
	}{
		{"empty text, title in single quotes", "[][R]\n\n[R]:0 '0'"},
		{"full reference", "See [a][R] here.\n\n[R]: /x\n"},
		{"full reference with a title", "See [a][R] here.\n\n[R]: /x 'T'\n"},
		{"shortcut reference with a title", "See [R] here.\n\n[R]: /x 'T'\n"},
		{"collapsed reference", "See [R][] here.\n\n[R]: /x\n"},
		{"definition spread over lines", "[a][R] x\n\n[R]:\n /y\n 'T'\n"},
		{"image reference", "See ![a][R] here.\n\n[R]: /x 'T'\n"},
		{"two references to one definition", "[a][R] and [b][R].\n\n[R]: /x \"T\"\n"},
		{"definition before its reference", "[R]: /x 'T'\n\nSee [a][R] here.\n"},
		{"definition in a list item", "- See [the docs][d] here.\n\n  [d]: /docs\n"},
		{"definition in a blockquote", "> See [a][R] here.\n>\n> [R]: /x 'T'\n"},
		{"label wrapping inside a quote", "> See [a][\n> R] here.\n\n[R]: /x\n"},
		{"title with a double quote", "See [a][R] here.\n\n[R]: /x 'say \"hi\"'\n"},
		{"unused definition", "Text.\n\n[R]: /x 'T'\n"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			out := roundtrip(t, tc.input)
			assert.Equal(t, blockShapes(readBlocks(t, tc.input)), blockShapes(readBlocks(t, out)),
				"the rebuilt output %q must re-read to the same blocks", out)
			assert.Equal(t, out, roundtrip(t, out), "the rebuild must be a fixed point")
		})
	}
}

// blockShapes names each block by type and source text, which is what a
// rebuild must keep.
func blockShapes(blocks []*model.Block) []string {
	shapes := make([]string, 0, len(blocks))
	for _, b := range blocks {
		shapes = append(shapes, fmt.Sprintf("%s:%q", b.Type, b.SourceText()))
	}
	return shapes
}

// TestReferenceDefinitionRebuildsTranslatedAtoms pins that the rebuild path
// writes a translated label and title into the definition, beside the
// reference that shows the label.
func TestReferenceDefinitionRebuildsTranslatedAtoms(t *testing.T) {
	t.Parallel()
	src := "See [R] here.\n\n[R]: /x 'T'\n"
	parts := readParts(t, src)
	for _, b := range testutil.FilterBlocks(parts) {
		switch b.Type {
		case "link-reference-label":
			b.SetTargetText(model.LocaleGerman, "S")
		case "link-reference-title":
			b.SetTargetText(model.LocaleGerman, "U")
		default:
			b.SetTargetText(model.LocaleGerman, "Siehe [S] hier.")
		}
	}

	var buf bytes.Buffer
	w := markdown.NewWriter()
	require.NoError(t, w.SetOutputWriter(&buf))
	w.SetLocale(model.LocaleGerman)
	require.NoError(t, w.Write(t.Context(), testutil.PartsToChannel(parts)))
	w.Close()

	assert.Equal(t, "Siehe [S] hier.\n\n[S]: /x \"U\"\n", buf.String())
}
