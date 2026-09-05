package markdown_test

import (
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestLinkTitleSpellingRoundTrips is the #2432 reproducer. A link's or
// image's closing runs were rebuilt from the parser's resolved values as
// `](dest "title")`, so a single-quoted or parenthesised title came back
// double-quoted, the whitespace around a title collapsed, an escape inside
// one was lost, and one of two links to the same destination took the
// other's angle brackets. The closer now replays from source.
func TestLinkTitleSpellingRoundTrips(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
		text  string // source text of the first block
	}{
		{"single quotes", "[a](b 't')\n", "at"},
		{"parentheses", "[a](b (t))\n", "at"},
		{"double quotes", "[a](b \"t\")\n", "at"},
		{"image single quotes", "![a](b 't')\n", "at"},
		{"image parentheses", "![a](b (t))\n", "at"},
		{"angle destination", "[a](<b> 't')\n", "at"},
		{"angle destination with a space", "[a](<b c> \"t\")\n", "at"},
		{"padding around the title", "[a](b  't'  )\n", "at"},
		{"padding around a bare destination", "[a]( b )\n", "a"},
		{"title on the next line", "[a](b\n't')\n", "at"},
		{"escaped quote in the title", "[a](b 'it\\'s')\n", "ait\\'s"},
		{"escaped double quote in the title", "[a](b \"say \\\"hi\\\"\")\n", "asay \\\"hi\\\""},
		{"parentheses in the destination", "[a](b(c)d)\n", "a"},
		{"escaped parenthesis in the destination", "[a](b\\)c)\n", "a"},
		{"empty link text", "[](b 't')\n", "t"},
		{"image inside a link, both titled", "[![i](s 'st')](b 'lt')\n", "istlt"},
		{"two links to one destination", "[a](b) [c](<b>)\n", "a c"},
		{"two links to one destination, the other way", "[a](<b>) [c](b)\n", "a c"},
		{"same destination, different title delimiters", "[a](b 't') [c](b \"u\")\n", "at cu"},
		{"in a sentence", "See [the docs](/docs 'Documentation').\n", "See the docsDocumentation."},
		{"titled definition of a reference link", "[a][r]\n\n[r]: b 't'\n", "a"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			blocks := assertSkeletonByteExact(t, tc.input)
			assert.Equal(t, tc.text, blocks[0].SourceText())
		})
	}
}

// TestLinkTitleTranslatedKeepsDelimiters pins the run shape: the title is a
// text run between two paired codes that carry the source's own delimiters,
// so a translated title is written back inside them.
func TestLinkTitleTranslatedKeepsDelimiters(t *testing.T) {
	t.Parallel()
	translate := func(blocks []*model.Block) {
		require.Len(t, blocks, 1)
		var target []model.Run
		for _, r := range blocks[0].Source {
			if r.Text == nil {
				target = append(target, r)
				continue
			}
			text := map[string]string{"a": "x", "t": "u"}[r.Text.Text]
			require.NotEmpty(t, text, "unexpected text run %q", r.Text.Text)
			target = append(target, model.Run{Text: &model.TextRun{Text: text}})
		}
		blocks[0].SetTargetRuns(model.LocaleGerman, target)
	}
	assert.Equal(t, "[x](b 'u')\n", roundtripTranslated(t, "[a](b 't')\n", model.LocaleGerman, translate))
	assert.Equal(t, "![x](b (u))\n", roundtripTranslated(t, "![a](b (t))\n", model.LocaleGerman, translate))
	assert.Equal(t, "[x](<b>  \"u\"  )\n", roundtripTranslated(t, "[a](<b>  \"t\"  )\n", model.LocaleGerman, translate))
}
