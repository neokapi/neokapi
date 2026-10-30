package host

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/workspace"
)

// shareText renders a share the way `kapi share` prints it.
func shareText(t *testing.T, res *ShareResult) string {
	t.Helper()
	var b bytes.Buffer
	require.NoError(t, res.FormatText(&b))
	return b.String()
}

// contentFolder is a folder with one Markdown file and no recipe, the way a
// person's docs folder looks before kapi has been run in it.
func contentFolder(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "docs"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(root, "docs", "guide.md"), []byte("# Guide\n\nWe utilise the widget.\n"), 0o600))
	return root
}

// TestShare_NoPlacePrintsTheLadder: a folder whose recipe names no place, or
// that has no recipe at all, prints the three rungs with the first marked and
// writes nothing.
func TestShare_NoPlacePrintsTheLadder(t *testing.T) {
	app, _ := contextOpsApp(t)
	for _, tt := range []struct {
		name string
		root string
	}{
		{name: "no recipe", root: contentFolder(t)},
		{name: "a recipe with no place", root: contextOpsProject(t, "share-ladder")},
	} {
		t.Run(tt.name, func(t *testing.T) {
			before, _ := os.ReadFile(recipeOf(tt.root))
			res, err := app.Share(t.Context(), ShareRequest{Project: recipeOf(tt.root)})
			require.NoError(t, err)
			assert.Equal(t, ShareRungMachine, res.Rung)
			assert.Nil(t, res.Sync, "nothing to sync with")
			require.Len(t, res.Ladder, 3)
			assert.True(t, res.Ladder[0].Current)
			assert.False(t, res.Ladder[1].Current)
			assert.False(t, res.BowrainPlugin)

			out := shareText(t, res)
			assert.True(t, strings.HasPrefix(out, "How far your rules reach\n"), out)
			assert.Contains(t, out, "  * "+machineLabel())
			assert.Contains(t, out, "your rules, your assistants")
			assert.Contains(t, out, "    With your team       kapi share --via git | folder <path>\n")
			assert.Contains(t, out, "    Managed on Bowrain   kapi share --via bowrain <server-url>  (needs the kapi-bowrain plugin)\n")

			after, _ := os.ReadFile(recipeOf(tt.root))
			assert.Equal(t, before, after, "the ladder writes nothing")
		})
	}
}

// TestShare_ViaFolderWritesTheRecipeAndSyncs: --via folder on a folder with no
// recipe writes the draft kapi init writes with the `context:` block the
// desktop saves, and syncs once; a second share keeps the recipe and syncs
// again with nothing new.
func TestShare_ViaFolderWritesTheRecipeAndSyncs(t *testing.T) {
	app, _ := contextOpsApp(t)
	root := contentFolder(t)
	shared := filepath.Join(t.TempDir(), "team")

	res, err := app.Share(t.Context(), ShareRequest{Project: recipeOf(root), Via: ShareViaFolder, Location: shared})
	require.NoError(t, err)
	assert.Equal(t, []ShareFile{{Path: "kapi.yaml", Action: ShareFileCreated}}, res.Files)
	assert.Equal(t, ShareRungTeam, res.Rung)
	require.NotNil(t, res.Sync)
	require.NotNil(t, res.Sync.Pull)
	require.NotNil(t, res.Sync.Push)

	proj, err := project.Load(recipeOf(root))
	require.NoError(t, err)
	assert.NotEmpty(t, proj.ID, "the recipe is the one kapi init writes, with an id")
	assert.NotEmpty(t, proj.Collections, "and the collections it proposes")
	require.NotNil(t, proj.Context)
	assert.Equal(t, project.ContextBackend{Backend: project.ContextBackendFile, Path: shared}, *proj.Context,
		"the block the desktop's Share section saves for the same folder")

	out := shareText(t, res)
	assert.Equal(t, "kapi.yaml  created\nWith your team · through a shared folder ("+shared+")\n"+
		"  pushed none · pulled none · last sync just now\n", out)

	proposeUtilise(t, app, root, person)
	res, err = app.Share(t.Context(), ShareRequest{Project: recipeOf(root), Via: ShareViaFolder, Location: shared})
	require.NoError(t, err)
	assert.Equal(t, []ShareFile{{Path: "kapi.yaml", Action: ShareFileKept}}, res.Files)
	assert.Equal(t, 1, res.Sync.Push.Pushed)
	assert.Contains(t, shareText(t, res), "  pushed 1 change · pulled none · last sync just now\n")

	res, err = app.Share(t.Context(), ShareRequest{Project: recipeOf(root)})
	require.NoError(t, err)
	assert.Empty(t, res.Files)
	assert.Equal(t, 0, res.Sync.Push.Pushed, "running it twice changes nothing")
}

// TestShare_ARelativeFolderNamesTheSamePlace: a relative folder was typed
// against the working directory, and the recipe keeps it relative to itself.
func TestShare_ARelativeFolderNamesTheSamePlace(t *testing.T) {
	app, _ := contextOpsApp(t)
	parent := t.TempDir()
	root := filepath.Join(parent, "site")
	require.NoError(t, os.MkdirAll(root, 0o700))
	t.Chdir(parent)

	_, err := app.Share(t.Context(), ShareRequest{Project: recipeOf(root), Via: ShareViaFolder, Location: "team"})
	require.NoError(t, err)
	proj, err := project.Load(recipeOf(root))
	require.NoError(t, err)
	assert.Equal(t, "../team", proj.Context.Path)
	info, err := ResolveContextBackend(recipeOf(root))
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(parent, "team"), info.Location)
}

// TestShare_ViaGit shares through a ref of the project's own repository,
// pushed to a bare repository standing in for the hosting service.
func TestShare_ViaGit(t *testing.T) {
	first, _, bare := sharedGitProject(t)
	app, _ := contextOpsApp(t)
	// The recipe sharedGitProject writes already names git; take it out, so
	// the share is what writes it.
	recipe, err := os.ReadFile(recipeOf(first))
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(recipeOf(first), bytes.Replace(recipe, []byte("context:\n  backend: git\n"), nil, 1), 0o600))
	proposeUtilise(t, app, first, person)

	res, err := app.Share(t.Context(), ShareRequest{Project: recipeOf(first), Via: ShareViaGit})
	require.NoError(t, err)
	assert.Equal(t, []ShareFile{{Path: "kapi.yaml", Action: ShareFileUpdated}}, res.Files)
	proj, err := project.Load(recipeOf(first))
	require.NoError(t, err)
	assert.Equal(t, project.ContextBackend{Backend: project.ContextBackendGit}, *proj.Context)
	assert.Equal(t, 1, res.Sync.Push.Pushed)

	out := shareText(t, res)
	assert.Contains(t, out, "With your team · through git (origin refs/kapi/context)\n")
	assert.Contains(t, out, "  pushed 1 change · pulled none · last sync just now\n")

	runGit(t, first, "--git-dir", bare, "rev-parse", "--verify", "refs/kapi/context")

	status, err := app.Share(t.Context(), ShareRequest{Project: recipeOf(first), Status: true})
	require.NoError(t, err)
	assert.Nil(t, status.Sync, "--status does not sync")
	assert.Equal(t, "With your team · through git (origin refs/kapi/context)\n"+
		"  nothing to push · nothing to pull · last sync just now\n", shareText(t, status))
}

// TestShare_RefusesWhatItCannotUse: a place that needs a value the person did
// not give, a place this folder cannot use and a name that is no place each
// fail before anything is written.
func TestShare_RefusesWhatItCannotUse(t *testing.T) {
	app, _ := contextOpsApp(t)
	for _, tt := range []struct {
		name string
		req  ShareRequest
		want string
		code int
	}{
		{name: "a folder with no path", req: ShareRequest{Via: ShareViaFolder}, want: "kapi share --via folder <path>"},
		{name: "s3 with no bucket", req: ShareRequest{Via: ShareViaS3}, want: "kapi share --via s3 s3://<bucket>/<prefix>"},
		{name: "git outside a repository", req: ShareRequest{Via: ShareViaGit}, want: "is not in a git repository"},
		{name: "git with a value", req: ShareRequest{Via: ShareViaGit, Location: "origin"}, want: "--via git takes no value"},
		{name: "no such place", req: ShareRequest{Via: "ftp"}, want: `--via "ftp" is not a place to share through; use git, folder, s3, bowrain`},
		{name: "a value with no place", req: ShareRequest{Location: "/tmp/x"}, want: "give it with --via"},
		{name: "bowrain without the plugin", req: ShareRequest{Via: ShareViaBowrain, Location: "https://bowrain.example"},
			want: "needs the kapi-bowrain plugin, which is not installed: install it with `kapi plugin install bowrain` or `brew install neokapi/tap/bowrain-cli`",
			code: ExitError},
		{name: "a place with --status", req: ShareRequest{Via: ShareViaFolder, Location: "/tmp/x", Status: true}, want: "--status"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			tt.req.Project = recipeOf(root)
			_, err := app.Share(t.Context(), tt.req)
			require.ErrorContains(t, err, tt.want)
			code := tt.code
			if code == 0 {
				code = ExitUsage
			}
			assert.Equal(t, code, ExitCode(nil, err))
			assert.NoFileExists(t, recipeOf(root), "nothing is written")
		})
	}
}

// TestShare_JSON: --json gives the rung, the place and the sync as data.
func TestShare_JSON(t *testing.T) {
	app, _ := contextOpsApp(t)
	root := contextOpsProject(t, "share-json")
	shared := filepath.Join(t.TempDir(), "team")
	res, err := app.Share(t.Context(), ShareRequest{Project: recipeOf(root), Via: ShareViaFolder, Location: shared})
	require.NoError(t, err)
	data, err := json.Marshal(res)
	require.NoError(t, err)
	var got map[string]any
	require.NoError(t, json.Unmarshal(data, &got))
	assert.Equal(t, "team", got["rung"])
	assert.Equal(t, false, got["bowrain_plugin"])
	assert.Equal(t, map[string]any{"kind": "file", "location": shared, "from": "recipe"}, got["backend"])
	assert.Equal(t, []any{map[string]any{"path": "kapi.yaml", "action": "updated"}}, got["files"])
	sync, ok := got["sync"].(map[string]any)
	require.True(t, ok, "the sync is in the data: %s", data)
	assert.Contains(t, sync, "pull")
	assert.Contains(t, sync, "push")
}

// TestShare_ARecipeConnectedToBowrain reads the plugin's block, which a
// binary without the plugin keeps as an unknown key, as the third rung.
func TestShare_ARecipeConnectedToBowrain(t *testing.T) {
	app, _ := contextOpsApp(t)
	root := contextOpsProject(t, "share-bowrain")
	recipe, err := os.ReadFile(recipeOf(root))
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(recipeOf(root), append(recipe, []byte("bowrain:\n  url: https://bowrain.example/acme/site\n")...), 0o600))

	res, err := app.Share(t.Context(), ShareRequest{Project: recipeOf(root)})
	require.NoError(t, err)
	assert.Equal(t, ShareRungBowrain, res.Rung)
	assert.Nil(t, res.Ladder)
	assert.Equal(t, "Managed on Bowrain · https://bowrain.example/acme/site\n"+
		"  send with kapi push · bring back with kapi pull\n", shareText(t, res))
}

// TestShareResult_When reads the time of the last sync in a person's words.
func TestShareResult_When(t *testing.T) {
	now := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
	r := &ShareResult{now: now}
	for _, tt := range []struct {
		ago  time.Duration
		want string
	}{
		{ago: 10 * time.Second, want: "just now"},
		{ago: time.Minute, want: "1 minute ago"},
		{ago: 5 * time.Minute, want: "5 minutes ago"},
		{ago: 3 * time.Hour, want: "3 hours ago"},
	} {
		assert.Equal(t, tt.want, r.when(now.Add(-tt.ago)))
	}
	assert.Equal(t, "never", r.when(time.Time{}))
	assert.True(t, strings.HasPrefix(r.when(now.Add(-72*time.Hour)), "on "))

	r.Status = &workspace.SyncStatus{ToPush: 2}
	assert.Equal(t, "2 changes to push · never synced", r.statusLine())
}
