package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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
func runHookStopCapture(t *testing.T, stdin string) (StopHookDecision, string) {
	t.Helper()
	dec, raw, stderr := runHookStopStdin(t, strings.NewReader(stdin))
	assert.Empty(t, stderr, "a hook that evaluated its guard must not warn")
	return dec, raw
}

// runHookStopStdin invokes the stop hook reading from r and returns the parsed
// decision, the raw stdout, and the raw stderr. Separate from
// runHookStopCapture so the un-evaluated-guard tests can assert on the warning
// channel, and so a test can hand the hook a real *os.File (the character-device
// no-payload case).
func runHookStopStdin(t *testing.T, r io.Reader) (StopHookDecision, string, string) {
	t.Helper()
	a := &App{}
	cmd := newHookStopCmd(a)
	cmd.SetIn(r)
	// Invoking RunHookStop directly bypasses cobra's Execute, which normally
	// seeds cmd.Context(); set it so the ctx-aware terms/content-memory lookups in the
	// verify gate get a real context instead of nil.
	cmd.SetContext(t.Context())
	var buf, errBuf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&errBuf)

	require.NoError(t, a.RunHookStop(cmd), "the stop hook must not error — its verdict is the JSON on stdout")

	raw := strings.TrimSpace(buf.String())
	var dec StopHookDecision
	if raw != "" {
		require.NoError(t, json.Unmarshal([]byte(raw), &dec), "hook output must be valid JSON: %s", raw)
	}
	return dec, raw, errBuf.String()
}

// hookNoticeOf decodes the systemMessage a hook emits when its guard could not
// be evaluated. Asserting on the parsed field rather than on the raw line keeps
// the wire shape — a decision-free payload, so the assistant's normal flow is
// untouched — part of the contract under test.
func hookNoticeOf(t *testing.T, raw string) string {
	t.Helper()
	var n HookNotice
	require.NoError(t, json.Unmarshal([]byte(raw), &n), "the notice must be valid JSON: %s", raw)
	return n.SystemMessage
}

// makeVerifyPass rewrites the project's source and target files so all gates
// pass: the competitor term is removed from the source and the target keeps the
// placeholder and uses the approved term.
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
	assert.Contains(t, dec.Reason, "kapi check --ship", "the reason should point at the gate")
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

// TestHookStop_NoPayloadOnCharDeviceStaysSilent is the discriminating control
// for the un-evaluated-guard notice: the documented no-payload case — the
// command run by hand with nothing piped in, so stdin is a character device —
// must stay completely silent. Without it, "warn whenever the payload is not a
// full struct" would pass the tests below while making every hand-run noisy.
func TestHookStop_NoPayloadOnCharDeviceStaysSilent(t *testing.T) {
	t.Chdir(t.TempDir())

	devnull, err := os.Open(os.DevNull)
	require.NoError(t, err)
	defer func() { _ = devnull.Close() }()

	dec, raw, stderr := runHookStopStdin(t, devnull)

	assert.Empty(t, raw, "no payload was offered — there is nothing to warn about")
	assert.Empty(t, stderr, "a hand-run hook must not warn")
	assert.Empty(t, dec.Decision)
}

// TestHookStop_WarnsOnEmptyPayload asserts that a pipe which delivered nothing
// is reported: the assistant invoked the hook, so the guard was expected to
// evaluate, and it did not.
func TestHookStop_WarnsOnEmptyPayload(t *testing.T) {
	root, _ := writeVerifyProject(t)
	t.Chdir(root)

	dec, raw, stderr := runHookStopStdin(t, strings.NewReader(""))

	assert.Empty(t, dec.Decision, "an unheard hook must fail open — never block")
	assert.Contains(t, hookNoticeOf(t, raw), "the payload on stdin was empty",
		"the notice must say why the guard did not run")
	assert.Contains(t, stderr, "kapi hook stop", "the warning must name the hook")
	assert.Contains(t, stderr, "the guard never ran",
		"the warning must distinguish an un-evaluated guard from one that passed")
}

// TestHookStop_WarnsOnMalformedPayload asserts a payload that is not the JSON
// the hook expects is reported rather than silently treated as a passing
// project. The project here has failing gates, so the discriminating fact is
// that the hook does NOT block (fail-open is preserved) while still saying so.
func TestHookStop_WarnsOnMalformedPayload(t *testing.T) {
	root, _ := writeVerifyProject(t)
	t.Chdir(root)

	dec, raw, stderr := runHookStopStdin(t, strings.NewReader(`{"cwd": [not json`))

	assert.Empty(t, dec.Decision, "a broken payload must never block the assistant")
	assert.Contains(t, hookNoticeOf(t, raw), "not the JSON this hook expects")
	assert.Contains(t, stderr, "kapi hook stop")
}

// TestHookStop_WarnsOnUnenterableCWD asserts the second, quieter bypass route is
// reported too: a cwd the hook cannot enter leaves the upward project walk
// starting from wherever the hook process happened to launch.
func TestHookStop_WarnsOnUnenterableCWD(t *testing.T) {
	root, _ := writeVerifyProject(t)
	t.Chdir(root)

	missing := filepath.Join(root, "no-such-dir")
	payload := fmt.Sprintf(`{"hook_event_name":"Stop","cwd":%q}`, missing)
	dec, raw, stderr := runHookStopStdin(t, strings.NewReader(payload))

	assert.Empty(t, dec.Decision, "an unenterable cwd must not block the assistant")
	assert.Contains(t, hookNoticeOf(t, raw), "could not be entered")
	assert.Contains(t, stderr, missing, "the warning must name the directory")
}

// failingReader fails the read after handing back prefix, standing in for a
// hook pipe that breaks mid-payload. #1480's sites conflated this with an empty
// payload; the two now read differently, so the message names the real cause.
type failingReader struct {
	prefix string
	n      int
}

func (r *failingReader) Read(p []byte) (int, error) {
	if r.n < len(r.prefix) {
		n := copy(p, r.prefix[r.n:])
		r.n += n
		return n, nil
	}
	return 0, errors.New("hook pipe reset by peer")
}

// TestHookStop_WarnsOnUnreadableStdin asserts a read failure is reported as a
// read failure, never conflated with an empty payload.
func TestHookStop_WarnsOnUnreadableStdin(t *testing.T) {
	root, _ := writeVerifyProject(t)
	t.Chdir(root)

	dec, raw, stderr := runHookStopStdin(t, &failingReader{prefix: `{"cwd":"`})

	assert.Empty(t, dec.Decision, "an unreadable payload must never block")
	msg := hookNoticeOf(t, raw)
	assert.Contains(t, msg, "stdin could not be read", "a read failure must not read as an empty payload")
	assert.Contains(t, msg, "hook pipe reset by peer", "the underlying cause must survive")
	assert.Contains(t, stderr, "kapi hook stop")
}

// TestHookStop_WarnsOnUnloadableProject asserts a project that exists but cannot
// be evaluated is reported — the gate is installed but inert, which must not read
// as a project with no gates to run.
func TestHookStop_WarnsOnUnloadableProject(t *testing.T) {
	root, _ := writeVerifyProject(t)
	t.Chdir(root)
	require.NoError(t, os.WriteFile(filepath.Join(root, "kapi.yaml"), []byte("defaults: [this is not a mapping]\n"), 0o644))

	dec, raw, stderr := runHookStopStdin(t, strings.NewReader(`{"hook_event_name":"Stop"}`))

	assert.Empty(t, dec.Decision, "an unevaluable project must not trap the assistant")
	assert.Contains(t, hookNoticeOf(t, raw), "could not be evaluated")
	assert.Contains(t, stderr, "kapi hook stop")
}

// TestHookPreEdit_WarnsOnUnreadableStdin is the pre-edit half of the read-error
// vs empty-payload distinction.
func TestHookPreEdit_WarnsOnUnreadableStdin(t *testing.T) {
	root, _ := writeVerifyProject(t)
	t.Chdir(root)

	dec, raw, stderr := runHookPreEditStdin(t, &failingReader{prefix: `{"tool_name":"Edit"`})

	assert.Empty(t, dec.HookSpecificOutput.PermissionDecision, "an unreadable payload must never deny")
	msg := hookNoticeOf(t, raw)
	assert.Contains(t, msg, "stdin could not be read")
	assert.Contains(t, msg, "hook pipe reset by peer")
	assert.Contains(t, stderr, "kapi hook pre-edit")
}

// TestHookStop_NoProjectStaysSilent asserts the other discriminating control:
// "there is no kapi project here" is not an un-evaluated guard, it is nothing to
// gate. It must stay silent on both channels, or every session outside a project
// would carry a warning.
func TestHookStop_NoProjectStaysSilent(t *testing.T) {
	t.Chdir(t.TempDir())

	dec, raw, stderr := runHookStopStdin(t, strings.NewReader(`{"hook_event_name":"Stop"}`))

	assert.Empty(t, raw, "no project → no decision and no notice")
	assert.Empty(t, stderr, "no project is not a failure to evaluate")
	assert.Empty(t, dec.Decision)
}

// runHookPreEditCapture invokes the pre-edit hook with the given stdin payload
// and returns the parsed decision plus the raw stdout (empty when the hook
// allows the edit to proceed).
func runHookPreEditCapture(t *testing.T, stdin string) (PreToolUseDecision, string) {
	t.Helper()
	dec, raw, stderr := runHookPreEditStdin(t, strings.NewReader(stdin))
	assert.Empty(t, stderr, "a hook that evaluated its guard must not warn")
	return dec, raw
}

// runHookPreEditStdin invokes the pre-edit hook reading from r and returns the
// parsed decision, the raw stdout, and the raw stderr.
func runHookPreEditStdin(t *testing.T, r io.Reader) (PreToolUseDecision, string, string) {
	t.Helper()
	a := &App{}
	cmd := newHookPreEditCmd(a)
	cmd.SetIn(r)
	// See runHookStopCapture: a direct call bypasses cobra's Execute, so seed
	// the context the ctx-aware project lookups expect.
	cmd.SetContext(t.Context())
	var buf, errBuf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&errBuf)

	require.NoError(t, a.RunHookPreEdit(cmd), "the pre-edit hook must not error — its verdict is the JSON on stdout")

	raw := strings.TrimSpace(buf.String())
	var dec PreToolUseDecision
	if raw != "" {
		require.NoError(t, json.Unmarshal([]byte(raw), &dec), "hook output must be valid JSON: %s", raw)
	}
	return dec, raw, errBuf.String()
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

// TestHookPreEdit_NoPayloadOnCharDeviceStaysSilent is the discriminating control
// for the notice: run by hand with nothing piped in (stdin is a character
// device), the hook must stay silent on both channels.
func TestHookPreEdit_NoPayloadOnCharDeviceStaysSilent(t *testing.T) {
	t.Chdir(t.TempDir())

	devnull, err := os.Open(os.DevNull)
	require.NoError(t, err)
	defer func() { _ = devnull.Close() }()

	dec, raw, stderr := runHookPreEditStdin(t, devnull)

	assert.Empty(t, raw, "no payload was offered — there is nothing to warn about")
	assert.Empty(t, stderr, "a hand-run hook must not warn")
	assert.Empty(t, dec.HookSpecificOutput.PermissionDecision)
}

// TestHookPreEdit_WarnsOnMalformedPayload is the regression for the bug: a
// payload the hook cannot parse used to zero ToolInput.FilePath, which the guard
// read as "nothing to guard" and allowed the edit with no signal anywhere. The
// edit must still proceed — and now the bypass is on record.
func TestHookPreEdit_WarnsOnMalformedPayload(t *testing.T) {
	root, targetFile := writeVerifyProject(t)
	t.Chdir(root)

	// Same payload as the deny case, truncated mid-object: exactly the shape a
	// partial write down the hook pipe produces.
	partial := preEditPayload(root, targetFile)
	dec, raw, stderr := runHookPreEditStdin(t, strings.NewReader(partial[:len(partial)-12]))

	assert.Empty(t, dec.HookSpecificOutput.PermissionDecision,
		"a broken payload must never deny — the guard fails open")
	assert.Contains(t, hookNoticeOf(t, raw), "not the JSON this hook expects")
	assert.Contains(t, stderr, "kapi hook pre-edit", "the warning must name the hook")
	assert.Contains(t, stderr, "the guard never ran",
		"the warning must distinguish an un-evaluated guard from one that passed")
}

// TestHookPreEdit_WarnsOnEmptyPayload asserts a pipe that delivered nothing is
// reported rather than read as "no file to guard".
func TestHookPreEdit_WarnsOnEmptyPayload(t *testing.T) {
	root, _ := writeVerifyProject(t)
	t.Chdir(root)

	dec, raw, stderr := runHookPreEditStdin(t, strings.NewReader(""))

	assert.Empty(t, dec.HookSpecificOutput.PermissionDecision)
	assert.Contains(t, hookNoticeOf(t, raw), "the payload on stdin was empty")
	assert.Contains(t, stderr, "kapi hook pre-edit")
}

// TestHookPreEdit_WarnsOnUnenterableCWD covers the second bypass route #1480
// names: a cwd the hook cannot enter leaves the upward walk running from
// wherever the hook process launched, so the guard matches nothing.
func TestHookPreEdit_WarnsOnUnenterableCWD(t *testing.T) {
	root, targetFile := writeVerifyProject(t)
	t.Chdir(t.TempDir())

	missing := filepath.Join(root, "no-such-dir")
	dec, raw, stderr := runHookPreEditStdin(t, strings.NewReader(preEditPayload(missing, targetFile)))

	assert.Empty(t, dec.HookSpecificOutput.PermissionDecision, "an unenterable cwd must not deny")
	assert.Contains(t, hookNoticeOf(t, raw), "could not be entered")
	assert.Contains(t, stderr, missing, "the warning must name the directory")
}

// TestHookPreEdit_WarnsOnUnloadableProject asserts the installed-but-inert case
// is reported: a recipe the hook cannot load leaves the guard unable to know
// which files are generated, so every target edit is allowed.
func TestHookPreEdit_WarnsOnUnloadableProject(t *testing.T) {
	root, targetFile := writeVerifyProject(t)
	t.Chdir(root)
	// Corrupt the recipe so project.LoadWithOptions fails. The payload is the
	// same one that denies against a loadable recipe, so the only variable is
	// whether the guard could evaluate.
	require.NoError(t, os.WriteFile(filepath.Join(root, "kapi.yaml"), []byte("defaults: [this is not a mapping]\n"), 0o644))

	dec, raw, stderr := runHookPreEditStdin(t, strings.NewReader(preEditPayload(root, targetFile)))

	assert.Empty(t, dec.HookSpecificOutput.PermissionDecision, "an unloadable project must not deny")
	assert.Contains(t, hookNoticeOf(t, raw), "could not be loaded")
	assert.Contains(t, stderr, "kapi hook pre-edit")
}

// TestHookPreEdit_NoProjectStaysSilent is the other discriminating control:
// outside a project there is nothing to guard, which is not a failure to
// evaluate and must stay silent on both channels.
func TestHookPreEdit_NoProjectStaysSilent(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	dec, raw, stderr := runHookPreEditStdin(t, strings.NewReader(preEditPayload(dir, filepath.Join(dir, "anything.json"))))

	assert.Empty(t, raw, "no project → no decision and no notice")
	assert.Empty(t, stderr, "no project is not a failure to evaluate")
	assert.Empty(t, dec.HookSpecificOutput.PermissionDecision)
}

// TestHookPreEdit_AllowsPayloadWithoutFilePath asserts a well-formed payload
// carrying no file path stays silent: the matcher delivered a tool call with
// nothing to guard, which is not an un-evaluated guard.
func TestHookPreEdit_AllowsPayloadWithoutFilePath(t *testing.T) {
	root, _ := writeVerifyProject(t)
	t.Chdir(root)

	dec, raw, stderr := runHookPreEditStdin(t, strings.NewReader(`{"hook_event_name":"PreToolUse","tool_name":"Bash"}`))

	assert.Empty(t, raw, "no file in the payload → nothing to guard")
	assert.Empty(t, stderr)
	assert.Empty(t, dec.HookSpecificOutput.PermissionDecision)
}
