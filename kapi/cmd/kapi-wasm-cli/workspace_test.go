//go:build js && wasm

package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/kpz"
)

// A workspace package carries the files and each project's log out of the
// engine, and reading it back into a directory started over restores both:
// the files at their paths, and the project's context merged from its log.
// `make test-wasm-stores` runs it in Node.
func TestWorkspace_ExportThenImportRestoresFilesAndContext(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	proj := filepath.Join(root, "site")
	require.NoError(t, os.MkdirAll(filepath.Join(proj, "docs"), 0o755))
	recipe := "version: v1\nname: site\ndefaults:\n  source_language: en\n  target_languages: [fr]\n" +
		"collections:\n  - name: docs\n    content:\n      - path: \"docs/*.json\"\n"
	require.NoError(t, os.WriteFile(filepath.Join(proj, "kapi.yaml"), []byte(recipe), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(proj, "docs", "app.json"), []byte(`{"a":"Hello"}`), 0o644))

	options, err := json.Marshal(map[string]string{"project": proj})
	require.NoError(t, err)
	out, err := serveChange(app.ApplyChangesJSON,
		[]byte(`{"ops":[{"op":"term","action":"upsert","term":"handbook","locale":"en"}]}`), options)
	require.NoError(t, err)
	require.Contains(t, string(out), `"applied"`, string(out))

	engineMu.Lock()
	exported, err := exportWorkspace(ctx, root)
	engineMu.Unlock()
	require.NoError(t, err)
	assert.Equal(t, 2, exported.Files)
	require.Len(t, exported.Projects, 1)
	assert.Positive(t, exported.Projects[0].Operations)
	assert.Empty(t, exported.Skipped)
	data, err := exported.pkg.Marshal()
	require.NoError(t, err)
	pkg, err := kpz.Unmarshal(data)
	require.NoError(t, err)
	assert.Equal(t, kpz.KindWorkspace, pkg.Kind)

	// Read it into another browser: the files are gone, and so is the
	// workspace that held the project's log.
	require.NoError(t, resetInTurn(ctx, root))
	require.NoError(t, os.RemoveAll(proj))
	t.Setenv("KAPI_DATA_DIR", t.TempDir())

	engineMu.Lock()
	imported, err := importWorkspace(ctx, root, data)
	engineMu.Unlock()
	require.NoError(t, err)
	assert.Equal(t, 2, imported.Files)
	require.Len(t, imported.Projects, 1)
	assert.Positive(t, imported.Projects[0].Merged)
	got, err := os.ReadFile(filepath.Join(proj, "docs", "app.json"))
	require.NoError(t, err)
	assert.JSONEq(t, `{"a":"Hello"}`, string(got))

	// A second import holds nothing the log does not already hold.
	engineMu.Lock()
	again, err := importWorkspace(ctx, root, data)
	engineMu.Unlock()
	require.NoError(t, err)
	require.Len(t, again.Projects, 1)
	assert.Zero(t, again.Projects[0].Merged)
}

// A package of another kind is refused by name.
func TestWorkspace_ImportRefusesAnotherKind(t *testing.T) {
	data, err := (&kpz.Package{Kind: kpz.KindContext, Layout: []kpz.LayoutDoc{{Path: "log/w/1.jsonl", Data: []byte("{}\n")}}}).Marshal()
	require.NoError(t, err)
	engineMu.Lock()
	defer engineMu.Unlock()
	_, err = importWorkspace(context.Background(), t.TempDir(), data)
	assert.ErrorContains(t, err, "kapi-context")
}
