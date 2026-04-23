package blockstore_test

import (
	"context"
	"path/filepath"
	"testing"

	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	bwblockstore "github.com/neokapi/neokapi/bowrain/store/blockstore"
	"github.com/neokapi/neokapi/bowrain/store/sqlitestore"
	"github.com/neokapi/neokapi/core/blockstore"
	"github.com/neokapi/neokapi/core/klf"
	"github.com/neokapi/neokapi/core/model"
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
		Type:         klf.BlockTypeJSXElement,
		Source:       []klf.Run{{Text: &klf.TextRun{Text: "Hello"}}},
	}
	if err := sess.PutBlock("default", block); err != nil {
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
	bs, _, _ := newTestStore(t)
	sess, err := bs.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}

	o := blockstore.Overlay{
		Kind:      "targets/fr",
		BlockHash: "hash-abc",
		Payload:   []byte(`{"runs":[{"text":{"text":"Bonjour"}}]}`),
	}
	if err := sess.PutOverlay(o); err != nil {
		t.Fatalf("put overlay: %v", err)
	}

	got, err := sess.GetOverlay("targets/fr", "hash-abc")
	if err != nil {
		t.Fatalf("get overlay: %v", err)
	}
	if got.Kind != o.Kind || got.BlockHash != o.BlockHash {
		t.Fatalf("overlay key mismatch: got %+v want %+v", got, o)
	}
	if string(got.Payload) != string(o.Payload) {
		t.Fatalf("overlay payload mismatch: got %s want %s", got.Payload, o.Payload)
	}
	if got.UpdatedAt == 0 {
		t.Fatalf("UpdatedAt not set on returned overlay")
	}

	// Overwrite — same key, new payload, should upsert.
	o2 := o
	o2.Payload = []byte(`{"runs":[{"text":{"text":"Salut"}}]}`)
	if err := sess.PutOverlay(o2); err != nil {
		t.Fatalf("put overlay 2: %v", err)
	}
	got2, err := sess.GetOverlay("targets/fr", "hash-abc")
	if err != nil {
		t.Fatalf("get overlay 2: %v", err)
	}
	if string(got2.Payload) != string(o2.Payload) {
		t.Fatalf("overwrite didn't stick: got %s", got2.Payload)
	}
	_ = sess.Commit()
}

func TestSession_ListOverlays(t *testing.T) {
	ctx := context.Background()
	bs, _, _ := newTestStore(t)
	sess, err := bs.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}

	writes := []blockstore.Overlay{
		{Kind: "targets/fr", BlockHash: "h1", Payload: []byte(`{"t":"a"}`)},
		{Kind: "targets/fr", BlockHash: "h2", Payload: []byte(`{"t":"b"}`)},
		{Kind: "targets/fr", BlockHash: "h3", Payload: []byte(`{"t":"c"}`)},
		{Kind: "targets/de", BlockHash: "h1", Payload: []byte(`{"t":"x"}`)},
		{Kind: "annotations/qa", BlockHash: "h1", Payload: []byte(`{"findings":[]}`)},
	}
	for _, o := range writes {
		if err := sess.PutOverlay(o); err != nil {
			t.Fatalf("put %+v: %v", o, err)
		}
	}
	if err := sess.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}

	sess2, _ := bs.Begin(ctx)
	defer sess2.Close()

	var frHashes []string
	for o, err := range sess2.ListOverlays("targets/fr") {
		if err != nil {
			t.Fatalf("list overlays: %v", err)
		}
		frHashes = append(frHashes, o.BlockHash)
	}
	if len(frHashes) != 3 {
		t.Fatalf("expected 3 fr overlays, got %d (%v)", len(frHashes), frHashes)
	}

	var qaCount int
	for o, err := range sess2.ListOverlays("annotations/qa") {
		if err != nil {
			t.Fatalf("list qa: %v", err)
		}
		if o.BlockHash != "h1" {
			t.Fatalf("unexpected qa hash %q", o.BlockHash)
		}
		qaCount++
	}
	if qaCount != 1 {
		t.Fatalf("expected 1 qa overlay, got %d", qaCount)
	}
}

func TestSession_OverlayNotFound(t *testing.T) {
	ctx := context.Background()
	bs, _, _ := newTestStore(t)
	sess, _ := bs.Begin(ctx)
	defer sess.Close()

	_, err := sess.GetOverlay("targets/fr", "nope")
	if err != blockstore.ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestSession_ClosedRejects(t *testing.T) {
	ctx := context.Background()
	bs, _, _ := newTestStore(t)
	sess, _ := bs.Begin(ctx)
	_ = sess.Commit()

	if err := sess.PutOverlay(blockstore.Overlay{Kind: "k", BlockHash: "h", Payload: []byte("{}")}); err != blockstore.ErrClosed {
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

	sess, err := bs.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}

	writes := []blockstore.Overlay{
		{Kind: "targets/fr", BlockHash: "h1", Payload: []byte(`{"text":"Bonjour","provider":"mock"}`)},
		{Kind: "annotations/qa", BlockHash: "h1", Payload: []byte(`{"findings":[]}`)},
		{Kind: "plugins/lint", BlockHash: "h1", Payload: []byte(`{"ruleId":"X"}`)},
	}
	for _, o := range writes {
		if err := sess.PutOverlay(o); err != nil {
			t.Fatalf("put %s: %v", o.Kind, err)
		}
	}
	_ = sess.Commit()

	// Direct SQL probe against each physical table to prove the
	// dispatch picked the right destination.
	db := cs.(*sqlitestore.SQLiteStore).DB()
	mustRowCount := func(q string, args ...any) {
		t.Helper()
		var count int
		if err := db.QueryRow(q, args...).Scan(&count); err != nil {
			t.Fatalf("probe %q: %v", q, err)
		}
		if count != 1 {
			t.Fatalf("expected 1 row from %q, got %d", q, count)
		}
	}
	mustRowCount(`SELECT count(*) FROM translations WHERE project_id=? AND block_id=? AND locale=?`,
		projectID, "h1", "fr")
	mustRowCount(`SELECT count(*) FROM annotations WHERE project_id=? AND block_id=? AND kind=?`,
		projectID, "h1", "annotations/qa")
	mustRowCount(`SELECT count(*) FROM overlays_ext WHERE project_id=? AND block_id=? AND kind=?`,
		projectID, "h1", "plugins/lint")

	// Read-back round-trip through the polymorphic API.
	sess2, _ := bs.Begin(ctx)
	defer sess2.Close()
	for _, o := range writes {
		got, err := sess2.GetOverlay(o.Kind, o.BlockHash)
		if err != nil {
			t.Fatalf("get %s: %v", o.Kind, err)
		}
		if got.Kind != o.Kind || got.BlockHash != o.BlockHash {
			t.Fatalf("%s key mismatch: got %+v", o.Kind, got)
		}
	}
}

// TestSession_Translations_PreservesOpaqueShape exercises the
// graceful path where a caller writes `targets/*` with a payload that
// doesn't fit the `{text, provider}` shape (e.g. a rich editor pushing
// runs). The dispatcher preserves the body verbatim via metadata.
func TestSession_Translations_PreservesOpaqueShape(t *testing.T) {
	ctx := context.Background()
	bs, _, _ := newTestStore(t)
	sess, _ := bs.Begin(ctx)

	o := blockstore.Overlay{
		Kind:      "targets/de",
		BlockHash: "h-opaque",
		Payload:   []byte(`{"runs":[{"text":{"text":"Hallo"}}]}`),
	}
	if err := sess.PutOverlay(o); err != nil {
		t.Fatalf("put: %v", err)
	}
	_ = sess.Commit()

	sess2, _ := bs.Begin(ctx)
	defer sess2.Close()
	got, err := sess2.GetOverlay("targets/de", "h-opaque")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if string(got.Payload) != string(o.Payload) {
		t.Fatalf("opaque payload didn't round-trip:\n  got %s\n want %s", got.Payload, o.Payload)
	}
}
