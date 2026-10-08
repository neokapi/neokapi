package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The gate task's rule against portal arrives once the agent has read the
// project's context: the result of the read lands it, the files gain their
// text, and the record says what set it off. A call that reads no context
// lands nothing.
func TestPairedLateContextLandsAfterTheFirstContextRead(t *testing.T) {
	task := pairedTaskByID(t, "recover-gate-refusal")
	require.NotNil(t, task.spec.LateContext)
	dir := t.TempDir()
	require.NoError(t, materializePairedTask(dir, task))
	style, err := os.ReadFile(filepath.Join(dir, "STYLE.md"))
	require.NoError(t, err)
	assert.NotContains(t, string(style), "portal", "the fixture starts without the rule")

	launch := PairedLaunch{Agent: PairedAgentSpec{Host: "claude", Model: "test"}, Condition: "skill-cli",
		Task: task.ID, Workspace: dir, LateContext: task.spec.LateContext}
	stream := `{"type":"system","subtype":"init","model":"test","session_id":"s"}` + "\n" +
		`{"type":"assistant","message":{"model":"test","content":[{"type":"tool_use","id":"t1","name":"Bash","input":{"command":"ls docs/en"}}]}}` + "\n" +
		`{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"t1","content":"reports.md"}]}}` + "\n" +
		`{"type":"assistant","message":{"model":"test","content":[{"type":"tool_use","id":"t2","name":"Bash","input":{"command":"kapi context docs/en/reports.md"}}]}}` + "\n" +
		`{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"t2","content":"Write plainly."}]}}` + "\n" +
		`{"type":"result","subtype":"success","is_error":false,"result":"done","usage":{"input_tokens":1,"output_tokens":1}}`
	result, err := parsePairedAgentStream(t.Context(), strings.NewReader(stream), launch)
	require.NoError(t, err)
	require.NotNil(t, result.LateContext)
	assert.True(t, result.LateContext.Applied, result.LateContext.Error)
	assert.Equal(t, "read:Bash", result.LateContext.Trigger)
	style, err = os.ReadFile(filepath.Join(dir, "STYLE.md"))
	require.NoError(t, err)
	assert.Contains(t, string(style), `never "portal"`)

	// The files the late context changed are held to the fixture with its
	// text added, so the attempt keeps every other file as it found it.
	validation, err := validatePairedTask(dir, task, &result)
	require.NoError(t, err)
	for _, c := range validation.Criteria {
		if strings.HasPrefix(c.ID, "unchanged:") {
			assert.True(t, c.Passed, c.ID)
		}
	}
}

// An agent that writes before it reads any context meets the rule at its
// first write.
func TestPairedLateContextLandsAtTheFirstWrite(t *testing.T) {
	task := pairedTaskByID(t, "recover-gate-refusal")
	dir := t.TempDir()
	require.NoError(t, materializePairedTask(dir, task))
	launch := PairedLaunch{Agent: PairedAgentSpec{Host: "claude", Model: "test"}, Condition: "skill-cli",
		Task: task.ID, Workspace: dir, LateContext: task.spec.LateContext}
	stream := `{"type":"system","subtype":"init","model":"test","session_id":"s"}` + "\n" +
		`{"type":"assistant","message":{"model":"test","content":[{"type":"tool_use","id":"t1","name":"Edit","input":{"file_path":"` + filepath.Join(dir, "docs/en/reports.md") + `"}}]}}` + "\n" +
		`{"type":"result","subtype":"success","is_error":false,"result":"done","usage":{"input_tokens":1,"output_tokens":1}}`
	result, err := parsePairedAgentStream(t.Context(), strings.NewReader(stream), launch)
	require.NoError(t, err)
	require.NotNil(t, result.LateContext)
	assert.Equal(t, "write:Edit", result.LateContext.Trigger)
}

func TestPairedReadsContext(t *testing.T) {
	shell := func(c string) map[string]any { return map[string]any{"command": c} }
	assert.True(t, pairedReadsContext("Bash", shell("kapi context docs/en/reports.md")))
	assert.True(t, pairedReadsContext("Bash", shell("cat STYLE.md")))
	assert.True(t, pairedReadsContext("shell", shell("kapi check docs/en/reports.md")))
	assert.True(t, pairedReadsContext("Read", map[string]any{"file_path": "/w/STYLE.md"}))
	assert.True(t, pairedReadsContext("mcp__kapi__context_read", nil))
	assert.False(t, pairedReadsContext("mcp__kapi__context_note", nil))
	assert.False(t, pairedReadsContext("Bash", shell("kapi inspect docs/en/reports.md")))
	assert.False(t, pairedReadsContext("mcp__kapi__read_blocks", nil))
}

// An attempt that changed no task file and ended on a question asked the
// person; one that changed nothing and asked nothing left it unchanged.
func TestPairedOutcome(t *testing.T) {
	asked := &PairedAgentResult{FinalText: "The project's terms forbid portal.\n\nShould I write overview page instead?"}
	assert.Equal(t, "asked", pairedOutcome(false, true, asked))
	assert.Equal(t, "unchanged", pairedOutcome(false, true, &PairedAgentResult{FinalText: "Done."}))
	assert.Equal(t, "failed", pairedOutcome(false, false, asked))
	assert.Equal(t, "passed", pairedOutcome(true, false, nil))
	assert.False(t, pairedAsks("Is this right? I changed it.\n\nDone."), "a question earlier in the message")
}
