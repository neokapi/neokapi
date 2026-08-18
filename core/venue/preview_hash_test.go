package venue

import (
	"testing"

	pb "github.com/neokapi/neokapi/core/proto/sync/v1"
	"github.com/stretchr/testify/assert"
)

// Re-pointing a collection at a different host is a recipe change like any
// other, so it has to move the entry hash. Left out of the fold, the edit would
// sit behind an unchanged hash and the server would keep serving the old URL
// forever — the reconcile is skipped precisely when the hash matches.
func TestContextEntryHash_MovesWithThePreviewHost(t *testing.T) {
	base := &pb.SyncContextEntry{Name: "bowrain-app", Channel: "app"}

	none := ComputeContextEntryHash(base)
	storybook := ComputeContextEntryHash(&pb.SyncContextEntry{
		Name: "bowrain-app", Channel: "app",
		Preview: &pb.SyncPreviewSource{Kind: "storybook", Url: "https://example.dev/sb/"},
	})
	moved := ComputeContextEntryHash(&pb.SyncContextEntry{
		Name: "bowrain-app", Channel: "app",
		Preview: &pb.SyncPreviewSource{Kind: "storybook", Url: "https://example.dev/other/"},
	})
	rekinded := ComputeContextEntryHash(&pb.SyncContextEntry{
		Name: "bowrain-app", Channel: "app",
		Preview: &pb.SyncPreviewSource{Kind: "ladle", Url: "https://example.dev/sb/"},
	})

	assert.NotEqual(t, none, storybook, "declaring a host is a change")
	assert.NotEqual(t, storybook, moved, "moving the host is a change")
	assert.NotEqual(t, storybook, rekinded, "changing how it is read is a change")
}

// An entry that declares no host hashes the same as one whose preview is an
// empty message, so a client that sends the field unset and one that sends it
// empty do not churn each other's rows.
func TestContextEntryHash_AnEmptyPreviewIsNoPreview(t *testing.T) {
	assert.Equal(t,
		ComputeContextEntryHash(&pb.SyncContextEntry{Name: "docs"}),
		ComputeContextEntryHash(&pb.SyncContextEntry{Name: "docs", Preview: &pb.SyncPreviewSource{}}))
}
