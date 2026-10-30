package server

import (
	"os"
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	platev "github.com/neokapi/neokapi/bowrain/core/event"
	"github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/bowrain/event"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/venue"
)

// uploadOnce uploads one file through the editor's upload path and returns the
// item's stored blocks, keyed by the key the file's reader gave each block.
func uploadOnce(t *testing.T, srv *Server, pid, name string, data []byte) (*ProjectInfoResponse, map[string]*venue.StoredBlock) {
	t.Helper()
	ctx := t.Context()
	info, err := editorAddFiles(ctx, srv.ContentStore, srv.FormatRegistry, pid, "main", map[string][]byte{name: data})
	require.NoError(t, err)
	require.Empty(t, info.Skipped, "the upload must not skip %s", name)

	stored, err := srv.ContentStore.GetBlocks(ctx, store.BlockQuery{ProjectID: pid, Stream: "main", ItemName: name})
	require.NoError(t, err)
	bySource := make(map[string]*venue.StoredBlock, len(stored))
	for _, sb := range stored {
		require.NotContains(t, bySource, sb.SourceID, "one row per reader key")
		bySource[sb.SourceID] = sb
	}
	return info, bySource
}

func storedIDs(blocks map[string]*venue.StoredBlock) []string {
	ids := make([]string, 0, len(blocks))
	for _, sb := range blocks {
		ids = append(ids, sb.Block.ID)
	}
	sort.Strings(ids)
	return ids
}

func itemBlockCount(t *testing.T, info *ProjectInfoResponse, name string) int {
	t.Helper()
	for _, it := range info.Items {
		if it.Name == name {
			return it.BlockCount
		}
	}
	t.Fatalf("item %s is missing from the upload response", name)
	return 0
}

// Uploading an unchanged file again leaves the item's blocks exactly as they
// were: the same rows, under the same ids, and nothing added.
func TestEditorUpload_AnUnchangedFileUploadedAgainKeepsItsBlocks(t *testing.T) {
	srv, token := newTestServer(t)
	pid := createProject(t, srv, token)

	data, err := os.ReadFile("../../apps/kapi-desktop/backend/sample/kapimart/src/en/email-templates.html")
	require.NoError(t, err)
	const name = "email-templates.html"

	firstInfo, first := uploadOnce(t, srv, pid, name, data)
	require.NotEmpty(t, first)
	for i := 2; i <= 3; i++ {
		info, again := uploadOnce(t, srv, pid, name, data)
		assert.Len(t, again, len(first), "upload %d of an unchanged file", i)
		assert.Equal(t, storedIDs(first), storedIDs(again), "upload %d keeps every block id", i)
		assert.Equal(t, itemBlockCount(t, firstInfo, name), itemBlockCount(t, info, name),
			"upload %d reports the same block count", i)
	}
}

// A second upload of a changed file keeps the rows of the blocks that are still
// there, with their translations, removes the block that is gone, and adds the
// block that is new.
func TestEditorUpload_AChangedFileKeepsTranslationsAndDropsWhatIsGone(t *testing.T) {
	srv, token := newTestServer(t)
	pid := createProject(t, srv, token)
	ctx := t.Context()
	const name = "en.json"

	_, first := uploadOnce(t, srv, pid, name,
		[]byte(`{"greeting":"Hello","farewell":"Goodbye","promo":"Save today"}`))
	require.Len(t, first, 3)
	greeting, farewell, promo := first["greeting"], first["farewell"], first["promo"]
	require.NotNil(t, greeting)
	require.NotNil(t, farewell)
	require.NotNil(t, promo)

	_, err := srv.ContentStore.UpdateBlock(ctx, pid, "main", greeting.Block.ID, func(sb *venue.StoredBlock) error {
		sb.Block.SetTargetText(model.LocaleID("fr"), "Bonjour")
		return nil
	})
	require.NoError(t, err)

	// promo is gone, farewell's wording changed, and a new block arrives.
	info, second := uploadOnce(t, srv, pid, name,
		[]byte(`{"greeting":"Hello","farewell":"See you soon","signup":"Create an account"}`))

	require.Len(t, second, 3)
	assert.Equal(t, 3, itemBlockCount(t, info, name))

	kept := second["greeting"]
	require.NotNil(t, kept)
	assert.Equal(t, greeting.Block.ID, kept.Block.ID, "the unchanged block keeps its row")
	assert.Equal(t, "Bonjour", kept.Block.TargetText(model.LocaleID("fr")), "and its translation")

	changed := second["farewell"]
	require.NotNil(t, changed)
	assert.Equal(t, farewell.Block.ID, changed.Block.ID, "a block whose wording changed keeps its row")
	assert.Equal(t, "See you soon", changed.Block.SourceText())

	assert.NotContains(t, second, "promo", "the block the file no longer has is removed")
	_, err = srv.ContentStore.GetBlock(ctx, pid, "main", promo.Block.ID)
	require.Error(t, err, "the removed block's row is gone")

	assert.Contains(t, second, "signup", "the new block is added")
}

// A block whose key changes is a different block: the old row goes and a new
// row stands under the new key.
func TestEditorUpload_ARenamedBlockReplacesTheOldRow(t *testing.T) {
	srv, token := newTestServer(t)
	pid := createProject(t, srv, token)
	ctx := t.Context()
	const name = "en.json"

	_, first := uploadOnce(t, srv, pid, name, []byte(`{"title":"Welcome","body":"Read on"}`))
	require.Len(t, first, 2)
	oldBody := first["body"]
	require.NotNil(t, oldBody)

	_, second := uploadOnce(t, srv, pid, name, []byte(`{"title":"Welcome","intro":"Read on"}`))
	require.Len(t, second, 2)
	assert.NotContains(t, second, "body")
	renamed := second["intro"]
	require.NotNil(t, renamed)
	assert.NotEqual(t, oldBody.Block.ID, renamed.Block.ID, "the new key stands on a new row")
	assert.Equal(t, first["title"].Block.ID, second["title"].Block.ID)

	_, err := srv.ContentStore.GetBlock(ctx, pid, "main", oldBody.Block.ID)
	require.Error(t, err, "the old key's row is gone")
}

// A file that reads to no blocks leaves its item with none.
func TestEditorUpload_AFileThatReadsToNothingRemovesEveryBlock(t *testing.T) {
	srv, token := newTestServer(t)
	pid := createProject(t, srv, token)
	const name = "en.json"

	_, first := uploadOnce(t, srv, pid, name, []byte(`{"title":"Welcome"}`))
	require.Len(t, first, 1)

	info, second := uploadOnce(t, srv, pid, name, []byte(`{}`))
	assert.Empty(t, second)
	assert.Equal(t, 0, itemBlockCount(t, info, name))
}

// On a running server the content store carries the event decorator. A
// re-upload that removes blocks still publishes the item's block events, so a
// surface showing the item refetches it.
func TestEditorUpload_ARemovalPublishesTheItemsBlockEvent(t *testing.T) {
	srv, token := newTestServer(t)
	pid := createProject(t, srv, token)
	const name = "en.json"

	_, first := uploadOnce(t, srv, pid, name, []byte(`{"title":"Welcome","body":"Read on"}`))
	require.Len(t, first, 2)

	bus := event.NewChannelEventBus()
	t.Cleanup(bus.Close)
	srv.ContentStore = event.NewEventEmittingStore(srv.ContentStore, bus)
	itemEvents := make(chan platev.Event, 16)
	sub := bus.SubscribeAll(func(ev platev.Event) {
		// The prune's event names the item and no block.
		if ev.Type == platev.EventBlockUpdated && ev.Data["item_name"] == name && ev.Data["block_id"] == "" {
			select {
			case itemEvents <- ev:
			default:
			}
		}
	})
	t.Cleanup(func() { bus.Unsubscribe(sub) })

	info, second := uploadOnce(t, srv, pid, name, []byte(`{"title":"Welcome"}`))
	require.Len(t, second, 1)
	require.NotNil(t, info)
	assert.Equal(t, 1, itemBlockCount(t, info, name))

	select {
	case ev := <-itemEvents:
		assert.Equal(t, pid, ev.ProjectID)
	case <-time.After(2 * time.Second):
		t.Fatal("the removal published no item event")
	}
}
