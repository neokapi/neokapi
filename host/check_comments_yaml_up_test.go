package host

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/memory"
	"github.com/neokapi/neokapi/memory/kmb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// yamlCommentsProject is a fresh-clone project whose one YAML file carries a
// comment and a translatable string, with the reviewed translation committed.
// comments sets `comments: true` on the item; remembered decides whether the
// committed content memory holds the translation.
func yamlCommentsProject(t *testing.T, comments, remembered bool) (*App, *EnvCommand, string) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("KAPI_CONFIG_DIR", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("KAPI_PLUGINS_DIR_ONLY", "1")
	t.Setenv("KAPI_PLUGINS_DIR", t.TempDir())
	t.Setenv("KAPI_NO_PROJECT", "1")

	require.NoError(t, os.MkdirAll(filepath.Join(dir, "src"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "src", "en.yaml"),
		[]byte("# Greeting shown on the start page.\ngreeting: Hello world # the first words\n"), 0o644))

	proj := &project.KapiProject{
		Version: project.CurrentVersion,
		Name:    "YAMLComments",
		Defaults: project.Defaults{
			SourceLanguage:  "en",
			TargetLanguages: []model.LocaleID{"nb"},
			Flow:            "recycle-only",
			SourceGate:      string(model.SourceGateNone),
			Materialize:     project.MaterializeOnConverge,
		},
		Collections: []project.Collection{{
			Name:    "app",
			Content: []project.ContentItem{{Path: "src/en.yaml", Target: "src/{lang}.yaml", Comments: project.ContentComments{Declared: comments}}},
		}},
		Flows: map[string]*flow.StepsSpec{
			"recycle-only": {Steps: []flow.FlowStep{
				{Tool: "recycle", Config: map[string]any{"fillTarget": true, "fillTargetThreshold": 100}},
			}},
		},
	}
	recipe := filepath.Join(dir, project.RecipeFileName)
	require.NoError(t, project.Save(recipe, proj))

	if remembered {
		stamp := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
		data, err := kmb.Marshal(kmb.FromModel([]memory.Entry{{
			ID:          "reviewed:greeting",
			HintSrcLang: "en",
			Variants: map[model.LocaleID][]model.Run{
				"en": {{Text: &model.TextRun{Text: "Hello world"}}},
				"nb": {{Text: &model.TextRun{Text: "Hei verden"}}},
			},
			CreatedAt: stamp,
			UpdatedAt: stamp,
		}}, nil))
		require.NoError(t, err)
		memDir := project.LayoutAt(dir).Export().MemoryDir()
		require.NoError(t, os.MkdirAll(memDir, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(memDir, "app-nb.memory.json"), data, 0o644))
		readProjectContext(t, dir)
	}

	t.Chdir(dir)
	a := &App{}
	a.InitRegistries()
	a.SourceLang = "en"
	cmd := NewEnvCommand(context.Background(), "up")
	a.AddFlowRunFlags(cmd)
	AddUpFlags(cmd)
	AddProjectFlag(cmd)
	require.NoError(t, cmd.Flags().Set("project", recipe))
	require.NoError(t, cmd.Flags().Set("fail-on-unknown", "true"))
	return a, cmd, recipe
}

// convergedYAML runs one convergence over a YAML comments project and returns
// what it wrote: the run's coverage and the target file, empty when none.
func convergedYAML(t *testing.T, comments, remembered bool) (map[string]int, []byte) {
	t.Helper()
	a, cmd, recipe := yamlCommentsProject(t, comments, remembered)
	out := runFreshConverge(t, a, cmd, recipe)
	require.Len(t, out.Locales, 1)
	target, err := os.ReadFile(filepath.Join(filepath.Dir(recipe), "src", "nb.yaml"))
	if err != nil {
		require.ErrorIs(t, err, os.ErrNotExist)
	}
	return out.Locales[0].Pct, target
}

// A file with a format reader is read by that reader whether or not its item
// sets `comments: true`: its strings converge exactly as they do without the
// key, through one read, with the comments kept where the skeleton holds them.
func TestUpConvergesAReaderFormatWithCommentsTrueUnchanged(t *testing.T) {
	t.Run("comments: true changes nothing kapi up writes", func(t *testing.T) {
		plainPct, plain := convergedYAML(t, false, true)
		keyedPct, keyed := convergedYAML(t, true, true)
		assert.Equal(t, string(plain), string(keyed), "the target is byte for byte what the item writes without the key")
		assert.Equal(t, plainPct, keyedPct)
		assert.Equal(t, 100, keyedPct["translated"])
		assert.Contains(t, string(keyed), "greeting: Hei verden")
		assert.Contains(t, string(keyed), "# Greeting shown on the start page.", "the skeleton keeps the head comment")
		assert.Contains(t, string(keyed), "# the first words", "the skeleton keeps the line comment")
	})

	t.Run("must fail: the comparison notices a run that converged nothing", func(t *testing.T) {
		_, converged := convergedYAML(t, true, true)
		_, unconverged := convergedYAML(t, true, false)
		assert.NotEqual(t, string(converged), string(unconverged),
			"a run with no translation to recycle writes something else, and the comparison has to see it")
	})
}
