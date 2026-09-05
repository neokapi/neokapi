package host

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/project"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRunFlowAllLocales_TracesEachFile: with Trace set, the orchestrator
// records every file of every locale pass on a recorder of its own and hands
// the finished trace to the sink after the file's done event. The trace is
// the shape the CLI's --trace file has (reader → tool-N → writer nodes, events,
// part snapshots after each step), which is what the desktop's Run view
// replays.
func TestRunFlowAllLocales_TracesEachFile(t *testing.T) {
	a, _, recipe, dir := newLoopProject(t, map[string]string{
		"en.json": `{"greeting":"Hello world","farewell":"Goodbye now"}`,
	})
	proj, err := project.Load(recipe)
	require.NoError(t, err)
	input := filepath.Join(dir, "src", "en.json")

	var events []FlowRunEvent
	_, err = a.RunFlowAllLocales(context.Background(), FlowRunOptions{
		FlowName:    "translate",
		Project:     proj,
		ProjectPath: recipe,
		InputPaths:  []string{input},
		Trace:       true,
	}, func(ev FlowRunEvent) { events = append(events, ev) })
	require.NoError(t, err)

	var traces []FlowRunEvent
	var order []string
	for _, ev := range events {
		switch ev.Type {
		case FlowEventFileDone, FlowEventFileTrace:
			order = append(order, ev.Type+":"+ev.Locale)
		}
		if ev.Type == FlowEventFileTrace {
			traces = append(traces, ev)
		}
	}
	require.Len(t, traces, 2, "one trace per file and locale pass")
	assert.Equal(t, []string{
		"file_done:nb", "file_trace:nb", "file_done:de", "file_trace:de",
	}, order, "a file's trace follows its done event")

	for _, ev := range traces {
		require.NotNil(t, ev.Trace, "%s: the event carries the trace", ev.Locale)
		assert.Equal(t, input, ev.FilePath)
		assert.Equal(t, filepath.Join(dir, "out", ev.Locale, "en.json"), ev.OutputPath)

		tr := ev.Trace
		assert.Equal(t, "translate", tr.Name)
		assert.Equal(t, "en.json", tr.InputFile.Name)
		assert.Equal(t, "json", tr.InputFile.Format)
		assert.Contains(t, tr.InputFile.Preview, "Hello world")
		assert.NotContains(t, tr.OutputFile.Preview, "Hello world", "the output preview is the translated file")
		assert.False(t, tr.Truncated)

		var ids []string
		for _, n := range tr.Nodes {
			ids = append(ids, n.ID+"="+n.Name)
		}
		assert.Equal(t, []string{"reader=json", "tool-0=recycle", "tool-1=translate", "writer=json"}, ids,
			"tool nodes are labelled with the spec's step names, in order")

		// The reader emits the document part and then one part per block; every
		// part is snapshotted as it leaves the reader and after each step.
		blocks := 0
		for id, set := range tr.Parts {
			assert.Contains(t, set.AfterNode, "tool-0", "%s: snapshotted after every step", id)
			assert.Contains(t, set.AfterNode, "tool-1", "%s: snapshotted after every step", id)
			if set.Initial.Type == "Block" {
				blocks++
				assert.NotEmpty(t, set.AfterNode["tool-1"].TargetText, "%s: the last snapshot carries the translation", id)
			}
		}
		assert.Equal(t, 2, blocks, "one snapshot set per block")
		// Four parts flow: the document's start and end (one id) and the two
		// blocks. Each is recorded leaving the reader, entering and leaving
		// every tool, and reaching the writer.
		var byNode = map[string]int{}
		for _, e := range tr.Events {
			byNode[e.NodeID]++
		}
		assert.Equal(t, 4, byNode["reader"], "one reader exit per part")
		assert.Equal(t, 8, byNode["tool-0"], "enter + exit per part")
		assert.Equal(t, 8, byNode["tool-1"])
		assert.Equal(t, 8, byNode["writer"])
		assert.Positive(t, tr.DurationUs)
	}

	// The output files are the deliverable; tracing changes nothing about them.
	for _, loc := range []string{"nb", "de"} {
		b, rerr := os.ReadFile(filepath.Join(dir, "out", loc, "en.json"))
		require.NoError(t, rerr)
		assert.NotContains(t, string(b), "Hello world")
	}
}

// TestRunFlowAllLocales_TraceBudget: the limits reach the recorder, and a trace
// they cut short says so.
func TestRunFlowAllLocales_TraceBudget(t *testing.T) {
	a, _, recipe, dir := newLoopProject(t, map[string]string{
		"en.json": `{"greeting":"Hello world","farewell":"Goodbye now","third":"A third one"}`,
	})
	proj, err := project.Load(recipe)
	require.NoError(t, err)

	var traces []*flow.FlowTrace
	_, err = a.RunFlowAllLocales(context.Background(), FlowRunOptions{
		FlowName:      "translate",
		Project:       proj,
		ProjectPath:   recipe,
		InputPaths:    []string{filepath.Join(dir, "src", "en.json")},
		TargetLocales: []string{"nb"},
		Trace:         true,
		TraceLimits:   flow.TraceLimits{MaxParts: 1},
	}, func(ev FlowRunEvent) {
		if ev.Type == FlowEventFileTrace {
			traces = append(traces, ev.Trace)
		}
	})
	require.NoError(t, err)
	require.Len(t, traces, 1)
	tr := traces[0]
	assert.True(t, tr.Truncated)
	assert.Len(t, tr.Parts, 1, "the first part is kept whole")
	for _, e := range tr.Events {
		assert.Contains(t, tr.Parts, e.PartID, "no event of a dropped part")
	}
}

// TestRunFlowAllLocales_NoTraceWithoutTheOption is the control: an untraced
// run emits no trace event and wraps nothing.
func TestRunFlowAllLocales_NoTraceWithoutTheOption(t *testing.T) {
	a, _, recipe, dir := newLoopProject(t, map[string]string{
		"en.json": `{"greeting":"Hello world"}`,
	})
	proj, err := project.Load(recipe)
	require.NoError(t, err)

	var types []string
	_, err = a.RunFlowAllLocales(context.Background(), FlowRunOptions{
		FlowName:    "translate",
		Project:     proj,
		ProjectPath: recipe,
		InputPaths:  []string{filepath.Join(dir, "src", "en.json")},
	}, func(ev FlowRunEvent) { types = append(types, ev.Type) })
	require.NoError(t, err)
	assert.NotContains(t, types, FlowEventFileTrace)
	assert.Contains(t, types, FlowEventFileDone)
}

func TestFilePreview(t *testing.T) {
	dir := t.TempDir()
	write := func(name string, body []byte) string {
		p := filepath.Join(dir, name)
		require.NoError(t, os.WriteFile(p, body, 0o644))
		return p
	}

	assert.Empty(t, filePreview(filepath.Join(dir, "missing")))

	short := write("short.txt", []byte("hello"))
	assert.Equal(t, "hello", filePreview(short))

	exact := write("exact.txt", []byte(strings.Repeat("a", tracePreviewBytes)))
	assert.Equal(t, strings.Repeat("a", tracePreviewBytes), filePreview(exact), "a file at the budget is not marked clipped")

	long := write("long.txt", []byte(strings.Repeat("a", tracePreviewBytes+50)))
	got := filePreview(long)
	assert.True(t, strings.HasSuffix(got, "\n... (truncated)"))
	assert.Equal(t, strings.Repeat("a", tracePreviewBytes), strings.TrimSuffix(got, "\n... (truncated)"))

	// A multi-byte rune straddling the budget is dropped rather than split.
	split := write("split.txt", append([]byte(strings.Repeat("a", tracePreviewBytes-1)), []byte("é_tail")...))
	got = filePreview(split)
	head := strings.TrimSuffix(got, "\n... (truncated)")
	assert.Equal(t, strings.Repeat("a", tracePreviewBytes-1), head)
	assert.True(t, strings.HasSuffix(got, "... (truncated)"))
}
