//go:build !js

package host

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/model"
)

// An agent translates a PO catalog in place as kapi apply does: apply_edits
// reads the catalog in the one language its operations name, and read_blocks
// in the one its editions name, so the msgstr the edit wrote reads back as
// that edition.
func TestMCPEditTools_TranslateABilingualCatalogInPlace(t *testing.T) {
	app := newToolboxApp(t)
	dir := t.TempDir()
	outsideAProject(t, dir)
	const po = "msgid \"\"\nmsgstr \"\"\n\"Content-Type: text/plain; charset=UTF-8\\n\"\n\nmsgid \"Hello\"\nmsgstr \"\"\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "messages.po"), []byte(po), 0o600))
	session := editSession(t, app, "edit-test")

	var page mcpPage
	isErr, body := callEditTool(t, session, "read_blocks", map[string]any{"doc": "messages.po", "editions": []string{"fr"}}, &page)
	require.False(t, isErr, body)
	ref, _, _ := page.blockWith(t, "Hello")

	set := map[string]any{"ops": []any{map[string]any{"op": "set_content",
		"at": map[string]any{"doc": "messages.po", "block": ref["block"], "edition": "fr"}, "if_match": model.AbsentRevision, "text": "Bonjour"}}}
	var res mcpResult
	isErr, body = callEditTool(t, session, "apply_edits", set, &res)
	require.False(t, isErr, body)
	assert.Equal(t, "applied", res.Status, body)
	assert.Contains(t, readFileIn(t, dir, "messages.po"), "msgid \"Hello\"\nmsgstr \"Bonjour\"\n")

	page = mcpPage{}
	isErr, body = callEditTool(t, session, "read_blocks", map[string]any{"doc": "messages.po", "editions": []string{"fr"}}, &page)
	require.False(t, isErr, body)
	require.Len(t, page.Blocks, 1, body)
	assert.Equal(t, "Bonjour", page.Blocks[0].Editions["fr"].Text, body)
}
