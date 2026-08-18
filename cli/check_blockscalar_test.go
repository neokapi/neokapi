package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The gate reads a YAML block scalar's value as YAML defines it, terminating
// line break included. That break is the style the author chose — `|` and `>`
// keep one by clip chomping, `|-` and `>-` ask for none — so grading it as
// trailing whitespace reports every paragraph a YAML author writes, on a
// character they cannot remove without changing the document. It was the
// dominant finding over this repository's own YAML content and the reason no
// enforcing gate could be wired over it.
//
// The four-scalar reproduction from the issue, plus the two chomping modifiers
// that complete the set and the defects that must survive.
func TestCheck_ABlockScalarTerminatorIsNotTrailingWhitespace(t *testing.T) {
	t.Setenv("KAPI_NO_PROJECT", "1")
	dir := t.TempDir()
	probe := filepath.Join(dir, "probe.yaml")
	require.NoError(t, os.WriteFile(probe, []byte(`literal: |
  A sentence in a literal block.
folded: >
  A sentence in a folded block
  wrapped over two lines.
stripped: |-
  A sentence with strip chomping.
foldedStripped: >-
  A sentence folded and stripped.
plain: A plain scalar sentence.
`), 0o644))

	a := &App{SourceLang: "en"}
	report, err := a.ComputeCheck(NewCheckCmd(a), []string{probe})
	require.NoError(t, err)

	assert.Zero(t, ruleCounts(report)["hygiene.trailing-whitespace"],
		"no scalar in this document has whitespace anyone wrote: %+v", report.Findings)
	assert.True(t, report.Pass)
	assert.Equal(t, 100, report.Summary.Score)
}

// TestCheck_StrayWhitespaceStillFails: the fix silences the terminator and
// nothing else. A trailing space before the break, and a blank line kept by `|+`,
// are whitespace the author put there and stay reportable.
func TestCheck_StrayWhitespaceStillFails(t *testing.T) {
	t.Setenv("KAPI_NO_PROJECT", "1")
	dir := t.TempDir()
	probe := filepath.Join(dir, "stray.yaml")
	require.NoError(t, os.WriteFile(probe, []byte("spaced: |\n  A sentence with a stray space.  \nkept: |+\n  A sentence with keep chomping.\n\n"), 0o644))

	a := &App{SourceLang: "en"}
	report, err := a.ComputeCheck(NewCheckCmd(a), []string{probe})
	require.NoError(t, err)

	assert.Equal(t, 2, ruleCounts(report)["hygiene.trailing-whitespace"],
		"both the stray space and the kept blank line are the author's: %+v", report.Findings)
}
