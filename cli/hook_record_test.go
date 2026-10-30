package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/contextop"
	"github.com/neokapi/neokapi/host/output"
)

// runHookStopCmd runs `kapi hook stop` through a root command, the way the
// plugin's hook runs it, with the Stop payload on stdin.
func runHookStopCmd(t *testing.T, a *App, stdin string, args ...string) (string, string) {
	t.Helper()
	root := &cobra.Command{Use: "kapi"}
	AddCommandGroups(a, root)
	output.AddPersistentFlags(root.PersistentFlags())
	root.AddCommand(NewHookCmd(a))
	root.SetArgs(append([]string{"hook", "stop"}, args...))
	root.SilenceUsage, root.SilenceErrors = true, true
	var out, errOut bytes.Buffer
	root.SetIn(strings.NewReader(stdin))
	root.SetOut(&out)
	root.SetErr(&errOut)
	require.NoError(t, root.Execute())
	return out.String(), errOut.String()
}

// TestHookStop_RecordsTheRunAndPrintsAsBefore runs the Stop hook over a
// failing project. Claude Code reads the same decision it always did, the run
// is in the project's log, `--json` names the record, and `kapi context log`
// lists it.
func TestHookStop_RecordsTheRunAndPrintsAsBefore(t *testing.T) {
	t.Setenv("KAPI_DATA_DIR", t.TempDir())
	root, _ := writeVerifyProject(t)
	t.Chdir(root)
	a := &App{}
	payload := fmt.Sprintf(`{"hook_event_name":"Stop","session_id":"cli-s1","cwd":%q}`, root)

	stdout, stderr := runHookStopCmd(t, a, payload)
	assert.Empty(t, stderr)
	var raw map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &raw), stdout)
	assert.Equal(t, "block", raw["decision"])
	assert.ElementsMatch(t, []string{"decision", "reason"}, keysOf(raw), "Claude Code reads the decision and nothing else")

	asJSON, _ := runHookStopCmd(t, a, payload, "--json")
	var res StopHookResult
	require.NoError(t, json.Unmarshal([]byte(asJSON), &res), asJSON)
	assert.Equal(t, "block", res.Decision)
	require.NotEmpty(t, res.Record, "--json names the record")

	recipe := filepath.Join(root, "kapi.yaml")
	log := runContext(t, a, "log", "--session", "cli-s1", "-p", recipe)
	assert.Contains(t, log, "#"+contextop.ShortID(res.Record))
	assert.Contains(t, log, "agent claude-code/cli-s1  checked ")
	assert.Contains(t, log, " failing (")
}

func keysOf(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
