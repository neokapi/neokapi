package host

import (
	"context"
	"maps"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/change"
)

// TestApplyTermEntry_SetsDoNotTranslate: a term operation that names
// do_not_translate sets the flag on its concept in the project's terms store.
// One that omits the flag leaves it, and one that names false clears it.
func TestApplyTermEntry_SetsDoNotTranslate(t *testing.T) {
	a, cmd, root, _ := newApplyAssetProject(t)
	ctx := context.Background()

	apply := func(op map[string]any) {
		t.Helper()
		res, err := applyJSON(t, a, cmd, changeSetOf(t, op), ApplyOptions{})
		require.NoError(t, err)
		assert.Equal(t, change.SetApplied, res.Status)
	}
	flag := func() bool {
		t.Helper()
		db, err := a.ProjectDB(ctx, root)
		require.NoError(t, err)
		concepts, err := db.Terms().Concepts(ctx)
		require.NoError(t, err)
		require.Len(t, concepts, 1)
		return concepts[0].DoNotTranslate
	}
	term := func(extra map[string]any) map[string]any {
		op := map[string]any{"op": "term", "action": "upsert", "term": "kapi", "locale": "en", "status": "preferred"}
		maps.Copy(op, extra)
		return op
	}

	apply(term(map[string]any{"do_not_translate": true}))
	assert.True(t, flag(), "the operation sets the flag")

	apply(term(nil))
	assert.True(t, flag(), "an operation that omits the flag leaves it")

	apply(term(map[string]any{"do_not_translate": false}))
	assert.False(t, flag(), "an operation that names false clears it")
}

// --dry-run writes nothing: a term operation is previewed, the store is left
// as it was, and the same change set without --dry-run lands it.
func TestApplyDryRunWritesNoAssetOperation(t *testing.T) {
	a, cmd, root, _ := newApplyAssetProject(t)
	ctx := context.Background()
	body := changeSetOf(t, map[string]any{"op": "term", "action": "upsert", "term": "dashboard", "locale": "en", "status": "preferred"})

	_, stderr, err := runApply(t, a, cmd, body, ApplyOptions{DryRun: true})
	require.NoError(t, err)
	assert.Contains(t, stderr, "change set previewed: 1 previewed")

	res, err := applyJSON(t, a, cmd, body, ApplyOptions{DryRun: true})
	require.NoError(t, err)
	assert.Equal(t, change.SetPreviewed, res.Status)
	assert.Equal(t, change.OpPreviewed, res.Ops[0].Status)

	db, err := a.ProjectDB(ctx, root)
	require.NoError(t, err)
	concepts, err := db.Terms().Concepts(ctx)
	require.NoError(t, err)
	assert.Empty(t, concepts, "--dry-run wrote the term")

	res, err = applyJSON(t, a, cmd, body, ApplyOptions{})
	require.NoError(t, err)
	assert.Equal(t, change.OpApplied, res.Ops[0].Status, "%+v", res.Ops[0].Error)
	concepts, err = db.Terms().Concepts(ctx)
	require.NoError(t, err)
	assert.Len(t, concepts, 1)
}
