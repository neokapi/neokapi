package host

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/ref/refcache"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDeletingWorkKeepsTheCheckoutsLocalState is item 7 of the edit model's
// release list: `.kapi/work/` is a cache. The redaction vault, the venue sync
// state and the personal filters sit beside it, so deleting it and running
// again loses none of them, and the vault keeps its protection.
func TestDeletingWorkKeepsTheCheckoutsLocalState(t *testing.T) {
	a, root, rec := redactingProject(t, "rules")
	ctx := t.Context()
	_, err := rec.Record(ctx, falconEdit("src/intro.en.json"))
	require.NoError(t, err)

	layout := project.LayoutAt(root)
	refs := refcache.Load(layout, "https://venue.test", "p1")
	refs.Consume("main", 42)
	require.NoError(t, refs.Save(layout))
	syncCache := filepath.Join(layout.SyncDir(), "sync-cache.json")
	require.NoError(t, os.WriteFile(syncCache, []byte(`{"claim_token":"claim-1"}`), 0o600))
	require.NoError(t, os.WriteFile(layout.LocalFiltersPath(), []byte(`{"filters":[]}`), 0o644))

	vault, err := os.ReadFile(layout.RedactionVaultPath())
	require.NoError(t, err)
	require.Contains(t, string(vault), "Falcon", "the edit withheld its value into the vault")
	info, err := os.Stat(layout.VaultDir())
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o700), info.Mode().Perm(), "the vault is owner-only")

	a.Shutdown()
	require.NoError(t, os.RemoveAll(layout.WorkDir()))

	a2, _ := freshApp(t, layout.RecipePath)
	rec2, err := a2.EditRecorder(ctx, root)
	require.NoError(t, err)
	_, err = rec2.Record(ctx, falconEdit("src/intro.en.json"))
	require.NoError(t, err)

	kept, err := os.ReadFile(layout.RedactionVaultPath())
	require.NoError(t, err, "the vault survives deleting work/")
	assert.Contains(t, string(kept), "Falcon", "with every withheld original")
	assert.Equal(t, int64(42), refcache.Load(layout, "https://venue.test", "p1").Ref("main").Content,
		"the position the checkout consumed survives")
	claim, err := os.ReadFile(syncCache)
	require.NoError(t, err, "the sync cache survives")
	assert.Contains(t, string(claim), "claim-1", "with the claim token nothing else holds")
	assert.FileExists(t, layout.LocalFiltersPath(), "the personal filters survive")
}
