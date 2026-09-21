package host

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/convergence"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/registry"
)

// A recipe can declare collections in formats a plugin supplies. On a machine
// without the plugin, `kapi up` sets those collections aside, says so in the
// plan, in the run's output and on its event stream, and converges the rest. A
// run over nothing it can read converges nothing and never reports success.

// declarePluginCollections adds the two shapes the lead reproduced to the
// self-seed project: a source-only collection in `sourcecode`, and a translated
// collection in `okf_idml` whose Norwegian target is already committed. When
// keepApp is false the readable JSON collection is removed.
func declarePluginCollections(t *testing.T, recipe string, keepApp bool) {
	t.Helper()
	root := filepath.Dir(recipe)
	write := func(rel, content string) {
		t.Helper()
		path := filepath.Join(root, filepath.FromSlash(rel))
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	}
	write("cask/kapi.rb", "cask \"kapi\" do\n  desc \"Content engine\"\nend\n")
	write("pkg/doc.idml", "<doc>Hello world</doc>\n")
	write("pkg/doc.nb.idml", "<doc>Hei verden</doc>\n")

	proj, err := project.Load(recipe)
	require.NoError(t, err)
	if !keepApp {
		proj.Collections = nil
	}
	proj.Collections = append(proj.Collections,
		project.Collection{
			Name:    "cask",
			Content: []project.ContentItem{{Path: "cask/*.rb", Format: &project.FormatSpec{Name: "sourcecode"}}},
		},
		project.Collection{
			Name: "layout",
			Content: []project.ContentItem{{
				Path: "pkg/doc.idml", Target: "pkg/doc.{lang}.idml",
				Format: &project.FormatSpec{Name: "okf_idml"},
			}},
		},
	)
	require.NoError(t, project.Save(recipe, proj))
}

func requireNoReader(t *testing.T, warnings []check.Warning, file, format, plugin string) {
	t.Helper()
	for _, w := range warnings {
		if w.Code == check.WarningFormatNoReader && w.Source == file {
			assert.Contains(t, w.Message, `"`+format+`"`)
			assert.Contains(t, w.Message, "(kapi plugins install "+plugin+")")
			return
		}
	}
	t.Fatalf("no format.no_reader warning names %s: %+v", file, warnings)
}

// The run converges the readable collection, sets the two plugin collections
// aside, and reports them in its result and as convergence events.
func TestUpSetsAsideCollectionsWithNoReader(t *testing.T) {
	a, cmd, recipe := newSelfSeedProject(t)
	declarePluginCollections(t, recipe, true)
	proj, err := project.Load(recipe)
	require.NoError(t, err)

	var events []convergence.Event
	var stderr bytes.Buffer
	cmd.SetErr(&stderr)
	var out ConvergeOutput
	err = a.RunDefaultFlowConverge(cmd, proj, recipe, ConvergeOptions{
		UntilGate: true, MaxPasses: 3, noChecks: true, capture: &out,
		onEvent: func(ev convergence.Event) { events = append(events, ev) },
	})
	require.NoError(t, err, "a missing plugin sets its collections aside: %s", stderr.String())

	require.Len(t, out.Locales, 1)
	assert.Equal(t, 100, out.Locales[0].Pct["translated"], "the readable collection converged")
	target, rerr := os.ReadFile(filepath.Join(filepath.Dir(recipe), "src", "nb.json"))
	require.NoError(t, rerr)
	assert.Contains(t, string(target), "Hei verden", "the readable collection was materialized")

	requireNoReader(t, out.Warnings, "pkg/doc.idml", "okf_idml", "okapi-bridge")
	requireNoReader(t, out.Warnings, "cask/kapi.rb", "sourcecode", "sourcecode")

	var logged []string
	for _, ev := range events {
		if ev.Type == convergence.EventLog {
			logged = append(logged, ev.Message)
		}
	}
	joined := strings.Join(logged, "\n")
	assert.Contains(t, joined, `No reader for format "okf_idml"`)
	assert.Contains(t, joined, "kapi plugins install okapi-bridge")
	assert.NotContains(t, joined, "kapi plugins install okf_idml")
	assert.Contains(t, joined, "pkg/doc.idml")

	var text bytes.Buffer
	require.NoError(t, out.FormatText(&text))
	assert.Contains(t, text.String(), `no reader for format "okf_idml"`)
}

// Seeding the committed context reads every committed translation. A committed
// target in a plugin format is left for a machine that can read it, and the rest
// of the record is still absorbed.
func TestSeedSkipsCommittedTargetsWithNoReader(t *testing.T) {
	a, _, recipe := newSelfSeedProject(t)
	declarePluginCollections(t, recipe, true)
	require.NoError(t, os.WriteFile(filepath.Join(filepath.Dir(recipe), "src", "nb.json"),
		[]byte(`{"greeting":"Hei verden"}`), 0o644))

	res, err := a.seedContext(context.Background(), recipe)
	require.NoError(t, err)
	assert.Positive(t, res.Record.Documents, "the readable committed translation was read")
}

// `kapi up --plan` prices the readable collections and names the one it set
// aside.
func TestUpPlanSetsAsideCollectionsWithNoReader(t *testing.T) {
	a, cmd, recipe := newSelfSeedProject(t)
	declarePluginCollections(t, recipe, true)
	proj, err := project.Load(recipe)
	require.NoError(t, err)

	plan, err := a.computeProjectPlan(context.Background(), proj, recipe)
	require.NoError(t, err)
	requireNoReader(t, plan.Warnings, "pkg/doc.idml", "okf_idml", "okapi-bridge")
	var app bool
	for _, s := range plan.Scopes {
		if s.Collection == "app" {
			app = true
		}
		assert.NotEqual(t, "layout", s.Collection, "a set-aside collection is not priced")
	}
	assert.True(t, app, "the readable collection is priced: %+v", plan.Scopes)

	var text bytes.Buffer
	require.NoError(t, plan.FormatText(&text))
	assert.Contains(t, text.String(), `no reader for format "okf_idml"`)

	// The plan path seeds an existing store before it prices, and the seed
	// reads the committed layout target.
	_, err = a.seedContext(context.Background(), recipe)
	require.NoError(t, err)
	require.NoError(t, cmd.Flags().Set("plan", "true"))
	var planOut bytes.Buffer
	cmd.SetOut(&planOut)
	require.NoError(t, a.ExecuteUp(cmd, recipe))
	assert.Contains(t, planOut.String(), `no reader for format "okf_idml"`)
}

// A project whose every collection needs a missing plugin has nothing a run
// could read. The run says nothing was converged and fails, and the plan says
// nothing could be priced rather than that nothing is left to do.
func TestUpOverOnlyUnreadableContentConvergesNothing(t *testing.T) {
	a, cmd, recipe := newSelfSeedProject(t)
	declarePluginCollections(t, recipe, false)
	proj, err := project.Load(recipe)
	require.NoError(t, err)

	var out ConvergeOutput
	err = a.RunDefaultFlowConverge(cmd, proj, recipe, ConvergeOptions{
		UntilGate: true, MaxPasses: 3, noChecks: true, capture: &out,
	})
	require.Error(t, err, "a run over nothing it can read never reports success")
	assert.Contains(t, err.Error(), "nothing was converged")
	assert.Contains(t, err.Error(), "kapi plugins install okapi-bridge")
	assert.False(t, out.Converged)

	plan, err := a.computeProjectPlan(context.Background(), proj, recipe)
	require.NoError(t, err)
	requireNoReader(t, plan.Warnings, "pkg/doc.idml", "okf_idml", "okapi-bridge")
	var text bytes.Buffer
	require.NoError(t, plan.FormatText(&text))
	assert.NotContains(t, text.String(), "Nothing to do")
	assert.NotContains(t, formatPlanLine(plan), "every unit has a committed target")
}

// Only a missing reader sets a collection aside. `--fail-on-unknown` asks for a
// file that cannot be processed to fail the run, and it still does.
func TestUpFailOnUnknownStillFailsOnAMissingReader(t *testing.T) {
	a, cmd, recipe := newSelfSeedProject(t)
	declarePluginCollections(t, recipe, true)
	require.NoError(t, cmd.Flags().Set("fail-on-unknown", "true"))
	proj, err := project.Load(recipe)
	require.NoError(t, err)

	err = a.RunDefaultFlowConverge(cmd, proj, recipe, ConvergeOptions{UntilGate: true, MaxPasses: 3, noChecks: true})
	require.Error(t, err)
	require.ErrorIs(t, err, registry.ErrUnknownFormat)
}

// A source in a format kapi reads that fails to parse was opened and is broken.
// The run still fails on it.
func TestUpStillFailsOnABrokenFileInAKnownFormat(t *testing.T) {
	a, cmd, recipe := newSelfSeedProject(t)
	declarePluginCollections(t, recipe, true)
	require.NoError(t, os.WriteFile(filepath.Join(filepath.Dir(recipe), "src", "en.json"),
		[]byte(`{"greeting": "Hello`), 0o644))
	proj, err := project.Load(recipe)
	require.NoError(t, err)

	err = a.RunDefaultFlowConverge(cmd, proj, recipe, ConvergeOptions{UntilGate: true, MaxPasses: 3, noChecks: true})
	require.Error(t, err)
	require.NotErrorIs(t, err, registry.ErrUnknownFormat)
}
