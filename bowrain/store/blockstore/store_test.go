package blockstore_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	bwblockstore "github.com/neokapi/neokapi/bowrain/store/blockstore"
	"github.com/neokapi/neokapi/bowrain/store/sqlitestore"
	"github.com/neokapi/neokapi/core/blockstore"
	"github.com/neokapi/neokapi/core/kbf"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/venue"
)

// newTestStore stands up a real SQLite-backed Bowrain ContentStore,
// wires a blockstore.Store adapter on top, seeds a project, and
// returns everything the tests need. Uses t.TempDir so each test gets
// a clean DB.
func newTestStore(t *testing.T) (blockstore.Store, platstore.ContentStore, string) {
	t.Helper()
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "bs.db")
	cs, err := sqlitestore.NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("new sqlite store: %v", err)
	}
	t.Cleanup(func() { _ = cs.DB().Close() })

	projectID := "proj-test"
	if err := cs.CreateProject(ctx, &platstore.Project{
		ID:                    projectID,
		Name:                  "Test",
		DefaultSourceLanguage: "en",
		TargetLanguages:       []model.LocaleID{"fr", "de"},
	}); err != nil {
		t.Fatalf("create project: %v", err)
	}

	bs, err := bwblockstore.New(bwblockstore.Options{
		ContentStore: cs,
		DB:           cs.DB(),
		Dialect:      bwblockstore.SQLiteDialect,
		ProjectID:    projectID,
		Stream:       "main",
	})
	if err != nil {
		t.Fatalf("new blockstore: %v", err)
	}
	t.Cleanup(func() { _ = bs.Close() })
	return bs, cs, projectID
}

// seedBlock stores one block and returns it as the store holds it. An overlay
// names a block, so every overlay test needs the block it names to exist.
func seedBlock(t *testing.T, cs platstore.ContentStore, projectID, id, text string) *venue.StoredBlock {
	t.Helper()
	ctx := context.Background()
	b := model.NewRunsBlock(id, []model.Run{{Text: &model.TextRun{Text: text}}})
	require.NoError(t, cs.StoreBlocks(ctx, projectID, "main", []*model.Block{b}))
	rows, err := cs.GetBlocks(ctx, platstore.BlockQuery{ProjectID: projectID, Stream: "main", IDs: []string{id}})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	return rows[0]
}

func TestStore_Capabilities(t *testing.T) {
	bs, _, _ := newTestStore(t)
	caps := bs.Capabilities()
	if !caps.RandomAccess || !caps.Concurrent || !caps.Writable || caps.Remote {
		t.Fatalf("unexpected capabilities: %+v", caps)
	}
}

func TestSession_PutGetBlock(t *testing.T) {
	ctx := context.Background()
	bs, _, _ := newTestStore(t)
	sess, err := bs.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}

	block := &blockstore.Block{
		ID:           "hello",
		Translatable: true,
		Type:         kbf.BlockTypeJSXElement,
		Source:       []kbf.Run{{Text: &kbf.TextRun{Text: "Hello"}}},
	}
	// Empty collection = project-level write; collection-scoped
	// writes route through StoreBlocksForItem and get a generated
	// internal ID (exercised in TestSession_PutBlock_CollectionScoped).
	if err := sess.PutBlock("", block); err != nil {
		t.Fatalf("put block: %v", err)
	}
	if err := sess.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}

	// Fresh session — stream blocks, grab the first, verify the
	// content hash the server computed is now populated and usable
	// as a GetBlock lookup key.
	sess2, err := bs.Begin(ctx)
	if err != nil {
		t.Fatalf("begin 2: %v", err)
	}
	defer sess2.Close()

	var streamed *blockstore.Block
	for b, err := range sess2.Blocks(blockstore.BlockFilter{Limit: 1}) {
		if err != nil {
			t.Fatalf("stream blocks: %v", err)
		}
		streamed = b
		break
	}
	if streamed == nil || streamed.ID != "hello" {
		t.Fatalf("expected streamed block with ID=hello, got %+v", streamed)
	}
	if streamed.Hash == "" {
		t.Fatalf("expected server-computed ContentHash on streamed block, got empty")
	}
	if len(streamed.Source) != 1 || streamed.Source[0].Text == nil || streamed.Source[0].Text.Text != "Hello" {
		t.Fatalf("source runs didn't round-trip: %+v", streamed.Source)
	}

	// Now look up that same block by its computed hash.
	got, err := sess2.GetBlock(streamed.Hash)
	if err != nil {
		t.Fatalf("get block by computed hash: %v", err)
	}
	if got == nil || got.ID != "hello" {
		t.Fatalf("GetBlock roundtrip mismatch: %+v", got)
	}
}

func TestSession_OverlayRoundTrip(t *testing.T) {
	ctx := context.Background()
	bs, cs, projectID := newTestStore(t)
	block := seedBlock(t, cs, projectID, "blk-greet", "Hello")
	sess, err := bs.Begin(ctx)
	require.NoError(t, err)

	o := blockstore.Overlay{
		Kind:      "targets/fr",
		BlockHash: block.ContentHash,
		Payload:   []byte(`{"runs":[{"text":"Bonjour"}]}`),
	}
	require.NoError(t, sess.PutOverlay(o))

	got, err := sess.GetOverlay("targets/fr", block.ContentHash)
	require.NoError(t, err)
	require.Equal(t, o.Kind, got.Kind)
	require.Equal(t, o.BlockHash, got.BlockHash)
	require.JSONEq(t, `{"runs":[{"text":"Bonjour"}],"text":"Bonjour"}`, string(got.Payload))
	require.NotZero(t, got.UpdatedAt, "UpdatedAt not set on returned overlay")

	// Overwrite — same key, new payload, should upsert.
	o2 := o
	o2.Payload = []byte(`{"runs":[{"text":"Salut"}]}`)
	require.NoError(t, sess.PutOverlay(o2))
	got2, err := sess.GetOverlay("targets/fr", block.ContentHash)
	require.NoError(t, err)
	require.JSONEq(t, `{"runs":[{"text":"Salut"}],"text":"Salut"}`, string(got2.Payload))
	require.NoError(t, sess.Commit())
}

// TestSession_OverlayKeyNamesNoBlock pins the refusal: an overlay is a layer on
// a block, and a row nothing can join to is indistinguishable from work not yet
// done.
func TestSession_OverlayKeyNamesNoBlock(t *testing.T) {
	ctx := context.Background()
	bs, _, _ := newTestStore(t)
	sess, err := bs.Begin(ctx)
	require.NoError(t, err)
	defer sess.Close()

	for _, kind := range []string{"targets/fr", "annotations/qa", "plugins/lint"} {
		t.Run(kind, func(t *testing.T) {
			err = sess.PutOverlay(blockstore.Overlay{
				Kind:      kind,
				BlockHash: "hash-nobody-holds",
				Payload:   []byte(`{"text":"Bonjour"}`),
			})
			require.ErrorIs(t, err, blockstore.ErrNotFound)

			_, err = sess.GetOverlay(kind, "hash-nobody-holds")
			require.ErrorIs(t, err, blockstore.ErrNotFound)
		})
	}
}

func TestSession_ListOverlays(t *testing.T) {
	ctx := context.Background()
	bs, cs, projectID := newTestStore(t)
	one := seedBlock(t, cs, projectID, "blk-1", "One")
	two := seedBlock(t, cs, projectID, "blk-2", "Two")
	three := seedBlock(t, cs, projectID, "blk-3", "Three")

	sess, err := bs.Begin(ctx)
	require.NoError(t, err)

	writes := []blockstore.Overlay{
		{Kind: "targets/fr", BlockHash: one.ContentHash, Payload: []byte(`{"text":"a"}`)},
		{Kind: "targets/fr", BlockHash: two.ContentHash, Payload: []byte(`{"text":"b"}`)},
		{Kind: "targets/fr", BlockHash: three.ContentHash, Payload: []byte(`{"text":"c"}`)},
		{Kind: "targets/de", BlockHash: one.ContentHash, Payload: []byte(`{"text":"x"}`)},
		{Kind: "annotations/qa", BlockHash: one.ContentHash, Payload: []byte(`{"findings":[]}`)},
	}
	for _, o := range writes {
		require.NoErrorf(t, sess.PutOverlay(o), "put %+v", o)
	}
	require.NoError(t, sess.Commit())

	sess2, err := bs.Begin(ctx)
	require.NoError(t, err)
	defer sess2.Close()

	// A listed overlay carries the same key the block iterator reports for its
	// block, so a caller can correlate the two without a second lookup.
	var frHashes []string
	for o, err := range sess2.ListOverlays("targets/fr") {
		require.NoError(t, err)
		frHashes = append(frHashes, o.BlockHash)
	}
	require.ElementsMatch(t, []string{one.ContentHash, two.ContentHash, three.ContentHash}, frHashes)

	var checkHashes []string
	for o, err := range sess2.ListOverlays("annotations/qa") {
		require.NoError(t, err)
		checkHashes = append(checkHashes, o.BlockHash)
	}
	require.Equal(t, []string{one.ContentHash}, checkHashes)
}

func TestSession_OverlayNotFound(t *testing.T) {
	ctx := context.Background()
	bs, _, _ := newTestStore(t)
	sess, _ := bs.Begin(ctx)
	defer sess.Close()

	_, err := sess.GetOverlay("targets/fr", "nope")
	if !errors.Is(err, blockstore.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestSession_ClosedRejects(t *testing.T) {
	ctx := context.Background()
	bs, _, _ := newTestStore(t)
	sess, _ := bs.Begin(ctx)
	_ = sess.Commit()

	err := sess.PutOverlay(blockstore.Overlay{Kind: "k", BlockHash: "h", Payload: []byte("{}")})
	if !errors.Is(err, blockstore.ErrClosed) {
		t.Fatalf("expected ErrClosed after commit, got %v", err)
	}
}

// TestSession_DispatchByKind proves #403: overlay writes land in
// different physical tables based on kind prefix (translations /
// annotations / overlays_ext) even though the callers see one
// polymorphic PutOverlay API.
func TestSession_DispatchByKind(t *testing.T) {
	ctx := context.Background()
	bs, cs, projectID := newTestStore(t)
	block := seedBlock(t, cs, projectID, "blk-dispatch", "Hello")

	sess, err := bs.Begin(ctx)
	require.NoError(t, err)

	writes := []blockstore.Overlay{
		{Kind: "targets/fr", BlockHash: block.ContentHash, Payload: []byte(`{"text":"Bonjour","provider":"mock"}`)},
		{Kind: "annotations/qa", BlockHash: block.ContentHash, Payload: []byte(`{"findings":[]}`)},
		{Kind: "plugins/lint", BlockHash: block.ContentHash, Payload: []byte(`{"ruleId":"X"}`)},
	}
	for _, o := range writes {
		require.NoErrorf(t, sess.PutOverlay(o), "put %s", o.Kind)
	}
	require.NoError(t, sess.Commit())

	// Direct SQL probe against each physical table to prove the
	// dispatch picked the right destination.
	db := cs.(*sqlitestore.SQLiteStore).DB()
	mustRowCount := func(q string, args ...any) {
		t.Helper()
		var count int
		require.NoErrorf(t, db.QueryRow(q, args...).Scan(&count), "probe %q", q)
		require.Equalf(t, 1, count, "expected 1 row from %q", q)
	}
	// Every row is filed under the block's own id — where the block round-trip
	// writes it, where hydration reads it back, and where deletion clears it.
	// The annotation is filed under the bare key, which is what Block.SetAnno is
	// handed on the way back; the plugin kind stays opaque because nothing but
	// this adapter reads it.
	mustRowCount(`SELECT count(*) FROM translations WHERE project_id=? AND block_id=? AND locale=?`,
		projectID, block.ID, "fr")
	mustRowCount(`SELECT count(*) FROM annotations WHERE project_id=? AND block_id=? AND kind=?`,
		projectID, block.ID, "qa")
	mustRowCount(`SELECT count(*) FROM overlays_ext WHERE project_id=? AND block_id=? AND kind=?`,
		projectID, block.ID, "plugins/lint")

	// Read-back round-trip through the polymorphic API.
	sess2, err := bs.Begin(ctx)
	require.NoError(t, err)
	defer sess2.Close()
	for _, o := range writes {
		got, err := sess2.GetOverlay(o.Kind, o.BlockHash)
		require.NoErrorf(t, err, "get %s", o.Kind)
		require.Equal(t, o.Kind, got.Kind)
		require.Equal(t, o.BlockHash, got.BlockHash)
	}
}

// TestSession_Translations_KeepsWriterResidue pins what happens to payload
// fields the Target model has no room for: the tool-config fingerprint an MT
// or AI cache checks before reusing a target survives the round-trip, while
// the target itself lands in the columns every platform reader consults.
func TestSession_Translations_KeepsWriterResidue(t *testing.T) {
	ctx := context.Background()
	bs, cs, projectID := newTestStore(t)
	block := seedBlock(t, cs, projectID, "blk-residue", "Hello")
	sess, err := bs.Begin(ctx)
	require.NoError(t, err)

	require.NoError(t, sess.PutOverlay(blockstore.Overlay{
		Kind:      "targets/de",
		BlockHash: block.ContentHash,
		Payload:   []byte(`{"runs":[{"text":"Hallo"}],"provider":"mock","config":"fp-1"}`),
	}))
	require.NoError(t, sess.Commit())

	sess2, err := bs.Begin(ctx)
	require.NoError(t, err)
	defer sess2.Close()
	got, err := sess2.GetOverlay("targets/de", block.ContentHash)
	require.NoError(t, err)
	require.JSONEq(t,
		`{"runs":[{"text":"Hallo"}],"text":"Hallo","provider":"mock","config":"fp-1"}`,
		string(got.Payload))

	rows, err := cs.GetBlocks(ctx, platstore.BlockQuery{ProjectID: projectID, Stream: "main", IDs: []string{block.ID}})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, "Hallo", rows[0].Block.TargetText("de"))
}

// TestSession_PutBlock_CollectionScoped exercises the collection arg:
// a non-empty collection routes the write through StoreBlocksForItem
// (creating the collection if needed), a later Blocks(filter.Collection)
// iteration picks the block back up.
func TestSession_PutBlock_CollectionScoped(t *testing.T) {
	ctx := context.Background()
	bs, _, _ := newTestStore(t)

	sess, err := bs.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	want := &blockstore.Block{
		ID:           "src-1",
		Translatable: true,
		Source:       []kbf.Run{{Text: &kbf.TextRun{Text: "Scoped"}}},
	}
	if err := sess.PutBlock("greetings", want); err != nil {
		t.Fatalf("put block in collection: %v", err)
	}
	if err := sess.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}

	// Filter by the collection name — should yield our block.
	sess2, _ := bs.Begin(ctx)
	defer sess2.Close()
	var got int
	for b, err := range sess2.Blocks(blockstore.BlockFilter{Collection: "greetings"}) {
		if err != nil {
			t.Fatalf("iterate: %v", err)
		}
		if b == nil {
			continue
		}
		if len(b.Source) != 1 || b.Source[0].Text == nil || b.Source[0].Text.Text != "Scoped" {
			t.Fatalf("wrong block streamed: %+v", b.Source)
		}
		got++
	}
	if got != 1 {
		t.Fatalf("expected 1 block in collection, got %d", got)
	}

	// Filter by a collection that doesn't exist — no blocks, no error.
	var gotNone int
	for _, err := range sess2.Blocks(blockstore.BlockFilter{Collection: "nope"}) {
		if err != nil {
			t.Fatalf("iterate nope: %v", err)
		}
		gotNone++
	}
	if gotNone != 0 {
		t.Fatalf("expected 0 blocks for nonexistent collection, got %d", gotNone)
	}
}

// TestHydration_StoreBlocksRoundtripsTargets proves the #405 read path:
// ContentStore.StoreBlocks landing a block with populated Targets +
// Annotations produces rows in the kind-specific tables, and a
// subsequent GetBlocks hydrates them back.
func TestHydration_StoreBlocksRoundtripsTargets(t *testing.T) {
	ctx := context.Background()
	_, cs, projectID := newTestStore(t)

	b := model.NewRunsBlock("blk-hello", []model.Run{{Text: &model.TextRun{Text: "Hello"}}})
	b.SetTargetRuns("fr", []model.Run{{Text: &model.TextRun{Text: "Bonjour"}}})
	b.SetTargetRuns("de", []model.Run{{Text: &model.TextRun{Text: "Hallo"}}})

	require.NoError(t, cs.StoreBlocks(ctx, projectID, "main", []*model.Block{b}))

	rows, err := cs.GetBlocks(ctx, platstore.BlockQuery{
		ProjectID: projectID,
		Stream:    "main",
		IDs:       []string{"blk-hello"},
	})
	if err != nil {
		t.Fatalf("GetBlocks: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 block, got %d", len(rows))
	}
	got := rows[0].Block
	if got.TargetText("fr") != "Bonjour" {
		t.Fatalf("fr target didn't round-trip: %q", got.TargetText("fr"))
	}
	if got.TargetText("de") != "Hallo" {
		t.Fatalf("de target didn't round-trip: %q", got.TargetText("de"))
	}

	// GetBlock (single) should do the same hydration.
	single, err := cs.GetBlock(ctx, projectID, "main", "blk-hello")
	if err != nil {
		t.Fatalf("GetBlock: %v", err)
	}
	if single.Block.TargetText("fr") != "Bonjour" {
		t.Fatalf("single-fetch fr didn't round-trip: %q", single.Block.TargetText("fr"))
	}
}
