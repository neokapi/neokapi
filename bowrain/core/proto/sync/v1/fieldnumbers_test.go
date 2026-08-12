package syncv1

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"google.golang.org/protobuf/proto"
)

// TestFieldNumbersAreAdditiveOnly pins every field number on the messages the
// freshness ref touched.
//
// Proto wire encoding depends on field NUMBERS and nothing else, so a
// renumbering compiles, passes every unit test, and silently decodes one field
// as another on the far side. Deployed clients are the far side: rc18 and rc21
// binaries talk to this server nightly, and additive-only is the whole of what
// keeps them working. That contract is checkable, so it is checked here rather
// than trusted to review.
func TestFieldNumbersAreAdditiveOnly(t *testing.T) {
	want := map[string]map[int32]string{
		"SyncPushInit": {
			1: "project_id", 2: "stream", 3: "content_types", 4: "item_hashes",
			5: "root_hash", 6: "terms_hash", 7: "memory_hash", 8: "context_hash",
			9: "collections",
		},
		"SyncPushInitResponse": {
			1: "upload_id", 2: "status", 3: "changed_items", 4: "deleted_items",
			5: "new_items", 6: "terms_changed", 7: "memory_changed",
			8: "unchanged_item_count", 9: "context_changed", 10: "undeclared_collections",
			11: "ref",
		},
		"SyncManifest": {
			1: "upload_id", 2: "project_id", 3: "stream", 4: "chunks", 5: "items",
			6: "actor_id", 7: "workspace_slug", 8: "connector_id", 9: "context",
			10: "contexts", 11: "expected_ref",
		},
		"SyncPullResponse": {
			1: "cursor", 2: "has_more", 10: "blocks", 11: "terms", 12: "memory_entries",
			13: "media", 14: "contexts", 15: "ref",
		},
		"SyncRef": {1: "content", 2: "context", 3: "terms", 4: "decisions"},
	}

	messages := map[string]proto.Message{
		"SyncPushInit":         &SyncPushInit{},
		"SyncPushInitResponse": &SyncPushInitResponse{},
		"SyncManifest":         &SyncManifest{},
		"SyncPullResponse":     &SyncPullResponse{},
		"SyncRef":              &SyncRef{},
	}

	for name, expected := range want {
		t.Run(name, func(t *testing.T) {
			fields := messages[name].ProtoReflect().Descriptor().Fields()
			got := make(map[int32]string, fields.Len())
			for i := range fields.Len() {
				f := fields.Get(i)
				got[int32(f.Number())] = string(f.Name())
			}
			assert.Equal(t, expected, got)
		})
	}
}
