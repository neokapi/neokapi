package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A Codex session's rollout records the calls its exec stream leaves out: a
// rejected patch and the apply_patch body. The count, the refusals and the
// native route come from it where the stream shows fewer.
func TestPairedCodexCallsComeFromTheRollout(t *testing.T) {
	state := t.TempDir()
	dir := filepath.Join(state, "codex", "sessions", "2026", "10", "03")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	lines := []string{
		`{"type":"session_meta","payload":{"id":"abc"}}`,
		`{"type":"response_item","payload":{"type":"custom_tool_call","call_id":"c1","name":"exec","input":"cat docs/en/reports.md"}}`,
		`{"type":"response_item","payload":{"type":"custom_tool_call_output","call_id":"c1","output":"# Reports"}}`,
		`{"type":"response_item","payload":{"type":"custom_tool_call","call_id":"c2","name":"apply_patch","input":"*** Begin Patch\n*** Update File: docs/en/reports.md\n@@\n-a\n+b\n*** End Patch"}}`,
		`{"type":"response_item","payload":{"type":"custom_tool_call_output","call_id":"c2","output":"apply_patch verification failed: Failed to find expected lines"}}`,
		`{"type":"response_item","payload":{"type":"custom_tool_call","call_id":"c3","name":"apply_patch","input":"*** Begin Patch\n*** Update File: /cell/workspace/docs/en/reports.md\n*** End Patch"}}`,
		`{"type":"response_item","payload":{"type":"custom_tool_call_output","call_id":"c3","output":"Success"}}`,
	}
	require.NoError(t, os.WriteFile(filepath.Join(dir, "rollout-2026-10-03T00-00-00-abc.jsonl"), []byte(strings.Join(lines, "\n")+"\n"), 0o600))

	result := &PairedAgentResult{SessionID: "abc", ToolCalls: 1}
	mergePairedCodexRollout(result, PairedLaunch{StateDir: state, Task: "recover-gate-refusal", Workspace: "/cell/workspace"})
	assert.Equal(t, 3, result.ToolCalls)
	assert.Equal(t, "rollout", result.ToolCallsSource)
	assert.Equal(t, 1, result.Refusals["host:patch_failed"])
	assert.Equal(t, []string{pairedRouteNative}, result.WriteRoutes)

	// A stream that saw as many calls keeps its own count.
	result = &PairedAgentResult{SessionID: "abc", ToolCalls: 5}
	mergePairedCodexRollout(result, PairedLaunch{StateDir: state, Task: "recover-gate-refusal"})
	assert.Equal(t, 5, result.ToolCalls)
	assert.Equal(t, "stream", result.ToolCallsSource)
}
