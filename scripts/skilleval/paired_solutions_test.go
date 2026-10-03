package main

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

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
			for i, step := range solution.Steps {
				if step.Interfere {
					record := newPairedInterferer(dir, *task.spec.Interference).apply()
					require.True(t, record.Applied, record.Error)
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
			result, err := validatePairedTask(dir, task, &PairedAgentResult{})
			require.NoError(t, err)
			assert.True(t, result.ObjectivePassed, "%+v", result.Criteria)
		})
	}
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
	assert.Contains(t, string(body), "from the dashboard.")
	code, _ = runPairedKapi(t, dir, alias, nil, "inspect", "-p", filepath.Join(dir, "kapi.yaml"), "docs/en/reports.md")
	assert.Equal(t, 2, code, "the alias refuses -p")
}
