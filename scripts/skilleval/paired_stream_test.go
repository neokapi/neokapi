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

// The other editor's change lands once a tool result has shown the agent the
// text it changes, and only once. Naming the file, or a search that printed
// another line of it, shows the agent nothing to be stale about.
func TestPairedInterferenceLandsWhenTheAgentSeesTheText(t *testing.T) {
	task := pairedTaskByID(t, "recover-stale-read")
	workspace := t.TempDir()
	require.NoError(t, materializePairedTask(workspace, task))
	file := filepath.Join(workspace, "docs", "en", "upgrade.md")
	launch := PairedLaunch{
		Agent: PairedAgentSpec{Host: "claude", Model: "test"}, Condition: "skill-cli",
		Workspace: workspace, Interference: task.spec.Interference,
	}
	stream := pairedClaudeEvents(t,
		pairedToolUse("1", "Glob", map[string]any{"pattern": "docs/**/*.md"}),
		pairedToolResult("1", "docs/en/upgrade.md"),
		pairedToolUse("4", "Skill", map[string]any{"skill": "kapi", "args": "edit docs/en/upgrade.md"}),
		pairedToolResult("4", "Launching skill: kapi"),
		// A search that prints only the sentence the agent edits.
		pairedToolUse("5", "Grep", map[string]any{"pattern": "Back up your data", "path": "docs"}),
		pairedToolResult("5", "docs/en/upgrade.md:8:Back up your data before you upgrade. The upgrade keeps your appointments and"),
		// A search whose pattern holds the text but that printed nothing.
		pairedToolUse("6", "Bash", map[string]any{"command": "grep -c 'it takes about five minutes' docs/en/upgrade.md"}),
		pairedToolResult("6", "1"),
		pairedToolUse("2", "Read", map[string]any{"file_path": file}),
		pairedToolResult("2", "9\tmessages, and it takes about five minutes."),
		pairedToolUse("3", "Read", map[string]any{"file_path": file}),
		pairedToolResult("3", "9\tmessages, and it takes about five minutes."),
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
	assert.Equal(t, 1, strings.Count(string(body), "about ten minutes"))
	assert.Equal(t, 6, result.ToolCalls)
	require.NotNil(t, result.Turns)
	assert.Equal(t, int64(3), *result.Turns)
}

// A Codex command lands the change through what it printed, not through the
// text of the command, and a write with no read first leaves the change to
// land after it, which the record says.
func TestPairedInterferenceFromCodexOutput(t *testing.T) {
	task := pairedTaskByID(t, "recover-stale-read")
	workspace := t.TempDir()
	require.NoError(t, materializePairedTask(workspace, task))
	file := filepath.Join(workspace, "docs", "en", "upgrade.md")
	launch := PairedLaunch{
		Agent: PairedAgentSpec{Host: "codex", Model: "test"}, Condition: "project-free",
		Workspace: workspace, Interference: task.spec.Interference,
	}
	observer := newPairedObserver(launch, &PairedAgentResult{})
	observer.codexCompleted("command_execution", map[string]any{
		"command": "/bin/zsh -c \"rg -n 'it takes about five minutes' docs\"", "aggregated_output": ""})
	assert.False(t, observer.result.Interference.Triggered, "a command naming the text has not shown it")
	// The agent writes without reading, as kapi-files ksed allows.
	body, err := os.ReadFile(file)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(file, []byte(strings.Replace(string(body), "your data before", "your data and settings before", 1)), 0o600))
	observer.codexCompleted("command_execution", map[string]any{
		"command": "/bin/zsh -c 'kapi-files ksed -i s/x/y/ docs/en/upgrade.md'", "aggregated_output": "wrote docs/en/upgrade.md"})
	assert.False(t, observer.result.Interference.Triggered)
	observer.codexCompleted("command_execution", map[string]any{
		"command": "/bin/zsh -c 'cat docs/en/upgrade.md'", "aggregated_output": "messages, and it takes about five minutes."})
	record := observer.result.Interference
	require.NotNil(t, record)
	assert.True(t, record.Applied)
	assert.True(t, record.AgentWroteFirst)
	assert.Equal(t, "command_execution", record.Trigger)
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
		// Claude Code 2.1 applies the edit and notes that the file changed.
		pairedToolUse("4", "Edit", map[string]any{"file_path": "b.md"}),
		pairedToolResult("4", "The file b.md has been updated successfully. (note: the file had been modified on disk since you last read it; the edit was applied to the current contents)"),
		// A change set that does not decode, from the CLI and over MCP.
		pairedToolUse("5", "Bash", map[string]any{"command": "kapi-files apply edits.json"}),
		pairedToolResult("5", "Exit code 2\nError: apply: invalid at /type: unknown field \"type\"; a change set takes schema, mode, gate, require_basis, note, evidence, ops"),
		pairedToolUse("6", "mcp__kapi__apply_edits", map[string]any{"ops": []any{}}),
		pairedToolResult("6", `{"schema":"kapi.change-result/v1","status":"refused","error":{"code":"invalid","pointer":"/ops/3/edits/0/replace","message":"unknown field"}}`),
		pairedToolUse("7", "Bash", map[string]any{"command": "kapi apply bad.json"}),
		pairedToolResult("7", "Error: apply: invalid: unexpected end of JSON input"),
		// A writer's refusal of a branch holding ICU syntax, from the CLI and
		// as a bare MCP tool error.
		pairedToolUse("8", "Bash", map[string]any{"command": "kapi apply plural.json"}),
		pairedToolResult("8", "Exit code 1\nError: prepare lib/l10n/app_en.arb: arb writer: the message would not read back as written: message inboxCount: unexpected '}'"),
		pairedToolUse("9", "mcp__kapi__apply_edits", map[string]any{"ops": []any{}}),
		pairedToolResult("9", "prepare lib/l10n/app_en.arb: arb writer: the message would not read back as written: message inboxCount: the text of a branch holds ICU syntax"),
	)
	result, err := parsePairedAgentStream(strings.NewReader(stream), PairedLaunch{
		Agent: PairedAgentSpec{Host: "claude", Model: "test"}, Condition: "mcp",
	})
	require.NoError(t, err)
	assert.Equal(t, map[string]int{
		"gate_failed": 1, "stale": 1, "host:stale": 2,
		"invalid:/type": 1, "invalid:/ops/*/edits/*/replace": 1, "invalid": 1,
		"write:not_read_back": 2,
	}, result.Refusals)
}

func TestPairedRouteAudit(t *testing.T) {
	state, workspace := t.TempDir(), t.TempDir()
	for _, tc := range []struct {
		name, condition, tool, command string
		input                          map[string]any
		violation                      bool
		attempts                       []string
	}{
		{name: "bare kapi in the baseline is an attempt", condition: "baseline", tool: "Bash", command: "kapi --help", attempts: []string{"kapi"}},
		{name: "kapi by path outside the cell", condition: "skill-cli", tool: "Bash", command: "/opt/homebrew/bin/kapi inspect a.md", violation: true},
		{name: "kapi by path inside a Codex shell", condition: "skill-cli", tool: "shell",
			command: `/bin/zsh -c "cd docs && /opt/homebrew/bin/kapi inspect a.md"`, violation: true},
		{name: "kapi by path after env", condition: "mcp", tool: "Bash", command: "env KAPI_ACTOR=agent ~/.local/bin/kapi apply x.json", violation: true},
		{name: "the cell's own kapi by path", condition: "skill-cli", tool: "Bash", command: filepath.Join(state, "bin", "kapi") + " inspect a.md"},
		{name: "kapi in the MCP arm", condition: "mcp", tool: "Bash", command: "kapi inspect a.md"},
		{name: "toolbox under the alias", condition: "project-free", tool: "Bash", command: "kapi-files ksed 's/a/b/' a.md"},
		{name: "kapi in the project-free arm", condition: "project-free", tool: "Bash", command: "kapi inspect a.md", attempts: []string{"kapi"}},
		{name: "kapi in a pipeline of the project-free arm", condition: "project-free", tool: "Bash",
			command: "cat edits.json | kapi apply -", attempts: []string{"kapi"}},
		// The arm's own skill folder is an argument, not a program.
		{name: "listing the kapi skill", condition: "skill-cli", tool: "Bash", command: "ls .agents/skills/kapi"},
		{name: "listing the kapi skill recursively", condition: "skill-cli", tool: "Bash", command: "ls -R .claude/skills/kapi/"},
		{name: "finding the alias skill's files", condition: "project-free", tool: "Bash", command: "find .agents/skills/kapi-files -type f"},
		{name: "reading the skill in a Codex shell", condition: "skill-cli", tool: "shell",
			command: `/bin/zsh -c "sed -n '1,240p' .agents/skills/kapi/SKILL.md && rg -n -F 'kapi' docs/en/upgrade.md"`},
		{name: "a heredoc that mentions kapi", condition: "baseline", tool: "Bash",
			command: "cat > notes.txt <<'EOF'\nkapi inspect a.md\n/opt/homebrew/bin/kapi apply x\nEOF\nwc -l notes.txt"},
		{name: "searching for the word kapi", condition: "baseline", tool: "Bash", command: "grep -rn kapi docs"},
		{name: "the alias's skill in its arm", condition: "project-free", tool: "Skill", input: map[string]any{"skill": "kapi-files"}},
		// A skill the arm does not hold is not installed, so calling one
		// loads nothing: it is recorded, and the session goes on.
		{name: "kapi's skill in the project-free arm", condition: "project-free", tool: "Skill", input: map[string]any{"skill": "kapi"},
			attempts: []string{"skill:kapi"}},
		{name: "a skill in the baseline", condition: "baseline", tool: "Skill", input: map[string]any{"skill": "kapi"}, attempts: []string{"skill:kapi"}},
		{name: "a host-bundled skill in a skill arm", condition: "skill-cli", tool: "Skill", input: map[string]any{"skill": "verify"},
			attempts: []string{"skill:verify"}},
		{name: "kapi MCP outside the MCP arm", condition: "skill-cli", tool: "mcp__kapi__read_blocks", input: map[string]any{}, violation: true},
		{name: "foreign MCP in the MCP arm", condition: "mcp", tool: "mcp__other__read", input: map[string]any{}, violation: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			input := tc.input
			if input == nil {
				input = map[string]any{"command": tc.command}
			}
			result := &PairedAgentResult{}
			observer := newPairedObserver(PairedLaunch{Condition: tc.condition, StateDir: state, Workspace: workspace}, result)
			violation := observer.toolUse("1", tc.tool, input)
			assert.Equal(t, tc.violation, violation != "", violation)
			assert.Equal(t, tc.attempts, result.RouteAttempts)
		})
	}
}

// Paths a tool call names outside the cell are recorded with where they lie:
// another attempt or the study's records, the checkout, shared temporary
// directories. The cell's own files and system programs are not.
func TestPairedOutsideCellAudit(t *testing.T) {
	study := t.TempDir()
	attempt := filepath.Join(study, "pilot", "task-claude-baseline-01")
	workspace, state := filepath.Join(attempt, "workspace"), filepath.Join(attempt, "state")
	sibling := filepath.Join(study, "pilot", "task-claude-baseline-02", "workspace", "docs", "en", "upgrade.md")
	require.NoError(t, os.MkdirAll(filepath.Join(workspace, "docs"), 0o700))
	require.NoError(t, os.MkdirAll(state, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(attempt, "preparation.json"), []byte("{}"), 0o600))
	repo, main := t.TempDir(), t.TempDir()
	tmp := t.TempDir()
	launch := PairedLaunch{Condition: "baseline", Workspace: workspace, StateDir: state, StudyDir: study, RepoRoot: repo,
		MainCheckout: main, TmpDir: tmp}
	for _, tc := range []struct {
		name, tool string
		input      map[string]any
		want       []string
	}{
		{name: "the cell's own files", tool: "Bash", input: map[string]any{"command": "cat " + filepath.Join(workspace, "docs", "a.md") + " > " + filepath.Join(tmp, "x")}},
		{name: "system programs", tool: "shell", input: map[string]any{"command": "/bin/zsh -c '/usr/bin/python3 /dev/null'"}},
		{name: "a URL", tool: "Bash", input: map[string]any{"command": "echo https://harbor.example/help"}},
		{name: "the cell's HOME", tool: "Bash", input: map[string]any{"command": "ls ~/"}},
		{name: "another attempt", tool: "Read", input: map[string]any{"file_path": sibling}, want: []string{"study:" + sibling}},
		{name: "the attempt's records", tool: "Bash", input: map[string]any{"command": "cat ../preparation.json"}, want: []string{"study:../preparation.json"}},
		{name: "a parent that holds nothing", tool: "Bash", input: map[string]any{"command": "cat ../STYLE.md"}},
		{name: "the checkout", tool: "Grep", input: map[string]any{"path": filepath.Join(repo, "scripts")}, want: []string{"checkout:" + filepath.Join(repo, "scripts")}},
		{name: "the main checkout of the study's worktree", tool: "Read", input: map[string]any{"file_path": filepath.Join(main, "docs", "internals", "evals.md")},
			want: []string{"checkout:" + filepath.Join(main, "docs", "internals", "evals.md")}},
		{name: "a shared temporary file", tool: "Write", input: map[string]any{"file_path": "/tmp/claude/upgrade_edit.json"}, want: []string{"temp:/tmp/claude/upgrade_edit.json"}},
		{name: "a Codex patch", tool: "file_change", input: map[string]any{"changes": []any{map[string]any{"path": filepath.Join(workspace, "docs", "a.md")}}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result := &PairedAgentResult{}
			newPairedObserver(launch, result).auditPaths(tc.tool, tc.input)
			assert.Equal(t, tc.want, result.OutsideCell)
		})
	}
}

// An infrastructure failure is told apart from the agent's own, so it can be
// run again and the host paused.
func TestPairedInfraFailures(t *testing.T) {
	for text, want := range map[string]string{
		"API Error: 529 {\"type\":\"error\",\"error\":{\"type\":\"overloaded_error\"}}":      "overload",
		"OAuth token has expired. Please obtain a new token or refresh your existing token.": "auth",
		"Invalid API key · Please run /login":                                                "auth",
		"stream disconnected before completion: error sending request":                       "network",
		"Error: fetch failed (ECONNRESET)":                                                   "network",
		"API Error: Request timed out.":                                                      "network",
		"Selected model is at capacity. Please try a different model.":                       "overload",
		"write out/nb.xliff: no space left on device":                                        "disk",
		"Reached maximum number of turns (40)":                                               "",
		"The agent could not finish the edit":                                                "",
	} {
		assert.Equal(t, want, pairedInfraFailure(text), text)
	}
	stream := `{"type":"system","subtype":"init","model":"test","session_id":"s"}` + "\n" +
		`{"type":"result","subtype":"error_during_execution","is_error":true,"result":"API Error: 529 Overloaded"}`
	result, err := parsePairedAgentStream(strings.NewReader(stream), PairedLaunch{Agent: PairedAgentSpec{Host: "claude", Model: "test"}, Condition: "baseline"})
	require.Error(t, err)
	assert.Equal(t, "infra_failed", result.Status)
	assert.Equal(t, "overload", result.InfraFailure)
	stream = `{"type":"thread.started","thread_id":"t"}` + "\n" + `{"type":"error","message":"stream disconnected before completion: error sending request for url"}`
	result, err = parsePairedAgentStream(strings.NewReader(stream), PairedLaunch{Agent: PairedAgentSpec{Host: "codex", Model: "test"}, Condition: "baseline"})
	require.Error(t, err)
	assert.Equal(t, "infra_failed", result.Status)
	assert.Equal(t, "network", result.InfraFailure)
}
