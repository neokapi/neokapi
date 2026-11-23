package source

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/host"
)

// mustLocalConnector is NewLocalConnector for a test, failing it on error.
func mustLocalConnector(t *testing.T, app *host.App, proj *Project, reg *registry.FormatRegistry) *BowrainSourceConnector {
	t.Helper()
	conn, err := NewLocalConnector(t.Context(), app, proj, reg)
	require.NoError(t, err)
	return conn
}
