//go:build !js

package host

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/project"
)

func TestMCPApplyEdits_APreviewInAFreshProjectWritesNothing(t *testing.T) {
	root, recipe := freshProject(t)
	app := &App{SourceLang: "en"}
	t.Cleanup(app.Shutdown)
	app.InitRegistries()
	session := editSession(t, app, "preview-agent")

	var page mcpPage
	isErr, body := callEditTool(t, session, "read_blocks", map[string]any{"doc": "docs/guide.md", "project": recipe}, &page)
	require.False(t, isErr, body)
	ref, rev, _ := page.blockWith(t, "every day")
	before := fullTreeOf(t, root)

	var res change.Result
	isErr, body = callEditTool(t, session, "apply_edits", map[string]any{
		"project": recipe, "mode": "preview",
		"ops": []any{map[string]any{"op": "set_content", "at": ref, "if_match": rev, "text": "We use the widget each day."}},
	}, &res)
	require.False(t, isErr, body)
	require.Equal(t, change.SetPreviewed, res.Status, body)
	require.Len(t, res.Docs, 1)
	assert.Contains(t, res.Docs[0].Diff, "+We use the widget each day.")
	assert.Equal(t, before, fullTreeOf(t, root), "a preview leaves every file and directory of the project as it was")
	assert.NoDirExists(t, filepath.Join(root, project.StateDirName), "a preview creates no state directory, store or lock")
}
