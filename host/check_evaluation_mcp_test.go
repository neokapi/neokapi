//go:build !js

package host

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCheckEvaluationDraftNamesTheProjectThatGovernedIt checks a draft over
// MCP: one named for a destination is held to the voice and terms in force
// there and the record names that project, while a snippet checked on its own
// is held to the call's options and names none.
func TestCheckEvaluationDraftNamesTheProjectThatGovernedIt(t *testing.T) {
	isolateCheckExecution(t)
	pinEvaluationBuild(t)
	dir := t.TempDir()
	writeCheckInput(t, dir, "app.json", `{"title":"Hello world"}`)
	recipe := writeCheckInput(t, dir, "kapi.yaml", `version: v1
id: prj_ffffgggghhhhiiiijjjj
name: Evaluation Fixture
defaults:
  source_language: en
collections:
  - path: app.json
`)
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".kapi"), 0o755))
	app := &App{SourceLang: "en"}
	app.SetWorkspaceRoot(filepath.Join(dir, "workspace"))

	_, ungoverned, err := app.checkTextMCP(t.Context(), checkTextInput{Text: "Ready to ship."})
	require.NoError(t, err)
	require.NotNil(t, ungoverned.Evaluation)
	assert.Nil(t, ungoverned.Evaluation.Context, "no project governed this draft, so the record claims none")

	_, governed, err := app.checkTextMCP(t.Context(), checkTextInput{
		Text: "Ready to ship.", Project: recipe, ContextPath: "app.json",
	})
	require.NoError(t, err)
	require.NotNil(t, governed.Evaluation)
	require.NotNil(t, governed.Evaluation.Context)
	assert.Equal(t, "prj_ffffgggghhhhiiiijjjj", governed.Evaluation.Context.Project)
}
