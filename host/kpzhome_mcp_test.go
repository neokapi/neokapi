//go:build !js

package host

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestKpzEdit_TheMCPEditToolsReachAKPZDocument: an agent reads a KPZ's source
// through read_blocks and edits it through apply_edits, by the reference and
// revision the read reported; the edit lands in the KPZ's workspace home.
func TestKpzEdit_TheMCPEditToolsReachAKPZDocument(t *testing.T) {
	_, work := kpzWorkspace(t, "fr")
	app := newToolboxApp(t)
	outsideAProject(t, filepath.Dir(work))
	session := editSession(t, app, "kpz-edit")

	var page mcpPage
	isErr, text := callEditTool(t, session, "read_blocks", map[string]any{"doc": "work.kpz!messages.json"}, &page)
	require.False(t, isErr, text)
	ref, rev, _ := page.blockWith(t, "Hello")
	set := map[string]any{"ops": []any{map[string]any{
		"op": "set_content", "at": ref, "if_match": rev, "text": "Hello from an agent",
	}}}
	var res mcpResult
	isErr, text = callEditTool(t, session, "apply_edits", set, &res)
	require.False(t, isErr, text)
	assert.Equal(t, "applied", res.Status, text)
	assert.NotEmpty(t, mustRev(t, session, "work.kpz!messages.json", "Hello from an agent"))
}
