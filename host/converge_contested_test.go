package host

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A rebuild has to reproduce the wording it started from.
//
// Two collections of one product are reviewed apart and absorbed together, so
// the corpus holds two approved Norwegian answers for one English string. When
// the answer was chosen by how often the corpus repeated each spelling, a cold
// rebuild of the smaller collection came back reworded in the larger one's
// voice — on shipped CLI help — and adding or removing an unrelated string
// anywhere in the project could flip it back.
//
// The fixture is that shape at its smallest. The engine's answer is the more
// repeated one AND the one that sorts first, so it wins under the count and
// under the tie-break alike; only resolving by where each was approved leaves
// each collection with its own reviewed wording.
func newContestedConvergeProject(t *testing.T) (*App, *EnvCommand, string) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("KAPI_CONFIG_DIR", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("KAPI_PLUGINS_DIR_ONLY", "1")
	t.Setenv("KAPI_PLUGINS_DIR", t.TempDir())
	t.Setenv("KAPI_NO_PROJECT", "1")

	for _, sub := range []string{"cli", "engine"} {
		require.NoError(t, os.MkdirAll(filepath.Join(dir, sub), 0o755))
	}
	write := func(rel, content string) {
		require.NoError(t, os.WriteFile(filepath.Join(dir, rel), []byte(content), 0o644))
	}
	// "Recycle" is answered one way in the CLI's catalog and another in the
	// engine's; "Cancel" is pending in both, which is what leaves the locale
	// short of its gate so a pass runs and both targets are written again.
	write("cli/en.json", `{"recycle":"Recycle","cancel":"Cancel"}`)
	write("cli/nb.json", `{"recycle":"Gjenbruk"}`)
	write("engine/en.json", `{"a":"Recycle","b":"Recycle","c":"Recycle","cancel":"Cancel"}`)
	write("engine/nb.json", `{"a":"Bruk om igjen","b":"Bruk om igjen","c":"Bruk om igjen"}`)

	proj := &project.KapiProject{
		Version: project.CurrentVersion,
		Name:    "ContestedConvergeTest",
		Defaults: project.Defaults{
			SourceLanguage:  "en",
			TargetLanguages: []model.LocaleID{"nb"},
			Flow:            "recycle-only",
			SourceGate:      string(model.SourceGateNone),
			Materialize:     project.MaterializeOnConverge,
		},
		Profiles: map[string]project.Profile{
			"neokapi": {Channels: []project.Channel{{ID: "cli"}, {ID: "engine"}}},
		},
		Collections: []project.Collection{
			{Name: "neokapi-cli", Channel: "neokapi/cli", Path: "cli/en.json", Target: "cli/{lang}.json"},
			{Name: "neokapi-engine", Channel: "neokapi/engine", Path: "engine/en.json", Target: "engine/{lang}.json"},
		},
		Flows: map[string]*flow.StepsSpec{
			"recycle-only": {Steps: []flow.FlowStep{
				{Tool: "recycle", Config: map[string]any{"fillTarget": true, "fillTargetThreshold": 100}},
			}},
		},
	}
	recipe := filepath.Join(dir, project.RecipeFileName)
	require.NoError(t, project.Save(recipe, proj))

	t.Chdir(dir)
	a := &App{}
	a.InitRegistries()
	a.SourceLang = "en"
	cmd := NewEnvCommand(context.Background(), "up")
	a.AddFlowRunFlags(cmd)
	AddUpFlags(cmd)
	AddProjectFlag(cmd)
	require.NoError(t, cmd.Flags().Set("project", recipe))
	return a, cmd, recipe
}

// TestConverge_ColdRebuildKeepsEachCollectionsOwnWording is the drill's finding
// as a regression test: a rebuild from a cold store must give each collection
// back the wording its own reviewer approved, not the one the corpus repeats.
func TestConverge_ColdRebuildKeepsEachCollectionsOwnWording(t *testing.T) {
	a, cmd, recipe := newContestedConvergeProject(t)
	root := filepath.Dir(recipe)

	runFreshConverge(t, a, cmd, recipe)

	cli, err := os.ReadFile(filepath.Join(root, "cli", "nb.json"))
	require.NoError(t, err)
	assert.Contains(t, string(cli), "Gjenbruk",
		"the CLI keeps the wording approved in the CLI's own collection")
	assert.NotContains(t, string(cli), "Bruk om igjen",
		"the engine's wording, repeated three times over, does not reach the CLI")

	engine, err := os.ReadFile(filepath.Join(root, "engine", "nb.json"))
	require.NoError(t, err)
	assert.Contains(t, string(engine), "Bruk om igjen")
	assert.NotContains(t, string(engine), "Gjenbruk")
}

// TestConverge_ColdRebuildIsByteForByteReproducible: the loop's premise is that
// a reviewed decision is durable, so running it twice from a cold store has to
// produce the same bytes. It is the property the repeat count broke — the
// winner moved with the corpus rather than with anyone's decision, so a rebuild
// could silently replace approved text with other approved text.
func TestConverge_ColdRebuildIsByteForByteReproducible(t *testing.T) {
	a, cmd, recipe := newContestedConvergeProject(t)
	root := filepath.Dir(recipe)
	targets := []string{filepath.Join(root, "cli", "nb.json"), filepath.Join(root, "engine", "nb.json")}

	runFreshConverge(t, a, cmd, recipe)
	first := map[string]string{}
	for _, p := range targets {
		b, err := os.ReadFile(p)
		require.NoError(t, err)
		first[p] = string(b)
	}

	// Cold again: the derived store is deleted, so the second run reads the
	// committed record from nothing, exactly as a fresh clone does.
	a.Shutdown()
	require.NoError(t, os.RemoveAll(project.LayoutAt(root).WorkDir()))
	b, cmd2, _ := reopenContestedProject(t, root, recipe)
	runFreshConverge(t, b, cmd2, recipe)

	for _, p := range targets {
		after, err := os.ReadFile(p)
		require.NoError(t, err)
		assert.Equal(t, first[p], string(after), "a cold rebuild reproduces the file byte for byte")
	}
}

// reopenContestedProject opens a second App over the same tree, the way a later
// invocation of the CLI does.
func reopenContestedProject(t *testing.T, root, recipe string) (*App, *EnvCommand, string) {
	t.Helper()
	a := &App{}
	a.InitRegistries()
	a.SourceLang = "en"
	cmd := NewEnvCommand(context.Background(), "up")
	a.AddFlowRunFlags(cmd)
	AddUpFlags(cmd)
	AddProjectFlag(cmd)
	require.NoError(t, cmd.Flags().Set("project", recipe))
	t.Chdir(root)
	return a, cmd, recipe
}
