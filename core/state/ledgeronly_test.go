package state_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/state"
)

// Reading and writing a project's decisions with no checkout in hand.
//
// A project's context store lives in a workspace, outside every checkout, and
// a project registered on a machine may have no working tree on it. What that
// project has decided is therefore a question about the LEDGER: a view belongs
// to a checkout, and there is none to ask.

func TestOpenLedger_ReadsWhatEveryCheckoutRecorded(t *testing.T) {
	db := sharedDB(t)
	ctx := t.Context()

	// Two checkouts of one project, each recording in its own view.
	first, err := state.OpenWorkFromDB(ctx, db, filepath.Join(t.TempDir(), "units"))
	require.NoError(t, err)
	require.NoError(t, first.Put(ctx, unit("u1", "d-intro", "Alpha")))

	second, err := state.OpenWorkFromDB(ctx, db, filepath.Join(t.TempDir(), "units"))
	require.NoError(t, err)
	require.NoError(t, second.Put(ctx, unit("u2", "d-intro", "Bravo")))

	one, err := first.All(ctx)
	require.NoError(t, err)
	assert.Len(t, one, 1, "a checkout sees what it recorded")

	ledger, err := state.OpenLedger(ctx, db)
	require.NoError(t, err)
	all, err := ledger.Ledger(ctx)
	require.NoError(t, err)
	require.Len(t, all, 2, "the ledger holds what every checkout recorded")
	assert.Equal(t, "u1", all[0].Unit)
	assert.Equal(t, "u2", all[1].Unit)

	// The same reading from a handle that does have a checkout, so a caller
	// asking the ledger gets one answer whichever handle it holds.
	fromCheckout, err := first.Ledger(ctx)
	require.NoError(t, err)
	assert.Equal(t, all, fromCheckout)
}

func TestOpenLedger_LeavesEveryCheckoutViewAlone(t *testing.T) {
	db := sharedDB(t)
	ctx := t.Context()
	committed := filepath.Join(t.TempDir(), "units")

	checkout, err := state.OpenWorkFromDB(ctx, db, committed)
	require.NoError(t, err)
	require.NoError(t, checkout.Put(ctx, unit("u1", "d-intro", "Alpha")))

	ledger, err := state.OpenLedger(ctx, db)
	require.NoError(t, err)
	require.NoError(t, ledger.Record(ctx, unit("u2", "d-intro", "Bravo")))

	held, err := checkout.All(ctx)
	require.NoError(t, err)
	require.Len(t, held, 1, "a decision recorded with no checkout answers for none")
	assert.Equal(t, "u1", held[0].Unit)

	all, err := ledger.Ledger(ctx)
	require.NoError(t, err)
	assert.Len(t, all, 2, "and it is in the ledger all the same")

	// Nothing about the process's own directory is treated as a record: a
	// handle with no checkout reads none and writes none.
	require.NoError(t, ledger.Import(ctx))
	require.NoError(t, ledger.Commit(ctx))
	cwd, err := os.Getwd()
	require.NoError(t, err)
	entries, err := os.ReadDir(cwd)
	require.NoError(t, err)
	for _, e := range entries {
		assert.NotEqual(t, state.CommittedExt, filepath.Ext(e.Name()),
			"a ledger handle wrote a record into the working directory")
	}
}

func TestOpenLedger_LeavesOutARevokedPairing(t *testing.T) {
	db := sharedDB(t)
	ctx := t.Context()

	checkout, err := state.OpenWorkFromDB(ctx, db, filepath.Join(t.TempDir(), "units"))
	require.NoError(t, err)
	require.NoError(t, checkout.Put(ctx, unit("u1", "d-intro", "Alpha")))
	require.NoError(t, checkout.Put(ctx, unit("u2", "d-intro", "Bravo")))
	require.NoError(t, checkout.Delete(ctx, nbKey("d-intro", "u1")))

	ledger, err := state.OpenLedger(ctx, db)
	require.NoError(t, err)
	all, err := ledger.Ledger(ctx)
	require.NoError(t, err)
	require.Len(t, all, 1, "a withdrawn decision is not what the project has decided")
	assert.Equal(t, "u2", all[0].Unit)
}

func TestOpenLedger_NeedsADatabase(t *testing.T) {
	_, err := state.OpenLedger(t.Context(), nil)
	require.Error(t, err)
}
