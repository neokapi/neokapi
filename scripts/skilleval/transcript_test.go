package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The transcript is published, so these tests are about what reaches the file
// rather than about what the parser can hold: a session carries the workspace
// path and whatever the agent printed of it, and the dataset is tracked in git.

// stream builds a stream-json body from its lines, which is what the agent
// writes on stdout under --output-format stream-json.
func stream(lines ...string) string { return strings.Join(lines, "\n") + "\n" }

// TestParseStreamRecordsTheCallAndWhatCameBack.
//
// The result arrives in a later event than the call, joined only by the id on
// both. Matching them is the whole point: a tool list says the agent ran Bash,
// and the pair says it ran kapi and got "no such command".
func TestParseStreamRecordsTheCallAndWhatCameBack(t *testing.T) {
	body := stream(
		`{"type":"assistant","message":{"content":[{"type":"text","text":"Reading the recipe first."}]}}`,
		`{"type":"assistant","message":{"content":[{"type":"tool_use","id":"t1","name":"Bash","input":{"command":"kapi status"}}]}}`,
		`{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"t1","content":"3 collections, 1 locale"}]}}`,
		`{"type":"assistant","message":{"content":[{"type":"text","text":"Done."}]}}`,
	)
	var r Run
	parseStream(strings.NewReader(body), &r)

	require.Len(t, r.Events, 3)
	assert.Equal(t, "text", r.Events[0].Kind)
	assert.Equal(t, "Reading the recipe first.", r.Events[0].Text)

	assert.Equal(t, "tool", r.Events[1].Kind)
	assert.Equal(t, "Bash", r.Events[1].Name)
	assert.JSONEq(t, `{"command":"kapi status"}`, r.Events[1].Input)
	assert.Equal(t, "3 collections, 1 locale", r.Events[1].Output, "the result is matched to its call")

	assert.Equal(t, "Done.", r.Events[2].Text)
	assert.Equal(t, "Done.", r.FinalText, "the summary still works")
	assert.Equal(t, 3, r.Messages)
	assert.True(t, r.Triggered)
}

// TestParseStreamMarksAFailedResult: an error result is the interesting one,
// and it reads identically to a successful one without the flag.
func TestParseStreamMarksAFailedResult(t *testing.T) {
	body := stream(
		`{"type":"assistant","message":{"content":[{"type":"tool_use","id":"t1","name":"Bash","input":{"command":"kapi apply x"}}]}}`,
		`{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"t1","is_error":true,"content":"refused: inline codes"}]}}`,
	)
	var r Run
	parseStream(strings.NewReader(body), &r)
	require.Len(t, r.Events, 1)
	assert.True(t, r.Events[0].Failed)
	assert.Equal(t, "refused: inline codes", r.Events[0].Output)
}

// TestParseStreamReadsBlockResults: a tool may answer with content blocks
// rather than a string, and dropping those would silently lose the output of
// whichever tools happen to use them.
func TestParseStreamReadsBlockResults(t *testing.T) {
	body := stream(
		`{"type":"assistant","message":{"content":[{"type":"tool_use","id":"t1","name":"Read","input":{"file_path":"a.md"}}]}}`,
		`{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"t1","content":[{"type":"text","text":"line one"},{"type":"image"}]}]}}`,
	)
	var r Run
	parseStream(strings.NewReader(body), &r)
	require.Len(t, r.Events, 1)
	assert.Equal(t, "line one\n[image]", r.Events[0].Output, "a block with no text is named, not dropped")
}

// TestRecordScrubsEveryString.
//
// Nothing else scrubs: record is the only way an Event is built, so a caller
// cannot forget. The guard that would otherwise catch this runs over the whole
// repository and reports a line number in a generated file.
func TestRecordScrubsEveryString(t *testing.T) {
	body := stream(
		`{"type":"assistant","message":{"content":[{"type":"text","text":"running from /Users/me/src/neokapi/bin/kapi"}]}}`,
		`{"type":"assistant","message":{"content":[{"type":"tool_use","id":"t1","name":"Bash","input":{"command":"cd /private/var/folders/x/T/skilleval-p01 && kapi up"}}]}}`,
		`{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"t1","content":"wrote /Users/me/out.docx"}]}}`,
	)
	var r Run
	parseStream(strings.NewReader(body), &r)

	body2, err := json.Marshal(r.Events)
	require.NoError(t, err)
	assert.NotContains(t, string(body2), "/Users/me")
	assert.NotContains(t, string(body2), "/private/var")
}

// TestCapsBoundOneSession: a session is bounded so one pathological run cannot
// publish a file nobody can open, and it says how much it dropped rather than
// ending as if the agent had stopped.
func TestCapsBoundOneSession(t *testing.T) {
	var r Run
	for i := range maxSessionEvents + 25 {
		r.record(Event{Kind: "text", Text: strings.Repeat("x", 10) + string(rune('a'+i%26))})
	}
	assert.Len(t, r.Events, maxSessionEvents)
	assert.Equal(t, 25, r.EventsDropped)
}

func TestCapsBoundOneSessionByBytes(t *testing.T) {
	var r Run
	big := strings.Repeat("y", maxEventText)
	for range 200 {
		r.record(Event{Kind: "text", Text: big})
	}
	assert.Less(t, len(r.Events), 200)
	assert.Positive(t, r.EventsDropped)
	assert.LessOrEqual(t, r.eventBytes, maxSessionBytes+maxEventText)
}

// TestClipCutsOnARuneBoundary: cutting mid-rune fills a Norwegian transcript
// with replacement characters, and nb is a language this repo publishes in.
func TestClipCutsOnARuneBoundary(t *testing.T) {
	got := clip(strings.Repeat("ø", 20), 15)
	assert.True(t, strings.HasPrefix(got, strings.Repeat("ø", 7)))
	assert.Contains(t, got, "truncated")
	for _, r := range got {
		assert.NotEqual(t, '�', r, "no replacement character survives")
	}
}

// TestSplitSessionsMovesEventsOutOfTheDataset: the page imports the dataset, so
// an inline session is paid for by every reader of the summary.
func TestSplitSessionsMovesEventsOutOfTheDataset(t *testing.T) {
	rep := &Report{
		Mode:    modeTrigger,
		Surface: "skill",
		Results: []Result{
			{
				Scenario: Scenario{ID: "p01-first", Prompt: "do the thing"},
				Runs:     []Run{{Events: []Event{{Kind: "text", Text: "hello"}}}},
				Unaided:  []Run{{Events: []Event{{Kind: "text", Text: "control"}}}},
			},
			{Scenario: Scenario{ID: "p02-second"}, Runs: []Run{{}}},
		},
	}
	files := splitSessions(rep)

	require.Len(t, files, 1, "a scenario with no events gets no file")
	name := sessionFileName(rep.Key(), "p01-first")
	require.Contains(t, files, name)
	assert.Equal(t, name, rep.Results[0].Transcript)
	assert.Empty(t, rep.Results[1].Transcript)

	assert.Empty(t, rep.Results[0].Runs[0].Events, "and the dataset no longer carries them")
	assert.Equal(t, "hello", files[name].Runs[0].Events[0].Text)
	assert.Equal(t, "control", files[name].Unaided[0].Events[0].Text)
	assert.Equal(t, "do the thing", files[name].Prompt)
}

// TestWriteSessionsPrunesOnlyItsOwnKey.
//
// merge() writes one surface at a time on purpose, because `-only mcp` measures
// nothing about the skill surface. Pruning across keys would delete transcripts
// that run could not replace, which is the same mistake as overwriting the
// dataset with a partial one.
func TestWriteSessionsPrunesOnlyItsOwnKey(t *testing.T) {
	dir := t.TempDir()
	other := sessionFileName("mcp:trigger", "m01-lookup")
	stale := sessionFileName("skill:trigger", "p09-retired")
	for _, n := range []string{other, stale} {
		require.NoError(t, os.WriteFile(filepath.Join(dir, n), []byte("{}"), 0o644))
	}

	keep := sessionFileName("skill:trigger", "p01-first")
	err := writeSessions(dir, "skill:trigger", map[string]*SessionFile{
		keep: {Key: "skill:trigger", Scenario: "p01-first", Runs: []Session{{Events: []Event{{Kind: "text", Text: "hi"}}}}},
	})
	require.NoError(t, err)

	assert.FileExists(t, filepath.Join(dir, keep))
	assert.FileExists(t, filepath.Join(dir, other), "another surface's transcripts survive")
	assert.NoFileExists(t, filepath.Join(dir, stale), "a scenario this key no longer has does not")
}

// TestSessionFileNameIsSafeAndStable: the name is a path and a URL, and it has
// to be the same on the next run or every re-run leaves the old one behind.
func TestSessionFileNameIsSafeAndStable(t *testing.T) {
	n := sessionFileName("skill:completion", "p04-cross-format-sweep")
	assert.Equal(t, "skill-completion--p04-cross-format-sweep.json", n)
	assert.Equal(t, n, sessionFileName("skill:completion", "p04-cross-format-sweep"))
	assert.NotContains(t, n, ":")
	assert.NotContains(t, n, "/")
}
