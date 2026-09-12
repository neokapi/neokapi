package diffscope

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/format"
)

// The fixtures under testdata/ are git's own output, captured from a scratch
// repository with `git diff`, `git diff -U0` and `git diff --cached -M`, plus
// one `diff -u` for the plain form. They are read as bytes: mixed.diff holds
// CRLF content lines that an editor must not normalize.

func parseFixture(t *testing.T, name string) []File {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	require.NoError(t, err)
	files, err := Parse(data)
	require.NoError(t, err)
	return files
}

func byPath(t *testing.T, files []File, path string) File {
	t.Helper()
	for _, f := range files {
		if f.Path() == path {
			return f
		}
	}
	t.Fatalf("no file %q in the diff", path)
	return File{}
}

func lines(first, last int) format.LineRange { return format.LineRange{First: first, Last: last} }

func TestParse_GitFileKinds(t *testing.T) {
	files := parseFixture(t, "mixed.diff")
	require.Len(t, files, 10, "every file git listed")

	added := byPath(t, files, "added.md")
	assert.Equal(t, Added, added.Status)
	assert.Empty(t, added.OldPath)
	assert.Equal(t, []Change{{Lines: lines(1, 2)}}, added.Changes)

	binary := byPath(t, files, "blob.bin")
	assert.Equal(t, Modified, binary.Status)
	assert.True(t, binary.Binary)
	assert.Empty(t, binary.Changes)

	crlf := byPath(t, files, "crlf.md")
	assert.Equal(t, []Change{{Lines: lines(2, 2)}}, crlf.Changes)

	quoted := byPath(t, files, "dir with space/naïve.md")
	assert.Equal(t, "dir with space/naïve.md", quoted.OldPath)
	assert.Equal(t, []Change{{Lines: lines(1, 1)}}, quoted.Changes)

	gone := byPath(t, files, "gone.md")
	assert.Equal(t, Deleted, gone.Status)
	assert.Empty(t, gone.NewPath)
	assert.Equal(t, "gone.md", gone.OldPath)

	pure := byPath(t, files, "pure-renamed.md")
	assert.Equal(t, Renamed, pure.Status)
	assert.Equal(t, "pure.md", pure.OldPath)
	assert.Empty(t, pure.Changes, "a rename with no content change touches no line")

	tail := byPath(t, files, "tail.md")
	assert.Equal(t, []Change{{Lines: lines(1, 1)}}, tail.Changes, "the no-newline marker is not a line")

	tool := byPath(t, files, "tool.sh")
	assert.True(t, tool.ModeChanged)
	assert.Equal(t, Modified, tool.Status)
	assert.Empty(t, tool.Changes)
}

func TestParse_RenameWithEdits(t *testing.T) {
	files := parseFixture(t, "rename-modified.diff")
	require.Len(t, files, 1)
	f := files[0]
	assert.Equal(t, Renamed, f.Status)
	assert.Equal(t, "notes.md", f.OldPath)
	assert.Equal(t, "renamed-notes.md", f.NewPath)
	assert.Equal(t, []Change{{Lines: lines(4, 4)}}, f.Changes)
}

func TestParse_PlainUnifiedDiff(t *testing.T) {
	files := parseFixture(t, "plain-u.diff")
	require.Len(t, files, 1)
	assert.Equal(t, "/dev/fd/13", files[0].NewPath, "the timestamp after the tab is not part of the path")
	assert.Equal(t, []Change{{Lines: lines(2, 2)}}, files[0].Changes)
}

func TestParse_Deletions(t *testing.T) {
	// guide.md: a heading on line 1, a seven-line paragraph on lines 3-9, a
	// closing paragraph on line 11.
	tests := []struct {
		fixture string
		want    []Change
	}{
		// Line 6 removed: in the post-image it sat between lines 5 and 6.
		{"delete-inside-block.diff", []Change{{Lines: lines(5, 6), Deletion: true, After: 5}}},
		{"delete-inside-block-u0.diff", []Change{{Lines: lines(5, 6), Deletion: true, After: 5}}},
		// Line 3, the paragraph's first, removed: it sat between lines 2 and 3.
		{"delete-first-line-u0.diff", []Change{{Lines: lines(2, 3), Deletion: true, After: 2}}},
	}
	for _, tt := range tests {
		t.Run(tt.fixture, func(t *testing.T) {
			files := parseFixture(t, tt.fixture)
			require.Len(t, files, 1)
			assert.Equal(t, tt.want, files[0].Changes)
		})
	}
}

func TestParse_HunkShapes(t *testing.T) {
	tests := []struct {
		name string
		diff string
		want []Change
	}{
		{
			name: "a replacement touches only its new lines",
			diff: "--- a/f\n+++ b/f\n@@ -1,4 +1,3 @@\n a\n-b\n-c\n+B\n d\n",
			want: []Change{{Lines: lines(2, 2)}},
		},
		{
			name: "two runs in one hunk",
			diff: "--- a/f\n+++ b/f\n@@ -1,5 +1,5 @@\n-a\n+A\n b\n c\n-d\n+D\n e\n",
			want: []Change{{Lines: lines(1, 1)}, {Lines: lines(4, 4)}},
		},
		{
			name: "deleting a file's first line",
			diff: "--- a/f\n+++ b/f\n@@ -1 +0,0 @@\n-a\n",
			want: []Change{{Lines: lines(1, 1), Deletion: true, After: 0}},
		},
		{
			name: "a context line whose space was stripped",
			diff: "--- a/f\n+++ b/f\n@@ -1,3 +1,3 @@\n a\n\n-c\n+C\n",
			want: []Change{{Lines: lines(3, 3)}},
		},
		{
			name: "two hunks",
			diff: "--- a/f\n+++ b/f\n@@ -2 +2 @@\n-b\n+B\n@@ -9,0 +10,2 @@\n+x\n+y\n",
			want: []Change{{Lines: lines(2, 2)}, {Lines: lines(10, 11)}},
		},
		{
			name: "CRLF on every diff line",
			diff: "--- a/f\r\n+++ b/f\r\n@@ -1,2 +1,2 @@\r\n a\r\n-b\r\n+B\r\n",
			want: []Change{{Lines: lines(2, 2)}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			files, err := Parse([]byte(tt.diff))
			require.NoError(t, err)
			require.Len(t, files, 1)
			assert.Equal(t, "f", files[0].Path())
			assert.Equal(t, tt.want, files[0].Changes)
		})
	}
}

func TestParse_RefusesMalformedDiffs(t *testing.T) {
	tests := []struct {
		name, diff, reason string
	}{
		{"a hunk shorter than its counts", "--- a/f\n+++ b/f\n@@ -1,3 +1,3 @@\n a\n-b\n", "ends early"},
		{"a hunk longer than its counts", "--- a/f\n+++ b/f\n@@ -1 +1 @@\n-a\n+b\n+c\n", "beyond the hunk's counts"},
		{"a hunk with no file", "@@ -1 +1 @@\n-a\n+b\n", "no file header"},
		{"a combined diff", "diff --cc f\n@@@ -1,1 -1,1 +1,1 @@@\n", "combined"},
		{"a garbled hunk header", "--- a/f\n+++ b/f\n@@ -x +1 @@\n", "malformed hunk header"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Parse([]byte(tt.diff))
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.reason)
		})
	}
}

func TestParse_EmptyDiff(t *testing.T) {
	files, err := Parse(nil)
	require.NoError(t, err)
	assert.Empty(t, files)
}

// guideExtents are the blocks of the guide.md the fixtures were taken from.
var guideExtents = []format.Extent{
	{Block: "heading", Lines: lines(1, 1)},
	{Block: "paragraph", Lines: lines(3, 9)},
	{Block: "closing", Lines: lines(11, 11)},
}

func blockIDs(xs []format.Extent) []string {
	ids := []string{}
	for _, x := range xs {
		ids = append(ids, x.Block)
	}
	return ids
}

// TestTouched checks each resolution and, beside it, the neighbouring line that
// must not resolve: a test of the first line passes as easily with an
// intersection that is off by one as with a correct one, unless the line before
// is asserted to miss.
func TestTouched(t *testing.T) {
	tests := []struct {
		name    string
		changes []Change
		want    []string
	}{
		{"a line inside a block", []Change{{Lines: lines(6, 6)}}, []string{"paragraph"}},
		{"the block's first line", []Change{{Lines: lines(3, 3)}}, []string{"paragraph"}},
		{"the line before the block", []Change{{Lines: lines(2, 2)}}, []string{}},
		{"the block's last line", []Change{{Lines: lines(9, 9)}}, []string{"paragraph"}},
		{"the line after the block", []Change{{Lines: lines(10, 10)}}, []string{}},
		{"a line past every block", []Change{{Lines: lines(40, 40)}}, []string{}},
		{"a hunk spanning two blocks", []Change{{Lines: lines(9, 11)}}, []string{"paragraph", "closing"}},
		{"two changes in one block report it once", []Change{{Lines: lines(4, 4)}, {Lines: lines(8, 8)}}, []string{"paragraph"}},
		{"a deletion inside the block", []Change{{Lines: lines(5, 6), Deletion: true}}, []string{"paragraph"}},
		{"a deletion of the block's first line", []Change{{Lines: lines(2, 3), Deletion: true}}, []string{"paragraph"}},
		{"a deletion between blank lines", []Change{{Lines: lines(10, 10), Deletion: true}}, []string{}},
		{"no changes", nil, []string{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, blockIDs(Touched(guideExtents, tt.changes)))
		})
	}
}

// TestTouched_OneLineHunkReturnsTheWholeBlock runs a real one-line git diff
// against extents derived from the file it was taken from.
func TestTouched_OneLineHunkReturnsTheWholeBlock(t *testing.T) {
	post := "# Guide\n\nOne two three.\nFour five six.\nSeven eight nine.\nTen ELEVEN twelve.\nThirteen fourteen.\nFifteen sixteen.\nSeventeen eighteen.\n\nA closing paragraph.\n"
	entries := []format.SkeletonEntry{
		{Type: format.SkeletonText, Data: []byte("# ")},
		{Type: format.SkeletonRef, Data: []byte("heading")},
		{Type: format.SkeletonText, Data: []byte("\n\n")},
		{Type: format.SkeletonRef, Data: []byte("paragraph")},
		{Type: format.SkeletonText, Data: []byte("\n\n")},
		{Type: format.SkeletonRef, Data: []byte("closing")},
		{Type: format.SkeletonText, Data: []byte("\n")},
	}
	extents, err := format.AlignSkeleton([]byte(post), entries)
	require.NoError(t, err)

	files := parseFixture(t, "modify-one-line.diff")
	require.Len(t, files, 1)
	require.Equal(t, []Change{{Lines: lines(6, 6)}}, files[0].Changes)

	touched := Touched(extents, files[0].Changes)
	require.Len(t, touched, 1)
	assert.Equal(t, "paragraph", touched[0].Block)
	assert.Equal(t, lines(3, 9), touched[0].Lines, "seven lines, the whole paragraph")
	assert.Equal(t, "One two three.\nFour five six.\nSeven eight nine.\nTen ELEVEN twelve.\nThirteen fourteen.\nFifteen sixteen.\nSeventeen eighteen.",
		post[touched[0].Start:touched[0].End])
}
