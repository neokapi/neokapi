//go:build unix

package filehome_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/change/filehome"
	"github.com/neokapi/neokapi/core/formats"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/registry"
)

// linkProject makes a project root and, beside it, a directory outside it
// holding one file the home must never write.
func linkProject(t *testing.T) (root, outside, secret string) {
	t.Helper()
	base := t.TempDir()
	root = filepath.Join(base, "project")
	outside = filepath.Join(base, "outside")
	require.NoError(t, os.MkdirAll(filepath.Join(root, "locales"), 0o755))
	require.NoError(t, os.MkdirAll(outside, 0o755))
	secret = filepath.Join(outside, "secret.json")
	require.NoError(t, os.WriteFile(secret, []byte(`{"kept":true}`), 0o644))
	return root, outside, secret
}

func produce(t *testing.T, home *filehome.Home, path string) error {
	t.Helper()
	before, err := filehome.Digest(path)
	if err != nil {
		return err
	}
	p, err := home.Produce(context.Background(), path, before, produceBytes(`{"fr":true}`))
	if err != nil {
		return err
	}
	defer func() { _ = p.Release() }()
	return p.Commit(context.Background())
}

func TestProduceRefusesALinkOutOfTheRoot(t *testing.T) {
	t.Run("a target file that links out", func(t *testing.T) {
		root, _, secret := linkProject(t)
		link := filepath.Join(root, "locales", "fr.json")
		require.NoError(t, os.Symlink(secret, link))
		err := produce(t, filehome.New(nil, filehome.Options{Root: root}), link)
		require.ErrorContains(t, err, "locales/fr.json")
		b, _ := os.ReadFile(secret)
		assert.JSONEq(t, `{"kept":true}`, string(b))
	})

	t.Run("a new file beneath a directory that links out", func(t *testing.T) {
		root, outside, _ := linkProject(t)
		require.NoError(t, os.Symlink(outside, filepath.Join(root, "site")))
		err := produce(t, filehome.New(nil, filehome.Options{Root: root}), filepath.Join(root, "site", "fr", "app.json"))
		require.ErrorContains(t, err, "the symbolic link site leads outside the root")
		assert.NoDirExists(t, filepath.Join(outside, "fr"))
	})

	t.Run("a link that stays inside the root is written through", func(t *testing.T) {
		root, _, _ := linkProject(t)
		require.NoError(t, os.MkdirAll(filepath.Join(root, "real"), 0o755))
		require.NoError(t, os.Symlink("real", filepath.Join(root, "fr")))
		require.NoError(t, produce(t, filehome.New(nil, filehome.Options{Root: root}), filepath.Join(root, "fr", "app.json")))
		assert.FileExists(t, filepath.Join(root, "real", "app.json"))
	})

	t.Run("a root reached through a link", func(t *testing.T) {
		root, _, _ := linkProject(t)
		via := filepath.Join(t.TempDir(), "via")
		require.NoError(t, os.Symlink(root, via))
		require.NoError(t, produce(t, filehome.New(nil, filehome.Options{Root: via}), filepath.Join(via, "locales", "de.json")))
		assert.FileExists(t, filepath.Join(root, "locales", "de.json"))
	})

	t.Run("a path a person named outside the root is written as named", func(t *testing.T) {
		root, outside, _ := linkProject(t)
		out := filepath.Join(outside, "export.json")
		home := filehome.New(nil, filehome.Options{Root: root})
		home.PermitOutside(out)
		require.NoError(t, produce(t, home, out))
		assert.FileExists(t, out)
	})

	t.Run("a path outside the root that no person named is refused", func(t *testing.T) {
		root, outside, _ := linkProject(t)
		out := filepath.Join(root, "..", "outside", "fr.json")
		err := produce(t, filehome.New(nil, filehome.Options{Root: root}), out)
		require.ErrorContains(t, err, "lies outside the project")
		assert.NoFileExists(t, filepath.Join(outside, "fr.json"))
	})

	t.Run("a link put in place while the run worked is refused at the commit", func(t *testing.T) {
		root, outside, _ := linkProject(t)
		home := filehome.New(nil, filehome.Options{Root: root})
		target := filepath.Join(root, "site", "fr.json")
		p, err := home.Produce(context.Background(), target, "", produceBytes(`{"fr":true}`))
		require.NoError(t, err)
		defer func() { _ = p.Release() }()
		require.NoError(t, os.Symlink(outside, filepath.Join(root, "site")))
		err = p.Commit(context.Background())
		require.ErrorContains(t, err, "leads outside the root")
		assert.NoFileExists(t, filepath.Join(outside, "fr.json"))
	})
}

// A link put in place after the last containment check, between the check and
// the rename, is refused by the rename: it resolves its names inside the root.
func TestCommitRefusesALinkSwappedInAfterTheCheck(t *testing.T) {
	root, outside, _ := linkProject(t)
	home := filehome.New(nil, filehome.Options{Root: root})
	target := filepath.Join(root, "locales", "fr", "app.json")
	p, err := home.Produce(context.Background(), target, "", produceBytes(`{"fr":true}`))
	require.NoError(t, err)
	defer func() { _ = p.Release() }()

	filehome.SetBeforeCommit(func(string) {
		require.NoError(t, os.Symlink(outside, filepath.Join(root, "locales", "fr")))
	})
	t.Cleanup(func() { filehome.SetBeforeCommit(nil) })

	err = p.Commit(context.Background())
	require.ErrorContains(t, err, "leads outside the root")
	assert.NoFileExists(t, filepath.Join(outside, "app.json"), "nothing is written through the link")
}

// A change set reads and writes its documents through the layout, which
// refuses a document that a link leads out of the root.
func TestChangeRefusesALinkOutOfTheRoot(t *testing.T) {
	root, _, secret := linkProject(t)
	require.NoError(t, os.Symlink(secret, filepath.Join(root, "locales", "en.json")))
	reg := registry.NewFormatRegistry()
	formats.RegisterAll(reg)
	home := filehome.New(filehome.DirLayout{Root: root, Formats: reg, SourceLocale: model.LocaleID("en")}, filehome.Options{})
	_, err := home.Open(context.Background(), "locales/en.json")
	var ce *change.Error
	require.ErrorAs(t, err, &ce)
	assert.Equal(t, change.CodeInvalid, ce.Code)
	assert.Contains(t, ce.Message, "locales/en.json")
	assert.NotContains(t, ce.Message, "secret.json")
}

// applyGreeting applies one set_content to doc in the fixture.
func applyGreeting(t *testing.T, f *fixture, doc string) *change.Result {
	t.Helper()
	text := "Hi"
	set := change.Set{Mode: change.ModeApply, Gate: change.GateEnforce, Ops: []change.Op{{
		Kind: change.KindSetContent, At: change.Ref{Doc: doc, Block: "greeting"}, IfMatch: change.AnyRevision,
		Body: &change.SetContent{Text: &text},
	}}}
	res, err := f.svc.Apply(context.Background(), set, person)
	require.NoError(t, err)
	return res
}

// The backup copy is staged and renamed into place: a copy name that is a
// hard link to a file outside the root replaces that name, and the outside
// file keeps its bytes.
func TestBackupNeverWritesThroughAHardLink(t *testing.T) {
	const src = `{"greeting": "Hello"}` + "\n"
	f := newFixture(t, map[string]string{"a.json": src}, filehome.Options{BackupSuffix: ".bak"})
	outside := filepath.Join(t.TempDir(), "victim.json")
	require.NoError(t, os.WriteFile(outside, []byte("kept"), 0o644))
	require.NoError(t, os.Link(outside, filepath.Join(f.dir, "a.json.bak")))

	res := applyGreeting(t, f, "a.json")
	require.Equal(t, change.SetApplied, res.Status, "%+v", res.Ops[0].Error)
	b, _ := os.ReadFile(outside)
	assert.Equal(t, "kept", string(b), "the file the hard link shared is untouched")
	assert.Equal(t, src, f.read(t, "a.json.bak"))
}

// A directory link put in place in the change-set commit, after the last
// check and before the rename, is refused, and nothing lands outside.
func TestChangeCommitRefusesALinkSwappedInAfterTheCheck(t *testing.T) {
	f := newFixture(t, map[string]string{"locales/a.json": `{"greeting": "Hello"}` + "\n"})
	outside := t.TempDir()
	filehome.SetBeforeCommit(func(string) {
		require.NoError(t, os.Rename(filepath.Join(f.dir, "locales"), filepath.Join(f.dir, "locales.moved")))
		require.NoError(t, os.Symlink(outside, filepath.Join(f.dir, "locales")))
	})
	t.Cleanup(func() { filehome.SetBeforeCommit(nil) })

	res := applyGreeting(t, f, "locales/a.json")
	assert.NotEqual(t, change.SetApplied, res.Status)
	entries, err := os.ReadDir(outside)
	require.NoError(t, err)
	assert.Empty(t, entries, "nothing is written outside the root")
}

// On a case-insensitive volume a path spelled with another case is still
// beneath the root, and a link out of the root under it is refused.
func TestProduceRefusesALinkOutOfTheRootUnderAnotherCase(t *testing.T) {
	root, outside, _ := linkProject(t)
	upper := filepath.Join(filepath.Dir(root), "PROJECT")
	if _, err := os.Stat(upper); err != nil {
		t.Skip("the volume is case-sensitive")
	}
	require.NoError(t, os.RemoveAll(filepath.Join(root, "locales")))
	require.NoError(t, os.Symlink(outside, filepath.Join(root, "locales")))
	err := produce(t, filehome.New(nil, filehome.Options{Root: root}), filepath.Join(upper, "locales", "fr.json"))
	require.Error(t, err)
	assert.NoFileExists(t, filepath.Join(outside, "fr.json"))
}
