//go:build !js && !windows

package host

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/workspace"
)

// syncRefused syncs and requires the approval refusal.
func syncRefused(t *testing.T, app *App, recipe string) error {
	t.Helper()
	_, err := app.SyncProjectContext(t.Context(), ContextSyncRequest{Project: recipe})
	require.Error(t, err)
	return err
}

// A folder inside the project that a pull replaces with a link to another
// folder is refused, and nothing is written where the link leads.
func TestContextStore_AFolderInTheProjectSwappedForALinkIsRefused(t *testing.T) {
	app, _ := contextOpsApp(t)
	app.isTTY = func() bool { return false }
	recipe := folderProject(t, app, "shared")
	root := filepath.Dir(recipe)
	require.NoError(t, os.Mkdir(filepath.Join(root, "shared"), 0o755))
	_, err := app.ApproveContextStore(recipe)
	require.NoError(t, err)

	outside := realTempDir(t)
	require.NoError(t, os.RemoveAll(filepath.Join(root, "shared")))
	require.NoError(t, os.Symlink(outside, filepath.Join(root, "shared")))

	err = syncRefused(t, app, recipe)
	assert.Contains(t, err.Error(), "is a link")
	assert.NoDirExists(t, filepath.Join(outside, "log"), "nothing is written where the link leads")
	_, err = app.ApproveContextStore(recipe)
	require.Error(t, err, "a folder in the project reached through a link cannot be allowed either")
}

// An absolute folder whose parent is replaced by a link leads somewhere
// else, which asks again; the prompt and the approval name the folder the
// path leads to.
func TestContextStore_AParentSwappedForALinkAsksAgain(t *testing.T) {
	app, _ := contextOpsApp(t)
	app.isTTY = func() bool { return false }
	base := realTempDir(t)
	approved := filepath.Join(base, "share")
	require.NoError(t, os.Mkdir(approved, 0o755))
	recipe := folderProject(t, app, filepath.Join(base, "via", "team"))
	require.NoError(t, os.Symlink(approved, filepath.Join(base, "via")))

	location, err := ContextStoreLocation(recipe)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(approved, "team"), location, "the location shown is the resolved one")
	_, err = app.ApproveContextStore(recipe)
	require.NoError(t, err)
	_, err = app.SyncProjectContext(t.Context(), ContextSyncRequest{Project: recipe})
	require.NoError(t, err)
	assert.DirExists(t, filepath.Join(approved, "team"))

	elsewhere := realTempDir(t)
	require.NoError(t, os.Remove(filepath.Join(base, "via")))
	require.NoError(t, os.Symlink(elsewhere, filepath.Join(base, "via")))
	assert.False(t, ContextStoreAllowed(recipe))
	err = syncRefused(t, app, recipe)
	require.ErrorIs(t, err, ErrContextStoreNotApproved)
	assert.NoDirExists(t, filepath.Join(elsewhere, "team"))
}

// The approval is bound to the project: another repository at the same path
// naming the same folder asks again.
func TestContextStore_AnotherProjectAtTheSamePathAsksAgain(t *testing.T) {
	app, _ := contextOpsApp(t)
	app.isTTY = func() bool { return false }
	folder := filepath.Join(realTempDir(t), "team")
	recipe := folderProject(t, app, folder)
	_, err := app.ApproveContextStore(recipe)
	require.NoError(t, err)
	require.True(t, ContextStoreAllowed(recipe))

	proj, err := project.Load(recipe)
	require.NoError(t, err)
	proj.ID = projectIDFor("someone-elses")
	require.NoError(t, project.Save(recipe, proj))

	assert.False(t, ContextStoreAllowed(recipe))
	require.ErrorIs(t, syncRefused(t, app, recipe), ErrContextStoreNotApproved)
}

// A path starting with ~ or naming an environment variable is refused, since
// kapi expands neither.
func TestContextStore_AHomeOrVariablePathIsRefused(t *testing.T) {
	app, _ := contextOpsApp(t)
	for _, path := range []string{"~/team", "$HOME/team", "${TEAM_SHARE}/kapi"} {
		t.Run(path, func(t *testing.T) {
			recipe := folderProject(t, app, path)
			err := syncRefused(t, app, recipe)
			assert.Contains(t, err.Error(), "kapi expands neither")
			assert.NoDirExists(t, filepath.Join(filepath.Dir(recipe), "~"))
		})
	}
}

// An approval the desktop sends names the folder its dialog showed; when the
// recipe leads elsewhere by the click, nothing is recorded.
func TestContextStore_AnApprovalOfAnotherFolderIsRefused(t *testing.T) {
	app, _ := contextOpsApp(t)
	shown := filepath.Join(realTempDir(t), "team")
	recipe := folderProject(t, app, shown)
	proj, err := project.Load(recipe)
	require.NoError(t, err)
	proj.Context.Path = filepath.Join(realTempDir(t), "other")
	require.NoError(t, project.Save(recipe, proj))

	_, err = app.ApproveContextStoreAt(recipe, shown)
	require.ErrorContains(t, err, "not the folder shown")
	assert.False(t, ContextStoreAllowed(recipe))
}

// A sync through a folder holding a link says what it passed over, in the
// result and its text, rather than reporting an empty folder.
func TestContextStore_ASyncReportsWhatItPassedOver(t *testing.T) {
	app, _ := contextOpsApp(t)
	folder := filepath.Join(realTempDir(t), "team")
	require.NoError(t, os.MkdirAll(filepath.Join(folder, "log", "w1"), 0o755))
	planted := "log/w1/" + strings.Repeat("0", 24) + ".jsonl"
	require.NoError(t, os.Symlink(filepath.Join(realTempDir(t), "elsewhere"), filepath.Join(folder, filepath.FromSlash(planted))))
	recipe := folderProject(t, app, folder)
	_, err := app.ApproveContextStore(recipe)
	require.NoError(t, err)

	res, err := app.SyncProjectContext(t.Context(), ContextSyncRequest{Project: recipe})
	require.NoError(t, err)
	require.NotNil(t, res.Pull)
	assert.Equal(t, []string{planted}, res.Pull.PassedOver)
	var text bytes.Buffer
	require.NoError(t, res.Pull.FormatText(&text))
	assert.Contains(t, text.String(), "passed over 1 entry in the folder that kapi does not read, such as a link: \""+planted+"\"")
	data, err := json.Marshal(res.Pull)
	require.NoError(t, err)
	assert.Contains(t, string(data), `"passed_over":["`+planted+`"],"passed_over_total":1`)
}

// The names a sync passed over come from whoever can write the folder: each
// is quoted so a control character never reaches the terminal, and a folder
// with many of them prints a few and the count.
func TestContextStore_PassedOverNamesAreQuotedAndCapped(t *testing.T) {
	app, _ := contextOpsApp(t)
	folder := filepath.Join(realTempDir(t), "team")
	dir := filepath.Join(folder, "log", "w1")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	target := filepath.Join(realTempDir(t), "elsewhere")
	escape := "\x1b[2J\x1b[1;1Hpushed 0 \u00b7 pulled 0"
	require.NoError(t, os.Symlink(target, filepath.Join(dir, "a"+escape)))
	for i := range 199 {
		require.NoError(t, os.Symlink(target, filepath.Join(dir, fmt.Sprintf("b-%03d", i))))
	}
	recipe := folderProject(t, app, folder)
	_, err := app.ApproveContextStore(recipe)
	require.NoError(t, err)

	res, err := app.SyncProjectContext(t.Context(), ContextSyncRequest{Project: recipe})
	require.NoError(t, err)
	require.NotNil(t, res.Pull)
	assert.Equal(t, 200, res.Pull.PassedOverTotal)
	assert.Len(t, res.Pull.PassedOver, workspace.PassedOverShown)

	var text bytes.Buffer
	require.NoError(t, res.Pull.FormatText(&text))
	assert.NotContains(t, text.String(), "\x1b", "no control character reaches the terminal")
	assert.Contains(t, text.String(), `"log/w1/a\x1b[2J\x1b[1;1Hpushed 0 · pulled 0"`)
	assert.Contains(t, text.String(), "passed over 200 entries")
	assert.Contains(t, text.String(), ", and 190 more\n")

	data, err := json.Marshal(res.Pull)
	require.NoError(t, err)
	assert.Contains(t, string(data), `"passed_over_total":200`)
}

// A folder that is not there yet is announced with the ancestor it will be
// created under, created by the first sync, and covered by the approval as
// the directory that exists afterwards.
func TestContextStore_AFolderNotThereYetIsCreatedUnderItsAncestor(t *testing.T) {
	app, _ := contextOpsApp(t)
	base := realTempDir(t)
	folder := filepath.Join(base, "teams", "docs")
	recipe := folderProject(t, app, folder)
	layout, err := project.LayoutFor(recipe)
	require.NoError(t, err)

	fp, err := contextStoreFingerprint(layout, folder)
	require.NoError(t, err)
	assert.Equal(t, base, fp.Existing)
	var prompt bytes.Buffer
	printContextStorePrompt(&prompt, recipe, fp)
	assert.Contains(t, prompt.String(), "The folder does not exist yet. The first sync will create it under \""+base+"\".")

	_, err = app.ApproveContextStore(recipe)
	require.NoError(t, err)
	assert.NoDirExists(t, folder, "approving creates nothing")
	_, err = app.SyncProjectContext(t.Context(), ContextSyncRequest{Project: recipe})
	require.NoError(t, err)
	assert.DirExists(t, filepath.Join(folder, "log"))
	assert.True(t, ContextStoreAllowed(recipe), "the approval covers the directory created")

	after, err := contextStoreFingerprint(layout, folder)
	require.NoError(t, err)
	assert.Equal(t, folder, after.Existing)
	assert.NotNil(t, after.Info)
}
