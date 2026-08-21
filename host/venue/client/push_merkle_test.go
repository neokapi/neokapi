package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/venue"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPushItemHashesCoverSentSubset documents and verifies the additive-only
// wire contract (#43): the ItemHashes/RootHash in the init request are computed
// over exactly the blocks the caller passed in (the changed subset), and match
// venue.ComputeItemHash/ComputeRootHash over that same set. They are change
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
			bh[b.ID] = model.ComputeIdentity(b).RecordHash()
		}
		wantItem[item] = venue.ComputeItemHash(bh)
	}
	wantRoot := venue.ComputeRootHash(wantItem)

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
	_, err := c.Push(context.Background(), blocksByItem, nil, nil, nil)
	require.NoError(t, err)

	assert.Equal(t, wantItem, gotInit.ItemHashes,
		"init must send item hashes over exactly the changed subset")
	assert.Equal(t, wantRoot, gotInit.RootHash,
		"init must send the root hash over exactly the changed subset")
}

// The non-destructive invariant of #43, restated for a push that no longer
// negotiates: a venue that reports items or blocks as deleted must have no
// effect on the client. It uploads what it was given and nothing else.
//
// The verdicts themselves are now vestigial — a push declares a tree and a
// scope, and the venue computes the transition from those. What matters here is
// that a client hearing "these are deleted" does not act on it, because the
// party that reads a deletion is the one holding the content, not the one
// sending it.
func TestPushIgnoresServerReportedDeletions(t *testing.T) {
	blocksByItem := map[string][]*model.Block{
		"locales/en.json": {
			{ID: "b1", Translatable: true, Source: []model.Run{{Text: &model.TextRun{Text: "Hello"}}}},
		},
	}

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
		[]ItemMeta{{Name: "locales/en.json", Format: "json"}}, nil, nil)
	require.NoError(t, err)
	require.NotNil(t, resp)

	// Exactly one chunk uploaded, for the one item actually sent. Phantom
	// deletions produced no extra requests and no destructive action.
	assert.Len(t, uploadedChunkPaths, 1,
		"venue-reported deletions must not trigger any extra or destructive request")
}
