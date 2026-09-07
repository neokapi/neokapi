package markdown_test

import (
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestReferenceDefinitionSpellingRoundTrips is the #2462 reproducer. A
// definition was emitted from the parser's resolved label, destination and
// title, so CommonMark 4.7's line ending between any two of them came back
// folded onto one line. The bytes replay from source instead.
func TestReferenceDefinitionSpellingRoundTrips(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
	}{
		{"destination and title on their own lines", "See [a][R] here.\n\n[R]:\n  /x\n  'T'\n"},
		{"destination on its own line", "See [a][R] here.\n\n[R]:\n  /x\n"},
		{"title on its own line", "See [a][R] here.\n\n[R]: /x\n  'T'\n"},
		{"visible label, spread over lines", "See [R] here.\n\n[R]:\n  /x\n  'T'\n"},
		{"padded separator and title", "See [a][R] here.\n\n[R]:  /x  \"T\"\n"},
		{"angle destination with a space", "See [a][R] here.\n\n[R]: <x y> (T)\n"},
		{"escaped space in the destination", "See [a][R] here.\n\n[R]: /x\\ y\n"},
		{"visible label on one line", "See [R] here.\n\n[R]: /x 'T'\n"},
		{"collapsed reference", "See [R][] here.\n\n[R]: /x\n"},
		{"unused definition", "[R]: /x\n"},
		{"unused definition with a title", "[R]: /x 'T'\n"},
		{"parenthesised title on its own line", "See [a][R] here.\n\n[R]: /x\n  (T)\n"},
		{"tab between the label and the destination", "See [a][R] here.\n\n[R]:\t/x\n"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.input, roundtripWithSkeleton(t, tc.input), "skeleton path is not byte-exact")
		})
	}
}

// TestReferenceDefinitionKeepsWhatPrecedesIt covers #2482. Whatever sits
// between the line's start and the definition rides the skeleton: the 0-3
// spaces of indent CommonMark 4.7 allows, and a container's own marker. The
// reader used to emit the gap up to the line start and drop the rest, which
// cost a blockquote its marker and a list item both its marker and the
// definition. Okapi strips the indent on writeback; MarkdownCanonical folds
// that difference so parity still holds.
func TestReferenceDefinitionKeepsWhatPrecedesIt(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
	}{
		{"indented definition", "See [a][R] here.\n\n   [R]: /x\n"},
		{"definition in a blockquote", "> [d]: /docs\n"},
		{"definition in a list item", "- [d]: /docs\n"},
		{"definition in an ordered item", "1. [d]: /docs\n"},
		{"definition after an item's text", "- See [the docs][d] here.\n\n  [d]: /docs\n"},
		{"item then a second item", "- [d]: /docs\n- Second item.\n"},
		{"definition in a nested quote", ">> [d]: /docs\n"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.input, roundtripWithSkeleton(t, tc.input), "skeleton path is not byte-exact")
		})
	}
}

// TestReferenceDefinitionInAListItemKeepsResolving pins what the lost marker
// cost beyond the bytes: the definition an item's link resolves against was
// gone, so the link rendered as literal text.
func TestReferenceDefinitionInAListItemKeepsResolving(t *testing.T) {
	t.Parallel()
	src := "- See [the docs][d] here.\n\n  [d]: /docs\n"
	blocks := readBlocks(t, src)
	require.NotEmpty(t, blocks)
	assert.Equal(t, "See the docs here.", blocks[0].SourceText())
}

// TestReferenceDefinitionAtomsTranslate pins the run shape a spread definition
// still offers: the visible label and the used title are their own blocks, and
// a translated one is written back between the source's own bytes.
func TestReferenceDefinitionAtomsTranslate(t *testing.T) {
	t.Parallel()
	src := "See [R] here.\n\n[R]:\n  /x\n  'T'\n"

	blocks := readBlocks(t, src)
	var texts []string
	for _, b := range blocks {
		texts = append(texts, b.SourceText())
	}
	assert.Equal(t, []string{"See R here.", "R", "T"}, texts)

	translate := func(bs []*model.Block) {
		for _, b := range bs {
			if b.Type == "link-reference-title" {
				b.SetTargetRuns(model.LocaleGerman, []model.Run{{Text: &model.TextRun{Text: "U"}}})
			}
		}
	}
	assert.Equal(t, "See [R] here.\n\n[R]:\n  /x\n  'U'\n",
		roundtripTranslated(t, src, model.LocaleGerman, translate))
}
