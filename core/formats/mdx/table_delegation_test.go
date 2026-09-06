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

// TestTableInsideAJSXElementRidesTheJSXPath pins where the delegation stops.
// The MDX scanner takes a block-level JSX region up to its closing tag as one
// segment, so a table inside a <TabItem> is a JSX text child and reaches the
// translator as one string of table markup rather than as cells. No page under
// web/docs has one, and #2473 carries the fix; this test is here so the day it
// changes, it says so.
func TestTableInsideAJSXElementRidesTheJSXPath(t *testing.T) {
	t.Parallel()
	src, err := os.ReadFile(filepath.Join("testdata", "table-in-jsx.mdx"))
	require.NoError(t, err)

	r := NewReader()
	store, err := format.NewSkeletonStore()
	require.NoError(t, err)
	defer func() { _ = store.Close() }()
	r.SetSkeletonStore(store)
	require.NoError(t, r.Open(context.Background(), &model.RawDocument{
		Reader:       io.NopCloser(bytes.NewReader(src)),
		SourceLocale: model.LocaleEnglish,
	}))

	var jsxText []string
	for pr := range r.Read(context.Background()) {
		require.NoError(t, pr.Error)
		if b, ok := pr.Part.Resource.(*model.Block); ok && b.Type == "jsx-text" {
			jsxText = append(jsxText, b.SourceText())
		}
	}

	var tableChildren int
	for _, text := range jsxText {
		if strings.Contains(text, "| ---") {
			tableChildren++
		}
	}
	assert.Equal(t, 2, tableChildren, "both tab tables arrive as JSX text children")
	assert.Contains(t, jsxText, "The callout holds prose of its own.",
		"ordinary prose in a JSX block is still a block of its own")
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
