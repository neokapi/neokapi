//go:build unix

package project

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
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
	require.NoError(t, os.WriteFile(filepath.Join(l.SyncDir(), "sync-cache.json"), []byte(`{"claim_token":"clm_earlier","server_url":"https://app.example","workspace":"team","project_id":"p1"}`), 0o600))
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
		earlier := earlierCheckout(t, root, "version: v1\nname: app\nid: prj_same\nbowrain:\n  url: https://app.example/team/p1\n")
		rec, err := dataroot.ReadRecord(earlier)
		require.NoError(t, err)
		require.NotNil(t, rec.Project, "the record names the project")
		assert.Equal(t, dataroot.CheckoutProject{ID: "prj_same", Name: "app"}, *rec.Project)

		later := laterCheckout(t, root, "version: v1\nname: app renamed\nid: prj_same\nbowrain:\n  url: https://APP.example/team/p1/\n")
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

// The sync state is bound to one server project. Two checkouts that share a
// recipe id bring it across only when the recipe connects to the server
// project it is bound to.
func TestRelinkBringsTheSyncStateOnlyToItsServerProject(t *testing.T) {
	captureStateNotices(t)
	targets := func(files []StateFile) []string {
		var out []string
		for _, f := range files {
			out = append(out, f.Target)
		}
		return out
	}

	t.Run("a recipe that connects to another server project leaves the sync state, whatever the person states", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "app")
		earlier := earlierCheckout(t, root, "version: v1\nname: app\nid: prj_same\nbowrain:\n  url: https://app.example/team/p1\n")
		later := laterCheckout(t, root, "version: v1\nname: app fork\nid: prj_same\nbowrain:\n  url: https://app.example/team/p2\n")
		for _, opts := range []RelinkOptions{{}, {SameProject: true}} {
			plan, err := PlanRelink(later, root, opts)
			require.NoError(t, err)
			assert.False(t, plan.Unverified, "the ids match")
			assert.Equal(t, []string{"sync/sync-cache.json"}, targets(plan.Withheld))
			assert.Contains(t, targets(plan.Files), "vault/redaction.json", "the vault of the same project comes across")
			require.NotNil(t, plan.SyncProject)
			assert.Equal(t, "https://app.example project team/p1", plan.SyncProject.String())
			require.Len(t, plan.WithheldWhy, 1)
			assert.Contains(t, plan.WithheldWhy[0], "bound to https://app.example project team/p1, and this recipe connects to https://app.example project team/p2")
			assert.False(t, plan.RemovesSource)
		}
		plan, err := PlanRelink(later, root, RelinkOptions{SameProject: true})
		require.NoError(t, err)
		require.NoError(t, ApplyTransfer(later, plan, false))
		assert.FileExists(t, later.RedactionVaultPath())
		assert.NoFileExists(t, filepath.Join(later.SyncDir(), "sync-cache.json"), "the sync state did not cross")
		assert.FileExists(t, filepath.Join(earlier, SyncDirName, "sync-cache.json"))
	})

	t.Run("a recipe that connects to no server project leaves the sync state unless asked", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "app")
		earlier := earlierCheckout(t, root, "version: v1\nname: app\nid: prj_same\n")
		later := laterCheckout(t, root, "version: v1\nname: app\nid: prj_same\n")
		plan, err := PlanRelink(later, root, RelinkOptions{})
		require.NoError(t, err)
		assert.Equal(t, []string{"sync/sync-cache.json"}, targets(plan.Withheld))
		require.Len(t, plan.WithheldWhy, 1)
		assert.Contains(t, plan.WithheldWhy[0], "connects to no server project")
		require.NoError(t, ApplyTransfer(later, plan, false))
		assert.NoFileExists(t, filepath.Join(later.SyncDir(), "sync-cache.json"))
		assert.FileExists(t, filepath.Join(earlier, SyncDirName, "sync-cache.json"))

		asked, err := PlanRelink(later, root, RelinkOptions{SameProject: true})
		require.NoError(t, err)
		assert.Empty(t, asked.Withheld)
		require.NoError(t, ApplyTransfer(later, asked, false))
		assert.FileExists(t, filepath.Join(later.SyncDir(), "sync-cache.json"))
	})
}

// A transfer that fails part way leaves the checkout's state as it was and
// says what to do; running it again starts over.
func TestATransferThatStopsPartWayRemovesWhatItWrote(t *testing.T) {
	captureStateNotices(t)
	root := filepath.Join(t.TempDir(), "app")
	l := laterCheckout(t, root, "version: v1\nname: app\n")
	in := filepath.Join(root, StateDirName)
	require.NoError(t, os.MkdirAll(filepath.Join(in, VaultDirName), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(in, VaultDirName, "redaction.json"), []byte(`{"v":1}`), 0o600))
	require.NoError(t, os.MkdirAll(filepath.Join(in, SyncDirName), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(in, SyncDirName, "sync-cache.json"), []byte(`{"s":1}`), 0o600))
	plan, err := PlanImport(l)
	require.NoError(t, err)
	require.Len(t, plan.Files, 2)
	require.NoError(t, EnsureLayout(l))
	// The sync directory of the state cannot be written, so the second file
	// fails after the first was written.
	syncDir := filepath.Join(l.LocalStateDir(), SyncDirName)
	require.NoError(t, os.MkdirAll(syncDir, 0o500))
	t.Cleanup(func() { _ = os.Chmod(syncDir, 0o700) })

	err = ApplyTransfer(l, plan, false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "could not be written")
	assert.Contains(t, err.Error(), "nothing was taken")
	assert.Contains(t, err.Error(), "run the command again")
	assert.NoFileExists(t, l.RedactionVaultPath(), "the copy already written was removed")
	assert.FileExists(t, filepath.Join(in, VaultDirName, "redaction.json"), "the source is kept")

	require.NoError(t, os.Chmod(syncDir, 0o700))
	again, err := PlanImport(l)
	require.NoError(t, err)
	require.NoError(t, ApplyTransfer(l, again, false), "running it again starts over")
	assert.FileExists(t, l.RedactionVaultPath())
}

// The workspace is part of the server project a sync state is bound to, and a
// cache that does not record it cannot say.
func TestRelinkComparesTheWorkspace(t *testing.T) {
	captureStateNotices(t)
	syncCache := func(t *testing.T, earlier, body string) {
		t.Helper()
		require.NoError(t, os.WriteFile(filepath.Join(earlier, SyncDirName, "sync-cache.json"), []byte(body), 0o600))
	}
	withheld := func(plan StateTransfer) bool {
		for _, f := range plan.Withheld {
			if f.Target == "sync/sync-cache.json" {
				return true
			}
		}
		return false
	}

	t.Run("another workspace on the same server", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "app")
		earlier := earlierCheckout(t, root, "version: v1\nname: app\nid: prj_same\n")
		syncCache(t, earlier, `{"server_url":"https://app.example","workspace":"ws-a","project_id":"app"}`)
		later := laterCheckout(t, root, "version: v1\nname: app\nid: prj_same\nbowrain:\n  url: https://app.example/ws-b/app\n")
		plan, err := PlanRelink(later, root, RelinkOptions{SameProject: true})
		require.NoError(t, err)
		assert.True(t, withheld(plan), "another workspace keeps the sync state, whatever the person states")
		assert.Contains(t, plan.WithheldWhy[0], "ws-a/app")
	})

	t.Run("the same workspace", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "app")
		earlier := earlierCheckout(t, root, "version: v1\nname: app\nid: prj_same\n")
		syncCache(t, earlier, `{"server_url":"https://app.example","workspace":"ws-a","project_id":"app"}`)
		later := laterCheckout(t, root, "version: v1\nname: app\nid: prj_same\nbowrain:\n  url: https://app.example/ws-a/app\n")
		plan, err := PlanRelink(later, root, RelinkOptions{})
		require.NoError(t, err)
		assert.False(t, withheld(plan))
	})

	t.Run("a cache that records no workspace cannot say", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "app")
		earlier := earlierCheckout(t, root, "version: v1\nname: app\nid: prj_same\n")
		syncCache(t, earlier, `{"server_url":"https://app.example","project_id":"app"}`)
		later := laterCheckout(t, root, "version: v1\nname: app\nid: prj_same\nbowrain:\n  url: https://app.example/ws-a/app\n")
		plan, err := PlanRelink(later, root, RelinkOptions{})
		require.NoError(t, err)
		assert.True(t, withheld(plan))
		assert.Contains(t, plan.WithheldWhy[0], "one of them names no workspace")
		asked, err := PlanRelink(later, root, RelinkOptions{SameProject: true})
		require.NoError(t, err)
		assert.False(t, withheld(asked), "--same-project takes it")
	})

	t.Run("servers compare through the host's normalisation", func(t *testing.T) {
		prev := ServerURLNormalizer
		ServerURLNormalizer = func(s string) string { return strings.TrimSuffix(strings.ToLower(strings.TrimRight(s, "/")), ":443") }
		t.Cleanup(func() { ServerURLNormalizer = prev })
		root := filepath.Join(t.TempDir(), "app")
		earlier := earlierCheckout(t, root, "version: v1\nname: app\nid: prj_same\n")
		syncCache(t, earlier, `{"server_url":"https://app.example:443","workspace":"ws-a","project_id":"app"}`)
		later := laterCheckout(t, root, "version: v1\nname: app\nid: prj_same\nbowrain:\n  url: https://APP.example/ws-a/app\n")
		plan, err := PlanRelink(later, root, RelinkOptions{})
		require.NoError(t, err)
		assert.False(t, withheld(plan))
	})
}

// A temporary file a stopped write of the record left is not state, and no
// plan takes it.
func TestRelinkLeavesARecordTemporaryFile(t *testing.T) {
	captureStateNotices(t)
	root := filepath.Join(t.TempDir(), "app")
	earlier := earlierCheckout(t, root, "version: v1\nname: app\nid: prj_same\n")
	require.NoError(t, os.WriteFile(filepath.Join(earlier, ".checkout.json.123456.tmp"), []byte("{"), 0o600))
	later := laterCheckout(t, root, "version: v1\nname: app\nid: prj_same\n")
	plan, err := PlanRelink(later, root, RelinkOptions{SameProject: true})
	require.NoError(t, err)
	for _, f := range append(plan.Files, plan.Withheld...) {
		assert.NotContains(t, f.Source, ".checkout.json")
	}
}

// Each cell of the comparison table answers as the table says: only a value
// both sides name, equal after normalisation, is the same.
func TestCompareFieldFollowsTheTable(t *testing.T) {
	lower := strings.ToLower
	for _, tt := range []struct {
		name             string
		recorded, recipe string
		want             fieldVerdict
	}{
		{"both name it, equal", "A", "a", fieldSame},
		{"both name it, different", "a", "b", fieldDiffers},
		{"only the sync state names it", "a", "", fieldUnknown},
		{"only the recipe names it", "", "a", fieldUnknown},
		{"neither names it", "", "", fieldUnknown},
	} {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, compareField(tt.recorded, tt.recipe, lower))
		})
	}
}

// The server, the workspace and the project are decided by the one rule: a
// field that differs makes another server project, every field the same makes
// one, and anything else cannot say.
func TestBindingVerdictDecidesEveryFieldByOneRule(t *testing.T) {
	full := SyncBinding{Server: "https://app.example", Workspace: "ws", Project: "p"}
	for _, tt := range []struct {
		name             string
		recorded, recipe SyncBinding
		same, known      bool
	}{
		{"every field the same", full, full, true, true},
		{"another server", full, SyncBinding{Server: "https://other.example", Workspace: "ws", Project: "p"}, false, true},
		{"another workspace", full, SyncBinding{Server: "https://app.example", Workspace: "ws2", Project: "p"}, false, true},
		{"another project", full, SyncBinding{Server: "https://app.example", Workspace: "ws", Project: "q"}, false, true},
		{"the recipe names no workspace", full, SyncBinding{Server: "https://app.example", Project: "p"}, false, false},
		{"the sync state names no workspace", SyncBinding{Server: "https://app.example", Project: "p"}, full, false, false},
		{"neither names a workspace", SyncBinding{Server: "https://app.example", Project: "p"}, SyncBinding{Server: "https://app.example", Project: "p"}, false, false},
		{"a difference outweighs a field nobody names", SyncBinding{Server: "https://app.example", Project: "p"}, SyncBinding{Server: "https://app.example", Workspace: "ws", Project: "q"}, false, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			same, known := bindingVerdict(tt.recorded, tt.recipe)
			assert.Equal(t, tt.same, same, "same")
			assert.Equal(t, tt.known, known, "known")
		})
	}
}
