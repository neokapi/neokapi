package cli

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// recordingSink collects the orchestrator's event stream. The metrics ticker
// and AI progress callbacks may emit from other goroutines, so it locks.
type recordingSink struct {
	mu     sync.Mutex
	events []FlowRunEvent
}

func (s *recordingSink) sink() RunEventSink {
	return func(ev FlowRunEvent) {
		s.mu.Lock()
		defer s.mu.Unlock()
		s.events = append(s.events, ev)
	}
}

func (s *recordingSink) byType(t string) []FlowRunEvent {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []FlowRunEvent
	for _, ev := range s.events {
		if ev.Type == t {
			out = append(out, ev)
		}
	}
	return out
}

// TestRunFlowAllLocales_LocaleSelectionParity is the locale-selection parity
// test for the unified orchestrator: the exported API (the path the desktop
// runner adapts to Wails events) and the CLI's plain project flow-run
// (runFromProject) must resolve the identical locale set for the same
// recipe + flow — both route through flow.ResolveFlowLocales now.
func TestRunFlowAllLocales_LocaleSelectionParity(t *testing.T) {
	a := processOnlyApp(t)
	recipe, srcRel, root := processOnlyProjectFixture(t, []model.LocaleID{"fr-FR", "de-DE"})
	src := filepath.Join(root, srcRel)

	proj, err := project.LoadWithOptions(recipe, project.LoadOptions{SkipRequiresCheck: true})
	require.NoError(t, err)
	spec := proj.Flow("pseudo")
	require.NotNil(t, spec)

	// The single selection implementation both surfaces share: the fixture's
	// pseudo flow is one pseudo-translate step — bilingual with default
	// locale qps — so it resolves to exactly one qps pass regardless of the
	// project's target list.
	want := flow.ResolveFlowLocales(spec, flow.BuildToolInfoMap(a.ToolReg), "en-US", []string{"fr-FR", "de-DE"})
	require.Equal(t, [][]string{{"en-US", "qps"}}, want)

	// Surface 1: the exported orchestrator (the desktop path), with a
	// recording sink.
	rec := &recordingSink{}
	res, err := a.RunFlowAllLocales(t.Context(), FlowRunOptions{
		FlowName:    "pseudo",
		ProjectPath: recipe,
		InputPaths:  []string{src},
	}, rec.sink())
	require.NoError(t, err)
	assert.Equal(t, want, res.LocalePasses, "orchestrator must use the shared selection")
	assert.Equal(t, 1, res.FilesProcessed)

	// The orchestrator writes files (the desktop venue): the qps output lands
	// at the project's content target path.
	qpsOut := filepath.Join(root, "src/locales/qps/messages.json")
	require.Equal(t, []string{qpsOut}, res.OutputPaths)
	data, rerr := os.ReadFile(qpsOut)
	require.NoError(t, rerr, "the run must write the qps target file")
	assert.NotEqual(t, `{"greeting":"Hello, world."}`, string(data))

	// Event vocabulary: state (running + one per pass), per-file progress,
	// file_done with the output path, a closing complete with totals.
	states := rec.byType(FlowEventState)
	require.Len(t, states, 2)
	assert.Equal(t, "running", states[0].Message)
	assert.Equal(t, "qps", states[1].Locale)
	assert.Contains(t, states[1].Message, "Running for qps (1 files)")

	progress := rec.byType(FlowEventProgress)
	require.NotEmpty(t, progress)
	assert.Equal(t, 1, progress[0].FileCount)
	assert.Equal(t, src, progress[0].FilePath)

	done := rec.byType(FlowEventFileDone)
	require.Len(t, done, 1)
	assert.Equal(t, qpsOut, done[0].OutputPath)

	complete := rec.byType(FlowEventComplete)
	require.Len(t, complete, 1)
	assert.Equal(t, 1, complete[0].FilesProcessed)

	for _, ev := range rec.byType(FlowEventPipelineMetrics) {
		require.Len(t, ev.Steps, 1)
		assert.Equal(t, "pseudo-translate", ev.Steps[0].Name)
	}

	// Surface 2: the CLI's plain project flow-run with no --target-lang. The
	// in-project default is process-only, so the same selection shows up as a
	// targets/qps overlay in the project store (and no fr-FR/de-DE work).
	a2 := processOnlyApp(t)
	out, err := runRunCmd(t, a2, recipe, "pseudo", "-i", src)
	require.NoError(t, err, "run output: %s", out)
	assert.Equal(t, 1, storeOverlayCount(t, recipe, "targets/qps"),
		"the CLI plain flow-run must resolve the same qps pass as the orchestrator")
	assert.Equal(t, 0, storeOverlayCount(t, recipe, "targets/fr-FR"),
		"no first-project-target pass: selection is shared, not positional")
}

// TestRunFlowAllLocales_NilSink verifies the sink is optional: a nil sink
// runs to completion and returns the structured result.
func TestRunFlowAllLocales_NilSink(t *testing.T) {
	a := processOnlyApp(t)
	recipe, srcRel, root := processOnlyProjectFixture(t, []model.LocaleID{"fr-FR"})

	res, err := a.RunFlowAllLocales(t.Context(), FlowRunOptions{
		FlowName:    "pseudo",
		ProjectPath: recipe,
		InputPaths:  []string{filepath.Join(root, srcRel)},
	}, nil)
	require.NoError(t, err)
	assert.Equal(t, [][]string{{"en-US", "qps"}}, res.LocalePasses)
	assert.Equal(t, 1, res.FilesProcessed)
	assert.FileExists(t, filepath.Join(root, "src/locales/qps/messages.json"))
}

// TestRunFlowAllLocales_BilingualNoDefault_IteratesProjectTargets verifies
// the shared selection's project-target fan-out: a flow whose bilingual tool
// declares no default locale runs one pass per project target.
func TestRunFlowAllLocales_BilingualNoDefault_IteratesProjectTargets(t *testing.T) {
	a := processOnlyApp(t)
	recipe, srcRel, root := processOnlyProjectFixture(t, []model.LocaleID{"fr-FR", "de-DE"})

	// recycle is bilingual with no default locale → one pass per target.
	proj, err := project.LoadWithOptions(recipe, project.LoadOptions{SkipRequiresCheck: true})
	require.NoError(t, err)
	spec := &flow.StepsSpec{Steps: []flow.FlowStep{{Tool: "recycle"}}}

	rec := &recordingSink{}
	res, err := a.RunFlowAllLocales(t.Context(), FlowRunOptions{
		FlowName:    "reuse",
		Spec:        spec,
		Project:     proj,
		ProjectPath: recipe,
		InputPaths:  []string{filepath.Join(root, srcRel)},
	}, rec.sink())
	require.NoError(t, err)
	assert.Equal(t, [][]string{{"en-US", "fr-FR"}, {"en-US", "de-DE"}}, res.LocalePasses)
	assert.Equal(t, 2, res.FilesProcessed, "one execution per (file × pass)")

	states := rec.byType(FlowEventState)
	require.Len(t, states, 3) // running + one per pass
	assert.Equal(t, "fr-FR", states[1].Locale)
	assert.Equal(t, "de-DE", states[2].Locale)
}

// TestRunFlowAllLocales_CancelEmitsComplete pins the documented contract:
// context cancellation is not a run error — the run completes early and the
// sink still receives the terminal FlowEventComplete (a UI relies on it to
// stop spinners), with the files processed so far.
func TestRunFlowAllLocales_CancelEmitsComplete(t *testing.T) {
	a := processOnlyApp(t)
	recipe, srcRel, root := processOnlyProjectFixture(t, []model.LocaleID{"fr-FR"})

	ctx, cancel := context.WithCancel(t.Context())
	cancel() // cancelled before the first pass

	var events []FlowRunEvent
	res, err := a.RunFlowAllLocales(ctx, FlowRunOptions{
		FlowName:    "pseudo",
		ProjectPath: recipe,
		InputPaths:  []string{filepath.Join(root, srcRel)},
	}, func(ev FlowRunEvent) { events = append(events, ev) })
	require.NoError(t, err, "cancellation must not surface as a run error")
	require.NotNil(t, res)
	assert.Equal(t, 0, res.FilesProcessed)

	var sawComplete bool
	for _, ev := range events {
		if ev.Type == FlowEventComplete {
			sawComplete = true
		}
	}
	assert.True(t, sawComplete, "complete event must be emitted on cancellation")
}
