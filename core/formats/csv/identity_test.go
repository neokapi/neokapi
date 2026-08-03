package csv_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	csvfmt "github.com/neokapi/neokapi/core/formats/csv"
	"github.com/neokapi/neokapi/core/model"
)

// namesByText maps each translatable cell's text to the name the reader gave it.
func namesByText(t *testing.T, input string, cfgFn func(*csvfmt.Config)) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, b := range collectBlocks(readCSVWithConfig(t, input, cfgFn)) {
		out[b.SourceText()] = b.Name
	}
	return out
}

// A row addressed by its key column keeps its name when a row above it goes.
//
// This is the whole point of naming a cell after the row's key: the deletion is
// unrelated to the surviving rows, so nothing about them was re-decided, and a
// review decision or a memory match recorded against them still names them.
func TestCSVIdentity_KeyColumnSurvivesDeletion(t *testing.T) {
	t.Parallel()

	const v1 = "id,text\nalpha,Alpha\nbravo,Bravo\ncharlie,Charlie\n"
	const v2 = "id,text\nalpha,Alpha\ncharlie,Charlie\n"

	for _, tc := range []struct {
		name  string
		cfgFn func(*csvfmt.Config)
	}{
		{"configured key column", func(c *csvfmt.Config) {
			c.KeyColumns = []int{0}
			c.TranslatableColumns = []int{1}
		}},
		// No configuration at all: the header labels column 0 `id` and its values
		// are unique, so it addresses the row.
		{"inferred key column", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			before := namesByText(t, v1, tc.cfgFn)
			after := namesByText(t, v2, tc.cfgFn)

			require.Contains(t, before, "Charlie")
			require.Contains(t, after, "Charlie")
			assert.Equal(t, before["Charlie"], after["Charlie"],
				"a row after an unrelated deletion keeps its address")
			assert.Contains(t, before["Charlie"], "charlie",
				"the name is built from the row's key, not its position")
		})
	}
}

// Two rows carrying the same text are still two different cells.
func TestCSVIdentity_IdenticalTextGetsDistinctNames(t *testing.T) {
	t.Parallel()

	names := map[string]bool{}
	for _, b := range collectBlocks(readCSV(t, "id,text\na,Same\nb,Same\nc,Same\n")) {
		if b.SourceText() != "Same" {
			continue
		}
		assert.False(t, names[b.Name], "duplicate block name %q", b.Name)
		names[b.Name] = true
	}
	assert.Len(t, names, 3)
}

// Reading the same file twice produces the same names.
func TestCSVIdentity_StableAcrossTwoReads(t *testing.T) {
	t.Parallel()

	const input = "id,en\ngreeting,Hello\nfarewell,Goodbye\n"
	assert.Equal(t, namesByText(t, input, nil), namesByText(t, input, nil))
}

// A duplicated key is not an address, so the column is not treated as one: the
// name falls back to the row ordinal rather than silently giving two rows the
// same address.
func TestCSVIdentity_RepeatedKeyIsNotInferred(t *testing.T) {
	t.Parallel()

	blocks := collectBlocks(readCSVWithConfig(t, "id,text\nx,Alpha\nx,Bravo\n", func(c *csvfmt.Config) {
		c.TranslatableColumns = []int{1}
	}))
	require.Len(t, blocks, 2)
	assert.Equal(t, "text/row1", blocks[0].Name)
	assert.Equal(t, "text/row2", blocks[1].Name)
}

// A column whose header does not declare it a key is never inferred as one,
// however unique its values are. Inferring from a content column would make the
// name content-derived, which is what core/model/structural.go rules out.
func TestCSVIdentity_ContentColumnIsNotInferred(t *testing.T) {
	t.Parallel()

	blocks := collectBlocks(readCSV(t, "en,de\nHello,Hallo\nGoodbye,Tschuss\n"))
	require.NotEmpty(t, blocks)
	assert.Equal(t, "en/row1", blocks[0].Name)
}

// No header row means no column labels and no key: the name is positional, and
// says so.
func TestCSVIdentity_HeaderlessFallsBackToPosition(t *testing.T) {
	t.Parallel()

	blocks := collectBlocks(readCSVWithConfig(t, "Alpha,Bravo\nCharlie,Delta\n", func(c *csvfmt.Config) {
		c.HasHeader = false
	}))
	require.Len(t, blocks, 4)
	assert.Equal(t, "col0/row1", blocks[0].Name)
	assert.Equal(t, "col1/row2", blocks[3].Name)
}

// A bilingual table's unit is the row, so the row's key names it.
func TestCSVIdentity_BilingualRowNamedByKey(t *testing.T) {
	t.Parallel()

	read := func(input string) map[string]string {
		return namesByText(t, input, func(c *csvfmt.Config) {
			c.Separator = '\t'
			c.SourceColumn = 1
			c.TargetColumn = 2
		})
	}
	before := read("id\tsource\ttarget\ng1\tHello\tHallo\ng2\tBye\tTschuss\ng3\tYes\tJa\n")
	after := read("id\tsource\ttarget\ng1\tHello\tHallo\ng3\tYes\tJa\n")

	require.Equal(t, "g3", before["Yes"], "the row's key addresses it")
	assert.Equal(t, before["Yes"], after["Yes"],
		"deleting an earlier row does not rename a later one")
}

// The name follows the key, so the writer cannot be joining on it. This pins
// that: a keyed table still round-trips byte-exact on the non-skeleton path.
func TestCSVIdentity_KeyedTableStillRoundTrips(t *testing.T) {
	t.Parallel()

	const input = "id,text\nalpha,Alpha\nbravo,Bravo\n"
	assert.Equal(t, input, roundTrip(t, input, nil))
}

// The structural helpers are shared, so a name is a path, not an ad-hoc string.
func TestCSVIdentity_NameIsAStructuralPath(t *testing.T) {
	t.Parallel()

	blocks := collectBlocks(readCSVWithConfig(t, "id,text\nalpha,Alpha\n", func(c *csvfmt.Config) {
		c.TranslatableColumns = []int{1}
	}))
	require.Len(t, blocks, 1)
	assert.Equal(t, model.StructuralPath("alpha", "text"), blocks[0].Name)
}
