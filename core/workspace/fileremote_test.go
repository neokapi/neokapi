//go:build !js && !windows

package workspace_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/workspace"
)

// segment is a name in the layout's log directory.
var segment = workspace.SegmentName("w1", strings.Repeat("0", 24))

// A link another writer plants inside the shared folder leads neither a read
// nor a write out of it.
func TestFileRemote_ALinkInsideTheFolderStaysInside(t *testing.T) {
	outside := t.TempDir()
	share := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(outside, "secret"), []byte("outside"), 0o600))

	t.Run("a read through a planted link", func(t *testing.T) {
		require.NoError(t, os.MkdirAll(filepath.Join(share, "log", "w1"), 0o755))
		planted := filepath.Join(share, filepath.FromSlash(segment))
		require.NoError(t, os.Symlink(filepath.Join(outside, "secret"), planted))
		t.Cleanup(func() { _ = os.Remove(planted) })

		r := workspace.NewFileRemote(share)
		names, err := r.List(t.Context(), workspace.RemoteLogDir)
		require.NoError(t, err)
		assert.Empty(t, names, "a link is not an object")
		_, err = r.Get(t.Context(), segment)
		require.Error(t, err, "the link is not followed")
	})

	t.Run("a write through a planted directory link", func(t *testing.T) {
		require.NoError(t, os.RemoveAll(filepath.Join(share, "log")))
		require.NoError(t, os.Symlink(outside, filepath.Join(share, "log")))

		r := workspace.NewFileRemote(share)
		err := r.Put(t.Context(), workspace.Object{Name: segment, Data: []byte("{}\n")})
		require.Error(t, err)
		assert.NoDirExists(t, filepath.Join(outside, "w1"), "nothing is written outside the folder")
	})
}

// The folder is opened once: replacing its path with a link afterwards does
// not move the remote.
func TestFileRemote_AFolderSwappedAfterTheOpenIsNotFollowed(t *testing.T) {
	parent := t.TempDir()
	outside := t.TempDir()
	share := filepath.Join(parent, "share")
	require.NoError(t, os.Mkdir(share, 0o755))

	r := workspace.NewFileRemote(share)
	t.Cleanup(func() { _ = r.Close() })
	_, err := r.List(t.Context(), workspace.RemoteLogDir)
	require.NoError(t, err)

	require.NoError(t, os.Rename(share, filepath.Join(parent, "moved")))
	require.NoError(t, os.Symlink(outside, share))
	require.NoError(t, r.Put(t.Context(), workspace.Object{Name: segment, Data: []byte("{}\n")}))

	assert.NoDirExists(t, filepath.Join(outside, "log"), "the write went to the folder that was opened")
	assert.FileExists(t, filepath.Join(parent, "moved", filepath.FromSlash(segment)))
}

// A folder replaced between resolving its path and opening it is refused:
// the remote opens only the directory it was told to expect.
func TestFileRemote_AFolderReplacedBeforeTheOpenIsRefused(t *testing.T) {
	parent := t.TempDir()
	share := filepath.Join(parent, "share")
	require.NoError(t, os.Mkdir(share, 0o755))
	approved, err := os.Stat(share)
	require.NoError(t, err)

	require.NoError(t, os.Rename(share, filepath.Join(parent, "approved")))
	require.NoError(t, os.Mkdir(share, 0o755))

	r := workspace.NewFileRemote(share).Expect(approved)
	_, err = r.List(t.Context(), workspace.RemoteLogDir)
	require.ErrorIs(t, err, workspace.ErrRemoteReplaced)
	err = r.Put(t.Context(), workspace.Object{Name: segment, Data: []byte("{}\n")})
	require.ErrorIs(t, err, workspace.ErrRemoteReplaced)
	assert.NoDirExists(t, filepath.Join(share, "log"), "nothing is written to the directory swapped in")

	same := workspace.NewFileRemote(filepath.Join(parent, "approved")).Expect(approved)
	_, err = same.List(t.Context(), workspace.RemoteLogDir)
	require.NoError(t, err, "the directory that was approved opens")
}

// What a listing passes over is reported, so a sync can say why it pulled
// nothing from a folder holding a link.
func TestFileRemote_WhatAListingPassesOverIsReported(t *testing.T) {
	share := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(share, "log", "w1"), 0o755))
	require.NoError(t, os.Symlink(filepath.Join(t.TempDir(), "elsewhere"), filepath.Join(share, filepath.FromSlash(segment))))

	r := workspace.NewFileRemote(share)
	names, err := r.List(t.Context(), workspace.RemoteLogDir)
	require.NoError(t, err)
	assert.Empty(t, names)
	names, total := r.TakePassedOver()
	assert.Equal(t, []string{segment}, names)
	assert.Equal(t, 1, total)
	names, total = r.TakePassedOver()
	assert.Empty(t, names, "taken once")
	assert.Zero(t, total)
}

// A folder can hold any number of entries kapi passes over; the remote keeps
// the first few names and counts the rest.
func TestFileRemote_ThePassedOverNamesAreCapped(t *testing.T) {
	share := t.TempDir()
	dir := filepath.Join(share, "log", "w1")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	target := filepath.Join(t.TempDir(), "elsewhere")
	for i := range 200 {
		require.NoError(t, os.Symlink(target, filepath.Join(dir, fmt.Sprintf("link-%03d", i))))
	}

	r := workspace.NewFileRemote(share)
	_, err := r.List(t.Context(), workspace.RemoteLogDir)
	require.NoError(t, err)
	names, total := r.TakePassedOver()
	assert.Len(t, names, workspace.PassedOverShown)
	assert.Equal(t, 200, total)
}
