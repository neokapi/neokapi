package asciidoc_test

import (
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestReaderRolesAndStructure is the Structure & Geometry (G) axis evidence:
// the reader populates the core/model structural roles for every logical
// construct in the exemplar. Together with the table-group structure
// (TestReaderTableStructure) and reading-order groups
// (TestReaderReadingOrderGroups) this earns G3.
func TestReaderRolesAndStructure(t *testing.T) {
	t.Parallel()
	input := mustRead(t, filepath.Join("testdata", "sample.adoc"))
	blocks := readBlocks(t, input)

	// Document title -> heading role, level 1.
	title := blockByText(t, blocks, "Document Title")
	assert.Equal(t, model.RoleHeading, title.SemanticRole())
	assertLevel(t, title, 1)

	// Section headings carry heading role with the `=`-count level.
	assertLevel(t, requireRole(t, blockByText(t, blocks, "First Section"), model.RoleHeading), 2)
	assertLevel(t, requireRole(t, blockByText(t, blocks, "Subsection"), model.RoleHeading), 3)

	// Paragraph and admonition -> paragraph role.
	requireRole(t, blockByText(t, blocks, "This is the first paragraph with bold, italic, and mono text."), model.RoleParagraph)
	requireRole(t, blockByText(t, blocks, "This is an admonition paragraph."), model.RoleParagraph)

	// Block title -> caption role.
	requireRole(t, blockByText(t, blocks, "A list of items"), model.RoleCaption)

	// List items -> list-item role with nesting level.
	assertLevel(t, requireRole(t, blockByText(t, blocks, "First item"), model.RoleListItem), 1)
	assertLevel(t, requireRole(t, blockByText(t, blocks, "Nested item"), model.RoleListItem), 2)

	// Table cells: header row vs body cells carry the right roles.
	requireRole(t, blockByText(t, blocks, "Name"), model.RoleTableHeader)
	requireRole(t, blockByText(t, blocks, "Role"), model.RoleTableHeader)
	requireRole(t, blockByText(t, blocks, "Alice"), model.RoleTableCell)
	requireRole(t, blockByText(t, blocks, "Engineer"), model.RoleTableCell)
}

// TestReaderTableStructure asserts the table is reconstructed as a table group
// of table-row groups whose cells carry RoleTableCell / RoleTableHeader.
func TestReaderTableStructure(t *testing.T) {
	t.Parallel()
	input := "|===\n| H1 | H2\n\n| a | b\n| c | d\n|===\n"
	parts := readParts(t, input)

	var groupTypes []string
	var headerRows int
	for _, p := range parts {
		if p.Type == model.PartGroupStart {
			g := p.Resource.(*model.GroupStart)
			groupTypes = append(groupTypes, g.Type)
			if g.Type == "table-row" && g.Properties["header"] == "true" {
				headerRows++
			}
		}
	}
	assert.Contains(t, groupTypes, "table")
	rowCount := 0
	for _, g := range groupTypes {
		if g == "table-row" {
			rowCount++
		}
	}
	assert.Equal(t, 3, rowCount, "header + two body rows")
	assert.Equal(t, 1, headerRows, "exactly one header row (blank line after first row)")

	blocks := readBlocks(t, input)
	requireRole(t, blockByText(t, blocks, "H1"), model.RoleTableHeader)
	requireRole(t, blockByText(t, blocks, "a"), model.RoleTableCell)
}

// TestReaderTableColsAttribute asserts a `[%header,cols=N]` table whose cells
// are each on their own line (no blank-line row boundary — the shape the
// generative writer emits) is parsed into N-wide rows with a header, by honoring
// the cols/%header block attribute rather than inferring 1 column from the first
// line.
func TestReaderTableColsAttribute(t *testing.T) {
	t.Parallel()
	input := "[%header,cols=3]\n|===\n| Format\n| Read\n| Write\n| Markdown\n| yes\n| yes\n| HTML\n| no\n| yes\n|===\n"
	parts := readParts(t, input)

	rowCount, headerRows := 0, 0
	for _, p := range parts {
		if p.Type == model.PartGroupStart {
			g := p.Resource.(*model.GroupStart)
			if g.Type == "table-row" {
				rowCount++
				if g.Properties["header"] == "true" {
					headerRows++
				}
			}
		}
	}
	assert.Equal(t, 3, rowCount, "cols=3 must group 9 cells into a header + two body rows")
	assert.Equal(t, 1, headerRows, "%header must promote the first row")

	blocks := readBlocks(t, input)
	requireRole(t, blockByText(t, blocks, "Format"), model.RoleTableHeader)
	requireRole(t, blockByText(t, blocks, "Markdown"), model.RoleTableCell)
}

// TestReaderReadingOrderGroups asserts groups are emitted (reading order, G2)
// and balanced: document wraps sections, sections nest, lists/tables bracket
// their items.
func TestReaderReadingOrderGroups(t *testing.T) {
	t.Parallel()
	input := mustRead(t, filepath.Join("testdata", "sample.adoc"))
	parts := readParts(t, input)

	var starts, ends int
	var typesSeen = map[string]int{}
	for _, p := range parts {
		switch p.Type {
		case model.PartGroupStart:
			starts++
			typesSeen[p.Resource.(*model.GroupStart).Type]++
		case model.PartGroupEnd:
			ends++
		}
	}
	assert.Equal(t, starts, ends, "balanced group brackets")
	assert.Positive(t, typesSeen["document"], "a document group brackets the body")
	assert.Positive(t, typesSeen["section"], "sections are grouped")
	assert.Positive(t, typesSeen["list"], "lists are grouped")
	assert.Positive(t, typesSeen["table"], "tables are grouped")
}

func requireRole(t *testing.T, b *model.Block, role string) *model.Block {
	t.Helper()
	require.Equal(t, role, b.SemanticRole(), "block %q role", b.SourceText())
	return b
}

func assertLevel(t *testing.T, b *model.Block, level int) {
	t.Helper()
	s, ok := b.Structure()
	require.True(t, ok, "block %q missing structure annotation", b.SourceText())
	assert.Equal(t, level, s.Level, "block %q level", b.SourceText())
}
