// Command emdashcheck is the matcher behind scripts/check-em-dashes.sh. It
// reads a newline-separated file list on stdin and reports em dashes (U+2014)
// in whichever tier -part names.
//
// CLAUDE.md caps user-facing prose at one em dash per thousand words and asks
// for zero. Three tiers carry that prose, and each needs a different reading:
//
//	-part go        string literals in Go source. Comments are exempt, so the
//	                scan runs over the parsed syntax tree rather than the file
//	                text: an ast.BasicLit of kind STRING is a string literal and
//	                nothing else, while comments live in a node list this never
//	                visits.
//	-part catalogs  the English source text of an extracted catalog. Every
//	                string in the JSON is read except what sits under a
//	                `targets` key, which is a translation rather than source.
//	                Reading every string (rather than projecting the run
//	                sequence) is what reaches the text inside a plural or select
//	                run, which a projection could drop.
//	-part docs      Markdown and MDX prose. Fenced code blocks are skipped, and
//	                each file is allowed floor(words/1000) dashes, CLAUDE.md's
//	                ceiling.
//	-part ui        TypeScript and TSX that renders text no catalog covers. An
//	                em dash outside a comment is reported: in JS/TSX, the
//	                character can only otherwise appear inside a string, a
//	                template or JSX text, which is all shipped text. The scan
//	                tracks comment and string state so `//` inside a string
//	                starts no comment.
//
// The caller owns file selection, so what the guard checks is what the guard
// was handed.
//
// Usage:
//
//	git ls-files '*.go' | go run ./scripts/emdashcheck -part go
//	go run ./scripts/emdashcheck --self-test
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

const emDash = "—"

// wordsPerDash is CLAUDE.md's ceiling for prose: one em dash per thousand
// words. A file's allowance is floor(words/wordsPerDash), so a document under a
// thousand words of prose is allowed none.
const wordsPerDash = 1000

// allowedLiterals are the exact string values that may hold an em dash, in Go
// source and in a catalog alike. In each one the character is a character
// rather than prose:
//
//   - "—" on its own is an empty cell in a rendered table (the ai column of
//     `kapi status --review`, the lift column of the context eval, an empty
//     value cell in the desktop's collections panel).
//   - "—-( \t" is a cutset passed to strings.TrimLeft in scripts/contract-audit,
//     which strips the punctuation a skip reason may open with.
//
// Anything with text around it is prose and is rewritten instead, " — " used as
// a separator between two rendered pieces included.
var allowedLiterals = map[string]bool{
	emDash:   true,
	"—-( \t": true,
}

// holdsEmDash reports whether a value carries an em dash the guard should
// report: it contains one and is not one of the allowlisted exact values.
func holdsEmDash(v string) bool {
	return strings.Contains(v, emDash) && !allowedLiterals[v]
}

var generatedRe = regexp.MustCompile(`^// Code generated .* DO NOT EDIT\.$`)

type finding struct {
	Path  string
	Where string // line:col for Go, a JSON pointer for a catalog, a line for docs
	Value string
}

func main() {
	part := flag.String("part", "go", "which tier to scan: go, catalogs or docs")
	selfTestFlag := flag.Bool("self-test", false, "prove the matcher both ways and exit")
	flag.Parse()

	if *selfTestFlag {
		if err := selfTest(); err != nil {
			fmt.Fprintf(os.Stderr, "self-test: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("emdashcheck self-test passed")
		return
	}

	paths := readPaths(os.Stdin)

	var (
		findings []finding
		err      error
	)
	switch *part {
	case "go":
		findings, err = scanGoFiles(paths)
	case "catalogs":
		findings, err = scanCatalogFiles(paths)
	case "docs":
		findings, err = scanDocFiles(paths)
	case "ui":
		findings, err = scanUIFiles(paths)
	default:
		fmt.Fprintf(os.Stderr, "emdashcheck: unknown -part %q (want go, catalogs, docs or ui)\n", *part)
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "emdashcheck: %v\n", err)
		os.Exit(2)
	}

	if len(findings) == 0 {
		return
	}
	for _, f := range findings {
		fmt.Printf("%s:%s: %s\n", f.Path, f.Where, excerpt(f.Value))
	}
	fmt.Fprintf(os.Stderr, "\n%d em dash finding(s) in the %s tier.\n", len(findings), *part)
	os.Exit(1)
}

func readPaths(r *os.File) []string {
	var paths []string
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for sc.Scan() {
		if p := strings.TrimSpace(sc.Text()); p != "" {
			paths = append(paths, p)
		}
	}
	return paths
}

// ── go ───────────────────────────────────────────────────────────────────────

func scanGoFiles(paths []string) ([]finding, error) {
	var out []finding
	for _, p := range paths {
		if skipGoPath(p) {
			continue
		}
		src, err := os.ReadFile(p)
		if err != nil {
			return nil, err
		}
		f, err := scanGoSource(p, src)
		if err != nil {
			return nil, err
		}
		out = append(out, f...)
	}
	return out, nil
}

// skipGoPath reports whether a path is outside the scan: tests may say
// anything, testdata holds fixtures captured from elsewhere, and a generated
// file is fixed by regenerating it from a source this scan already covers.
func skipGoPath(p string) bool {
	if !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
		return true
	}
	for _, seg := range strings.Split(filepath.ToSlash(p), "/") {
		if seg == "testdata" || seg == "vendor" || seg == "node_modules" {
			return true
		}
	}
	return false
}

func scanGoSource(path string, src []byte) ([]finding, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, src, parser.ParseComments|parser.SkipObjectResolution)
	if err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	for _, group := range file.Comments {
		for _, c := range group.List {
			if generatedRe.MatchString(c.Text) {
				return nil, nil
			}
		}
	}

	var out []finding
	ast.Inspect(file, func(n ast.Node) bool {
		lit, ok := n.(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return true
		}
		value, err := strconv.Unquote(lit.Value)
		if err != nil {
			// An unparseable literal cannot hold a decoded em dash either; fall
			// back to the raw text so nothing hides behind a quirk.
			value = lit.Value
		}
		if !holdsEmDash(value) {
			return true
		}
		pos := fset.Position(lit.Pos())
		out = append(out, finding{
			Path:  path,
			Where: fmt.Sprintf("%d:%d", pos.Line, pos.Column),
			Value: value,
		})
		return true
	})
	return out, nil
}

// ── catalogs ─────────────────────────────────────────────────────────────────

func scanCatalogFiles(paths []string) ([]finding, error) {
	var out []finding
	for _, p := range paths {
		raw, err := os.ReadFile(p)
		if err != nil {
			return nil, err
		}
		var doc any
		if err := json.Unmarshal(raw, &doc); err != nil {
			return nil, fmt.Errorf("parsing %s: %w", p, err)
		}
		out = append(out, walkCatalog(p, "", doc)...)
	}
	return out, nil
}

// walkCatalog reports every string in a catalog that holds an em dash, keyed by
// its JSON pointer. Values under a `targets` key are translations, which the
// loop materializes from the source, so they are read past rather than judged:
// a target catches up on the next convergence.
func walkCatalog(path, pointer string, v any) []finding {
	switch t := v.(type) {
	case string:
		if !holdsEmDash(t) {
			return nil
		}
		return []finding{{Path: path, Where: pointer, Value: t}}
	case []any:
		var out []finding
		for i, item := range t {
			out = append(out, walkCatalog(path, fmt.Sprintf("%s/%d", pointer, i), item)...)
		}
		return out
	case map[string]any:
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		var out []finding
		for _, k := range keys {
			if k == "targets" {
				continue
			}
			out = append(out, walkCatalog(path, pointer+"/"+escapePointer(k), t[k])...)
		}
		return out
	default:
		return nil
	}
}

func escapePointer(k string) string {
	k = strings.ReplaceAll(k, "~", "~0")
	return strings.ReplaceAll(k, "/", "~1")
}

// ── docs ─────────────────────────────────────────────────────────────────────

func scanDocFiles(paths []string) ([]finding, error) {
	var out []finding
	for _, p := range paths {
		raw, err := os.ReadFile(p)
		if err != nil {
			return nil, err
		}
		out = append(out, scanDocSource(p, string(raw))...)
	}
	return out, nil
}

var fenceRe = regexp.MustCompile("^\\s*(```|~~~)")

// scanDocSource counts the em dashes in a document's prose and compares that
// against the file's allowance. Fenced code blocks hold CLI output and config,
// where a dash is data, so they are skipped for both the count and the word
// total that sets the allowance.
func scanDocSource(path, src string) []finding {
	var (
		inFence bool
		dashes  int
		words   int
		lines   []int
	)
	for i, line := range strings.Split(src, "\n") {
		if fenceRe.MatchString(line) {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}
		words += len(strings.Fields(line))
		if n := strings.Count(line, emDash); n > 0 {
			dashes += n
			lines = append(lines, i+1)
		}
	}
	allowance := words / wordsPerDash
	if dashes <= allowance {
		return nil
	}
	return []finding{{
		Path:  path,
		Where: fmt.Sprintf("%d", lines[0]),
		Value: fmt.Sprintf("%d em dash(es) in %d words of prose, allowance %d (first at line %d)",
			dashes, words, allowance, lines[0]),
	}}
}

// ── ui ───────────────────────────────────────────────────────────────────────

func scanUIFiles(paths []string) ([]finding, error) {
	var out []finding
	for _, p := range paths {
		if strings.HasSuffix(p, ".test.ts") || strings.HasSuffix(p, ".test.tsx") ||
			strings.Contains(filepath.ToSlash(p), "/__tests__/") {
			continue
		}
		raw, err := os.ReadFile(p)
		if err != nil {
			return nil, err
		}
		out = append(out, scanUISource(p, string(raw))...)
	}
	return out, nil
}

// scanUISource reports every em dash in a TS/TSX file that is not inside a
// comment. A comment is the one place the character is not shipped text: an em
// dash anywhere else in JavaScript is inside a string, a template literal or
// JSX text, since no operator, keyword or identifier can carry it.
//
// The scan tracks string state as well as comment state, because a `//` inside
// a string ("https://…") starts no comment, and a quote inside a comment starts
// no string. An empty cell (the character on its own, between JSX tags or as a
// whole string literal) is allowed, as it is in Go.
func scanUISource(path, src string) []finding {
	type state int
	const (
		code state = iota
		lineComment
		blockComment
		single
		double
		backtick
	)

	st := code
	line := 1
	var out []finding
	runes := []rune(src)
	for i := 0; i < len(runes); i++ {
		c := runes[i]
		next := rune(0)
		if i+1 < len(runes) {
			next = runes[i+1]
		}
		if c == '\n' {
			line++
			if st == lineComment {
				st = code
			}
			continue
		}
		switch st {
		case code:
			switch {
			case c == '/' && next == '/':
				st = lineComment
				i++
			case c == '/' && next == '*':
				st = blockComment
				i++
			case c == '\'':
				st = single
			case c == '"':
				st = double
			case c == '`':
				st = backtick
			case string(c) == emDash && !uiEmptyCell(runes, i):
				out = append(out, finding{
					Path:  path,
					Where: fmt.Sprintf("%d", line),
					Value: uiExcerptAround(runes, i),
				})
			}
		case lineComment:
			// consumed at the newline above
		case blockComment:
			if c == '*' && next == '/' {
				st = code
				i++
			}
		case single, double, backtick:
			if c == '\\' {
				i++
				continue
			}
			if (st == single && c == '\'') || (st == double && c == '"') || (st == backtick && c == '`') {
				st = code
				continue
			}
			if string(c) == emDash && !uiEmptyCell(runes, i) {
				out = append(out, finding{
					Path:  path,
					Where: fmt.Sprintf("%d", line),
					Value: uiExcerptAround(runes, i),
				})
			}
		}
	}
	return out
}

// uiEmptyCell reports whether the dash at i is the whole rendered value: `>—<`
// between JSX tags, or "—" / '—' / `—` as an entire literal. Both are the empty
// cell the Go allowlist admits.
func uiEmptyCell(runes []rune, i int) bool {
	if i == 0 || i+1 >= len(runes) {
		return false
	}
	before, after := runes[i-1], runes[i+1]
	if before == '>' && after == '<' {
		return true
	}
	return (before == '"' && after == '"') ||
		(before == '\'' && after == '\'') ||
		(before == '`' && after == '`')
}

func uiExcerptAround(runes []rune, i int) string {
	lo := max(i-40, 0)
	hi := min(i+40, len(runes))
	return strings.TrimSpace(string(runes[lo:hi]))
}

// ── shared ───────────────────────────────────────────────────────────────────

func excerpt(v string) string {
	v = strings.ReplaceAll(v, "\n", "\\n")
	if len(v) > 160 {
		v = v[:157] + "..."
	}
	return v
}

// selfTest proves each matcher both ways. A matcher that quietly stops matching
// is worse than no check at all.
//
// Every fixture below spells the em dash as `@` and substitutes it in, so this
// file holds no literal that the go part would flag when it scans itself.
func selfTest() error {
	if err := selfTestGo(); err != nil {
		return err
	}
	if err := selfTestCatalogs(); err != nil {
		return err
	}
	if err := selfTestDocs(); err != nil {
		return err
	}
	return selfTestUI()
}

func fixture(s string) string { return strings.ReplaceAll(s, "@", emDash) }

func selfTestGo() error {
	dirty := fixture(`package p

// A comment with an em dash @ this is exempt.
const (
	Prose = "staged, not committed @ run kapi commit"
	Raw   = ` + "`" + `a raw literal @ also prose` + "`" + `
	Cell  = "@"
	Plain = "no dash here"
)
`)
	got, err := scanGoSource("dirty.go", []byte(dirty))
	if err != nil {
		return err
	}
	if len(got) != 2 {
		return fmt.Errorf("go: expected 2 findings in the planted file, got %d: %v", len(got), got)
	}
	if !strings.Contains(got[0].Value, "kapi commit") || !strings.Contains(got[1].Value, "raw literal") {
		return fmt.Errorf("go: unexpected findings: %v", got)
	}

	generated := fixture(`package p

// Code generated by stringer. DO NOT EDIT.

const Prose = "a generated string @ exempt"
`)
	got, err = scanGoSource("zz_generated.go", []byte(generated))
	if err != nil {
		return err
	}
	if len(got) != 0 {
		return fmt.Errorf("go: a generated file was flagged: %v", got)
	}

	for _, p := range []string{"x_test.go", "core/formats/testdata/x.go", "a/vendor/b/c.go", "notes.md"} {
		if !skipGoPath(p) {
			return fmt.Errorf("go: %s should be skipped", p)
		}
	}
	if skipGoPath("host/status.go") {
		return fmt.Errorf("go: host/status.go should be scanned")
	}
	return nil
}

func selfTestCatalogs() error {
	catalog := fixture(`{
	  "documents": [{
	    "blocks": [
	      {"id": "a", "source": [{"text": "Staged, not committed @ run kapi commit"}]},
	      {"id": "b", "source": [{"plural": {"forms": {"other": [{"text": "nested @ prose"}]}}}]},
	      {"id": "c", "source": [{"text": "clean"}],
	       "targets": {"nb": [{"text": "en oversettelse @ med tankestrek"}]}},
	      {"id": "d", "source": [{"text": "@"}]}
	    ]
	  }]
	}`)
	var doc any
	if err := json.Unmarshal([]byte(catalog), &doc); err != nil {
		return err
	}
	got := walkCatalog("app.kbf.json", "", doc)
	if len(got) != 2 {
		return fmt.Errorf("catalogs: expected 2 findings (source and nested plural), got %d: %v", len(got), got)
	}
	for _, f := range got {
		if strings.Contains(f.Value, "oversettelse") {
			return fmt.Errorf("catalogs: a target-locale string was flagged: %v", f)
		}
	}
	if !strings.Contains(got[1].Value, "nested") {
		return fmt.Errorf("catalogs: the plural form's text was not reached: %v", got)
	}
	for _, f := range got {
		if f.Value == emDash {
			return fmt.Errorf("catalogs: the bare glyph (an empty cell) was flagged: %v", f)
		}
	}
	return nil
}

func selfTestDocs() error {
	short := fixture("# Title\n\nOne dash @ in a short document.\n")
	if got := scanDocSource("short.md", short); len(got) != 1 {
		return fmt.Errorf("docs: a short document with one dash should fail, got %v", got)
	}

	fenced := fixture("# Title\n\n```\nPASS @ 3 finding(s)\n```\n\nNo prose dash here.\n")
	if got := scanDocSource("fenced.md", fenced); len(got) != 0 {
		return fmt.Errorf("docs: a dash inside a fence was flagged: %v", got)
	}

	long := fixture("# Title\n\n" + strings.Repeat("word ", 1200) + "\n\nOne dash @ inside a long document.\n")
	if got := scanDocSource("long.md", long); len(got) != 0 {
		return fmt.Errorf("docs: a 1200-word document is allowed one dash, got %v", got)
	}

	tooMany := fixture("# Title\n\n" + strings.Repeat("word ", 1200) + "\n\nTwo @ dashes @ here.\n")
	if got := scanDocSource("many.md", tooMany); len(got) != 1 {
		return fmt.Errorf("docs: two dashes in a 1200-word document should fail, got %v", got)
	}
	return nil
}

func selfTestUI() error {
	dirty := fixture(`// A line comment with an em dash @ exempt.
/* A block comment @ exempt too. */
const label = "Nothing written yet @ press Run";
const url = "https://example.com/a@b"; // the // in a string starts no comment
export function Cell() {
  return <span>{value || <em>@</em>}</span>;
}
const jsx = <p>Run the flow @ the engine executes your code.</p>;
`)
	got := scanUISource("Panel.tsx", dirty)
	if len(got) != 3 {
		return fmt.Errorf("ui: expected 3 findings (string, url, jsx text), got %d: %v", len(got), got)
	}
	for _, f := range got {
		if strings.Contains(f.Value, "exempt") {
			return fmt.Errorf("ui: a comment was flagged: %v", f)
		}
	}

	clean := fixture(`const cell = "@";
const nothing = <span>@</span>;
// @ and /* @ */
`)
	if got := scanUISource("Clean.tsx", clean); len(got) != 0 {
		return fmt.Errorf("ui: an empty cell or a comment was flagged: %v", got)
	}
	return nil
}
