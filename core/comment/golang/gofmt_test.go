package golang

import (
	"testing"

	"github.com/neokapi/neokapi/core/comment"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func disagreements(t *testing.T, src string) (*comment.File, []comment.Disagreement) {
	t.Helper()
	got := locateString(t, "demo.go", src)
	d, err := Provider{}.Disagreements("demo.go", []byte(src), got)
	require.NoError(t, err)
	return got, d
}

func disagreeingText(f *comment.File, ds []comment.Disagreement) []string {
	var out []string
	for _, d := range ds {
		out = append(out, model.RunsText(f.Comments[d.Comment].Runs))
	}
	return out
}

func TestGofmtAgreesWithAFormattedFile(t *testing.T) {
	_, ds := disagreements(t, spanFixture)
	assert.Empty(t, ds)
}

func TestGofmtDisagreesWithAMisindentedComment(t *testing.T) {
	src := "package demo\n\n// Parse parses.\nfunc Parse() {\n\t// Indented correctly.\n\tx := 1\n  // Indented with spaces.\n\t_ = x\n}\n"
	f, ds := disagreements(t, src)
	assert.Equal(t, []string{"Indented with spaces."}, disagreeingText(f, ds))
}

// The spacing between code and a trailing comment is alignment gofmt owns,
// and belongs to the code rather than to the comment.
func TestGofmtIgnoresTrailingCommentAlignment(t *testing.T) {
	src := "package demo\n\nconst (\n\tA = 1 // first\n\tLonger = 2 // second\n)\n"
	_, ds := disagreements(t, src)
	assert.Empty(t, ds)
}

func TestGofmtDisagreesWithADocCommentItReformats(t *testing.T) {
	// gofmt indents a doc comment's code block with a tab and puts a blank line
	// on either side of it.
	src := "package demo\n\n// Parse parses.\n//\n// Code:\n//     x := 1\n// Text.\nfunc Parse() {}\n\n// Clean is fine.\nfunc Clean() {}\n"
	f, ds := disagreements(t, src)
	assert.Equal(t, []string{"Parse parses.\n\nCode:\n\n\n\nText."}, disagreeingText(f, ds))
	require.Len(t, ds, 1)
	assert.Contains(t, ds[0].Formatted, "//\tx := 1")
}

func TestGofmtAttributesAMovedDirectiveToTheProseAroundIt(t *testing.T) {
	// gofmt moves a directive to the end of its doc comment, behind a blank
	// line. Only the directive's line changes, and the prose on both sides of
	// it is the doc comment being rewritten.
	src := "package demo\n\n// F does it.\n//go:noinline\n// More prose.\nfunc F() {}\n\n// G is fine.\n//\n//go:noinline\nfunc G() {}\n"
	f, ds := disagreements(t, src)
	assert.Equal(t, []string{"F does it.", "More prose."}, disagreeingText(f, ds))
}

func TestDiffLines(t *testing.T) {
	changed, inserted := diffLines([]string{"a", "b", "c"}, []string{"a", "x", "b", "c", "y"})
	assert.Equal(t, []bool{false, false, false}, changed)
	assert.Equal(t, []bool{false, true, false, true}, inserted)

	changed, inserted = diffLines([]string{"a", "b", "c"}, []string{"a", "c"})
	assert.Equal(t, []bool{false, true, false}, changed)
	assert.Equal(t, []bool{false, false, false, false}, inserted)
}
