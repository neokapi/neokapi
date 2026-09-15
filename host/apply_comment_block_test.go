package host

import (
	"bytes"
	"go/format"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/comment"
	"github.com/neokapi/neokapi/core/comment/golang"
	fmtpkg "github.com/neokapi/neokapi/core/format"
)

// repairBlockGo holds a doubled word in the delimited doc comment of Parse, on
// lines 5-9, written with a line of asterisks.
const repairBlockGo = "package demo\n\nimport \"io\"\n\n/*\n * Parse reads the the input from an [io.Reader].\n *\n * It stops at the end.\n */\nfunc Parse(r io.Reader) {}\n\n// Other is untouched.\nfunc Other() {}\n"

// repairedBlockGo is repairBlockGo with the doubled word removed and nothing
// else changed.
var repairedBlockGo = strings.Replace(repairBlockGo, "the the", "the", 1)

const repairedBlockParse = "Parse reads the input from an [io.Reader].\n\nIt stops at the end."

// TestProseP4_go is the P4 rung for Go comments through the product. A finding
// in a delimited comment is repaired through `kapi apply` and MCP apply_edits
// in the layout the comment was written in, with every other byte of the file
// identical and gofmt agreeing, and the check that comes back with the edit
// clears it. Text holding `*/` is refused and writes nothing, and the write
// canary refuses a renderer that would write it.
//
// The subtest named "must fail" breaks the write path on purpose and asserts
// that the run notices. Nothing in here skips.
func TestProseP4_go(t *testing.T) {
	formatted, err := format.Source([]byte(repairBlockGo))
	require.NoError(t, err)
	require.Equal(t, repairBlockGo, string(formatted), "the fixture is gofmt-clean")

	t.Run("a finding in a delimited comment is repaired through kapi apply in its layout", func(t *testing.T) {
		isolateCheckExecution(t)
		file := writeCheckInput(t, t.TempDir(), "parse.go", repairBlockGo)
		before, err := (&App{SourceLang: "en"}).ComputeCheck(executionCommand(t), []string{file})
		require.NoError(t, err)
		finding := findingOf(t, before, "hygiene.doubled-word")
		require.Equal(t, "func/Parse", finding.Location.Block)
		require.Equal(t, &fmtpkg.LineRange{First: 5, Last: 9}, finding.Location.Lines)

		out, _, err := runApplyChangeSet(t, NewEnvCommand(t.Context(), "apply"), false,
			commentEntry(file, finding.Location.Block, finding.Location.Lines, repairedBlockParse))
		require.NoError(t, err)
		require.Len(t, out.Comments, 1)
		edit := out.Comments[0].Edits[0]
		assert.Equal(t, commentWritten, edit.Status, edit.Detail)
		assert.Equal(t, &fmtpkg.LineRange{First: 5, Last: 9}, edit.Lines)
		result := out.Comments[0].Check
		require.NotNil(t, result, out.Comments[0].CheckError)
		assert.Equal(t, check.VerdictPassed, result.Verdict, result.DidNotRun)
		assert.NotContains(t, rulesOf(result), "hygiene.doubled-word", "the repair cleared the finding")

		after, err := os.ReadFile(file)
		require.NoError(t, err)
		assert.Equal(t, repairedBlockGo, string(after), "the doubled word is the only change")
		formatted, err := format.Source(after)
		require.NoError(t, err)
		assert.Equal(t, string(after), string(formatted), "gofmt agrees with the written file")
	})

	t.Run("a finding in a delimited comment is repaired through MCP apply_edits", func(t *testing.T) {
		isolateCheckExecution(t)
		file := writeCheckInput(t, t.TempDir(), "parse.go", repairBlockGo)
		result := callApplyEdits(t, commentEntry(file, "func/Parse", &fmtpkg.LineRange{First: 5, Last: 9}, repairedBlockParse))
		assert.True(t, result.OK)
		require.Len(t, result.Comments, 1)
		assert.Equal(t, commentWritten, result.Comments[0].Edits[0].Status, result.Comments[0].Edits[0].Detail)
		require.NotNil(t, result.Comments[0].Check)
		assert.Equal(t, check.VerdictPassed, result.Comments[0].Check.Verdict)
		assertUnchanged(t, file, repairedBlockGo)
	})

	t.Run("text holding */ is refused and writes nothing", func(t *testing.T) {
		isolateCheckExecution(t)
		file := writeCheckInput(t, t.TempDir(), "parse.go", repairBlockGo)
		out, _, err := runApplyChangeSet(t, NewEnvCommand(t.Context(), "apply"), false,
			commentEntry(file, "func/Parse", nil, "Parse reads the input from an [io.Reader] */ and stops.\n\nIt stops at the end."))
		assert.Equal(t, ExitGate, ExitCode(nil, err))
		edit := out.Comments[0].Edits[0]
		assert.Equal(t, commentRefused, edit.Status)
		assert.Equal(t, string(comment.RefusedTerminator), edit.Reason, edit.Detail)
		assert.Empty(t, out.Comments[0].Diff)
		assert.Nil(t, out.Comments[0].Check)
		assertUnchanged(t, file, repairBlockGo)
	})

	t.Run("must fail: a renderer that writes */ as it is invalidates the run and writes nothing", func(t *testing.T) {
		isolateCheckExecution(t)
		file := writeCheckInput(t, t.TempDir(), "parse.go", repairBlockGo)
		swapCommentProviders(t, rawTerminatorGo{})
		out, _, err := runApplyChangeSet(t, NewEnvCommand(t.Context(), "apply"), false, commentEntry(file, "func/Parse", nil, repairedBlockParse))
		assert.Equal(t, ExitGate, ExitCode(nil, err))
		edit := out.Comments[0].Edits[0]
		assert.Equal(t, commentNotRun, edit.Status)
		assert.Equal(t, reasonCanary, edit.Reason)
		assert.Contains(t, edit.Detail, "not refused as holding its terminator")
		assertUnchanged(t, file, repairBlockGo)
	})
}

// rawTerminatorGo renders like the Go provider, then writes each `*/` the text
// holds into the comment as it is.
type rawTerminatorGo struct{ golang.Provider }

func (p rawTerminatorGo) Render(name string, src []byte, c comment.Comment, text string, opts comment.RenderOptions) ([]byte, error) {
	const standIn = "zzTERMINATORzz"
	span, err := p.Provider.Render(name, src, c, strings.ReplaceAll(text, "*/", standIn), opts)
	return bytes.ReplaceAll(span, []byte(standIn), []byte("*/")), err
}
