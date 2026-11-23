//go:build unix

package storage

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A database whose file, or a file kept beside it, is a symbolic link is
// refused before SQLite opens anything, and the file the link names is left
// as it was. FollowLink opens a store a person named through its link.
func TestOpenRefusesADatabaseThatIsALink(t *testing.T) {
	for _, suffix := range sqliteSidecars {
		dir := t.TempDir()
		other := filepath.Join(t.TempDir(), "other.db"+suffix)
		require.NoError(t, os.WriteFile(other, nil, 0o644))
		db := filepath.Join(dir, "store.db")
		require.NoError(t, os.Symlink(other, db+suffix))

		_, err := Open(db)
		require.ErrorContains(t, err, "is a symbolic link", "suffix %q", suffix)
		info, err := os.Stat(other)
		require.NoError(t, err)
		assert.Zero(t, info.Size(), "nothing is written through the link (suffix %q)", suffix)
	}

	named := filepath.Join(t.TempDir(), "named.db")
	link := filepath.Join(t.TempDir(), "link.db")
	require.NoError(t, os.Symlink(named, link))
	db, err := OpenWith(link, Options{FollowLink: true})
	require.NoError(t, err)
	require.NoError(t, db.Close())
	assert.FileExists(t, named, "a store a person named is opened through its link")
}
