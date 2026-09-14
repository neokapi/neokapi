package comments_test

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/comment"
	"github.com/neokapi/neokapi/core/comment/commenttest"
	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/plugin/commentproto"
	"github.com/neokapi/neokapi/plugins/sourcecode/internal/comments"
)

// Corpus floors per language. The accounting of each fixture is what catches a
// provider that loses a comment; the floors catch a corpus that shrank to
// nothing and still accounted for everything in it.
var corpusFloors = map[string]int{"typescript": 20, "tsx": 10, "javascript": 5, "python": 15, "bash": 20, "css": 15, "rust": 10}

// literalsOf lists, per fixture, the substrings holding a comment marker as
// content. No comment or exclusion may overlap one.
var literalsOf = map[string][]string{
	"literals.ts.txt":  {`"// not a comment"`, `'/* not a comment */'`, "`// not a comment ${", "`https://example.com/path`", `/\/\/ not a comment/g`},
	"literals.tsx.txt": {`"// not a comment"`, `<p>// not a comment</p>`, `<p>/* not a comment */</p>`},
	"literals.mjs.txt": {`"// not a comment"`, "`/* not a comment */ ${", `/\/\* not a comment \*\//`},
	"unicode.ts.txt":   {`"héllo // ✓"`},
	"crlf.ts.txt":      {`"// not a comment"`},
	"canary.ts.txt":    {`"// not a comment"`},
	"canary.tsx.txt":   {`<p>// not a comment `},
	"canary.mjs.txt":   {"`// not a comment`"},
	"literals.py.txt":  {`"# not a comment"`, `'# not a comment either'`, "\"\"\"\n# not a comment inside a docstring\n\"\"\"", `f"{greeting} # not a comment in an f-string"`},
	"unicode.py.txt":   {`"héllo # ✓"`},
	"crlf.py.txt":      {`"# not a comment"`},
	"canary.py.txt":    {`"# not a comment"`},
	"literals.sh.txt":  {`"# not a comment"`, `'# not a comment either'`, `${greeting#prefix}`, `word#not-a-comment`, `$'# not a comment in an ANSI-C string'`, "# not a comment inside a heredoc"},
	"canary.sh.txt":    {`"# not a comment"`},
	"literals.css.txt": {`"/* not a comment */"`, `'/* not a comment either */'`, `url(/*not-a-comment*/)`, `url(a/*not*/b.png)`},
	"canary.css.txt":   {`"/* not a comment */"`},
	"literals.rs.txt":  {`"// not a comment"`, `r#"/* not a comment */"#`, `b"// not a comment either"`, `'/'`, `"a \" // not a comment after an escaped quote"`, `r"// not a comment in a raw string"`, `"/* not a comment */"`, `"// not a comment in a macro"`},
	"unicode.rs.txt":   {`"héllo // ✓"`},
	"crlf.rs.txt":      {`"// not a comment"`},
	"canary.rs.txt":    {`"// not a comment"`},
	"tokenizer.rs.txt": {`"name = \"// here\""`, `Token::Str("// here")`},
}

// directiveFixtures names, per language, a fixture of directives alone that
// holds a comment of every form the language's syntax tests.
var directiveFixtures = map[string]string{
	"typescript": "directives.ts.txt",
	"python":     "directives.py.txt",
	"bash":       "directives.sh.txt",
	"css":        "directives.css.txt",
	"rust":       "directives.rs.txt",
}

// declaredFixtures names, per language, a fixture carrying declaredDirectives,
// and how many of its comments open with one.
var declaredFixtures = map[string]struct {
	name  string
	forms int
}{
	"typescript": {"declared.ts.txt", 5},
	"python":     {"declared.py.txt", 4},
	"bash":       {"declared.sh.txt", 4},
	"css":        {"declared.css.txt", 3},
	"rust":       {"declared.rs.txt", 5},
}

// generatedFixtures names, per language, the fixtures a generator's header
// marks as generated.
var generatedFixtures = map[string][]string{
	"typescript": {"src__contract.gen.ts.txt", "embeds__types.ts.txt", "internal__eventdata.d.ts.txt"},
	"python":     {"generated.py.txt"},
	"bash":       {"generated.sh.txt"},
	"css":        {"generated.css.txt"},
	"rust":       {"generated.rs.txt"},
}

// unparsed holds, per language, a file with a syntax error on its second line.
var unparsed = map[string]string{
	"typescript": "// Parses.\nexport function parse( {\n",
	"python":     "# Parses.\ndef parse(\n",
	"bash":       "# Builds.\nbuild() {\n",
	"css":        "/* Styles. */\n.header { color: red;\n",
	"rust":       "/// Parses.\nfn parse( {\n",
}

// fixture is one corpus file and, for an oracle that records its spans, the
// golden beside it.
type fixture struct {
	name   string
	src    []byte
	golden golden
}

// corpus reads a language's fixtures. For an oracle that records its spans, a
// fixture without a golden, or whose golden records other bytes, fails the
// test: an oracle that cannot vouch for the bytes it is compared with proves
// nothing.
func corpus(t *testing.T, language string) []fixture {
	t.Helper()
	o, ok := oracles[language]
	require.True(t, ok, "no oracle reads %s", language)
	paths, err := filepath.Glob(filepath.Join("testdata", "corpus", language, "*.txt"))
	require.NoError(t, err)
	require.NotEmpty(t, paths, "no %s fixtures", language)
	var out []fixture
	for _, p := range paths {
		src, err := os.ReadFile(p)
		require.NoError(t, err)
		fx := fixture{name: filepath.Base(p), src: src}
		if o.golden != "" {
			fx.golden, err = readGolden(p, o)
			require.NoError(t, err)
			require.Equal(t, fx.golden.sha256, sum(src), "%s changed since its %s golden was written; run %s", fx.name, o.name, o.script)
		}
		out = append(out, fx)
	}
	return out
}

// provider reads a language through the wire the host reads it through: what
// the package locates is carried as a LocateComments response and read back.
type provider struct {
	lang   comments.Language
	locate func(language, name string, src []byte) (*comment.File, error)
}

func newProvider(t *testing.T, language string) provider {
	t.Helper()
	lang, ok := comments.Lookup(language)
	require.True(t, ok, "no %s language", language)
	return provider{lang: lang, locate: comments.Locate}
}

func (p provider) Language() string       { return p.lang.Name }
func (p provider) Extensions() []string   { return p.lang.Extensions }
func (p provider) Canary() comment.Canary { return p.lang.Canary }

func (p provider) LineText(line []byte) (int, string, bool) { return p.lang.Markers.LineText(line) }

func (p provider) Locate(name string, src []byte) (*comment.File, error) {
	f, err := p.locate(p.lang.Name, name, src)
	if err != nil {
		return nil, err
	}
	return commentproto.FromProto(p.lang.Name, len(src), commentproto.ToProto(f))
}

// suite is the conformance suite over a language's corpus. Its scan is the
// language's oracle: the golden recorded for exactly the bytes it is given, the
// canary included, or the oracle read in the test.
func suite(t *testing.T, language string, p comment.Provider) commenttest.Suite {
	t.Helper()
	o := oracles[language]
	fixtures := corpus(t, language)
	goldens := map[string]golden{}
	s := commenttest.Suite{Provider: p}
	for _, fx := range fixtures {
		goldens[fx.golden.sha256] = fx.golden
		s.Fixtures = append(s.Fixtures, commenttest.Fixture{Name: fx.name, Source: string(fx.src), Literals: literalsOf[fx.name]})
	}
	s.Scan = func(name string, src []byte) ([]commenttest.Unit, error) {
		if o.scan != nil {
			spans, err := o.scan(src)
			if err != nil {
				return nil, fmt.Errorf("%s could not read %s: %w", o.name, name, err)
			}
			return o.units(src, spans), nil
		}
		g, ok := goldens[sum(src)]
		if !ok {
			return nil, fmt.Errorf("no %s golden records the bytes of %s; add it to testdata/corpus and run %s", o.name, name, o.script)
		}
		return o.units(src, g.spans), nil
	}
	return s
}

// broken damages what a provider locates in every fixture, but not in the canary.
type broken struct {
	provider
	edit func(src []byte, f *comment.File)
}

func (b broken) Locate(name string, src []byte) (*comment.File, error) {
	f, err := b.provider.Locate(name, src)
	if err == nil && name != "canary" {
		b.edit(src, f)
	}
	return f, err
}

// canaryless is a provider with no canary to give.
type canaryless struct{ provider }

func (canaryless) Canary() comment.Canary { return comment.Canary{} }

// declaredDirectives are the markers the declared fixtures carry, declared as a
// recipe declares them.
var declaredDirectives = comment.Directives{"okapi-skip:", "okapi-unmapped:"}

// unmarked reads no comment line as whole.
type unmarked struct{ provider }

func (unmarked) LineText([]byte) (int, string, bool) { return 0, "", false }

// proseP1 is the P1 rung for one language: the shared conformance suite over a
// corpus drawn from this repository, every fixture held to the language's
// oracle, and a provider broken each way the suite must notice.
func proseP1(t *testing.T, language string) {
	p := newProvider(t, language)

	t.Run("the shared conformance suite", func(t *testing.T) {
		commenttest.Run(t, suite(t, language, p))
	})

	t.Run("the corpus locates comments and is not a sliver", func(t *testing.T) {
		fixtures := corpus(t, language)
		assert.GreaterOrEqual(t, len(fixtures), corpusFloors[language], "the %s corpus shrank", language)
		located, excluded := 0, 0
		for _, fx := range fixtures {
			f, err := p.Locate(fx.name, fx.src)
			require.NoError(t, err, fx.name)
			located += len(f.Comments)
			excluded += len(f.Excluded)
		}
		t.Logf("prose-corpus %s fixtures=%d comments=%d excluded=%d", language, len(fixtures), located, excluded)
		assert.Positive(t, located, "the corpus located no comment")
	})

	t.Run("must fail: the conformance suite catches a broken provider", func(t *testing.T) {
		for _, tc := range []struct {
			name string
			p    comment.Provider
			want commenttest.Property
		}{
			{"a span one byte short", broken{p, func(_ []byte, f *comment.File) {
				if len(f.Comments) > 0 {
					f.Comments[0].End--
				}
			}}, commenttest.PropSpan},
			{"a marker inside a string or template literal counted", broken{p, countLiteralMarker}, commenttest.PropLiteral},
			{"the directive filter dropped", provider{lang: p.lang, locate: func(language, name string, src []byte) (*comment.File, error) {
				return comments.LocateWithout(language, name, src)
			}}, commenttest.PropAccount},
			{"no canary", canaryless{p}, commenttest.PropCanary},
		} {
			t.Run(tc.name, func(t *testing.T) {
				failures := commenttest.Verify(suite(t, language, tc.p))
				require.NotEmpty(t, failures)
				assert.Contains(t, commenttest.Properties(failures), tc.want, "%v", commenttest.Err(failures))
			})
		}
	})

	if name, ok := directiveFixtures[language]; ok {
		t.Run("each directive form is set aside, and dropping any one lets it through", func(t *testing.T) {
			src := fixtureBytes(t, language, name)
			got, err := comments.Locate(language, name, src)
			require.NoError(t, err)
			assert.Empty(t, got.Comments, "a file of directives has no prose")
			require.NotEmpty(t, got.Excluded)
			for _, e := range got.Excluded {
				assert.Equal(t, comment.ReasonDirective, e.Reason, "%q", src[e.Start:e.End])
				assert.NotEmpty(t, e.Form, "%q", src[e.Start:e.End])
			}
			forms := comments.DirectiveForms(language)
			require.NotEmpty(t, forms)
			for _, form := range forms {
				t.Run("must fail: without "+form, func(t *testing.T) {
					got, err := comments.LocateWithout(language, name, src, form)
					require.NoError(t, err)
					assert.NotEmpty(t, got.Comments, "without %q the directive fixture should expose a directive as prose", form)
				})
			}
		})
	}

	if declared, ok := declaredFixtures[language]; ok {
		t.Run("declared directives are set aside through the provider's LineText", func(t *testing.T) {
			s := suite(t, language, newProvider(t, language))
			s.Directives = declaredDirectives
			commenttest.Run(t, s)

			got, err := comment.Locate(newProvider(t, language), declared.name, fixtureBytes(t, language, declared.name), declaredDirectives)
			require.NoError(t, err)
			forms := 0
			for _, e := range got.Excluded {
				if e.Reason == comment.ReasonDirective && slices.Contains(declaredDirectives, e.Form) {
					forms++
				}
			}
			assert.Equal(t, declared.forms, forms, "the suite ran over the declared markers")
		})

		t.Run("must fail: a provider whose LineText reads no line", func(t *testing.T) {
			s := suite(t, language, unmarked{newProvider(t, language)})
			s.Directives = declaredDirectives
			failures := commenttest.Verify(s)
			require.NotEmpty(t, failures)
			assert.Contains(t, commenttest.Properties(failures), commenttest.PropDirective, "%v", commenttest.Err(failures))
			assert.Contains(t, commenttest.Properties(failures), commenttest.PropLineText, "%v", commenttest.Err(failures))
		})
	}

	if names, ok := generatedFixtures[language]; ok {
		t.Run("a generated file's comments are all set aside", func(t *testing.T) {
			for _, name := range names {
				got, err := comments.Locate(language, name, fixtureBytes(t, language, name))
				require.NoError(t, err)
				assert.Empty(t, got.Comments, name)
				require.NotEmpty(t, got.Excluded, name)
				for _, e := range got.Excluded {
					assert.Equal(t, comment.ReasonGenerated, e.Reason, name)
				}
			}
		})
	}

	if src, ok := unparsed[language]; ok {
		t.Run("a file that does not parse is unlocated", func(t *testing.T) {
			_, err := comments.Locate(language, "broken", []byte(src))
			require.ErrorIs(t, err, comment.ErrUnlocated)
			assert.Contains(t, err.Error(), "line 2")
		})
	}
}

// countLiteralMarker reports a comment marker inside a literal as a comment.
func countLiteralMarker(src []byte, f *comment.File) {
	for _, marker := range []string{"// not a comment", "/* not a comment */", "# not a comment"} {
		at := strings.Index(string(src), marker)
		if at < 0 {
			continue
		}
		end := at + len(marker)
		f.Comments = append(f.Comments, comment.Comment{
			Start: at, End: end, Lines: format.NewLineIndex(src).Range(at, end),
			Style: comment.StyleLine, Subject: "comment", Runs: []model.Run{model.TextR("not a comment")},
		})
		slices.SortFunc(f.Comments, func(a, b comment.Comment) int { return a.Start - b.Start })
		return
	}
}

func TestProseP1_typescript(t *testing.T) {
	proseP1(t, "typescript")

	t.Run("a doc comment sits directly before a declaration", func(t *testing.T) {
		src := fixtureBytes(t, "typescript", "doc.ts.txt")
		assert.Empty(t, subjectMismatches(t, newProvider(t, "typescript"), "doc.ts", src, docSubjects))
	})

	t.Run("must fail: every /** */ treated as a doc comment", func(t *testing.T) {
		src := fixtureBytes(t, "typescript", "doc.ts.txt")
		p := broken{newProvider(t, "typescript"), func(src []byte, f *comment.File) {
			for i := range f.Comments {
				if strings.HasPrefix(string(src[f.Comments[i].Start:]), "/**") {
					f.Comments[i].Doc = true
				}
			}
		}}
		assert.NotEmpty(t, subjectMismatches(t, p, "doc.ts", src, docSubjects))
	})

	t.Run("JSDoc tags, links, code and URLs are placeholders", func(t *testing.T) {
		got, err := newProvider(t, "typescript").Locate("doc.ts", fixtureBytes(t, "typescript", "doc.ts.txt"))
		require.NoError(t, err)
		read := commentOn(t, got, "func/read")
		assert.Equal(t, "Reads a file.\n\nthe file to read\nthe file's text, decoded as UTF-8\n", model.RunsText(read.Runs))
		assert.Equal(t, []string{"@param {string} path - ", "@returns ", "@example\nread(\"kapi.yaml\");"}, placeholders(read.Runs))

		options := commentOn(t, got, "interface/Options")
		assert.True(t, options.Deprecated, "a @deprecated tag marks the comment")
		assert.Equal(t, []string{"@deprecated ", "{@link ParseConfig}", "https://example.com/parse"}, placeholders(options.Runs))
		assert.Equal(t, "Options for a parse.\n\nUse  and see .", model.RunsText(options.Runs))

		keep := commentOn(t, got, "interface/Options/keep")
		assert.Equal(t, []string{"`whitespace`"}, placeholders(keep.Runs))
		assert.Equal(t, "Whether to keep .", model.RunsText(keep.Runs))
	})

	t.Run("a comment that names no generator is prose", func(t *testing.T) {
		// Its first comment speaks of "the generated contract" and names no
		// generator.
		got, err := comments.Locate("typescript", "types.ts", fixtureBytes(t, "typescript", "review__types.ts.txt"))
		require.NoError(t, err)
		assert.NotEmpty(t, got.Comments)
	})
}

func TestProseP1_tsx(t *testing.T) {
	proseP1(t, "tsx")

	t.Run("a JSX comment is a comment and JSX text is not", func(t *testing.T) {
		got, err := comments.Locate("tsx", "literals.tsx", fixtureBytes(t, "tsx", "literals.tsx.txt"))
		require.NoError(t, err)
		var texts []string
		for _, c := range got.Comments {
			texts = append(texts, c.Subject+": "+model.RunsText(c.Runs))
		}
		assert.Equal(t, []string{"func/Markers: Shows comment markers as text.", "func/Markers/comment: A JSX comment is a comment."}, texts)
	})
}

func TestProseP1_javascript(t *testing.T) {
	proseP1(t, "javascript")

	t.Run("the shebang is a directive", func(t *testing.T) {
		src := fixtureBytes(t, "javascript", "kapi-format__build.mjs.txt")
		got, err := comments.Locate("javascript", "build.mjs", src)
		require.NoError(t, err)
		require.NotEmpty(t, got.Excluded)
		assert.Equal(t, comment.Excluded{Start: 0, End: strings.IndexByte(string(src), '\n'), Lines: format.LineRange{First: 1, Last: 1}, Reason: comment.ReasonDirective, Form: "shebang"}, got.Excluded[0])
		assert.NotEmpty(t, got.Comments)
	})
}

func TestProseP1_python(t *testing.T) {
	proseP1(t, "python")
	proseSubjects(t, "python", "subjects.py.txt", []string{
		"comment doc=false",
		"comment doc=false",
		"var/LIMIT doc=false",
		"func/parse doc=false",
		"func/parse/comment doc=false",
		"class/Parser/source doc=false",
		"class/Parser/run doc=false",
		"class/Parser/run/comment doc=false",
		"class/Parser/comment doc=false",
	})

	t.Run("an encoding declaration is a directive on the first two lines only", func(t *testing.T) {
		got, err := comments.Locate("python", "coding.py", []byte("#!/usr/bin/env python3\n# vim: set fileencoding=utf-8 :\n"))
		require.NoError(t, err)
		assert.Empty(t, got.Comments)
		require.Len(t, got.Excluded, 2)
		assert.Equal(t, "coding", got.Excluded[1].Form)

		got, err = comments.Locate("python", "coding.py", []byte("x = 1\ny = 2\n# vim: set fileencoding=utf-8 :\n"))
		require.NoError(t, err)
		assert.Len(t, got.Comments, 1, "on the third line the declaration is prose")
	})
}

func TestProseP1_bash(t *testing.T) {
	proseP1(t, "bash")
	proseSubjects(t, "bash", "subjects.sh.txt", []string{
		"comment doc=false",
		"comment doc=false",
		"var/OUT doc=false",
		"var/VERSION doc=false",
		"func/build doc=false",
		"func/build/comment doc=false",
		"func/build/comment doc=false",
		"comment doc=false",
	})
}

func TestProseP1_css(t *testing.T) {
	proseP1(t, "css")
	proseSubjects(t, "css", "nested.css.txt", []string{
		"media doc=false",
		"media/rule/.c doc=false",
		"media/rule/.c/comment doc=false",
		"rule/.e doc=false",
		"comment doc=false",
	})
}

func TestProseP1_rust(t *testing.T) {
	proseP1(t, "rust")
	proseSubjects(t, "rust", "doc.rs.txt", rustDocSubjects)

	t.Run("must fail: every comment with a doc marker read as documentation", func(t *testing.T) {
		src := fixtureBytes(t, "rust", "doc.rs.txt")
		p := broken{newProvider(t, "rust"), func(src []byte, f *comment.File) {
			for i := range f.Comments {
				text := string(src[f.Comments[i].Start:])
				if strings.HasPrefix(text, "///") || strings.HasPrefix(text, "/**") {
					f.Comments[i].Doc = true
				}
			}
		}}
		assert.NotEmpty(t, subjectMismatches(t, p, "doc.rs.txt", src, rustDocSubjects))
	})

	t.Run("an inner doc comment documents the module it sits in", func(t *testing.T) {
		assert.Empty(t, subjectMismatches(t, newProvider(t, "rust"), "module.rs.txt", fixtureBytes(t, "rust", "module.rs.txt"), []string{
			"module doc=true",
			"module doc=true",
			"mod/lexer doc=true",
			"mod/lexer/func/lex doc=false",
		}))
	})

	t.Run("four slashes and three asterisks make plain comments", func(t *testing.T) {
		src := fixtureBytes(t, "rust", "edge.rs.txt")
		assert.Empty(t, subjectMismatches(t, newProvider(t, "rust"), "edge.rs.txt", src, []string{
			"func/four doc=false",
			"func/three doc=false",
			"func/edges doc=true",
			"func/fenced doc=true",
		}))
		got, err := newProvider(t, "rust").Locate("edge.rs", src)
		require.NoError(t, err)
		edges := commentOn(t, got, "func/edges")
		assert.Equal(t, "A doc comment with blank lines at its edges.", model.RunsText(edges.Runs))
	})
}

// rustDocSubjects are the subjects and doc flags doc.rs declares.
var rustDocSubjects = []string{
	"module doc=true",
	"comment doc=false",
	"struct/Point doc=true",
	"struct/Point/x doc=true",
	"struct/Point/y doc=false",
	"enum/Color doc=true",
	"enum/Color/Red doc=true",
	"enum/Color/comment doc=false",
	"trait/Shape doc=true",
	"trait/Shape/area doc=true",
	"impl/Point/origin doc=true",
	"impl/Point/origin/comment doc=false",
	"impl/fmt::Display for Point/fmt doc=true",
	"const/LIMIT doc=false",
	"mod/util doc=true",
	"mod/util/func/clamp doc=true",
}

// proseSubjects holds a language's comments to the subjects a fixture
// declares, and a provider that reads a comment above a declaration as its
// documentation to failing them. Only a documentation comment a language
// defines, such as a JSDoc block, documents a declaration.
func proseSubjects(t *testing.T, language, name string, want []string) {
	t.Run("a comment carries the subject of what it sits on", func(t *testing.T) {
		assert.Empty(t, subjectMismatches(t, newProvider(t, language), name, fixtureBytes(t, language, name), want))
	})

	t.Run("must fail: a comment above a declaration read as its documentation", func(t *testing.T) {
		p := broken{newProvider(t, language), func(_ []byte, f *comment.File) {
			for i := range f.Comments {
				f.Comments[i].Doc = !strings.HasSuffix(f.Comments[i].Subject, "comment")
			}
		}}
		assert.NotEmpty(t, subjectMismatches(t, p, name, fixtureBytes(t, language, name), want))
	})
}

// docSubjects are the subjects and doc flags doc.ts declares.
var docSubjects = []string{
	"comment doc=false",
	"func/parse doc=true",
	"func/parse/comment doc=false",
	"class/Parser doc=true",
	"class/Parser/source doc=true",
	"class/Parser/run doc=false",
	"class/Parser/stop doc=false",
	"comment doc=false",
	"interface/Options doc=true",
	"interface/Options/keep doc=true",
	"enum/Kind doc=true",
	"enum/Kind/Text doc=true",
	"type/Id doc=true",
	"namespace/Util/const/join doc=true",
	"global/interface/Window doc=true",
	"func/read doc=true",
}

// subjectMismatches locates a fixture and lists every comment whose subject or
// doc flag differs from want.
func subjectMismatches(t *testing.T, p comment.Provider, name string, src []byte, want []string) []string {
	t.Helper()
	got, err := p.Locate(name, src)
	require.NoError(t, err)
	var have []string
	for _, c := range got.Comments {
		have = append(have, fmt.Sprintf("%s doc=%v", c.Subject, c.Doc))
	}
	var mismatches []string
	for i := range max(len(want), len(have)) {
		w, h := "<none>", "<none>"
		if i < len(want) {
			w = want[i]
		}
		if i < len(have) {
			h = have[i]
		}
		if w != h {
			mismatches = append(mismatches, fmt.Sprintf("comment %d: want %s, have %s", i, w, h))
		}
	}
	return mismatches
}

func fixtureBytes(t *testing.T, language, name string) []byte {
	t.Helper()
	src, err := os.ReadFile(filepath.Join("testdata", "corpus", language, name))
	require.NoError(t, err)
	return src
}

func commentOn(t *testing.T, f *comment.File, subject string) comment.Comment {
	t.Helper()
	for _, c := range f.Comments {
		if c.Subject == subject {
			return c
		}
	}
	require.Failf(t, "subject not found", "no comment on %q", subject)
	return comment.Comment{}
}

func placeholders(runs []model.Run) []string {
	var out []string
	for _, r := range runs {
		if r.Ph != nil {
			out = append(out, r.Ph.Data)
		}
	}
	return out
}
