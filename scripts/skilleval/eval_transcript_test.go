package main

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func claudeToolEvent(name, input string) string {
	return `{"type":"assistant","message":{"model":"claude-sonnet-5","content":[{"type":"tool_use","name":"` +
		name + `","input":` + input + `}]}}`
}

func claudeStream(events ...string) string {
	head := `{"type":"system","subtype":"init","session_id":"s-1","model":"claude-sonnet-5"}`
	tail := `{"type":"result","subtype":"success","result":"done","usage":{"input_tokens":10,"output_tokens":4}}`
	return strings.Join(append(append([]string{head}, events...), tail), "\n") + "\n"
}

func TestReadEvalTranscriptClassifiesClaudeCalls(t *testing.T) {
	stream := claudeStream(
		claudeToolEvent("mcp__kapi__context_search", `{"query":"sign in"}`),
		claudeToolEvent("Read", `{"file_path":"docs/troubleshooting.md"}`),
		claudeToolEvent("mcp__kapi__context_observe", `{"text":"the docs address the reader as you"}`),
		claudeToolEvent("Write", `{"file_path":"docs/troubleshooting.md"}`),
		claudeToolEvent("mcp__kapi__check_file", `{"path":"docs/troubleshooting.md"}`),
	)

	transcript, err := readEvalTranscript(strings.NewReader(stream), "claude")
	require.NoError(t, err)
	assert.Equal(t, "completed", transcript.Status)
	assert.Equal(t, "claude-sonnet-5", transcript.ActualModel)
	assert.Equal(t, "s-1", transcript.SessionID)
	assert.True(t, transcript.UsageObserved)
	assert.Equal(t, int64(10), transcript.InputTokens)

	assert.Equal(t, []string{"context_search", "context_observe", "check_file"},
		evalToolNames(transcript.kapiCalls()))
	assert.Len(t, transcript.ofKind(evalKindRecord), 1)
	assert.Len(t, transcript.ofKind(evalKindCheck), 1)
	assert.True(t, transcript.AskedBeforeWriting)
}

func TestReadEvalTranscriptSeesAWriteBeforeAnyAsk(t *testing.T) {
	stream := claudeStream(
		claudeToolEvent("Write", `{"file_path":"docs/plans.md"}`),
		claudeToolEvent("mcp__kapi__context_search", `{"query":"plan"}`),
	)
	transcript, err := readEvalTranscript(strings.NewReader(stream), "claude")
	require.NoError(t, err)
	assert.False(t, transcript.AskedBeforeWriting)
}

func TestReadEvalTranscriptOfASessionThatIgnoredKapi(t *testing.T) {
	stream := claudeStream(
		claudeToolEvent("Read", `{"file_path":"README.md"}`),
		claudeToolEvent("Write", `{"file_path":"docs/releases/2026-09.md"}`),
	)
	transcript, err := readEvalTranscript(strings.NewReader(stream), "claude")
	require.NoError(t, err)
	assert.Empty(t, transcript.kapiCalls(), "a session that reached no kapi surface is the negative result")
	assert.False(t, transcript.AskedBeforeWriting)
	assert.False(t, transcript.SkillLoaded)
	assert.Equal(t, "none", evalCallSummary(transcript))
}

func TestReadEvalTranscriptReadsShellCommands(t *testing.T) {
	stream := claudeStream(
		claudeToolEvent("Bash", `{"command":"kapi context docs/billing.md"}`),
		claudeToolEvent("Bash", `{"command":"cd docs && kapi context observe \"the docs say sign in\" --seen-in billing.md"}`),
		claudeToolEvent("Bash", `{"command":"kapi context correct \"log in\" \"sign in\" --seen-in docs/troubleshooting.md --suggest"}`),
		claudeToolEvent("Bash", `{"command":"git status"}`),
		claudeToolEvent("Bash", `{"command":"kapi check --diff-against HEAD --json"}`),
	)

	transcript, err := readEvalTranscript(strings.NewReader(stream), "claude")
	require.NoError(t, err)
	assert.Equal(t,
		[]string{"kapi context", "kapi context observe", "kapi context correct", "kapi check"},
		evalToolNames(transcript.kapiCalls()))
	assert.Len(t, transcript.ofKind(evalKindRecord), 2)
	assert.Len(t, transcript.ofKind(evalKindAsk), 1)
	assert.Len(t, transcript.ofKind(evalKindCheck), 1)
}

func TestReadEvalTranscriptReadsAContextResource(t *testing.T) {
	stream := claudeStream(
		claudeToolEvent("ReadMcpResourceTool", `{"server":"kapi","uri":"context://docs/billing.md"}`),
		claudeToolEvent("Write", `{"file_path":"docs/plans.md"}`),
	)
	transcript, err := readEvalTranscript(strings.NewReader(stream), "claude")
	require.NoError(t, err)
	assert.Equal(t, []string{evalContextResource}, evalToolNames(transcript.kapiCalls()))
	assert.True(t, transcript.AskedBeforeWriting)
}

func TestReadEvalTranscriptSeesTheSkillLoad(t *testing.T) {
	stream := claudeStream(claudeToolEvent("Skill", `{"skill":"kapi"}`))
	transcript, err := readEvalTranscript(strings.NewReader(stream), "claude")
	require.NoError(t, err)
	assert.True(t, transcript.SkillLoaded)

	read := claudeStream(claudeToolEvent("Read", `{"file_path":"/w/.agents/skills/kapi/SKILL.md"}`))
	transcript, err = readEvalTranscript(strings.NewReader(read), "claude")
	require.NoError(t, err)
	assert.True(t, transcript.SkillLoaded, "a host with no skill loader still meets the guidance by reading it")
}

func TestReadEvalTranscriptOfCodex(t *testing.T) {
	stream := strings.Join([]string{
		`{"type":"thread.started","thread_id":"t-1","model":"gpt-5.6-terra"}`,
		`{"type":"item.completed","item":{"type":"mcp_tool_call","server":"kapi","tool":"context_observe"}}`,
		`{"type":"item.completed","item":{"type":"mcp_tool_call","server":"codex","tool":"read_mcp_resource","arguments":{"server":"kapi","uri":"context://docs/billing.md"}}}`,
		`{"type":"item.completed","item":{"type":"command_execution","command":"kapi check --diff-against HEAD"}}`,
		`{"type":"item.completed","item":{"type":"file_change","changes":[{"path":"docs/plans.md"}]}}`,
		`{"type":"item.completed","item":{"type":"agent_message","text":"written"}}`,
		`{"type":"turn.completed","usage":{"input_tokens":20,"output_tokens":5}}`,
	}, "\n") + "\n"

	transcript, err := readEvalTranscript(strings.NewReader(stream), "codex")
	require.NoError(t, err)
	assert.Equal(t, "completed", transcript.Status)
	assert.Equal(t, "gpt-5.6-terra", transcript.ActualModel)
	assert.Equal(t, []string{"context_observe", evalContextResource, "kapi check"},
		evalToolNames(transcript.kapiCalls()))
	assert.Equal(t, "written", transcript.FinalText)
	assert.True(t, transcript.AskedBeforeWriting)
	assert.Len(t, transcript.ofKind(evalKindWrite), 1)
}

func TestReadEvalTranscriptReportsARateLimit(t *testing.T) {
	stream := strings.Join([]string{
		`{"type":"thread.started","thread_id":"t-2","model":"gpt-5.6-terra"}`,
		`{"type":"turn.failed","error":{"message":"usage limit reached for this account"}}`,
	}, "\n") + "\n"
	transcript, err := readEvalTranscript(strings.NewReader(stream), "codex")
	require.NoError(t, err)
	assert.Equal(t, "rate_limited", transcript.Status)
	assert.True(t, transcript.RateLimited)
}

func TestReadEvalTranscriptRejectsAMalformedStream(t *testing.T) {
	_, err := readEvalTranscript(strings.NewReader("{not json}\n"), "claude")
	require.ErrorContains(t, err, "invalid agent event")
}

func TestEvalKapiRoute(t *testing.T) {
	tests := []struct {
		command string
		want    string
		kind    string
	}{
		{command: "kapi context docs/billing.md", want: "kapi context", kind: evalKindAsk},
		{command: "kapi context search widget", want: "kapi context search", kind: evalKindAsk},
		{command: "kapi context observe \"they say you\"", want: "kapi context observe", kind: evalKindRecord},
		{command: "kapi context withdraw 14", want: "kapi context withdraw", kind: evalKindRecord},
		{command: "kapi context log --limit 10", want: "kapi context log", kind: evalKindAsk},
		{command: "kapi check --diff-against HEAD", want: "kapi check", kind: evalKindCheck},
		{command: "kapi apply --from edits.jsonl", want: "kapi apply", kind: evalKindWrite},
		{command: "kcat docs/billing.md", want: "kcat", kind: evalKindOther},
	}
	for _, test := range tests {
		t.Run(test.command, func(t *testing.T) {
			transcript := EvalTranscript{Calls: []EvalCall{}}
			transcript.addShellCall(test.command)
			require.Len(t, transcript.Calls, 1)
			assert.Equal(t, test.want, transcript.Calls[0].Tool)
			assert.Equal(t, test.kind, transcript.Calls[0].Kind)
			assert.Equal(t, "cli", transcript.Calls[0].Surface)
		})
	}
}
