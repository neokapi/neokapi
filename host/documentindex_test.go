package host

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/reconcile"
	"github.com/neokapi/neokapi/core/state"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// renameProject is a project whose sources are matched by a glob, so moving a
// file leaves it in scope — which is what a rename is.
func renameProject(t *testing.T) (a *App, root, recipe string) {
	t.Helper()
	root = t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "src", "guides"), 0o755))

	proj := &project.KapiProject{
		Version: project.CurrentVersion,
		Name:    "RenameTest",
		Defaults: project.Defaults{
			SourceLanguage:  "en",
			TargetLanguages: []model.LocaleID{"nb"},
		},
		Collections: []project.Collection{
			{Name: "app", Path: "src/**/*.en.json", Target: "src/{path}.{lang}.json"},
		},
	}
	recipe = filepath.Join(root, project.RecipeFileName)
	require.NoError(t, project.Save(recipe, proj))

	a = &App{}
	a.InitRegistries()
	return a, root, recipe
}

// extractOnce runs the pre-pass every `kapi up` runs: it fills the block cache
// and, with it, resolves which document each source file is.
func extractOnce(t *testing.T, a *App, recipe string) {
	t.Helper()
	proj, err := project.Load(recipe)
	require.NoError(t, err)
	pctx := project.NewProjectContext(proj, recipe)
	resolved, rerr := pctx.ResolveContent(a.FormatReg)
	require.NoError(t, rerr)
	_, _, serr := a.syncProjectBlockStore(t.Context(), pctx, recipe, resolved)
	require.NoError(t, serr)
}

// TestARenamedDocumentKeepsItsApprovals is the defect. A decision is filed
// against the document it was made in; while that document's identity WAS its
// path, `git mv` detached every approval inside it — silently, since nothing
// failed and the next pass simply re-approved from scratch. The venue stopped
// doing this when it learned to reconcile a declared tree (#2134); this is the
// same fix on the local side.
func TestARenamedDocumentKeepsItsApprovals(t *testing.T) {
	a, root, recipe := renameProject(t)
	before := filepath.Join(root, "src", "guides", "intro.en.json")
	require.NoError(t, os.WriteFile(before, []byte(`{"greeting":"Hello world"}`), 0o644))

	extractOnce(t, a, recipe)

	docs, err := a.DocumentIndex(t.Context(), root)
	require.NoError(t, err)
	key := docs.Scope(root, before)
	require.NotEqual(t, "src/guides/intro.en.json", key,
		"extraction gives the document an identity that is not its address")

	// An approval, filed the way every surface files one.
	st, err := a.OpenProjectState(t.Context(), root)
	require.NoError(t, err)
	require.NoError(t, st.Put(t.Context(), state.UnitState{
		Scope:    key,
		Unit:     "greeting",
		Variant:  model.Variant("nb"),
		Status:   "approved",
		Decision: state.Decision{ReviewState: "approved", By: "reviewer"},
	}))

	// The file moves. Its contents do not.
	after := filepath.Join(root, "src", "intro.en.json")
	require.NoError(t, os.Rename(before, after))
	extractOnce(t, a, recipe)

	docs, err = a.DocumentIndex(t.Context(), root)
	require.NoError(t, err)
	assert.Equal(t, key, docs.Scope(root, after),
		"the renamed document is the same document")

	got, found := st.Get(t.Context(), state.Key{
		Scope: docs.Scope(root, after), Unit: "greeting", Variant: model.Variant("nb"),
	})
	require.True(t, found, "so the approval inside it survived the rename")
	assert.Equal(t, "reviewer", got.Decision.By)
}

// TestDocumentKeyNamesADocumentAsTheIndexDoes pins that the key a run asks
// for one document (documentKey, which a flow and the edit recorder ask per
// document) is the key the whole index gives the path, for a renamed
// document, one that never moved, and a path nobody read.
func TestDocumentKeyNamesADocumentAsTheIndexDoes(t *testing.T) {
	a, root, recipe := renameProject(t)
	t.Cleanup(a.Shutdown)
	before := filepath.Join(root, "src", "guides", "intro.en.json")
	require.NoError(t, os.WriteFile(before, []byte(`{"greeting":"Hello world"}`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "src", "setup.en.json"), []byte(`{"step":"Install it"}`), 0o644))
	extractOnce(t, a, recipe)
	require.NoError(t, os.Rename(before, filepath.Join(root, "src", "intro.en.json")))
	extractOnce(t, a, recipe)

	docs, err := a.DocumentIndex(t.Context(), root)
	require.NoError(t, err)
	tests := []struct{ name, rel string }{
		{"a renamed document", "src/intro.en.json"},
		{"a document that stayed", "src/setup.en.json"},
		{"the address a document left", "src/guides/intro.en.json"},
		{"a path nobody read", "src/new.en.json"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, docs.Key(tt.rel), a.documentKey(t.Context(), root, tt.rel))
		})
	}
	assert.Equal(t, reconcile.DocumentKeyFor("src/guides/intro.en.json"), a.documentKey(t.Context(), root, "src/intro.en.json"),
		"the renamed document keeps the key its first path gave it")
}

// TestASecondCheckoutNamesADocumentAsTheProjectDoes: one checkout follows a
// file through a rename and keeps its key. A second checkout of the project,
// made after the rename, has extracted nothing; it reads the key the first
// recorded rather than the one the new path derives to, so a decision filed by
// either answers in both.
func TestASecondCheckoutNamesADocumentAsTheProjectDoes(t *testing.T) {
	a, root, recipe := renameProject(t)
	a.SetWorkspaceRoot(filepath.Join(t.TempDir(), "workspace"))
	before := filepath.Join(root, "src", "guides", "intro.en.json")
	require.NoError(t, os.WriteFile(before, []byte(`{"greeting":"Hello world"}`), 0o644))
	extractOnce(t, a, recipe)
	after := filepath.Join(root, "src", "intro.en.json")
	require.NoError(t, os.Rename(before, after))
	extractOnce(t, a, recipe)
	docs, err := a.DocumentIndex(t.Context(), root)
	require.NoError(t, err)
	key := docs.Scope(root, after)
	require.Equal(t, reconcile.DocumentKeyFor("src/guides/intro.en.json"), key, "the first checkout kept the key")

	_, second, _ := renameProject(t)
	require.NoError(t, os.WriteFile(filepath.Join(second, "src", "intro.en.json"), []byte(`{"greeting":"Hello world"}`), 0o644))
	docs, err = a.DocumentIndex(t.Context(), second)
	require.NoError(t, err)
	assert.Equal(t, key, docs.Scope(second, filepath.Join(second, "src", "intro.en.json")),
		"the second checkout names the document by the project's key, not its new path")
}

// TestDocumentScopeDerivesTheKeyBeforeAnythingIsExtracted: a project whose store
// holds no identities names each file by the key its path derives to, which is
// the key the next extraction mints for it. Answering with the path instead made
// a fresh checkout unable to read its own committed record: every decision and
// every recorded basis in it is filed under a key, and a reader asking about
// paths matched none of them.
func TestDocumentScopeDerivesTheKeyBeforeAnythingIsExtracted(t *testing.T) {
	a, root, _ := renameProject(t)
	path := filepath.Join(root, "src", "guides", "intro.en.json")
	want := reconcile.DocumentKeyFor("src/guides/intro.en.json")

	docs, err := a.DocumentIndex(t.Context(), root)
	require.NoError(t, err)
	assert.Equal(t, want, docs.Scope(root, path),
		"nothing extracted yet, so the key is the one the path derives to")
	assert.Equal(t, want, DocumentIndex{}.Scope(root, path),
		"and the zero index answers the same way")
	assert.True(t, reconcile.IsDocumentKey(want))
}

// TestAMintedKeyHasNoAddressAlias: a record filed under an address answers
// under the key the address derives to, and a record filed under a key has
// no other spelling, the ordinal a second document at one path is minted
// with included.
func TestAMintedKeyHasNoAddressAlias(t *testing.T) {
	key := reconcile.DocumentKeyFor("docs/intro.md")
	assert.Equal(t, []string{key}, scopeAliases("docs/intro.md"))
	assert.Empty(t, scopeAliases(key))
	assert.Empty(t, scopeAliases(key+"-2"), "a minted key is a key, not an address")
}
