package markdown

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/text"
	"golang.org/x/net/html"

	"github.com/neokapi/neokapi/core/comment"
	"github.com/neokapi/neokapi/core/comment/commenttest"
	"github.com/neokapi/neokapi/core/comment/markup"
	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
)

const pageFixture = "---\n" +
	"title: \"<!-- not a comment -->\"\n" +
	"---\n" +
	"\n" +
	"<!-- The page head. -->\n" +
	"\n" +
	"# Install\n" +
	"\n" +
	"<!-- truncate -->\n" +
	"\n" +
	"Use `<!-- not a comment -->` in a code span, and <!-- an inline comment --> here.\n" +
	"\n" +
	"```html\n" +
	"<!-- not a comment -->\n" +
	"```\n" +
	"\n" +
	"    <!-- not a comment, indented code -->\n" +
	"\n" +
	"## From Homebrew\n" +
	"\n" +
	"<div>\n" +
	"  <!-- Inside an HTML block. -->\n" +
	"  <span title=\"<!-- not a comment -->\">x</span>\n" +
	"</div>\n" +
	"\n" +
	"<!--\n" +
	"Spans several lines,\n" +
	"and ends here.\n" +
	"-->\n" +
	"\n" +
	"<!-- markdownlint-disable MD033 -->\n" +
	"<!-- BEGIN:downloads-cli -->\n" +
	"<!-- kapi:voice -->\n" +
	"\n" +
	"\\<!-- escaped, not a comment\n" +
	"\n" +
	"> <!-- In a quote. -->\n" +
	"\n" +
	"<!-- -->\n"

// pageLiterals hold comment markers that are content: in front matter, a code
// span, fenced and indented code, and an attribute value.
var pageLiterals = []string{"<!-- not a comment -->", "<!-- not a comment, indented code -->", "\\<!-- escaped"}

// directiveFixture has one line for each directive form, matched by that form
// alone.
const directiveFixture = "<!-- truncate -->\n" +
	"<!-- kapi:voice -->\n" +
	"<!-- BEGIN: gap-analysis report (generated) -->\n" +
	"<!-- markdownlint-disable MD033 -->\n" +
	"<!-- prettier-ignore -->\n" +
	"<!-- @formatter:off -->\n" +
	"<!--suppress HtmlUnknownTag -->\n"

func TestLocateMarkdownCommentsFindsEveryCommentWithItsSpan(t *testing.T) {
	src := []byte(pageFixture)
	got, err := LocateComments(src)
	require.NoError(t, err)
	assert.Equal(t, "markdown", got.Language)

	type located struct {
		subject string
		text    string
		lines   format.LineRange
		bytes   string
	}
	var comments []located
	for _, c := range got.Comments {
		assert.Equal(t, comment.StyleBlock, c.Style)
		comments = append(comments, located{c.Subject, model.RunsText(c.Runs), c.Lines, string(src[c.Start:c.End])})
	}
	assert.Equal(t, []located{
		{"comment/document", "The page head.", format.LineRange{First: 5, Last: 5}, "<!-- The page head. -->"},
		{"comment/install", "an inline comment", format.LineRange{First: 11, Last: 11}, "<!-- an inline comment -->"},
		{"comment/install/from-homebrew", "Inside an HTML block.", format.LineRange{First: 22, Last: 22}, "<!-- Inside an HTML block. -->"},
		{"comment/install/from-homebrew", "Spans several lines,\nand ends here.", format.LineRange{First: 26, Last: 29}, "<!--\nSpans several lines,\nand ends here.\n-->"},
		{"comment/install/from-homebrew", "In a quote.", format.LineRange{First: 37, Last: 37}, "<!-- In a quote. -->"},
	}, comments)

	var excluded []string
	for _, e := range got.Excluded {
		excluded = append(excluded, string(e.Reason)+" "+e.Form+" "+string(src[e.Start:e.End]))
	}
	assert.Equal(t, []string{
		"directive truncate <!-- truncate -->",
		"directive markdownlint <!-- markdownlint-disable MD033 -->",
		"directive region <!-- BEGIN:downloads-cli -->",
		"directive kapi:voice <!-- kapi:voice -->",
		"blank  <!-- -->",
	}, excluded)
}

func TestLocateMarkdownCommentsRefusesWhatItCannotPlace(t *testing.T) {
	for name, src := range map[string]string{
		"a comment never closed":      "Text <!-- never closed\n\nMore text.\n",
		"a marker in a link title":    "[a](https://example.test \"<!-- c -->\")\n",
		"a comment closing on --!>":   "Text <!-- a --!> b --> c\n",
		"an HTML block ending inside": "<div>\n<!-- a\n</div>\n",
	} {
		t.Run(name, func(t *testing.T) {
			_, err := LocateComments([]byte(src))
			require.ErrorIs(t, err, ErrCommentsUnlocated)
			require.ErrorIs(t, err, comment.ErrUnlocated, "the host treats any provider's unlocated file the same way")
		})
	}
}

func TestMarkdownLineText(t *testing.T) {
	for _, tc := range []struct {
		line string
		n    int
		text string
		ok   bool
	}{
		{"<!-- a -->", 10, " a ", true},
		{"<!-->", 5, "", true},
		{"<!--->", 6, "", true},
		{"<!---->", 7, "", true},
		{"<!-- a -- b --> after", 15, " a -- b ", true},
		{"<!-- spans on to the next line", 0, "", false},
		{"text", 0, "", false},
	} {
		t.Run(tc.line, func(t *testing.T) {
			n, text, ok := CommentProvider{}.LineText([]byte(tc.line))
			assert.Equal(t, tc.ok, ok)
			assert.Equal(t, tc.n, n)
			assert.Equal(t, tc.text, text)
		})
	}
}

func TestMarkdownDirectiveFixtureCoversEveryForm(t *testing.T) {
	got, err := LocateComments([]byte(directiveFixture))
	require.NoError(t, err)
	assert.Empty(t, got.Comments)
	for _, f := range commentDirectiveForms {
		assert.True(t, slices.ContainsFunc(got.Excluded, func(e comment.Excluded) bool { return e.Form == f.Name }),
			"no line in the directive fixture exercises %q", f.Name)
	}

	t.Run("must fail: a classifier missing a directive form", func(t *testing.T) {
		for i, f := range commentDirectiveForms {
			t.Run(f.Name, func(t *testing.T) {
				forms := slices.Delete(slices.Clone(commentDirectiveForms), i, i+1)
				got, err := locateComments([]byte(directiveFixture), func(inner string) (string, bool) { return markup.Classify(inner, forms) })
				require.NoError(t, err)
				assert.NotEmpty(t, got.Comments, "without %q the fixture exposes a directive as prose", f.Name)
			})
		}
	})
}

func markdownSuite(p comment.Provider) commenttest.Suite {
	return commenttest.Suite{
		Provider: p,
		Scan:     mdUnits,
		Fixtures: []commenttest.Fixture{
			{Name: "page.md", Source: pageFixture, Literals: pageLiterals},
			{Name: "crlf.md", Source: strings.ReplaceAll(pageFixture, "\n", "\r\n"), Literals: pageLiterals},
			{Name: "directives.md", Source: directiveFixture},
		},
	}
}

// TestProseP1_markdown is the P1 rung for Markdown comments: the conformance
// suite over fixtures, the repository's Markdown files accounted for against
// the Markdown parser and the HTML tokenizer, and providers broken on purpose
// that the suite must catch.
func TestProseP1_markdown(t *testing.T) {
	t.Run("the shared conformance suite", func(t *testing.T) {
		commenttest.Run(t, markdownSuite(CommentProvider{}))
	})

	t.Run("the repository's Markdown files account for every comment", func(t *testing.T) {
		root := markdownRepoRoot(t)
		cmd := exec.CommandContext(t.Context(), "git", "ls-files", "-z", "*.md", "*.markdown")
		cmd.Dir = root
		listing, err := cmd.Output()
		require.NoError(t, err)
		files, comments, refused := 0, 0, 0
		for rel := range strings.SplitSeq(strings.TrimRight(string(listing), "\x00"), "\x00") {
			if rel == "" || strings.Contains(rel, "node_modules/") {
				continue
			}
			src, err := os.ReadFile(filepath.Join(root, rel))
			require.NoError(t, err)
			got, err := LocateComments(src)
			if errors.Is(err, ErrCommentsUnlocated) {
				refused++
				t.Logf("%s: %v", rel, err)
				continue
			}
			require.NoError(t, err, rel)
			units, err := mdUnits(rel, src)
			require.NoError(t, err, rel)
			assert.NoError(t, commenttest.Err(commenttest.Account(src, got, units)), rel)
			files++
			comments += len(units)
		}
		t.Logf("markdown-comments files=%d comments=%d refused=%d", files, comments, refused)
		assert.Positive(t, files, "the corpus holds no Markdown file the provider reads")
		assert.Positive(t, comments, "the corpus holds no comment, so it proves nothing about losing one")
	})

	t.Run("must fail: the conformance suite catches a broken provider", func(t *testing.T) {
		for _, tc := range []struct {
			name string
			edit func([]byte, *comment.File)
			want commenttest.Property
		}{
			{"a span one byte short", func(_ []byte, f *comment.File) { f.Comments[0].End-- }, commenttest.PropSpan},
			{"a comment dropped", func(_ []byte, f *comment.File) { f.Comments = f.Comments[1:] }, commenttest.PropAccount},
			{"a marker inside a code span counted", countCodeSpanMarker, commenttest.PropLiteral},
		} {
			t.Run(tc.name, func(t *testing.T) {
				failures := commenttest.Verify(markdownSuite(brokenMarkdown{edit: tc.edit}))
				require.NotEmpty(t, failures)
				assert.Contains(t, commenttest.Properties(failures), tc.want, "%v", commenttest.Err(failures))
			})
		}
	})
}

var declaredDirectives = comment.Directives{"okapi-skip:", "okapi-unmapped:"}

// declaredFixture holds declared markers where a Markdown comment can open with
// one: as a whole comment, and inline after text. A marker inside a comment over
// several lines, and prose that only mentions a marker, stay prose.
const declaredFixture = "# Page\n" +
	"\n" +
	"<!-- okapi-skip: PageTest#testEmpty -->\n" +
	"\n" +
	"Hello <!-- okapi-unmapped: PageTest#testTrailing -->\n" +
	"\n" +
	"<!--\n" +
	"okapi-skip: inside a comment over several lines\n" +
	"-->\n" +
	"\n" +
	"<!-- See the okapi-skip: markers; OKAPI-SKIP: in capitals is prose too. -->\n"

// The conformance suite with directives declared: every comment that opens
// with one is set aside as a directive, and every other comment is accounted
// for as it is with nothing declared.
func TestMarkdownConformanceWithDeclaredDirectives(t *testing.T) {
	s := markdownSuite(CommentProvider{})
	s.Directives = declaredDirectives
	s.Fixtures = append(s.Fixtures,
		commenttest.Fixture{Name: "declared.md", Source: declaredFixture},
		commenttest.Fixture{Name: "declared-crlf.md", Source: strings.ReplaceAll(declaredFixture, "\n", "\r\n")},
	)
	commenttest.Run(t, s)

	got, err := comment.Locate(CommentProvider{}, "declared.md", []byte(declaredFixture), declaredDirectives)
	require.NoError(t, err)
	forms := 0
	for _, e := range got.Excluded {
		if e.Reason == comment.ReasonDirective {
			forms++
		}
	}
	assert.Equal(t, 2, forms, "the suite ran over the declared markers")
	assert.Len(t, got.Comments, 2, "the comment over several lines and the one that mentions a marker stay prose")
}

// mdUnits is the conformance suite's scan for Markdown: every inline HTML
// comment the Markdown parser reports, and every comment the HTML tokenizer
// reports inside an HTML block.
func mdUnits(_ string, src []byte) ([]commenttest.Unit, error) {
	_, _, base, _ := frontMatterBounds(src)
	body := src[base:]
	doc := goldmark.New(goldmark.WithExtensions(extension.GFM)).Parser().Parse(text.NewReader(body))
	var units []commenttest.Unit
	add := func(start, end int) {
		u := commenttest.Unit{Start: start, End: end, Open: len(markup.Open), Close: len(markup.Close), Group: len(units)}
		switch string(src[start:end]) {
		case "<!-->":
			u.Close = 1
		case "<!--->":
			u.Close = 2
		}
		_, u.Directive = classifyComment(string(src[start+u.Open : end-u.Close]))
		units = append(units, u)
	}
	err := ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch n := n.(type) {
		case *ast.RawHTML:
			if n.Segments.Len() > 0 {
				s, e := base+n.Segments.At(0).Start, base+n.Segments.At(n.Segments.Len()-1).Stop
				if bytes.HasPrefix(src[s:e], []byte(markup.Open)) {
					add(s, e)
				}
			}
		case *ast.HTMLBlock:
			lines := n.Lines()
			if lines.Len() == 0 {
				return ast.WalkSkipChildren, nil
			}
			s, e := base+lines.At(0).Start, base+lines.At(lines.Len()-1).Stop
			if n.HasClosure() && base+n.ClosureLine.Stop > e {
				e = base + n.ClosureLine.Stop
			}
			z := html.NewTokenizer(bytes.NewReader(src[s:e]))
			z.SetMaxBuf(0)
			for off := s; ; {
				tt := z.Next()
				if tt == html.ErrorToken {
					break
				}
				raw := z.Raw()
				if tt == html.CommentToken && bytes.HasPrefix(raw, []byte(markup.Open)) {
					add(off, off+len(raw))
				}
				off += len(raw)
			}
			return ast.WalkSkipChildren, nil
		}
		return ast.WalkContinue, nil
	})
	return units, err
}

// brokenMarkdown damages what the provider locates in every file with a
// comment, except the canary.
type brokenMarkdown struct {
	CommentProvider
	edit func([]byte, *comment.File)
}

func (b brokenMarkdown) Locate(name string, src []byte) (*comment.File, error) {
	f, err := b.CommentProvider.Locate(name, src)
	if err == nil && name != "canary" && len(f.Comments) > 0 {
		b.edit(src, f)
	}
	return f, err
}

// countCodeSpanMarker reports the comment marker inside a code span as a
// comment.
func countCodeSpanMarker(src []byte, f *comment.File) {
	const marker = "<!-- not a comment -->"
	at := bytes.Index(src, []byte("`"+marker))
	if at < 0 {
		return
	}
	at++
	f.Comments = append(f.Comments, comment.Comment{
		Start: at, End: at + len(marker), Lines: format.NewLineIndex(src).Range(at, at+len(marker)),
		Style: comment.StyleBlock, Subject: "comment/install", Runs: []model.Run{model.TextR("not a comment")},
	})
	slices.SortFunc(f.Comments, func(a, b comment.Comment) int { return a.Start - b.Start })
}

func markdownRepoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	require.NoError(t, err)
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.work")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		require.NotEqual(t, parent, dir, "no go.work above the package")
		dir = parent
	}
}
