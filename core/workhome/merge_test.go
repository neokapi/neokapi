package workhome_test

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/change/filehome"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/workspace"
)

// Two machines that write one edition from one head, and then merge their
// logs, hold every write in one id order. The fold reads that order, so both
// reach one head whichever log received which write first; the rebase then
// carries over the writes that changed other blocks, and a write that
// changed a block the other machine changed too stays listed as a conflict.

// mergeInto records every operation from holds into into, with the blobs
// they name, as a sync pull does, and folds them into into's projection.
func mergeInto(t *testing.T, into, from *machine) {
	t.Helper()
	ctx := context.Background()
	ops, err := from.ws.Ops(ctx, 0, 0)
	require.NoError(t, err)
	for _, op := range ops {
		for _, ref := range workspace.BlobRefs(op) {
			data, err := from.ws.Blob(ctx, ref)
			require.NoError(t, err)
			_, err = into.ws.PutBlob(ctx, data)
			require.NoError(t, err)
		}
	}
	arriving := make([]workspace.Op, len(ops))
	for i, op := range ops {
		op.Seq = 0
		arriving[i] = op
	}
	workspace.SortOps(arriving)
	_, err = into.ws.Record(ctx, arriving...)
	require.NoError(t, err)
	require.NoError(t, into.p.CatchUp(ctx))
}

// twoMachines are two workspaces whose project holds a.json, with its German
// edition drafted on the first and merged into the second.
func twoMachines(t *testing.T, blocks map[string]string) (*keptFixture, *keptFixture) {
	t.Helper()
	english := map[string]string{}
	for key := range blocks {
		english[key] = "English " + key
	}
	data, err := json.Marshal(english)
	require.NoError(t, err)
	source := string(data) + "\n"
	a := newKeptFixture(t, newMachine(t, t.TempDir()), map[string]string{"a.json": source}, filehome.Options{})
	b := newKeptFixture(t, newMachine(t, t.TempDir()), map[string]string{"a.json": source}, filehome.Options{})
	a.draft(t, "a.json", blocks)
	mergeInto(t, b.m, a.m)
	require.Equal(t, dumpHeads(t, a.m), dumpHeads(t, b.m), "the second machine starts from the first one's head")
	return a, b
}

// setGerman sets the German edition of one block of a.json, against the
// revision a read of the machine finds.
func setGerman(t *testing.T, f *keptFixture, block, text string) {
	t.Helper()
	ctx := context.Background()
	page, err := f.svc.Read(ctx, change.ReadRequest{Doc: "de/a.json", Blocks: []string{block}})
	require.NoError(t, err)
	require.Len(t, page.Blocks, 1)
	res, err := f.svc.Apply(ctx, change.Set{Ops: []change.Op{{Kind: change.KindSetContent, At: page.Blocks[0].Ref,
		IfMatch: page.Blocks[0].Rev, Body: &change.SetContent{Text: &text}}}}, person)
	require.NoError(t, err)
	require.Equal(t, change.SetApplied, res.Status, "%+v", res.Ops)
}

// german reads the German edition of a.json on a machine, by block.
func german(t *testing.T, f *keptFixture) map[string]string {
	t.Helper()
	kept, err := f.m.home.Edition(context.Background(), "a.json", model.EditionKey{Locale: "de"})
	require.NoError(t, err)
	out := map[string]string{}
	for key, ed := range kept.Blocks {
		out[key] = model.RunsText(ed.Runs)
	}
	return out
}

// TestWorkspaceHome_ConcurrentWritesMergeInEitherOrder is the property WP8
// asks of the workspace home: concurrent writes to one edition merged in
// either order reach the same head.
func TestWorkspaceHome_ConcurrentWritesMergeInEitherOrder(t *testing.T) {
	ctx := context.Background()
	a, b := twoMachines(t, map[string]string{"greeting": "Hallo", "farewell": "Tschüss", "thanks": "Danke"})

	// Each machine writes on its own: a block of its own, and one block both
	// write.
	setGerman(t, a, "greeting", "Guten Tag")
	setGerman(t, b, "farewell", "Auf Wiedersehen")
	setGerman(t, a, "thanks", "Vielen Dank")
	setGerman(t, b, "thanks", "Danke schön")

	// Each log receives the other's writes, in opposite orders.
	aBefore, bBefore := snapshotLog(t, a.m), snapshotLog(t, b.m)
	copyBlobs(t, b.m, a.m)
	copyBlobs(t, a.m, b.m)
	slices.Reverse(bBefore)
	mergeLog(t, b.m, aBefore)
	mergeLog(t, a.m, bBefore)
	require.Equal(t, dumpHeads(t, a.m), dumpHeads(t, b.m), "both machines reach one head from the same writes")

	// The rebase carries the write to the other block over, on both machines,
	// and the same rebase made twice is one operation once the logs merge.
	na, err := a.m.p.RebaseWorkspace(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, na, "one write changed a block the other machine left alone")
	// A pull applies what it merged through the projector's syncer, which
	// rebases the same way.
	require.NoError(t, b.m.p.Syncer().Apply(ctx, false))
	assert.Equal(t, german(t, a), german(t, b), "both machines carried the same write over")
	mergeInto(t, a.m, b.m)
	mergeInto(t, b.m, a.m)
	require.Equal(t, dumpHeads(t, a.m), dumpHeads(t, b.m), "the rebased head is one head on both machines")

	got := german(t, a)
	assert.Equal(t, "Guten Tag", got["greeting"], "a write to a block of its own lands")
	assert.Equal(t, "Auf Wiedersehen", got["farewell"], "on either machine")
	assert.Contains(t, []string{"Vielen Dank", "Danke schön"}, got["thanks"], "the write that sorts first holds the shared block")

	conflicts, err := a.m.home.Conflicts(ctx)
	require.NoError(t, err)
	require.Len(t, conflicts, 1, "the other write to the shared block is a conflict, not a silent loss")
	assert.Equal(t, []string{"thanks"}, conflicts[0].Blocks)
	other, err := b.m.home.Conflicts(ctx)
	require.NoError(t, err)
	assert.Equal(t, conflicts, other, "and both machines list it")
}

// snapshotLog reads every operation a machine's log holds.
func snapshotLog(t *testing.T, m *machine) []workspace.Op {
	t.Helper()
	ops, err := m.ws.Ops(context.Background(), 0, 0)
	require.NoError(t, err)
	return ops
}

// mergeLog merges ops, read from from's log, into into in the order given.
func mergeLog(t *testing.T, into *machine, ops []workspace.Op) {
	t.Helper()
	ctx := context.Background()
	for _, op := range ops {
		op.Seq = 0
		_, err := into.ws.Record(ctx, op)
		require.NoError(t, err)
	}
	require.NoError(t, into.p.CatchUp(ctx))
}

// TestWorkspaceHome_RandomWritesConvergeInAnyMergeOrder writes random
// changes to one edition on three machines, merges every log into every other
// in a random order, and asserts the machines agree: on the head, on the
// writes that did not advance it, and, after each rebases and they merge
// again, on the rebased head.
func TestWorkspaceHome_RandomWritesConvergeInAnyMergeOrder(t *testing.T) {
	blocks := []string{"a", "b", "c", "d"}
	for seed := range uint64(6) {
		t.Run(fmt.Sprintf("seed %d", seed), func(t *testing.T) {
			ctx := context.Background()
			rng := rand.New(rand.NewPCG(seed, seed*7+1))
			start := map[string]string{}
			for _, k := range blocks {
				start[k] = "Anfang " + k
			}
			first, second := twoMachines(t, start)
			third := newKeptFixture(t, newMachine(t, t.TempDir()), map[string]string{"a.json": mustRead(t, first, "a.json")}, filehome.Options{})
			mergeInto(t, third.m, first.m)
			machines := []*keptFixture{first, second, third}

			for i := range 9 {
				f := machines[rng.IntN(len(machines))]
				setGerman(t, f, blocks[rng.IntN(len(blocks))], fmt.Sprintf("Fassung %d", i))
			}

			logs := make([][]workspace.Op, len(machines))
			for i, f := range machines {
				logs[i] = snapshotLog(t, f.m)
			}
			for i, f := range machines {
				order := rng.Perm(len(machines))
				for _, j := range order {
					if j == i {
						continue
					}
					ops := slices.Clone(logs[j])
					rng.Shuffle(len(ops), func(x, y int) { ops[x], ops[y] = ops[y], ops[x] })
					copyBlobs(t, f.m, machines[j].m)
					mergeLog(t, f.m, ops)
				}
			}
			want := dumpHeads(t, first.m)
			for _, f := range machines[1:] {
				require.Equal(t, want, dumpHeads(t, f.m), "every machine folds the same writes to the same head")
			}

			for _, f := range machines {
				_, err := f.m.p.RebaseWorkspace(ctx)
				require.NoError(t, err)
			}
			for _, f := range machines {
				for _, g := range machines {
					if f != g {
						mergeInto(t, f.m, g.m)
					}
				}
			}
			want = dumpHeads(t, first.m)
			for _, f := range machines[1:] {
				require.Equal(t, want, dumpHeads(t, f.m), "every machine reaches the same rebased head")
			}
		})
	}
}

// copyBlobs copies every blob from's log names into into.
func copyBlobs(t *testing.T, into, from *machine) {
	t.Helper()
	ctx := context.Background()
	for _, op := range snapshotLog(t, from) {
		for _, ref := range workspace.BlobRefs(op) {
			data, err := from.ws.Blob(ctx, ref)
			require.NoError(t, err)
			_, err = into.ws.PutBlob(ctx, data)
			require.NoError(t, err)
		}
	}
}

func mustRead(t *testing.T, f *keptFixture, name string) string {
	t.Helper()
	page, err := f.svc.Read(context.Background(), change.ReadRequest{Doc: name})
	require.NoError(t, err)
	texts := map[string]string{}
	for _, b := range page.Blocks {
		texts[b.Ref.Block] = b.Text
	}
	data, err := json.Marshal(texts)
	require.NoError(t, err)
	return string(data) + "\n"
}
