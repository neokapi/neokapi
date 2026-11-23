package mcptools

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/host"
)

// Every location argument of the tools this package registers, beside the
// host's, is declared, so the roots middleware checks it on every call.
func TestEveryKapiMCPPathArgumentIsDeclared(t *testing.T) {
	undeclared, err := host.AuditMCPPathArgs(t.Context(), testApp())
	require.NoError(t, err)
	assert.Empty(t, undeclared)
}
