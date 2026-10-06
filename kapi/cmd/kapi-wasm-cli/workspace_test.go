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

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/storage"
	"github.com/neokapi/neokapi/kpz"
	"github.com/neokapi/neokapi/terms"
)

// A workspace package carries the files and each project's log out of the
// engine, and reading it back into a directory started over restores both:
// the files at their paths, and the project's context merged from its log.
// `make test-wasm-stores` runs it in Node.
func TestWorkspace_ExportThenImportRestoresFilesAndContext(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	t.Cleanup(func() {
		_ = resetInTurn(context.Background(), root)
		_ = storage.RemoveAll(root)
	})
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

// A reset forgets the project in the workspace, and the log keeps the
// operations that built its context for history. Importing the package the
// workspace exported before the reset, in the same browser, brings that
// context back: the operations held only before the removal count as not
// held, and are recorded and applied again.
func TestWorkspace_ImportAfterResetRestoresContext(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	t.Cleanup(func() {
		_ = resetInTurn(context.Background(), root)
		_ = storage.RemoveAll(root)
	})
	proj := filepath.Join(root, "site")
	require.NoError(t, os.MkdirAll(filepath.Join(proj, "docs"), 0o755))
	recipe := "version: v1\nname: site\ndefaults:\n  source_language: en\n  target_languages: [fr]\n" +
		"collections:\n  - name: docs\n    content:\n      - path: \"docs/*.json\"\n"
	require.NoError(t, os.WriteFile(filepath.Join(proj, "kapi.yaml"), []byte(recipe), 0o644))
	options, err := json.Marshal(map[string]string{"project": proj})
	require.NoError(t, err)
	out, err := serveChange(app.ApplyChangesJSON,
		[]byte(`{"ops":[{"op":"term","action":"upsert","term":"handbook","locale":"en"}]}`), options)
	require.NoError(t, err)
	require.Contains(t, string(out), `"applied"`, string(out))
	assert.Equal(t, 1, projectTerms(t, proj))

	engineMu.Lock()
	exported, err := exportWorkspace(ctx, root)
	engineMu.Unlock()
	require.NoError(t, err)
	data, err := exported.pkg.Marshal()
	require.NoError(t, err)

	require.NoError(t, resetInTurn(ctx, root))
	require.NoError(t, os.RemoveAll(proj))

	engineMu.Lock()
	imported, err := importWorkspace(ctx, root, data)
	engineMu.Unlock()
	require.NoError(t, err)
	require.Len(t, imported.Projects, 1)
	assert.Positive(t, imported.Projects[0].Merged, "the operations the reset forgot are merged again")
	assert.Equal(t, 1, projectTerms(t, proj), "the term is back in the project's store")

	// Importing it once more adds nothing.
	engineMu.Lock()
	again, err := importWorkspace(ctx, root, data)
	engineMu.Unlock()
	require.NoError(t, err)
	require.Len(t, again.Projects, 1)
	assert.Zero(t, again.Projects[0].Merged)
	assert.Equal(t, 1, projectTerms(t, proj))
}

// projectTerms counts the concepts in the project's terms store.
func projectTerms(t *testing.T, proj string) int {
	t.Helper()
	engineMu.Lock()
	defer engineMu.Unlock()
	db, err := app.ProjectDB(context.Background(), proj)
	require.NoError(t, err)
	n, err := db.Terms().Count(context.Background())
	require.NoError(t, err)
	return n
}

// A terms store outside every project travels as a terms bundle and comes
// back at its path; the stores inside a project's state directory and the
// workspace's own do not travel, since their projects' logs carry them.
func TestWorkspace_ExportCarriesATermStoreOutsideProjects(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	t.Cleanup(func() {
		_ = resetInTurn(context.Background(), root)
		_ = storage.RemoveAll(root)
	})
	proj := filepath.Join(root, "site")
	require.NoError(t, os.MkdirAll(filepath.Join(proj, "docs"), 0o755))
	recipe := "version: v1\nname: site\ndefaults:\n  source_language: en\n  target_languages: [fr]\n" +
		"collections:\n  - name: docs\n    content:\n      - path: \"docs/*.json\"\n"
	require.NoError(t, os.WriteFile(filepath.Join(proj, "kapi.yaml"), []byte(recipe), 0o644))
	options, err := json.Marshal(map[string]string{"project": proj})
	require.NoError(t, err)
	_, err = serveChange(app.ApplyChangesJSON,
		[]byte(`{"ops":[{"op":"term","action":"upsert","term":"handbook","locale":"en"}]}`), options)
	require.NoError(t, err)

	store := filepath.Join(root, "shared", "terms.db")
	require.NoError(t, os.MkdirAll(filepath.Dir(store), 0o755))
	tb, err := terms.NewSQLiteStore(store)
	require.NoError(t, err)
	require.NoError(t, tb.AddConcept(ctx, terms.Concept{
		ID:    "berth",
		Terms: []terms.Term{{Text: "berth", Locale: model.LocaleEnglish, Status: model.TermPreferred}},
	}))
	require.NoError(t, tb.Close())

	engineMu.Lock()
	exported, err := exportWorkspace(ctx, root)
	engineMu.Unlock()
	require.NoError(t, err)
	require.Len(t, exported.TermStores, 1, "only the store outside the project travels as a store")
	assert.Equal(t, store, exported.TermStores[0].Store)
	assert.Equal(t, 1, exported.TermStores[0].Concepts)
	data, err := exported.pkg.Marshal()
	require.NoError(t, err)

	require.NoError(t, storage.Remove(store))
	held, err := storage.Exists(store)
	require.NoError(t, err)
	require.False(t, held)

	engineMu.Lock()
	imported, err := importWorkspace(ctx, root, data)
	engineMu.Unlock()
	require.NoError(t, err)
	require.Len(t, imported.TermStores, 1)
	assert.Equal(t, 1, imported.TermStores[0].Concepts)

	tb, err = terms.NewSQLiteStore(store)
	require.NoError(t, err)
	defer func() { _ = tb.Close() }()
	c, ok, err := tb.GetConcept(ctx, "berth")
	require.NoError(t, err)
	require.True(t, ok, "the concept is back in the store at its path")
	require.Len(t, c.Terms, 1)
	assert.Equal(t, "berth", c.Terms[0].Text)
}
