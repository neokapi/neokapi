package comments_test

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/tdewolff/parse/v2"
	"github.com/tdewolff/parse/v2/css"
	"mvdan.cc/sh/v3/syntax"

	"github.com/neokapi/neokapi/core/comment/commenttest"
)

// Each language is held to comment spans read without the grammar the plugin
// reads it with. TypeScript, TSX and JavaScript are held to @babel/parser,
// Python to the tokenize module of its standard library, Rust to rustc's own
// lexer, Java to javac's tokenizer, C# to Roslyn, C and C++ to libclang, and Ruby
// to Ripper; scripts in testdata run those and write a golden beside each
// fixture. Bash is held to the parser
// of mvdan.cc/sh and CSS to the lexer of github.com/tdewolff/parse, both run
// by the test itself. Grouping and directives are decided here from the bytes,
// with rules written for this test and nothing shared with the provider.

// oracle reads one language's comment spans without the provider. A span is its
// start and end offsets and the lengths of its opening and closing markers.
type oracle struct {
	// name names the reader in a failure.
	name string
	// golden is the suffix of the file beside each fixture that records the
	// reader's spans, and script the script in testdata that writes it. scan,
	// set instead, reads the spans in the test.
	golden, script string
	scan           func(src []byte) ([][4]int, error)
	// line is the marker a comment that runs to the end of its line opens
	// with, or empty for a language without one.
	line string
	// directive reports whether the comment at span is one a tool reads.
	directive func(src []byte, span [4]int) bool
}

var babel = oracle{name: "Babel", golden: ".babel", script: "testdata/babel-goldens.mjs", line: "//", directive: babelDirective}

var oracles = map[string]oracle{
	"typescript": babel,
	"tsx":        babel,
	"javascript": babel,
	"python":     {name: "tokenize", golden: ".tokenize", script: "testdata/python-goldens.py", line: "#", directive: pythonDirective},
	"bash":       {name: "mvdan.cc/sh", scan: shellSpans, line: "#", directive: shellDirective},
	"css":        {name: "tdewolff/parse", scan: cssSpans, directive: cssDirective},
	"rust":       {name: "rustc's lexer", golden: ".rustc", script: "testdata/rust-goldens.py", line: "//", directive: rustDirective},
	"java":       {name: "javac's tokenizer", golden: ".javac", script: "testdata/java-goldens.py", line: "//", directive: javaDirective},
	"csharp":     {name: "Roslyn", golden: ".roslyn", script: "testdata/csharp-goldens.py", line: "//", directive: csharpDirective},
	"c":          {name: "libclang", golden: ".clang", script: "testdata/clang-goldens.py", line: "//", directive: cDirective},
	"cpp":        {name: "libclang", golden: ".clang", script: "testdata/clang-goldens.py", line: "//", directive: cDirective},
	"ruby":       {name: "Ripper", golden: ".ripper", script: "testdata/ruby-goldens.rb", line: "#", directive: rubyDirective},
}

// golden is one fixture's spans as a reader outside the test recorded them.
type golden struct {
	fixture string
	source  string
	sha256  string
	spans   [][4]int
}

// readGolden parses the golden beside a fixture.
func readGolden(fixture string, o oracle) (golden, error) {
	g := golden{fixture: fixture}
	name := filepath.Base(fixture) + o.golden
	f, err := os.Open(fixture + o.golden)
	if err != nil {
		return g, fmt.Errorf("no %s golden beside %s; run %s: %w", o.name, filepath.Base(fixture), o.script, err)
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		switch {
		case strings.HasPrefix(line, "# source "):
			g.source = strings.TrimPrefix(line, "# source ")
		case strings.HasPrefix(line, "# sha256 "):
			g.sha256 = strings.TrimPrefix(line, "# sha256 ")
		case strings.HasPrefix(line, "#"), line == "":
		default:
			fields := strings.Fields(line)
			if len(fields) != 4 {
				return g, fmt.Errorf("%s: malformed span %q", name, line)
			}
			var span [4]int
			for i, field := range fields {
				n, err := strconv.Atoi(field)
				if err != nil {
					return g, fmt.Errorf("%s: malformed span %q: %w", name, line, err)
				}
				span[i] = n
			}
			g.spans = append(g.spans, span)
		}
	}
	if g.sha256 == "" {
		return g, fmt.Errorf("%s records no sha256", name)
	}
	return g, sc.Err()
}

func sum(src []byte) string {
	h := sha256.Sum256(src)
	return hex.EncodeToString(h[:])
}

// units turns an oracle's spans into the units the conformance suite holds a
// provider to. Line comments share a group while each sits alone on its line
// and the next follows on the very next line. Every other comment is a group
// of its own.
func (o oracle) units(src []byte, spans [][4]int) []commenttest.Unit {
	units := make([]commenttest.Unit, len(spans))
	group := 0
	for i, s := range spans {
		if i > 0 && !o.continuesGroup(src, spans[i-1], s) {
			group++
		}
		units[i] = commenttest.Unit{
			Start: s[0], End: s[1], Open: s[2], Close: s[3],
			Group: group, Directive: o.directive(src, s),
		}
	}
	return units
}

func (o oracle) continuesGroup(src []byte, prev, cur [4]int) bool {
	if !o.isLineComment(src, prev) || !o.isLineComment(src, cur) || !aloneOnLine(src, prev[0]) || !aloneOnLine(src, cur[0]) {
		return false
	}
	gap := src[prev[1]:cur[0]]
	return bytes.Count(gap, []byte("\n")) == 1 && len(bytes.TrimSpace(gap)) == 0
}

func (o oracle) isLineComment(src []byte, s [4]int) bool {
	return o.line != "" && s[3] == 0 && bytes.HasPrefix(src[s[0]:s[1]], []byte(o.line))
}

func aloneOnLine(src []byte, start int) bool {
	lineStart := bytes.LastIndexByte(src[:start], '\n') + 1
	return len(bytes.TrimLeft(src[lineStart:start], " \t")) == 0
}

// babelDirectives are the comment forms JavaScript and TypeScript tools read,
// matched against the comment as written.
var babelDirectives = []*regexp.Regexp{
	regexp.MustCompile(`^#!`),
	regexp.MustCompile(`^//\s*(eslint-disable|eslint-enable\b|eslint-env\b|oxlint-(disable|enable)|biome-ignore|tslint:|@ts-(expect-error|ignore|nocheck|check)\b|prettier-ignore|(istanbul|c8|v8) ignore\b|@vite-ignore\b|[#@] ?source(Mapping)?URL=|@(vitest|jest)-environment\b|@jsx(ImportSource|Frag|Runtime)?\b)`),
	regexp.MustCompile(`^///\s*<(reference|amd-module|amd-dependency)`),
	regexp.MustCompile(`^/\*\*?\s*(eslint-disable|eslint-enable\b|eslint-env\b|eslint\s+[@\w/-]+\s*:|globals?\s+[\w$]|oxlint-(disable|enable)|biome-ignore|tslint:|@ts-(expect-error|ignore|nocheck|check)\b|prettier-ignore|(istanbul|c8|v8) ignore\b|[#@]__(PURE|NO_SIDE_EFFECTS)__\s*\*/$|@vite-ignore\b|webpack[A-Z]\w*\s*:|turbopackIgnore\s*:|[#@] ?source(Mapping)?URL=)`),
	regexp.MustCompile(`(?m)^\s*(/\*\*?|\*)?\s*@(vitest-environment|jest-environment|jsx|jsxImportSource|jsxFrag|jsxRuntime)\b`),
}

func babelDirective(src []byte, s [4]int) bool {
	text := src[s[0]:s[1]]
	for i, re := range babelDirectives {
		// The docblock pragma rule reads every line of a block comment.
		if i == len(babelDirectives)-1 && !bytes.HasPrefix(text, []byte("/*")) {
			continue
		}
		if re.Match(text) {
			return true
		}
	}
	return false
}

// pythonDirectives are the comments Python tools read: a marker a tool names at
// the start of the comment, flake8's noqa and Bandit's nosec anywhere in it,
// and coverage.py's default exclusion.
var pythonDirectives = []*regexp.Regexp{
	regexp.MustCompile(`^#\s*(type|pylint|mypy|pyright|ruff|isort):`),
	regexp.MustCompile(`^#\s*fmt:\s*(on|off|skip)\b`),
	regexp.MustCompile(`(?i)#\s*noqa\b`),
	regexp.MustCompile(`#\s*nosec\b`),
	regexp.MustCompile(`#\s*(pragma|PRAGMA)[:\s]?\s*(no|NO)\s*(cover|COVER|branch|BRANCH)\b`),
}

// pep263 is the pattern PEP 263 gives for a source encoding declaration, which
// Python reads on the first two lines.
var pep263 = regexp.MustCompile(`^[ \t\f]*#.*?coding[:=][ \t]*[-_.a-zA-Z0-9]+`)

func pythonDirective(src []byte, s [4]int) bool {
	text := src[s[0]:s[1]]
	if s[0] == 0 && bytes.HasPrefix(text, []byte("#!")) {
		return true
	}
	lineStart := bytes.LastIndexByte(src[:s[0]], '\n') + 1
	if bytes.Count(src[:s[0]], []byte("\n")) < 2 && pep263.Match(src[lineStart:s[1]]) {
		return true
	}
	return slices.ContainsFunc(pythonDirectives, func(re *regexp.Regexp) bool { return re.Match(text) })
}

// shellcheckDirective is a ShellCheck directive, a comment naming one of the
// keys ShellCheck reads.
var shellcheckDirective = regexp.MustCompile(`^#\s*shellcheck\s+(disable|enable|source|source-path|shell|external-sources)=`)

func shellDirective(src []byte, s [4]int) bool {
	text := src[s[0]:s[1]]
	return s[0] == 0 && bytes.HasPrefix(text, []byte("#!")) || shellcheckDirective.Match(text)
}

// cssDirective is a comment stylelint or Prettier reads, or a source map
// reference.
var cssDirectiveForm = regexp.MustCompile(`^/\*\s*(stylelint-(disable|enable)|prettier-ignore\b|[#@]\s?source(Mapping)?URL=)`)

func cssDirective(src []byte, s [4]int) bool {
	return cssDirectiveForm.Match(src[s[0]:s[1]])
}

// rustDirectiveForm is a comment a tool reads in Rust source: an SPDX licence
// tag, which licence scanners read, or a folding marker rust-analyzer reads.
var rustDirectiveForm = regexp.MustCompile(`^(//|/\*)\s*SPDX-License-Identifier:|^//\s*(region|endregion)\b`)

func rustDirective(src []byte, s [4]int) bool {
	return rustDirectiveForm.Match(src[s[0]:s[1]])
}

// javaDirectiveForms are the comments a Java tool reads: IntelliJ's inspection
// suppressions, Checkstyle's switches, the formatter switches Eclipse, IntelliJ
// and Spotless read, Checkstyle's fall-through relief, Eclipse's
// externalized-string markers, and Sonar's and PMD's suppressions anywhere in a
// comment.
var javaDirectiveForms = []*regexp.Regexp{
	regexp.MustCompile(`^//\s*noinspection\b`),
	regexp.MustCompile(`^(//|/\*)\s*(CHECKSTYLE[:.]|@formatter:(on|off)\b|spotless:(on|off)\b|falls?[ -]?thr(u|ough)\b)`),
	regexp.MustCompile(`^//\s*\$NON-NLS-\d+\$`),
	regexp.MustCompile(`\b(NOSONAR|NOPMD)\b`),
}

func javaDirective(src []byte, s [4]int) bool {
	return slices.ContainsFunc(javaDirectiveForms, func(re *regexp.Regexp) bool { return re.Match(src[s[0]:s[1]]) })
}

// csharpDirectiveForms are the comments a C# tool reads: ReSharper's
// inspection switches, the formatter switches Rider and CSharpier read, and
// Sonar's suppression anywhere in a comment.
var csharpDirectiveForms = []*regexp.Regexp{
	regexp.MustCompile(`^//\s*ReSharper\s+(disable|restore|enable)\b`),
	regexp.MustCompile(`^(//|/\*)\s*(@formatter:(on|off)|csharpier-ignore)\b`),
	regexp.MustCompile(`\bNOSONAR\b`),
}

func csharpDirective(src []byte, s [4]int) bool {
	return slices.ContainsFunc(csharpDirectiveForms, func(re *regexp.Regexp) bool { return re.Match(src[s[0]:s[1]]) })
}

// cDirectiveForms are the comments a C or C++ tool reads: an SPDX licence tag,
// clang-format's switches, cppcheck's, include-what-you-use's, lcov's and
// gcovr's pragmas, clang-tidy's and Sonar's suppressions anywhere in a comment,
// and an editor's mode line.
var cDirectiveForms = []*regexp.Regexp{
	regexp.MustCompile(`^(//|/\*)\s*(SPDX-License-Identifier:|clang-format\s+(on|off)\b|cppcheck-suppress\b|IWYU\s+pragma:|(LCOV|GCOVR)_EXCL_)`),
	regexp.MustCompile(`\b(NOLINT|NOLINTNEXTLINE|NOLINTBEGIN|NOLINTEND|NOSONAR)\b`),
	regexp.MustCompile(`^(//|/\*)\s*-\*-.*-\*-\s*(\*/)?$`),
}

// cClosingLine is a line a label comment may follow: a conditional directive's
// close or turn, or a brace that ends a namespace or linkage block.
var (
	cClosingDirective = regexp.MustCompile(`^\s*#\s*(endif|else)\b`)
	cClosingBrace     = regexp.MustCompile(`^\s*\};?\s*$`)
	cBraceLabel       = regexp.MustCompile(`^(//|/\*)\s*(end\s+(of\s+)?)?((anonymous|unnamed)\s+)?(namespace\b|extern\s+"C")`)
)

func cDirective(src []byte, s [4]int) bool {
	text := src[s[0]:s[1]]
	if slices.ContainsFunc(cDirectiveForms, func(re *regexp.Regexp) bool { return re.Match(text) }) {
		return true
	}
	lineStart := bytes.LastIndexByte(src[:s[0]], '\n') + 1
	before := src[lineStart:s[0]]
	return cClosingDirective.Match(before) || cClosingBrace.Match(before) && cBraceLabel.Match(text)
}

// rubyDirectiveForms are the comments Ruby and its tools read: the magic
// comments the interpreter reads, Sorbet's sigil, RuboCop's and Standard's
// switches, and RDoc's and SimpleCov's directives.
var rubyDirectiveForms = []*regexp.Regexp{
	regexp.MustCompile(`^#\s*(-\*-.*)?\b(frozen_string_literal|encoding|coding|warn_indent|shareable_constant_value)\s*:`),
	regexp.MustCompile(`^#\s*typed:\s*(ignore|false|true|strict|strong)\b`),
	regexp.MustCompile(`^#\s*(rubocop:(disable|enable|todo)|standard:(disable|enable))\b`),
	regexp.MustCompile(`^#\s*:(nodoc|stopdoc|startdoc|doc|notnew|yields|call-seq|nocov):`),
}

func rubyDirective(src []byte, s [4]int) bool {
	text := src[s[0]:s[1]]
	if s[0] == 0 && bytes.HasPrefix(text, []byte("#!")) {
		return true
	}
	return slices.ContainsFunc(rubyDirectiveForms, func(re *regexp.Regexp) bool { return re.Match(text) })
}

// shellSpans reads the comments of a Bash script with mvdan.cc/sh.
func shellSpans(src []byte) ([][4]int, error) {
	f, err := syntax.NewParser(syntax.KeepComments(true), syntax.Variant(syntax.LangBash)).Parse(bytes.NewReader(src), "")
	if err != nil {
		return nil, err
	}
	var spans [][4]int
	syntax.Walk(f, func(n syntax.Node) bool {
		c, ok := n.(*syntax.Comment)
		if !ok {
			return true
		}
		start, end := int(c.Pos().Offset()), int(c.End().Offset())
		// The parser counts the line break after a comment that ends in a
		// backslash into the comment. The shell ends every comment before its
		// line break.
		if end > start && src[end-1] == '\n' {
			end--
		}
		spans = append(spans, [4]int{start, end, 1, 0})
		return true
	})
	slices.SortFunc(spans, func(a, b [4]int) int { return a[0] - b[0] })
	return spans, nil
}

// cssSpans reads the comments of a stylesheet with the tdewolff CSS lexer,
// whose tokens cover every byte of the input in order.
func cssSpans(src []byte) ([][4]int, error) {
	l := css.NewLexer(parse.NewInputBytes(src))
	var spans [][4]int
	offset := 0
	for {
		tt, data := l.Next()
		if tt == css.ErrorToken {
			break
		}
		if tt == css.CommentToken {
			spans = append(spans, [4]int{offset, offset + len(data), 2, 2})
		}
		offset += len(data)
	}
	if err := l.Err(); err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	if offset != len(src) {
		return nil, fmt.Errorf("the CSS lexer read %d of %d bytes", offset, len(src))
	}
	return spans, nil
}
