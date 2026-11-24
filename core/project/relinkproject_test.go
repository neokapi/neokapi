//go:build unix

package project

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/dataroot"
)

// earlierCheckout writes a checkout at root with recipe, gives its state a
// vault, a sync state and saved filters, removes the checkout, and returns the
// state directory kapi recorded for it.
func earlierCheckout(t *testing.T, root, recipe string) string {
	t.Helper()
	require.NoError(t, os.MkdirAll(root, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, RecipeFileName), []byte(recipe), 0o644))
	l := LayoutAt(root)
	withVault(t, l, `{"originals":"of the earlier project"}`)
	_, err := EnsureLocalDir(l, SyncDirName)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(l.SyncDir(), "sync-cache.json"), []byte(`{"claim_token":"clm_earlier","server_url":"https://app.example"}`), 0o600))
	require.NoError(t, os.WriteFile(l.LocalFiltersPath(), []byte(`{"active":"mine"}`), 0o600))
	dir := l.LocalStateDir()
	require.NoError(t, os.RemoveAll(root))
	return dir
}

// laterCheckout writes another directory at root with recipe.
func laterCheckout(t *testing.T, root, recipe string) Layout {
	t.Helper()
	require.NoError(t, os.MkdirAll(root, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, RecipeFileName), []byte(recipe), 0o644))
	return LayoutAt(root)
}

// assertNothingCrossed checks that the later checkout's state holds no vault
// and no sync state, and that the earlier state directory still holds both.
func assertNothingCrossed(t *testing.T, later Layout, earlier string) {
	t.Helper()
	assert.NoFileExists(t, later.RedactionVaultPath())
	assert.NoFileExists(t, filepath.Join(later.SyncDir(), "sync-cache.json"))
	assert.FileExists(t, filepath.Join(earlier, VaultDirName, "redaction.json"))
	assert.FileExists(t, filepath.Join(earlier, SyncDirName, "sync-cache.json"))
}

func TestRelinkTakesOnlyThisProjectsState(t *testing.T) {
	captureStateNotices(t)

	t.Run("the same project is relinked and the earlier directory removed", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "app")
		earlier := earlierCheckout(t, root, "version: v1\nname: app\nid: prj_same\n")
		rec, err := dataroot.ReadRecord(earlier)
		require.NoError(t, err)
		require.NotNil(t, rec.Project, "the record names the project")
		assert.Equal(t, dataroot.CheckoutProject{ID: "prj_same", Name: "app"}, *rec.Project)

		later := laterCheckout(t, root, "version: v1\nname: app renamed\nid: prj_same\n")
		plan, err := PlanRelink(later, root, RelinkOptions{})
		require.NoError(t, err)
		assert.False(t, plan.Unverified)
		assert.Empty(t, plan.Withheld)
		assert.True(t, plan.RemovesSource, "the plan says the earlier directory goes")
		require.NoError(t, ApplyTransfer(later, plan, false))
		assert.FileExists(t, later.RedactionVaultPath())
		assert.FileExists(t, filepath.Join(later.SyncDir(), "sync-cache.json"))
		assert.NoDirExists(t, earlier)
	})

	t.Run("another project is refused, naming both, and nothing crosses", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "app")
		earlier := earlierCheckout(t, root, "version: v1\nname: billing\nid: prj_billing\n")
		later := laterCheckout(t, root, "version: v1\nname: docs\nid: prj_docs\n")
		_, err := PlanRelink(later, root, RelinkOptions{SameProject: true})
		require.Error(t, err)
		assert.Contains(t, err.Error(), `"billing" (prj_billing)`)
		assert.Contains(t, err.Error(), `"docs" (prj_docs)`)
		assertNothingCrossed(t, later, earlier)
	})

	t.Run("a recorded id the recipe no longer has is another project", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "app")
		earlier := earlierCheckout(t, root, "version: v1\nname: app\nid: prj_app\n")
		later := laterCheckout(t, root, "version: v1\nname: app\n")
		_, err := PlanRelink(later, root, RelinkOptions{})
		require.ErrorContains(t, err, "kapi does not take one project's state into another")
		assertNothingCrossed(t, later, earlier)
	})

	t.Run("two recipes with no id and different names are different projects", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "app")
		earlier := earlierCheckout(t, root, "version: v1\nname: billing\n")
		later := laterCheckout(t, root, "version: v1\nname: docs\n")
		_, err := PlanRelink(later, root, RelinkOptions{})
		require.ErrorContains(t, err, `belongs to "billing", and this checkout is "docs"`)
		assertNothingCrossed(t, later, earlier)
	})

	t.Run("a record with no project leaves the vault and the sync state unless asked", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "app")
		earlier := earlierCheckout(t, root, "version: v1\nname: app\nid: prj_app\n")
		// A record kapi wrote before it recorded the project.
		rec, err := dataroot.ReadRecord(earlier)
		require.NoError(t, err)
		rec.Project = nil
		data, err := json.Marshal(rec)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(filepath.Join(earlier, dataroot.CheckoutRecordFilename), data, 0o600))

		later := laterCheckout(t, root, "version: v1\nname: app\nid: prj_app\n")
		plan, err := PlanRelink(later, root, RelinkOptions{})
		require.NoError(t, err)
		assert.True(t, plan.Unverified)
		assert.Equal(t, "a project kapi did not record", plan.Project)
		assert.Equal(t, []string{"https://app.example"}, plan.Servers, "the servers are shown")
		var withheld []string
		for _, f := range plan.Withheld {
			withheld = append(withheld, f.Target)
		}
		assert.ElementsMatch(t, []string{"vault/redaction.json", "sync/sync-cache.json"}, withheld)
		require.Len(t, plan.Files, 1)
		assert.Equal(t, LocalFiltersFilename, plan.Files[0].Target)
		assert.False(t, plan.RemovesSource, "the directory that keeps the withheld files stays")
		require.NoError(t, ApplyTransfer(later, plan, false))
		assert.FileExists(t, later.LocalFiltersPath())
		assertNothingCrossed(t, later, earlier)

		asked, err := PlanRelink(later, root, RelinkOptions{SameProject: true})
		require.NoError(t, err)
		assert.True(t, asked.Unverified)
		assert.Empty(t, asked.Withheld)
		assert.True(t, asked.RemovesSource)
		require.NoError(t, ApplyTransfer(later, asked, false))
		assert.FileExists(t, later.RedactionVaultPath())
		assert.NoDirExists(t, earlier)
	})
}
