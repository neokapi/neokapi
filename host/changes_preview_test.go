package host

import (
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/project"
)

// A preview computes and checks a change set and writes nothing: no document,
// no project store, no lock directory, no record. It reads the project store
// when the project has one, and creates none when it has not.

// fullTreeOf is every entry under root, the work directory included, a directory by its name and mode and a
// file by its mode and the digest of its bytes.
func fullTreeOf(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	require.NoError(t, filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if d.IsDir() {
			out[filepath.ToSlash(rel)+"/"] = info.Mode().String()
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(data)
		out[filepath.ToSlash(rel)] = info.Mode().String() + " " + hex.EncodeToString(sum[:])
		return nil
	}))
	return out
}

// freshProject is a project that has never been read or written by kapi: a
// recipe and one document, and no state directory.
func freshProject(t *testing.T) (root, recipe string) {
	t.Helper()
	isolateCheckExecution(t)
	root = t.TempDir()
	recipe = filepath.Join(root, project.RecipeFileName)
	require.NoError(t, os.WriteFile(recipe, []byte(strings.Replace(commitRecipe, "%s", projectIDFor("fresh"+t.Name()), 1)), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "docs"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "docs", "guide.md"), []byte("# Guide\n\nWe use the widget every day.\n"), 0o644))
	return root, recipe
}

// guideEdit is the change set that rewrites the guide's paragraph to text,
// from the revision a read of the project at recipe reports, in preview.
func guideEdit(t *testing.T, app *App, recipe, text string) string {
	t.Helper()
	p := guideParagraph(t, commitFixture{app: app, root: filepath.Dir(recipe), recipe: recipe})
	return changeSetOf(t, map[string]any{"op": "set_content", "at": map[string]any{"doc": "docs/guide.md", "block": p.Ref.Block}, "if_match": p.Rev, "text": text})
}

func TestApply_ADryRunInAFreshProjectWritesNothing(t *testing.T) {
	root, recipe := freshProject(t)
	app := &App{SourceLang: "en"}
	t.Cleanup(app.Shutdown)
	body := guideEdit(t, app, recipe, "We use the widget each day.")
	before := fullTreeOf(t, root)

	res, err := applyJSON(t, app, commitCommand(t, recipe), body, ApplyOptions{DryRun: true})
	require.NoError(t, err)
	require.Equal(t, change.SetPreviewed, res.Status, "%+v", res.Ops)
	require.Len(t, res.Docs, 1)
	assert.Contains(t, res.Docs[0].Diff, "+We use the widget each day.", "a preview shows what it would write")
	assert.Nil(t, res.Record)
	assert.Equal(t, before, fullTreeOf(t, root), "a dry run leaves every file and directory of the project as it was")
	assert.NoDirExists(t, filepath.Join(root, project.StateDirName), "a dry run creates no state directory, store or lock")
}

// A project that has a store is held to what the store holds in a preview as
// in a write: the preview refuses the edit the write would refuse, and writes
// nothing.
func TestApply_ADryRunReadsTheProjectsStore(t *testing.T) {
	f := newCommitFixture(t)
	locks := filepath.Join(f.root, ".kapi", "work", "locks")
	require.NoDirExists(t, locks)
	original := readFile(t, f.recipe, "docs/guide.md")
	body := guideEdit(t, f.app, f.recipe, "We utilize the widget every day.")

	cmd := f.command(t)
	res, err := applyJSON(t, f.app, cmd, body, ApplyOptions{DryRun: true})
	assert.Equal(t, ExitGate, ExitCode(cmd, err))
	require.Equal(t, change.SetRefused, res.Status, "%+v", res.Ops)
	require.NotNil(t, res.Ops[0].Error)
	assert.Equal(t, change.CodeGateFailed, res.Ops[0].Error.Code)
	assert.Contains(t, findingRules(docFindings(&res)), "terms.vocabulary", "the terms the store holds govern a preview")
	assert.Equal(t, original, readFile(t, f.recipe, "docs/guide.md"))
	assert.NoDirExists(t, locks, "a preview takes no lock")
	assert.Empty(t, editOps(t, f.app, f.root), "a preview records nothing")
}
