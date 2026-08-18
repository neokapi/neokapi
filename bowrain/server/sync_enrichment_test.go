package server

import (
	"context"
	"net/http/httptest"
	"testing"

	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/core/model"
	apiclient "github.com/neokapi/neokapi/host/venue/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The whole delivery path, real client against real server: content pushed by a
// kapi that recorded nothing about a block acquires the property a later kapi
// records, on an ordinary push, with the source text untouched and nobody
// forcing anything.
//
// This is the case that made `--force` the only remedy, and the one a
// hash-level unit test cannot prove on its own: it needs the client's own
// negotiation, the server's diff, and the worker that stores what arrives.
func TestSyncPush_ARecordedPropertyReachesContentAlreadyPushed(t *testing.T) {
	srv, token := newTestServer(t)
	pid := createProject(t, srv, token)

	http := httptest.NewServer(srv.GetEcho())
	defer http.Close()
	client := apiclient.NewProjectBearerClient(http.URL, pid, token)
	ctx := context.Background()

	// The push a released kapi made: a JSX string, no properties recorded.
	bare := &model.Block{ID: "b1", Translatable: true, Name: "greeting"}
	bare.SetSourceText("Hello")
	first, err := client.Push(ctx, map[string][]*model.Block{
		"src/App.jsx": {bare},
	}, []apiclient.ItemMeta{{Name: "src/App.jsx", Format: "jsx"}}, nil, nil)
	require.NoError(t, err)
	require.Equal(t, 1, first.BlocksUploaded)
	drainPushQueue(t, srv)

	// The same file, unedited, read by a kapi whose JSX reader now records the
	// runtime catalog key the reviewer needs to render the string in place.
	enriched := &model.Block{ID: "b1", Translatable: true, Name: "greeting",
		Properties: map[string]string{"hash": "k1_9c2f"}}
	enriched.SetSourceText("Hello")
	require.Equal(t,
		model.ComputeIdentity(bare).ContentHash,
		model.ComputeIdentity(enriched).ContentHash,
		"the text is identical, which is why a content-hash diff reports nothing to do")

	second, err := client.Push(ctx, map[string][]*model.Block{
		"src/App.jsx": {enriched},
	}, []apiclient.ItemMeta{{Name: "src/App.jsx", Format: "jsx"}}, nil, nil)
	require.NoError(t, err)
	assert.Equal(t, 1, second.BlocksUploaded, "the block is asked for, once")
	drainPushQueue(t, srv)

	stored, err := srv.ContentStore.GetBlocks(t.Context(), platstore.BlockQuery{
		ProjectID: pid, Stream: "main", ItemName: "src/App.jsx",
	})
	require.NoError(t, err)
	require.Len(t, stored, 1)
	assert.Equal(t, "k1_9c2f", stored[0].Block.Properties["hash"],
		"the reviewer's join key reached content that was pushed before it existed")

	// And the push after that is quiet: the re-upload happens once per
	// enrichment, not on every push from then on.
	again := &model.Block{ID: "b1", Translatable: true, Name: "greeting",
		Properties: map[string]string{"hash": "k1_9c2f"}}
	again.SetSourceText("Hello")
	third, err := client.Push(ctx, map[string][]*model.Block{
		"src/App.jsx": {again},
	}, []apiclient.ItemMeta{{Name: "src/App.jsx", Format: "jsx"}}, nil, nil)
	require.NoError(t, err)
	assert.Zero(t, third.BlocksUploaded)
	assert.Equal(t, apiclient.PushUnchanged, third.PushID)
}

// A fleet is not all one version: a CI runner pinned to an older kapi, or a
// laptop that has not upgraded, pushes the same files with a reader that
// records less. Its push must not be able to delete what it does not know
// about — otherwise the two vintages take turns adding and removing the field,
// and the reviewer's in-context reading works or not depending on which one
// pushed last.
//
// A CI runner makes this the ordinary case rather than a corner: the sync cache
// lives under .kapi/work/, which is gitignored, so a fresh checkout declares
// every item and the whole corpus is re-negotiated on every run.
func TestSyncPush_AnOlderProducerDoesNotStripWhatItCannotRecord(t *testing.T) {
	srv, token := newTestServer(t)
	pid := createProject(t, srv, token)

	http := httptest.NewServer(srv.GetEcho())
	defer http.Close()
	client := apiclient.NewProjectBearerClient(http.URL, pid, token)
	ctx := context.Background()

	enriched := &model.Block{ID: "b1", Translatable: true, Name: "greeting",
		Properties: map[string]string{"hash": "k1_9c2f", "component": "Header"}}
	enriched.SetSourceText("Hello")
	_, err := client.Push(ctx, map[string][]*model.Block{
		"src/App.jsx": {enriched},
	}, []apiclient.ItemMeta{{Name: "src/App.jsx", Format: "jsx"}}, nil, nil)
	require.NoError(t, err)
	drainPushQueue(t, srv)

	// The older runner: same text, same file, a reader that records neither.
	bare := &model.Block{ID: "b1", Translatable: true, Name: "greeting"}
	bare.SetSourceText("Hello")
	_, err = client.Push(ctx, map[string][]*model.Block{
		"src/App.jsx": {bare},
	}, []apiclient.ItemMeta{{Name: "src/App.jsx", Format: "jsx"}}, nil, nil)
	require.NoError(t, err)
	drainPushQueue(t, srv)

	stored, err := srv.ContentStore.GetBlocks(t.Context(), platstore.BlockQuery{
		ProjectID: pid, Stream: "main", ItemName: "src/App.jsx",
	})
	require.NoError(t, err)
	require.Len(t, stored, 1)
	assert.Equal(t, "k1_9c2f", stored[0].Block.Properties["hash"],
		"a push that says nothing about a property must not delete it")
	assert.Equal(t, "Header", stored[0].Block.Properties["component"])
}

// Silence is not deletion, but disagreement is: a push whose blocks DO carry a
// property is authoritative about it, so a value genuinely removed at the
// source — a developer note deleted from the code — is removed here too.
func TestSyncPush_APropertyThePushKnowsAboutIsStillRemovable(t *testing.T) {
	srv, token := newTestServer(t)
	pid := createProject(t, srv, token)

	http := httptest.NewServer(srv.GetEcho())
	defer http.Close()
	client := apiclient.NewProjectBearerClient(http.URL, pid, token)
	ctx := context.Background()

	noted := func(id, text, note string) *model.Block {
		b := &model.Block{ID: id, Translatable: true, Name: id}
		if note != "" {
			b.Properties = map[string]string{"locNote": note}
		}
		b.SetSourceText(text)
		return b
	}

	// A current kapi declares what its readers emit, computed over its whole
	// scan — which is how the server tells "removed" from "never heard of".
	declaresNotes := apiclient.DeclareBlockProperties([]string{"locNote"})

	_, err := client.Push(ctx, map[string][]*model.Block{
		"src/App.jsx": {noted("b1", "Hello", "keep it short"), noted("b2", "Bye", "informal")},
	}, []apiclient.ItemMeta{{Name: "src/App.jsx", Format: "jsx"}}, nil, nil, declaresNotes)
	require.NoError(t, err)
	drainPushQueue(t, srv)

	// The note on b1 is deleted in the source. The push still knows what a
	// locNote is — b2 carries one — so the removal is real.
	_, err = client.Push(ctx, map[string][]*model.Block{
		"src/App.jsx": {noted("b1", "Hello", ""), noted("b2", "Bye", "informal")},
	}, []apiclient.ItemMeta{{Name: "src/App.jsx", Format: "jsx"}}, nil, nil, declaresNotes)
	require.NoError(t, err)
	drainPushQueue(t, srv)

	stored, err := srv.ContentStore.GetBlocks(t.Context(), platstore.BlockQuery{
		ProjectID: pid, Stream: "main", ItemName: "src/App.jsx",
	})
	require.NoError(t, err)
	byID := map[string]map[string]string{}
	for _, sb := range stored {
		byID[sb.SourceID] = sb.Block.Properties
	}
	assert.Empty(t, byID["b1"]["locNote"], "a note deleted at the source is deleted here")
	assert.Equal(t, "informal", byID["b2"]["locNote"])
}
