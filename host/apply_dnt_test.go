package host

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestApplyTermEntry_SetsDoNotTranslate: a term entry that names
// do_not_translate sets the flag on its concept in the project's terms store.
// An entry that omits the flag leaves it, and one that names false clears it.
func TestApplyTermEntry_SetsDoNotTranslate(t *testing.T) {
	a, cmd, root, _ := newApplyAssetProject(t)
	ctx := context.Background()

	apply := func(line string) {
		t.Helper()
		path := filepath.Join(t.TempDir(), "changes.jsonl")
		require.NoError(t, os.WriteFile(path, []byte(line+"\n"), 0o644))
		_, err := captureStdout(t, func() error { return a.RunApply(cmd, path, false, "", true) })
		require.NoError(t, err)
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

	apply(`{"kind": "term", "op": "upsert", "term": "kapi", "locale": "en", "status": "preferred", "do_not_translate": true}`)
	assert.True(t, flag(), "the entry sets the flag")

	apply(`{"kind": "term", "op": "upsert", "term": "kapi", "locale": "en", "status": "preferred"}`)
	assert.True(t, flag(), "an entry that omits the flag leaves it")

	apply(`{"kind": "term", "op": "upsert", "term": "kapi", "locale": "en", "status": "preferred", "do_not_translate": false}`)
	assert.False(t, flag(), "an entry that names false clears it")
}
