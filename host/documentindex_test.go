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
