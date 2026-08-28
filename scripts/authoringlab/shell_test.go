package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A model reads a source tree however it likes. opus-5 does it with `ls` and
// `cat` and never calls the Read tool, so counting only that tool recorded zero
// files for a run that spent 1.7M tokens of context on the repository — a fact
// about the parser, published as a fact about the model.
//
// These pin the shell half. It is a heuristic and the tests are mostly about
// what it must NOT claim.

func labTree(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, f := range []string{"README.md", "crates/ignore/src/types.rs", "GUIDE.md"} {
		p := filepath.Join(dir, f)
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
		require.NoError(t, os.WriteFile(p, []byte("x"), 0o644))
	}
	return dir
}

func TestFilesFromShellFindsWhatWasOpened(t *testing.T) {
	dir := labTree(t)
	got := filesFromShell("cat README.md", dir)
	require.Len(t, got, 1)
	assert.Equal(t, filepath.Join(dir, "README.md"), got[0])

	got = filesFromShell("head -50 crates/ignore/src/types.rs", dir)
	require.Len(t, got, 1, "a flag is not a file")
	assert.Contains(t, got[0], "types.rs")
}

// TestFilesFromShellReadsEachSegment: an agent chains its reading, and counting
// only the first command would under-report exactly the runs that read most.
func TestFilesFromShellReadsEachSegment(t *testing.T) {
	dir := labTree(t)
	got := filesFromShell("cat README.md; head -20 GUIDE.md | wc -l", dir)
	assert.Len(t, got, 2)
}

// TestFilesFromShellInventsNothing is the one that matters. The count is
// evidence that the agent read the repository, so a heuristic that promotes a
// flag, a directory or a word into a file would manufacture that evidence.
func TestFilesFromShellInventsNothing(t *testing.T) {
	dir := labTree(t)
	for _, cmd := range []string{
		"ls -la crates",               // listing a directory reads no file
		"cat missing.md",              // a path that is not there
		"cat crates",                  // a directory
		"rg --type rust fn README.md", // a search, counted as a search
		"echo cat",                    // the word, not the command
		"head -20",                    // a flag and nothing else
		"cat $FILE",                   // a path this cannot resolve
		"cat *.md",                    // a glob this does not expand
	} {
		assert.Empty(t, filesFromShell(cmd, dir), "should claim nothing for: %s", cmd)
	}
}

// TestFilesFromShellHandlesAbsolutePaths: the agent works in the tree but names
// files both ways, and the same file must not be counted twice under two names.
func TestFilesFromShellHandlesAbsolutePaths(t *testing.T) {
	dir := labTree(t)
	abs := filepath.Join(dir, "README.md")
	got := filesFromShell("cat "+abs, dir)
	require.Len(t, got, 1)
	assert.Equal(t, abs, got[0])
	assert.Equal(t, "README.md", relToRepo(got[0], dir), "and it is named relative to the tree")
}
