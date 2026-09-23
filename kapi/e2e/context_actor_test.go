//go:build e2e

package e2e

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Who the command line records a context operation as, against the built
// binary (issue #2912).
//
// The unit tests drive the resolution directly. This drives the released
// shape: separate kapi processes, an environment that carries what an agent
// host exports, and the log read back afterwards.

// kapiEnv runs kapi with extra environment on top of the isolation set, and
// returns stdout and stderr together with the error.
func kapiEnv(t *testing.T, extra []string, args ...string) (string, error) {
	t.Helper()
	cmd := exec.Command(kapiBin, args...)
	cmd.Env = append(append(os.Environ(), isoEnv...), extra...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// kapiEnvOK runs kapi with extra environment and fails the test on a non-zero
// exit.
func kapiEnvOK(t *testing.T, extra []string, args ...string) string {
	t.Helper()
	out, err := kapiEnv(t, extra, args...)
	require.NoError(t, err, "kapi %s failed:\n%s", strings.Join(args, " "), out)
	return out
}

// contextActorProject writes a project with one file worth recording facts
// about.
func contextActorProject(t *testing.T, name string) string {
	t.Helper()
	root := t.TempDir()
	write := func(rel, body string) {
		t.Helper()
		file := filepath.Join(root, filepath.FromSlash(rel))
		require.NoError(t, os.MkdirAll(filepath.Dir(file), 0o755))
		require.NoError(t, os.WriteFile(file, []byte(body), 0o644))
	}
	write("kapi.yaml", "version: v1\nname: "+name+`
defaults:
  source_language: en
collections:
  - name: Docs
    source_only: true
    content:
      - path: "docs/**/*.md"
`)
	write("docs/guide.md", "# Guide\n\nWe utilise the widget every day.\n")
	return root
}

// loggedOperation is the part of `kapi context log --json` these tests read.
type loggedOperation struct {
	ID    string `json:"id"`
	Kind  string `json:"kind"`
	Actor struct {
		Kind    string `json:"kind"`
		Name    string `json:"name"`
		Session string `json:"session"`
	} `json:"actor"`
	Subject struct {
		Kind string `json:"kind"`
	} `json:"subject"`
	Status string `json:"status"`
	Note   string `json:"note"`
}

// readLog runs `kapi context log --json` and decodes it.
func readLog(t *testing.T, extra []string, args ...string) []loggedOperation {
	t.Helper()
	out := kapiEnvOK(t, extra, args...)
	var body struct {
		Operations []loggedOperation `json:"operations"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &body), "read the log as JSON:\n%s", out)
	return body.Operations
}

// TestCommandLineAgentIsRecordedAndRefusedTheDecision is the reproduction in
// #2912, end to end: two kapi processes under one agent run and one under a
// person's shell, then the log.
func TestCommandLineAgentIsRecordedAndRefusedTheDecision(t *testing.T) {
	root := contextActorProject(t, "ctx-actor-cli")
	recipe := filepath.Join(root, "kapi.yaml")
	agent := []string{"KAPI_ACTOR=agent", "KAPI_AGENT_NAME=codex", "KAPI_AGENT_SESSION=s-e2e-one"}

	kapiEnvOK(t, agent, "context", "observe", "the docs address the reader as you",
		"--seen-in", "docs/guide.md", "-p", recipe)
	kapiEnvOK(t, agent, "context", "observe", "--term", "use", "--instead-of", "utilise",
		"--seen-in", "docs/guide.md", "--quote", "We utilise the widget", "-p", recipe)
	kapi(t, "context", "observe", "the guide is written in the present tense", "-p", recipe)

	all := readLog(t, nil, "context", "log", "--json", "-p", recipe)
	require.Len(t, all, 3)

	byAgent := readLog(t, nil, "context", "log", "--actor", "agent", "--json", "-p", recipe)
	require.Len(t, byAgent, 2, "the agent's entries answer `--actor agent`")
	for _, op := range byAgent {
		assert.Equal(t, "agent", op.Actor.Kind)
		assert.Equal(t, "codex", op.Actor.Name)
		assert.Equal(t, "s-e2e-one", op.Actor.Session)
	}

	byPerson := readLog(t, nil, "context", "log", "--actor", "person", "--json", "-p", recipe)
	require.Len(t, byPerson, 1, "a shell with no agent in it is still a person")
	assert.Empty(t, byPerson[0].Actor.Session)

	bySession := readLog(t, nil, "context", "log", "--session", "s-e2e-one", "--json", "-p", recipe)
	assert.Len(t, bySession, 2, "`revert --session` has something to name")

	mine := readLog(t, agent, "context", "log", "--session", "this", "--json", "-p", recipe)
	assert.Len(t, mine, 2, "the run reads its own work back without being told the id")

	// The decision is the person's, and the refusal says what to do instead.
	var candidate string
	for _, op := range byAgent {
		if op.Kind == "observe" && op.Subject.Kind == "term" {
			candidate = op.ID
		}
	}
	require.NotEmpty(t, candidate)

	refused, err := kapiEnv(t, agent, "context", "keep", candidate, "-p", recipe)
	require.Error(t, err, "an agent keeping its own suggestion:\n%s", refused)
	assert.Contains(t, refused, "only a person keeps a suggestion")
	assert.Contains(t, refused, "kapi context log --status suggested")

	kapi(t, "context", "keep", candidate, "-p", recipe)
	after := readLog(t, nil, "context", "log", "--session", "s-e2e-one", "--subjects", "--json", "-p", recipe)
	require.Len(t, after, 2)
	var confirmed int
	for _, op := range after {
		if op.Status == "established" {
			confirmed++
		}
	}
	assert.Equal(t, 1, confirmed, "the person's decision landed on the agent's proposal")
}

// TestCommandLineAgentIsDetectedFromItsHost covers the path that needs no
// configuration: the marker variable an agent host exports is enough.
func TestCommandLineAgentIsDetectedFromItsHost(t *testing.T) {
	root := contextActorProject(t, "ctx-actor-detected")
	recipe := filepath.Join(root, "kapi.yaml")
	// The host's own session variable is cleared, so the session comes from the
	// process that started this command. Two kapi processes launched from one
	// test process therefore stand for two commands of one agent run.
	claudeCode := []string{"CLAUDECODE=1", "CLAUDE_CODE_SESSION_ID="}

	kapiEnvOK(t, claudeCode, "context", "observe", "the guide names the product Kapi Desktop",
		"--seen-in", "docs/guide.md", "-p", recipe)
	kapiEnvOK(t, claudeCode, "context", "observe", "the guide keeps the present tense",
		"--seen-in", "docs/guide.md", "-p", recipe)

	recorded := readLog(t, nil, "context", "log", "--actor", "agent", "--json", "-p", recipe)
	require.Len(t, recorded, 2)
	for _, op := range recorded {
		assert.Equal(t, "agent", op.Actor.Kind)
		assert.Equal(t, "claude-code", op.Actor.Name)
		assert.NotEmpty(t, op.Actor.Session)
		assert.Empty(t, op.Note, "a detected agent leaves no note of its own")
	}
	assert.Equal(t, recorded[0].Actor.Session, recorded[1].Actor.Session,
		"two kapi processes of one run record under one session")
}

// TestAPersonInAnAgentShellSaysSo: the override is the person's, and the
// record says the operation used it.
func TestAPersonInAnAgentShellSaysSo(t *testing.T) {
	root := contextActorProject(t, "ctx-actor-override")
	recipe := filepath.Join(root, "kapi.yaml")

	kapiEnvOK(t, []string{"CLAUDECODE=1", "KAPI_ACTOR=person"},
		"context", "observe", "the guide addresses the reader as you", "-p", recipe)

	recorded := readLog(t, nil, "context", "log", "--json", "-p", recipe)
	require.Len(t, recorded, 1)
	assert.Equal(t, "person", recorded[0].Actor.Kind)
	assert.Equal(t, "recorded as a person in a claude-code shell", recorded[0].Note)
}

// TestKapiInitWiresCodex is issue #2911: Codex reads a repository's own
// configuration layer, so `kapi init` writes the entry inside the project.
func TestKapiInitWiresCodex(t *testing.T) {
	root := t.TempDir()
	out := kapi(t, "init", "--dir", root, "--name", "codex-wiring", "--agents", "codex")
	assert.Contains(t, out, ".codex/config.toml")
	assert.Contains(t, out, "once you trust this repository there")

	written, err := os.ReadFile(filepath.Join(root, ".codex", "config.toml"))
	require.NoError(t, err)
	assert.Contains(t, string(written), "[mcp_servers.kapi]")
	assert.Contains(t, string(written), `command = "kapi"`)
}
