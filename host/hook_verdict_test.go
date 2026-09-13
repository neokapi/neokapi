package host

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// runStopHookIn runs the Stop hook for a session whose working directory is
// root, asserts it failed open with a notice, and returns the notice and stderr.
func runStopHookIn(t *testing.T, root string) (string, string) {
	t.Helper()
	// The fixtures isolate check execution, which opts out of project
	// discovery; the hook finds its project from the session directory.
	t.Setenv("KAPI_NO_PROJECT", "")
	t.Chdir(root)
	payload, err := json.Marshal(map[string]string{"hook_event_name": "Stop", "cwd": root})
	require.NoError(t, err)
	cmd := NewEnvCommand(t.Context(), "stop")
	var out, errOut bytes.Buffer
	cmd.SetIn(bytes.NewReader(payload))
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	require.NoError(t, (&App{}).RunHookStop(cmd))

	var raw map[string]any
	require.NoError(t, json.Unmarshal(out.Bytes(), &raw), "stdout: %s", out.String())
	assert.NotContains(t, raw, "decision", "a gate that did not run reached no verdict to block on")
	var notice HookNotice
	require.NoError(t, json.Unmarshal(out.Bytes(), &notice))
	require.NotEmpty(t, notice.SystemMessage)
	return notice.SystemMessage, errOut.String()
}

// TestHookStop_ABrokenCheckerNoticeDiffersFromNothingToCheck runs the Stop hook
// over a project with nothing to check and over one whose checker misses its
// canary. Both fail open. The broken checker's notice must say the result
// cannot be trusted, and must not read like an empty scope or a guard that
// never ran.
func TestHookStop_ABrokenCheckerNoticeDiffersFromNothingToCheck(t *testing.T) {
	t.Setenv("KAPI_NO_PROJECT", "")

	empty, emptyErr := runStopHookIn(t, writeEmptyShipProject(t))
	assert.Contains(t, empty, "nothing_to_check")
	assert.NotContains(t, empty, "cannot be trusted")
	assert.Contains(t, emptyErr, empty)

	root, _ := sourceShipFixture(t)
	real := hygieneTool
	hygieneTool = inertHygiene
	t.Cleanup(func() { hygieneTool = real })

	broken, brokenErr := runStopHookIn(t, root)
	// The analyzer's own reason also says its result cannot be trusted, so the
	// notice's framing is asserted as one phrase.
	assert.Contains(t, broken, "a checker is broken, so this result cannot be trusted")
	assert.Contains(t, broken, "checker_invalid")
	assert.Contains(t, broken, "hygiene")
	assert.NotContains(t, broken, "never ran")
	assert.NotContains(t, broken, "nothing in scope")
	assert.Contains(t, brokenErr, broken)

	assert.NotEqual(t, empty, broken)
}
