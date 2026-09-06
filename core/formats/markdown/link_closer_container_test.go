package markdown_test

import (
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestLinkCloserWrapsInsideAContainer is the #2461 reproducer. CommonMark 6.6
// allows one line ending in the whitespace around a link's destination and
// title. Inside a blockquote or a list item the bytes after that line ending
// carry the continuation prefix the parser stripped, the closer scan stopped on
// it, and the closer was rebuilt from the parser's resolved values: a
// single-quoted title came back double-quoted and a reference label lost its
// marker. The scan skips the prefix, so the replayed bytes keep it.
func TestLinkCloserWrapsInsideAContainer(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
		text  string // source text of the first block
	}{
		{"quoted link", "> [a](/x\n> 'T') here.\n", "aT here."},
		{"quoted image", "> ![a](/x\n> 'T') here.\n", "aT here."},
		{"twice-quoted link", ">> [a](/x\n>> 'T') here.\n", "aT here."},
		{"quoted link, destination on its own line", "> [a](\n> /x\n> 'T') here.\n", "aT here."},
		{"quoted link, no space after the marker", ">[a](/x\n>'T') here.\n", "aT here."},
		{"link in a list item", "- [a](/x\n  'T') here.\n", "aT here."},
		{"link in a quoted list item", "> - [a](/x\n>   'T') here.\n", "aT here."},
		{"quoted link, double quotes", "> [a](/x\n> \"T\") here.\n", "aT here."},
		{"quoted link, parenthesised title", "> [a](/x\n> (T)) here.\n", "aT here."},
		{"quoted link, no title", "> [a](/x\n> ) here.\n", "a here."},
		{"top level, unchanged", "[a](/x\n'T') here.\n", "aT here."},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			blocks := assertSkeletonByteExact(t, tc.input)
			assert.Equal(t, tc.text, blocks[0].SourceText())
		})
	}
}

// TestReferenceCloserWrapsInsideAContainer covers the reference form, whose
// label the parser resolves with the container's prefix already stripped, so a
// rebuild from that value wrote "][\nb]" where the source spelled "][\n> b]".
func TestReferenceCloserWrapsInsideAContainer(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
	}{
		{"quoted full reference", "> [a][\n> b] here.\n\n[ b]: /y\n"},
		{"quoted full reference image", "> ![a][\n> b] here.\n\n[ b]: /y\n"},
		{"twice-quoted full reference", ">> [a][\n>> b] here.\n\n[ b]: /y\n"},
		{"full reference in a list item", "- [a][\n  b] here.\n\n[ b]: /y\n"},
		{"collapsed reference", "> [a][] here.\n\n[a]: /y\n"},
		{"shortcut reference", "> [a] here.\n\n[a]: /y\n"},
		{"full reference, one line", "> [a][b] here.\n\n[b]: /y\n"},
		{"label with an upper-case spelling", "[a][B] here.\n\n[b]: /y\n"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assertSkeletonByteExact(t, tc.input)
		})
	}
}

// TestWrappedLinkTitleTranslatesInsideAContainer pins the run shape: the title
// is a text run between paired codes carrying the source's own bytes, the
// continuation prefix among them, so a translated title is written back inside
// the wrap the source spelled.
func TestWrappedLinkTitleTranslatesInsideAContainer(t *testing.T) {
	t.Parallel()
	translate := func(blocks []*model.Block) {
		require.Len(t, blocks, 1)
		var target []model.Run
		for _, r := range blocks[0].Source {
			if r.Text == nil {
				target = append(target, r)
				continue
			}
			text := map[string]string{"a": "x", "T": "U", " here.": " da."}[r.Text.Text]
			require.NotEmpty(t, text, "unexpected text run %q", r.Text.Text)
			target = append(target, model.Run{Text: &model.TextRun{Text: text}})
		}
		blocks[0].SetTargetRuns(model.LocaleGerman, target)
	}
	assert.Equal(t, "> [x](/x\n> 'U') da.\n",
		roundtripTranslated(t, "> [a](/x\n> 'T') here.\n", model.LocaleGerman, translate))
}
