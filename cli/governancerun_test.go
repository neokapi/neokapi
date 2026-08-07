package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// twoCollectionFixture writes a project with two named content collections —
// one file each — and the `pseudo` flow. When platformChannel is non-empty the
// platform collection sits at its own point in the context space, which is the
// "one repo, two products" recipe; empty leaves both collections at the
// project's default point.
// Returns the recipe path, the two source files (framework first), and the root.
func twoCollectionFixture(t *testing.T, platformChannel string) (recipe string, sources []string, root string) {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)

	require.NoError(t, os.WriteFile(filepath.Join(dir, "voice.yaml"),
		[]byte("id: house\nname: House Style\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "platform-voice.yaml"),
		[]byte("id: platform\nname: Platform Voice\n"), 0o644))

	platform := project.Collection{
		Name: "platform",
		Content: []project.ContentItem{{
			Path:   "platform/en/*.json",
			Format: &project.FormatSpec{Name: "json"},
			Target: "platform/{lang}/*.json",
		}},
	}
	platform.Channel = platformChannel

	proj := &project.KapiProject{
		Version: project.CurrentVersion,
		Name:    "TwoBrands",
		Defaults: project.Defaults{
			SourceLanguage:  "en-US",
			TargetLanguages: []model.LocaleID{"fr-FR"},
			Voice:           &project.VoiceBinding{ProfileFile: "voice.yaml"},
		},
		Profiles: map[string]project.Profile{
			"platform": {
				Channels: []project.Channel{{ID: "app"}, {ID: "docs"}},
				Voice:    &project.VoiceBinding{ProfileFile: "platform-voice.yaml"},
			},
		},
		Collections: []project.Collection{
			{
				Name: "framework",
				Content: []project.ContentItem{{
					Path:   "framework/en/*.json",
					Format: &project.FormatSpec{Name: "json"},
					Target: "framework/{lang}/*.json",
				}},
			},
			platform,
		},
		Flows: map[string]*flow.StepsSpec{
			"pseudo": {Steps: []flow.FlowStep{{Tool: "pseudo-translate"}}},
		},
	}
	recipe = filepath.Join(dir, project.RecipeFileName)
	require.NoError(t, project.Save(recipe, proj))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, project.StateDirName), 0o755))

	for _, rel := range []string{"framework/en/messages.json", "platform/en/messages.json"} {
		abs := filepath.Join(dir, filepath.FromSlash(rel))
		require.NoError(t, os.MkdirAll(filepath.Dir(abs), 0o755))
		require.NoError(t, os.WriteFile(abs, []byte(`{"greeting":"Hello, world."}`), 0o644))
		sources = append(sources, abs)
	}
	return recipe, sources, dir
}

// TestRunFlowAllLocales_UngovernedCollectionsRunOnce is the regression guard on
// per-collection context: a project whose collections name no context of their
// own must still run each locale pass ONCE over the whole input set — one
// "Running for <locale>" state event, one tool chain — not one run per
// collection. The run count is the observable.
func TestRunFlowAllLocales_UngovernedCollectionsRunOnce(t *testing.T) {
	a := processOnlyApp(t)
	recipe, sources, root := twoCollectionFixture(t, "")

	rec := &recordingSink{}
	res, err := a.RunFlowAllLocales(t.Context(), FlowRunOptions{
		FlowName:    "pseudo",
		ProjectPath: recipe,
		InputPaths:  sources,
	}, rec.sink())
	require.NoError(t, err)
	require.Equal(t, [][]string{{"en-US", "qps"}}, res.LocalePasses)
	assert.Equal(t, 2, res.FilesProcessed)

	// One "running" event plus exactly one pass event: a single grouped run.
	states := rec.byType(FlowEventState)
	require.Len(t, states, 2, "collections naming no context must not split the run: %+v", states)
	assert.Equal(t, "running", states[0].Message)
	assert.Contains(t, states[1].Message, "Running for qps (2 files)",
		"the single pass covers every input file")

	for _, rel := range []string{"framework/qps/messages.json", "platform/qps/messages.json"} {
		assert.FileExists(t, filepath.Join(root, filepath.FromSlash(rel)))
	}
}

// TestRunFlowAllLocales_GovernedCollectionSplitsTheRun is the same run with one
// collection placed at another point in the context space: the pass splits into
// one run per point, each carrying its own bindings, and every file is still
// processed exactly once.
func TestRunFlowAllLocales_GovernedCollectionSplitsTheRun(t *testing.T) {
	a := processOnlyApp(t)
	recipe, sources, root := twoCollectionFixture(t, "platform/app")

	rec := &recordingSink{}
	res, err := a.RunFlowAllLocales(t.Context(), FlowRunOptions{
		FlowName:    "pseudo",
		ProjectPath: recipe,
		InputPaths:  sources,
	}, rec.sink())
	require.NoError(t, err)
	assert.Equal(t, 2, res.FilesProcessed, "each file runs exactly once")

	states := rec.byType(FlowEventState)
	require.Len(t, states, 3, "one pass event per context group: %+v", states)
	assert.Equal(t, "running", states[0].Message)
	for _, ev := range states[1:] {
		assert.Contains(t, ev.Message, "Running for qps (1 files)",
			"each group covers only its own collection's files")
	}

	done := rec.byType(FlowEventFileDone)
	require.Len(t, done, 2)
	for _, rel := range []string{"framework/qps/messages.json", "platform/qps/messages.json"} {
		assert.FileExists(t, filepath.Join(root, filepath.FromSlash(rel)))
	}
}
