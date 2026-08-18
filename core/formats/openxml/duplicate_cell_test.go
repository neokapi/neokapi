package openxml

import (
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// #2067. A worksheet cell's grid anchor is identified by the `r` reference the
// sheet supplies. OOXML says a reference is unique within a sheet; a repaired or
// machine-generated workbook repeats one, and the reader neither validated nor
// separated it — so two anchors arrived under one id and collided in the block
// store, which keys on (source document, block id).
//
// These anchors are non-translatable and carry no skeleton ref, so nothing was
// ever written back through one. That bounds the damage to the store's own view
// of the grid, and does not change what the id has to be.
func TestReader_RepeatedCellReferencesGetDistinctBlockIDs(t *testing.T) {
	const sheet = `<?xml version="1.0" encoding="UTF-8"?>
<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">
  <sheetData>
    <row r="1">
      <c r="A1"><v>1</v></c>
      <c r="A1"><v>2</v></c>
      <c r="B1"><v>3</v></c>
    </row>
  </sheetData>
</worksheet>`

	blockCounter := 0
	var ids model.IDBuilder
	cfg := &Config{}
	cfg.Reset()
	cfg.SetExtractNonTranslatableContent(true)
	p := &smlParser{cfg: cfg, blockCounter: &blockCounter, ids: &ids}

	var anchors []*model.Block
	require.NoError(t, p.parseWorksheet([]byte(sheet), "xl/worksheets/sheet1.xml", func(b *model.Block) {
		if b.Type == "cell" {
			anchors = append(anchors, b)
		}
	}))
	require.Len(t, anchors, 3)

	seen := map[string]bool{}
	for _, b := range anchors {
		assert.False(t, seen[b.ID], "block id %q is not an identity", b.ID)
		seen[b.ID] = true
	}

	// The first anchor of a repeated reference keeps its id, so a conforming
	// workbook is untouched, and the repeat remembers what the sheet said.
	assert.Equal(t, "cell-sheet1-A1", anchors[0].ID)
	assert.Empty(t, anchors[0].Properties[model.PropDocumentID])
	assert.Equal(t, "cell-sheet1-A1", model.DocumentID(anchors[1]))
	assert.Equal(t, "A1", anchors[1].Properties["cell"],
		"the cell's own address is format data and is untouched by separation")
}
