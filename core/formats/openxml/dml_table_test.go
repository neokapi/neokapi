package openxml

import (
	"bytes"
	"context"
	"io"
	"os"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// pptxParts reads a .pptx and returns every part the reader emits, groups
// included, in document order.
func pptxParts(t *testing.T, path string) []*model.Part {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)

	r := NewReader()
	ctx := context.Background()
	require.NoError(t, r.Open(ctx, &model.RawDocument{
		URI:          path,
		SourceLocale: model.LocaleEnglish,
		Reader:       io.NopCloser(bytes.NewReader(data)),
	}))
	t.Cleanup(func() { _ = r.Close() })

	var out []*model.Part
	for pr := range r.Read(ctx) {
		require.NoError(t, pr.Error)
		if pr.Part != nil {
			out = append(out, pr.Part)
		}
	}
	return out
}

// A slide table (a:tbl in a graphic frame) is bracketed as a table Group of
// table-row Groups, its paragraphs carry the table-cell role, and a merged
// cell carries its span, so the deck converts to a grid rather than to a run
// of loose paragraphs. The fixture's second slide holds a 4 × 3 table whose
// last row merges its first two cells.
func TestSlideTableIsBracketedAsTable(t *testing.T) {
	parts := pptxParts(t, "testdata/table.pptx")

	var tables, rows int
	var cells []*model.Block
	for _, p := range parts {
		switch p.Type {
		case model.PartGroupStart:
			g := p.Resource.(*model.GroupStart)
			switch g.Type {
			case "table":
				tables++
			case "table-row":
				rows++
			}
		case model.PartBlock:
			b := p.Resource.(*model.Block)
			if r := b.SemanticRole(); r == model.RoleTableCell || r == model.RoleTableHeader {
				cells = append(cells, b)
			}
		}
	}
	assert.Equal(t, 1, tables, "one table group")
	assert.Equal(t, 4, rows, "one row group per a:tr")

	texts := make([]string, 0, len(cells))
	roles := make([]string, 0, len(cells))
	for _, c := range cells {
		texts = append(texts, c.SourceText())
		roles = append(roles, c.SemanticRole())
	}
	assert.Equal(t, []string{
		"Plan", "Monthly", "Payout",
		"Starter", "€0", "Monthly",
		"Growth", "€49", "Weekly",
		"Enterprise, custom terms", "Daily",
	}, texts, "every cell paragraph is a table cell; the merge continuation holds no text")
	th, td := model.RoleTableHeader, model.RoleTableCell
	assert.Equal(t, []string{th, th, th, td, td, td, td, td, td, td, td}, roles,
		"the first row is the header row the table's firstRow flag declares")

	merged := cells[len(cells)-2]
	s, ok := merged.Structure()
	require.True(t, ok)
	assert.Equal(t, 2, s.ColSpan, "the merged cell spans the two columns")

	// Group ids share the block counter, so none collides with a block.
	ids := map[string]bool{}
	for _, p := range parts {
		var id string
		switch p.Type {
		case model.PartGroupStart:
			id = p.Resource.(*model.GroupStart).ID
		case model.PartBlock:
			id = p.Resource.(*model.Block).ID
		default:
			continue
		}
		assert.False(t, ids[id], "duplicate part id %s", id)
		ids[id] = true
	}
}

// Every slide title appears once: a slide reached from presentation.xml and
// again from its notes slide is read a single time. The layouts' prompt text
// ("Click to edit Master title style") belongs to the editor and is not
// extracted unless the masters option asks for it.
func TestDeckReadsEachSlideOnceWithoutLayouts(t *testing.T) {
	blocks := pptxBlocks(t, "testdata/table.pptx")
	require.NotEmpty(t, blocks)

	// Keyed by text and role: the deck's title is also its dc:title core
	// property, which is a property block rather than a second heading.
	seen := map[roledBlock]int{}
	for _, b := range blocks {
		seen[b]++
		assert.NotContains(t, b.text, "Click to edit", "layout prompt text extracted: %q", b.text)
	}
	assert.Equal(t, 1, seen[roledBlock{"Table deck", model.RoleHeading, 1}], "the title slide's title, once")
	assert.Equal(t, 1, seen[roledBlock{"Fees at a glance", model.RoleHeading, 2}], "the table slide's title, once although it has notes")
	assert.Equal(t, 1, seen[roledBlock{"Keep this slide short.", "", 0}], "the speaker notes, once")
}
