//go:build unix

package project

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// State that arrives in a project after kapi looked and found no state
// directory, with files newer than that look (an archive unpacked, a sync
// client, a checkout finishing), is unmarked like any other: the MAC alone
// decides, never a file's time.
func TestStateThatArrivesAfterKapiLookedIsUnmarked(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, RecipeFileName), []byte("version: v1\nname: p\n"), 0o644))
	l := LayoutAt(root)
	require.NoError(t, CheckStateOwned(t.Context(), l), "no state directory yet")

	require.NoError(t, os.MkdirAll(l.VaultDir(), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(l.VaultDir(), "redaction.json"), []byte(`{"planted":"original"}`), 0o600))
	var unverified *UnverifiedStateError
	require.ErrorAs(t, CheckStateOwned(t.Context(), l), &unverified)
	assert.NoFileExists(t, filepath.Join(l.StateDir, MarkFilename), "no mark is written for state kapi did not create")
}

// The origin a mark records came with the state, from wherever that was: it
// is cleaned where it is read, capped, and quoted where it is shown.
func TestTheRecordedOriginIsShownSafely(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, RecipeFileName), []byte("version: v1\nname: p\n"), 0o644))
	l := LayoutAt(root)
	require.NoError(t, os.MkdirAll(l.VaultDir(), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(l.VaultDir(), "redaction.json"), []byte("{}"), 0o600))
	hostile := "evil\x1b[2J\x1b]0;title\x07\u202e" + strings.Repeat("x", 5000)
	data, err := json.Marshal(stateMark{Version: markVersion, ID: "00", MAC: "00", Host: hostile, Path: "/elsewhere\n", Created: "today"})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(l.StateDir, MarkFilename), data, 0o600))

	err = CheckStateOwned(t.Context(), l)
	var unverified *UnverifiedStateError
	require.ErrorAs(t, err, &unverified)
	msg := err.Error()
	assert.NotContains(t, msg, "\x1b", "no control character reaches the message")
	assert.NotContains(t, msg, "\u202e", "no invisible format character reaches the message")
	assert.Less(t, len(unverified.Origin), 700, "the origin is capped")
	assert.Contains(t, msg, `It appears to come from "host evil`, "the origin is quoted")

	origin, err := DescribeState(l)
	require.NoError(t, err)
	assert.Equal(t, unverified.Origin, origin.Origin, "every reader gets the cleaned value")
}

// A state directory kapi creates carries its mark from the moment it exists:
// it is made with the mark in a staged directory and renamed into place, and
// nothing staged is left behind.
func TestAStateDirectoryKapiCreatesExistsOnlyWithItsMark(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, RecipeFileName), []byte("version: v1\nname: p\n"), 0o644))
	l := LayoutAt(root)
	require.NoError(t, EnsureLayout(l))
	key, err := stateKey()
	require.NoError(t, err)
	mark, err := readMark(l)
	require.NoError(t, err)
	assert.True(t, markValid(key, l, mark))
	entries, err := os.ReadDir(root)
	require.NoError(t, err)
	for _, e := range entries {
		assert.NotContains(t, e.Name(), ".creating-", "no staged directory is left")
	}

	// A layout call refuses an existing state directory with state and no mark,
	// and leaves it unmarked.
	other := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(other, RecipeFileName), []byte("version: v1\nname: p\n"), 0o644))
	ol := LayoutAt(other)
	require.NoError(t, os.MkdirAll(ol.SyncDir(), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(ol.SyncDir(), "sync-cache.json"), []byte("{}"), 0o600))
	require.Error(t, EnsureLayout(ol))
	assert.NoFileExists(t, filepath.Join(ol.StateDir, MarkFilename))
}
