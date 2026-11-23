package bowrainmcp

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/host"
)

// Every location argument of the plugin's tools is declared, so kapi mcp's
// roots middleware checks it on every call.
func TestEveryBowrainMCPPathArgumentIsDeclared(t *testing.T) {
	undeclared, err := host.AuditMCPPathArgs(t.Context(), bowrainTestApp())
	require.NoError(t, err)
	assert.Empty(t, undeclared)
}
