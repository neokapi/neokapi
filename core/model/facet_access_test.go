package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Block-scoped facet access (the former annotation map) ---

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

	// AnnoMap reflects block-scoped facets, excluding positional ones.
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

func TestAnnoMap_ExcludesPositionalFacets(t *testing.T) {
	t.Parallel()
	b := NewBlock("b1", "hello world")
	// A positional facet (segmentation) must not appear in AnnoMap.
	b.SetSegmentation(nil, []Span{{ID: "s1", Range: RunRange{StartRun: 0, EndRun: 1}}})
	b.SetAnno("note", &NoteAnnotation{Text: "hi"})

	m := b.AnnoMap()
	assert.Len(t, m, 1)
	_, hasNote := m["note"]
	assert.True(t, hasNote)
	_, hasSeg := m["segmentation"]
	assert.False(t, hasSeg)
}

// --- Positional facet span CRUD (term/entity/term-candidate) ---

func TestFacetSpanCRUD(t *testing.T) {
	t.Parallel()
	b := NewBlock("b1", "John Smith visited Paris")

	// No entity facet initially.
	assert.Nil(t, b.FacetOf(FacetEntity))
	assert.Nil(t, b.FacetSpan(FacetEntity, "entity:0"))

	b.AddFacetSpan(FacetEntity, Span{
		ID:    "entity:0",
		Range: RunRangeForBytes(b.Source, 0, 10),
		Value: &EntityAnnotation{Text: "John Smith", Type: EntityPerson},
	})
	b.AddFacetSpan(FacetEntity, Span{
		ID:    "entity:1",
		Range: RunRangeForBytes(b.Source, 19, 24),
		Value: &EntityAnnotation{Text: "Paris", Type: EntityLocation},
	})

	// Both spans merge into one entity facet.
	f := b.FacetOf(FacetEntity)
	require.NotNil(t, f)
	assert.Len(t, f.Spans, 1+1)

	// Lookup by ID; the span carries position (Range) and payload (Value).
	s := b.FacetSpan(FacetEntity, "entity:1")
	require.NotNil(t, s)
	ea := s.Value.(*EntityAnnotation)
	assert.Equal(t, "Paris", ea.Text)
	start, end := s.Range.ByteSpan(b.Source)
	assert.Equal(t, 19, start)
	assert.Equal(t, 24, end)

	// In-place mutation via the returned pointer.
	ea.DNT = true
	assert.True(t, b.FacetSpan(FacetEntity, "entity:1").Value.(*EntityAnnotation).DNT)

	// Removal.
	assert.True(t, b.RemoveFacetSpan(FacetEntity, "entity:0"))
	assert.False(t, b.RemoveFacetSpan(FacetEntity, "entity:0")) // already gone
	assert.Nil(t, b.FacetSpan(FacetEntity, "entity:0"))
	assert.Len(t, b.FacetOf(FacetEntity).Spans, 1)

	// Positional facets are not block-scoped — never appear in AnnoMap.
	assert.Empty(t, b.AnnoMap())
}

func TestFacetTermCandidateIsPositional(t *testing.T) {
	t.Parallel()
	// term-candidate is registered positional so its spans stay out of AnnoMap.
	assert.True(t, FacetTermCandidate.IsPositional())
	assert.True(t, FacetEntity.IsPositional())
	assert.True(t, FacetTerm.IsPositional())
	assert.False(t, FacetNote.IsPositional())
	assert.False(t, FacetWordCount.IsPositional())
}

func TestRegisterPositionalFacet(t *testing.T) {
	t.Parallel()
	const custom FacetType = "test-custom-positional"
	assert.False(t, custom.IsPositional())
	RegisterPositionalFacet(custom)
	assert.True(t, custom.IsPositional())
}
