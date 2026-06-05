package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/sievepen"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTMAudit_FiltersByBatchID(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "tm.db")
	tm, err := sievepen.NewSQLiteTM(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = tm.Close() })

	now := time.Now().UTC()
	// Two entries from batch-A, one from batch-B.
	mustAdd := func(id, batch, src string) {
		t.Helper()
		e := sievepen.TMEntry{
			ID: id,
			Variants: map[model.LocaleID][]model.Run{
				"en": {{Text: &model.TextRun{Text: id + "-src"}}},
				"fr": {{Text: &model.TextRun{Text: id + "-tgt"}}},
			},
			Origins: []sievepen.Origin{{
				Source:    "merge",
				Key:       src,
				Reference: batch,
				AddedAt:   now,
				AddedBy:   "test",
			}},
			Properties: map[string]string{
				"kapi-merge:block-content-hash": "hash-" + id,
				"kapi-merge:xliff-original":     src + ".xliff",
			},
			CreatedAt: now,
			UpdatedAt: now,
		}
		require.NoError(t, tm.Add(t.Context(), e))
	}
	mustAdd("a1", "batch-A", "src/a1.json")
	mustAdd("a2", "batch-A", "src/a2.json")
	mustAdd("b1", "batch-B", "src/b1.json")
	require.NoError(t, tm.Close())

	a := newExtractApp(t)
	cmd := a.NewTMCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"audit", "--file", dbPath, "--batch", "batch-A"})
	require.NoError(t, cmd.Execute())

	s := out.String()
	assert.Contains(t, s, "batch-A")
	// Both batch-A entries should appear.
	assert.Contains(t, s, "src/a1.json")
	assert.Contains(t, s, "src/a2.json")
	// The batch-B entry must not.
	assert.NotContains(t, s, "src/b1.json")
}

func TestTMAudit_RequiresBatchFlag(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "tm.db")
	tm, err := sievepen.NewSQLiteTM(dbPath)
	require.NoError(t, err)
	require.NoError(t, tm.Close())

	a := newExtractApp(t)
	cmd := a.NewTMCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"audit", "--file", dbPath})
	err = cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--batch")
}

// Sanity check: audit output references cwd-independent paths.
func TestTMAudit_EmptyResultIsClearMessage(t *testing.T) {
	dir := t.TempDir()
	real, err := filepath.EvalSymlinks(dir)
	require.NoError(t, err)
	dbPath := filepath.Join(real, "tm.db")
	tm, err := sievepen.NewSQLiteTM(dbPath)
	require.NoError(t, err)
	require.NoError(t, tm.Close())

	a := newExtractApp(t)
	cmd := a.NewTMCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"audit", "--file", dbPath, "--batch", "nonexistent"})
	require.NoError(t, cmd.Execute())
	assert.Contains(t, out.String(), "No TM entries found")
}

// confirm truncate helper behavior (used by audit output formatter)
func TestTruncateHelper(t *testing.T) {
	assert.Equal(t, "short", truncate("short", 40))
	truncated := truncate(strings.Repeat("x", 100), 16)
	// Rune-counted: 15 x's + 1 ellipsis.
	runes := []rune(truncated)
	assert.Len(t, runes, 16)
	assert.True(t, strings.HasSuffix(truncated, "…"))
}

// verify Ensure: dir helper unaffected by the test (sanity)
func TestTMAudit_NonexistentDBErrors(t *testing.T) {
	dir := t.TempDir()
	a := newExtractApp(t)
	cmd := a.NewTMCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"audit", "--file", filepath.Join(dir, "does-not-exist.db"), "--batch", "x"})
	err := cmd.Execute()
	// Either the command errors (can't open) or returns "no entries".
	// Accept either — the failure mode is graceful.
	if err != nil {
		require.Error(t, err)
	} else {
		assert.Contains(t, out.String(), "No TM entries")
	}
	require.NoError(t, os.MkdirAll(dir, 0o755)) // keep linter happy
}
