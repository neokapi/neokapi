package cli

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMCPExplicitProjectFlag(t *testing.T) {
	t.Setenv("KAPI_NO_PROJECT", "1")
	cmd := NewMCPCmd(&App{}, "kapi")
	require.NotNil(t, cmd.Flags().Lookup("project"))
	assert.Equal(t, "p", cmd.Flags().Lookup("project").Shorthand)
	require.NoError(t, cmd.ParseFlags([]string{"-p", "custom.kapi"}))
	path, err := cmd.Flags().GetString("project")
	require.NoError(t, err)
	assert.Equal(t, "custom.kapi", path)
}

func TestMCPUnavailableExplicitProjectFailsBeforeServing(t *testing.T) {
	t.Setenv("KAPI_NO_PROJECT", "1")
	cmd := NewMCPCmd(&App{}, "kapi")
	cmd.SetArgs([]string{"-p", filepath.Join(t.TempDir(), "missing.kapi")})
	require.ErrorContains(t, cmd.Execute(), "load MCP project")
}
