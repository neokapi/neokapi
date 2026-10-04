package main

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// An mcp attempt records what the host gave the model: the kapi tools its
// tool list declared, or a kapi call where it declares none. A list without
// them fails the attempt, which did not run the arm.
func TestPairedMCPExposure(t *testing.T) {
	launch := PairedLaunch{Agent: PairedAgentSpec{Host: "claude", Model: "test"}, Condition: "mcp"}
	end := `{"type":"result","subtype":"success","is_error":false,"result":"done","usage":{"input_tokens":1,"output_tokens":1}}`
	init := func(tools string) string {
		return `{"type":"system","subtype":"init","model":"test","session_id":"s","tools":[` + tools + `]}` + "\n"
	}

	result, err := parsePairedAgentStream(t.Context(), strings.NewReader(init(`"Bash","mcp__kapi__read_blocks","mcp__kapi__apply_edits"`)+end), launch)
	require.NoError(t, err)
	assert.Equal(t, "declared", result.MCPExposure)
	assert.Equal(t, []string{"mcp__kapi__read_blocks", "mcp__kapi__apply_edits"}, result.MCPGiven)

	result, err = parsePairedAgentStream(t.Context(), strings.NewReader(init(`"Bash","Read"`)+end), launch)
	require.Error(t, err)
	assert.Equal(t, "mcp_absent", result.Status)
	assert.Equal(t, "absent", result.MCPExposure)

	codex := PairedLaunch{Agent: PairedAgentSpec{Host: "codex", Model: "test"}, Condition: "mcp"}
	stream := `{"type":"thread.started","thread_id":"t","model":"test"}` + "\n" +
		`{"type":"item.completed","item":{"type":"mcp_tool_call","server":"kapi","tool":"read_blocks","arguments":{"doc":"a.md"}}}` + "\n" +
		`{"type":"turn.completed","usage":{"input_tokens":1,"output_tokens":1}}`
	result, err = parsePairedAgentStream(t.Context(), strings.NewReader(stream), codex)
	require.NoError(t, err)
	assert.Equal(t, "called", result.MCPExposure)

	stream = `{"type":"thread.started","thread_id":"t","model":"test"}` + "\n" + `{"type":"turn.completed","usage":{"input_tokens":1,"output_tokens":1}}`
	result, err = parsePairedAgentStream(t.Context(), strings.NewReader(stream), codex)
	require.NoError(t, err)
	assert.Equal(t, "unverified", result.MCPExposure)

	baseline := launch
	baseline.Condition = "baseline"
	result, err = parsePairedAgentStream(t.Context(), strings.NewReader(init(`"Bash"`)+end), baseline)
	require.NoError(t, err)
	assert.Empty(t, result.MCPExposure, "only an mcp arm is held to it")
}

// The summary leaves a failed mcp attempt out and counts what the rest show.
func TestPairedSummaryReportsMCPExposure(t *testing.T) {
	tokens := func(n int64) *int64 { return &n }
	session := PairedSession{Task: "edit-po-context", Agent: PairedAgentSpec{Host: "codex"}, Condition: "mcp"}
	row := func(status, exposure string) pairedScoreRow {
		return pairedScoreRow{Session: session, Phase: "main", Status: status, MCPExposure: exposure,
			InputTokens: tokens(1), OutputTokens: tokens(1)}
	}
	var markdown strings.Builder
	writePairedSummary(&markdown, []pairedScoreRow{row("completed", "called"), row("completed", "unverified"), row("mcp_absent", "absent")})
	text := markdown.String()
	assert.Contains(t, text, "1 mcp attempts are left out: the host gave the model no kapi tools")
	assert.Contains(t, text, "declared by the host 0, called without a declared list 1, unverified 1")
	assert.Contains(t, text, "| edit-po-context | codex | mcp | 2 |")
}
