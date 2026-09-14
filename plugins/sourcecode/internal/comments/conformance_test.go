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
var corpusFloors = map[string]int{"typescript": 20, "tsx": 10, "javascript": 5, "python": 15, "bash": 20, "css": 15, "rust": 10, "java": 15, "csharp": 9, "c": 11, "cpp": 8, "ruby": 9}

// literalsOf lists, per fixture, the substrings holding a comment marker as
// content. No comment or exclusion may overlap one.
var literalsOf = map[string][]string{
	"literals.ts.txt":   {`"// not a comment"`, `'/* not a comment */'`, "`// not a comment ${", "`https://example.com/path`", `/\/\/ not a comment/g`},
	"literals.tsx.txt":  {`"// not a comment"`, `<p>// not a comment</p>`, `<p>/* not a comment */</p>`},
	"literals.mjs.txt":  {`"// not a comment"`, "`/* not a comment */ ${", `/\/\* not a comment \*\//`},
	"unicode.ts.txt":    {`"héllo // ✓"`},
	"crlf.ts.txt":       {`"// not a comment"`},
	"canary.ts.txt":     {`"// not a comment"`},
	"canary.tsx.txt":    {`<p>// not a comment `},
	"canary.mjs.txt":    {"`// not a comment`"},
	"literals.py.txt":   {`"# not a comment"`, `'# not a comment either'`, "\"\"\"\n# not a comment inside a docstring\n\"\"\"", `f"{greeting} # not a comment in an f-string"`},
	"unicode.py.txt":    {`"héllo # ✓"`},
	"crlf.py.txt":       {`"# not a comment"`},
	"canary.py.txt":     {`"# not a comment"`},
	"literals.sh.txt":   {`"# not a comment"`, `'# not a comment either'`, `${greeting#prefix}`, `word#not-a-comment`, `$'# not a comment in an ANSI-C string'`, "# not a comment inside a heredoc"},
	"canary.sh.txt":     {`"# not a comment"`},
	"literals.css.txt":  {`"/* not a comment */"`, `'/* not a comment either */'`, `url(/*not-a-comment*/)`, `url(a/*not*/b.png)`},
	"canary.css.txt":    {`"/* not a comment */"`},
	"literals.rs.txt":   {`"// not a comment"`, `r#"/* not a comment */"#`, `b"// not a comment either"`, `'/'`, `"a \" // not a comment after an escaped quote"`, `r"// not a comment in a raw string"`, `"/* not a comment */"`, `"// not a comment in a macro"`},
	"unicode.rs.txt":    {`"héllo // ✓"`},
	"crlf.rs.txt":       {`"// not a comment"`},
	"canary.rs.txt":     {`"// not a comment"`},
	"tokenizer.rs.txt":  {`"name = \"// here\""`, `Token::Str("// here")`},
	"literals.java.txt": {`"// not a comment"`, `"/* not a comment */"`, `'/'`, "\"\"\"\n        // not a comment in a text block\n        /* nor this */\n        \"\"\"", `"a \" // not a comment after an escaped quote"`},
	"unicode.java.txt":  {`"héllo // ✓"`},
	"crlf.java.txt":     {`"// not a comment"`},
	"canary.java.txt":   {`"// not a comment"`},
	"literals.cs.txt":   {`"// not a comment"`, `@"/* not a comment */"`, `$"{1} // not a comment in an interpolated string"`, `'/'`, "\"\"\"\n        // not a comment in a raw string\n        \"\"\"", `@"a "" // not a comment after an escaped quote"`},
	"unicode.cs.txt":    {`"héllo // ✓"`},
	"crlf.cs.txt":       {`"// not a comment"`},
	"canary.cs.txt":     {`"// not a comment"`},
	"literals.c.txt":    {`"// not a comment"`, `"/* not a comment */"`, `'/'`, `"a \" // not a comment after an escaped quote"`},
	"lexical.c.txt":     {`'/'`, `'"'`, `'\''`, "\"a string \\\n// not a comment in a spliced string\""},
	"unicode.c.txt":     {`"héllo // ✓"`},
	"crlf.c.txt":        {`"// not a comment"`},
	"canary.c.txt":      {`"// not a comment"`},
	"lexical.cpp.txt":   {"R\"x(a)\" /* not a comment */ \")x\"", `u8R"(// not a comment)"`, `'/'`, `1'000`, `u8'/'`},
	"literals.cpp.txt":  {`"// not a comment"`, `R"(/* not a comment */)"`, "R\"delim(\n// not a comment in a raw string\n)delim\"", `u8"// not a comment either"`, `'/'`},
	"unicode.cpp.txt":   {`"héllo // ✓"`},
	"crlf.cpp.txt":      {`"// not a comment"`},
	"canary.cpp.txt":    {`R"(// not a comment)"`},
	"literals.rb.txt":   {`"# not a comment"`, `'# not a comment either'`, `"#{greeting} # not a comment after interpolation"`, `%q(# not a comment in a percent literal)`, `/# not a comment in a regexp/`, `?#`, "# not a comment inside a heredoc", "# not a comment after __END__"},
	"unicode.rb.txt":    {`"héllo # ✓"`},
	"crlf.rb.txt":       {`"# not a comment"`},
	"canary.rb.txt":     {`"# not a comment #{text}"`},
}

// directiveFixtures names, per language, a fixture of directives alone that
// holds a comment of every form the language's syntax tests.
var directiveFixtures = map[string]string{
	"typescript": "directives.ts.txt",
	"python":     "directives.py.txt",
	"bash":       "directives.sh.txt",
	"css":        "directives.css.txt",
	"rust":       "directives.rs.txt",
	"java":       "directives.java.txt",
	"csharp":     "directives.cs.txt",
	"c":          "directives.c.txt",
	"ruby":       "directives.rb.txt",
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
	"java":       {"declared.java.txt", 5},
	"csharp":     {"declared.cs.txt", 5},
	"c":          {"declared.c.txt", 5},
	"cpp":        {"declared.cpp.txt", 5},
	"ruby":       {"declared.rb.txt", 4},
}

// generatedFixtures names, per language, the fixtures a generator's header
// marks as generated.
var generatedFixtures = map[string][]string{
	"typescript": {"src__contract.gen.ts.txt", "embeds__types.ts.txt", "internal__eventdata.d.ts.txt"},
	"python":     {"generated.py.txt"},
	"bash":       {"generated.sh.txt"},
	"css":        {"generated.css.txt"},
	"rust":       {"generated.rs.txt"},
	"java":       {"generated.java.txt"},
	"csharp":     {"generated.cs.txt"},
	"c":          {"generated.c.txt"},
	"cpp":        {"generated.cpp.txt"},
	"ruby":       {"generated.rb.txt"},
}

// unparsed holds, per language, a file with a syntax error on its second line.
var unparsed = map[string]string{
	"typescript": "// Parses.\nexport function parse( {\n",
	"python":     "# Parses.\ndef parse(\n",
	"bash":       "# Builds.\nbuild() {\n",
	"css":        "/* Styles. */\n.header { color: red;\n",
	"rust":       "/// Parses.\nfn parse( {\n",
	"java":       "/** Parses. */\npublic class Parser {\n",
	"csharp":     "/// <summary>Parses.</summary>\npublic class Parser\n{\n",
	"ruby":       "# Parses.\ndef parse(\n",
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

func TestProseP1_java(t *testing.T) {
	proseP1(t, "java")
	proseSubjects(t, "java", "doc.java.txt", javaDocSubjects)

	t.Run("must fail: every /** */ treated as Javadoc", func(t *testing.T) {
		src := fixtureBytes(t, "java", "doc.java.txt")
		p := broken{newProvider(t, "java"), func(src []byte, f *comment.File) {
			for i := range f.Comments {
				if strings.HasPrefix(string(src[f.Comments[i].Start:]), "/**") {
					f.Comments[i].Doc = true
				}
			}
		}}
		assert.NotEmpty(t, subjectMismatches(t, p, "doc.java.txt", src, javaDocSubjects))
	})

	t.Run("Javadoc tags, inline tags, HTML and code are placeholders", func(t *testing.T) {
		got, err := newProvider(t, "java").Locate("doc.java", fixtureBytes(t, "java", "doc.java.txt"))
		require.NoError(t, err)
		parser := commentOn(t, got, "class/Parser")
		assert.Equal(t, []string{"<p>", "{@link java.io.Reader}", "<code>Recipe</code>", "@author Kapi", "@see Formatter"}, placeholders(parser.Runs))
		run := commentOn(t, got, "class/Parser/run")
		assert.Equal(t, []string{"<pre>\nnew Parser(\"x\").run();\n</pre>", "@return ", "@throws IllegalStateException "}, placeholders(run.Runs))
		assert.Equal(t, "Runs the parser.\n\n\n\nthe number of blocks\nwhen the parser has stopped", model.RunsText(run.Runs))
	})
}

// javaDocSubjects are the subjects and doc flags doc.java declares.
var javaDocSubjects = []string{
	"package doc=true",
	"comment doc=false",
	"class/Parser doc=true",
	"class/Parser/source doc=true",
	"class/Parser/count doc=false",
	"class/Parser/Parser doc=true",
	"class/Parser/run doc=true",
	"class/Parser/run/comment doc=false",
	"class/Parser/run/comment doc=false",
	"class/Parser/enum/Kind doc=true",
	"class/Parser/enum/Kind/TEXT doc=true",
	"class/Parser/record/Point doc=true",
	"class/Parser/interface/Visitor/visit doc=true",
	"annotation/Marker doc=true",
	"annotation/Marker/value doc=true",
}

func TestProseP1_csharp(t *testing.T) {
	proseP1(t, "csharp")
	proseSubjects(t, "csharp", "doc.cs.txt", csharpDocSubjects)

	t.Run("must fail: every /// treated as documentation", func(t *testing.T) {
		src := fixtureBytes(t, "csharp", "doc.cs.txt")
		p := broken{newProvider(t, "csharp"), func(src []byte, f *comment.File) {
			for i := range f.Comments {
				if strings.HasPrefix(string(src[f.Comments[i].Start:]), "///") {
					f.Comments[i].Doc = true
				}
			}
		}}
		assert.NotEmpty(t, subjectMismatches(t, p, "doc.cs.txt", src, csharpDocSubjects))
	})

	t.Run("XML documentation tags and code elements are placeholders", func(t *testing.T) {
		got, err := newProvider(t, "csharp").Locate("doc.cs", fixtureBytes(t, "csharp", "doc.cs.txt"))
		require.NoError(t, err)
		parser := commentOn(t, got, "class/Parser")
		assert.Equal(t, []string{"<summary>", `<see cref="Reader"/>`, "</summary>", "<remarks>", "<c>Recipe</c>", "</remarks>"}, placeholders(parser.Runs))
		assert.Equal(t, "Parses recipes with .\nReturns a .", model.RunsText(parser.Runs))
		run := commentOn(t, got, "class/Parser/Run")
		assert.Contains(t, placeholders(run.Runs), "<code>\nnew Parser(\"x\").Run();\n</code>")
	})

	t.Run("a namespace adds nothing to a subject", func(t *testing.T) {
		assert.Empty(t, subjectMismatches(t, newProvider(t, "csharp"), "scoped.cs.txt", fixtureBytes(t, "csharp", "scoped.cs.txt"), []string{
			"interface/IShape doc=true",
			"interface/IShape/Area doc=true",
			"struct/Pair doc=true",
			"struct/Pair/class/Inner doc=true",
			"delegate/Handler doc=true",
			"comment doc=false",
		}))
	})
}

// csharpDocSubjects are the subjects and doc flags doc.cs declares.
var csharpDocSubjects = []string{
	"comment doc=false",
	"class/Parser doc=true",
	"class/Parser/source doc=true",
	"class/Parser/count doc=false",
	"class/Parser/Parser doc=true",
	"class/Parser/Name doc=true",
	"class/Parser/Run doc=true",
	"class/Parser/Run/comment doc=false",
	"class/Parser/Run/comment doc=false",
	"class/Parser/Stopped doc=true",
	"enum/Kind doc=true",
	"enum/Kind/Text doc=true",
	"record/Point doc=true",
}

func TestProseP1_c(t *testing.T) {
	proseP1(t, "c")
	proseSubjects(t, "c", "doc.c.txt", cDocSubjects)

	t.Run("Doxygen commands are placeholders", func(t *testing.T) {
		got, err := newProvider(t, "c").Locate("doc.c", fixtureBytes(t, "c", "doc.c.txt"))
		require.NoError(t, err)
		parse := commentOn(t, got, "func/parse")
		assert.Equal(t, []string{"\\param text ", "\\return "}, placeholders(parse.Runs))
		assert.Equal(t, "Parses the input.\n\nthe text to read\nthe number of blocks", model.RunsText(parse.Runs))
		header := got.Comments[0]
		assert.Equal(t, []string{"@file parser.h", "@brief "}, placeholders(header.Runs), "a file command names a file")
	})

	t.Run("a file whose comments the grammar places apart from the lexical scan is unlocated", func(t *testing.T) {
		directives, err := os.ReadFile(filepath.Join("testdata", "unplaced", "preprocessor.c.txt"))
		require.NoError(t, err)
		for _, tc := range []struct {
			name, src, at string
		}{
			{"comments on directive lines", string(directives), "line 3, column 18"},
			{"a comment the grammar folds into a macro's value", "#define LIMIT 10 // after an object macro\n", "line 1, column 18"},
			{"a comment marker the grammar reads inside a macro's string", "#define MARKER \"/* not a comment */\"\n", "line 1, column 17"},
			{"a block comment opened across a line splice", "int x; /\\\n* a comment */\n", "line 1, column 8"},
			{"a line comment a splice with a space extends", "// runs on \\ \nonto this line\nint x;\n", "line 1, column 1"},
		} {
			t.Run(tc.name, func(t *testing.T) {
				_, err := comments.LexicalComments("c", []byte(tc.src))
				require.NoError(t, err, "the scan reads the file whole")
				_, err = comments.Locate("c", "unplaced.c", []byte(tc.src))
				require.ErrorIs(t, err, comment.ErrUnlocated)
				assert.Contains(t, err.Error(), "a comment at "+tc.at+" reads differently")
			})
		}
	})

	t.Run("a comment beside a syntax error documents nothing", func(t *testing.T) {
		for _, tc := range []struct {
			name, language, src, subject string
			doc                          bool
		}{
			{"before a declaration with no error", "c", "/** Parses the the input. */\nint parse(const char *text);\n", "func/parse", true},
			{"before a declaration the grammar cannot read", "c", "/** Parses the the input. */\nint parse(const char *text\n", "comment", false},
			{"before a declaration holding an error", "c", "/** Parses the input. */\nint parse(const char *text) { return 1 +; }\n", "comment", false},
			{"inside an error", "c", "KAPI_API(int) = {\n  /** Documents b. */\n  int b;\n", "comment", false},
			{"inside an error inside a namespace", "cpp", "namespace kapi {\nKAPI_API(int) = {\n  /** Documents b. */\n  int b;\n}\n", "comment", false},
			{"inside a declaration holding an error", "cpp", "namespace kapi {\nclass Parser {\n  KAPI_API(int) = {\n  /// Documents run.\n  void run();\n};\n}\n", "comment", false},
			{"before a declaration an earlier error leaves whole", "c", "int a = 1 +;\n/** Documents b. */\nint b;\n", "var/b", true},
			{"inside a namespace a later error leaves whole", "cpp", "namespace kapi {\nclass Parser {\n  /// Documents run.\n  void run();\n};\n}\nint a = 1 +;\n", "namespace/kapi/class/Parser/run", true},
		} {
			t.Run(tc.name, func(t *testing.T) {
				got, err := comments.Locate(tc.language, "parse."+tc.language, []byte(tc.src))
				require.NoError(t, err, "the tree places each comment where the scan does")
				require.Len(t, got.Comments, 1)
				assert.Equal(t, tc.subject, got.Comments[0].Subject)
				assert.Equal(t, tc.doc, got.Comments[0].Doc)
			})
		}
	})
}

// cDocSubjects are the subjects and doc flags doc.c declares.
var cDocSubjects = []string{
	"comment doc=false",
	"comment doc=false",
	"macro/PARSER_LIMIT doc=true",
	"type/point_t doc=true",
	"type/point_t/comment doc=false",
	"type/point_t/y doc=true",
	"enum/kind doc=true",
	"enum/kind/KIND_TEXT doc=true",
	"enum/kind/comment doc=false",
	"func/parse doc=true",
	"var/count doc=false",
	"func/parser_free doc=true",
	"func/parser_free/comment doc=false",
}

func TestProseP1_cpp(t *testing.T) {
	proseP1(t, "cpp")
	proseSubjects(t, "cpp", "doc.cpp.txt", cppDocSubjects)

	t.Run("a header the grammar cannot parse whole is located", func(t *testing.T) {
		src := fixtureBytes(t, "cpp", "macros.hpp.txt")
		got, err := comments.Locate("cpp", "macros.hpp", src)
		require.NoError(t, err, "macros and a brace split across #ifdef branches leave syntax errors in the tree")
		require.Len(t, got.Comments, 1)
		assert.Equal(t, "comment", got.Comments[0].Subject, "the declaration after the comment holds a syntax error, so what it declares is a guess")
		assert.False(t, got.Comments[0].Doc)
		require.Len(t, got.Excluded, 1)
		assert.Equal(t, "label", got.Excluded[0].Form, "the comment on the closing brace names the block it closes")
	})

	t.Run("closing labels are set aside, and each directive form is too", func(t *testing.T) {
		src := fixtureBytes(t, "cpp", "directives.cpp.txt")
		got, err := comments.Locate("cpp", "directives.cpp", src)
		require.NoError(t, err)
		assert.Empty(t, got.Comments)
		for _, form := range comments.DirectiveForms("cpp") {
			if form != "clang-format" && form != "label" {
				continue
			}
			t.Run("must fail: without "+form, func(t *testing.T) {
				without, err := comments.LocateWithout("cpp", "directives.cpp", src, form)
				require.NoError(t, err)
				assert.NotEmpty(t, without.Comments)
			})
		}
	})
}

// cppDocSubjects are the subjects and doc flags doc.cpp declares.
var cppDocSubjects = []string{
	"comment doc=false",
	"namespace/kapi/class/Parser doc=true",
	"namespace/kapi/class/Parser/Parser doc=true",
	"namespace/kapi/class/Parser/run doc=true",
	"namespace/kapi/class/Parser/stop doc=true",
	"namespace/kapi/class/Parser/comment doc=false",
	"namespace/kapi/class/Parser/source_ doc=false",
	"namespace/kapi/enum/Kind doc=true",
	"namespace/kapi/enum/Kind/Text doc=true",
	"namespace/kapi/func/largest doc=true",
	"namespace/kapi/type/Path doc=true",
	"func/kapi::Parser::run doc=true",
	"func/kapi::Parser::run/comment doc=false",
}

func TestProseP1_ruby(t *testing.T) {
	proseP1(t, "ruby")
	proseSubjects(t, "ruby", "doc.rb.txt", rubyDocSubjects)

	t.Run("YARD tags and their types are placeholders", func(t *testing.T) {
		got, err := newProvider(t, "ruby").Locate("doc.rb", fixtureBytes(t, "ruby", "doc.rb.txt"))
		require.NoError(t, err)
		parser := commentOn(t, got, "module/Kapi/class/Parser")
		assert.Equal(t, []string{"@param source [String] ", "@return [Parser] "}, placeholders(parser.Runs))
		assert.Equal(t, "Reads a recipe.\n\nthe recipe's text\na parser", model.RunsText(parser.Runs))
	})

	t.Run("a =begin block is one comment", func(t *testing.T) {
		assert.Empty(t, subjectMismatches(t, newProvider(t, "ruby"), "embdoc.rb.txt", fixtureBytes(t, "ruby", "embdoc.rb.txt"), []string{
			"comment doc=false",
			"const/VALUE doc=true",
		}))
	})

	t.Run("a comment in a Homebrew cask is named for the call it sits in", func(t *testing.T) {
		got, err := comments.Locate("ruby", "kapi-desktop.rb", fixtureBytes(t, "ruby", "homebrew__kapi-desktop.rb.txt"))
		require.NoError(t, err)
		var subjects []string
		for _, c := range got.Comments {
			subjects = append(subjects, c.Subject)
		}
		assert.Equal(t, []string{"comment", "cask/comment"}, subjects)
	})
}

// rubyDocSubjects are the subjects and doc flags doc.rb declares.
var rubyDocSubjects = []string{
	"comment doc=false",
	"module/Kapi doc=true",
	"module/Kapi/LIMIT doc=true",
	"module/Kapi/class/Parser doc=true",
	"module/Kapi/class/Parser/comment doc=false",
	"module/Kapi/class/Parser/initialize doc=true",
	"module/Kapi/class/Parser/initialize/comment doc=false",
	"module/Kapi/class/Parser/self.run doc=true",
	"func/helper doc=true",
	"cask/desc doc=true",
	"func/check doc=false",
	"func/check doc=true",
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
