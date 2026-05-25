package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// runHookStopCapture invokes the stop hook with the given stdin payload and
// returns the parsed decision plus the raw stdout (empty when the hook allows
// Claude to stop).
func runHookStopCapture(t *testing.T, stdin string) (stopHookDecision, string) {
	t.Helper()
	a := &App{}
	cmd := a.newHookStopCmd()
	cmd.SetIn(strings.NewReader(stdin))
	var buf bytes.Buffer
	cmd.SetOut(&buf)

	require.NoError(t, a.runHookStop(cmd), "the stop hook must not error — its verdict is the JSON on stdout")

	raw := strings.TrimSpace(buf.String())
	var dec stopHookDecision
	if raw != "" {
		require.NoError(t, json.Unmarshal([]byte(raw), &dec), "hook output must be valid JSON: %s", raw)
	}
	return dec, raw
}

// makeVerifyPass rewrites the project's source and target files so all gates
// pass: the competitor term is removed from the source and the target keeps the
// placeholder and uses the approved glossary term.
func makeVerifyPass(t *testing.T, root, targetFile string) {
	t.Helper()
	goodSrc := `{
  "greeting": "Hello {name}, welcome!",
  "save": "Save"
}
`
	require.NoError(t, os.WriteFile(filepath.Join(root, "locales", "en", "app.json"), []byte(goodSrc), 0o644))
	good := `{
  "greeting": "Bonjour {name}, bienvenue!",
  "save": "Enregistrer"
}
`
	require.NoError(t, os.WriteFile(targetFile, []byte(good), 0o644))
}

// TestHookStop_BlocksOnFailingProject asserts that a project with failing gates
// makes the stop hook block, with the verify findings carried in the reason.
func TestHookStop_BlocksOnFailingProject(t *testing.T) {
	root, _ := writeVerifyProject(t)
	t.Chdir(root)

	dec, raw := runHookStopCapture(t, `{"hook_event_name":"Stop","session_id":"s1"}`)

	require.NotEmpty(t, raw, "a failing project must produce a block decision")
	assert.Equal(t, "block", dec.Decision, "the hook must keep Claude working")
	assert.Contains(t, dec.Reason, "kapi verify", "the reason should point at the gate")
	assert.Contains(t, dec.Reason, "Enregistrer", "the terminology finding should be surfaced to Claude")
}

// TestHookStop_HonorsPayloadCWD asserts that the hook evaluates the project in
// the cwd from the Stop payload, even when the process starts elsewhere.
func TestHookStop_HonorsPayloadCWD(t *testing.T) {
	root, _ := writeVerifyProject(t)
	// Start somewhere without a project; t.Chdir restores the original cwd at
	// cleanup, so the hook's os.Chdir into root does not leak between tests.
	t.Chdir(t.TempDir())

	payload := fmt.Sprintf(`{"hook_event_name":"Stop","cwd":%q}`, root)
	dec, raw := runHookStopCapture(t, payload)

	require.NotEmpty(t, raw, "the hook should resolve the project from the payload cwd")
	assert.Equal(t, "block", dec.Decision)
}

// TestHookStop_AllowsPassingProject asserts that when the gates pass the hook
// emits nothing, leaving Claude free to stop.
func TestHookStop_AllowsPassingProject(t *testing.T) {
	root, targetFile := writeVerifyProject(t)
	makeVerifyPass(t, root, targetFile)
	t.Chdir(root)

	dec, raw := runHookStopCapture(t, `{"hook_event_name":"Stop"}`)

	assert.Empty(t, raw, "a passing project must not block — Claude may stop")
	assert.Empty(t, dec.Decision)
}

// TestHookStop_AllowsWhenNoProject asserts the hook fails open: outside any
// .kapi project there is nothing to gate, so Claude may stop.
func TestHookStop_AllowsWhenNoProject(t *testing.T) {
	t.Chdir(t.TempDir())

	dec, raw := runHookStopCapture(t, `{"hook_event_name":"Stop"}`)

	assert.Empty(t, raw, "no project → no decision")
	assert.Empty(t, dec.Decision)
}

// TestHookStop_AllowsOnEmptyStdin asserts that an empty/garbage payload (e.g.
// run by hand) does not error or block when there is no project.
func TestHookStop_AllowsOnEmptyStdin(t *testing.T) {
	t.Chdir(t.TempDir())

	dec, raw := runHookStopCapture(t, "")

	assert.Empty(t, raw)
	assert.Empty(t, dec.Decision)
}

// runHookPreEditCapture invokes the pre-edit hook with the given stdin payload
// and returns the parsed decision plus the raw stdout (empty when the hook
// allows the edit to proceed).
func runHookPreEditCapture(t *testing.T, stdin string) (preToolUseDecision, string) {
	t.Helper()
	a := &App{}
	cmd := a.newHookPreEditCmd()
	cmd.SetIn(strings.NewReader(stdin))
	var buf bytes.Buffer
	cmd.SetOut(&buf)

	require.NoError(t, a.runHookPreEdit(cmd), "the pre-edit hook must not error — its verdict is the JSON on stdout")

	raw := strings.TrimSpace(buf.String())
	var dec preToolUseDecision
	if raw != "" {
		require.NoError(t, json.Unmarshal([]byte(raw), &dec), "hook output must be valid JSON: %s", raw)
	}
	return dec, raw
}

// preEditPayload builds a PreToolUse stdin payload for the Edit tool writing
// the given absolute file path, with cwd set to root.
func preEditPayload(root, file string) string {
	return fmt.Sprintf(`{"hook_event_name":"PreToolUse","tool_name":"Edit","cwd":%q,"tool_input":{"file_path":%q}}`, root, file)
}

// TestHookPreEdit_DeniesTargetFile asserts that editing a file the project
// generates as a translation target is denied, with the reason pointing at the
// source, locale, and the extract → merge round-trip.
func TestHookPreEdit_DeniesTargetFile(t *testing.T) {
	root, targetFile := writeVerifyProject(t)
	t.Chdir(root)

	dec, raw := runHookPreEditCapture(t, preEditPayload(root, targetFile))

	require.NotEmpty(t, raw, "editing a generated target must produce a deny decision")
	assert.Equal(t, "PreToolUse", dec.HookSpecificOutput.HookEventName)
	assert.Equal(t, "deny", dec.HookSpecificOutput.PermissionDecision, "a target edit must be blocked")
	reason := dec.HookSpecificOutput.PermissionDecisionReason
	assert.Contains(t, reason, filepath.Join("locales", "fr", "app.json"), "the reason names the target")
	assert.NotContains(t, reason, root, "the target should render relative to the project root, not as an absolute path")
	assert.Contains(t, reason, filepath.Join("locales", "en", "app.json"), "the reason names the source to edit instead")
	assert.Contains(t, reason, "kapi merge", "the reason points at the round-trip")
	assert.Contains(t, reason, "[fr]", "the reason names the target locale")
}

// TestHookPreEdit_AllowsSourceFile asserts that editing the source file (not a
// generated target) is allowed — the hook emits nothing.
func TestHookPreEdit_AllowsSourceFile(t *testing.T) {
	root, _ := writeVerifyProject(t)
	t.Chdir(root)

	srcFile := filepath.Join(root, "locales", "en", "app.json")
	dec, raw := runHookPreEditCapture(t, preEditPayload(root, srcFile))

	assert.Empty(t, raw, "editing a source file must not be blocked")
	assert.Empty(t, dec.HookSpecificOutput.PermissionDecision)
}

// TestHookPreEdit_AllowsUnrelatedFile asserts that editing a file the project
// does not generate (e.g. application code) is allowed.
func TestHookPreEdit_AllowsUnrelatedFile(t *testing.T) {
	root, _ := writeVerifyProject(t)
	t.Chdir(root)

	other := filepath.Join(root, "src", "main.go")
	dec, raw := runHookPreEditCapture(t, preEditPayload(root, other))

	assert.Empty(t, raw, "an unrelated file must not be blocked")
	assert.Empty(t, dec.HookSpecificOutput.PermissionDecision)
}

// TestHookPreEdit_HonorsPayloadCWD asserts the hook resolves the project from
// the payload cwd even when the process starts elsewhere.
func TestHookPreEdit_HonorsPayloadCWD(t *testing.T) {
	root, targetFile := writeVerifyProject(t)
	// Start somewhere without a project; t.Chdir restores the original cwd at
	// cleanup, so the hook's os.Chdir into root does not leak between tests.
	t.Chdir(t.TempDir())

	dec, raw := runHookPreEditCapture(t, preEditPayload(root, targetFile))

	require.NotEmpty(t, raw, "the hook should resolve the project from the payload cwd")
	assert.Equal(t, "deny", dec.HookSpecificOutput.PermissionDecision)
}

// TestHookPreEdit_AllowsWhenNoProject asserts the hook fails open: outside any
// .kapi project there is nothing to guard, so the edit proceeds.
func TestHookPreEdit_AllowsWhenNoProject(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	dec, raw := runHookPreEditCapture(t, preEditPayload(dir, filepath.Join(dir, "anything.json")))

	assert.Empty(t, raw, "no project → no decision")
	assert.Empty(t, dec.HookSpecificOutput.PermissionDecision)
}

// TestHookPreEdit_AllowsOnEmptyStdin asserts an empty payload (e.g. run by
// hand) does not error or block.
func TestHookPreEdit_AllowsOnEmptyStdin(t *testing.T) {
	t.Chdir(t.TempDir())

	dec, raw := runHookPreEditCapture(t, "")

	assert.Empty(t, raw)
	assert.Empty(t, dec.HookSpecificOutput.PermissionDecision)
}
