package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	pb "github.com/neokapi/neokapi/bowrain/core/proto/sync/v1"
	bowsync "github.com/neokapi/neokapi/bowrain/core/sync"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// contextTestServer stands in for the sync endpoints, recording the init
// request and the commit manifest so a test can assert what the context content
// type actually put on the wire. contextChanged is what the server reports back
// from the fast-path negotiation.
type contextTestServer struct {
	init   PushInitRequest
	commit PushCommitRequest
}

func newContextTestServer(t *testing.T, contextChanged bool, undeclared []string) (*contextTestServer, *BowrainClient) {
	t.Helper()
	rec := &contextTestServer{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/push/init"):
			_ = json.NewDecoder(r.Body).Decode(&rec.init)
			_ = json.NewEncoder(w).Encode(PushInitResponse{
				UploadID:              "up1",
				Status:                "diff_computed",
				ContextChanged:        contextChanged,
				UndeclaredCollections: undeclared,
			})
		case strings.HasSuffix(r.URL.Path, "/push/commit"):
			_ = json.NewDecoder(r.Body).Decode(&rec.commit)
			_ = json.NewEncoder(w).Encode(SyncPushResponse{PushID: "push1"})
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	t.Cleanup(srv.Close)
	return rec, NewClaimTokenClient(srv.URL, "proj1", "tok")
}

func testContextEntries() []*pb.SyncContextEntry {
	return []*pb.SyncContextEntry{
		{
			Name:             "docs",
			Coordinates:      map[string]string{"product": "kapi"},
			Channel:          "docs",
			VoiceProfile:     "Acme Voice",
			VoiceProfileJson: []byte(`{"name":"Acme Voice"}`),
			Owner:            bowsync.ContextOwnerRecipe,
		},
		{Name: "marketing", Owner: bowsync.ContextOwnerRecipe},
	}
}

// TestPushCarriesDeclaredContext pins that a push negotiates on the context
// hash, names the collections it declares, and lands the entries in the commit
// manifest — the whole point of making context a content type rather than a
// side call.
func TestPushCarriesDeclaredContext(t *testing.T) {
	rec, c := newContextTestServer(t, true, nil)

	pushCtx := NewPushContext(testContextEntries())
	_, err := c.Push(context.Background(), nil, nil, pushCtx)
	require.NoError(t, err)

	assert.Equal(t, pushCtx.Hash, rec.init.ContextHash)
	assert.ElementsMatch(t, []string{"docs", "marketing"}, rec.init.Collections)

	require.Len(t, rec.commit.Contexts, 2)
	assert.Equal(t, "docs", rec.commit.Contexts[0].Name)
	assert.Equal(t, map[string]string{"product": "kapi"}, rec.commit.Contexts[0].Coordinates)
	assert.Equal(t, "docs", rec.commit.Contexts[0].Channel)
	assert.Equal(t, "Acme Voice", rec.commit.Contexts[0].VoiceProfile)
	assert.JSONEq(t, `{"name":"Acme Voice"}`, string(rec.commit.Contexts[0].VoiceProfileJson),
		"the authored voice travels inside the push, not through a call beside it")
	assert.Equal(t, bowsync.ContextOwnerRecipe, rec.commit.Contexts[0].Owner)
	assert.NotEmpty(t, rec.commit.Contexts[0].ContentHash,
		"each entry carries the hash the server compares against to stay idempotent")
}

// TestPushSkipsContextWhenServerReportsUnchanged pins the fast path. An
// unedited recipe negotiates on a hash and then sends nothing, so the cost of
// declaring a context on every push is one string.
func TestPushSkipsContextWhenServerReportsUnchanged(t *testing.T) {
	rec, c := newContextTestServer(t, false, nil)

	pushCtx := NewPushContext(testContextEntries())
	_, err := c.Push(context.Background(), nil, nil, pushCtx)
	require.NoError(t, err)

	assert.Equal(t, pushCtx.Hash, rec.init.ContextHash, "the hash is still negotiated")
	assert.Empty(t, rec.commit.Contexts, "an unchanged context is not re-sent")
}

// TestPushWithoutContextMakesNoClaim pins the difference between "this push
// declares no collections" and "this push says nothing about collections". A
// content-only caller sends an empty context hash, which is what stops it
// looking like a recipe that just dropped every collection it had.
func TestPushWithoutContextMakesNoClaim(t *testing.T) {
	rec, c := newContextTestServer(t, false, nil)

	_, err := c.Push(context.Background(), nil, nil, nil)
	require.NoError(t, err)

	assert.Empty(t, rec.init.ContextHash)
	assert.Empty(t, rec.init.Collections)
	assert.Empty(t, rec.commit.Contexts)
}

// TestPushReportsUndeclaredCollections pins that the server's report reaches
// the caller on both the fast path and the ordinary one — a recipe that drops a
// collection is told what the server still holds, on the same push that dropped
// it.
func TestPushReportsUndeclaredCollections(t *testing.T) {
	t.Run("after a full push", func(t *testing.T) {
		_, c := newContextTestServer(t, true, []string{"marketing"})
		resp, err := c.Push(context.Background(), nil, nil, NewPushContext(testContextEntries()))
		require.NoError(t, err)
		assert.Equal(t, []string{"marketing"}, resp.UndeclaredCollections)
	})

	t.Run("on the unchanged fast path", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(PushInitResponse{
				Status:                "unchanged",
				UndeclaredCollections: []string{"marketing"},
			})
		}))
		defer srv.Close()
		c := NewClaimTokenClient(srv.URL, "proj1", "tok")

		resp, err := c.Push(context.Background(), nil, nil, NewPushContext(testContextEntries()))
		require.NoError(t, err)
		assert.Equal(t, "unchanged", resp.PushID)
		assert.Equal(t, []string{"marketing"}, resp.UndeclaredCollections)
	})
}

// TestPushContextHashCoversGovernance pins what the fast path is sensitive to.
// The hash must move when the governance moves, or an edited voice would be
// negotiated away as "unchanged" and never reach the hub.
func TestPushContextHashCoversGovernance(t *testing.T) {
	base := NewPushContext(testContextEntries()).Hash

	cases := []struct {
		name  string
		apply func(e []*pb.SyncContextEntry)
	}{
		{"a moved coordinate", func(e []*pb.SyncContextEntry) { e[0].Coordinates["product"] = "bowrain" }},
		{"a different channel", func(e []*pb.SyncContextEntry) { e[0].Channel = "email" }},
		{"a rebound voice", func(e []*pb.SyncContextEntry) { e[0].VoiceProfile = "Other Voice" }},
		{"an edited voice", func(e []*pb.SyncContextEntry) {
			e[0].VoiceProfileJson = []byte(`{"name":"Acme Voice","description":"x"}`)
		}},
		{"a new collection", func(e []*pb.SyncContextEntry) { e[0].Name = "docs-v2" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			entries := testContextEntries()
			tc.apply(entries)
			assert.NotEqual(t, base, NewPushContext(entries).Hash)
		})
	}

	t.Run("the same recipe twice", func(t *testing.T) {
		assert.Equal(t, base, NewPushContext(testContextEntries()).Hash)
	})

	t.Run("entry order does not matter", func(t *testing.T) {
		entries := testContextEntries()
		entries[0], entries[1] = entries[1], entries[0]
		assert.Equal(t, base, NewPushContext(entries).Hash,
			"the fold is over names, so re-ordering the recipe is not a change")
	})
}

// TestPushContextIgnoresUnnamedEntries keeps a malformed entry from poisoning
// the fold: a bare recipe entry declares no collection, and the hash must be
// the same whether or not one slipped into the list.
func TestPushContextIgnoresUnnamedEntries(t *testing.T) {
	withBlank := append(testContextEntries(), &pb.SyncContextEntry{})
	assert.Equal(t, NewPushContext(testContextEntries()).Hash, NewPushContext(withBlank).Hash)
}

// TestPushContextEmptyFoldMatchesNoCollections pins the boundary the server
// compares against: a project that declares no collections must negotiate the
// same hash a server holding none folds, so it reports "unchanged" instead of
// reconciling nothing on every push.
func TestPushContextEmptyFoldMatchesNoCollections(t *testing.T) {
	assert.Equal(t, bowsync.ComputeContextHash(nil), NewPushContext(nil).Hash)
	assert.NotEmpty(t, NewPushContext(nil).Hash, "the empty fold is a real hash, not the empty string")
}

// blockFor keeps the governance-hash cases above readable by giving the
// content-carrying tests a block to push.
func blockFor(id, text string) *model.Block {
	return &model.Block{ID: id, Translatable: true, Source: []model.Run{{Text: &model.TextRun{Text: text}}}}
}

// TestPushCarriesContextAlongsideContent pins that the two content types travel
// in one push rather than in two: the same request that uploads blocks commits
// the collections they belong to.
func TestPushCarriesContextAlongsideContent(t *testing.T) {
	rec, c := newContextTestServer(t, true, nil)

	_, err := c.Push(context.Background(),
		map[string][]*model.Block{"docs/a.json": {blockFor("b1", "Hello")}},
		[]ItemMeta{{Name: "docs/a.json", Format: "json", Collection: "docs"}},
		NewPushContext(testContextEntries()))
	require.NoError(t, err)

	var items []ItemMeta
	require.NoError(t, json.Unmarshal(rec.commit.Items, &items))
	require.Len(t, items, 1)
	assert.Equal(t, "docs", items[0].Collection)
	require.Len(t, rec.commit.Contexts, 2)
}

// TestItemMetaCarriesTheCollection extends the manifest round trip to the field
// the context content type turns on: the collection the client resolved for an
// item has to reach the worker under the name the generated schema reads, or
// the item is stored ungrouped and the collection row it belongs to is never
// linked to anything.
func TestItemMetaCarriesTheCollection(t *testing.T) {
	encoded, err := json.Marshal(ItemMeta{Name: "docs/a.json", Format: "json", Collection: "docs"})
	require.NoError(t, err)

	var decoded pb.SyncItemMeta
	require.NoError(t, json.Unmarshal(encoded, &decoded))
	assert.Equal(t, "docs", decoded.Collection)
}
