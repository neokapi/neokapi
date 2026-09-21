package host_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/host"
)

// A by-location answer for a file an item claims only for its comments answers
// for the file's content: no collection governs it, and the point and voice are
// the project's default ones. That holds for an item declared
// `comments: {only: true}`, and for one that declares the comments of a file no
// reader parses. A Declared request answers for the blocks the project reads
// from the file, which are all comments, at the point the comments sit at.
func TestContextAnswersForAFilesContentPastACommentsOnlyItem(t *testing.T) {
	t.Setenv("KAPI_CONFIG_DIR", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("KAPI_PLUGINS_DIR_ONLY", "1")
	t.Setenv("KAPI_PLUGINS_DIR", t.TempDir())
	root := t.TempDir()
	write := func(rel, body string) {
		path := filepath.Join(root, filepath.FromSlash(rel))
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
	}
	write("kapi.yaml", `version: v1
name: acme
defaults:
  source_language: en
  voice: .kapi/voice.yaml
profiles:
  source:
    channels: [comments]
collections:
  - name: config-comments
    channel: source/comments
    source_only: true
    content:
      - path: "config/*.yaml"
        comments:
          only: true
  - name: code-comments
    channel: source/comments
    source_only: true
    content:
      - path: "code/*.go"
        comments: true
`)
	write(".kapi/voice.yaml", "name: Acme\n")
	write(".kapi/profiles/source/voice.yaml", "name: Source comments\n")
	write("config/app.yaml", "# The greeting.\ngreeting: Hello\n")
	write("code/parse.go", "package code\n\n// Parse reads the input.\nfunc Parse() {}\n")
	recipe := filepath.Join(root, "kapi.yaml")
	t.Setenv("KAPI_PROJECT", recipe)
	t.Chdir(root)

	// A by-location answer resolves its voice from the project store, so the
	// fixture's profiles reach it through the explicit import.
	readProjectContext(t, root)

	proj, err := project.Load(recipe)
	require.NoError(t, err)
	defaults, err := proj.ResolveGovernanceFor(project.GovernancePoint{})
	require.NoError(t, err)

	at := func(t *testing.T, req host.ContextPointRequest) host.ContextPointSources {
		t.Helper()
		cmd := host.NewEnvCommand(t.Context(), "context")
		host.AddProjectFlag(cmd)
		host.AddResourceFlags(cmd)
		require.NoError(t, cmd.Flags().Set("project", recipe))
		src, done := (&host.App{}).ContextSourcesAt(cmd, req)
		t.Cleanup(done)
		require.NotNil(t, src.Governance)
		return src
	}

	for _, file := range []struct{ path, collection string }{
		{"config/app.yaml", "config-comments"},
		{"code/parse.go", "code-comments"},
	} {
		t.Run(file.path, func(t *testing.T) {
			content := at(t, host.ContextPointRequest{Path: file.path})
			assert.Empty(t, content.Collection, "no collection governs the file's content")
			assert.Equal(t, defaults.Ref().String(), content.Governance.Ref().String())
			if assert.NotNil(t, content.Voice) {
				assert.Equal(t, "Acme", content.Voice.Name)
			}

			declared := at(t, host.ContextPointRequest{Path: file.path, Declared: true})
			assert.Equal(t, file.collection, declared.Collection, "every block read from the file is a comment")
			assert.Equal(t, "source/comments", declared.Governance.Ref().String())
			if assert.NotNil(t, declared.Voice) {
				assert.Equal(t, "Source comments", declared.Voice.Name)
			}
		})
	}
}
