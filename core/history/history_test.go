package history_test

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/history"
	"github.com/neokapi/neokapi/core/reconcile"
	"github.com/neokapi/neokapi/core/storage"
)

func openDB(t *testing.T) *storage.DB {
	t.Helper()
	db, err := storage.OpenWith(filepath.Join(t.TempDir(), "context.db"), storage.ProjectOptions())
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func openStore(t *testing.T) *history.Store {
	t.Helper()
	s, err := history.Open(openDB(t))
	require.NoError(t, err)
	return s
}

func at(sec int) time.Time { return time.Date(2026, 10, 1, 12, 0, sec, 0, time.UTC) }

func TestReadsAnswerMostRecentFirst(t *testing.T) {
	s := openStore(t)
	ctx := t.Context()
	rows := []history.Row{
		{Op: "op1", Address: "a1", Doc: "d-1", Block: "p#1", Edition: "en", Before: "absent", After: "r:1", ContentHash: "h1", ContextHash: "c1", Actor: "tool", Origin: "flow:up", At: at(1)},
		{Op: "op2", Address: "a2", Doc: "d-1", Block: "p#1", Edition: "en", Before: "r:1", After: "r:2", ContentHash: "h2", ContextHash: "c1", Actor: "agent", ActorName: "claude", Session: "s1", Origin: "apply", At: at(2)},
		{Op: "op2", Address: "a2", Doc: "d-1", Block: "p#1", Edition: "fr", Before: "r:f1", After: "r:f2", Basis: "r:2", ContentHash: "h2", ContextHash: "c1", Actor: "agent", ActorName: "claude", Session: "s1", Origin: "apply", At: at(2)},
		{Op: "op3", Address: "a3", Doc: "d-1", Block: "p#2", Key: "u-k", Edition: "en", Before: "r:a", After: "r:b", ContentHash: "h3", ContextHash: "c2", Actor: "person", Origin: "desktop", At: at(3)},
		{Op: "op4", Address: "a4", Doc: "d-2", Block: "p#1", Edition: "en", Before: "r:x", After: "r:y", ContentHash: "h4", ContextHash: "c1", Origin: "observed", At: at(4)},
	}
	require.NoError(t, s.Put(ctx, rows))
	require.NoError(t, s.Put(ctx, rows), "applying the same rows again changes nothing")

	got, err := s.Edition(ctx, "d-1", "p#1", "en")
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, "op2", got[0].Op)
	assert.Equal(t, rows[1], got[0], "a row reads back as it was written")

	last, found, err := s.LastWrite(ctx, "d-1", "p#1", "fr")
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, "claude", last.ActorName)
	assert.Equal(t, "r:2", last.Basis)

	_, found, err = s.LastWrite(ctx, "d-1", "p#9", "en")
	require.NoError(t, err)
	assert.False(t, found)

	doc, err := s.Document(ctx, "d-1")
	require.NoError(t, err)
	assert.Len(t, doc, 4)

	heads, err := s.Heads(ctx, "d-1")
	require.NoError(t, err)
	assert.Equal(t, map[history.EditionRef]string{
		{Block: "p#1", Edition: "en"}: "op2",
		{Block: "p#1", Edition: "fr"}: "op2",
		{Block: "p#2", Edition: "en"}: "op3",
	}, heads)

	asked := []history.Reach{
		{Block: "p#1", Edition: "en", Rev: "r:2"},
		{Block: "p#1", Edition: "en", Rev: "r:1"},
		{Block: "p#1", Edition: "en", Rev: "r:9"},
		{Block: "p#1", Edition: "fr", Rev: "r:f2"},
	}
	reached, err := s.Reached(ctx, "d-1", asked)
	require.NoError(t, err)
	assert.Equal(t, map[history.Reach]string{
		{Block: "p#1", Edition: "en", Rev: "r:2"}:  "a2",
		{Block: "p#1", Edition: "en", Rev: "r:1"}:  "a1",
		{Block: "p#1", Edition: "fr", Rev: "r:f2"}: "a2",
	}, reached, "each revision asked about answers with the address that reached it; one never reached is left out")
	none, err := s.Reached(ctx, "d-2", nil)
	require.NoError(t, err)
	assert.Empty(t, none)

	head, err := s.DocumentHead(ctx, "d-2")
	require.NoError(t, err)
	assert.Equal(t, "op4", head)
	head, err = s.DocumentHead(ctx, "d-none")
	require.NoError(t, err)
	assert.Empty(t, head)
}

// TestARowIsKeyedByItsOperationsAddress: two logs that recorded one edit under
// different ids keep the older id once they merge. The row the newer id
// projected is the row the older one rewrites, so the store ends with the
// rows a rebuild from the merged log writes.
func TestARowIsKeyedByItsOperationsAddress(t *testing.T) {
	s := openStore(t)
	ctx := t.Context()
	newer := history.Row{Op: "op9", Address: "edit:x", Doc: "d-1", Block: "p#1", Edition: "nb",
		Before: "r:1", After: "r:2", Origin: "observed", At: at(9)}
	require.NoError(t, s.Put(ctx, []history.Row{newer}))

	older := newer
	older.Op, older.Origin, older.At = "op1", "flow:up", at(1)
	require.NoError(t, s.Put(ctx, []history.Row{older}))

	got, err := s.Edition(ctx, "d-1", "p#1", "nb")
	require.NoError(t, err)
	assert.Equal(t, []history.Row{older}, got, "one row, as the operation the log kept wrote it")
}

// TestReachedAsksTheIndexRatherThanTheDocument: a document with a long
// history answers a lookup of a few revisions with the rows those revisions
// name.
func TestReachedAsksTheIndexRatherThanTheDocument(t *testing.T) {
	s := openStore(t)
	ctx := t.Context()
	var rows []history.Row
	for pass := range 50 {
		for b := range 40 {
			rows = append(rows, history.Row{
				Op: fmt.Sprintf("op%03d", pass), Address: fmt.Sprintf("a%03d", pass), Doc: "d-1",
				Block: fmt.Sprintf("p#%d", b), Edition: "nb",
				Before: fmt.Sprintf("r:%d.%d", pass, b), After: fmt.Sprintf("r:%d.%d", pass+1, b), At: at(pass),
			})
		}
	}
	require.NoError(t, s.Put(ctx, rows))
	reached, err := s.Reached(ctx, "d-1", []history.Reach{{Block: "p#7", Edition: "nb", Rev: "r:50.7"}})
	require.NoError(t, err)
	assert.Equal(t, map[history.Reach]string{{Block: "p#7", Edition: "nb", Rev: "r:50.7"}: "a049"}, reached)

	plan := history.ReachedPlan(t, s)
	assert.Contains(t, plan, "SEARCH h USING INDEX block_history_reached (doc=? AND block=? AND edition=? AND after=?)",
		"each revision is a search of the index on the revision it names: %q", plan)
	for _, step := range plan {
		assert.NotContains(t, step, "SCAN h", "no lookup walks the document's history")
	}
}

func TestPriorsCarryTheMostRecentIdentityEvidence(t *testing.T) {
	s := openStore(t)
	ctx := t.Context()
	require.NoError(t, s.Put(ctx, []history.Row{
		{Op: "op1", Address: "a1", Doc: "d-1", Block: "p#1", Edition: "en", ContentHash: "old", ContextHash: "c1", At: at(1)},
		{Op: "op2", Address: "a2", Doc: "d-1", Block: "p#1", Edition: "en", ContentHash: "new", ContextHash: "c1", At: at(2)},
		{Op: "op2", Address: "a2", Doc: "d-1", Block: "p#1", Edition: "fr", ContentHash: "new", ContextHash: "c1", At: at(2)},
		{Op: "op3", Address: "a3", Doc: "d-1", Block: "p#3", Key: "u-moved", Edition: "en", ContentHash: "m", ContextHash: "c3", At: at(3)},
		{Op: "op4", Address: "a4", Doc: "d-2", Block: "p#1", Edition: "en", ContentHash: "other", ContextHash: "c1", At: at(4)},
	}))
	priors, err := s.Priors(ctx, "d-1")
	require.NoError(t, err)
	assert.Equal(t, []reconcile.Unit{
		{Key: "p#1", Scope: "d-1", ContentHash: "new", ContextHash: "c1"},
		{Key: "u-moved", Scope: "d-1", ContentHash: "m", ContextHash: "c3"},
	}, priors, "one unit per block, keyed by its durable key where it has one")
}
