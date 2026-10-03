package history_test

import (
	"fmt"
	"path/filepath"
	"strings"
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
		{Op: "op1", Address: "a1", Doc: "d-1", Block: "p#1", Edition: "en", Before: "absent", After: "r:1", ContentHash: "h1", ContextHash: "c1", Actor: "tool", Origin: "flow:up", Ops: []string{"set_content"}, Tool: "pseudo-translate", At: at(1)},
		{Op: "op2", Address: "a2", Doc: "d-1", Block: "p#1", Edition: "en", Before: "r:1", After: "r:2", ContentHash: "h2", ContextHash: "c1", Actor: "agent", ActorName: "claude", Session: "s1", Origin: "apply", Ops: []string{"replace_text", "set_attribute"}, At: at(2)},
		{Op: "op2", Address: "a2", Doc: "d-1", Block: "p#1", Edition: "fr", Before: "r:f1", After: "r:f2", Basis: "r:2", ContentHash: "h2", ContextHash: "c1", Actor: "agent", ActorName: "claude", Session: "s1", Origin: "apply", At: at(2)},
		{Op: "op3", Address: "a3", Doc: "d-1", Block: "p#2", Key: "u-k", Edition: "en", Before: "r:a", After: "r:b", ContentHash: "h3", ContextHash: "c2", Actor: "person", Origin: "desktop", At: at(3)},
		{Op: "op4", Address: "a4", Doc: "d-2", Block: "p#1", Edition: "en", Before: "r:x", After: "r:y", ContentHash: "h4", ContextHash: "c1", Origin: "observed", At: at(4)},
	}
	require.NoError(t, s.Put(ctx, rows))
	require.NoError(t, s.Put(ctx, rows), "applying the same rows again changes nothing")

	got, err := s.Edition(ctx, "d-1", "p#1", "en", 0)
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, "op2", got[0].Op)
	assert.Equal(t, rows[1], got[0], "a row reads back as it was written")
	assert.Equal(t, rows[0], got[1], "the operation kinds and the tool read back")

	recent, err := s.Edition(ctx, "d-1", "p#1", "en", 1)
	require.NoError(t, err)
	assert.Equal(t, []history.Row{rows[1]}, recent, "a limit keeps the most recent rows")

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

// TestAnOperationTakesThePlaceOfOneHoldingItsAddress: two logs that recorded
// one edit under different ids keep the older id once they merge. The older
// one's rows take the place of the rows the newer id projected, so the store
// ends with the rows a rebuild from the merged log writes.
func TestAnOperationTakesThePlaceOfOneHoldingItsAddress(t *testing.T) {
	s := openStore(t)
	ctx := t.Context()
	newer := history.Row{Op: "op9", Address: "edit:x", Doc: "d-1", Block: "p#1", Edition: "nb",
		Before: "r:1", After: "r:2", Origin: "observed", At: at(9)}
	require.NoError(t, s.Put(ctx, []history.Row{newer}))

	older := newer
	older.Op, older.Origin, older.At = "op1", "flow:up", at(1)
	require.NoError(t, s.Put(ctx, []history.Row{older}))

	got, err := s.Edition(ctx, "d-1", "p#1", "nb", 0)
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

	plan := strings.Join(history.ReachedPlan(t, s), "\n")
	assert.Regexp(t, `SEARCH h USING (COVERING )?INDEX block_history_reached \(doc=\? AND block=\? AND edition=\? AND after=\?\)`, plan,
		"each revision is a search of the index on the revision it names")
	assert.NotContains(t, plan, "SCAN h", "no lookup walks the document's history")
	assert.NotContains(t, plan, "SCAN o")
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

func TestLatestIsTheLastChangeToEachEdition(t *testing.T) {
	ctx := t.Context()
	s := openStore(t)
	at := time.Date(2026, 10, 3, 0, 0, 0, 0, time.UTC)
	row := func(op, block, edition, after string) history.Row {
		return history.Row{Op: op, Address: "a-" + op, Doc: "d-1", Block: block, Edition: edition,
			Before: "absent", After: after, Actor: "tool", At: at}
	}
	require.NoError(t, s.Put(ctx, []history.Row{row("op-1", "p", "fr", "r:1"), row("op-1", "q", "fr", "r:2")}))
	require.NoError(t, s.Put(ctx, []history.Row{row("op-2", "p", "fr", "r:3")}))
	require.NoError(t, s.Put(ctx, []history.Row{row("op-3", "p", "de", "r:4")}))
	require.NoError(t, s.Put(ctx, []history.Row{{Op: "op-4", Address: "a-other", Doc: "d-2", Block: "p", Edition: "fr", After: "r:9", At: at}}))

	tests := []struct {
		name     string
		editions []string
		want     map[string]string
	}{
		{name: "every edition", want: map[string]string{"p@de": "r:4", "p@fr": "r:3", "q@fr": "r:2"}},
		{name: "one edition", editions: []string{"fr"}, want: map[string]string{"p@fr": "r:3", "q@fr": "r:2"}},
		{name: "two editions", editions: []string{"fr", "de"}, want: map[string]string{"p@de": "r:4", "p@fr": "r:3", "q@fr": "r:2"}},
		{name: "an edition nobody changed", editions: []string{"ja"}, want: map[string]string{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := s.Latest(ctx, "d-1", tt.editions...)
			require.NoError(t, err)
			have := map[string]string{}
			for _, r := range got {
				have[r.Block+"@"+r.Edition] = r.After
				assert.Equal(t, "a-"+r.Op, r.Address, "the row carries its operation's address")
			}
			assert.Equal(t, tt.want, have)

			plan := strings.Join(history.LatestPlan(t, s, tt.editions...), "\n")
			assert.Regexp(t, `SEARCH h USING (COVERING )?INDEX sqlite_autoindex_block_history_1 \(doc=\? AND block=\? AND edition=\? AND op=\?\)`, plan,
				"each edition's most recent change is one seek of the primary key")
			assert.NotContains(t, plan, "CORRELATED", "no subquery runs once per row of the history")
		})
	}
}
