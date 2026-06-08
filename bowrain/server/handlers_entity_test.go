package server

import (
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// nextFacetSpanIndex assigns the stable "entity:N" / "term-candidate:N"
// identities the entity CRUD handlers hand back to clients. These tests pin its
// behaviour (the part of the entity-editing rework most likely to regress)
// without needing the HTTP/store/auth scaffolding.

func TestNextFacetSpanIndex_Empty(t *testing.T) {
	t.Parallel()
	b := model.NewBlock("b1", "hello")
	assert.Equal(t, 0, nextFacetSpanIndex(b, model.FacetEntity, "entity:"))
}

func TestNextFacetSpanIndex_Sequential(t *testing.T) {
	t.Parallel()
	b := model.NewBlock("b1", "John visited Paris")
	b.AddFacetSpan(model.FacetEntity, model.Span{ID: "entity:0", Value: &model.EntityAnnotation{Text: "John"}})
	b.AddFacetSpan(model.FacetEntity, model.Span{ID: "entity:1", Value: &model.EntityAnnotation{Text: "Paris"}})
	assert.Equal(t, 2, nextFacetSpanIndex(b, model.FacetEntity, "entity:"))
}

func TestNextFacetSpanIndex_AfterDeleteUsesMaxPlusOne(t *testing.T) {
	t.Parallel()
	b := model.NewBlock("b1", "John visited Paris today")
	b.AddFacetSpan(model.FacetEntity, model.Span{ID: "entity:0", Value: &model.EntityAnnotation{Text: "John"}})
	b.AddFacetSpan(model.FacetEntity, model.Span{ID: "entity:1", Value: &model.EntityAnnotation{Text: "Paris"}})
	b.AddFacetSpan(model.FacetEntity, model.Span{ID: "entity:2", Value: &model.EntityAnnotation{Text: "today"}})
	// Delete the middle one: the next index is still max+1, so identities never
	// collide with a surviving span.
	require.True(t, b.RemoveFacetSpan(model.FacetEntity, "entity:1"))
	assert.Equal(t, 3, nextFacetSpanIndex(b, model.FacetEntity, "entity:"))
}

func TestNextFacetSpanIndex_PerFacetType(t *testing.T) {
	t.Parallel()
	b := model.NewBlock("b1", "John visited Paris")
	b.AddFacetSpan(model.FacetEntity, model.Span{ID: "entity:0", Value: &model.EntityAnnotation{Text: "John"}})
	// Term-candidate indices are tracked independently of entity indices.
	assert.Equal(t, 0, nextFacetSpanIndex(b, model.FacetTermCandidate, "term-candidate:"))
	b.AddFacetSpan(model.FacetTermCandidate, model.Span{ID: "term-candidate:0", Value: &model.TermCandidateAnnotation{Text: "Sprint"}})
	assert.Equal(t, 1, nextFacetSpanIndex(b, model.FacetTermCandidate, "term-candidate:"))
	// Entity index unaffected.
	assert.Equal(t, 1, nextFacetSpanIndex(b, model.FacetEntity, "entity:"))
}
