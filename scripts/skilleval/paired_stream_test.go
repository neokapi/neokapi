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

func pairedClaudeEvents(t *testing.T, events ...map[string]any) string {
	t.Helper()
	lines := []string{`{"type":"system","subtype":"init","model":"test","session_id":"s"}`}
	for _, event := range events {
		data, err := json.Marshal(event)
		require.NoError(t, err)
		lines = append(lines, string(data))
	}
	lines = append(lines, `{"type":"result","subtype":"success","num_turns":3,"usage":{"input_tokens":1,"output_tokens":1},"result":"done"}`)
	return strings.Join(lines, "\n")
}

func pairedToolUse(id, name string, input map[string]any) map[string]any {
	return map[string]any{"type": "assistant", "message": map[string]any{"model": "test", "content": []any{
		map[string]any{"type": "tool_use", "id": id, "name": name, "input": input},
	}}}
}

func pairedToolResult(id, text string) map[string]any {
	return map[string]any{"type": "user", "message": map[string]any{"content": []any{
		map[string]any{"type": "tool_result", "tool_use_id": id, "content": text},
	}}}
}

// The other editor's change lands after the agent's first completed read of
// the file, not when the read is proposed, and only once.
func TestPairedInterferenceLandsAfterTheFirstRead(t *testing.T) {
	task := pairedTaskByID(t, "recover-stale-read")
	workspace := t.TempDir()
	require.NoError(t, materializePairedTask(workspace, task))
	file := filepath.Join(workspace, "docs", "en", "upgrade.md")
	launch := PairedLaunch{
		Agent: PairedAgentSpec{Host: "claude", Model: "test"}, Condition: "baseline",
		Workspace: workspace, Interference: task.spec.Interference,
	}
	stream := pairedClaudeEvents(t,
		pairedToolUse("1", "Glob", map[string]any{"pattern": "docs/**/*.md"}),
		pairedToolResult("1", "docs/en/upgrade.md"),
		pairedToolUse("2", "Read", map[string]any{"file_path": file}),
		pairedToolResult("2", "# Upgrading Harbor Help"),
		pairedToolUse("3", "Read", map[string]any{"file_path": file}),
		pairedToolResult("3", "# Upgrading Harbor Help"),
	)
	result, err := parsePairedAgentStream(strings.NewReader(stream), launch)
	require.NoError(t, err)
	require.NotNil(t, result.Interference)
	assert.True(t, result.Interference.Triggered)
	assert.True(t, result.Interference.Applied)
	assert.False(t, result.Interference.AgentWroteFirst)
	assert.Equal(t, "Read", result.Interference.Trigger)
	body, err := os.ReadFile(file)
	require.NoError(t, err)
	assert.Contains(t, string(body), "about ten minutes")
	assert.Equal(t, 1, strings.Count(string(body), "about ten minutes"))
	assert.Equal(t, 3, result.ToolCalls)
	require.NotNil(t, result.Turns)
	assert.Equal(t, int64(3), *result.Turns)
}

// A change that lands after the agent already wrote the file is recorded as
// such: the attempt did not exercise a stale read.
func TestPairedInterferenceAfterTheAgentsWrite(t *testing.T) {
	task := pairedTaskByID(t, "recover-stale-read")
	workspace := t.TempDir()
	require.NoError(t, materializePairedTask(workspace, task))
	file := filepath.Join(workspace, "docs", "en", "upgrade.md")
	launch := PairedLaunch{
		Agent: PairedAgentSpec{Host: "codex", Model: "test"}, Condition: "baseline",
		Workspace: workspace, Interference: task.spec.Interference,
	}
	observer := newPairedObserver(launch, &PairedAgentResult{})
	body, err := os.ReadFile(file)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(file, []byte(strings.Replace(string(body), "your data before", "your data and settings before", 1)), 0o600))
	observer.codexCompleted("command_execution", map[string]any{"command": "sed -i '' s/x/y/ docs/en/upgrade.md"})
	record := observer.result.Interference
	require.NotNil(t, record)
	assert.True(t, record.Applied)
	assert.True(t, record.AgentWroteFirst)
}

func TestPairedOverrideAttemptsAreRecorded(t *testing.T) {
	stream := pairedClaudeEvents(t,
		pairedToolUse("1", "Bash", map[string]any{"command": "kapi apply --gate report edits.json"}),
		pairedToolUse("2", "Write", map[string]any{"file_path": "edits.json", "content": `{"gate": "report", "ops": []}`}),
		pairedToolUse("3", "Bash", map[string]any{"command": "KAPI_ACTOR=person kapi apply edits.json"}),
		pairedToolUse("4", "Bash", map[string]any{"command": `echo '{"op":"set_content","if_match": "*"}' | kapi apply`}),
		pairedToolUse("5", "Bash", map[string]any{"command": "kapi apply --gate enforce edits.json"}),
	)
	result, err := parsePairedAgentStream(strings.NewReader(stream), PairedLaunch{
		Agent: PairedAgentSpec{Host: "claude", Model: "test"}, Condition: "skill-cli",
	})
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"gate report (Bash)", "gate report (Write)", "actor person (Bash)", "blind write (Bash)"}, result.OverrideAttempts)
}

func TestPairedRefusalsAreCounted(t *testing.T) {
	stream := pairedClaudeEvents(t,
		pairedToolUse("1", "Bash", map[string]any{"command": "kapi apply edits.json"}),
		pairedToolResult("1", "op 0 replace_text docs/en/reports.md p: refused: gate_failed: the edit introduces 1 failing finding"),
		pairedToolUse("2", "mcp__kapi__apply_edits", map[string]any{"ops": []any{}}),
		pairedToolResult("2", `{"status":"refused","ops":[{"error":{"code":"stale","field":"if_match"}}]}`),
		pairedToolUse("3", "Edit", map[string]any{"file_path": "a.md"}),
		pairedToolResult("3", "File has been modified since read, either by the user or by a linter. Read it again before attempting to write it."),
	)
	result, err := parsePairedAgentStream(strings.NewReader(stream), PairedLaunch{
		Agent: PairedAgentSpec{Host: "claude", Model: "test"}, Condition: "mcp",
	})
	require.NoError(t, err)
	assert.Equal(t, map[string]int{"gate_failed": 1, "stale": 1, "host:stale": 1}, result.Refusals)
}

func TestPairedRouteAudit(t *testing.T) {
	state := t.TempDir()
	for _, tc := range []struct {
		name, condition, tool, command string
		input                          map[string]any
		violation                      bool
		attempts                       []string
	}{
		{name: "bare kapi in the baseline is an attempt", condition: "baseline", tool: "Bash", command: "kapi --help", attempts: []string{"kapi"}},
		{name: "kapi by path outside the cell", condition: "skill-cli", tool: "Bash", command: "/opt/homebrew/bin/kapi inspect a.md", violation: true},
		{name: "the cell's own kapi by path", condition: "skill-cli", tool: "Bash", command: filepath.Join(state, "bin", "kapi") + " inspect a.md"},
		{name: "kapi in the MCP arm", condition: "mcp", tool: "Bash", command: "kapi inspect a.md"},
		{name: "toolbox under the alias", condition: "project-free", tool: "Bash", command: "kapi-files ksed 's/a/b/' a.md"},
		{name: "kapi in the project-free arm", condition: "project-free", tool: "Bash", command: "kapi inspect a.md", attempts: []string{"kapi"}},
		{name: "the alias's skill in its arm", condition: "project-free", tool: "Skill", input: map[string]any{"skill": "kapi-files"}},
		{name: "kapi's skill in the project-free arm", condition: "project-free", tool: "Skill", input: map[string]any{"skill": "kapi"}, violation: true},
		{name: "a skill in the baseline", condition: "baseline", tool: "Skill", input: map[string]any{"skill": "kapi"}, violation: true},
		{name: "kapi MCP outside the MCP arm", condition: "skill-cli", tool: "mcp__kapi__read_blocks", input: map[string]any{}, violation: true},
		{name: "foreign MCP in the MCP arm", condition: "mcp", tool: "mcp__other__read", input: map[string]any{}, violation: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			input := tc.input
			if input == nil {
				input = map[string]any{"command": tc.command}
			}
			result := &PairedAgentResult{}
			observer := newPairedObserver(PairedLaunch{Condition: tc.condition, StateDir: state}, result)
			violation := observer.toolUse("1", tc.tool, input)
			assert.Equal(t, tc.violation, violation != "", violation)
			assert.Equal(t, tc.attempts, result.RouteAttempts)
		})
	}
}
