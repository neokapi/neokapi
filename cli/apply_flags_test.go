package cli

import (
	"bytes"
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
