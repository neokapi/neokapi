package golang

import (
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/neokapi/neokapi/core/comment"
	"github.com/neokapi/neokapi/core/comment/commenttest"
	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Corpus floors. The exact accounting of every file is what catches a scanner
// that loses one comment; these catch a scan that ran over the wrong tree, or
// over almost nothing, and still accounted for everything it saw. They sit
// well below the repository's size so that ordinary deletions never trip them.
const (
	corpusMinFiles    = 3000
	corpusMinComments = 40000
)

// TestProseP1_go is the P1 rung for Go comments: every comment group located as
// blocks with exact byte spans and inclusive line ranges, every directive form
// classified and never addressable, and a scan of the repository's own Go files
// that accounts for every comment exactly once.
//
// The subtests named "must fail" corrupt a result on purpose and assert that
// the check notices. A check that cannot fail proves nothing, so these are part
// of the rung rather than beside it. Nothing in here skips.
func TestProseP1_go(t *testing.T) {
	t.Run("spans are the parser's own positions", func(t *testing.T) {
		src := []byte(spanFixture)
		got, err := Provider{}.Locate("demo.go", src)
		require.NoError(t, err)
		require.NoError(t, accountFor("demo.go", src, got))
		require.NotEmpty(t, got.Comments)

		answer := findSubject(t, got, "const/Answer", true)
		assert.Equal(t, "// Answer is the answer.", string(src[answer.Start:answer.End]))
		assert.Equal(t, format.LineRange{First: 11, Last: 11}, answer.Lines, "a one-line comment is inclusive at both ends")

		pkg := findSubject(t, got, "package", true)
		assert.Equal(t, format.LineRange{First: 1, Last: 3}, pkg.Lines)
		assert.Equal(t, "// Package demo is a fixture.\n//\n// It has two paragraphs.", string(src[pkg.Start:pkg.End]))

		block := findSubject(t, got, "type/Block", true)
		assert.Equal(t, comment.StyleBlock, block.Style)
		assert.Equal(t, format.LineRange{First: 21, Last: 23}, block.Lines, "a block comment's last line is the line of */")
	})

	t.Run("CRLF block comments keep their closing delimiter", func(t *testing.T) {
		src := []byte(strings.ReplaceAll(spanFixture, "\n", "\r\n"))
		got, err := Provider{}.Locate("demo.go", src)
		require.NoError(t, err)
		require.NoError(t, accountFor("demo.go", src, got))
		block := findSubject(t, got, "type/Block", true)
		assert.True(t, strings.HasSuffix(string(src[block.Start:block.End]), "*/"))
	})

	t.Run("a file of directives has no addressable comment", func(t *testing.T) {
		src := []byte(directiveFixture)
		got, err := Provider{}.Locate("directives.go", src)
		require.NoError(t, err)
		require.NoError(t, accountFor("directives.go", src, got))
		assert.Empty(t, got.Comments)
		require.NotEmpty(t, got.Excluded)
		for _, e := range got.Excluded {
			assert.Equal(t, comment.ReasonDirective, e.Reason, "%q", src[e.Start:e.End])
		}
	})

	t.Run("prose sharing a group with a directive stays addressable", func(t *testing.T) {
		src := []byte(mixedFixture)
		got, err := Provider{}.Locate("mixed.go", src)
		require.NoError(t, err)
		require.NoError(t, accountFor("mixed.go", src, got))
		var texts []string
		for _, c := range got.Comments {
			texts = append(texts, model.RunsText(c.Runs))
		}
		assert.Equal(t, []string{
			"data holds the fixtures. The prose above a pragma is still prose.",
			"before the directive",
			"after the directive",
		}, texts)
	})

	t.Run("the shared conformance suite", func(t *testing.T) {
		commenttest.Run(t, goSuite(Provider{}))
	})

	t.Run("must fail: the conformance suite catches a broken provider", func(t *testing.T) {
		for _, tc := range []struct {
			name string
			p    comment.Provider
			want commenttest.Property
		}{
			{"a span one byte short", brokenGo{edit: func(_ []byte, f *comment.File) { f.Comments[0].End-- }}, commenttest.PropSpan},
			{"a comment dropped", brokenGo{edit: func(_ []byte, f *comment.File) { f.Comments = f.Comments[1:] }}, commenttest.PropAccount},
			{"a marker inside a string counted", brokenGo{edit: countStringMarker}, commenttest.PropLiteral},
		} {
			t.Run(tc.name, func(t *testing.T) {
				failures := commenttest.Verify(goSuite(tc.p))
				require.NotEmpty(t, failures)
				assert.Contains(t, commenttest.Properties(failures), tc.want, "%v", commenttest.Err(failures))
			})
		}
	})

	t.Run("the repository's Go files account for every comment", func(t *testing.T) {
		stats := scanCorpus(t, repoRoot(t))
		t.Logf("prose-corpus go files=%d unparsed=%d generated=%d comments=%d groups=%d words=%d "+
			"directive-lines=%d directive-groups=%d mixed-groups=%d mixed-words=%d block-comments=%d "+
			"cgo-preambles=%d example-outputs=%d deprecated=%d",
			stats.files, stats.unparsed, stats.generated, stats.comments, stats.groups, stats.words,
			stats.directiveLines, stats.directiveGroups, stats.mixedGroups, stats.mixedWords, stats.blockComments,
			stats.cgoPreambles, stats.exampleOutputs, stats.deprecated)
		require.Empty(t, stats.failures, "files whose comments are not accounted for")
		assert.GreaterOrEqual(t, stats.files, corpusMinFiles, "the scan found too few Go files; is the root right?")
		assert.GreaterOrEqual(t, stats.comments, corpusMinComments, "the scan located too few comments")
	})

	t.Run("must fail: a span one byte short", func(t *testing.T) {
		src := []byte(spanFixture)
		for _, corrupt := range []struct {
			name string
			edit func(*comment.Comment)
		}{
			{"end one byte early", func(c *comment.Comment) { c.End-- }},
			{"start one byte late", func(c *comment.Comment) { c.Start++ }},
			{"last line one too far", func(c *comment.Comment) { c.Lines.Last++ }},
		} {
			t.Run(corrupt.name, func(t *testing.T) {
				got, err := Provider{}.Locate("demo.go", src)
				require.NoError(t, err)
				corrupt.edit(&got.Comments[0])
				assert.Error(t, accountFor("demo.go", src, got))
			})
		}
	})

	t.Run("must fail: a classifier missing a directive form", func(t *testing.T) {
		for i, form := range directiveForms {
			t.Run(form.name, func(t *testing.T) {
				forms := slices.Delete(slices.Clone(directiveForms), i, i+1)
				got, err := locate("directives.go", []byte(directiveFixture), forms)
				require.NoError(t, err)
				assert.NotEmpty(t, got.Comments, "without %q the directive fixture should expose a directive as prose", form.name)
			})
		}
	})

	t.Run("must fail: two groups merged into one span", func(t *testing.T) {
		src := []byte(spanFixture)
		got, err := Provider{}.Locate("demo.go", src)
		require.NoError(t, err)
		first, second := got.Comments[0], got.Comments[1]
		merged := first
		merged.End, merged.Lines.Last = second.End, second.Lines.Last
		got.Comments = append([]comment.Comment{merged}, got.Comments[2:]...)
		// The lines between the two groups (the package clause, the import)
		// hold no comment, and the one comment inside them joins the merge.
		got.Excluded = slices.DeleteFunc(got.Excluded, func(e comment.Excluded) bool {
			return e.Start > first.Start && e.End < second.End
		})
		err = accountFor("demo.go", src, got)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "merges comment group")
	})

	t.Run("must fail: a comment dropped", func(t *testing.T) {
		src := []byte(spanFixture)
		got, err := Provider{}.Locate("demo.go", src)
		require.NoError(t, err)
		got.Comments = got.Comments[1:]
		err = accountFor("demo.go", src, got)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "neither addressable nor excluded")
	})

	t.Run("must fail: a directive inside an addressable span", func(t *testing.T) {
		src := []byte(mixedFixture)
		got, err := Provider{}.Locate("mixed.go", src)
		require.NoError(t, err)
		// Stretch the first comment over the blank line and the //go:embed
		// pragma below it, and drop their exclusions.
		c := &got.Comments[0]
		embed := strings.Index(mixedFixture, "//go:embed data") + len("//go:embed data")
		c.End, c.Lines.Last = embed, 7
		got.Excluded = slices.DeleteFunc(got.Excluded, func(e comment.Excluded) bool { return e.End <= embed })
		err = accountFor("mixed.go", src, got)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "contains the directive")
	})
}

func goSuite(p comment.Provider) commenttest.Suite {
	return commenttest.Suite{
		Provider: p,
		Scan:     goUnits,
		Fixtures: []commenttest.Fixture{
			{Name: "demo.go", Source: spanFixture},
			{Name: "crlf.go", Source: strings.ReplaceAll(spanFixture, "\n", "\r\n")},
			{Name: "directives.go", Source: directiveFixture},
			{Name: "mixed.go", Source: mixedFixture},
			{Name: "literals.go", Source: literalFixture, Literals: []string{`"// not a comment"`, "`/* not a comment */`"}},
		},
	}
}

// brokenGo damages what the Go provider locates in every file with a comment,
// except the canary.
type brokenGo struct {
	Provider
	edit func(src []byte, f *comment.File)
}

func (b brokenGo) Locate(name string, src []byte) (*comment.File, error) {
	f, err := b.Provider.Locate(name, src)
	if err == nil && name != "canary" && len(f.Comments) > 0 {
		b.edit(src, f)
	}
	return f, err
}

// countStringMarker reports the `//` inside a string literal as a comment.
func countStringMarker(src []byte, f *comment.File) {
	at := strings.Index(string(src), `// not a comment"`)
	if at < 0 {
		return
	}
	end := at + len("// not a comment")
	f.Comments = append(f.Comments, comment.Comment{
		Start: at, End: end, Lines: format.NewLineIndex(src).Range(at, end),
		Style: comment.StyleLine, Subject: "comment", Runs: []model.Run{model.TextR("not a comment")},
	})
	slices.SortFunc(f.Comments, func(a, b comment.Comment) int { return a.Start - b.Start })
}

func findSubject(t *testing.T, f *comment.File, subject string, doc bool) comment.Comment {
	t.Helper()
	for _, c := range f.Comments {
		if c.Subject == subject && c.Doc == doc {
			return c
		}
	}
	require.Failf(t, "subject not found", "no comment on %q (doc=%v)", subject, doc)
	return comment.Comment{}
}

// repoRoot walks up from the package directory to the directory holding
// go.work, the root of every module in the repository.
func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	require.NoError(t, err)
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.work")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		require.NotEqual(t, parent, dir, "no go.work above the package; the corpus scan needs the repository")
		dir = parent
	}
}

type corpusStats struct {
	files, unparsed, generated                  int
	comments, groups, words                     int
	directiveLines, directiveGroups             int
	mixedGroups, mixedWords                     int
	blockComments, cgoPreambles, exampleOutputs int
	deprecated                                  int
	failures                                    []string
}

// scanCorpus locates the comments of every Go file under root and accounts for
// each one. Worktrees, dependencies and vendored trees are skipped; test data is
// kept, because a fixture that parses is Go a reader meets.
func scanCorpus(t *testing.T, root string) corpusStats {
	t.Helper()
	var paths []string
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case ".claude", "node_modules", "vendor", ".git":
				return fs.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(p, ".go") {
			paths = append(paths, p)
		}
		return nil
	})
	require.NoError(t, err)

	results := make([]corpusStats, len(paths))
	var wg sync.WaitGroup
	sem := make(chan struct{}, runtime.GOMAXPROCS(0))
	for i, p := range paths {
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer func() { <-sem; wg.Done() }()
			results[i] = scanOne(root, p)
		}()
	}
	wg.Wait()

	var total corpusStats
	for _, r := range results {
		total.files += r.files
		total.unparsed += r.unparsed
		total.generated += r.generated
		total.comments += r.comments
		total.groups += r.groups
		total.words += r.words
		total.directiveLines += r.directiveLines
		total.directiveGroups += r.directiveGroups
		total.mixedGroups += r.mixedGroups
		total.mixedWords += r.mixedWords
		total.blockComments += r.blockComments
		total.cgoPreambles += r.cgoPreambles
		total.exampleOutputs += r.exampleOutputs
		total.deprecated += r.deprecated
		total.failures = append(total.failures, r.failures...)
	}
	return total
}

func scanOne(root, path string) corpusStats {
	var s corpusStats
	rel, _ := filepath.Rel(root, path)
	src, err := os.ReadFile(path)
	if err != nil {
		s.failures = append(s.failures, rel+": "+err.Error())
		return s
	}
	groups, err := groupsOf(path, src)
	if err != nil {
		// A file that does not parse has no exact positions to account for.
		s.unparsed = 1
		return s
	}
	got, err := Provider{}.Locate(path, src)
	if err != nil {
		s.failures = append(s.failures, rel+": "+err.Error())
		return s
	}
	if err := accountFor(path, src, got); err != nil {
		s.failures = append(s.failures, rel+": "+err.Error())
		return s
	}
	s.files = 1
	s.comments = len(got.Comments)

	// Per-group tallies: which groups hold prose, which hold directives, and
	// how much prose a rule excluding any group with a directive would lose.
	type tally struct{ prose, directives, words int }
	byGroup := make([]tally, len(groups))
	type bounds struct{ start, end int }
	extents := make([]bounds, len(groups))
	for i, g := range groups {
		// groupsOf parses into a fresh file set, whose one file starts at base 1.
		last := g.List[len(g.List)-1]
		extents[i] = bounds{start: int(g.Pos()) - 1, end: endOf(src, int(last.Pos())-1, last.Text)}
	}
	groupOf := func(start int) int {
		for i, b := range extents {
			if start >= b.start && start < b.end {
				return i
			}
		}
		return -1
	}
	for _, c := range got.Comments {
		w := len(strings.Fields(model.RunsText(c.Runs)))
		s.words += w
		if c.Style == comment.StyleBlock {
			s.blockComments++
		}
		if c.Deprecated {
			s.deprecated++
		}
		if gi := groupOf(c.Start); gi >= 0 {
			byGroup[gi].prose++
			byGroup[gi].words += w
		}
	}
	seenReason := map[comment.Reason]map[int]bool{}
	for _, e := range got.Excluded {
		gi := groupOf(e.Start)
		switch e.Reason {
		case comment.ReasonGenerated:
			s.generated = 1
		case comment.ReasonDirective:
			s.directiveLines++
			if gi >= 0 {
				byGroup[gi].directives++
			}
		case comment.ReasonCgoPreamble, comment.ReasonExampleOutput:
			if seenReason[e.Reason] == nil {
				seenReason[e.Reason] = map[int]bool{}
			}
			if !seenReason[e.Reason][gi] {
				seenReason[e.Reason][gi] = true
				if e.Reason == comment.ReasonCgoPreamble {
					s.cgoPreambles++
				} else {
					s.exampleOutputs++
				}
			}
		}
	}
	for _, g := range byGroup {
		switch {
		case g.prose > 0 && g.directives > 0:
			s.groups++
			s.mixedGroups++
			s.mixedWords += g.words
		case g.prose > 0:
			s.groups++
		case g.directives > 0:
			s.directiveGroups++
		}
	}
	return s
}
