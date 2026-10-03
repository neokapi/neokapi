//go:build !js

package host

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/change"
	fmtpkg "github.com/neokapi/neokapi/core/format"
)

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
		refusal := refusalOf(t, out)
		assert.Equal(t, change.CodeStale, refusal.Code, refusal.Message)
		require.NotNil(t, out.Result.Ops[0].Current, "the refusal carries the comment as it stands")
		assert.Contains(t, out.Result.Ops[0].Current.Text, "from any [io.Reader]")
		assert.Empty(t, out.Comments, "nothing reached the comment path")
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

	t.Run(`"*" takes the comment as it stands`, func(t *testing.T) {
		isolateCheckExecution(t)
		file := writeCheckInput(t, t.TempDir(), "parse.go", repairGo)
		out, _, err := runApplyChangeSet(t, NewEnvCommand(t.Context(), "apply"), false,
			guardedEntry(file, "func/Parse", repairedParse, nil))
		require.NoError(t, err)
		assert.Equal(t, commentWritten, out.Comments[0].Edits[0].Status, out.Comments[0].Edits[0].Detail)
	})

	t.Run("an operation with no if_match is refused before anything is applied", func(t *testing.T) {
		isolateCheckExecution(t)
		noProject(t)
		file := writeCheckInput(t, t.TempDir(), "parse.go", repairGo)
		body := changeSetOf(t, map[string]any{"op": "set_content", "at": map[string]any{"doc": file, "block": "func/Parse"}, "text": repairedParse})
		_, _, err := runApply(t, newToolboxApp(t), NewEnvCommand(t.Context(), "apply"), body, ApplyOptions{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "if_match")
		assert.Equal(t, ExitUsage, ExitCode(nil, err), "a malformed change set is not a gate failure")
		assertUnchanged(t, file, repairGo)
	})
}
