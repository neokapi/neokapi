package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPairedAgentStream(t *testing.T) {
	tests := []struct {
		name, host, condition, model, stream, status string
		success                                      bool
	}{
		{name: "Claude complete", host: "claude", condition: "baseline", model: "claude-sonnet-5", stream: `{"type":"system","subtype":"init","model":"claude-sonnet-5","session_id":"s"}
{"type":"assistant","message":{"model":"claude-sonnet-5","content":[{"type":"tool_use","name":"Read","input":{"file_path":"copy.md"}}]}}
{"type":"result","subtype":"success","is_error":false,"usage":{"input_tokens":12,"output_tokens":7},"result":"Done"}`, status: "completed", success: true},
		{name: "Codex complete", host: "codex", condition: "mcp", model: "gpt-5.6-terra", stream: `{"type":"thread.started","model":"gpt-5.6-terra","thread_id":"s"}
{"type":"item.completed","item":{"type":"mcp_tool_call","server":"kapi","tool":"check"}}
{"type":"turn.completed","usage":{"input_tokens":10,"output_tokens":3}}`, status: "completed", success: true},
		{name: "missing identity", host: "codex", condition: "baseline", model: "gpt-5.6-terra", stream: `{"type":"thread.started","thread_id":"s"}
{"type":"turn.completed","usage":{"input_tokens":10,"output_tokens":3}}`, status: "identity_unverified"},
		{name: "model mismatch", host: "claude", condition: "baseline", model: "claude-sonnet-5", stream: `{"type":"system","subtype":"init","model":"claude-opus-5"}
{"type":"result","subtype":"success"}`, status: "model_mismatch"},
		{name: "midrun fallback", host: "claude", condition: "baseline", model: "claude-sonnet-5", stream: `{"type":"system","subtype":"init","model":"claude-sonnet-5"}
{"type":"assistant","message":{"model":"claude-opus-5","content":[]}}
{"type":"result","subtype":"success"}`, status: "model_mismatch"},
		{name: "malformed", host: "claude", condition: "baseline", model: "claude-sonnet-5", stream: `{"type":`, status: "malformed_stream"},
		{name: "not an event", host: "claude", condition: "baseline", model: "claude-sonnet-5", stream: `{}`, status: "malformed_stream"},
		{name: "forbidden CLI", host: "codex", condition: "mcp", model: "gpt-5.6-terra", stream: `{"type":"item.started","item":{"type":"command_execution","command":"/opt/tools/kapi check"}}`, status: "route_violation"},
		{name: "forbidden MCP", host: "claude", condition: "skill-cli", model: "claude-sonnet-5", stream: `{"type":"assistant","message":{"content":[{"type":"tool_use","name":"mcp__kapi__check","input":{}}]}}`, status: "route_violation"},
		{name: "foreign MCP", host: "codex", condition: "mcp", model: "gpt-5.6-terra", stream: `{"type":"item.completed","item":{"type":"mcp_tool_call","server":"other","tool":"read"}}`, status: "route_violation"},
		{name: "rate limit", host: "codex", condition: "baseline", model: "gpt-5.6-terra", stream: `{"type":"error","message":"You have reached your usage limit"}`, status: "rate_limited"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := parsePairedAgentStream(strings.NewReader(tc.stream), PairedLaunch{Agent: PairedAgentSpec{Host: tc.host, Model: tc.model}, Condition: tc.condition})
			assert.Equal(t, tc.status, result.Status)
			if tc.success {
				require.NoError(t, err)
				assert.True(t, result.UsageObserved)
				assert.NotEmpty(t, result.Tools)
			} else {
				require.Error(t, err)
			}
		})
	}
}

func TestPairedRunTimeoutAndFailedLaunch(t *testing.T) {
	for _, tc := range []struct {
		name, script, status string
		timeout              time.Duration
	}{
		{name: "timeout", script: "#!/bin/sh\nwhile :; do :; done\n", status: "timeout", timeout: 30 * time.Millisecond},
		{name: "malformed", script: "#!/bin/sh\nprintf 'garbage\\n'\n", status: "malformed_stream", timeout: time.Second},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			binary := filepath.Join(dir, "agent")
			require.NoError(t, os.WriteFile(binary, []byte(tc.script), 0o700))
			result, err := runPairedAgent(context.Background(), PairedPrepared{Executable: binary, Args: []string{}, Env: []string{}, Blockers: []string{}, Launch: PairedLaunch{Agent: PairedAgentSpec{Host: "claude", Model: "test"}, Condition: "baseline", Workspace: dir, TranscriptPath: filepath.Join(dir, "transcript.jsonl"), Timeout: tc.timeout}})
			require.Error(t, err)
			assert.Equal(t, tc.status, result.Status)
		})
	}
}

func TestPairedRedactionAcrossWrites(t *testing.T) {
	var output bytes.Buffer
	writer := newPairedRedactor(&output, []string{"CLAUDE_CODE_OAUTH_TOKEN=private-token"})
	_, err := writer.Write([]byte("prefix private-"))
	require.NoError(t, err)
	_, err = writer.Write([]byte("token suffix\nprivate-token"))
	require.NoError(t, err)
	require.NoError(t, writer.Flush())
	assert.Equal(t, "prefix [REDACTED] suffix\n[REDACTED]", output.String())
}

func TestPairedPreparedNeverSerializesEnvironment(t *testing.T) {
	data, err := json.Marshal(PairedPrepared{Env: []string{"CLAUDE_CODE_OAUTH_TOKEN=private"}})
	require.NoError(t, err)
	assert.NotContains(t, string(data), "private")
	assert.NotContains(t, string(data), "TOKEN")
}

func TestPairedCodexModelFromOwnRollout(t *testing.T) {
	state := t.TempDir()
	dir := filepath.Join(state, "codex", "sessions", "2026", "09", "10")
	require.NoError(t, os.MkdirAll(dir, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "rollout-session.jsonl"), []byte(`{"type":"turn_context","payload":{"model":"gpt-5.6-terra"}}`), 0o600))
	actual, err := pairedCodexRolloutModel(state, "session")
	require.NoError(t, err)
	assert.Equal(t, "gpt-5.6-terra", actual)
	_, err = pairedCodexRolloutModel(state, "../session")
	require.Error(t, err)
}
