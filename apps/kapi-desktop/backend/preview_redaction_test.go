package backend

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/host"
)

// previewRedactionProject opens a project whose defaults.redaction withholds
// "Ada Lovelace", with a flow that redacts, runs a program that records what
// it reads, and restores. It returns the app, the tab and the file the program
// writes what it read to.
func previewRedactionProject(t *testing.T) (*App, string, string) {
	t.Helper()
	t.Setenv("KAPI_CONFIG_DIR", t.TempDir())
	app := NewApp()
	tab := newTestProject(t, app, "PreviewRedaction")
	op := app.getOpenProject(tab.ID)
	require.NotNil(t, op)
	root := filepath.Dir(op.Path)
	require.NoError(t, os.WriteFile(filepath.Join(root, "redact.yaml"),
		[]byte("version: v1\nrules:\n  - term: Ada Lovelace\n    category: person\n"), 0o600))
	op.Project.Defaults.Redaction = &project.RedactionSpec{Enabled: true, Rules: "redact.yaml", Detectors: []string{"rules"}}

	saw := filepath.Join(t.TempDir(), "saw.txt")
	script := filepath.Join(t.TempDir(), "record.sh")
	require.NoError(t, os.WriteFile(script, []byte("#!/bin/sh\ntee \""+saw+"\"\n"), 0o700))
	if op.Project.Flows == nil {
		op.Project.Flows = map[string]*flow.StepsSpec{}
	}
	op.Project.Flows["rewrite"] = &flow.StepsSpec{Steps: []flow.FlowStep{
		{Tool: "redact"},
		{Tool: "external-command", Config: map[string]any{"command": script, "applySource": true, "applyTarget": false}},
		{Tool: "unredact"},
	}}
	return app, tab.ID, saw
}

// The preview builds its chain as a flow run does, so its redact step takes
// the project's policy. A policy with no rules file would withhold nothing,
// and the preview refuses it as a run does.
func TestPreviewFlow_TheRedactStepTakesTheProjectsPolicy(t *testing.T) {
	app, tab, saw := previewRedactionProject(t)
	t.Setenv("KAPI_TRUST_EXEC", "1")
	app.getOpenProject(tab).Project.Defaults.Redaction.Rules = ""

	_, err := app.PreviewFlow(tab, "rewrite", "Write to Ada Lovelace today.", "en", "fr")
	require.ErrorContains(t, err, "the redact step takes the policy at defaults.redaction: redaction is enabled but no rules file is set")
	_, statErr := os.Stat(saw)
	assert.True(t, os.IsNotExist(statErr), "the program never ran")
}

// A preview that would run a program needs the trust decision a flow run
// needs.
func TestPreviewFlow_AnExecStepNeedsTrust(t *testing.T) {
	app, tab, saw := previewRedactionProject(t)
	t.Setenv("KAPI_TRUST_EXEC", "")

	_, err := app.PreviewFlow(tab, "rewrite", "Write to Ada Lovelace today.", "en", "fr")
	require.ErrorIs(t, err, host.ErrExecNotTrusted)
	_, statErr := os.Stat(saw)
	assert.True(t, os.IsNotExist(statErr), "the program never ran")
}
