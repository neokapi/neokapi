package csv_test

import (
	"testing"

	csvfmt "github.com/neokapi/neokapi/core/formats/csv"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// nonTranslatableBlocks returns every non-translatable Block in parts (the
// counterpart to collectBlocks, which returns only translatable ones).
func nonTranslatableBlocks(parts []*model.Part) []*model.Block {
	var out []*model.Block
	for _, p := range parts {
		if p.Type != model.PartBlock {
			continue
		}
		if b, ok := p.Resource.(*model.Block); ok && !b.Translatable {
			out = append(out, b)
		}
	}
	return out
}

func dataPartsNamed(parts []*model.Part, name string) []*model.Data {
	var out []*model.Data
	for _, p := range parts {
		if p.Type != model.PartData {
			continue
		}
		if d, ok := p.Resource.(*model.Data); ok && d.Name == name {
			out = append(out, d)
		}
	}
	return out
}

// --- Config / schema surface ---

func TestExtractNonTranslatableContent_DefaultOn(t *testing.T) {
	t.Parallel()
	// Zero-value config (however constructed) has surfacing ON.
	var zero csvfmt.Config
	assert.True(t, zero.ExtractNonTranslatableContent(), "zero-value Config defaults to extraction on")

	cfg := &csvfmt.Config{}
	cfg.Reset()
	assert.True(t, cfg.ExtractNonTranslatableContent(), "Reset() keeps extraction on")
}

func TestExtractNonTranslatableContent_Setter(t *testing.T) {
	t.Parallel()
	cfg := &csvfmt.Config{}
	cfg.Reset()
	cfg.SetExtractNonTranslatableContent(false)
	assert.False(t, cfg.ExtractNonTranslatableContent())
	cfg.SetExtractNonTranslatableContent(true)
	assert.True(t, cfg.ExtractNonTranslatableContent())
}

func TestExtractNonTranslatableContent_ApplyMap(t *testing.T) {
	t.Parallel()
	cfg := &csvfmt.Config{}
	cfg.Reset()

	require.NoError(t, cfg.ApplyMap(map[string]any{"extractNonTranslatableContent": false}))
	assert.False(t, cfg.ExtractNonTranslatableContent())

	require.NoError(t, cfg.ApplyMap(map[string]any{"extractNonTranslatableContent": true}))
	assert.True(t, cfg.ExtractNonTranslatableContent())

	require.Error(t, cfg.ApplyMap(map[string]any{"extractNonTranslatableContent": "yes"}))
}

func TestExtractNonTranslatableContent_Schema(t *testing.T) {
	t.Parallel()
	cfg := &csvfmt.Config{}
	s := cfg.Schema()
	require.NotNil(t, s)
	prop, ok := s.Properties["extractNonTranslatableContent"]
	require.True(t, ok, "schema declares extractNonTranslatableContent")
	assert.Equal(t, "boolean", prop.Type)
	assert.Equal(t, true, prop.Default)
}

// --- Non-translatable data cells (finding 1) ---

func TestExtractNonTranslatable_DataCells_DefaultOn(t *testing.T) {
	t.Parallel()
	// Columns 0 (id) and 2 (count) are non-translatable; column 1 (name) is.
	parts := readCSVWithConfig(t, "id,name,count\n1,Alice,10\n2,Bob,20\n", func(c *csvfmt.Config) {
		c.TranslatableColumns = []int{1}
	})

	// Translatable payload is unchanged: only the name cells.
	assert.Equal(t, []string{"Alice", "Bob"}, blockTexts(collectBlocks(parts)))

	// The non-translatable cells surface as RoleTableCell content blocks.
	nt := nonTranslatableBlocks(parts)
	var cellTexts []string
	for _, b := range nt {
		if b.SemanticRole() == model.RoleTableCell {
			assert.False(t, b.Translatable)
			assert.Equal(t, "table-cell", b.Type)
			cellTexts = append(cellTexts, b.SourceText())
		}
	}
	assert.Equal(t, []string{"1", "10", "2", "20"}, cellTexts)

	// And no data-cell Data parts remain (header is per-cell blocks here).
	for _, p := range parts {
		if p.Type == model.PartData {
			t.Errorf("unexpected Data part with extraction on: %+v", p.Resource)
		}
	}
}

func TestExtractNonTranslatable_DataCells_Off_StaysData(t *testing.T) {
	t.Parallel()
	parts := readCSVWithConfig(t, "id,name,count\n1,Alice,10\n2,Bob,20\n", func(c *csvfmt.Config) {
		c.TranslatableColumns = []int{1}
		c.SetExtractNonTranslatableContent(false)
	})

	// No non-translatable data cells surface as content blocks (header cells,
	// which are always per-cell blocks in this projection, are not counted).
	for _, b := range nonTranslatableBlocks(parts) {
		assert.NotEqual(t, model.RoleTableCell, b.SemanticRole(), "extraction off keeps data cells as Data")
	}

	// The id/count cells are opaque Data parts (legacy behavior).
	var dataContent []string
	for _, p := range parts {
		if p.Type == model.PartData {
			dataContent = append(dataContent, p.Resource.(*model.Data).Properties["content"])
		}
	}
	assert.Equal(t, []string{"1", "10", "2", "20"}, dataContent)
}

func TestExtractNonTranslatable_DataCells_Skeleton_ByteExact(t *testing.T) {
	t.Parallel()
	// Skeleton path, extraction on (default): non-translatable cells become
	// content blocks but their raw bytes stay in skeleton, so round-trip is
	// byte-exact.
	input := "id,name,count\n1,Alice,10\n2,\"B,ob\",20"
	output := skeletonRoundtrip(t, input, func(c *csvfmt.Config) {
		c.TranslatableColumns = []int{1}
	})
	assert.Equal(t, input, output)
}

// --- Preamble rows (finding 2) ---

func TestExtractNonTranslatable_Preamble_DefaultOn(t *testing.T) {
	t.Parallel()
	// Row 1 is preamble, row 2 is header, row 3+ is data.
	parts := readCSVWithConfig(t, "# Generated file\nid,value\n01,Hello\n02,World\n", func(c *csvfmt.Config) {
		c.HasHeader = true
		c.ColumnNamesRow = 2
		c.ValuesStartRow = 3
		c.TranslatableColumns = []int{1}
		c.KeyColumns = []int{0}
	})

	// Translatable payload unchanged.
	assert.Equal(t, []string{"Hello", "World"}, blockTexts(collectBlocks(parts)))

	// The preamble row surfaces as a non-translatable, whitespace-preserving block.
	var preamble *model.Block
	for _, b := range nonTranslatableBlocks(parts) {
		if b.Name == "preamble-row1" {
			preamble = b
		}
	}
	require.NotNil(t, preamble, "preamble row surfaces as a non-translatable block")
	assert.False(t, preamble.Translatable)
	assert.True(t, preamble.PreserveWhitespace)
	assert.Equal(t, "# Generated file", preamble.SourceText())

	// No preamble Data part remains.
	assert.Empty(t, dataPartsNamed(parts, "preamble-row1"))
}

func TestExtractNonTranslatable_Preamble_Off_StaysData(t *testing.T) {
	t.Parallel()
	parts := readCSVWithConfig(t, "# Generated file\nid,value\n01,Hello\n02,World\n", func(c *csvfmt.Config) {
		c.HasHeader = true
		c.ColumnNamesRow = 2
		c.ValuesStartRow = 3
		c.TranslatableColumns = []int{1}
		c.KeyColumns = []int{0}
		c.SetExtractNonTranslatableContent(false)
	})

	// No preamble content block; the preamble is an opaque Data part.
	for _, b := range nonTranslatableBlocks(parts) {
		assert.NotEqual(t, "preamble-row1", b.Name, "extraction off keeps preamble as Data")
	}
	pre := dataPartsNamed(parts, "preamble-row1")
	require.Len(t, pre, 1)
	assert.Equal(t, "# Generated file", pre[0].Properties["content"])
}

func TestExtractNonTranslatable_Preamble_Skeleton_ByteExact(t *testing.T) {
	t.Parallel()
	input := "# Generated file\nid,value\n01,Hello\n02,World\n"
	output := skeletonRoundtrip(t, input, func(c *csvfmt.Config) {
		c.HasHeader = true
		c.ColumnNamesRow = 2
		c.ValuesStartRow = 3
	})
	assert.Equal(t, input, output)
}

func TestExtractNonTranslatable_Preamble_NonSkeletonRoundTrip(t *testing.T) {
	t.Parallel()
	// The non-skeleton (cross-format) writer still reconstructs the preamble
	// row from the surfaced non-translatable block.
	input := "# Comment line\nCol1,Col2\nR1C1,R1C2\n"
	output := roundTrip(t, input, func(c *csvfmt.Config) {
		c.HasHeader = true
		c.ColumnNamesRow = 2
		c.ValuesStartRow = 3
	})
	assert.Contains(t, output, "# Comment line")
	assert.Contains(t, output, "Col1,Col2")
	assert.Contains(t, output, "R1C1,R1C2")
}
