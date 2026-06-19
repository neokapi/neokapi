package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTimingAnnotationRoundTrip(t *testing.T) {
	b := &Block{ID: "b1"}
	if _, ok := b.Timing(); ok {
		t.Fatal("fresh block should have no timing")
	}
	b.SetTiming(&TimingAnnotation{StartMS: 1500, EndMS: 3200, SourceRef: "cue7"})

	got, ok := b.Timing()
	require.True(t, ok)
	assert.Equal(t, int64(1500), got.StartMS)
	assert.Equal(t, int64(3200), got.EndMS)
	assert.Equal(t, "cue7", got.SourceRef)
	assert.Equal(t, AnnoTiming, got.TypeName())
}

func TestTimingPayloadRegistered(t *testing.T) {
	p, ok := NewPayload(AnnoTiming)
	require.True(t, ok)
	_, isTiming := p.(*TimingAnnotation)
	assert.True(t, isTiming)
}

// A block can carry both anchor facets at once (on-screen text in a video frame).
func TestGeometryAndTimingCompose(t *testing.T) {
	b := &Block{ID: "b1"}
	b.SetGeometry(&GeometryAnnotation{Page: 1, BBox: Rect{X: 10, Y: 20, W: 100, H: 30}})
	b.SetTiming(&TimingAnnotation{StartMS: 500, EndMS: 900})

	g, ok := b.Geometry()
	require.True(t, ok)
	assert.Equal(t, 1, g.Page)
	tm, ok := b.Timing()
	require.True(t, ok)
	assert.Equal(t, int64(500), tm.StartMS)
}

func TestSourceOriginRoundTrip(t *testing.T) {
	b := &Block{ID: "b1"}
	if _, ok := b.SourceOrigin(); ok {
		t.Fatal("parsed block should have no source origin")
	}
	b.SetSourceOrigin(&Origin{Kind: OriginOCR, Engine: "ppocr", Confidence: 0.62})

	got, ok := b.SourceOrigin()
	require.True(t, ok)
	assert.Equal(t, OriginOCR, got.Kind)
	assert.Equal(t, "ppocr", got.Engine)
	assert.InDelta(t, 0.62, got.Confidence, 1e-9)
	assert.Equal(t, AnnoSourceOrigin, got.TypeName())
}

func TestSourceOriginPayloadRegistered(t *testing.T) {
	p, ok := NewPayload(AnnoSourceOrigin)
	require.True(t, ok)
	_, isOrigin := p.(*Origin)
	assert.True(t, isOrigin)
}
