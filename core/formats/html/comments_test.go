package html

import (
	"bytes"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/html"

	"github.com/neokapi/neokapi/core/comment"
	"github.com/neokapi/neokapi/core/comment/commenttest"
	"github.com/neokapi/neokapi/core/comment/markup"
	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
)

const pageFixture = `<!DOCTYPE html>
<!-- The start page. -->
<html lang="en">
<head>
  <title>Start <!-- not a comment --></title>
  <style>/* <!-- not a comment --> */ p { color: red; }</style>
  <!--[if mso]><style>td { padding: 0; }</style><![endif]-->
</head>
<body>
  <!-- The greeting on the start page. -->
  <section id="hero">
    <p title="<!-- not a comment -->">Hello</p> <!-- Shown first. -->
  </section>
  <!--
    Spans several lines,
    and ends here.
  -->
  <textarea><!-- not a comment --></textarea>
  <script>
    var s = "<!-- not a comment -->";
  </script>
  <!-- <p>Retired</p> -->
  <!--$--><!--html--><!--/$-->
  <!-- body -->
  <!--#include virtual="/footer.html" -->
  <!-- -->
  <!---->
  <!--> abrupt
  <!-- closes with a bang --!>
</body>
</html>
<!-- After the document. -->
`

// pageLiterals hold comment markers that are content: in a title, a style, an
// attribute value, a textarea and a script.
var pageLiterals = []string{"<!-- not a comment -->"}

// directiveFixture has one line for each directive form, matched by that form
// alone, and a React marker.
const directiveFixture = `<body>
  <!--[if mso]><table><![endif]-->
  <!--#include virtual="/footer.html" -->
  <!-- markdownlint-disable MD033 -->
  <!-- prettier-ignore -->
  <!-- @formatter:off -->
  <!--suppress HtmlUnknownTag -->
  <!-- ReSharper disable MarkupTextTypo -->
  <!--$-->
</body>
`

func TestLocateHTMLCommentsFindsEveryCommentWithItsSpan(t *testing.T) {
	src := []byte(pageFixture)
	got, err := LocateComments(src)
	require.NoError(t, err)
	assert.Equal(t, "html", got.Language)

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
		{"comment/html", "The start page.", format.LineRange{First: 2, Last: 2}, "<!-- The start page. -->"},
		{"comment/section[hero]", "The greeting on the start page.", format.LineRange{First: 10, Last: 10}, "<!-- The greeting on the start page. -->"},
		{"comment/p", "Shown first.", format.LineRange{First: 12, Last: 12}, "<!-- Shown first. -->"},
		{"comment/textarea", "Spans several lines,\nand ends here.", format.LineRange{First: 14, Last: 17}, "<!--\n    Spans several lines,\n    and ends here.\n  -->"},
		{"comment/document", "", format.LineRange{First: 22, Last: 22}, "<!-- <p>Retired</p> -->"},
		{"comment/document", "body", format.LineRange{First: 24, Last: 24}, "<!-- body -->"},
		{"comment/document", "closes with a bang", format.LineRange{First: 29, Last: 29}, "<!-- closes with a bang --!>"},
		{"comment/document", "After the document.", format.LineRange{First: 32, Last: 32}, "<!-- After the document. -->"},
	}, comments)

	retired := got.Comments[4].Runs
	require.Len(t, retired, 1)
	require.NotNil(t, retired[0].Ph, "commented-out markup is a placeholder, never prose")
	assert.Equal(t, markup.SubMarkup, retired[0].Ph.SubType)

	var excluded []string
	for _, e := range got.Excluded {
		excluded = append(excluded, string(e.Reason)+" "+e.Form+" "+string(src[e.Start:e.End]))
	}
	assert.Equal(t, []string{
		"directive conditional-comment <!--[if mso]><style>td { padding: 0; }</style><![endif]-->",
		"directive react <!--$-->",
		"directive react <!--html-->",
		"directive react <!--/$-->",
		"directive ssi <!--#include virtual=\"/footer.html\" -->",
		"blank  <!-- -->",
		"blank  <!---->",
		"blank  <!-->",
	}, excluded)
}

func TestLocateHTMLCommentsInScripts(t *testing.T) {
	for name, tc := range map[string]struct {
		src      string
		comments []string
	}{
		"a marker inside a script is text": {
			"<script>if (a <!-- b) {}</script><!-- after -->", []string{"<!-- after -->"},
		},
		"an end tag hidden behind an escaped script": {
			"<script><!-- <script></script> --></script><!-- after -->", []string{"<!-- after -->"},
		},
		"a self-closing script still reads its text": {
			"<script/><!-- inside --></script><!-- after -->", []string{"<!-- after -->"},
		},
		"a raw text end tag in capitals": {
			"<STYLE><!-- inside --></Style ><!-- after -->", []string{"<!-- after -->"},
		},
		"a quoted attribute holding >": {
			"<p data-x='a > <!-- b -->'>x</p><!-- after -->", []string{"<!-- after -->"},
		},
		"a bogus comment holding a marker ends at its first >": {
			"<?php echo '<!-- x'; ?><!-- after -->", []string{"<!-- after -->"},
		},
	} {
		t.Run(name, func(t *testing.T) {
			src := []byte(tc.src)
			got, err := LocateComments(src)
			require.NoError(t, err)
			var texts []string
			for _, c := range got.Comments {
				texts = append(texts, string(src[c.Start:c.End]))
			}
			assert.Equal(t, tc.comments, texts)
			units, err := htmlUnits("", src)
			require.NoError(t, err)
			assert.NoError(t, commenttest.Err(commenttest.Account(src, got, units)))
		})
	}
}

func TestLocateHTMLCommentsRefusesWhatItCannotPlace(t *testing.T) {
	for name, src := range map[string]string{
		"an unclosed comment":                     "<p>Hello</p>\n<!-- a",
		"a comment the parser reads inside SVG":   "<svg><style><!-- in SVG a comment --></style></svg>",
		"a CDATA section the parser reads in SVG": "<svg><![CDATA[ x > <!-- y --> ]]></svg>",
	} {
		t.Run(name, func(t *testing.T) {
			_, err := LocateComments([]byte(src))
			require.ErrorIs(t, err, ErrCommentsUnlocated)
			require.ErrorIs(t, err, comment.ErrUnlocated, "the host treats any provider's unlocated file the same way")
		})
	}

	t.Run("a scan and a tokenizer that disagree", func(t *testing.T) {
		tokens := []tokenComment{{start: 3, end: 13}}
		require.NoError(t, agree([]span{{3, 13}}, tokens))
		for _, scanned := range [][]span{nil, {{3, 12}}, {{3, 13}, {20, 30}}} {
			assert.ErrorIs(t, agree(scanned, tokens), ErrCommentsUnlocated)
		}
	})
}

func TestHTMLLineText(t *testing.T) {
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
		{"<!-- a --!> after", 11, " a ", true},
		{"<!-- a -- b -->", 15, " a -- b ", true},
		{"<!-- spans on to the next line", 0, "", false},
		{"<p>", 0, "", false},
	} {
		t.Run(tc.line, func(t *testing.T) {
			n, text, ok := CommentProvider{}.LineText([]byte(tc.line))
			assert.Equal(t, tc.ok, ok)
			assert.Equal(t, tc.n, n)
			assert.Equal(t, tc.text, text)
		})
	}
}

func TestHTMLDirectiveFixtureCoversEveryForm(t *testing.T) {
	got, err := LocateComments([]byte(directiveFixture))
	require.NoError(t, err)
	assert.Empty(t, got.Comments)
	for _, name := range append([]string{"react"}, formNames()...) {
		assert.True(t, slices.ContainsFunc(got.Excluded, func(e comment.Excluded) bool { return e.Form == name }),
			"no line in the directive fixture exercises %q", name)
	}

	t.Run("must fail: a classifier missing a directive form", func(t *testing.T) {
		for i, f := range directiveForms {
			t.Run(f.Name, func(t *testing.T) {
				forms := slices.Delete(slices.Clone(directiveForms), i, i+1)
				got, err := locateComments([]byte(directiveFixture), func(inner string) (string, bool) { return classifyWith(inner, forms, true) })
				require.NoError(t, err)
				assert.NotEmpty(t, got.Comments, "without %q the fixture exposes a directive as prose", f.Name)
			})
		}
		t.Run("react", func(t *testing.T) {
			got, err := locateComments([]byte(directiveFixture), func(inner string) (string, bool) { return classifyWith(inner, directiveForms, false) })
			require.NoError(t, err)
			assert.NotEmpty(t, got.Comments)
		})
	})
}

func formNames() []string {
	names := make([]string, len(directiveForms))
	for i, f := range directiveForms {
		names[i] = f.Name
	}
	return names
}

func htmlSuite(p comment.Provider) commenttest.Suite {
	return commenttest.Suite{
		Provider: p,
		Scan:     htmlUnits,
		Fixtures: []commenttest.Fixture{
			{Name: "page.html", Source: pageFixture, Literals: pageLiterals},
			{Name: "crlf.html", Source: strings.ReplaceAll(pageFixture, "\n", "\r\n"), Literals: pageLiterals},
			{Name: "directives.html", Source: directiveFixture},
		},
	}
}

// TestProseP1_html is the P1 rung for HTML comments: the conformance suite over
// fixtures, the repository's HTML files accounted for against the HTML
// tokenizer, and providers broken on purpose that the suite must catch.
func TestProseP1_html(t *testing.T) {
	t.Run("the shared conformance suite", func(t *testing.T) {
		commenttest.Run(t, htmlSuite(CommentProvider{}))
	})

	t.Run("the repository's HTML files account for every comment", func(t *testing.T) {
		root := htmlRepoRoot(t)
		cmd := exec.CommandContext(t.Context(), "git", "ls-files", "-z", "*.html", "*.htm", "*.xhtml")
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
			units, err := htmlUnits(rel, src)
			require.NoError(t, err, rel)
			assert.NoError(t, commenttest.Err(commenttest.Account(src, got, units)), rel)
			files++
			comments += len(units)
		}
		t.Logf("html-comments files=%d comments=%d refused=%d", files, comments, refused)
		assert.Positive(t, files, "the corpus holds no HTML file the provider reads")
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
			{"a marker inside an attribute value counted", countAttributeMarker, commenttest.PropLiteral},
		} {
			t.Run(tc.name, func(t *testing.T) {
				failures := commenttest.Verify(htmlSuite(brokenHTML{edit: tc.edit}))
				require.NotEmpty(t, failures)
				assert.Contains(t, commenttest.Properties(failures), tc.want, "%v", commenttest.Err(failures))
			})
		}
	})
}

var declaredDirectives = comment.Directives{"okapi-skip:", "okapi-unmapped:"}

// declaredFixture holds declared markers where an HTML comment can open with
// one: as a whole comment, and in a comment after an element on its line. A
// marker inside a comment over several lines, and prose that only mentions a
// marker, stay prose.
const declaredFixture = `<body>
  <!-- okapi-skip: PageTest#testEmpty -->
  <p id="greeting">Hello</p> <!-- okapi-unmapped: PageTest#testTrailing -->
  <!--
    okapi-skip: inside a comment over several lines
  -->
  <p>Multi</p>
  <!-- See the okapi-skip: markers; OKAPI-SKIP: in capitals is prose too. -->
  <p>Sort</p>
</body>
`

// The conformance suite with directives declared: every comment that opens
// with one is set aside as a directive, and every other comment is accounted
// for as it is with nothing declared.
func TestHTMLConformanceWithDeclaredDirectives(t *testing.T) {
	s := htmlSuite(CommentProvider{})
	s.Directives = declaredDirectives
	s.Fixtures = append(s.Fixtures,
		commenttest.Fixture{Name: "declared.html", Source: declaredFixture},
		commenttest.Fixture{Name: "declared-crlf.html", Source: strings.ReplaceAll(declaredFixture, "\n", "\r\n")},
	)
	commenttest.Run(t, s)

	got, err := comment.Locate(CommentProvider{}, "declared.html", []byte(declaredFixture), declaredDirectives)
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

// htmlUnits is the conformance suite's scan for HTML: every `<!-- -->` comment
// the HTML tokenizer reports, at the bytes of its token.
func htmlUnits(_ string, src []byte) ([]commenttest.Unit, error) {
	z := html.NewTokenizer(bytes.NewReader(src))
	z.SetMaxBuf(0)
	var units []commenttest.Unit
	off := 0
	for {
		tt := z.Next()
		if tt == html.ErrorToken {
			if errors.Is(z.Err(), io.EOF) {
				return units, nil
			}
			return nil, z.Err()
		}
		raw := z.Raw()
		if tt == html.CommentToken && bytes.HasPrefix(raw, []byte(markup.Open)) {
			u := commenttest.Unit{Start: off, End: off + len(raw), Open: len(markup.Open), Group: len(units)}
			switch s := string(raw); {
			case s == "<!-->":
				u.Close = 1
			case s == "<!--->":
				u.Close = 2
			case strings.HasSuffix(s, "--!>"):
				u.Close = 4
			case strings.HasSuffix(s, "-->"):
				u.Close = 3
			}
			_, u.Directive = classify(string(raw[u.Open : len(raw)-u.Close]))
			units = append(units, u)
		}
		off += len(raw)
	}
}

// brokenHTML damages what the provider locates in every file with a comment,
// except the canary.
type brokenHTML struct {
	CommentProvider
	edit func([]byte, *comment.File)
}

func (b brokenHTML) Locate(name string, src []byte) (*comment.File, error) {
	f, err := b.CommentProvider.Locate(name, src)
	if err == nil && name != "canary" && len(f.Comments) > 0 {
		b.edit(src, f)
	}
	return f, err
}

// countAttributeMarker reports the comment marker inside an attribute value as
// a comment.
func countAttributeMarker(src []byte, f *comment.File) {
	const marker = "<!-- not a comment -->"
	at := bytes.Index(src, []byte(`title="`+marker))
	if at < 0 {
		return
	}
	at += len(`title="`)
	f.Comments = append(f.Comments, comment.Comment{
		Start: at, End: at + len(marker), Lines: format.NewLineIndex(src).Range(at, at+len(marker)),
		Style: comment.StyleBlock, Subject: "comment/p", Runs: []model.Run{model.TextR("not a comment")},
	})
	slices.SortFunc(f.Comments, func(a, b comment.Comment) int { return a.Start - b.Start })
}

func htmlRepoRoot(t *testing.T) string {
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
