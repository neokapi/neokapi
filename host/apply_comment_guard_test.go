package host

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"maps"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/comment/golang"
	fmtpkg "github.com/neokapi/neokapi/core/format"
)

// commentSHA256 locates the comment id in src and returns the hex SHA-256 of
// its bytes and the lines it spans, computed here and not by the code under
// test.
func commentSHA256(t *testing.T, src, id string) (string, fmtpkg.LineRange) {
	t.Helper()
	located, err := golang.Provider{}.Locate("parse.go", []byte(src))
	require.NoError(t, err)
	for i, b := range located.Blocks() {
		if b.ID == id {
			c := located.Comments[i]
			sum := sha256.Sum256([]byte(src[c.Start:c.End]))
			return hex.EncodeToString(sum[:]), c.Lines
		}
	}
	t.Fatalf("no comment %s", id)
	return "", fmtpkg.LineRange{}
}

// findingFingerprints reads the comment_sha256 of each finding from a report's
// JSON, keyed by the finding's block.
func findingFingerprints(t *testing.T, report check.Report) map[string]string {
	t.Helper()
	body, err := json.Marshal(report)
	require.NoError(t, err)
	var doc struct {
		Findings []struct {
			Location map[string]any `json:"location"`
		} `json:"findings"`
	}
	require.NoError(t, json.Unmarshal(body, &doc))
	out := map[string]string{}
	for _, f := range doc.Findings {
		block, _ := f.Location["block"].(string)
		sum, _ := f.Location["comment_sha256"].(string)
		out[block] = sum
	}
	return out
}

func guardedEntry(file, id, text string, guard map[string]any) map[string]any {
	e := map[string]any{"kind": "comment", "file": file, "id": id, "text": text}
	maps.Copy(e, guard)
	return e
}

// A comment edit is guarded by what the agent read: the fingerprint a check
// reports for the comment, or the comment's prose. The line range only locates
// it.
func TestCommentEditGuard(t *testing.T) {
	t.Run("a comment finding carries the fingerprint of the comment's bytes", func(t *testing.T) {
		isolateCheckExecution(t)
		file := writeCheckInput(t, t.TempDir(), "parse.go", repairGo)
		want, _ := commentSHA256(t, repairGo, "func/Parse")

		report, err := (&App{SourceLang: "en"}).ComputeCheck(executionCommand(t), []string{file})
		require.NoError(t, err)
		assert.Equal(t, want, findingFingerprints(t, report)["func/Parse"], "a whole-file check")

		_, mcpReport, err := (&App{SourceLang: "en"}).checkFileMCP(t.Context(), checkFileInput{File: file})
		require.NoError(t, err)
		assert.Equal(t, want, findingFingerprints(t, mcpReport)["func/Parse"], "MCP check_file")

		diff := diffCheckFiles(t, nil, map[string]string{"parse.go": parseGo}, editParse)
		scoped, _ := commentSHA256(t, parseGo, "func/Parse")
		assert.Equal(t, scoped, findingFingerprints(t, diff)["func/Parse"], "a diff-scoped check")
	})

	t.Run("the fingerprint a finding reports, passed back, repairs the finding", func(t *testing.T) {
		isolateCheckExecution(t)
		file := writeCheckInput(t, t.TempDir(), "parse.go", repairGo)
		report, err := (&App{SourceLang: "en"}).ComputeCheck(executionCommand(t), []string{file})
		require.NoError(t, err)
		finding := findingOf(t, report, "hygiene.doubled-word")
		sum := findingFingerprints(t, report)[finding.Location.Block]
		require.NotEmpty(t, sum)

		out, _, err := runApplyChangeSet(t, NewEnvCommand(t.Context(), "apply"), false,
			guardedEntry(file, finding.Location.Block, repairedParse, map[string]any{"comment_sha256": sum, "lines": finding.Location.Lines}))
		require.NoError(t, err)
		assert.Equal(t, commentWritten, out.Comments[0].Edits[0].Status, out.Comments[0].Edits[0].Detail)
	})

	t.Run("an edit whose comment changed since it was checked is refused and writes nothing", func(t *testing.T) {
		isolateCheckExecution(t)
		file := writeCheckInput(t, t.TempDir(), "parse.go", repairGo)
		sum, lines := commentSHA256(t, repairGo, "func/Parse")
		// A teammate rewrites the comment after the check, keeping its lines.
		theirs := strings.Replace(repairGo, "from an [io.Reader]", "from any [io.Reader]", 1)
		require.NoError(t, os.WriteFile(file, []byte(theirs), 0o644))

		out, _, err := runApplyChangeSet(t, NewEnvCommand(t.Context(), "apply"), false,
			guardedEntry(file, "func/Parse", repairedParse, map[string]any{"comment_sha256": sum, "lines": lines}))
		assert.Equal(t, ExitGate, ExitCode(nil, err))
		require.Len(t, out.Comments, 1)
		edit := out.Comments[0].Edits[0]
		assert.Equal(t, commentRefused, edit.Status)
		assert.Equal(t, "changed", edit.Reason, edit.Detail)
		assertUnchanged(t, file, theirs)
	})

	t.Run("an edit whose comment moved lines and kept its bytes is written at the new lines", func(t *testing.T) {
		isolateCheckExecution(t)
		file := writeCheckInput(t, t.TempDir(), "parse.go", repairGo)
		sum, lines := commentSHA256(t, repairGo, "func/Parse")
		require.Equal(t, fmtpkg.LineRange{First: 5, Last: 7}, lines)
		// Code added above the comment after the check moves it two lines down.
		moved := strings.Replace(repairGo, "import \"io\"\n", "import \"io\"\n\nvar _ = io.EOF\n", 1)
		require.NoError(t, os.WriteFile(file, []byte(moved), 0o644))

		out, _, err := runApplyChangeSet(t, NewEnvCommand(t.Context(), "apply"), false,
			guardedEntry(file, "func/Parse", repairedParse, map[string]any{"comment_sha256": sum, "lines": lines}))
		require.NoError(t, err)
		edit := out.Comments[0].Edits[0]
		assert.Equal(t, commentWritten, edit.Status, edit.Detail)
		assert.Equal(t, &fmtpkg.LineRange{First: 7, Last: 9}, edit.Lines)
		after, err := os.ReadFile(file)
		require.NoError(t, err)
		assert.Equal(t, strings.Replace(moved, "// Parse reads the the input", "// Parse reads the input", 1), string(after))
	})

	t.Run("current_text guards an edit that carries no fingerprint", func(t *testing.T) {
		isolateCheckExecution(t)
		file := writeCheckInput(t, t.TempDir(), "parse.go", repairGo)
		out, _, err := runApplyChangeSet(t, NewEnvCommand(t.Context(), "apply"), false,
			guardedEntry(file, "func/Parse", repairedParse, map[string]any{"current_text": "Parse reads the input from an [io.Reader].\n\nIt stops at the end."}))
		assert.Equal(t, ExitGate, ExitCode(nil, err))
		assert.Equal(t, "changed", out.Comments[0].Edits[0].Reason, out.Comments[0].Edits[0].Detail)
		assertUnchanged(t, file, repairGo)

		out, _, err = runApplyChangeSet(t, NewEnvCommand(t.Context(), "apply"), false,
			guardedEntry(file, "func/Parse", repairedParse, map[string]any{"current_text": "Parse reads the the input from an [io.Reader].\n\nIt stops at the end."}))
		require.NoError(t, err)
		assert.Equal(t, commentWritten, out.Comments[0].Edits[0].Status, out.Comments[0].Edits[0].Detail)
	})

	t.Run("an entry with no guard is rejected before anything is applied", func(t *testing.T) {
		isolateCheckExecution(t)
		file := writeCheckInput(t, t.TempDir(), "parse.go", repairGo)
		_, _, err := runApplyChangeSetRaw(t, guardedEntry(file, "func/Parse", repairedParse, map[string]any{"lines": map[string]int{"first": 5, "last": 7}}))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "comment_sha256")
		assert.NotEqual(t, ExitGate, ExitCode(nil, err), "a malformed change-set is not a gate failure")
		assertUnchanged(t, file, repairGo)
	})
}

// runApplyChangeSetRaw runs kapi apply over entries and returns the error
// without reading a report, for a change-set rejected before any output.
func runApplyChangeSetRaw(t *testing.T, entries ...map[string]any) (string, string, error) {
	t.Helper()
	var lines []string
	for _, e := range entries {
		b, err := json.Marshal(e)
		require.NoError(t, err)
		lines = append(lines, string(b))
	}
	path := t.TempDir() + "/changeset.jsonl"
	require.NoError(t, os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600))
	cmd := NewEnvCommand(t.Context(), "apply")
	var stdout, stderr strings.Builder
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	err := (&App{SourceLang: "en"}).RunApply(cmd, path, false, "", true)
	return stdout.String(), stderr.String(), err
}
