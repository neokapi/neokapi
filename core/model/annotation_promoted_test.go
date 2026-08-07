package model_test

// Structure and geometry are annotations by contract and fields by storage.
// Every accessor has to present them exactly as if they sat in the map, because
// the wire, the stores and the parse cache all serialize whatever AnnoMap gives
// them — a promoted annotation that the map does not report is one that does
// not survive a round trip.

import (
	"maps"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPromotedAnnotationsReadBackLikeMapEntries(t *testing.T) {
	b := &model.Block{ID: "b1"}
	b.SetSemanticRole(model.RoleTableCell, 0)
	b.SetGeometry(&model.GeometryAnnotation{Page: 2, Origin: "cell-grid"})
	b.AddNote(&model.NoteAnnotation{Text: "a note"})

	t.Run("Anno", func(t *testing.T) {
		v, ok := b.Anno(model.AnnoStructure)
		require.True(t, ok)
		assert.Equal(t, model.RoleTableCell, v.(*model.StructureAnnotation).Role)

		v, ok = b.Anno(model.AnnoGeometry)
		require.True(t, ok)
		assert.Equal(t, 2, v.(*model.GeometryAnnotation).Page)
	})

	t.Run("AnnoMap carries all three", func(t *testing.T) {
		am := b.AnnoMap()
		assert.Len(t, am, 3)
		assert.Contains(t, am, model.AnnoStructure)
		assert.Contains(t, am, model.AnnoGeometry)
		assert.Contains(t, am, model.AnnoNote)
	})

	t.Run("Annos yields the same set", func(t *testing.T) {
		got := maps.Collect(b.Annos())
		assert.Equal(t, b.AnnoMap(), got)
	})

	t.Run("DelAnno removes them", func(t *testing.T) {
		c := &model.Block{ID: "c1"}
		c.SetSemanticRole(model.RoleHeading, 1)
		c.SetGeometry(&model.GeometryAnnotation{Page: 1})
		c.DelAnno(model.AnnoStructure)
		c.DelAnno(model.AnnoGeometry)

		_, ok := c.Structure()
		assert.False(t, ok)
		_, ok = c.Geometry()
		assert.False(t, ok)
		assert.Empty(t, c.AnnoMap())
	})
}

// AnnoMap used to hand out the block's own storage. It now merges the promoted
// fields in, so it is a snapshot — a caller that writes to it is not writing to
// the block, and SetAnno is the mutation path.
func TestAnnoMapIsASnapshot(t *testing.T) {
	b := &model.Block{ID: "b1"}
	b.SetSemanticRole(model.RoleParagraph, 0)
	b.AddNote(&model.NoteAnnotation{Text: "a note"})

	am := b.AnnoMap()
	delete(am, model.AnnoStructure)
	am["invented"] = &model.Notes{}

	_, ok := b.Structure()
	assert.True(t, ok, "deleting from the snapshot must not delete from the block")
	_, ok = b.Anno("invented")
	assert.False(t, ok, "adding to the snapshot must not add to the block")
}

// A block with no promoted annotation returns its map untouched, so the common
// case allocates nothing.
func TestAnnoMapAllocatesNothingWithoutPromotedAnnotations(t *testing.T) {
	b := &model.Block{ID: "b1"}
	b.AddNote(&model.NoteAnnotation{Text: "a note"})
	assert.Len(t, b.AnnoMap(), 1)
}

// The promoted keys are typed, and a caller that stores something else under
// one still gets it back — the field is an optimisation, not a schema.
func TestUnexpectedTypeUnderAPromotedKeyStillRoundTrips(t *testing.T) {
	b := &model.Block{ID: "b1"}
	b.SetAnno(model.AnnoStructure, &model.Notes{Items: []*model.NoteAnnotation{{Text: "odd"}}})

	v, ok := b.Anno(model.AnnoStructure)
	require.True(t, ok)
	notes, ok := v.(*model.Notes)
	require.True(t, ok, "an unexpected payload must come back as it went in")
	assert.Equal(t, "odd", notes.Items[0].Text)
	assert.Contains(t, b.AnnoMap(), model.AnnoStructure)
}
