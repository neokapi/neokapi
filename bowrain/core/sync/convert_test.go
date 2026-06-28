package sync

import (
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBlockRoundTrip(t *testing.T) {
	b := &model.Block{
		ID:                 "b1",
		Name:               "greeting",
		Type:               "text",
		MimeType:           "text/plain",
		Translatable:       true,
		Properties:         map[string]string{"context": "homepage"},
		PreserveWhitespace: true,
	}
	b.SetSourceText("Hello world")

	// Convert to proto.
	pb := BlockToProto(b, "en.json")
	assert.Equal(t, "b1", pb.Id)
	assert.Equal(t, "en.json", pb.ItemName)
	assert.Equal(t, "Hello world", pb.SourceText)
	assert.Equal(t, "text", pb.Type)
	assert.True(t, pb.Translatable)
	assert.True(t, pb.PreserveWhitespace)
	assert.NotEmpty(t, pb.ContentHash)
	assert.Equal(t, "homepage", pb.Properties["context"])

	// Convert back.
	b2, err := ProtoToBlock(pb)
	require.NoError(t, err)
	assert.Equal(t, "b1", b2.ID)
	assert.Equal(t, "greeting", b2.Name)
	assert.Equal(t, "Hello world", b2.SourceText())
	assert.True(t, b2.Translatable)
	assert.True(t, b2.PreserveWhitespace)
	assert.Equal(t, "homepage", b2.Properties["context"])
}

func TestBlockWithTargets(t *testing.T) {
	b := &model.Block{ID: "b1", Translatable: true}
	b.SetSourceText("Hello")
	b.SetTargetText("fr", "Bonjour")
	b.SetTargetText("de", "Hallo")

	pb := BlockToProto(b, "en.json")
	require.NotNil(t, pb.Targets)
	assert.Len(t, pb.Targets, 2)
	require.Len(t, pb.Targets["fr"].Segments, 1)
	require.Len(t, pb.Targets["fr"].Segments[0].Runs, 1)
	assert.Equal(t, "Bonjour", pb.Targets["fr"].Segments[0].Runs[0].GetText().GetText())

	b2, err := ProtoToBlock(pb)
	require.NoError(t, err)
	assert.Equal(t, "Bonjour", b2.TargetText("fr"))
	assert.Equal(t, "Hallo", b2.TargetText("de"))
}

// TestBlockTargetStatusRoundTrip verifies the target's lifecycle status survives
// the sync round-trip (carried on the segment, so coverage/ship gates see review
// state pulled back from the server) — the bowrain transport carrier.
func TestBlockTargetStatusRoundTrip(t *testing.T) {
	b := &model.Block{ID: "b1", Translatable: true}
	b.SetSourceText("Hello")
	b.SetTargetText("fr", "Bonjour")
	b.StampTargetProvenance("fr", model.TargetStatusReviewed, model.Origin{Kind: model.OriginHuman})

	b2, err := ProtoToBlock(BlockToProto(b, "en.json"))
	require.NoError(t, err)
	tgt := b2.Target("fr")
	require.NotNil(t, tgt)
	assert.Equal(t, "Bonjour", b2.TargetText("fr"))
	assert.Equal(t, model.TargetStatusReviewed, tgt.Status, "lifecycle status survives sync")
}

func TestBlockWithAnnotations(t *testing.T) {
	b := &model.Block{ID: "b1"}
	b.SetSourceText("Test")
	b.AddNote(&model.NoteAnnotation{Text: "translator note", From: "dev"})

	pb := BlockToProto(b, "en.json")
	assert.NotEmpty(t, pb.AnnotationsJson)

	b2, err := ProtoToBlock(pb)
	require.NoError(t, err)
	assert.NotEmpty(t, b2.AnnoMap())
}

func TestComputeItemHash_Deterministic(t *testing.T) {
	hashes := map[string]string{
		"b1": "hash1",
		"b2": "hash2",
		"b3": "hash3",
	}

	h1 := ComputeItemHash(hashes)
	h2 := ComputeItemHash(hashes)
	assert.Equal(t, h1, h2, "same input should produce same hash")
	assert.Len(t, h1, 64, "SHA-256 hex = 64 chars")
}

func TestComputeItemHash_DifferentContent(t *testing.T) {
	h1 := ComputeItemHash(map[string]string{"b1": "hash1"})
	h2 := ComputeItemHash(map[string]string{"b1": "hash2"})
	assert.NotEqual(t, h1, h2)
}

func TestComputeRootHash_Deterministic(t *testing.T) {
	items := map[string]string{
		"en.json":     "item-hash-1",
		"messages.po": "item-hash-2",
	}

	h1 := ComputeRootHash(items)
	h2 := ComputeRootHash(items)
	assert.Equal(t, h1, h2)
}

func TestComputeRootHash_DifferentItems(t *testing.T) {
	h1 := ComputeRootHash(map[string]string{"en.json": "hash1"})
	h2 := ComputeRootHash(map[string]string{"en.json": "hash2"})
	assert.NotEqual(t, h1, h2)
}
