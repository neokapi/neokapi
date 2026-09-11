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
// The fixture is an escaped backtick that touches a code span's opening fence:
// the walk that recovers the fence from source counts the escaped tick as part
// of it, so the rebuilt opener is one tick longer than the source spelled
// (#2618). The task list, the hard break, the single-quoted title, the
// underscore emphasis, the quoted link whose title wrapped, the code span whose
// fence sat on the next line, the unbalanced strikethrough and the link
// destination with an unbalanced parenthesis all sat here and reconstruct now,
// and the note they carried holds for their replacement: these tests assert the
// reporting, so the day this shape round-trips they will fail and the fixture
// should become whatever still diverges.
//
// A single-block span is the deliberate choice. Quarantine salvages a span by
// isolating the blocks that failed, so a fixture with neighbours exercises that
// path instead — which is what TestQuarantineKeepsTheOtherBlocks is for.
const divergingSrc = "A tick \\``code` in a sentence.\n"

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
	blocks, diags := readAll(t, divergingSrc)

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

	assert.NotEmpty(t, blocks, "a plain link reconstructs")
	assert.Empty(t, diags, "nothing was lost, so there is nothing to report")
}

// The diagnostic exists to end a bisect, so it has to name a position and show
// the bytes that diverged.
func TestDiagnosticLocatesTheDivergence(t *testing.T) {
	_, diags := readAll(t, "First paragraph.\n\n"+divergingSrc)
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
		// Both of these used to go opaque: a code span wrapping a line took
		// its continuation prefix with it, because the span's runs were built
		// by joining the parser's child segments and the prefix sits in the
		// source BETWEEN them. Taking the content verbatim keeps it.
		{name: "code span wrapping a blockquote continuation",
			src: "> A quoted line with `code that\n> wraps` across the break.\n"},
		{name: "code span wrapping a list continuation",
			src: "- An item with `code that\n  wraps` across the break.\n"},
		// An entity used to be left in the text as words. In a link it cost
		// the region its blocks; in prose it was offered to the translator and
		// came back as `&àḿþ;`. It is a placeholder now, so it survives both.
		{name: "entity in link text", src: "See [Ship gates &amp; CI](/x).\n"},
		{name: "entity in bold", src: "Ship gates **&amp; CI** here.\n"},
		{name: "numeric entity", src: "Ship gates &#38; CI here.\n"},
		// A code span whose closing fence sat on the next line lost the break:
		// CommonMark 6.1 turns the interior line ending into a space, so the
		// parser's content is shorter than the source spelled and the fences
		// carry those bytes now.
		{name: "code span closing on the next line", src: "See ` code\n` here.\n"},
		{name: "code span closing in a quote", src: "> See ` code\n> ` here.\n"},
		{name: "code span closing in a list item", src: "- See ` code\n  ` here.\n"},
		{name: "code span with a padded fence closing on the next line", src: "See `` ` a\n `` here.\n"},
		{name: "code span with a trailing space before the break", src: "See `code \n` here.\n"},
		// An unbalanced strikethrough was spelled from the parser's delimiter
		// length, which claimed a tilde its neighbour carried.
		{name: "strikethrough with a shorter closing run", src: "An unbalanced ~~strike~ here.\n"},
		{name: "strikethrough with a shorter opening run", src: "An unbalanced ~strike~~ here.\n"},
		{name: "single-tilde strikethrough", src: "A ~struck~ word here.\n"},
		{name: "strikethrough around emphasis", src: "A ~*struck*~ word here.\n"},
		// The controls: the same constructs without the wrapping code span.
		// If these ever go opaque a fix above has over-reached.
		{name: "plain wrapped blockquote", src: "> A quoted line that wraps onto\n> a second line here.\n"},
		{name: "plain wrapped list item", src: "- An item that wraps onto\n  a second line here.\n"},
		{name: "entity outside a link", src: "Ship gates &amp; CI here.\n"},
		// A hard break used to lose the spaces or backslash that spelled it,
		// which cost the paragraph its block; the spelling rides a placeholder
		// now, and a paragraph mixing hard and soft breaks keeps its marker.
		{name: "hard break with spaces", src: "One line  \nnext line.\n"},
		{name: "hard break with a backslash", src: "One line\\\nnext line.\n"},
		{name: "hard break in a blockquote", src: "> One line  \n> next line.\n"},
		{name: "hard and soft breaks in a blockquote", src: "> One line  \n> next line\n> third line.\n"},
		// A GFM task list used to cost its whole file: the checkbox carries no
		// segment of its own, so the rebuild dropped it, and eight of them took
		// a 6.4KB release note out of the translation entirely.
		{name: "task list", src: "- [ ] `config.go` with a trailing clause.\n"},
		{name: "task list checked", src: "- [x] Tag is annotated.\n"},
		{name: "task list upper case", src: "- [X] Capitalised marker.\n"},
		// An ATX heading's closing sequence used to be written on a line of
		// its own, which re-read as a second, empty heading.
		{name: "ATX heading with a closing sequence", src: "# Title #\n\nBody.\n"},
		{name: "ATX heading with a closing sequence and trailing whitespace", src: "## Title ##   \n\nBody.\n"},
		// A soft break after a trailing space or tab, or a CRLF soft break,
		// used to collapse to one space, which cost a Windows-authored page
		// every wrapped paragraph.
		{name: "soft break after a trailing space", src: "One line \nnext line.\n"},
		{name: "soft break after a trailing tab", src: "One line\t\nnext line.\n"},
		{name: "CRLF soft break", src: "One line\r\nnext line.\r\n"},
		{name: "CRLF soft break in a blockquote", src: "> One line\r\n> next line.\r\n"},
		{name: "CRLF soft break in a list item", src: "- One line\r\n  next line.\r\n"},
		// An autolink with nothing before it used to lose its angle brackets.
		// MDX reads a line-initial `<` as JSX, so the first-in-block shape is
		// covered by a list item rather than a paragraph.
		{name: "list item that is only an autolink", src: "- <https://example.com>\n- Second item.\n"},
		{name: "autolink first in an emphasis", src: "*<https://example.com>* here.\n"},
		{name: "autolink after a link", src: "[site](https://example.com) <https://example.com> here.\n"},
		// A link's closer used to be rebuilt as `](dest "title")` whatever the
		// source spelled.
		{name: "single-quoted link title", src: "See [the docs](/docs 'Documentation').\n"},
		{name: "parenthesised image title", src: "![alt](image.png (An image))\n"},
		{name: "title on its own line", src: "[a](https://example.com\n'Title').\n"},
		{name: "two links to one destination spelled two ways", src: "[a](https://example.com) and [b](<https://example.com>).\n"},
		// An underscore emphasis whose first child is not a text node used to
		// come back with asterisks, and `_*a*_` as `**a**`, which re-reads as
		// strong and changes the block outright.
		{name: "underscore emphasis around a link", src: "See _[the docs](/docs)_ here.\n"},
		{name: "underscore emphasis around a code span", src: "See _`config.go`_ here.\n"},
		{name: "underscore emphasis around an autolink", src: "See _<https://example.com>_ here.\n"},
		{name: "underscore strong around an image", src: "See __![alt](image.png)__ here.\n"},
		{name: "underscore emphasis around an emphasis", src: "See _*nested*_ here.\n"},
		// A link or image whose closing markup wrapped onto the next line of a
		// container used to lose the source's spelling: the closer scan stopped
		// on the `> ` the parser had stripped, and the closer was rebuilt from
		// the resolved values with double quotes.
		{name: "quoted link whose title wraps", src: "> See [the docs](/docs\n> 'Documentation') here.\n"},
		{name: "quoted image whose title wraps", src: "> See ![alt](/img\n> 'Caption') here.\n"},
		{name: "twice-quoted link whose title wraps", src: ">> See [the docs](/docs\n>> 'Documentation') here.\n"},
		{name: "quoted reference link whose label wraps", src: "> See [the docs][\n> d] here.\n\n[ d]: /docs\n"},
		{name: "link in a list item whose title wraps", src: "- See [the docs](/docs\n  'Documentation') here.\n"},
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

// A block that cannot be rebuilt costs itself, not the page. Before this, one
// diverging paragraph took every heading and paragraph around it out of the
// translation, and the only symptom was a page still in its source language.
func TestQuarantineKeepsTheOtherBlocks(t *testing.T) {
	src := "First paragraph here.\n\n" + divergingSrc + "\nThird paragraph here.\n"
	blocks, diags := readAll(t, src)

	var texts []string
	for _, b := range blocks {
		texts = append(texts, b.SourceText())
	}
	assert.Equal(t, []string{"First paragraph here.", "Third paragraph here."}, texts,
		"the neighbours of a divergent block keep their translatability")

	require.NotEmpty(t, diags, "a block staying in the source language is invisible; say it")
	assert.Equal(t, "structure.markdown-block-opaque", diags[0].Category)
	assert.Contains(t, diags[0].Message, "1 of 3 blocks",
		"the message must say how much was lost, not merely that something was")
	assert.Contains(t, diags[0].Message, "the rest still translate")
}

// Salvage is not a licence to rewrite bytes: whatever the reader decides about
// translatability, an untranslated read->write stays byte-identical.
func TestQuarantinedSpanStillRoundTrips(t *testing.T) {
	src := "First paragraph here.\n\n" + divergingSrc + "\nThird paragraph here.\n"
	assert.Equal(t, src, string(roundTrip(t, []byte(src))))
}
