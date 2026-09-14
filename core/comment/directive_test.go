package comment_test

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/comment"
	"github.com/neokapi/neokapi/core/comment/golang"
	"github.com/neokapi/neokapi/core/format"
	yamlformat "github.com/neokapi/neokapi/core/formats/yaml"
	"github.com/neokapi/neokapi/core/model"
)

// declared holds the markers scripts/contract-audit reads in Go test comments.
var declared = comment.Directives{"okapi:", "okapi-skip:", "okapi-unmapped:", "okapi-deferred:"}

// Corpus floors for the repository's own markers. They catch a walk that
// reached almost nothing and still accounted for all of it, and sit below the
// repository's count so ordinary deletions never trip them.
const (
	corpusMinFiles   = 40
	corpusMinMarkers = 1800
)

func lines(first, last int) format.LineRange { return format.LineRange{First: first, Last: last} }

// spanOf returns the one place s sits in src.
func spanOf(t *testing.T, src []byte, s string) (int, int) {
	t.Helper()
	i := bytes.Index(src, []byte(s))
	require.GreaterOrEqual(t, i, 0, "%q is not in the source", s)
	require.Equal(t, i, bytes.LastIndex(src, []byte(s)), "%q is in the source more than once", s)
	return i, i + len(s)
}

func directive(t *testing.T, src []byte, s, form string) comment.Excluded {
	t.Helper()
	start, end := spanOf(t, src, s)
	return comment.Excluded{Start: start, End: end, Lines: format.NewLineIndex(src).Range(start, end), Reason: comment.ReasonDirective, Form: form}
}

func blank(t *testing.T, src []byte, start int) comment.Excluded {
	t.Helper()
	end := start + len("//")
	return comment.Excluded{Start: start, End: end, Lines: format.NewLineIndex(src).Range(start, end), Reason: comment.ReasonBlank}
}

func spanText(src []byte, c comment.Comment) string { return string(src[c.Start:c.End]) }

func isSpace(b byte) bool { return b == ' ' || b == '\t' || b == '\r' || b == '\n' }

// accounted holds got to the provider's own reading, plain, of the same bytes:
// every byte of a comment plain located that is not whitespace is claimed by
// exactly one comment or exclusion of got, got claims no byte plain does not,
// and each claim reports the lines the file puts it on.
func accounted(t *testing.T, name string, src []byte, plain, got *comment.File) {
	t.Helper()
	type claim struct {
		start, end int
		lines      format.LineRange
	}
	all := func(f *comment.File) []claim {
		var out []claim
		for _, c := range f.Comments {
			out = append(out, claim{c.Start, c.End, c.Lines})
		}
		for _, e := range f.Excluded {
			out = append(out, claim{e.Start, e.End, e.Lines})
		}
		return out
	}
	inComment := make([]bool, len(src))
	for _, c := range all(plain) {
		for i := c.start; i < c.end; i++ {
			inComment[i] = true
		}
	}
	claimed := make([]int, len(src))
	idx := format.NewLineIndex(src)
	for _, c := range all(got) {
		require.True(t, c.start >= 0 && c.start < c.end && c.end <= len(src), "%s: span %d-%d", name, c.start, c.end)
		assert.Equal(t, idx.Range(c.start, c.end), c.lines, "%s: lines of %q", name, src[c.start:c.end])
		for i := c.start; i < c.end; i++ {
			claimed[i]++
		}
	}
	for i, b := range src {
		switch {
		case claimed[i] > 1:
			t.Fatalf("%s: byte %d (%q) on line %d is claimed %d times", name, i, b, idx.Line(i), claimed[i])
		case !inComment[i] && claimed[i] > 0:
			t.Fatalf("%s: byte %d (%q) on line %d is claimed and holds no comment", name, i, b, idx.Line(i))
		case inComment[i] && claimed[i] == 0 && !isSpace(b):
			t.Fatalf("%s: byte %d (%q) on line %d is in a comment and claimed by nothing", name, i, b, idx.Line(i))
		}
	}
}

func locateBoth(t *testing.T, p comment.Provider, name string, src []byte, d comment.Directives) (plain, got *comment.File) {
	t.Helper()
	plain, err := p.Locate(name, src)
	require.NoError(t, err)
	got, err = comment.Locate(p, name, src, d)
	require.NoError(t, err)
	accounted(t, name, src, plain, got)
	return plain, got
}

func TestDeclaredDirectivesInGo(t *testing.T) {
	t.Run("a marker in the middle of a doc comment splits it, byte for byte", func(t *testing.T) {
		for name, eol := range map[string]string{"LF": "\n", "CRLF": "\r\n"} {
			t.Run(name, func(t *testing.T) {
				src := []byte(strings.ReplaceAll("package demo\n\n// Parse reads the input.\n// okapi-skip: ParseTest#testEmpty\n// It returns an error for empty input.\nfunc Parse() {}\n", "\n", eol))
				_, got := locateBoth(t, golang.Provider{}, "demo.go", src, declared)

				require.Len(t, got.Comments, 2)
				first, second := got.Comments[0], got.Comments[1]
				assert.Equal(t, "// Parse reads the input.", spanText(src, first))
				assert.Equal(t, lines(3, 3), first.Lines)
				assert.Equal(t, "Parse reads the input.", model.RunsText(first.Runs))
				assert.Equal(t, "// It returns an error for empty input.", spanText(src, second))
				assert.Equal(t, lines(5, 5), second.Lines)
				assert.Equal(t, "It returns an error for empty input.", model.RunsText(second.Runs))
				for _, c := range got.Comments {
					assert.Equal(t, "func/Parse", c.Subject, "each piece documents what the whole comment documented")
					assert.True(t, c.Doc)
				}
				assert.Equal(t, []comment.Excluded{directive(t, src, "// okapi-skip: ParseTest#testEmpty", "okapi-skip:")}, got.Excluded)

				blocks := got.Blocks()
				require.Len(t, blocks, 2)
				assert.Equal(t, []string{"func/Parse", "func/Parse#2"}, []string{blocks[0].ID, blocks[1].ID})
			})
		}
	})

	t.Run("blank lines beside a marker are set aside as blank", func(t *testing.T) {
		src := []byte("package demo\n\n// Parse reads the input.\n//\n// okapi-skip: ParseTest#testEmpty\n//\n// It returns an error.\nfunc Parse() {}\n")
		_, got := locateBoth(t, golang.Provider{}, "demo.go", src, declared)
		require.Len(t, got.Comments, 2)
		assert.Equal(t, lines(3, 3), got.Comments[0].Lines)
		assert.Equal(t, lines(7, 7), got.Comments[1].Lines)
		idx := format.NewLineIndex(src)
		lineStart := func(line int) int {
			start, _ := spanOf(t, src, "// Parse reads the input.\n")
			for n := 3; n < line; n++ {
				start += bytes.IndexByte(src[start:], '\n') + 1
			}
			require.Equal(t, line, idx.Line(start))
			return start
		}
		assert.Equal(t, []comment.Excluded{
			blank(t, src, lineStart(4)),
			directive(t, src, "// okapi-skip: ParseTest#testEmpty", "okapi-skip:"),
			blank(t, src, lineStart(6)),
		}, got.Excluded)
	})

	t.Run("a comment of markers alone is set aside whole", func(t *testing.T) {
		src := []byte("package demo\n\n// okapi-skip: ParseTest#testEmpty\n// okapi-unmapped: ParseTest#testLong\n// okapi-deferred: ParseTest#testHuge\nfunc Parse() {}\n")
		_, got := locateBoth(t, golang.Provider{}, "demo.go", src, declared)
		assert.Empty(t, got.Comments)
		assert.Equal(t, []comment.Excluded{
			directive(t, src, "// okapi-skip: ParseTest#testEmpty", "okapi-skip:"),
			directive(t, src, "// okapi-unmapped: ParseTest#testLong", "okapi-unmapped:"),
			directive(t, src, "// okapi-deferred: ParseTest#testHuge", "okapi-deferred:"),
		}, got.Excluded)
	})

	t.Run("a trailing comment and a delimited comment on one line", func(t *testing.T) {
		src := []byte("package demo\n\nfunc Parse() {\n\tx := 1 // okapi-skip: ParseTest#testEmpty\n\t/* okapi-unmapped: ParseTest#testLong */\n\t_ = x\n}\n")
		_, got := locateBoth(t, golang.Provider{}, "demo.go", src, declared)
		assert.Empty(t, got.Comments)
		assert.Equal(t, []comment.Excluded{
			directive(t, src, "// okapi-skip: ParseTest#testEmpty", "okapi-skip:"),
			directive(t, src, "/* okapi-unmapped: ParseTest#testLong */", "okapi-unmapped:"),
		}, got.Excluded)
	})

	t.Run("the comments around a split keep their own reading", func(t *testing.T) {
		src := []byte("package demo\n\n// Package demo is a fixture.\n\n// Parse reads.\n// okapi-skip: ParseTest#testEmpty\n// It returns.\nfunc Parse() {}\n\n// Deprecated: use Parse.\nfunc Old() {}\n")
		plain, got := locateBoth(t, golang.Provider{}, "demo.go", src, declared)
		require.Len(t, got.Comments, 4)
		assert.Equal(t, plain.Comments[0], got.Comments[0])
		assert.Equal(t, plain.Comments[2], got.Comments[3])
		assert.True(t, got.Comments[3].Deprecated)
	})

	t.Run("the longest declared marker names the directive", func(t *testing.T) {
		src := []byte("package demo\n\n// okapi-skip: ParseTest#testEmpty\nfunc Parse() {}\n")
		_, got := locateBoth(t, golang.Provider{}, "demo.go", src, comment.Directives{"okapi-", "okapi-skip:"})
		require.Len(t, got.Excluded, 1)
		assert.Equal(t, "okapi-skip:", got.Excluded[0].Form)
	})
}

func TestDeclaredDirectivesInYAML(t *testing.T) {
	t.Run("a marker between prose lines splits the comment", func(t *testing.T) {
		src := []byte("# Greets the reader.\n# okapi-skip: GreetingTest#testAll\n# Shown on the landing page.\ngreeting: Hello # okapi-unmapped: GreetingTest#testTrailing\n")
		_, got := locateBoth(t, yamlformat.CommentProvider{}, "app.yaml", src, declared)
		require.Len(t, got.Comments, 2)
		assert.Equal(t, "# Greets the reader.", spanText(src, got.Comments[0]))
		assert.Equal(t, lines(1, 1), got.Comments[0].Lines)
		assert.Equal(t, "# Shown on the landing page.", spanText(src, got.Comments[1]))
		assert.Equal(t, lines(3, 3), got.Comments[1].Lines)
		for _, c := range got.Comments {
			assert.Equal(t, "comment/greeting", c.Subject)
		}
		assert.Equal(t, []comment.Excluded{
			directive(t, src, "# okapi-skip: GreetingTest#testAll", "okapi-skip:"),
			directive(t, src, "# okapi-unmapped: GreetingTest#testTrailing", "okapi-unmapped:"),
		}, got.Excluded)
	})

	t.Run("CRLF and trailing blanks stay outside the span", func(t *testing.T) {
		src := []byte("# Greets.\r\n# okapi-skip: GreetingTest#testAll  \r\n# Shown.\r\ngreeting: Hello\r\n")
		_, got := locateBoth(t, yamlformat.CommentProvider{}, "app.yaml", src, declared)
		require.Len(t, got.Comments, 2)
		assert.Equal(t, []comment.Excluded{directive(t, src, "# okapi-skip: GreetingTest#testAll", "okapi-skip:")}, got.Excluded)
	})
}

// A declared directive is a marker at the start of a comment line and nothing
// else. Each case reads exactly as it does with nothing declared.
func TestDeclaredDirectivesLeaveProseAlone(t *testing.T) {
	for name, line := range map[string]string{
		"a marker later in the line":        "See the okapi-skip: markers in the tests.",
		"a marker in another case":          "OKAPI-SKIP: ParseTest#testEmpty",
		"a marker quoted in prose":          "`// okapi-skip: Class#method` markers name a test.",
		"a marker the recipe does not name": "audit-later: ParseTest#testEmpty",
		"a marker without its colon":        "okapi-skip ParseTest#testEmpty",
		"a list item holding a marker":      "- okapi-skip: ParseTest#testEmpty",
	} {
		t.Run(name, func(t *testing.T) {
			goSrc := []byte("package demo\n\n// Parse reads.\n// " + line + "\nfunc Parse() {}\n")
			plain, got := locateBoth(t, golang.Provider{}, "demo.go", goSrc, declared)
			assert.Equal(t, plain, got)

			yamlSrc := []byte("# Greets.\n# " + line + "\ngreeting: Hello\n")
			plain, got = locateBoth(t, yamlformat.CommentProvider{}, "app.yaml", yamlSrc, declared)
			assert.Equal(t, plain, got)
		})
	}

	t.Run("a marker inside a delimited comment over several lines", func(t *testing.T) {
		src := []byte("package demo\n\n/*\nParse reads.\nokapi-skip: ParseTest#testEmpty\n*/\nfunc Parse() {}\n")
		plain, got := locateBoth(t, golang.Provider{}, "demo.go", src, declared)
		assert.Equal(t, plain, got)
	})

	t.Run("nothing declared", func(t *testing.T) {
		src := []byte("package demo\n\n// okapi-skip: ParseTest#testEmpty\nfunc Parse() {}\n")
		plain, got := locateBoth(t, golang.Provider{}, "demo.go", src, nil)
		assert.Equal(t, plain, got)
	})
}

// replayingProvider answers every Locate with what it located first, so it never
// reads the bytes the layer masks.
type replayingProvider struct {
	golang.Provider
	first **comment.File
}

func (p replayingProvider) Locate(name string, src []byte) (*comment.File, error) {
	if *p.first == nil {
		f, err := p.Provider.Locate(name, src)
		if err != nil {
			return nil, err
		}
		*p.first = f
	}
	return *p.first, nil
}

// forgetfulProvider loses the last comment of a file on every reading after the
// first.
type forgetfulProvider struct {
	golang.Provider
	calls *int
}

func (p forgetfulProvider) Locate(name string, src []byte) (*comment.File, error) {
	f, err := p.Provider.Locate(name, src)
	*p.calls++
	if err != nil || *p.calls == 1 || len(f.Comments) == 0 {
		return f, err
	}
	f.Comments = f.Comments[:len(f.Comments)-1]
	return f, nil
}

// The layer takes a split comment's pieces from a second reading of the file
// with the marker lines blanked, and holds that reading to the first. A
// provider whose second reading does not line up leaves the file's comments
// unlocated rather than placed approximately.
func TestDeclaredDirectivesRefuseAReadingThatDoesNotLineUp(t *testing.T) {
	src := []byte("package demo\n\n// Parse reads.\n// okapi-skip: ParseTest#testEmpty\n// It returns.\nfunc Parse() {}\n\n// Format writes.\nfunc Format() {}\n")

	t.Run("must fail: a reading that ignores the blanked line", func(t *testing.T) {
		var first *comment.File
		_, err := comment.Locate(replayingProvider{first: &first}, "demo.go", src, declared)
		require.ErrorIs(t, err, comment.ErrUnlocated)
	})

	t.Run("must fail: a reading that differs away from the marker", func(t *testing.T) {
		calls := 0
		_, err := comment.Locate(forgetfulProvider{calls: &calls}, "demo.go", src, declared)
		require.ErrorIs(t, err, comment.ErrUnlocated)
	})
}

// Every `okapi:`, `okapi-skip:`, `okapi-unmapped:` and `okapi-deferred:` line in
// a comment of this repository's Go files is set aside, and each file's comments
// stay accounted for exactly once.
func TestDeclaredDirectivesOverTheRepositorysMarkers(t *testing.T) {
	_, here, _, ok := runtime.Caller(0)
	require.True(t, ok)
	root := filepath.Join(filepath.Dir(here), "..", "..")
	files, set := 0, 0
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", ".claude", "node_modules", "testdata":
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".go" {
			return nil
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		carries := false
		for _, m := range declared {
			carries = carries || bytes.Contains(src, []byte(m))
		}
		if !carries {
			return nil
		}
		files++
		plain, got := locateBoth(t, golang.Provider{}, path, src, declared)
		assert.GreaterOrEqual(t, len(got.Comments)+len(got.Excluded), len(plain.Comments)+len(plain.Excluded))
		for _, c := range got.Comments {
			for line := range strings.SplitSeq(spanText(src, c), "\n") {
				body, isLine := strings.CutPrefix(strings.TrimLeft(line, " \t"), "//")
				if !isLine {
					continue
				}
				body = strings.TrimLeft(body, " \t")
				for _, m := range declared {
					assert.False(t, strings.HasPrefix(body, m), "%s: %q is addressable", path, line)
				}
			}
		}
		for _, e := range got.Excluded {
			if _, ok := declared.Match(e.Form); ok && e.Reason == comment.ReasonDirective {
				set++
			}
		}
		return nil
	})
	require.NoError(t, err)
	t.Logf("%d files carry a marker; %d marker lines set aside", files, set)
	assert.GreaterOrEqual(t, files, corpusMinFiles, "the walk reached the files that carry markers")
	assert.GreaterOrEqual(t, set, corpusMinMarkers, "the marker lines were set aside")
}
