//go:build !js

package host

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDiffCheck_MCPMatchesTheCLI(t *testing.T) {
	isolateCheckExecution(t)
	dir := t.TempDir()
	writeCheckInput(t, dir, "doc.md", "# Title\n\nOne.\n\nTwo, changed.\n")
	patch := "--- a/doc.md\n+++ b/doc.md\n@@ -5 +5 @@\n-Two.\n+Two, changed.\n"
	t.Chdir(dir)
	app := &App{SourceLang: "en"}

	cmd := diffCommand(t)
	require.NoError(t, cmd.Flags().Set("diff-file", StdinName))
	cmd.SetIn(bytes.NewBufferString(patch))
	cli, err := app.ComputeCheck(cmd, nil)
	require.NoError(t, err)

	_, mcp, err := app.checkFileMCP(t.Context(), checkFileInput{Diff: patch})
	require.NoError(t, err)
	require.NotNil(t, mcp.Scope)
	assert.Equal(t, cli.Scope.Files, mcp.Scope.Files)
	assert.Equal(t, cli.Verdict, mcp.Verdict)

	_, _, err = app.checkFileMCP(t.Context(), checkFileInput{})
	require.ErrorContains(t, err, "file is required")
	_, _, err = app.checkFileMCP(t.Context(), checkFileInput{Diff: patch, Target: "x.md"})
	require.Error(t, err)
}
