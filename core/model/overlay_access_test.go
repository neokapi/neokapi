package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Block-scoped annotation access ---

func TestBlockAnno_SetGetDelete(t *testing.T) {
	t.Parallel()
	b := NewBlock("b1", "hello")

	if _, ok := b.Anno("note"); ok {
		t.Fatal("expected no note initially")
	}

	note := &NoteAnnotation{Text: "hi"}
	b.SetAnno("note", note)

	got, ok := b.Anno("note")
	require.True(t, ok)
	assert.Same(t, note, got)
	assert.Len(t, b.AnnoMap(), 1)

	// SetAnno upserts in place.
	b.SetAnno("note", &NoteAnnotation{Text: "bye"})
	got, _ = b.Anno("note")
	assert.Equal(t, "bye", got.(*NoteAnnotation).Text)
	assert.Len(t, b.AnnoMap(), 1)

	b.DelAnno("note")
	_, ok = b.Anno("note")
	assert.False(t, ok)
	assert.Empty(t, b.AnnoMap())
}

func TestAnnoAs_TypedLookup(t *testing.T) {
	t.Parallel()
	b := NewBlock("b1", "hello")
	b.SetAnno("note", &NoteAnnotation{Text: "hi"})

	n, ok := AnnoAs[*NoteAnnotation](b, "note")
	require.True(t, ok)
	assert.Equal(t, "hi", n.Text)

	// Wrong type → not ok.
	_, ok = AnnoAs[*AltTranslation](b, "note")
	assert.False(t, ok)

	// Absent key → not ok.
	_, ok = AnnoAs[*NoteAnnotation](b, "missing")
	assert.False(t, ok)
}

func TestAnnotationsAndOverlaysAreSeparate(t *testing.T) {
	t.Parallel()
	b := NewBlock("b1", "hello world")
	// A positional overlay and a block annotation live in distinct carriers.
	b.SetSegmentation(nil, []Span{{ID: "s1", Range: RunRange{StartRun: 0, EndRun: 1}}})
	b.SetAnno("note", &NoteAnnotation{Text: "hi"})

	assert.Len(t, b.AnnoMap(), 1, "annotations carry the note only")
	_, hasNote := b.AnnoMap()["note"]
	assert.True(t, hasNote)
	assert.NotNil(t, b.OverlayOf(OverlaySegmentation), "segmentation is an overlay, not an annotation")
	_, segIsAnno := b.AnnoMap()["segmentation"]
	assert.False(t, segIsAnno)
}

// --- Positional overlay span CRUD (segmentation/term/entity/…) ---

func TestOverlaySpanCRUD(t *testing.T) {
	t.Parallel()
	b := NewBlock("b1", "John Smith visited Paris")

	// No entity overlay initially.
	assert.Nil(t, b.OverlayOf(OverlayEntity))
	assert.Nil(t, b.OverlaySpan(OverlayEntity, "entity:0"))

	b.AddOverlaySpan(OverlayEntity, Span{
		ID:    "entity:0",
		Range: RunRangeForBytes(b.Source, 0, 10),
		Value: &EntityAnnotation{Text: "John Smith", Type: EntityPerson},
	})
	b.AddOverlaySpan(OverlayEntity, Span{
		ID:    "entity:1",
		Range: RunRangeForBytes(b.Source, 19, 24),
		Value: &EntityAnnotation{Text: "Paris", Type: EntityLocation},
	})

	// Both spans merge into one entity overlay.
	o := b.OverlayOf(OverlayEntity)
	require.NotNil(t, o)
	assert.Len(t, o.Spans, 2)

	// Lookup by ID; the span carries position (Range) and payload (Value).
	s := b.OverlaySpan(OverlayEntity, "entity:1")
	require.NotNil(t, s)
	ea := s.Value.(*EntityAnnotation)
	assert.Equal(t, "Paris", ea.Text)
	start, end := s.Range.ByteSpan(b.Source)
	assert.Equal(t, 19, start)
	assert.Equal(t, 24, end)

	// In-place mutation via the returned pointer.
	ea.DNT = true
	assert.True(t, b.OverlaySpan(OverlayEntity, "entity:1").Value.(*EntityAnnotation).DNT)

	// Span removal.
	assert.True(t, b.RemoveOverlaySpan(OverlayEntity, "entity:0"))
	assert.False(t, b.RemoveOverlaySpan(OverlayEntity, "entity:0")) // already gone
	assert.Nil(t, b.OverlaySpan(OverlayEntity, "entity:0"))
	assert.Len(t, b.OverlayOf(OverlayEntity).Spans, 1)

	// Whole-overlay removal.
	b.RemoveOverlay(OverlayEntity)
	assert.Nil(t, b.OverlayOf(OverlayEntity))

	// Overlays never leak into the annotation map.
	assert.Empty(t, b.AnnoMap())
}
