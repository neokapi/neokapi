package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	bowsync "github.com/neokapi/neokapi/bowrain/core/sync"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPushItemHashesCoverSentSubset documents and verifies the additive-only
// wire contract (#43): the ItemHashes/RootHash in the init request are computed
// over exactly the blocks the caller passed in (the changed subset), and match
// bowsync.ComputeItemHash/ComputeRootHash over that same set. They are change
// indicators, not authoritative Merkle roots — the server must treat the push
// as additive.
func TestPushItemHashesCoverSentSubset(t *testing.T) {
	blocksByItem := map[string][]*model.Block{
		"locales/en.json": {
			{ID: "b1", Translatable: true, Source: []model.Run{{Text: &model.TextRun{Text: "Hello"}}}},
			{ID: "b2", Translatable: true, Source: []model.Run{{Text: &model.TextRun{Text: "World"}}}},
		},
		"locales/de.json": {
			{ID: "b3", Translatable: true, Source: []model.Run{{Text: &model.TextRun{Text: "Hallo"}}}},
		},
	}

	// Expected hashes computed independently over the same (changed) subset.
	wantItem := map[string]string{}
	for item, blocks := range blocksByItem {
		bh := map[string]string{}
		for _, b := range blocks {
			bh[b.ID] = model.ComputeIdentity(b).ContentHash
		}
		wantItem[item] = bowsync.ComputeItemHash(bh)
	}
	wantRoot := bowsync.ComputeRootHash(wantItem)

	var gotInit PushInitRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/push/init"):
			_ = json.NewDecoder(r.Body).Decode(&gotInit)
			// Report unchanged so the test focuses on the init hashes.
			_ = json.NewEncoder(w).Encode(PushInitResponse{Status: "unchanged"})
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer srv.Close()

	c := NewClaimTokenClient(srv.URL, "proj1", "tok")
	_, err := c.Push(context.Background(), blocksByItem, nil)
	require.NoError(t, err)

	assert.Equal(t, wantItem, gotInit.ItemHashes,
		"init must send item hashes over exactly the changed subset")
	assert.Equal(t, wantRoot, gotInit.RootHash,
		"init must send the root hash over exactly the changed subset")
}

// TestPushIgnoresServerReportedDeletions verifies the non-destructive invariant
// of #43: a server that (wrongly, given partial hashes) reports DeletedItems in
// init and Deleted blocks in diff must have NO effect on the client — it never
// removes anything locally and proceeds to upload only the needed blocks. The
// only chunks uploaded are for the items/blocks the server marked needed.
func TestPushIgnoresServerReportedDeletions(t *testing.T) {
	blocksByItem := map[string][]*model.Block{
		"locales/en.json": {
			{ID: "b1", Translatable: true, Source: []model.Run{{Text: &model.TextRun{Text: "Hello"}}}},
		},
	}

	var diffRequests []PushDiffRequest
	var uploadedChunkPaths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/push/init"):
			// Bogus deletions: items the client never sent, flagged deleted.
			_ = json.NewEncoder(w).Encode(PushInitResponse{
				UploadID:     "up1",
				Status:       "diff_computed",
				NewItems:     []string{"locales/en.json"},
				DeletedItems: []string{"locales/fr.json", "locales/de.json"},
			})
		case strings.HasSuffix(r.URL.Path, "/push/diff"):
			var req PushDiffRequest
			_ = json.NewDecoder(r.Body).Decode(&req)
			diffRequests = append(diffRequests, req)
			// Bogus block deletions alongside the genuinely needed block.
			_ = json.NewEncoder(w).Encode(PushDiffResponse{
				Needed:    []string{"b1"},
				Deleted:   []string{"b-phantom-1", "b-phantom-2"},
				Transport: "proxy",
			})
		case strings.Contains(r.URL.Path, "/push/chunks/"):
			uploadedChunkPaths = append(uploadedChunkPaths, r.URL.Path)
			w.WriteHeader(http.StatusOK)
		case strings.HasSuffix(r.URL.Path, "/push/commit"):
			_ = json.NewEncoder(w).Encode(SyncPushResponse{PushID: "p1", Stored: 1})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	c := NewClaimTokenClient(srv.URL, "proj1", "tok")
	resp, err := c.Push(context.Background(), blocksByItem,
		[]ItemMeta{{Name: "locales/en.json", Format: "json"}})
	require.NoError(t, err)
	require.NotNil(t, resp)

	// The client diffed only the item it actually sent — never the phantom
	// "deleted" items.
	require.Len(t, diffRequests, 1)
	assert.Equal(t, "locales/en.json", diffRequests[0].ItemName)

	// Exactly one chunk uploaded (the single needed block). Phantom deletions
	// produced no extra requests and no destructive action.
	assert.Len(t, uploadedChunkPaths, 1,
		"server-reported deletions must not trigger any extra/destructive requests")
}
