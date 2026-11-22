//go:build unix

package host

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/kpz"
)

// projectLinkedOut makes a project folder holding a link, at link (relative to
// the project), to a directory outside it. The outside directory holds
// secret.txt, which is returned with the project root.
func projectLinkedOut(t *testing.T, link string) (root, outside, secret string) {
	t.Helper()
	base := t.TempDir()
	root = filepath.Join(base, "project")
	outside = filepath.Join(base, "outside")
	require.NoError(t, os.MkdirAll(root, 0o755))
	require.NoError(t, os.MkdirAll(outside, 0o755))
	secret = filepath.Join(outside, "secret.txt")
	require.NoError(t, os.WriteFile(secret, []byte("outside the project"), 0o644))
	at := filepath.Join(root, filepath.FromSlash(link))
	require.NoError(t, os.MkdirAll(filepath.Dir(at), 0o755))
	require.NoError(t, os.Symlink(outside, at))
	return root, outside, secret
}

// assertOutsideUntouched checks that dir still holds only secret.txt, as written.
func assertOutsideUntouched(t *testing.T, dir, secret string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Len(t, entries, 1, "nothing is written outside the project")
	got, err := os.ReadFile(secret)
	require.NoError(t, err)
	assert.Equal(t, "outside the project", string(got))
}

// A source member restored under a directory that is a link out of the
// project is refused, and nothing lands outside.
func TestRestoreSourcesRefusesALinkOutOfTheProject(t *testing.T) {
	root, outside, secret := projectLinkedOut(t, "src")
	pkg := &kpz.Package{Source: []kpz.SourceDoc{{
		Path: kpz.SourceDir + "src/en.json", Content: kpz.BytesContent([]byte("PWNED")),
	}}}
	err := restoreSources(pkg, root)
	require.ErrorContains(t, err, "leads outside the root")
	assertOutsideUntouched(t, outside, secret)
}

// statInProject refuses a path through a link out of the project, answers
// fs.ErrNotExist for a missing file, and describes a file inside.
func TestStatInProjectRefusesALinkOutOfTheProject(t *testing.T) {
	root, _, _ := projectLinkedOut(t, "out")
	require.NoError(t, os.WriteFile(filepath.Join(root, "in.txt"), []byte("in"), 0o644))

	_, err := statInProject(root, filepath.Join(root, "out", "secret.txt"))
	require.ErrorContains(t, err, "leads outside the root")
	_, err = statInProject(root, filepath.Join(root, "..", "outside", "secret.txt"))
	require.Error(t, err, "a path that climbs out of the project")
	_, err = statInProject(root, filepath.Join(root, "missing.txt"))
	require.ErrorIs(t, err, os.ErrNotExist)
	info, err := statInProject(root, filepath.Join(root, "in.txt"))
	require.NoError(t, err)
	assert.Equal(t, int64(2), info.Size())

	assert.False(t, targetDelivered(root, filepath.Join(root, "out", "secret.txt")),
		"a target through a link out of the project is not delivered")
}

// A directory walk lists no file through a link that leads out of the
// directory it walks.
func TestWalkDirFilesSkipsALinkOutOfTheTree(t *testing.T) {
	root, outside, _ := projectLinkedOut(t, "out")
	require.NoError(t, os.WriteFile(filepath.Join(root, "in.xlf"), []byte("in"), 0o644))
	require.NoError(t, os.Symlink(filepath.Join(outside, "secret.txt"), filepath.Join(root, "file.xlf")))

	files, err := walkDirFiles(root)
	require.NoError(t, err)
	assert.Equal(t, []string{filepath.Join(root, "in.xlf")}, files)
}

// The committed target a record is digested from is read through the
// project root: a target that is a link out of the project is an error.
func TestRecordDigestRefusesATargetLinkOutOfTheProject(t *testing.T) {
	root, _, _ := projectLinkedOut(t, "fr")
	src := filepath.Join(root, "en.json")
	require.NoError(t, os.WriteFile(src, []byte(`{"a":"Hello"}`), 0o644))
	u := recordUnit{
		root:       root,
		sourcePath: src, sourceRel: "en.json",
		targetPath: filepath.Join(root, "fr", "secret.txt"), targetRel: "fr/secret.txt",
	}
	_, ok, err := recordDigest(u)
	require.ErrorContains(t, err, "leads outside the root")
	assert.False(t, ok)

	u.targetPath, u.targetRel = filepath.Join(root, "missing.json"), "missing.json"
	_, ok, err = recordDigest(u)
	require.NoError(t, err, "a target not written yet is pending work")
	assert.False(t, ok)
}

// A source a projection drift check finds through a link out of the project
// counts as gone, and is never read.
func TestSourceMovedTreatsALinkOutOfTheProjectAsGone(t *testing.T) {
	root, _, _ := projectLinkedOut(t, "src")
	got := sourceMoved(root, filepath.Join(root, "src", "secret.txt"), project.SourceStamp{})
	assert.Equal(t, sourceGone, got)
}

// kapi init writes no recipe through a kapi.yaml that is a link out of the
// folder: the link to a missing file is replaced by the recipe, and nothing
// is created where it pointed.
func TestInitProjectWritesNoRecipeThroughALink(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "project")
	outside := filepath.Join(base, "outside")
	require.NoError(t, os.MkdirAll(root, 0o755))
	require.NoError(t, os.MkdirAll(outside, 0o755))
	target := filepath.Join(outside, "kapi.yaml")
	recipe := filepath.Join(root, project.RecipeFileName)
	require.NoError(t, os.Symlink(target, recipe))

	_, err := InitProject(root, InitOptions{})
	require.NoError(t, err)
	assert.NoFileExists(t, target)
	info, err := os.Lstat(recipe)
	require.NoError(t, err)
	assert.True(t, info.Mode().IsRegular(), "the recipe is a file in the folder")
}

// A returned file names its redaction batch; a batch that is not one file
// name in the vault is refused rather than joined into a path.
func TestRestoreRedactedBlocksRefusesABatchThatIsAPath(t *testing.T) {
	root := t.TempDir()
	layout := project.LayoutAt(root)
	for _, batch := range []string{"../../../secret", "a/b", ".."} {
		err := restoreRedactedBlocks(layout, batch, "doc.json", nil, "fr", true, false)
		require.ErrorContains(t, err, "not usable as a path segment", batch)
	}
	require.NoError(t, restoreRedactedBlocks(layout, "", "doc.json", nil, "fr", true, false))
}
