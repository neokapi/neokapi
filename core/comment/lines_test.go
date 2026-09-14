package comment_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/comment"
	"github.com/neokapi/neokapi/core/comment/golang"
	"github.com/neokapi/neokapi/core/model"
)

func TestLineKinds(t *testing.T) {
	t.Run("a Go file's lines by what its comment layer reads in them", func(t *testing.T) {
		src := "// Package demo parses.\n" + // 1 package doc
			"package demo\n" + // 2 code
			"\n" + // 3 blank
			"//go:generate stringer -type=Kind\n" + // 4 directive
			"\n" + // 5 blank
			"// Parse reads.\n" + // 6 comment
			"//\n" + // 7 a blank line of the comment
			"// It stops.\n" + // 8 comment
			"/*\n" + // 9 block comment
			"prose // inside\n" + // 10 block comment
			"*/\n" + // 11 block comment
			"func Parse() string { // trailing\n" + // 12 code with a comment after it
			"\treturn \"// not a comment\"\n" + // 13 code
			"}\n" // 14 code
		f, err := golang.Provider{}.Locate("demo.go", []byte(src))
		require.NoError(t, err)
		kinds := f.LineKinds([]byte(src))
		require.Len(t, kinds, 15, "index 0 is unused, and a final line break opens no line")
		assert.Equal(t, []comment.LineKind{
			comment.LinePackageDoc, comment.LineCode, comment.LineBlank, comment.LineSetAside, comment.LineBlank,
			comment.LineComment, comment.LineComment, comment.LineComment,
			comment.LineComment, comment.LineComment, comment.LineComment,
			comment.LineCode, comment.LineCode, comment.LineCode,
		}, kinds[1:])
	})

	t.Run("must fail: a line with code beside a comment is code, whichever comes first", func(t *testing.T) {
		src := []byte("x /* a */ y\n/* b */ z\n")
		f := &comment.File{Comments: []comment.Comment{
			{Start: 2, End: 9, Runs: []model.Run{model.TextR("a")}},
			{Start: 12, End: 19, Runs: []model.Run{model.TextR("b")}},
		}}
		assert.Equal(t, []comment.LineKind{comment.LineCode, comment.LineCode}, f.LineKinds(src)[1:])
	})

	t.Run("an empty file has one blank line", func(t *testing.T) {
		assert.Equal(t, []comment.LineKind{comment.LineBlank, comment.LineBlank}, (&comment.File{}).LineKinds(nil))
	})
}
