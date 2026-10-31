package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/contextop"
	"github.com/neokapi/neokapi/host"
	"github.com/neokapi/neokapi/host/output"
)

// runHookPostEditCmd runs `kapi hook post-edit` through a root command, the
// way the plugin's hook runs it, with the PostToolUse payload on stdin.
func runHookPostEditCmd(t *testing.T, a *App, stdin string, args ...string) (string, string) {
	t.Helper()
	root := &cobra.Command{Use: "kapi"}
	AddCommandGroups(a, root)
	output.AddPersistentFlags(root.PersistentFlags())
	root.AddCommand(NewHookCmd(a))
	root.SetArgs(append([]string{"hook", "post-edit"}, args...))
	root.SilenceUsage, root.SilenceErrors = true, true
	var out, errOut bytes.Buffer
	root.SetIn(strings.NewReader(stdin))
	root.SetOut(&out)
	root.SetErr(&errOut)
	require.NoError(t, root.Execute(), "the edit hook's verdict is the JSON on stdout, never the exit code")
	return out.String(), errOut.String()
}

// postEditPayload is the PostToolUse event Claude Code sends after a Write.
func postEditPayload(cwd, file, toolUseID string) string {
	return fmt.Sprintf(`{"hook_event_name":"PostToolUse","session_id":"edit-s1","cwd":%q,`+
		`"tool_name":"Write","tool_use_id":%q,"tool_input":{"file_path":%q,"content":""},`+
		`"tool_response":{"filePath":%q,"type":"update"}}`, cwd, toolUseID, file, file)
}

// TestHookPostEdit_BlocksAFailingFileAndCountsTheFixAsCaught: an edit that
// leaves a failing finding in a source file blocks with the finding and the
// command to run; the clean edit that follows prints nothing, and the session
// counts the finding as caught.
func TestHookPostEdit_BlocksAFailingFileAndCountsTheFixAsCaught(t *testing.T) {
	t.Setenv("KAPI_DATA_DIR", t.TempDir())
	root, _ := writeVerifyProject(t)
	t.Chdir(t.TempDir())
	a := &App{}
	source := filepath.Join(root, "locales", "en", "app.json")

	stdout, stderr := runHookPostEditCmd(t, a, postEditPayload(root, source, "toolu_1"))
	assert.Empty(t, stderr)
	var raw map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &raw), stdout)
	assert.ElementsMatch(t, []string{"decision", "reason"}, keysOf(raw), "Claude Code reads the decision and the reason")
	assert.Equal(t, "block", raw["decision"])
	reason, _ := raw["reason"].(string)
	assert.Contains(t, reason, "in locales/en/app.json")
	assert.Contains(t, reason, "FAILS [")
	assert.Contains(t, reason, "Globex")
	assert.Contains(t, reason, "Run `kapi check locales/en/app.json`")

	clean := "{\n  \"greeting\": \"Hello {name}, welcome!\",\n  \"save\": \"Save\"\n}\n"
	require.NoError(t, os.WriteFile(source, []byte(clean), 0o644))
	asJSON, stderr := runHookPostEditCmd(t, a, postEditPayload(root, source, "toolu_2"), "--json")
	assert.Empty(t, stderr)
	var res host.PostEditHookResult
	require.NoError(t, json.Unmarshal([]byte(asJSON), &res), asJSON)
	assert.Empty(t, res.Decision, "a clean file carries no decision")
	require.NotEmpty(t, res.Record, "--json names the record")

	stdout, _ = runHookPostEditCmd(t, a, postEditPayload(root, source, "toolu_3"))
	assert.Empty(t, stdout, "a clean file prints nothing")

	outcomes, err := a.SessionOutcomes(t.Context(), host.ContextSessionRequest{Project: filepath.Join(root, "kapi.yaml"), Session: "edit-s1"})
	require.NoError(t, err)
	assert.Equal(t, 3, outcomes.Checked)
	assert.Equal(t, 1, outcomes.Caught, "the finding the first run reported and the second no longer found")
	assert.Equal(t, 0, outcomes.Failing)
	assert.Equal(t, 1, outcomes.Files)

	recipe := filepath.Join(root, "kapi.yaml")
	log := runContext(t, a, "log", "--session", "edit-s1", "-p", recipe)
	assert.Contains(t, log, "#"+contextop.ShortID(res.Record))
}

// TestHookPostEdit_LeavesOtherFilesAlone: a generated target, a file the
// project does not read, a file outside the project and an edit outside any
// project print nothing and record no run.
func TestHookPostEdit_LeavesOtherFilesAlone(t *testing.T) {
	t.Setenv("KAPI_DATA_DIR", t.TempDir())
	root, target := writeVerifyProject(t)
	t.Chdir(t.TempDir())
	a := &App{}

	unread := filepath.Join(root, "notes.md")
	require.NoError(t, os.WriteFile(unread, []byte("Welcome to Globex.\n"), 0o644))
	elsewhere := t.TempDir()
	outside := filepath.Join(elsewhere, "app.json")
	require.NoError(t, os.WriteFile(outside, []byte(`{"a": "Globex"}`), 0o644))

	for name, payload := range map[string]string{
		"generated target":    postEditPayload(root, target, "toolu_t"),
		"unread file":         postEditPayload(root, unread, "toolu_u"),
		"outside the project": postEditPayload(root, outside, "toolu_o"),
		"no project":          postEditPayload(elsewhere, outside, "toolu_n"),
		"no file path":        fmt.Sprintf(`{"hook_event_name":"PostToolUse","cwd":%q,"tool_input":{}}`, root),
	} {
		stdout, stderr := runHookPostEditCmd(t, a, payload)
		assert.Empty(t, stdout, name)
		assert.Empty(t, stderr, name)
	}
	outcomes, err := a.SessionOutcomes(t.Context(), host.ContextSessionRequest{Project: filepath.Join(root, "kapi.yaml"), Session: "edit-s1"})
	require.NoError(t, err)
	assert.Zero(t, outcomes.Checked, "no run is recorded for a file the hook leaves alone")
}

// TestHookPostEdit_WarnsWhenTheCheckCannotRun: a malformed payload leaves the
// edit standing and says so on both channels, naming the hook.
func TestHookPostEdit_WarnsWhenTheCheckCannotRun(t *testing.T) {
	a := &App{}
	stdout, stderr := runHookPostEditCmd(t, a, "{not json")
	var notice host.HookNotice
	require.NoError(t, json.Unmarshal([]byte(stdout), &notice), stdout)
	assert.Contains(t, notice.SystemMessage, "kapi hook post-edit: the guard did not run")
	assert.Contains(t, stderr, "kapi hook post-edit")
}

// TestHookPostEdit_NamesTheFileAsTheProjectDoes: from a subdirectory of the
// project, the reason names the file by its project-relative path, and the
// command to run by the path the agent types from where it is.
func TestHookPostEdit_NamesTheFileAsTheProjectDoes(t *testing.T) {
	t.Setenv("KAPI_DATA_DIR", t.TempDir())
	root, _ := writeVerifyProject(t)
	t.Chdir(t.TempDir())
	a := &App{}
	source := filepath.Join(root, "locales", "en", "app.json")

	stdout, _ := runHookPostEditCmd(t, a, postEditPayload(filepath.Join(root, "locales"), source, "toolu_sub"))
	var dec host.PostEditHookDecision
	require.NoError(t, json.Unmarshal([]byte(stdout), &dec), stdout)
	assert.Contains(t, dec.Reason, "in locales/en/app.json.")
	assert.Contains(t, dec.Reason, "] locales/en/app.json")
	assert.Contains(t, dec.Reason, "Run `kapi check en/app.json`")
}

// TestHookPostEdit_AMissingReaderPrintsNothing: a declared file whose
// comments no installed reader opens prints nothing, because the reader is a
// property of this machine, and the run is on record as one that did not run.
func TestHookPostEdit_AMissingReaderPrintsNothing(t *testing.T) {
	t.Setenv("KAPI_DATA_DIR", t.TempDir())
	t.Setenv("KAPI_NO_PROJECT", "")
	t.Setenv("KAPI_PLUGINS_DIR_ONLY", "1")
	t.Setenv("KAPI_PLUGINS_DIR", "")
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "src"), 0o755))
	recipe := "version: v1\nname: comments\ndefaults:\n  source_language: en\ncollections:\n" +
		"  - name: code\n    source_only: true\n    content:\n      - path: \"src/**/*.tsx\"\n        comments:\n          only: true\n"
	require.NoError(t, os.WriteFile(filepath.Join(root, "kapi.yaml"), []byte(recipe), 0o644))
	file := filepath.Join(root, "src", "App.tsx")
	require.NoError(t, os.WriteFile(file, []byte("// Log in to start.\nexport const App = () => null;\n"), 0o644))
	t.Chdir(t.TempDir())
	a := &App{}

	stdout, stderr := runHookPostEditCmd(t, a, postEditPayload(root, file, "toolu_tsx"))
	assert.Empty(t, stdout)
	assert.Empty(t, stderr)

	runs := runContext(t, a, "log", "--session", "edit-s1", "-p", filepath.Join(root, "kapi.yaml"))
	assert.Contains(t, runs, "did not run")
	assert.Contains(t, runs, "src/App.tsx")
}
