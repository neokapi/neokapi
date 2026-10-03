//go:build !js

package host

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/check"
)

// TestCheckNotRun_TheMCPToolsTellTheCausesApart runs the same two cases through
// the agent-facing tools: check_text with a broken checker, and check_file with
// a diff that touches no block.
func TestCheckNotRun_TheMCPToolsTellTheCausesApart(t *testing.T) {
	isolateCheckExecution(t)
	dir := t.TempDir()
	writeCheckInput(t, dir, "doc.md", "# Title\n\nPara.\n\n")
	t.Chdir(dir)
	app := &App{SourceLang: "en"}

	real := hygieneTool
	hygieneTool = inertHygiene
	t.Cleanup(func() { hygieneTool = real })
	_, broken, err := app.checkTextMCP(t.Context(), checkTextInput{Text: "Hello world"})
	require.NoError(t, err)
	hygieneTool = real

	_, empty, err := app.checkFileMCP(t.Context(), checkFileInput{Diff: "--- a/doc.md\n+++ b/doc.md\n@@ -3,0 +4 @@ Para.\n+\n"})
	require.NoError(t, err)

	assert.Equal(t, check.VerdictDidNotRun, broken.Verdict)
	assert.Equal(t, check.VerdictDidNotRun, empty.Verdict)
	assert.Equal(t, check.CauseCheckerInvalid, broken.DidNotRunCause)
	assert.Equal(t, check.CauseNothingToCheck, empty.DidNotRunCause)
	require.NotEmpty(t, broken.DidNotRun)
	require.NotEmpty(t, empty.DidNotRun)
	assert.Contains(t, broken.DidNotRun[0], "canary")
	assert.NotContains(t, empty.DidNotRun[0], "canary")
}
