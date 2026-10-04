package state_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/state"
	"github.com/neokapi/neokapi/core/storage"
)

// linkBlock is a block whose source links to href and whose French
// translation carries the link too, so an href change moves a revision and
// leaves every text, and every hash, as it was.
func linkBlock(srcHref, frHref string) *model.Block {
	link := func(text, href string) []model.Run {
		return []model.Run{
			model.PcOpenR(model.PcOpenRun{ID: "1", Type: "link", Data: `<a href="` + href + `">`}),
			model.TextR(text),
			model.PcCloseR(model.PcCloseRun{ID: "1", Type: "link", Data: "</a>"}),
		}
	}
	b := model.NewRunsBlock("tu1", link("Read the guide", srcHref))
	b.SourceLocale = "en"
	b.SetTargetRuns("fr", link("Lisez le guide", frHref))
	return b
}

func frKey() state.Key {
	return state.Key{Scope: "d-guide", Unit: "u1", Variant: model.Variant("fr")}
}

// approvalOf is an approval of the French translation of b, recorded against
// its revisions when revs is true and against its hashes alone otherwise, as a
// build before revisions recorded it.
func approvalOf(b *model.Block, note string, revs bool) state.UnitState {
	r := state.ReadTarget(b, "fr", "en")
	u := state.UnitState{
		Scope: "d-guide", Unit: "u1", Variant: model.Variant("fr"),
		Status:      model.TargetStatusEstablished,
		TargetHash:  r.TargetHash,
		ContentHash: r.ContentHash,
		Decision:    state.Decision{ReviewState: "approved", Note: note},
		Updated:     "2026-10-01T00:00:00Z",
	}
	if revs {
		u.Basis, u.Revision = r.Basis, r.Revision
	}
	return u
}

// A ledger an earlier build wrote holds the hashes alone. This build opens it,
// adds the revision columns and rewrites nothing: the decision keeps answering
// by its hashes, a decision recorded since carries revisions beside them, the
// two sit side by side, and a withdrawal of the newer one is never undone by
// the older.
func TestWorkStore_ReadsALedgerWrittenBeforeRevisions(t *testing.T) {
	ctx := t.Context()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "work", "state.db")
	committed := filepath.Join(dir, "units")
	require.NoError(t, os.MkdirAll(filepath.Dir(dbPath), 0o755))

	b := linkBlock("https://a.example", "https://a.example")
	legacy := approvalOf(b, "approved before revisions", false)

	// The database as a build before revisions left it: the schema through
	// version 7, one ledger entry and the view row pointing at it.
	db, err := storage.Open(dbPath)
	require.NoError(t, err)
	require.NoError(t, storage.Migrate(db, "state", state.MigrationsThrough(7)))
	payload, err := json.Marshal(legacy)
	require.NoError(t, err)
	id, err := state.Address(legacy, "", false)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `
INSERT INTO unit_decision (id, scope, unit, variant, content_hash, target_hash, actor, origin, recorded_at, revoked, payload)
VALUES (?, 'd-guide', 'u1', 'fr', ?, ?, '', 'local', '2026-10-01T00:00:00.000000000Z', 0, ?)`,
		id, legacy.ContentHash, legacy.TargetHash, string(payload))
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `
INSERT INTO unit_view (checkout, scope, unit, variant, content_hash, target_hash, exported)
VALUES (?, 'd-guide', 'u1', 'fr', ?, ?, 1)`, state.CheckoutID(committed), legacy.ContentHash, legacy.TargetHash)
	require.NoError(t, err)
	require.NoError(t, db.Close())

	w, err := state.OpenWork(ctx, dbPath, committed)
	require.NoError(t, err)
	t.Cleanup(func() { _ = w.Close() })

	got, ok := w.Get(ctx, frKey())
	require.True(t, ok, "the view still points at the entry")
	assert.Equal(t, legacy, got, "the entry reads back exactly as it was written")
	r := state.ReadTarget(b, "fr", "en")
	assert.True(t, got.Fresh(r))
	found, ok := w.Lookup(ctx, frKey(), r)
	require.True(t, ok, "a reader holding revisions finds the entry by its hashes")
	assert.Equal(t, legacy, found)

	// A link change moves no hash, so the entry reads as it always did.
	moved := linkBlock("https://b.example", "https://a.example")
	assert.True(t, got.Fresh(state.ReadTarget(moved, "fr", "en")), "a record written before revisions is read by its hashes")

	// A decision recorded now carries revisions beside the hashes.
	next := approvalOf(b, "re-reviewed", true)
	require.NoError(t, w.Put(ctx, next))
	got, ok = w.Get(ctx, frKey())
	require.True(t, ok)
	assert.Equal(t, next, got)
	assert.True(t, got.SourceStale(state.ReadTarget(moved, "fr", "en")), "and is read by them: the link moved under it")
	found, ok = w.Lookup(ctx, frKey(), r)
	require.True(t, ok)
	assert.Equal(t, "re-reviewed", found.Decision.Note, "the entry that names more of the content answers")

	held, err := w.Ledger(ctx)
	require.NoError(t, err)
	assert.Len(t, held, 2, "the older entry stays in the ledger, under its own pairing")

	require.NoError(t, w.Delete(ctx, frKey()))
	_, ok = w.Lookup(ctx, frKey(), r)
	assert.False(t, ok, "a withdrawal is not undone by the entry recorded before revisions")
}

// Two translations that differ in an inline code alone are two pairings: a
// decision on one answers for it and never for the other, though their text
// and their hashes are one.
func TestWorkStore_PairsByRevision(t *testing.T) {
	ctx := t.Context()
	w, _ := openWork(t)
	first := linkBlock("https://a.example", "https://a.example")
	second := linkBlock("https://a.example", "https://b.example")
	require.Equal(t, state.ReadTarget(first, "fr", "en").TargetHash, state.ReadTarget(second, "fr", "en").TargetHash)

	require.NoError(t, w.Put(ctx, approvalOf(first, "first link", true)))
	require.NoError(t, w.Put(ctx, approvalOf(second, "second link", true)))

	got, ok := w.Lookup(ctx, frKey(), state.ReadTarget(first, "fr", "en"))
	require.True(t, ok)
	assert.Equal(t, "first link", got.Decision.Note)
	got, ok = w.Lookup(ctx, frKey(), state.ReadTarget(second, "fr", "en"))
	require.True(t, ok)
	assert.Equal(t, "second link", got.Decision.Note)

	held, err := w.Ledger(ctx)
	require.NoError(t, err)
	assert.Len(t, held, 2)

	current, ok := w.Get(ctx, frKey())
	require.True(t, ok)
	assert.Equal(t, "second link", current.Decision.Note, "the view points at the pairing recorded last")
	assert.True(t, current.Stale(state.ReadTarget(first, "fr", "en")))
	assert.False(t, current.Stale(state.ReadTarget(second, "fr", "en")))
}

// The committed shards carry the revisions, and an import reads them back into
// the pairing they were recorded under.
func TestWorkStore_ShardsCarryTheRevisions(t *testing.T) {
	ctx := t.Context()
	w, committed := openWork(t)
	b := linkBlock("https://a.example", "https://a.example")
	u := approvalOf(b, "", true)
	require.NoError(t, w.Put(ctx, u))
	require.NoError(t, w.Commit(ctx))

	onDisk, err := state.ReadCommitted(committed)
	require.NoError(t, err)
	require.Len(t, onDisk, 1)
	assert.Equal(t, u.Basis, onDisk[0].Basis)
	assert.Equal(t, u.Revision, onDisk[0].Revision)

	other, err := state.OpenWork(ctx, filepath.Join(t.TempDir(), "state.db"), committed)
	require.NoError(t, err)
	t.Cleanup(func() { _ = other.Close() })
	require.NoError(t, other.Import(ctx))
	got, ok := other.Lookup(ctx, frKey(), state.ReadTarget(b, "fr", "en"))
	require.True(t, ok)
	assert.Equal(t, u, got)
}
