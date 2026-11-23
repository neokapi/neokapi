//go:build !js

package host

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// An App serving an MCP client answers an agent, which cannot allow a folder.
func TestContextStore_AnMCPServerCannotApprove(t *testing.T) {
	app, _ := contextOpsApp(t)
	app.NoteMCPSession(t.Context(), "", "")
	recipe := folderProject(t, app, filepath.Join(t.TempDir(), "team"))
	_, err := app.ApproveContextStore(recipe)
	require.ErrorContains(t, err, "only a person can allow")
}
