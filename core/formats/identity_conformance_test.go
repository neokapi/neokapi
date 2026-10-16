package formats_test

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/convergence"
	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/formats/html"
	"github.com/neokapi/neokapi/core/formats/json"
	"github.com/neokapi/neokapi/core/formats/markdown"
	"github.com/neokapi/neokapi/core/formats/mdx"
	"github.com/neokapi/neokapi/core/formats/plaintext"
	"github.com/neokapi/neokapi/core/formats/resx"
	"github.com/neokapi/neokapi/core/formats/xml"
	"github.com/neokapi/neokapi/core/formats/yaml"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
)

// Block identity has to survive an edit to the file the block lives in.
//
// Everything downstream matches on it: a review decision is recorded against a
// unit (AD-033), the content memory recycles against it, and an iterative
// workflow re-reads the same file after every change. If identity moves when an
// unrelated block is deleted, a decision made about one string silently becomes
// a decision about another, and recycling misses work that has not changed.
//
// AD-003 already names the hazard — format readers "assign IDs from the source
// format (XLIFF tu1, tu2, etc.), but these are not unique across files" — and
// pushes reconciliation to the persistence layer. This test asks the narrower
// question that layer depends on: within ONE file, across an edit, does a block
// that did not change keep the identity the rest of the system matches it by?
//
// The identity under test is convergence.BlockKey (Name when the reader sets
// one, the reader's ID otherwise), because that is what host/convergereport,
// host/coverage and the push's change detection actually key on.
func TestBlockIdentity_SurvivesAnUnrelatedDeletion(t *testing.T) {
	cases := []struct {
		name   string
		reader func() format.DataFormatReader
		v1, v2 string
		// stable records the measured state. A format is stable when its name
		// comes from structure the author controls (a key path, a heading
		// trail, a container element), so an unrelated edit cannot move it.
		// What still numbers positionally is pinned by
		// TestBlockIdentity_ProseFormatsNamePositionally.
		stable bool
	}{
		{
			name:   "json",
			stable: true,
			reader: func() format.DataFormatReader { return json.NewReader() },
			v1:     `{"a":"Alpha","b":"Bravo","c":"Charlie"}`,
			v2:     `{"a":"Alpha","c":"Charlie"}`,
		},
		{
			name:   "yaml",
			stable: true,
			reader: func() format.DataFormatReader { return yaml.NewReader() },
			v1:     "a: Alpha\nb: Bravo\nc: Charlie\n",
			v2:     "a: Alpha\nc: Charlie\n",
		},
		{
			name:   "resx",
			stable: true,
			reader: func() format.DataFormatReader { return resx.NewReader() },
			v1: `<?xml version="1.0" encoding="utf-8"?>
<root>
  <data name="a" xml:space="preserve"><value>Alpha</value></data>
  <data name="b" xml:space="preserve"><value>Bravo</value></data>
  <data name="c" xml:space="preserve"><value>Charlie</value></data>
</root>
`,
			v2: `<?xml version="1.0" encoding="utf-8"?>
<root>
  <data name="a" xml:space="preserve"><value>Alpha</value></data>
  <data name="c" xml:space="preserve"><value>Charlie</value></data>
</root>
`,
		},
		{
			// Markdown names blocks by the heading trail they sit under plus
			// their kind, so the deletion below — a paragraph in ANOTHER
			// section — cannot re-address Charlie. The ordinal that separates
			// two paragraphs of ONE section is all that still moves; see
			// markdown.TestMarkdownNames_ShiftWithinTheSameSection for that
			// boundary, which is why the edit here crosses a heading.
			name:   "markdown",
			stable: true,
			reader: func() format.DataFormatReader { return markdown.NewReader() },
			v1:     "## Install\n\nAlpha\n\nBravo\n\n## Configure\n\nCharlie\n",
			v2:     "## Install\n\nAlpha\n\n## Configure\n\nCharlie\n",
		},
		{
			// MDX reads its markdown spans through the markdown reader with one
			// shared naming state, so the heading trail carries across the JSX
			// element between the sections.
			name:   "mdx",
			stable: true,
			reader: func() format.DataFormatReader { return mdx.NewReader() },
			v1:     "## Install\n\nAlpha\n\nBravo\n\n<Note>Aside.</Note>\n\n## Configure\n\nCharlie\n",
			v2:     "## Install\n\nAlpha\n\n<Note>Aside.</Note>\n\n## Configure\n\nCharlie\n",
		},
		{
			// HTML names a block by its element path, with the sibling ordinal
			// counted on the PARENT. A deletion inside one <section> cannot
			// re-address the paragraphs of the next. Siblings of one parent are
			// the positional case; see the test below.
			name:   "html",
			stable: true,
			reader: func() format.DataFormatReader { return html.NewReader() },
			v1:     "<section><h2>Install</h2><p>Alpha</p><p>Bravo</p></section><section><h2>Configure</h2><p>Charlie</p></section>",
			v2:     "<section><h2>Install</h2><p>Alpha</p></section><section><h2>Configure</h2><p>Charlie</p></section>",
		},
		{
			// XML is the same shape: the ordinal lives on the parent element.
			name:   "xml",
			stable: true,
			reader: func() format.DataFormatReader { return xml.NewReader() },
			v1:     "<doc><install><p>Alpha</p><p>Bravo</p></install><configure><p>Charlie</p></configure></doc>",
			v2:     "<doc><install><p>Alpha</p></install><configure><p>Charlie</p></configure></doc>",
		},
		{
			name:   "plaintext",
			reader: func() format.DataFormatReader { return plaintext.NewReader() },
			v1:     "Alpha\n\nBravo\n\nCharlie\n",
			v2:     "Alpha\n\nCharlie\n",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if !tc.stable {
				t.Skip("known gap, see TestBlockIdentity_ProseFormatsNamePositionally")
			}
			before := identitiesByText(t, tc.reader(), tc.v1)
			after := identitiesByText(t, tc.reader(), tc.v2)

			require.Contains(t, before, "Alpha")
			require.Contains(t, after, "Alpha")
			require.Contains(t, before, "Charlie", "Charlie must be read from v1")
			require.Contains(t, after, "Charlie", "Charlie survives the edit — only Bravo was deleted")

			// Alpha precedes the deletion, so a positional scheme gets it right
			// by luck. It is here to prove the test is not simply broken.
			require.Equal(t, before["Alpha"], after["Alpha"],
				"a block before the deletion must keep its identity")

			// Charlie follows the deletion. This is the one that matters: its
			// text did not change, so nothing about it was re-decided.
			require.Equal(t, before["Charlie"], after["Charlie"],
				"a block AFTER an unrelated deletion must keep its identity — "+
					"otherwise a decision recorded about Charlie now names whatever "+
					"took its slot, and content memory stops matching text that never changed")
		})
	}
}

// identitiesByText reads content and maps each block's source text to the
// identity the rest of the system matches it by.
func identitiesByText(t *testing.T, r format.DataFormatReader, content string) map[string]string {
	t.Helper()

	out := map[string]string{}
	for _, b := range readIdentityBlocks(t, r, content) {
		out[b.SourceText()] = convergence.BlockKey(b)
	}
	return out
}

// readIdentityBlocks reads content and returns its blocks with text, in order.
func readIdentityBlocks(t *testing.T, r format.DataFormatReader, content string) []*model.Block {
	t.Helper()

	ctx := context.Background()
	require.NoError(t, r.Open(ctx, testutil.RawDocFromString(content, model.LocaleEnglish)))

	var out []*model.Block
	for pr := range r.Read(ctx) {
		require.NoError(t, pr.Error)
		if pr.Part == nil || pr.Part.Type != model.PartBlock {
			continue
		}
		b, ok := pr.Part.Resource.(*model.Block)
		if !ok || b == nil || b.SourceText() == "" {
			continue
		}
		out = append(out, b)
	}
	return out
}

// A one-paragraph deletion in a documentation page moves no other block, or
// exactly the later paragraphs of its own section.
//
// This is the push's own question. The source venue keys every block on
// convergence.BlockKey and compares the transfer hash, which folds the content
// hash with the context hash model.ComputeIdentity derives from the name, so a
// block is re-sent exactly when one of those changes. The page below has the
// shape of a docs page: a title, sections at two levels, an ordered list, code
// fences, a table and prose in several sections. It is read through the
// markdown reader and through the mdx reader the docs collections declare.
//
// The ordinal that separates sibling paragraphs of one section is the only
// positional element left in a markdown name. Deleting a section's only
// paragraph therefore moves nothing, and deleting the first of two moves the
// second and nothing else. Measured over web/docs, a paragraph has 1.6 later
// siblings in its section on average, so the second case is the ceiling a
// typical edit reaches. markdown.TestMarkdownNames_ShiftWithinTheSameSection
// pins the same boundary on the names themselves.
func TestBlockIdentity_OneParagraphDeletionMovesOnlyItsLaterSiblings(t *testing.T) {
	const page = `# Test Strategy

## Principles

1. **Every format has tests.** No reader ships without them.
2. **Roundtrip is the standard.** Read, write, compare.

## Test Patterns

### Roundtrip Test

The most important test for any format:

` + "```go\nfunc TestRoundTrip(t *testing.T) {}\n```" + `

### Extraction Test

Verify specific blocks are extracted with correct content:

` + "```go\nfunc TestExtraction(t *testing.T) {}\n```" + `

### Flow Execution Test

Build a tool and drive it through the executor:

` + "```go\nfunc TestFlowExecution(t *testing.T) {}\n```" + `

## Running Tests

Single test:

` + "```bash\ngo test ./core/flow/ -run TestName -v\n```" + `

Pass the tags in one flag.

## Test Tags

| Tag           | Purpose                 |
| ------------- | ----------------------- |
| (none)        | Unit tests only         |
| integration   | Plus integration tests  |

## CI

Tests run on every push and pull request.
`
	deletions := []struct {
		name      string
		paragraph string
		// moved names the blocks whose identity the deletion is allowed to
		// change: the later paragraphs of the deleted one's section.
		moved []string
	}{
		{
			name:      "the only paragraph of its section",
			paragraph: "Verify specific blocks are extracted with correct content:\n\n",
		},
		{
			name:      "the first of two paragraphs in one section",
			paragraph: "Single test:\n\n",
			moved:     []string{"Pass the tags in one flag."},
		},
	}
	readers := map[string]func() format.DataFormatReader{
		"markdown": func() format.DataFormatReader { return markdown.NewReader() },
		"mdx":      func() format.DataFormatReader { return mdx.NewReader() },
	}

	type identity struct{ key, content, context string }
	idOf := func(b *model.Block) identity {
		id := model.ComputeIdentity(b)
		return identity{convergence.BlockKey(b), id.ContentHash, id.ContextHash}
	}

	for name, newReader := range readers {
		for _, del := range deletions {
			t.Run(name+"/"+del.name, func(t *testing.T) {
				require.Contains(t, page, del.paragraph)
				edited := strings.Replace(page, del.paragraph, "", 1)

				before := readIdentityBlocks(t, newReader(), page)
				after := readIdentityBlocks(t, newReader(), edited)
				require.Len(t, after, len(before)-1, "exactly the deleted paragraph is gone")

				wasByText := map[string]identity{}
				for _, b := range before {
					wasByText[b.SourceText()] = idOf(b)
				}

				var moved []string
				for _, b := range after {
					was, ok := wasByText[b.SourceText()]
					require.True(t, ok, "block %q appeared from nowhere", b.SourceText())
					if was != idOf(b) {
						moved = append(moved, b.SourceText())
					}
				}
				assert.Equal(t, del.moved, moved,
					"a one-paragraph deletion moves the later paragraphs of its own section and nothing else")
			})
		}
	}
}

// Formats without author-controlled structure name blocks by position, and
// that is left in place.
//
// Keyed formats above are stable because their name IS the key. Plaintext is a
// flat list of lines with nothing to scope an ordinal to, so it names by
// "lineN" and a deletion renames every block below it. HTML and XML scope the
// ordinal to the parent element, which is the tightest structure the author
// gives: siblings of ONE parent still renumber, as the cases here show, and the
// sectioned documents above do not. Markdown's equivalent boundary is sibling
// paragraphs of one section (markdown.TestMarkdownNames_ShiftWithinTheSameSection).
//
// Making those names content-derived was tried and rejected. It fixes deletion
// and breaks editing — a reworded paragraph becomes a different block and loses
// its history — and worse, model.ComputeIdentity builds ContextHash from the
// name, so deriving the name from content collapses the two hashes into one and
// destroys the independent signal that identity matching depends on.
//
// So the positional name stays, and is read as what it honestly is: a CONTEXT
// signal. Identity is resolved against it in core/reconcile, which matches a
// read against the previous one on content and context together. See
// TestReaders_IdentitySurvivesAnUnrelatedDeletion for the end-to-end claim.
//
// This test pins the reader behaviour that reconcile is built on: if these names
// ever stop moving, the context signal has changed and the matching rules need
// re-examining.
func TestBlockIdentity_ProseFormatsNamePositionally(t *testing.T) {
	for _, tc := range []struct {
		name       string
		reader     func() format.DataFormatReader
		v1, v2     string
		wasCharlie string
		nowCharlie string
	}{
		{
			name:       "plaintext",
			reader:     func() format.DataFormatReader { return plaintext.NewReader() },
			v1:         "Alpha\n\nBravo\n\nCharlie\n",
			v2:         "Alpha\n\nCharlie\n",
			wasCharlie: "line5", nowCharlie: "line3",
		},
		{
			name:       "html siblings of one parent",
			reader:     func() format.DataFormatReader { return html.NewReader() },
			v1:         "<p>Alpha</p><p>Bravo</p><p>Charlie</p>",
			v2:         "<p>Alpha</p><p>Charlie</p>",
			wasCharlie: "p#3", nowCharlie: "p#2",
		},
		{
			name:       "xml siblings of one parent",
			reader:     func() format.DataFormatReader { return xml.NewReader() },
			v1:         "<doc><p>Alpha</p><p>Bravo</p><p>Charlie</p></doc>",
			v2:         "<doc><p>Alpha</p><p>Charlie</p></doc>",
			wasCharlie: "doc.p#3", nowCharlie: "doc.p#2",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before := identitiesByText(t, tc.reader(), tc.v1)
			after := identitiesByText(t, tc.reader(), tc.v2)

			require.Equal(t, tc.wasCharlie, before["Charlie"])
			require.Equal(t, tc.nowCharlie, after["Charlie"],
				"a positional name follows position; this is the context signal reconcile reads")
			require.NotEqual(t, before["Charlie"], after["Charlie"],
				"names moving is the premise core/reconcile is built on")
		})
	}
}
