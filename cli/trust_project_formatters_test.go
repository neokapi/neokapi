package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// kapi apply and kapi mcp each take --trust-project-formatters, which lets the
// comment edits they run use the formatter of the project they edit. Without
// the flag, the App trusts no project's formatter.
func TestTrustProjectFormattersFlag(t *testing.T) {
	t.Setenv("KAPI_NO_PROJECT", "1")

	apply := &App{}
	require.NoError(t, NewApplyCmd(apply).ParseFlags([]string{"--trust-project-formatters"}))
	assert.True(t, apply.TrustProjectFormatters)

	server := &App{}
	require.NoError(t, NewMCPCmd(server, "kapi").ParseFlags([]string{"--trust-project-formatters"}))
	assert.True(t, server.TrustProjectFormatters)

	untrusted := &App{}
	require.NoError(t, NewMCPCmd(untrusted, "kapi").ParseFlags(nil))
	assert.False(t, untrusted.TrustProjectFormatters)
}
