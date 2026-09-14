package mdx

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

	"github.com/neokapi/neokapi/core/comment"
	"github.com/neokapi/neokapi/core/comment/commenttest"
	"github.com/neokapi/neokapi/core/comment/markup"
	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
)

const pageFixture = "---\n" +
	"title: \"{/* not a comment */}\"\n" +
	"---\n" +
	"\n" +
	"import Tabs from \"@theme/Tabs\";\n" +
	"\n" +
	"{/* The page head. */}\n" +
	"\n" +
	"# Install\n" +
	"\n" +
	"{/* truncate */}\n" +
	"\n" +
	"Use `{/* not a comment */}` in a code span.\n" +
	"\n" +
	"```mdx\n" +
	"{/* not a comment */}\n" +
	"```\n" +
	"\n" +
	"## From Homebrew\n" +
	"\n" +
	"{/*\n" +
	"  Spans several lines,\n" +
	"  and ends here.\n" +
	"*/}\n" +
	"\n" +
	"export const meta = {/* not a comment */ title: \"x\"};\n" +
	"\n" +
	"{/* prettier-ignore */}\n" +
	"\n" +
	"{/*   */}\n"

// pageLiterals hold comment markers that are content: in front matter, a code
// span, a fenced code block and an ESM statement.
var pageLiterals = []string{"{/* not a comment */"}

const generatedFixture = "---\n" +
	"title: generated\n" +
	"---\n" +
	"\n" +
	"{/*\n" +
	"  GENERATED FILE. DO NOT EDIT.\n" +
	"  Produced by a generator.\n" +
	"*/}\n" +
	"\n" +
	"# Page\n" +
	"\n" +
	"{/* A comment the generator wrote. */}\n"

// directiveFixture has one line for each directive form, matched by that form
// alone.
const directiveFixture = "{/* truncate */}\n" +
	"\n" +
	"{/* prettier-ignore */}\n" +
	"\n" +
	"{/* markdownlint-disable MD033 */}\n" +
	"\n" +
	"{/* eslint-disable-next-line no-unused-vars */}\n"

func TestLocateMDXCommentsFindsEveryCommentWithItsSpan(t *testing.T) {
	src := []byte(pageFixture)
	got, err := LocateComments(src)
	require.NoError(t, err)
	assert.Equal(t, "mdx", got.Language)

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
		{"comment/document", "The page head.", format.LineRange{First: 7, Last: 7}, "{/* The page head. */}"},
		{"comment/install/from-homebrew", "Spans several lines,\nand ends here.", format.LineRange{First: 21, Last: 24}, "{/*\n  Spans several lines,\n  and ends here.\n*/}"},
	}, comments)

	var excluded []string
	for _, e := range got.Excluded {
		excluded = append(excluded, string(e.Reason)+" "+e.Form+" "+string(src[e.Start:e.End]))
	}
	assert.Equal(t, []string{
		"directive truncate {/* truncate */}",
		"directive prettier-ignore {/* prettier-ignore */}",
		"blank  {/*   */}",
	}, excluded)
}

func TestLocateMDXCommentsSetsAsideAGeneratedFile(t *testing.T) {
	got, err := LocateComments([]byte(generatedFixture))
	require.NoError(t, err)
	assert.Empty(t, got.Comments)
	require.Len(t, got.Excluded, 2)
	for _, e := range got.Excluded {
		assert.Equal(t, comment.ReasonGenerated, e.Reason)
	}
}

// A JSX element's children are Markdown, so a comment marker in their code is
// content, as a page with tabs quotes one.
func TestLocateMDXCommentsReadsCodeInJSXChildrenAsContent(t *testing.T) {
	src := []byte("<Tabs>\n<TabItem value=\"cli\" label=\"CLI\">\n\n## Set up\n\nThe section sits between `<!-- kapi:voice -->` markers, and `{/* note */}` is an expression.\n\n</TabItem>\n</Tabs>\n\n{/* After the tabs. */}\n")
	got, err := LocateComments(src)
	require.NoError(t, err)
	require.Len(t, got.Comments, 1)
	assert.Equal(t, "{/* After the tabs. */}", string(src[got.Comments[0].Start:got.Comments[0].End]))
	assert.Equal(t, "comment/set-up", got.Comments[0].Subject, "a heading in a JSX element's children opens a section")
	units, err := mdxUnits("tabs.mdx", src)
	require.NoError(t, err)
	assert.NoError(t, commenttest.Err(commenttest.Account(src, got, units)))
}

func TestLocateMDXCommentsRefusesWhatItCannotPlace(t *testing.T) {
	for name, src := range map[string]string{
		"an expression comment inside JSX":   "<Tabs>\n{/* note */}\n</Tabs>\n",
		"an expression comment in a line":    "Hello {/* note */} world.\n",
		"an HTML comment":                    "Hello.\n\n<!-- note -->\n",
		"two comments in one expression":     "{/* a */ /* b */}\n",
		"text after a comment on its line":   "{/* a */} and more\n",
		"an expression that is never closed": "{/* never closed\n",
	} {
		t.Run(name, func(t *testing.T) {
			_, err := LocateComments([]byte(src))
			require.ErrorIs(t, err, ErrCommentsUnlocated)
			require.ErrorIs(t, err, comment.ErrUnlocated, "the host treats any provider's unlocated file the same way")
		})
	}
}

func TestMDXLineText(t *testing.T) {
	for _, tc := range []struct {
		line string
		n    int
		text string
		ok   bool
	}{
		{"{/* a */}", 9, " a ", true},
		{"{ /* a */ }", 11, " a ", true},
		{"{/**/}", 6, "", true},
		{"{/* a */} after", 9, " a ", true},
		{"{/* spans on to the next line", 0, "", false},
		{"{value}", 0, "", false},
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

func TestMDXDirectiveFixtureCoversEveryForm(t *testing.T) {
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

func mdxSuite(p comment.Provider) commenttest.Suite {
	return commenttest.Suite{
		Provider: p,
		Scan:     mdxUnits,
		Fixtures: []commenttest.Fixture{
			{Name: "page.mdx", Source: pageFixture, Literals: pageLiterals},
			{Name: "crlf.mdx", Source: strings.ReplaceAll(pageFixture, "\n", "\r\n"), Literals: pageLiterals},
			{Name: "generated.mdx", Source: generatedFixture},
			{Name: "directives.mdx", Source: directiveFixture},
		},
	}
}

// TestProseP1_mdx is the P1 rung for MDX comments: the conformance suite over
// fixtures, the repository's MDX files accounted for against a line scan made
// without the MDX reader, and providers broken on purpose that the suite must
// catch.
func TestProseP1_mdx(t *testing.T) {
	t.Run("the shared conformance suite", func(t *testing.T) {
		commenttest.Run(t, mdxSuite(CommentProvider{}))
	})

	t.Run("the repository's MDX files account for every comment", func(t *testing.T) {
		root := mdxRepoRoot(t)
		cmd := exec.CommandContext(t.Context(), "git", "ls-files", "-z", "*.mdx")
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
			units, err := mdxUnits(rel, src)
			require.NoError(t, err, rel)
			assert.NoError(t, commenttest.Err(commenttest.Account(src, got, units)), rel)
			files++
			comments += len(units)
		}
		t.Logf("mdx-comments files=%d comments=%d refused=%d", files, comments, refused)
		assert.Positive(t, files, "the corpus holds no MDX file the provider reads")
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
				failures := commenttest.Verify(mdxSuite(brokenMDX{edit: tc.edit}))
				require.NotEmpty(t, failures)
				assert.Contains(t, commenttest.Properties(failures), tc.want, "%v", commenttest.Err(failures))
			})
		}
	})
}

var declaredDirectives = comment.Directives{"okapi-skip:", "okapi-unmapped:"}

// declaredFixture holds a declared marker opening a whole expression comment.
// A marker inside a comment over several lines, and prose that only mentions a
// marker, stay prose.
const declaredFixture = "# Page\n" +
	"\n" +
	"{/* okapi-skip: PageTest#testEmpty */}\n" +
	"\n" +
	"{/*\n" +
	"okapi-skip: inside a comment over several lines\n" +
	"*/}\n" +
	"\n" +
	"{/* See the okapi-skip: markers; OKAPI-SKIP: in capitals is prose too. */}\n"

// The conformance suite with directives declared: every comment that opens
// with one is set aside as a directive, and every other comment is accounted
// for as it is with nothing declared.
func TestMDXConformanceWithDeclaredDirectives(t *testing.T) {
	s := mdxSuite(CommentProvider{})
	s.Directives = declaredDirectives
	s.Fixtures = append(s.Fixtures,
		commenttest.Fixture{Name: "declared.mdx", Source: declaredFixture},
		commenttest.Fixture{Name: "declared-crlf.mdx", Source: strings.ReplaceAll(declaredFixture, "\n", "\r\n")},
	)
	commenttest.Run(t, s)

	got, err := comment.Locate(CommentProvider{}, "declared.mdx", []byte(declaredFixture), declaredDirectives)
	require.NoError(t, err)
	forms := 0
	for _, e := range got.Excluded {
		if e.Reason == comment.ReasonDirective {
			forms++
		}
	}
	assert.Equal(t, 1, forms, "the suite ran over the declared marker")
	assert.Len(t, got.Comments, 2, "the comment over several lines and the one that mentions a marker stay prose")
}

// mdxUnits is the conformance suite's scan for MDX, written without the MDX
// reader: outside a fenced code block, a line that opens at column 0 with `{/*`
// starts a comment that closes at the first `*/}`.
func mdxUnits(_ string, src []byte) ([]commenttest.Unit, error) {
	var units []commenttest.Unit
	fence := ""
	for lineStart := 0; lineStart < len(src); {
		next := len(src)
		if i := bytes.IndexByte(src[lineStart:], '\n'); i >= 0 {
			next = lineStart + i + 1
		}
		line := src[lineStart:next]
		trimmed := bytes.TrimLeft(line, " ")
		switch {
		case fence != "":
			if bytes.HasPrefix(trimmed, []byte(fence)) {
				fence = ""
			}
		case bytes.HasPrefix(trimmed, []byte("```")):
			fence = "```"
		case bytes.HasPrefix(trimmed, []byte("~~~")):
			fence = "~~~"
		case bytes.HasPrefix(line, []byte("{/*")):
			shut := bytes.Index(src[lineStart:], []byte("*/}"))
			if shut < 0 {
				return nil, errors.New("an expression comment that never closes")
			}
			end := lineStart + shut + len("*/}")
			u := commenttest.Unit{Start: lineStart, End: end, Open: len("{/*"), Close: len("*/}"), Group: len(units)}
			_, u.Directive = classifyComment(string(src[u.Start+u.Open : u.End-u.Close]))
			units = append(units, u)
			next = len(src)
			if i := bytes.IndexByte(src[end:], '\n'); i >= 0 {
				next = end + i + 1
			}
		}
		lineStart = next
	}
	return units, nil
}

// brokenMDX damages what the provider locates in every file with a comment,
// except the canary.
type brokenMDX struct {
	CommentProvider
	edit func([]byte, *comment.File)
}

func (b brokenMDX) Locate(name string, src []byte) (*comment.File, error) {
	f, err := b.CommentProvider.Locate(name, src)
	if err == nil && name != "canary" && len(f.Comments) > 0 {
		b.edit(src, f)
	}
	return f, err
}

// countCodeSpanMarker reports the comment marker inside a code span as a
// comment.
func countCodeSpanMarker(src []byte, f *comment.File) {
	const marker = "{/* not a comment */}"
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

func mdxRepoRoot(t *testing.T) string {
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
