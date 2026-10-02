package history_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/history"
	"github.com/neokapi/neokapi/core/reconcile"
	"github.com/neokapi/neokapi/core/storage"
)

func openStore(t *testing.T) *history.Store {
	t.Helper()
	db, err := storage.OpenWith(filepath.Join(t.TempDir(), "context.db"), storage.ProjectOptions())
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	s, err := history.Open(db)
	require.NoError(t, err)
	return s
}

func at(sec int) time.Time { return time.Date(2026, 10, 1, 12, 0, sec, 0, time.UTC) }

func TestReadsAnswerMostRecentFirst(t *testing.T) {
	s := openStore(t)
	ctx := t.Context()
	rows := []history.Row{
		{Op: "op1", Doc: "d-1", Block: "p#1", Edition: "en", Before: "absent", After: "r:1", ContentHash: "h1", ContextHash: "c1", Actor: "tool", Origin: "flow:up", At: at(1)},
		{Op: "op2", Doc: "d-1", Block: "p#1", Edition: "en", Before: "r:1", After: "r:2", ContentHash: "h2", ContextHash: "c1", Actor: "agent", ActorName: "claude", Session: "s1", Origin: "apply", At: at(2)},
		{Op: "op2", Doc: "d-1", Block: "p#1", Edition: "fr", Before: "r:f1", After: "r:f2", Basis: "r:2", ContentHash: "h2", ContextHash: "c1", Actor: "agent", ActorName: "claude", Session: "s1", Origin: "apply", At: at(2)},
		{Op: "op3", Doc: "d-1", Block: "p#2", Key: "u-k", Edition: "en", Before: "r:a", After: "r:b", ContentHash: "h3", ContextHash: "c2", Actor: "person", Origin: "desktop", At: at(3)},
		{Op: "op4", Doc: "d-2", Block: "p#1", Edition: "en", Before: "r:x", After: "r:y", ContentHash: "h4", ContextHash: "c1", Origin: "observed", At: at(4)},
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

	reached, err := s.Reached(ctx, "d-1")
	require.NoError(t, err)
	assert.Equal(t, "op2", reached[history.Reach{Block: "p#1", Edition: "en", Rev: "r:2"}])
	assert.Equal(t, "op1", reached[history.Reach{Block: "p#1", Edition: "en", Rev: "r:1"}])
	assert.Empty(t, reached[history.Reach{Block: "p#1", Edition: "en", Rev: "r:9"}])

	head, err := s.DocumentHead(ctx, "d-2")
	require.NoError(t, err)
	assert.Equal(t, "op4", head)
	head, err = s.DocumentHead(ctx, "d-none")
	require.NoError(t, err)
	assert.Empty(t, head)
}

func TestPriorsCarryTheMostRecentIdentityEvidence(t *testing.T) {
	s := openStore(t)
	ctx := t.Context()
	require.NoError(t, s.Put(ctx, []history.Row{
		{Op: "op1", Doc: "d-1", Block: "p#1", Edition: "en", ContentHash: "old", ContextHash: "c1", At: at(1)},
		{Op: "op2", Doc: "d-1", Block: "p#1", Edition: "en", ContentHash: "new", ContextHash: "c1", At: at(2)},
		{Op: "op2", Doc: "d-1", Block: "p#1", Edition: "fr", ContentHash: "new", ContextHash: "c1", At: at(2)},
		{Op: "op3", Doc: "d-1", Block: "p#3", Key: "u-moved", Edition: "en", ContentHash: "m", ContextHash: "c3", At: at(3)},
		{Op: "op4", Doc: "d-2", Block: "p#1", Edition: "en", ContentHash: "other", ContextHash: "c1", At: at(4)},
	}))
	priors, err := s.Priors(ctx, "d-1")
	require.NoError(t, err)
	assert.Equal(t, []reconcile.Unit{
		{Key: "p#1", Scope: "d-1", ContentHash: "new", ContextHash: "c1"},
		{Key: "u-moved", Scope: "d-1", ContentHash: "m", ContextHash: "c3"},
	}, priors, "one unit per block, keyed by its durable key where it has one")
}
