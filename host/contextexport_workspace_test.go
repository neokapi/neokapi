package host

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/kpz"
)

// Backing up a whole workspace.
//
// A workspace holds the authored context of every project a machine account
// works on, one database each, outside every checkout and in no version
// control. The properties that make an archive of it a recovery story rather
// than a listing: it carries every registered project including the ones whose
// checkout is gone, it restores into an empty workspace under the identities it
// left with, and exporting what it restored gives the same bytes back.

// workspaceProject scaffolds one project in a workspace and seeds its context.
// It returns the project root and the recipe path.
func workspaceProject(t *testing.T, a *App, name string) (string, string) {
	t.Helper()
	root := t.TempDir()
	recipe := writePortableTree(t, root)

	// The recipe's `name:` is the project's identity in a workspace, so each
	// project scaffolded here carries its own.
	data, err := os.ReadFile(recipe)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(recipe,
		bytes.Replace(data, []byte("name: portable\n"), []byte("name: "+name+"\n"), 1), 0o644))

	seedPortable(t, a, root, recipe)
	return root, recipe
}

// newWorkspaceApp returns an App bound to one workspace directory, so every
// project it opens registers in the same one.
func newWorkspaceApp(t *testing.T, root string) *App {
	t.Helper()
	a := &App{SourceLang: "en"}
	a.InitRegistries()
	a.SetWorkspaceRoot(root)
	t.Cleanup(a.Shutdown)
	return a
}

// seededWorkspace builds a workspace holding two projects, and returns the
// workspace directory together with each project's root.
func seededWorkspace(t *testing.T) (wsDir string, roots []string) {
	t.Helper()
	wsDir = t.TempDir()
	a := newWorkspaceApp(t, wsDir)
	for _, name := range []string{"alpha", "beta"} {
		root, _ := workspaceProject(t, a, name)
		roots = append(roots, root)
	}
	// The handles are released so a second App over the same workspace opens
	// the files rather than queueing behind pools this one still holds.
	a.Shutdown()
	return wsDir, roots
}

// TestWorkspaceExport_CarriesEveryRegisteredProject: the archive holds one
// context package per project plus the registry that names them, and nothing
// about the machine it was written on.
func TestWorkspaceExport_CarriesEveryRegisteredProject(t *testing.T) {
	wsDir, _ := seededWorkspace(t)
	a := newWorkspaceApp(t, wsDir)
	ctx := context.Background()

	out := filepath.Join(t.TempDir(), "workspace.kpz")
	res, err := a.ExportWorkspaceContext(ctx, ContextWorkspaceExportRequest{Out: out})
	require.NoError(t, err)
	require.Len(t, res.Projects, 2)
	assert.Equal(t, "alpha", res.Projects[0].Key, "projects travel ordered by identity")
	assert.Equal(t, "beta", res.Projects[1].Key)
	assert.NotEmpty(t, res.RootHash)
	assert.Positive(t, res.Bytes)
	for _, p := range res.Projects {
		assert.Equal(t, 2, p.Concepts, "%s carries its terms", p.Key)
		assert.Equal(t, 2, p.VoiceProfiles, "%s carries its voice profiles", p.Key)
		assert.Positive(t, p.Entries, "%s carries its approved wording", p.Key)
		assert.Positive(t, p.Decisions, "%s carries its decisions", p.Key)
		assert.True(t, p.CheckedOut)
	}

	pkg, closer, err := kpz.OpenFile(out)
	require.NoError(t, err)
	defer func() { _ = closer.Close() }()
	assert.Equal(t, kpz.KindWorkspace, pkg.Kind)
	require.Len(t, pkg.Projects, 2)

	// Each member is a context package a restore of one project reads on its
	// own, and the machine the archive came from is nowhere in it.
	for _, doc := range pkg.Projects {
		body, rerr := kpz.ReadAll(doc.Content)
		require.NoError(t, rerr)
		inner, rerr := kpz.Unmarshal(body)
		require.NoError(t, rerr)
		assert.Equal(t, kpz.KindContext, inner.Kind)
		assert.NotEmpty(t, inner.Voice)
	}
	data, err := os.ReadFile(out)
	require.NoError(t, err)
	assert.NotContains(t, string(data), wsDir, "the workspace directory does not travel")
	assert.NotContains(t, string(data), "workspace_checkouts")
}

// TestWorkspaceExport_ReadsAProjectWithNoCheckoutHere: a project whose working
// tree has been deleted still has its context store in the workspace, and that
// is the case a whole-workspace backup exists for.
func TestWorkspaceExport_ReadsAProjectWithNoCheckoutHere(t *testing.T) {
	wsDir, roots := seededWorkspace(t)
	require.NoError(t, os.RemoveAll(roots[1]))

	a := newWorkspaceApp(t, wsDir)
	out := filepath.Join(t.TempDir(), "workspace.kpz")
	res, err := a.ExportWorkspaceContext(context.Background(), ContextWorkspaceExportRequest{Out: out})
	require.NoError(t, err)
	require.Len(t, res.Projects, 2, "a project with no checkout here still travels")

	byKey := map[string]ContextWorkspaceProject{}
	for _, p := range res.Projects {
		byKey[p.Key] = p
	}
	assert.True(t, byKey["alpha"].CheckedOut)
	assert.False(t, byKey["beta"].CheckedOut, "the checkout is gone and the context is not")
	assert.Equal(t, byKey["alpha"].Concepts, byKey["beta"].Concepts)
	assert.Equal(t, byKey["alpha"].Decisions, byKey["beta"].Decisions)

	var buf bytes.Buffer
	require.NoError(t, res.FormatText(&buf))
	assert.Contains(t, buf.String(), "no checkout here")
}

// TestWorkspaceBundle_RoundTripsByteForByte: exporting a workspace, restoring
// it into an empty one and exporting again yields the same archive.
//
// Nothing is normalized. The per-project timestamps #2879 names are written by
// the committed-record absorb, which reads a checkout's target documents; a
// workspace restore reads none, so what the bundle carried is what comes back.
func TestWorkspaceBundle_RoundTripsByteForByte(t *testing.T) {
	wsDir, _ := seededWorkspace(t)
	ctx := context.Background()

	a := newWorkspaceApp(t, wsDir)
	first := filepath.Join(t.TempDir(), "workspace.kpz")
	res, err := a.ExportWorkspaceContext(ctx, ContextWorkspaceExportRequest{Out: first})
	require.NoError(t, err)
	a.Shutdown()

	fresh := t.TempDir()
	b := newWorkspaceApp(t, fresh)
	restored, err := b.RestoreWorkspaceContext(ctx, ContextWorkspaceRestoreRequest{Bundle: first})
	require.NoError(t, err)
	require.Len(t, restored.Projects, 2)
	require.Positive(t, restored.Projects[0].Entries,
		"the round trip is only a claim about timestamps if wording travelled")
	for i, p := range restored.Projects {
		assert.Equal(t, res.Projects[i].Key, p.Key)
		assert.Equal(t, res.Projects[i].Name, p.Name)
		assert.Equal(t, res.Projects[i].Concepts, p.Concepts)
		assert.Equal(t, res.Projects[i].Entries, p.Entries)
		assert.Equal(t, res.Projects[i].VoiceProfiles, p.VoiceProfiles)
		assert.Equal(t, res.Projects[i].Decisions, p.Decisions)
	}

	second := filepath.Join(t.TempDir(), "workspace.kpz")
	again, err := b.ExportWorkspaceContext(ctx, ContextWorkspaceExportRequest{Out: second})
	require.NoError(t, err)
	assert.Equal(t, res.RootHash, again.RootHash, "the same context has the same content identity")

	firstBytes, err := os.ReadFile(first)
	require.NoError(t, err)
	secondBytes, err := os.ReadFile(second)
	require.NoError(t, err)
	assert.Equal(t, firstBytes, secondBytes, "a workspace bundle survives a restore byte for byte")
}

// TestWorkspaceRestore_RefusesAWorkspaceThatHoldsProjects covers the three
// modes: refuse by default, merge idempotently, replace on request.
func TestWorkspaceRestore_RefusesAWorkspaceThatHoldsProjects(t *testing.T) {
	wsDir, _ := seededWorkspace(t)
	ctx := context.Background()

	a := newWorkspaceApp(t, wsDir)
	bundle := filepath.Join(t.TempDir(), "workspace.kpz")
	res, err := a.ExportWorkspaceContext(ctx, ContextWorkspaceExportRequest{Out: bundle})
	require.NoError(t, err)

	_, err = a.RestoreWorkspaceContext(ctx, ContextWorkspaceRestoreRequest{Bundle: bundle})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--merge")
	assert.Contains(t, err.Error(), "--replace")

	merged, err := a.RestoreWorkspaceContext(ctx, ContextWorkspaceRestoreRequest{
		Bundle: bundle, Mode: RestoreMerge,
	})
	require.NoError(t, err)
	require.Len(t, merged.Projects, 2)
	assert.Empty(t, merged.Replaced)

	// Merging is idempotent: the workspace still exports to the same archive.
	after := filepath.Join(t.TempDir(), "after.kpz")
	again, err := a.ExportWorkspaceContext(ctx, ContextWorkspaceExportRequest{Out: after})
	require.NoError(t, err)
	assert.Equal(t, res.RootHash, again.RootHash, "restoring a workspace over itself changes nothing")
}

// TestWorkspaceRestore_ReplaceNamesWhatItEmptiesFirst: a replace destroys work,
// so it says which projects it is about to empty before it empties any of them.
func TestWorkspaceRestore_ReplaceNamesWhatItEmptiesFirst(t *testing.T) {
	wsDir, _ := seededWorkspace(t)
	ctx := context.Background()

	a := newWorkspaceApp(t, wsDir)
	bundle := filepath.Join(t.TempDir(), "workspace.kpz")
	_, err := a.ExportWorkspaceContext(ctx, ContextWorkspaceExportRequest{Out: bundle})
	require.NoError(t, err)

	var notice bytes.Buffer
	res, err := a.RestoreWorkspaceContext(ctx, ContextWorkspaceRestoreRequest{
		Bundle: bundle, Mode: RestoreReplace, Notice: &notice,
	})
	require.NoError(t, err)
	require.Len(t, res.Replaced, 2)
	assert.Equal(t, "alpha", res.Replaced[0].Key)
	assert.Equal(t, "beta", res.Replaced[1].Key)

	said := notice.String()
	assert.Contains(t, said, "Replacing the context of 2 projects")
	assert.Contains(t, said, wsDir, "and says which workspace it means")
	for _, p := range res.Replaced {
		assert.Contains(t, said, p.Key, "every project it emptied is named")
	}
	// The heading comes before the names, so a person reading the first line
	// knows what the list under it is.
	assert.Less(t, strings.Index(said, "Replacing"), strings.Index(said, "alpha"))

	require.Len(t, res.Projects, 2)
	assert.Equal(t, 2, res.Projects[0].Concepts, "the bundle went in after the clearing")
}

// TestWorkspaceRestore_RestoresAProjectWithNoCheckout: a project restored into
// an empty workspace is governed from the moment a checkout of it appears,
// without anything being imported from the checkout first.
func TestWorkspaceRestore_RestoresAProjectWithNoCheckout(t *testing.T) {
	wsDir, roots := seededWorkspace(t)
	ctx := context.Background()

	a := newWorkspaceApp(t, wsDir)
	bundle := filepath.Join(t.TempDir(), "workspace.kpz")
	_, err := a.ExportWorkspaceContext(ctx, ContextWorkspaceExportRequest{Out: bundle})
	require.NoError(t, err)
	a.Shutdown()

	fresh := t.TempDir()
	b := newWorkspaceApp(t, fresh)
	_, err = b.RestoreWorkspaceContext(ctx, ContextWorkspaceRestoreRequest{Bundle: bundle})
	require.NoError(t, err)

	// A clone of one project, holding no `.kapi/` context of its own, reads its
	// terms and voice profiles out of the restored workspace.
	clone := t.TempDir()
	cloneRecipe := writePortableTree(t, clone)
	data, err := os.ReadFile(filepath.Join(roots[0], project.RecipeFileName))
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(cloneRecipe, data, 0o644))
	require.NoError(t, os.RemoveAll(project.LayoutAt(clone).StateDir))
	require.NoError(t, os.MkdirAll(project.LayoutAt(clone).StateDir, 0o755))

	db, err := b.ProjectDB(ctx, clone)
	require.NoError(t, err)
	concepts, err := db.Terms().Concepts(ctx)
	require.NoError(t, err)
	assert.Len(t, concepts, 2, "a fresh checkout reads the restored terms")
	profiles, err := db.Voice().ListProfiles(ctx, LocalScope)
	require.NoError(t, err)
	assert.Len(t, profiles, 2, "a fresh checkout reads the restored voice profiles")
}

// TestWorkspaceExport_DryRunWritesNothing: the listing reports what an export
// would carry, which is how a person sees what they have before backing it up.
func TestWorkspaceExport_DryRunWritesNothing(t *testing.T) {
	wsDir, _ := seededWorkspace(t)
	a := newWorkspaceApp(t, wsDir)

	out := filepath.Join(t.TempDir(), "unwritten.kpz")
	res, err := a.ExportWorkspaceContext(context.Background(), ContextWorkspaceExportRequest{
		Out: out, DryRun: true,
	})
	require.NoError(t, err)
	require.Len(t, res.Projects, 2)
	assert.Empty(t, res.Path)
	assert.Empty(t, res.RootHash)
	assert.Zero(t, res.Bytes)
	assert.NoFileExists(t, out)
	for _, p := range res.Projects {
		assert.Empty(t, p.Bundle, "a listing writes no member")
		assert.Positive(t, p.Concepts)
	}

	var buf bytes.Buffer
	require.NoError(t, res.FormatText(&buf))
	assert.Contains(t, buf.String(), "holds 2 projects")
	assert.NotContains(t, buf.String(), "Wrote")

	// A listing needs no destination, and an export does.
	_, err = a.ExportWorkspaceContext(context.Background(), ContextWorkspaceExportRequest{DryRun: true})
	require.NoError(t, err)
	_, err = a.ExportWorkspaceContext(context.Background(), ContextWorkspaceExportRequest{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "-o")
}

// TestWorkspaceBundle_RefusesTheOtherProfile: the two restores read different
// archives, and each says which one it wanted.
func TestWorkspaceBundle_RefusesTheOtherProfile(t *testing.T) {
	wsDir, roots := seededWorkspace(t)
	ctx := context.Background()
	a := newWorkspaceApp(t, wsDir)

	recipe := filepath.Join(roots[0], project.RecipeFileName)
	single := filepath.Join(t.TempDir(), "context.kpz")
	_, err := a.ExportProjectContext(ctx, recipe, single)
	require.NoError(t, err)

	whole := filepath.Join(t.TempDir(), "workspace.kpz")
	_, err = a.ExportWorkspaceContext(ctx, ContextWorkspaceExportRequest{Out: whole})
	require.NoError(t, err)

	_, err = a.RestoreWorkspaceContext(ctx, ContextWorkspaceRestoreRequest{Bundle: single})
	require.Error(t, err)
	assert.Contains(t, err.Error(), kpz.KindContext)
	assert.Contains(t, err.Error(), "--workspace")

	_, err = a.RestoreProjectContext(ctx, recipe, whole, RestoreMerge)
	require.Error(t, err)
	assert.Contains(t, err.Error(), kpz.KindWorkspace)
}

// TestWorkspaceExport_RefusesAnEmptyWorkspace: a workspace nothing has been
// opened in says so rather than writing an archive of nothing.
func TestWorkspaceExport_RefusesAnEmptyWorkspace(t *testing.T) {
	a := newWorkspaceApp(t, t.TempDir())
	_, err := a.ExportWorkspaceContext(context.Background(), ContextWorkspaceExportRequest{
		Out: filepath.Join(t.TempDir(), "workspace.kpz"),
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "holds no projects")
}
