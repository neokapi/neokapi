//go:build !windows

package backend

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/host"
)

// The flow editor's preview runs a project's commands only once the person
// allowed them through the dialog, and a script changed during the session
// is asked about again: the preview refuses it, and the dialog's question
// names the changed file.
func TestExecTrust_TheDesktopAsksBeforeEachRun(t *testing.T) {
	t.Setenv("KAPI_CONFIG_DIR", t.TempDir())
	t.Setenv("KAPI_TRUST_EXEC", "")
	app := newWorkspaceApp(t)
	dir, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	recipe := filepath.Join(dir, "kapi.yaml")
	require.NoError(t, os.WriteFile(recipe, []byte("version: v1\nid: prj_dddddddddddddddddddddddd\nname: Commands\nflows:\n  cmd:\n    steps:\n"+
		"      - tool: external-command\n        config:\n          command: ./tools/run.sh\n          apply_source: true\n          apply_target: false\n"), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "tools"), 0o755))
	script := filepath.Join(dir, "tools", "run.sh")
	require.NoError(t, os.WriteFile(script, []byte("#!/bin/sh\ncat\n"), 0o755))
	tab, err := app.OpenProject(recipe)
	require.NoError(t, err)
	t.Cleanup(func() { app.CloseProject(tab.ID) })

	_, err = app.PreviewFlow(tab.ID, "cmd", "Hello", "en", "fr")
	require.ErrorIs(t, err, host.ErrExecNotTrusted, "nothing runs before the answer")

	req, err := app.PendingExecTrust(tab.ID)
	require.NoError(t, err)
	require.NotNil(t, req)
	assert.Equal(t, []string{"tools/run.sh"}, req.Checked)
	assert.NotEmpty(t, req.Limit)
	require.NoError(t, app.ApproveExecTrust(tab.ID, req.Digest))
	none, err := app.PendingExecTrust(tab.ID)
	require.NoError(t, err)
	assert.Nil(t, none)
	_, err = app.PreviewFlow(tab.ID, "cmd", "Hello", "en", "fr")
	require.NotErrorIs(t, err, host.ErrExecNotTrusted)

	require.NoError(t, os.WriteFile(script, []byte("#!/bin/sh\necho changed; cat\n"), 0o755))
	_, err = app.PreviewFlow(tab.ID, "cmd", "Hello", "en", "fr")
	require.ErrorIs(t, err, host.ErrExecNotTrusted, "a script changed mid-session is not run")
	again, err := app.PendingExecTrust(tab.ID)
	require.NoError(t, err)
	require.NotNil(t, again)
	assert.Equal(t, []string{"tools/run.sh"}, again.Changed)
	require.Error(t, app.ApproveExecTrust(tab.ID, req.Digest), "the answer shown before the change does not cover it")
}
