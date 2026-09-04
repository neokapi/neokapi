package backend

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/host"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunFlowBadTab(t *testing.T) {
	app := NewApp()
	err := app.RunFlow("bad-tab", "test", []string{"file.json"}, []string{"fr"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestRunFlowNotFound(t *testing.T) {
	app := NewApp()
	tab := newTestProject(t, app, "RunTest")

	err := app.RunFlow(tab.ID, "nonexistent", []string{"file.json"}, []string{"fr"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestRunFlowNoInputs(t *testing.T) {
	app := NewApp()
	tab := newTestProject(t, app, "RunTest2")
	_ = app.SaveFlow(tab.ID, "qa", &flow.StepsSpec{
		Steps: []flow.FlowStep{{Tool: "qa"}},
	})

	err := app.RunFlow(tab.ID, "qa", nil, []string{"fr"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no input files")
}

// TestRunFlowExecutesViaSharedOrchestrator runs a real flow end-to-end through
// the shared cli orchestrator (RunFlow → cli.App.RunFlowAllLocales) and
// asserts the Wails-facing contract held by the former hand-rolled loop: the
// event vocabulary (state/progress/complete with the same fields), the locale
// pass resolved from tool cardinality (pseudo-translate → its default qps),
// and the written output file.
func TestRunFlowExecutesViaSharedOrchestrator(t *testing.T) {
	app := NewApp()
	tab := newTestProject(t, app, "RunOrchestrated")

	op := app.getOpenProject(tab.ID)
	require.NotNil(t, op)
	root := filepath.Dir(op.Path)
	src := filepath.Join(root, "messages.json")
	require.NoError(t, os.WriteFile(src, []byte(`{"greeting":"Hello, world."}`), 0o644))

	_ = app.SaveFlow(tab.ID, "pseudo", &flow.StepsSpec{
		Steps: []flow.FlowStep{{Tool: "pseudo-translate"}},
	})

	require.NoError(t, app.RunFlow(tab.ID, "pseudo", []string{src}, []string{"fr-FR"}))

	// The run executes on a goroutine; poll until it leaves "running".
	deadline := time.Now().Add(30 * time.Second)
	for app.GetRunState() == string(RunStateRunning) && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	require.Equal(t, string(RunStateComplete), app.GetRunState())

	events := app.GetRunEvents()
	require.NotEmpty(t, events)

	byType := map[string][]RunEvent{}
	for _, ev := range events {
		assert.Equal(t, "pseudo", ev.FlowID)
		byType[ev.Type] = append(byType[ev.Type], ev)
	}
	states := byType["state"]
	require.Len(t, states, 2)
	assert.Equal(t, "running", states[0].Message)
	assert.Contains(t, states[1].Message, "Running for qps (1 files)",
		"locale pass must come from the shared flow.ResolveFlowLocales selection")

	progress := byType["progress"]
	require.NotEmpty(t, progress)
	assert.Equal(t, 1, progress[0].FileCount)
	assert.Equal(t, src, progress[0].FilePath)

	complete := byType["complete"]
	require.Len(t, complete, 1)
	assert.Equal(t, 1, complete[0].FilesProcessed)
	assert.Empty(t, byType["error"])

	// No content target template in the recipe → default sibling output.
	data, err := os.ReadFile(filepath.Join(root, "messages_qps.json"))
	require.NoError(t, err, "the run must write the qps output file")
	assert.NotEqual(t, `{"greeting":"Hello, world."}`, string(data))

	// The run recorded the file and told the frontend so: the "trace" event
	// names the file, and the retained trace is what the Run view replays.
	traces := byType["trace"]
	require.Len(t, traces, 1)
	assert.Equal(t, src, traces[0].FilePath)
	assert.Equal(t, "qps", traces[0].Locale)
	assert.Equal(t, "Recorded a trace of messages.json for qps", traces[0].Message)

	run := app.ListRunTraces()
	require.NotNil(t, run)
	assert.Equal(t, "pseudo", run.FlowName)
	assert.Equal(t, []flow.FlowStep{{Tool: "pseudo-translate"}}, run.Steps)
	assert.Equal(t, desktopTraceLimits.MaxParts, run.MaxParts)
	require.Len(t, run.Files, 1)
	assert.Equal(t, RunTraceFile{
		FilePath: src, Locale: "qps", OutputPath: filepath.Join(root, "messages_qps.json"),
	}, run.Files[0])

	tr := app.GetLastTrace("", "")
	require.NotNil(t, tr, "the last completed file's trace")
	assert.Same(t, tr, app.GetLastTrace(src, ""))
	assert.Same(t, tr, app.GetLastTrace(src, "qps"))
	assert.Nil(t, app.GetLastTrace(filepath.Join(root, "other.json"), ""))
	assert.Nil(t, app.GetLastTrace(src, "fr-FR"))
	assert.Equal(t, "pseudo", tr.Name)
	assert.Equal(t, "messages.json", tr.InputFile.Name)
	var ids []string
	for _, n := range tr.Nodes {
		ids = append(ids, n.ID)
	}
	assert.Equal(t, []string{"reader", "tool-0", "writer"}, ids)
	assert.NotEmpty(t, tr.Events)
	assert.NotEmpty(t, tr.Parts)
}

// TestRunTraces_EmptyBeforeAnyRun: nothing has run, so there is nothing to
// list or replay.
func TestRunTraces_EmptyBeforeAnyRun(t *testing.T) {
	app := NewApp()
	assert.Nil(t, app.ListRunTraces())
	assert.Nil(t, app.GetLastTrace("", ""))

	app.runState = newRunner()
	assert.Nil(t, app.ListRunTraces(), "a runner that has not run lists nothing")
	assert.Nil(t, app.GetLastTrace("", ""))
}

func traceEvent(path, locale string, parts int) host.FlowRunEvent {
	tr := &flow.FlowTrace{
		Name:  "t",
		Parts: map[string]*flow.PartSnapshotSet{},
	}
	for i := range parts {
		id := fmt.Sprintf("b%d", i)
		tr.Parts[id] = &flow.PartSnapshotSet{Initial: flow.PartSnapshot{ID: id, Type: "Block", Summary: strings.Repeat("x", 100)}}
	}
	return host.FlowRunEvent{Type: host.FlowEventFileTrace, FilePath: path, Locale: locale, OutputPath: path + ".out", Trace: tr}
}

// TestRetainTrace_KeepsTheLastFilesByCompletion: the retained set is the
// newest maxRetainedTraces completions; the oldest goes first.
func TestRetainTrace_KeepsTheLastFilesByCompletion(t *testing.T) {
	app := NewApp()
	app.runState = newRunner()
	app.runState.mu.Lock()
	app.runState.resetTracesLocked("f", &flow.StepsSpec{Steps: []flow.FlowStep{{Tool: "qa"}}})
	app.runState.mu.Unlock()

	for i := range maxRetainedTraces + 2 {
		require.True(t, app.runState.retainTrace(traceEvent(fmt.Sprintf("/in/%d.json", i), "nb", 1)))
	}
	run := app.ListRunTraces()
	require.Len(t, run.Files, maxRetainedTraces)
	assert.Equal(t, "/in/2.json", run.Files[0].FilePath, "the two oldest completions were evicted")
	assert.Equal(t, fmt.Sprintf("/in/%d.json", maxRetainedTraces+1), run.Files[maxRetainedTraces-1].FilePath)
	assert.Nil(t, app.GetLastTrace("/in/0.json", ""))
	assert.NotNil(t, app.GetLastTrace("/in/2.json", ""))
	assert.Same(t, app.GetLastTrace("", ""), app.GetLastTrace(fmt.Sprintf("/in/%d.json", maxRetainedTraces+1), ""))
}

// TestRetainTrace_ByteCap: the set as JSON stays under maxRetainedTraceBytes,
// evicting older traces to fit a new one, and a trace that alone exceeds the
// cap is refused rather than kept over it.
func TestRetainTrace_ByteCap(t *testing.T) {
	r := newRunner()
	r.mu.Lock()
	r.resetTracesLocked("f", &flow.StepsSpec{})
	r.mu.Unlock()

	// Size each trace at three tenths of the cap, measured rather than
	// assumed, so three fit and a fourth forces an eviction.
	probe, err := json.Marshal(traceEvent("probe", "", 1000).Trace)
	require.NoError(t, err)
	parts := int(0.3 * float64(maxRetainedTraceBytes) / (float64(len(probe)) / 1000))
	for i := range 3 {
		require.True(t, r.retainTrace(traceEvent(fmt.Sprintf("/in/%d.json", i), "", parts)))
	}
	require.Len(t, r.traces, 3)
	assert.LessOrEqual(t, r.traceBytes, maxRetainedTraceBytes)

	require.True(t, r.retainTrace(traceEvent("/in/3.json", "", parts)))
	assert.LessOrEqual(t, r.traceBytes, maxRetainedTraceBytes)
	assert.Equal(t, "/in/1.json", r.traces[0].file.FilePath, "the oldest was evicted to make room")
	assert.Equal(t, "/in/3.json", r.traces[len(r.traces)-1].file.FilePath)

	huge := traceEvent("/in/huge.json", "", maxRetainedTraceBytes/120)
	assert.False(t, r.retainTrace(huge), "a trace over the cap on its own is not kept")
	assert.Equal(t, "/in/3.json", r.traces[len(r.traces)-1].file.FilePath)
	assert.LessOrEqual(t, r.traceBytes, maxRetainedTraceBytes)

	assert.False(t, r.retainTrace(host.FlowRunEvent{Type: host.FlowEventFileTrace, FilePath: "/in/none.json"}), "an event without a trace retains nothing")
}

// TestRetainTrace_LocalePasses: a file run for two locales keeps one trace per
// pass; the file's newest pass answers a lookup without a locale.
func TestRetainTrace_LocalePasses(t *testing.T) {
	app := NewApp()
	app.runState = newRunner()
	app.runState.mu.Lock()
	app.runState.resetTracesLocked("f", &flow.StepsSpec{})
	app.runState.mu.Unlock()

	require.True(t, app.runState.retainTrace(traceEvent("/in/a.json", "nb", 1)))
	require.True(t, app.runState.retainTrace(traceEvent("/in/b.json", "nb", 1)))
	require.True(t, app.runState.retainTrace(traceEvent("/in/a.json", "de", 1)))

	run := app.ListRunTraces()
	require.Len(t, run.Files, 3)
	assert.Equal(t, RunTraceFile{FilePath: "/in/a.json", Locale: "de", OutputPath: "/in/a.json.out"}, run.Files[2])

	nb := app.GetLastTrace("/in/a.json", "nb")
	de := app.GetLastTrace("/in/a.json", "de")
	require.NotNil(t, nb)
	require.NotNil(t, de)
	assert.NotSame(t, nb, de)
	assert.Same(t, de, app.GetLastTrace("/in/a.json", ""), "the newest pass of the file")
	assert.Same(t, de, app.GetLastTrace("", ""), "the newest completion overall")
	assert.Nil(t, app.GetLastTrace("", "fr"), "no pass ran for fr")
}

// TestRunTraces_TruncatedFlagAndReset: a budget-cut trace is listed as such,
// and a new run starts the record over for its own flow and steps.
func TestRunTraces_TruncatedFlagAndReset(t *testing.T) {
	app := NewApp()
	app.runState = newRunner()
	app.runState.mu.Lock()
	app.runState.resetTracesLocked("first", &flow.StepsSpec{Steps: []flow.FlowStep{{Tool: "qa"}}})
	app.runState.mu.Unlock()

	ev := traceEvent("/in/a.json", "", 1)
	ev.Trace.Truncated = true
	require.True(t, app.runState.retainTrace(ev))
	run := app.ListRunTraces()
	require.Len(t, run.Files, 1)
	assert.True(t, run.Files[0].Truncated)

	spec := &flow.StepsSpec{Steps: []flow.FlowStep{{Tool: "translate", Config: map[string]any{"provider": "demo"}}}}
	app.runState.mu.Lock()
	app.runState.resetTracesLocked("second", spec)
	app.runState.mu.Unlock()
	spec.Steps[0].Config["provider"] = "edited"

	run = app.ListRunTraces()
	assert.Equal(t, "second", run.FlowName)
	assert.Empty(t, run.Files, "the previous run's traces are gone")
	assert.Nil(t, app.GetLastTrace("", ""))
	assert.Equal(t, "demo", run.Steps[0].Config["provider"], "the record holds the steps as they ran, not the edited flow")
}

func TestGetRunStateIdle(t *testing.T) {
	app := NewApp()
	assert.Equal(t, "idle", app.GetRunState())
}

func TestCancelRunNoOp(t *testing.T) {
	app := NewApp()
	app.CancelRun()
}

func TestRunnerState(t *testing.T) {
	r := newRunner()
	assert.Equal(t, RunStateIdle, r.state)
	assert.False(t, r.running)
}

func TestRunEventTypes(t *testing.T) {
	events := []RunEvent{
		{Type: "state", FlowID: "test", Message: "running"},
		{Type: "progress", FlowID: "test", FileIndex: 0, FileCount: 3, FilePath: "a.json"},
		{Type: "error", FlowID: "test", Message: "something failed"},
		{Type: "complete", FlowID: "test", DurationMs: 1234, FilesProcessed: 5},
	}
	for _, e := range events {
		assert.NotEmpty(t, e.Type)
		assert.NotEmpty(t, e.FlowID)
	}
}

func TestGetRunEventsEmpty(t *testing.T) {
	app := NewApp()
	events := app.GetRunEvents()
	assert.Nil(t, events)
}

func TestGetRunEventsAfterInit(t *testing.T) {
	app := NewApp()
	app.runState = newRunner()
	events := app.GetRunEvents()
	assert.Empty(t, events)
}

func TestEmitRunEventAccumulates(t *testing.T) {
	app := NewApp()
	app.runState = newRunner()

	app.emitRunEvent(RunEvent{Type: "state", FlowID: "translate", Message: "running"})
	app.emitRunEvent(RunEvent{Type: "progress", FlowID: "translate", FileIndex: 0, FileCount: 5})
	app.emitRunEvent(RunEvent{Type: "progress", FlowID: "translate", FileIndex: 1, FileCount: 5})

	events := app.GetRunEvents()
	require.Len(t, events, 3)
	assert.Equal(t, "state", events[0].Type)
	assert.Equal(t, "running", events[0].Message)
	assert.Equal(t, "progress", events[1].Type)
	assert.Equal(t, 0, events[1].FileIndex)
	assert.Equal(t, 1, events[2].FileIndex)
}

func TestRunEventsResetOnNewRun(t *testing.T) {
	app := NewApp()
	app.runState = newRunner()

	// Simulate events from a previous run.
	app.emitRunEvent(RunEvent{Type: "state", FlowID: "old", Message: "running"})
	app.emitRunEvent(RunEvent{Type: "complete", FlowID: "old"})
	require.Len(t, app.GetRunEvents(), 2)

	// Simulate starting a new run — events should be cleared.
	app.runState.mu.Lock()
	app.runState.events = nil
	app.runState.mu.Unlock()

	app.emitRunEvent(RunEvent{Type: "state", FlowID: "new", Message: "running"})
	events := app.GetRunEvents()
	require.Len(t, events, 1)
	assert.Equal(t, "new", events[0].FlowID)
}

func TestGetRunEventsReturnsCopy(t *testing.T) {
	app := NewApp()
	app.runState = newRunner()

	app.emitRunEvent(RunEvent{Type: "state", FlowID: "test", Message: "running"})

	events1 := app.GetRunEvents()
	events2 := app.GetRunEvents()

	// Mutating one copy should not affect the other.
	events1[0].Message = "modified"
	assert.Equal(t, "running", events2[0].Message)
}
