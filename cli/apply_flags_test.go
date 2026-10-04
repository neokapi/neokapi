package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --gate takes enforce or report; anything else is a malformed invocation and
// exits 2 before a change set is read.
func TestApplyGateFlagRefusesAnUnknownGate(t *testing.T) {
	t.Setenv("KAPI_NO_PROJECT", "1")
	cmd := NewApplyCmd(newAppForTest(t))
	cmd.SetArgs([]string{"--gate", "foo", "change.json"})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	err := cmd.Execute()
	require.Error(t, err)
	assert.Equal(t, ExitUsage, ExitCode(cmd, err))
	assert.Contains(t, err.Error(), "enforce or report")
}

// kapi inspect names its project with -p, as kapi apply does, and renders
// blocks in other formats with --render.
func TestInspectFlags(t *testing.T) {
	cmd := NewInspectCmd(newAppForTest(t))
	p := cmd.Flags().Lookup("project")
	require.NotNil(t, p)
	assert.Equal(t, "p", p.Shorthand)
	assert.Equal(t, "string", p.Value.Type())
	r := cmd.Flags().Lookup("render")
	require.NotNil(t, r)
	assert.True(t, strings.Contains(r.Usage, "html"))
}

// --out names the file of an edition outside a project; inside one the
// recipe's target names it, and --out is a usage error that says so.
func TestApplyOutIsRefusedInsideAProject(t *testing.T) {
	dir := t.TempDir()
	recipe := filepath.Join(dir, "kapi.yaml")
	require.NoError(t, os.WriteFile(recipe, []byte("version: v1\ndefaults:\n  source_language: en\ncollections:\n  - path: a.md\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.md"), []byte("Hello\n"), 0o644))
	change := filepath.Join(dir, "change.json")
	require.NoError(t, os.WriteFile(change, []byte(`{"ops":[]}`), 0o644))
	cmd := NewApplyCmd(newAppForTest(t))
	cmd.SetArgs([]string{"-p", recipe, "--out", "nb.md", change})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	err := cmd.Execute()
	require.Error(t, err)
	assert.Equal(t, ExitUsage, ExitCode(cmd, err))
	assert.Contains(t, err.Error(), "--out: inside a project the recipe's target names the file of each edition")
}
