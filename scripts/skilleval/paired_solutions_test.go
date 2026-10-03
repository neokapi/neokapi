package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// pairedSolution is a task's reference route through kapi.change/v1: the
// change sets an agent in the kapi arms would send, in order, with the exit
// code and refusal each draws. A step may instead make the other editor's
// change.
type pairedSolution struct {
	Steps []struct {
		Note      string          `json:"note"`
		Interfere bool            `json:"interfere,omitempty"`
		Exit      int             `json:"exit"`
		Code      string          `json:"code,omitempty"`
		Changeset json.RawMessage `json:"changeset,omitempty"`
	} `json:"steps"`
}

func pairedBuiltKapi(t *testing.T) string {
	t.Helper()
	root, err := repoRoot()
	if err != nil {
		t.Skipf("not in a git checkout: %v", err)
	}
	binary := findKapi(root)
	if binary == "" {
		t.Skip("bin/kapi is not built: run make build to send each task's reference change sets through this tree's kapi")
	}
	return binary
}

func runPairedKapi(t *testing.T, dir, binary string, extra []string, args ...string) (int, []byte) {
	t.Helper()
	command := exec.Command(binary, args...)
	command.Dir = dir
	command.Env = append([]string{"PATH=" + os.Getenv("PATH"), "HOME=" + dir}, isolationEnv(dir)...)
	command.Env = append(command.Env, extra...)
	output, err := command.Output()
	if exit, ok := errors.AsType[*exec.ExitError](err); ok {
		return exit.ExitCode(), output
	}
	require.NoError(t, err)
	return 0, output
}

// Every task can be done through the change contract in its project, with
// the refusals the recovery tasks are built around, and the result passes the
// task's own grader. This is what makes a kapi arm's failure a finding about
// the agent or the surface rather than about the task.
func TestPairedSolutionsThroughKapi(t *testing.T) {
	binary := pairedBuiltKapi(t)
	for _, task := range pairedTasks() {
		t.Run(task.ID, func(t *testing.T) {
			data, err := pairedFixtures.ReadFile("testdata/paired/solutions/" + task.ID + ".json")
			require.NoError(t, err)
			var solution pairedSolution
			require.NoError(t, json.Unmarshal(data, &solution))
			dir := t.TempDir()
			require.NoError(t, materializePairedTask(dir, task))
			require.NoError(t, readPairedContext(t.Context(), dir, binary))
			observed := &PairedAgentResult{}
			for i, step := range solution.Steps {
				if step.Interfere {
					record := newPairedInterferer(dir, *task.spec.Interference).apply()
					require.True(t, record.Applied, record.Error)
					observed.Interference = &record
					continue
				}
				changes := filepath.Join(t.TempDir(), "changes.json")
				require.NoError(t, os.WriteFile(changes, step.Changeset, 0o600))
				code, output := runPairedKapi(t, dir, binary, []string{"KAPI_ACTOR=agent"},
					"apply", "-p", filepath.Join(dir, "kapi.yaml"), "--json", changes)
				require.Equal(t, step.Exit, code, "step %d (%s): %s", i+1, step.Note, output)
				if step.Code != "" {
					assert.Contains(t, string(output), `"code": "`+step.Code+`"`, "step %d", i+1)
				}
			}
			result, err := validatePairedTask(dir, task, observed)
			require.NoError(t, err)
			assert.True(t, result.ObjectivePassed, "%+v", result.Criteria)
		})
	}
}

// The plural task's reference change set, a replace_text naming the one branch
// by its path, lands through each arm's surface: kapi apply in the project
// (skill-cli), apply_edits on the kapi MCP server bound to the project (mcp),
// and kapi-files apply with no project (project-free). Each result passes the
// task's own graders, so an arm's failure on the task is a finding about the
// agent rather than about whether its surface reaches a branch.
func TestPairedPluralRouteOnEverySurface(t *testing.T) {
	binary := pairedBuiltKapi(t)
	task := pairedTaskByID(t, "edit-plural-branch")
	data, err := pairedFixtures.ReadFile("testdata/paired/solutions/" + task.ID + ".json")
	require.NoError(t, err)
	var solution pairedSolution
	require.NoError(t, json.Unmarshal(data, &solution))
	require.Len(t, solution.Steps, 1)
	agent := []string{"KAPI_ACTOR=agent"}
	// The reference route, and set_content on the branch with the argument
	// typed as the prompt spells it.
	var reference struct {
		Ops []struct {
			IfMatch string `json:"if_match"`
		} `json:"ops"`
	}
	require.NoError(t, json.Unmarshal(solution.Steps[0].Changeset, &reference))
	require.Len(t, reference.Ops, 1)
	typed, err := json.Marshal(map[string]any{"ops": []any{map[string]any{
		"op": "set_content", "at": map[string]any{"doc": "lib/l10n/app_en.arb", "block": "inboxCount"},
		"if_match": reference.Ops[0].IfMatch, "path": []any{0, map[string]any{"plural": "one"}},
		"text": "{count} unread message",
	}}})
	require.NoError(t, err)
	// replace_text whose find names the argument as a read shows it, by its
	// token, and as the prompt spells it: a find reads as text does.
	findWith := func(find, text string) json.RawMessage {
		b, err := json.Marshal(map[string]any{"ops": []any{map[string]any{
			"op": "replace_text", "at": map[string]any{"doc": "lib/l10n/app_en.arb", "block": "inboxCount"},
			"if_match": reference.Ops[0].IfMatch,
			"edits":    []any{map[string]any{"path": []any{0, map[string]any{"plural": "one"}}, "find": find, "text": text}},
		}}})
		require.NoError(t, err)
		return b
	}
	// replace_text with the branch on the operation, where set_content takes
	// it too.
	onOperation, err := json.Marshal(map[string]any{"ops": []any{map[string]any{
		"op": "replace_text", "at": map[string]any{"doc": "lib/l10n/app_en.arb", "block": "inboxCount"},
		"if_match": reference.Ops[0].IfMatch, "path": []any{0, map[string]any{"plural": "one"}},
		"edits": []any{map[string]any{"find": "new message", "text": "unread message"}},
	}}})
	require.NoError(t, err)
	routes := map[string]json.RawMessage{
		"replace_text": solution.Steps[0].Changeset, "set_content typed": typed,
		"replace_text path on the operation": onOperation,
		"replace_text token":                 findWith(`<x id="p1/"/> new message`, `<x id="p1/"/> unread message`),
		"replace_text argument":              findWith("{count} new message", "{count} unread message"),
	}

	surfaces := []struct {
		name  string
		apply func(t *testing.T, dir, changes string, changeset json.RawMessage)
	}{
		{"cli", func(t *testing.T, dir, changes string, _ json.RawMessage) {
			code, output := runPairedKapi(t, dir, binary, agent, "apply", "-p", filepath.Join(dir, "kapi.yaml"), "--json", changes)
			require.Equal(t, 0, code, string(output))
		}},
		{"mcp", func(t *testing.T, dir, _ string, changeset json.RawMessage) {
			result := applyThroughPairedMCP(t, dir, binary, changeset)
			assert.NotEqual(t, true, result["isError"], "%v", result)
			structured, _ := result["structuredContent"].(map[string]any)
			assert.Equal(t, "applied", structured["status"], "%v", result)
		}},
		{"project-free", func(t *testing.T, dir, changes string, _ json.RawMessage) {
			alias := filepath.Join(t.TempDir(), pairedFilesAlias)
			require.NoError(t, os.Symlink(binary, alias))
			code, output := runPairedKapi(t, dir, alias, agent, "apply", "--json", changes)
			require.Equal(t, 0, code, string(output))
		}},
	}
	for route, changeset := range routes {
		for _, surface := range surfaces {
			t.Run(route+"/"+surface.name, func(t *testing.T) {
				dir := t.TempDir()
				require.NoError(t, materializePairedTask(dir, task))
				require.NoError(t, readPairedContext(t.Context(), dir, binary))
				changes := filepath.Join(t.TempDir(), "changes.json")
				require.NoError(t, os.WriteFile(changes, changeset, 0o600))
				surface.apply(t, dir, changes, changeset)
				result, err := validatePairedTask(dir, task, &PairedAgentResult{})
				require.NoError(t, err)
				assert.True(t, result.ObjectivePassed, "%+v", result.Criteria)
			})
		}
	}
}

// applyThroughPairedMCP sends a change set to apply_edits on the kapi MCP
// server bound to the project in dir, as the mcp arm's host does, and returns
// the tool result.
func applyThroughPairedMCP(t *testing.T, dir, binary string, changeset json.RawMessage) map[string]any {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, binary, "-p", filepath.Join(dir, "kapi.yaml"), "mcp")
	command.Dir = dir
	command.Env = append([]string{"PATH=" + os.Getenv("PATH"), "HOME=" + dir}, isolationEnv(dir)...)
	command.Stderr = io.Discard
	input, err := command.StdinPipe()
	require.NoError(t, err)
	output, err := command.StdoutPipe()
	require.NoError(t, err)
	require.NoError(t, command.Start())
	defer func() {
		_ = input.Close()
		_ = command.Process.Kill()
		_ = command.Wait()
	}()
	client := newPairedMCPDiscoveryClient(input, output)
	require.NoError(t, client.initialize(&PairedMCPReadiness{}))
	var args map[string]any
	require.NoError(t, json.Unmarshal(changeset, &args))
	result, err := client.call("tools/call", map[string]any{"name": "apply_edits", "arguments": args})
	require.NoError(t, err)
	return result
}

// The project-free alias applies the same change set with no project, so no
// check stands in front of the edit: the forbidden term lands.
func TestPairedProjectFreeAliasHasNoGate(t *testing.T) {
	binary := pairedBuiltKapi(t)
	task := pairedTaskByID(t, "recover-gate-refusal")
	data, err := pairedFixtures.ReadFile("testdata/paired/solutions/" + task.ID + ".json")
	require.NoError(t, err)
	var solution pairedSolution
	require.NoError(t, json.Unmarshal(data, &solution))
	dir := t.TempDir()
	require.NoError(t, materializePairedTask(dir, task))
	require.NoError(t, readPairedContext(t.Context(), dir, binary))
	bin := t.TempDir()
	alias := filepath.Join(bin, pairedFilesAlias)
	require.NoError(t, os.Symlink(binary, alias))
	changes := filepath.Join(t.TempDir(), "changes.json")
	require.NoError(t, os.WriteFile(changes, solution.Steps[0].Changeset, 0o600))
	code, output := runPairedKapi(t, dir, alias, []string{"KAPI_ACTOR=agent"}, "apply", "--json", changes)
	require.Equal(t, 0, code, string(output))
	body, err := os.ReadFile(filepath.Join(dir, "docs", "en", "reports.md"))
	require.NoError(t, err)
	assert.Contains(t, string(body), "from the portal.")
	code, _ = runPairedKapi(t, dir, alias, nil, "inspect", "-p", filepath.Join(dir, "kapi.yaml"), "docs/en/reports.md")
	assert.Equal(t, 2, code, "the alias refuses -p")
}
