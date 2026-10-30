package host

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/plugin/manifest"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/host/pluginhost"
)

// TestShare_ViaGitKeepsTheRemoteAndRef: --via git through a recipe whose git
// block names its own remote and ref keeps both, since --via has no way to
// name them.
func TestShare_ViaGitKeepsTheRemoteAndRef(t *testing.T) {
	first, _, bare := sharedGitProject(t)
	app, _ := contextOpsApp(t)
	recipe, err := os.ReadFile(recipeOf(first))
	require.NoError(t, err)
	named := bytes.Replace(recipe, []byte("context:\n  backend: git\n"),
		[]byte("context:\n  backend: git\n  remote: origin\n  ref: refs/kapi/team\n"), 1)
	require.NoError(t, os.WriteFile(recipeOf(first), named, 0o600))
	proposeUtilise(t, app, first, person)

	res, err := app.Share(t.Context(), ShareRequest{Project: recipeOf(first), Via: ShareViaGit})
	require.NoError(t, err)
	assert.Equal(t, []ShareFile{{Path: "kapi.yaml", Action: ShareFileKept}}, res.Files)
	proj, err := project.Load(recipeOf(first))
	require.NoError(t, err)
	assert.Equal(t, project.ContextBackend{Backend: project.ContextBackendGit, Remote: "origin", Ref: "refs/kapi/team"}, *proj.Context)
	assert.Contains(t, shareText(t, res), "through git (origin refs/kapi/team)")
	runGit(t, first, "--git-dir", bare, "rev-parse", "--verify", "refs/kapi/team")
}

// TestKeepUnsetFields: the fields --via cannot name carry over from a block of
// the same kind, and a block of another kind carries nothing.
func TestKeepUnsetFields(t *testing.T) {
	s3 := project.ContextBackend{Backend: project.ContextBackendS3, Bucket: "acme", Prefix: "site/"}
	was := &project.ContextBackend{Backend: project.ContextBackendS3, Bucket: "old", Prefix: "old/", Region: "eu-north-1", Endpoint: "https://s3.example"}
	assert.Equal(t, project.ContextBackend{Backend: project.ContextBackendS3, Bucket: "acme", Prefix: "site/", Region: "eu-north-1", Endpoint: "https://s3.example"},
		keepUnsetFields(s3, was), "the bucket and prefix are the ones named; the region and endpoint are kept")

	git := project.ContextBackend{Backend: project.ContextBackendGit}
	assert.Equal(t, project.ContextBackend{Backend: project.ContextBackendGit, Remote: "upstream", Ref: "refs/kapi/team"},
		keepUnsetFields(git, &project.ContextBackend{Backend: project.ContextBackendGit, Remote: "upstream", Ref: "refs/kapi/team"}))
	assert.Equal(t, git, keepUnsetFields(git, was), "an S3 block gives a git block nothing")
	assert.Equal(t, s3, keepUnsetFields(s3, nil))
}

// standInBowrain installs a plugin named bowrain whose connect handler runs
// script, standing in for the kapi-bowrain binary.
func standInBowrain(t *testing.T, app *App, script string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the stand-in plugin is a shell script")
	}
	dir := t.TempDir()
	bin := filepath.Join(dir, "kapi-bowrain")
	require.NoError(t, os.WriteFile(bin, []byte("#!/bin/sh\n"+script+"\n"), 0o755))
	m := &manifest.Manifest{
		ManifestVersion: manifest.CurrentVersion,
		Plugin:          bowrainPlugin,
		Version:         "0.0.1",
		Binary:          "kapi-bowrain",
		Capabilities:    manifest.Capabilities{Commands: []manifest.Command{{Name: "push"}}},
	}
	require.NoError(t, m.Validate())
	app.PluginHost = pluginhost.NewHost([]*pluginhost.Plugin{{
		Dir: dir, BinaryPath: bin, Manifest: m, Source: pluginhost.Source{Order: 1, Label: "test", Path: dir},
	}}, nil)
	t.Cleanup(func() { app.PluginHost = nil })
}

// TestShare_ViaBowrain: with the plugin, --via bowrain runs its connect
// handler on the recipe it writes; a connect that fails takes away the recipe
// the share wrote, and the folder is left as it was.
func TestShare_ViaBowrain(t *testing.T) {
	app, _ := contextOpsApp(t)

	t.Run("the connect fails", func(t *testing.T) {
		standInBowrain(t, app, "exit 3")
		root := contentFolder(t)
		_, err := app.Share(t.Context(), ShareRequest{Project: recipeOf(root), Via: ShareViaBowrain, Location: "https://bowrain.example"})
		require.Error(t, err)
		assert.NoFileExists(t, recipeOf(root))
		assert.NoDirExists(t, filepath.Join(root, project.StateDirName))
	})

	t.Run("the connect writes its block", func(t *testing.T) {
		standInBowrain(t, app, `printf 'bowrain:\n  url: %s\n' "$4" >> kapi.yaml`)
		root := contentFolder(t)
		res, err := app.Share(t.Context(), ShareRequest{Project: recipeOf(root), Via: ShareViaBowrain, Location: "https://bowrain.example/acme/site"})
		require.NoError(t, err)
		assert.True(t, res.BowrainPlugin)
		assert.Equal(t, []ShareFile{{Path: "kapi.yaml", Action: ShareFileCreated}}, res.Files)
		assert.Equal(t, ShareRungBowrain, res.Rung)
		assert.Equal(t, "https://bowrain.example/acme/site", res.Server)
	})

	t.Run("no server", func(t *testing.T) {
		standInBowrain(t, app, "exit 0")
		root := contentFolder(t)
		_, err := app.Share(t.Context(), ShareRequest{Project: recipeOf(root), Via: ShareViaBowrain})
		require.ErrorContains(t, err, "kapi share --via bowrain <server-url>")
		assert.Equal(t, ExitUsage, ExitCode(nil, err))
		assert.NoFileExists(t, recipeOf(root))
	})
}
