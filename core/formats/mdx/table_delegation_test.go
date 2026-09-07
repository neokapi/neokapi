package mdx

import (
	"bytes"
	"context"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// tableReadResult is what a page's tables came to: the cells offered for
// translation, and the opaque regions that swallowed a table whole.
type tableReadResult struct {
	cells        []*model.Block
	opaqueTables int
	roundTrip    []byte
}

// readTables reads src with content surfacing set to surfacing, counting the
// table-cell blocks and the opaque table regions, and writes it back
// untranslated.
func readTables(t *testing.T, src []byte, surfacing bool) tableReadResult {
	t.Helper()
	r := NewReader()
	r.cfg.SetExtractNonTranslatableContent(surfacing)
	store, err := format.NewSkeletonStore()
	require.NoError(t, err)
	defer func() { _ = store.Close() }()
	r.SetSkeletonStore(store)
	require.NoError(t, r.Open(context.Background(), &model.RawDocument{
		Reader:       io.NopCloser(bytes.NewReader(src)),
		SourceLocale: model.LocaleEnglish,
	}))

	var out tableReadResult
	var parts []*model.Part
	for pr := range r.Read(context.Background()) {
		require.NoError(t, pr.Error)
		parts = append(parts, pr.Part)
		switch v := pr.Part.Resource.(type) {
		case *model.Block:
			if v.Type == "table-cell" {
				out.cells = append(out.cells, v)
			}
		case *model.Data:
			if v.Properties["kind"] == "table" {
				out.opaqueTables++
			}
		}
	}
	out.roundTrip = writeParts(t, parts, store, "")
	return out
}

// TestDocsCorpusTablesAreNeverOpaque is the #2433 measurement. Every GFM table
// in an MDX span used to be cut out and kept verbatim, because the markdown
// reader normalised a cell's padding; with content surfacing off, which is how
// the embedded markdown reader and the parity runs are configured, that left
// 241 opaque table regions across the docs tree and not one translatable cell
// in any of them. Tables now travel the ordinary delegation, so the count is
// zero whatever the surfacing flag says.
func TestDocsCorpusTablesAreNeverOpaque(t *testing.T) {
	t.Parallel()
	var pages []string
	require.NoError(t, filepath.WalkDir(docsRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && (strings.HasSuffix(path, ".md") || strings.HasSuffix(path, ".mdx")) {
			pages = append(pages, path)
		}
		return nil
	}))
	require.NotEmpty(t, pages)

	withTables, opaque, cells := 0, 0, 0
	for _, page := range pages {
		src, err := os.ReadFile(page)
		require.NoError(t, err)
		if !bytes.Contains(src, []byte("\n| ")) && !bytes.Contains(src, []byte("\n|-")) {
			continue
		}
		withTables++
		for _, surfacing := range []bool{true, false} {
			got := readTables(t, src, surfacing)
			opaque += got.opaqueTables
			cells += len(got.cells)
			rel, _ := filepath.Rel(docsRoot, page)
			assert.Equalf(t, string(src), string(got.roundTrip),
				"%s does not round-trip byte-for-byte with surfacing=%v", rel, surfacing)
		}
	}

	assert.Positive(t, withTables, "the docs tree has pages with tables")
	assert.Zero(t, opaque, "no table may be kept opaque")
	assert.Positive(t, cells, "a table's cells are offered for translation")
}

// TestTableCellsCarryInlineRuns pins the difference delegation makes to a
// cell's content. The MDX-specific surfacing took a cell as a byte slice, so a
// code span, a link or an emphasis inside one reached the translator as
// characters and reached content memory as markup; the markdown reader parses
// them into runs, and the cell's text is the prose a reader reads.
func TestTableCellsCarryInlineRuns(t *testing.T) {
	t.Parallel()
	src, err := os.ReadFile(filepath.Join("testdata", "table-inline-markup.mdx"))
	require.NoError(t, err)

	got := readTables(t, src, true)
	require.NotEmpty(t, got.cells)
	assert.Equal(t, string(src), string(got.roundTrip))

	texts := make([]string, 0, len(got.cells))
	for _, cell := range got.cells {
		texts = append(texts, cell.SourceText())
	}
	assert.Contains(t, texts, "kapi up", "a code span's content, without its backticks")
	assert.Contains(t, texts, "The command reference", "a link's text, without its destination")
	assert.Contains(t, texts, "Content memory", "bold text, without its asterisks")
	assert.Contains(t, texts, "Beside the terms store")
	assert.Contains(t, texts, "A cell with code and a link and bold together")

	for _, cell := range got.cells {
		assert.NotContains(t, cell.SourceText(), "|", "a pipe is the table's markup, not a cell's text")
	}
}

// TestTableBetweenJSXBlocksDelegates covers a markdown span that opens with a
// table and sits between two JSX blocks: it has no prose before it to anchor
// the span, and the elements around it stay skeleton.
func TestTableBetweenJSXBlocksDelegates(t *testing.T) {
	t.Parallel()
	src, err := os.ReadFile(filepath.Join("testdata", "table-in-jsx.mdx"))
	require.NoError(t, err)

	for _, surfacing := range []bool{true, false} {
		got := readTables(t, src, surfacing)
		assert.Zero(t, got.opaqueTables)
		assert.Equal(t, string(src), string(got.roundTrip),
			"the page round-trips byte-for-byte with surfacing=%v", surfacing)

		texts := make([]string, 0, len(got.cells))
		for _, cell := range got.cells {
			texts = append(texts, cell.SourceText())
		}
		assert.Contains(t, texts, "Converges the project")
		assert.Contains(t, texts, "Sends the declared tree")
	}
}

// TestTableInsideAJSXElementDelegates covers a table inside a <TabItem>. A JSX
// element's blank-line-separated children are markdown by the MDX spec, so they
// take the same markdown delegation a top-level span does: the table arrives as
// its cells rather than as one string of pipes and delimiter rows, which a
// translator breaks by changing a column's width and content memory keys on
// (#2473).
func TestTableInsideAJSXElementDelegates(t *testing.T) {
	t.Parallel()
	src, err := os.ReadFile(filepath.Join("testdata", "table-in-jsx.mdx"))
	require.NoError(t, err)

	blocks := readAllBlocks(t, src)

	var jsxText, cells, paragraphs []string
	for _, b := range blocks {
		switch {
		case b.Type == "jsx-text":
			jsxText = append(jsxText, b.SourceText())
		case b.SemanticRole() == model.RoleTableHeader || b.SemanticRole() == model.RoleTableCell:
			cells = append(cells, b.SourceText())
		case b.Type == "":
			paragraphs = append(paragraphs, b.SourceText())
		}
	}

	for _, text := range jsxText {
		assert.NotContains(t, text, "| ---", "no table markup reaches the translator as one string")
	}
	assert.Equal(t, []string{"Command line", "Continuous integration"}, jsxText,
		"an attribute value is still a JSX text child")
	for _, want := range []string{"Flag", "Effect", "--dry-run", "Reports, writes not",
		"Job", "When it runs", "check", "On every pull request"} {
		assert.Contains(t, cells, want, "a cell inside a JSX element is its own block")
	}
	assert.Contains(t, paragraphs, "The callout holds prose of its own.",
		"prose in a JSX block is a paragraph of its own")
	assert.Contains(t, paragraphs, "A second table inside the same element:")

	// The delegation must not cost the region its byte-exact round trip.
	assert.Equal(t, string(src), string(roundTrip(t, src)))
}

// TestJSXChildDelegationStopsWhereTheSpecDoes pins the boundary: a child on the
// same line as its tags is inline text, an element the translatability table
// rules out holds markup rather than prose, and a child the markdown reader
// cannot reconstruct falls back to the verbatim block it always was.
func TestJSXChildDelegationStopsWhereTheSpecDoes(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		src      string
		jsxText  []string
		markdown []string
	}{
		{
			name:    "inline child",
			src:     "<Callout>Inline prose here.</Callout>\n",
			jsxText: []string{"Inline prose here."},
		},
		{
			name:    "no blank line after the child",
			src:     "<Callout>\n\nProse here.\n</Callout>\n",
			jsxText: []string{"Prose here."},
		},
		{
			name:    "element whose text is code",
			src:     "<script>\n\nconst x = 1;\n\n</script>\n",
			jsxText: []string{"const x = 1;"},
		},
		{
			name:    "child the markdown reader cannot reconstruct",
			src:     "<Callout>\n\nA link [x](( ) inside a sentence.\n\n</Callout>\n",
			jsxText: []string{"A link [x](( ) inside a sentence."},
		},
		{
			name:     "block-level child",
			src:      "<Callout>\n\nProse here.\n\n</Callout>\n",
			markdown: []string{"Prose here."},
		},
		{
			name:     "block-level list",
			src:      "<Callout>\n\n- One item\n- Another item\n\n</Callout>\n",
			markdown: []string{"One item", "Another item"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			blocks := readAllBlocks(t, []byte(tc.src))
			var jsxText, other []string
			for _, b := range blocks {
				if b.Type == "jsx-text" {
					jsxText = append(jsxText, b.SourceText())
					continue
				}
				other = append(other, b.SourceText())
			}
			assert.Equal(t, tc.jsxText, jsxText)
			assert.Equal(t, tc.markdown, other)
			assert.Equal(t, tc.src, string(roundTrip(t, []byte(tc.src))),
				"the round trip stays byte-exact whichever path the child takes")
		})
	}
}

// readAllBlocks returns every block the reader emits, translatable or not.
func readAllBlocks(t *testing.T, src []byte) []*model.Block {
	t.Helper()
	r := NewReader()
	store, err := format.NewSkeletonStore()
	require.NoError(t, err)
	defer func() { _ = store.Close() }()
	r.SetSkeletonStore(store)
	require.NoError(t, r.Open(context.Background(), &model.RawDocument{
		Reader:       io.NopCloser(bytes.NewReader(src)),
		SourceLocale: model.LocaleEnglish,
	}))
	var blocks []*model.Block
	for pr := range r.Read(context.Background()) {
		require.NoError(t, pr.Error)
		if b, ok := pr.Part.Resource.(*model.Block); ok {
			blocks = append(blocks, b)
		}
	}
	return blocks
}

// TestTranslatedCellInsideJSXSplicesBack drives the point of the delegation: a
// translated cell inside a JSX element is written back into the table's own
// markup, with the pipes and the delimiter row untouched.
func TestTranslatedCellInsideJSXSplicesBack(t *testing.T) {
	t.Parallel()
	src := []byte("<TabItem value=\"cli\">\n\n| Flag | Effect |\n| ---- | ------ |\n| a | Reports |\n\n</TabItem>\n")

	parts, store := readParts(t, src)
	for _, p := range parts {
		if b, ok := p.Resource.(*model.Block); ok && b.SourceText() == "Reports" {
			b.SetTargetRuns(model.LocaleGerman, []model.Run{{Text: &model.TextRun{Text: "Meldet"}}})
		}
	}

	assert.Equal(t, "<TabItem value=\"cli\">\n\n| Flag | Effect |\n| ---- | ------ |\n| a | Meldet |\n\n</TabItem>\n",
		string(writeParts(t, parts, store, model.LocaleGerman)))
}

// TestTableEscapedPipesDelegate covers the fixture the escape convention lives
// on: the markdown reader hands a cell back with `\|` unescaped and its writer
// escapes an unescaped pipe on the way out.
func TestTableEscapedPipesDelegate(t *testing.T) {
	t.Parallel()
	src, err := os.ReadFile(filepath.Join("testdata", "table-escaped-pipes.mdx"))
	require.NoError(t, err)

	got := readTables(t, src, true)
	assert.Zero(t, got.opaqueTables)
	assert.Equal(t, string(src), string(got.roundTrip))

	texts := make([]string, 0, len(got.cells))
	for _, cell := range got.cells {
		texts = append(texts, cell.SourceText())
	}
	assert.Contains(t, texts, "--output-format <json|text>")
	assert.Contains(t, texts, "enum=fast|thorough")
}

// TestTranslatedTableCellSplicesInPlace writes a table back for another locale:
// the translated cell takes the place of its own bytes and nothing else in the
// table moves, which is what the skeleton refs are for.
func TestTranslatedTableCellSplicesInPlace(t *testing.T) {
	t.Parallel()
	src := []byte("| Name | Value |\n| ---- | ----- |\n| one  | two   |\n")

	r := NewReader()
	store, err := format.NewSkeletonStore()
	require.NoError(t, err)
	defer func() { _ = store.Close() }()
	r.SetSkeletonStore(store)
	require.NoError(t, r.Open(context.Background(), &model.RawDocument{
		Reader:       io.NopCloser(bytes.NewReader(src)),
		SourceLocale: model.LocaleEnglish,
	}))

	var parts []*model.Part
	for pr := range r.Read(context.Background()) {
		require.NoError(t, pr.Error)
		parts = append(parts, pr.Part)
		block, ok := pr.Part.Resource.(*model.Block)
		if ok && block.SourceText() == "one" {
			block.SetTargetText(model.LocaleGerman, "eins")
		}
	}

	assert.Equal(t, "| Name | Value |\n| ---- | ----- |\n| eins  | two   |\n",
		string(writeParts(t, parts, store, model.LocaleGerman)))
}
