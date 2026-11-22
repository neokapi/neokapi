package mcptools

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
)

// An output_path is a tool argument an agent writes: over MCP it must lie
// inside the project, and one outside is refused with nothing written.
func TestHandleRunFlowRefusesAnOutputPathOutsideTheProject(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "input"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "input", "test.json"), []byte(`{"greeting": "Hello"}`), 0o644))
	kapiPath := filepath.Join(dir, project.RecipeFileName)
	require.NoError(t, project.Save(kapiPath, &project.KapiProject{
		Version:     project.CurrentVersion,
		Defaults:    project.Defaults{SourceLanguage: "en-US", TargetLanguages: []model.LocaleID{"qps"}},
		Collections: []project.Collection{{Path: "input/*.json"}},
	}))
	outside := t.TempDir()

	run := func(out string) error {
		_, _, err := handleRunFlow(t.Context(), testApp(), RunFlowInput{
			FlowName: "pseudo-translate", Project: kapiPath, TargetLang: "qps", OutputPath: out,
		})
		return err
	}

	t.Run("a path outside the project", func(t *testing.T) {
		out := filepath.Join(outside, "out.json")
		err := run(out)
		require.ErrorContains(t, err, "output_path")
		require.ErrorContains(t, err, "lies outside the project")
		assert.NoFileExists(t, out)
	})

	t.Run("a path that climbs out of the project", func(t *testing.T) {
		out := filepath.Join(dir, "..", filepath.Base(outside), "climb.json")
		require.ErrorContains(t, run(out), "lies outside the project")
		assert.NoFileExists(t, filepath.Join(outside, "climb.json"))
	})

	t.Run("a path beneath a link out of the project", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("creating a symbolic link needs a privilege on Windows")
		}
		require.NoError(t, os.Symlink(outside, filepath.Join(dir, "site")))
		err := run(filepath.Join(dir, "site", "linked.json"))
		require.ErrorContains(t, err, "output_path")
		require.ErrorContains(t, err, "leads outside the root")
		assert.NoFileExists(t, filepath.Join(outside, "linked.json"))
	})

	t.Run("a path inside the project is written", func(t *testing.T) {
		out := filepath.Join(dir, "out", "qps.json")
		require.NoError(t, run(out))
		assert.FileExists(t, out)
	})
}
