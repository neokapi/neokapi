package host

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/model"
)

// --diff writes a unified diff for a person to read and --json a report for a
// program to parse. Written to the same stream, neither arrives: the diff lines
// sit in front of the JSON document, so a reader that parses stdout fails on
// the first line.
func TestApplyDiffJSONWritesJSONToStdout(t *testing.T) {
	isolateCheckExecution(t)
	dir := t.TempDir()
	content := filepath.Join(dir, "en.json")
	const contentSrc = `{"greeting":"Hello world"}`
	require.NoError(t, os.WriteFile(content, []byte(contentSrc), 0o644))
	code := writeCheckInput(t, dir, "parse.go", repairGo)

	var lines []string
	for _, e := range []map[string]any{
		{"kind": "content", "file": content, "content_hash": model.ComputeContentHash("Hello world"), "text": "Hi planet"},
		commentEntry(code, "func/Parse", nil, repairedParse),
	} {
		b, err := json.Marshal(e)
		require.NoError(t, err)
		lines = append(lines, string(b))
	}
	changeset := filepath.Join(t.TempDir(), "changeset.jsonl")
	require.NoError(t, os.WriteFile(changeset, []byte(strings.Join(lines, "\n")+"\n"), 0o600))

	cmd := NewEnvCommand(t.Context(), "apply")
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	require.NoError(t, newToolboxApp(t).RunApply(cmd, changeset, true, "", true))

	var out applyOutput
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &out), "stdout is one JSON document: %s", stdout.String())
	require.Len(t, out.Comments, 1)
	assert.Contains(t, out.Comments[0].Diff, "+// Parse reads the input from an [io.Reader].",
		"the report carries the comment's diff")

	assert.Contains(t, stderr.String(), "Hi planet", "a person reads the content diff on stderr")
	assert.Contains(t, stderr.String(), "+// Parse reads the input from an [io.Reader].",
		"a person reads the comment diff on stderr")

	assertUnchanged(t, content, contentSrc)
	assertUnchanged(t, code, repairGo)
}
