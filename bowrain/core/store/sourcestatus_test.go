package store

import (
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPropsForStore_RoundTrip(t *testing.T) {
	b := &model.Block{Properties: map[string]string{"k": "v"}}
	b.SetEditionStatus(model.EditionKey{}, model.Status(model.SourceStatusWritten))
	props := PropsForStore(b)
	assert.Equal(t, "v", props["k"], "existing properties are preserved")
	assert.Equal(t, "written", props[PropSourceStatus], "the status is folded in")
	// The caller's own map is never mutated.
	_, leaked := b.Properties[PropSourceStatus]
	assert.False(t, leaked, "copy-on-write: the block's map is untouched")

	// Read side: lift it back onto the block and strip the reserved key.
	scanned := model.NewBlock("b1", "Hello")
	scanned.Properties = map[string]string{"k": "v", PropSourceStatus: "established"}
	// An origin annotation this process could not decode is kept as it was
	// read, like every other annotation.
	origin := &model.RawAnnotation{Kind: model.AnnoSourceOrigin, Body: []byte(`{"kind":`)}
	scanned.SetAnno(model.AnnoSourceOrigin, origin)
	ApplySourceStatusFromProps(scanned)
	src, _ := scanned.Edition(model.EditionKey{})
	assert.Equal(t, model.Status(model.SourceStatusEstablished), src.Status)
	_, stillThere := scanned.Properties[PropSourceStatus]
	assert.False(t, stillThere, "the reserved key is stripped on read")
	assert.Equal(t, "v", scanned.Properties["k"])
	kept, ok := scanned.Anno(model.AnnoSourceOrigin)
	require.True(t, ok, "lifting the status leaves the source origin alone")
	assert.Same(t, origin, kept)
	// Lifting the status leaves the content alone: the block still reads as
	// its reader produced it.
	runs, edited := scanned.SourceAsRead()
	assert.False(t, edited, "a status is not an edit of the source")
	assert.Equal(t, "Hello", model.RunsText(runs))
	// Delivery copies a stored block and puts a translation where the source
	// was read (bowrain/server/forge.go); a writer must read that copy as
	// unedited, as it does for a block that carries no status.
	delivered := *scanned
	delivered.SetSourceRuns([]model.Run{model.TextR("Bonjour")})
	runs, edited = delivered.SourceAsRead()
	assert.False(t, edited, "the lifted status kept no earlier source as read")
	assert.Equal(t, "Bonjour", model.RunsText(runs))
}

func TestPropsForStore_NoStatus(t *testing.T) {
	b := &model.Block{Properties: map[string]string{"k": "v"}}
	// No status → properties returned unchanged (same map).
	assert.Equal(t, b.Properties, PropsForStore(b))
}

func TestTranslateAfterFor(t *testing.T) {
	assert.Equal(t, model.TranslateAfterWritten, TranslateAfterFor(nil), "nil project → default")
	assert.Equal(t, model.TranslateAfterWritten, TranslateAfterFor(&Project{}), "unset → default")
	assert.Equal(t, model.TranslateAfterNone,
		TranslateAfterFor(&Project{Properties: map[string]string{TranslateAfterProperty: "none"}}))
	assert.Equal(t, model.TranslateAfterEstablished,
		TranslateAfterFor(&Project{Properties: map[string]string{TranslateAfterProperty: "established"}}))
}
