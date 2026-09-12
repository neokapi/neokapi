package diffscope

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/format"
)

func readTestdata(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	require.NoError(t, err)
	return data
}

// TestPreImage_RealCommit rebuilds the file a real commit to this repository
// changed (fac49dfd3, which removed the last item of a list in
// docs/internals/video-revision-runbook.md) from the file it left and its diff,
// and compares the result with git's own pre-image byte for byte.
func TestPreImage_RealCommit(t *testing.T) {
	post := readTestdata(t, "list-item-removal.post.md")
	pre := readTestdata(t, "list-item-removal.pre.md")
	files, err := Parse(readTestdata(t, "list-item-removal.diff"))
	require.NoError(t, err)
	require.Len(t, files, 1)

	rebuilt, err := files[0].PreImage(post)
	require.NoError(t, err)
	assert.Equal(t, string(pre), string(rebuilt))

	// The must-fail half: a post-image that differs on a line the hunk shows.
	corrupt := bytes.Replace(post, []byte("a path the app never used."), []byte("a path the app never used!"), 1)
	_, err = files[0].PreImage(corrupt)
	require.Error(t, err)
}

func TestPreImage_Shapes(t *testing.T) {
	tests := []struct {
		name, diff, post, pre string
	}{
		{
			name: "a replaced line",
			diff: "--- a/f\n+++ b/f\n@@ -1,3 +1,3 @@\n a\n-b\n+B\n c\n",
			post: "a\nB\nc\n", pre: "a\nb\nc\n",
		},
		{
			name: "lines outside the hunks",
			diff: "--- a/f\n+++ b/f\n@@ -3 +2,0 @@\n-c\n@@ -6 +5,2 @@\n-f\n+F\n+G\n",
			post: "a\nb\nd\ne\nF\nG\nh\n", pre: "a\nb\nc\nd\ne\nf\nh\n",
		},
		{
			name: "a removed first line",
			diff: "--- a/f\n+++ b/f\n@@ -1 +0,0 @@\n-a\n",
			post: "b\n", pre: "a\nb\n",
		},
		{
			name: "the old file lacked a final newline",
			diff: "--- a/f\n+++ b/f\n@@ -1 +1 @@\n-a\n\\ No newline at end of file\n+b\n",
			post: "b\n", pre: "a",
		},
		{
			name: "the new file lacks a final newline",
			diff: "--- a/f\n+++ b/f\n@@ -1 +1 @@\n-a\n+b\n\\ No newline at end of file\n",
			post: "b", pre: "a\n",
		},
		{
			name: "neither has one",
			diff: "--- a/f\n+++ b/f\n@@ -1,2 +1,2 @@\n-a\n+A\n b\n\\ No newline at end of file\n",
			post: "A\nb", pre: "a\nb",
		},
		{
			name: "CRLF content",
			diff: "--- a/f\n+++ b/f\n@@ -1,2 +1,2 @@\n-a\r\n+A\r\n b\r\n",
			post: "A\r\nb\r\n", pre: "a\r\nb\r\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			files, err := Parse([]byte(tt.diff))
			require.NoError(t, err)
			require.Len(t, files, 1)
			pre, err := files[0].PreImage([]byte(tt.post))
			require.NoError(t, err)
			assert.Equal(t, tt.pre, string(pre))
		})
	}
}

func TestPreLine(t *testing.T) {
	files, err := Parse([]byte("--- a/f\n+++ b/f\n@@ -3 +2,0 @@\n-c\n@@ -6 +5,2 @@\n-f\n+F\n+G\n"))
	require.NoError(t, err)
	f := files[0]
	// post: a b d e F G h   pre: a b c d e f h
	for post, want := range map[int]int{1: 1, 2: 2, 3: 4, 4: 5, 7: 7} {
		got, ok := f.PreLine(post)
		assert.True(t, ok, "line %d", post)
		assert.Equal(t, want, got, "line %d", post)
	}
	for _, added := range []int{5, 6} {
		_, ok := f.PreLine(added)
		assert.False(t, ok, "line %d was added", added)
	}
}

// jsonExtents locates the three values of a one-key-per-line JSON object.
func jsonExtents(t *testing.T, src string, keys ...string) []format.Extent {
	t.Helper()
	var entries []format.SkeletonEntry
	text := "{\n"
	for i, k := range keys {
		entries = append(entries, format.SkeletonEntry{Type: format.SkeletonText, Data: []byte(text + "  \"" + k + "\": \"")})
		entries = append(entries, format.SkeletonEntry{Type: format.SkeletonRef, Data: []byte(k)})
		text = "\""
		if i < len(keys)-1 {
			text += ","
		}
		text += "\n"
	}
	entries = append(entries, format.SkeletonEntry{Type: format.SkeletonText, Data: []byte(text + "}\n")})
	xs, err := format.AlignSkeleton([]byte(src), entries)
	require.NoError(t, err)
	return xs
}

func blockNames(xs []format.Extent) []string {
	out := []string{}
	for _, x := range xs {
		out = append(out, x.Block)
	}
	return out
}

// TestSettle_JSONNeighbours removes one key of a catalog. Placement alone takes
// the keys on either side; the pre-image shows both unchanged.
func TestSettle_JSONNeighbours(t *testing.T) {
	pre := "{\n  \"a\": \"Alpha\",\n  \"b\": \"Bravo\",\n  \"c\": \"Charlie\"\n}\n"
	post := "{\n  \"a\": \"Alpha\",\n  \"c\": \"Charlie\"\n}\n"
	files, err := Parse([]byte("--- a/x.json\n+++ b/x.json\n@@ -3 +2,0 @@\n-  \"b\": \"Bravo\",\n"))
	require.NoError(t, err)
	f := files[0]
	postExtents := jsonExtents(t, post, "a", "c")
	preExtents := jsonExtents(t, pre, "a", "b", "c")

	touched := Touched(postExtents, f.Changes)
	assert.Equal(t, []string{"a", "c"}, blockNames(touched), "the placement borders both neighbours")
	assert.Equal(t, []string{"a", "c"}, blockNames(Bordered(touched, f.Changes)))

	rebuilt, err := f.PreImage([]byte(post))
	require.NoError(t, err)
	require.Equal(t, pre, string(rebuilt))
	assert.Empty(t, Settle(f, []byte(post), touched, rebuilt, preExtents))
}

// TestSettle_KeepsWhatTheChangeAltered pairs each block Settle must keep with
// the pre-image that would let it go.
func TestSettle_KeepsWhatTheChangeAltered(t *testing.T) {
	paragraph := func(t *testing.T, src string, lines ...string) []format.Extent {
		t.Helper()
		// A document of paragraphs separated by blank lines.
		var entries []format.SkeletonEntry
		for i, id := range lines {
			if i > 0 {
				entries = append(entries, format.SkeletonEntry{Type: format.SkeletonText, Data: []byte("\n\n")})
			}
			entries = append(entries, format.SkeletonEntry{Type: format.SkeletonRef, Data: []byte(id)})
		}
		entries = append(entries, format.SkeletonEntry{Type: format.SkeletonText, Data: []byte("\n")})
		xs, err := format.AlignSkeleton([]byte(src), entries)
		require.NoError(t, err)
		return xs
	}

	t.Run("a paragraph that lost its first line", func(t *testing.T) {
		pre := "Intro.\n\nOne.\nTwo.\nThree.\n"
		post := "Intro.\n\nTwo.\nThree.\n"
		files, err := Parse([]byte("--- a/d.md\n+++ b/d.md\n@@ -3 +2,0 @@\n-One.\n"))
		require.NoError(t, err)
		f := files[0]
		touched := Touched(paragraph(t, post, "intro", "body"), f.Changes)
		require.Equal(t, []string{"body"}, blockNames(touched), "line 2 is blank, so only the paragraph below borders it")
		assert.Equal(t, []string{"body"}, blockNames(Settle(f, []byte(post), touched, []byte(pre), paragraph(t, pre, "intro", "body"))))
	})

	t.Run("a pre-image that does not fit the diff clears nothing", func(t *testing.T) {
		post := "Intro.\n\nTwo.\nThree.\n"
		files, err := Parse([]byte("--- a/d.md\n+++ b/d.md\n@@ -3 +2,0 @@\n-Gone.\n"))
		require.NoError(t, err)
		f := files[0]
		touched := Touched(paragraph(t, post, "intro", "body"), f.Changes)
		// One blank line more than the diff allows: the paragraph sits a line
		// lower than the post-image line maps to, so nothing matches.
		misfit := "Intro.\n\nGone.\n\nTwo.\nThree.\n"
		assert.Len(t, Settle(f, []byte(post), touched, []byte(misfit), paragraph(t, misfit, "intro", "gone", "body")), 1)
	})

	t.Run("a deletion strictly inside a block is not bordered", func(t *testing.T) {
		post := "One.\nThree.\n"
		files, err := Parse([]byte("--- a/d.md\n+++ b/d.md\n@@ -2 +1,0 @@\n-Two.\n"))
		require.NoError(t, err)
		f := files[0]
		touched := Touched(paragraph(t, post, "body"), f.Changes)
		require.Len(t, touched, 1)
		assert.Empty(t, Bordered(touched, f.Changes))
		pre := "One.\nTwo.\nThree.\n"
		assert.Len(t, Settle(f, []byte(post), touched, []byte(pre), paragraph(t, pre, "body")), 1)
	})

	t.Run("an added line keeps its block", func(t *testing.T) {
		post := "One.\nTwo.\n"
		files, err := Parse([]byte("--- a/d.md\n+++ b/d.md\n@@ -1,0 +2 @@\n+Two.\n"))
		require.NoError(t, err)
		touched := Touched(paragraph(t, post, "body"), files[0].Changes)
		assert.Empty(t, Bordered(touched, files[0].Changes))
		assert.Len(t, Settle(files[0], []byte(post), touched, []byte("One.\n"), paragraph(t, "One.\n", "body")), 1)
	})
}
