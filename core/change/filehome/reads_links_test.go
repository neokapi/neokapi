//go:build unix

package filehome_test

import (
	"archive/zip"
	"bytes"
	"context"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/change/filehome"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/rootpath"
)

// The digest a write is checked against is read through the root: a link out
// of it is refused, and a FIFO is refused without blocking the run.
func TestDigestReadsNothingOutOfTheRoot(t *testing.T) {
	root, _, secret := linkProject(t)
	home := filehome.New(nil, filehome.Options{Root: root})

	link := filepath.Join(root, "locales", "fr.json")
	require.NoError(t, os.Symlink(secret, link))
	_, err := home.Digest(link)
	require.ErrorContains(t, err, "leads outside the root")

	fifo := filepath.Join(root, "locales", "de.json")
	require.NoError(t, syscall.Mkfifo(fifo, 0o644))
	done := make(chan error, 1)
	go func() { _, err := home.Digest(fifo); done <- err }()
	select {
	case err := <-done:
		require.ErrorIs(t, err, rootpath.ErrNotRegular)
	case <-time.After(5 * time.Second):
		t.Fatal("the digest of a FIFO blocked")
	}
}

// A document opened while it was a file and replaced by a link out of the
// root before it is read is refused at the read.
func TestReadRefusesALinkSwappedInAfterOpen(t *testing.T) {
	root, _, secret := linkProject(t)
	require.NoError(t, os.WriteFile(secret, []byte(`{"hidden": "outside"}`), 0o644))
	doc := filepath.Join(root, "locales", "en.json")
	require.NoError(t, os.WriteFile(doc, []byte(`{"greeting": "Hello"}`), 0o644))
	home := filehome.New(filehome.DirLayout{Root: root, Formats: newRegistry(), SourceLocale: model.LocaleID("en")},
		filehome.Options{Root: root})
	sess, err := home.Open(context.Background(), "locales/en.json")
	require.NoError(t, err)
	defer sess.Close()

	require.NoError(t, os.Remove(doc))
	require.NoError(t, os.Symlink(secret, doc))
	var texts []string
	_, err = sess.Read(context.Background(), change.Want{}, func(b *model.Block) error {
		texts = append(texts, b.Key)
		return nil
	})
	require.ErrorContains(t, err, "leads outside the root")
	assert.Empty(t, texts, "no block of the outside file is read")
}

// An archive member is read from the archive the root holds: an archive
// replaced by a link out of the root is refused at the read.
func TestArchiveMemberReadRefusesALinkSwappedIn(t *testing.T) {
	root, outside, _ := linkProject(t)
	zipOf := func(path, body string) {
		var buf bytes.Buffer
		zw := zip.NewWriter(&buf)
		w, err := zw.Create("a.json")
		require.NoError(t, err)
		_, err = w.Write([]byte(body))
		require.NoError(t, err)
		require.NoError(t, zw.Close())
		require.NoError(t, os.WriteFile(path, buf.Bytes(), 0o644))
	}
	pack := filepath.Join(root, "pack.zip")
	zipOf(pack, `{"greeting": "Hello"}`)
	foreign := filepath.Join(outside, "other.zip")
	zipOf(foreign, `{"hidden": "outside"}`)
	home := filehome.New(filehome.DirLayout{Root: root, Formats: newRegistry(), SourceLocale: model.LocaleID("en")},
		filehome.Options{Root: root})
	sess, err := home.Open(context.Background(), "pack.zip!a.json")
	require.NoError(t, err)
	defer sess.Close()

	require.NoError(t, os.Remove(pack))
	require.NoError(t, os.Symlink(foreign, pack))
	var texts []string
	_, err = sess.Read(context.Background(), change.Want{}, func(b *model.Block) error {
		texts = append(texts, b.Key)
		return nil
	})
	require.ErrorContains(t, err, "leads outside the root")
	assert.Empty(t, texts)
}
