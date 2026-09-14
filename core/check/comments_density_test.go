package check

import (
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/comment"
	"github.com/neokapi/neokapi/core/comment/golang"
	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
)

func TestCountChangeLines(t *testing.T) {
	kinds := []comment.LineKind{
		comment.LineBlank, // unused
		comment.LineCode, comment.LineComment, comment.LineComment, comment.LinePackageDoc,
		comment.LineSetAside, comment.LineBlank, comment.LineComment,
	}
	got := CountChangeLines(kinds, []format.LineRange{{First: 1, Last: 3}, {First: 2, Last: 5}, {First: 7, Last: 7}, {First: 20, Last: 25}})
	assert.Equal(t, ChangeLines{Comment: 3, Code: 1, First: 2, Last: 7}, got,
		"each added line counts once, the package doc, a set-aside line and a blank line count as neither, and a range past the file counts nothing")
}

func TestCommentDensityFindings(t *testing.T) {
	limits := DefaultCommentLimits()
	for _, tc := range []struct {
		name          string
		comment, code int
		flag          bool
	}{
		// The owner's density verdicts, by comment and code lines added.
		{"GO-DE024", 37, 7, true},
		{"GO-DE025", 23, 5, true},
		{"GO-DE026", 27, 6, true},
		{"GO-DE027", 28, 7, true},
		{"GO-DE028", 31, 10, true},
		{"GO-DE029", 58, 27, true},
		{"GO-DE030", 10, 5, true},
		{"GO-DE031", 13, 12, true},
		{"one comment line for each code line is allowed", 8, 8, false},
		{"one more comment line is not", 9, 8, true},
		{"fewer comment lines than the minimum", 7, 0, false},
		{"the minimum with no code", 8, 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := CommentDensityFindings(ChangeLines{Comment: tc.comment, Code: tc.code}, limits)
			if !tc.flag {
				assert.Empty(t, got)
				return
			}
			require.Len(t, got, 1)
			assert.Equal(t, CategoryCommentDensity, got[0].Category)
			assert.Equal(t, SeverityMajor, got[0].Severity)
		})
	}

	got := CommentDensityFindings(ChangeLines{Comment: 23, Code: 5}, limits)
	require.Len(t, got, 1)
	assert.Equal(t, "Change adds 23 comment lines and 5 code lines, more than 1 comment lines for each code line", got[0].Message)

	custom := limits
	custom.DensityRatio, custom.DensityMinLines = 2.5, 3
	assert.Empty(t, CommentDensityFindings(ChangeLines{Comment: 5, Code: 2}, custom), "5 is not more than 2.5 for each of 2 code lines")
	assert.Len(t, CommentDensityFindings(ChangeLines{Comment: 6, Code: 2}, custom), 1)
}

// goDensity counts the lines of a Go file a change adds whole, with the Go
// provider's line kinds, and reports what the density check finds.
func goDensity(t *testing.T, src string, kindsOf func(*comment.File, []byte) []comment.LineKind, limits CommentLimits) []Finding {
	t.Helper()
	f, err := golang.Provider{}.Locate("canary.go", []byte(src))
	require.NoError(t, err)
	kinds := kindsOf(f, []byte(src))
	return CommentDensityFindings(CountChangeLines(kinds, []format.LineRange{{First: 1, Last: len(kinds) - 1}}), limits)
}

// markerKinds classifies a line by its leading comment marker alone, the way a
// check that does not read the provider's spans would.
func markerKinds(_ *comment.File, src []byte) []comment.LineKind {
	marker := regexp.MustCompile(`^\s*//`)
	kinds := []comment.LineKind{comment.LineBlank}
	for line := range strings.SplitSeq(string(src), "\n") {
		switch {
		case strings.TrimSpace(line) == "":
			kinds = append(kinds, comment.LineBlank)
		case marker.MatchString(line):
			kinds = append(kinds, comment.LineComment)
		default:
			kinds = append(kinds, comment.LineCode)
		}
	}
	return kinds
}

func TestCommentDensityCanary(t *testing.T) {
	spans := func(f *comment.File, src []byte) []comment.LineKind { return f.LineKinds(src) }
	for _, limits := range []CommentLimits{
		DefaultCommentLimits(),
		{DensityRatio: 4, DensityMinLines: 2},
		{DensityRatio: 0.5, DensityMinLines: 20},
	} {
		canary := CommentDensityCanary(limits)
		probe := func(kindsOf func(*comment.File, []byte) []comment.LineKind) func(*model.Block) ([]Finding, error) {
			return func(b *model.Block) ([]Finding, error) {
				return goDensity(t, model.RunsText(b.SourceRuns()), kindsOf, limits), nil
			}
		}
		caught, err := Probe([]Canary{canary}, "", probe(spans))
		require.NoError(t, err)
		assert.Equal(t, CanaryCaught, caught.Status, "%+v", limits)

		missed, err := Probe([]Canary{canary}, "", probe(markerKinds))
		require.NoError(t, err)
		assert.Equal(t, CanaryMissed, missed.Status, "a check that finds comments by their markers misses the canary under %+v", limits)
	}
}

func TestCommentDensityLeavesThePackageDocOut(t *testing.T) {
	doc := "// Package demo " + strings.Repeat("reads.\n// It ", 10) + "stops.\npackage demo\n\nfunc Parse() {}\n"
	assert.Empty(t, goDensity(t, doc, func(f *comment.File, src []byte) []comment.LineKind { return f.LineKinds(src) }, DefaultCommentLimits()),
		"eleven lines of package doc beside two code lines is a new package, not a dense change")

	asFunc := strings.Replace(doc, "package demo\n\nfunc Parse() {}", "package demo\n\n"+strings.ReplaceAll(strings.SplitN(doc, "package demo", 2)[0], "Package demo", "Parse")+"func Parse() {}", 1)
	assert.Len(t, goDensity(t, asFunc, func(f *comment.File, src []byte) []comment.LineKind { return f.LineKinds(src) }, DefaultCommentLimits()), 1,
		"must fail: the same lines on a declaration count")
}
