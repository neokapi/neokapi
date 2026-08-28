package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	coreprofile "github.com/neokapi/neokapi/core/profile"
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

// TestTheLabLoadsAProfileCarryingOnlyNotes.
//
// loadProfile treated every validation problem as fatal, so the moment tone
// stopped being an enum the lab refused to start — on the very register kapi
// had inferred from ripgrep's own documentation. A note is not a failure, and
// the distinction only helps if every gate reads it the same way.
func TestTheLabLoadsAProfileCarryingOnlyNotes(t *testing.T) {
	p, err := loadProfile()
	require.NoError(t, err, "the embedded profile loads")
	require.NotNil(t, p)

	probs := coreprofile.ValidateProfile(p)
	assert.NotEmpty(t, probs, "it does carry a note, so this test is not vacuous")
	assert.Empty(t, coreprofile.Blocking(probs), "and no note blocks it")
}

// TestBothCoordinatesRenderAGuide: a persona that resolves to nothing would
// make the governed arm identical to the bare one and the whole comparison
// silently meaningless.
func TestBothCoordinatesRenderAGuide(t *testing.T) {
	base, err := loadProfile()
	require.NoError(t, err)
	for _, pt := range points {
		guide, err := guideFor(base, pt)
		require.NoError(t, err, pt.Audience)
		assert.NotEmpty(t, guide, pt.Audience)
	}
}

// TestTaskPromptsPrescribeNoStyle.
//
// The end-user brief said the reader has "no interest in how the tool is
// built", which instructs the BARE arm to avoid implementation detail — the
// exact thing the coordinate exists to do. Both arms were steered and only one
// was credited, so the measured contrast understated itself. A task names the
// reader and the deliverable; anything about the prose belongs in the profile.
func TestTaskPromptsPrescribeNoStyle(t *testing.T) {
	banned := []string{
		"not a programmer", "no interest in how", "avoid", "do not use",
		"plain language", "jargon", "keep it simple", "technical detail",
	}
	for _, pt := range points {
		low := strings.ToLower(pt.Task)
		for _, b := range banned {
			assert.NotContains(t, low, b,
				"%s: the task prescribes prose, which belongs in the profile", pt.Audience)
		}
	}
}
