// Command okapi-test-scan extracts inline-string fixture material from
// Okapi filter test sources so the parity harness can drive every
// extractable Java @Test through the bridge with a known input.
//
// The scanner is intentionally lossy: it walks Java with regular
// expressions, not an AST, and only recovers tests whose input shape
// matches one of the dominant patterns in upstream Okapi:
//
//	@Test
//	public void testFoo() {
//	    String snippet = "...";        // single literal
//	    String snippet = "..."          // multi-line string concat
//	            + "...";
//	    ...
//	}
//
// Tests that build inputs programmatically, load resource files, or
// whose snippet variable is named something other than `snippet` are
// skipped — the survey upstream put that population at ~10–15% per
// filter, which is acceptable for a coverage pass that reports its
// own gaps.
//
// Output is a build-tagged Go file (see -out) carrying one
// []formats.FormatInput per scanned test class. Hand-curated fixtures
// in spec.go remain authoritative; the auto-generated set is
// referenced explicitly per FormatSpec.
//
// Usage:
//
//	go run ./scripts/okapi-test-scan \
//	    -src /path/to/okapi-java/okapi/filters/html/src/test/java \
//	    -class HtmlSnippetsTest \
//	    -package html \
//	    -out cli/parity/formats/fixtures_html_generated.go
package main

import (
	"bytes"
	"flag"
	"fmt"
	"go/format"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

func main() {
	src := flag.String("src", "", "Java test source root (directory walked recursively)")
	classFilter := flag.String("class", "", "Comma-separated short class names to include (e.g. HtmlSnippetsTest). Empty = all *Test.java.")
	pkg := flag.String("package", "formats", "Go package to write into")
	out := flag.String("out", "", "Output Go file path (required)")
	flag.Parse()

	if *src == "" || *out == "" {
		die("must set -src and -out")
	}
	classes := splitCSV(*classFilter)
	classMatch := func(name string) bool {
		if len(classes) == 0 {
			return true
		}
		for _, c := range classes {
			if c == name {
				return true
			}
		}
		return false
	}

	var scanned []scannedClass
	err := filepath.WalkDir(*src, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() || !strings.HasSuffix(path, "Test.java") {
			return nil
		}
		short := strings.TrimSuffix(filepath.Base(path), ".java")
		if !classMatch(short) {
			return nil
		}
		c, err := scanFile(path, short)
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		if len(c.Tests) > 0 {
			scanned = append(scanned, c)
		}
		return nil
	})
	if err != nil {
		die("scan: %v", err)
	}

	sort.SliceStable(scanned, func(i, j int) bool { return scanned[i].ClassName < scanned[j].ClassName })

	body, err := emit(*pkg, scanned)
	if err != nil {
		die("emit: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(*out), 0o755); err != nil {
		die("mkdir: %v", err)
	}
	if err := os.WriteFile(*out, body, 0o644); err != nil {
		die("write: %v", err)
	}

	totalTests := 0
	totalScanned := 0
	for _, c := range scanned {
		totalTests += c.SeenAtTests
		totalScanned += len(c.Tests)
		fmt.Fprintf(os.Stderr, "  %s: %d/%d tests extracted (%d had unsupported input shapes)\n",
			c.ClassName, len(c.Tests), c.SeenAtTests, c.SkippedReasons["unsupported"])
	}
	fmt.Fprintf(os.Stderr, "okapi-test-scan: %d/%d tests across %d classes → %s\n",
		totalScanned, totalTests, len(scanned), *out)
}

// scannedClass aggregates per-test results for one Java test class.
type scannedClass struct {
	ClassName      string         // short class name (e.g. HtmlSnippetsTest)
	SourceFile     string         // path to the .java file (relative to scan root)
	Tests          []scannedTest  // successfully extracted tests
	SeenAtTests    int            // total @Test methods seen
	SkippedReasons map[string]int // reason → count
}

// scannedTest is one extractable @Test method.
type scannedTest struct {
	Method  string // testFoo
	Snippet string // inline content (already unescaped from Java string literal form)
}

var (
	// `@Test` with optional parameter list, followed by the next
	// `public void <name>(...)` declaration. We intentionally accept
	// `@Test(expected = ...)` because the input still matters even if
	// the assertion is "throws".
	testHeaderRE = regexp.MustCompile(`(?m)^\s*@Test(?:\([^)]*\))?\s*$`)
	methodDeclRE = regexp.MustCompile(`^\s*public\s+void\s+(\w+)\s*\(`)
	// `String snippet = "..."`, optionally with `+ "..."` continuation
	// lines until a terminating `;`. We extract the concatenation as
	// one logical literal.
	snippetStartRE = regexp.MustCompile(`^\s*(?:final\s+)?String\s+snippet\s*=\s*(.+)$`)
)

func scanFile(path, className string) (scannedClass, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return scannedClass{}, err
	}
	c := scannedClass{
		ClassName:      className,
		SourceFile:     path,
		SkippedReasons: map[string]int{},
	}
	lines := strings.Split(string(data), "\n")

	for i := 0; i < len(lines); i++ {
		if !testHeaderRE.MatchString(lines[i]) {
			continue
		}
		// Find the next public-void declaration (allowing other
		// annotations or whitespace in between).
		j := i + 1
		var method string
		for j < len(lines) && j < i+10 {
			if m := methodDeclRE.FindStringSubmatch(lines[j]); m != nil {
				method = m[1]
				break
			}
			j++
		}
		if method == "" {
			c.SkippedReasons["no-method"]++
			continue
		}
		c.SeenAtTests++

		// Find the end of the method body (matching brace) so we can
		// scan only that region for the snippet declaration. Cheap
		// brace counter starting from the `{` after the signature.
		bodyStart := j
		for bodyStart < len(lines) && !strings.Contains(lines[bodyStart], "{") {
			bodyStart++
		}
		bodyEnd := scanForMethodEnd(lines, bodyStart)

		snippet, ok := extractSnippet(lines[bodyStart : bodyEnd+1])
		if !ok {
			c.SkippedReasons["unsupported"]++
			continue
		}
		c.Tests = append(c.Tests, scannedTest{Method: method, Snippet: snippet})
		i = bodyEnd
	}
	return c, nil
}

// scanForMethodEnd finds the closing `}` of the method body whose
// opening brace is on or after `start`. Returns the index of the line
// containing the closing brace.
func scanForMethodEnd(lines []string, start int) int {
	depth := 0
	for k := start; k < len(lines); k++ {
		for _, ch := range lines[k] {
			switch ch {
			case '{':
				depth++
			case '}':
				depth--
				if depth == 0 {
					return k
				}
			}
		}
	}
	return len(lines) - 1
}

// extractSnippet pulls the body of `String snippet = "..."` from a
// method body, supporting multi-line concatenation. Returns the
// unescaped snippet content and a boolean indicating success.
func extractSnippet(body []string) (string, bool) {
	for i, line := range body {
		m := snippetStartRE.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		// Stitch continuation lines until we find the terminating `;`.
		raw := m[1]
		for !strings.HasSuffix(strings.TrimSpace(raw), ";") && i+1 < len(body) {
			i++
			raw += " " + strings.TrimSpace(body[i])
		}
		raw = strings.TrimSuffix(strings.TrimSpace(raw), ";")
		return parseJavaConcat(raw)
	}
	return "", false
}

// parseJavaConcat joins a Java string-concatenation expression like
//
//	"a" + "b" + "c"
//
// into the single decoded literal `abc`. Returns false if any term
// isn't a simple double-quoted Java string (e.g. method calls, vars).
func parseJavaConcat(expr string) (string, bool) {
	var out strings.Builder
	i := 0
	for i < len(expr) {
		// Skip whitespace.
		for i < len(expr) && (expr[i] == ' ' || expr[i] == '\t' || expr[i] == '+' || expr[i] == '\n') {
			i++
		}
		if i >= len(expr) {
			break
		}
		if expr[i] != '"' {
			return "", false // non-literal term
		}
		end, lit, ok := readJavaString(expr[i:])
		if !ok {
			return "", false
		}
		out.WriteString(lit)
		i += end
	}
	return out.String(), true
}

// readJavaString reads a Java double-quoted string literal starting at
// s[0]=='"'. Returns the consumed length, the decoded contents, and
// success.
func readJavaString(s string) (int, string, bool) {
	if len(s) == 0 || s[0] != '"' {
		return 0, "", false
	}
	var buf strings.Builder
	i := 1
	for i < len(s) {
		c := s[i]
		if c == '"' {
			return i + 1, buf.String(), true
		}
		if c == '\\' && i+1 < len(s) {
			switch s[i+1] {
			case 'n':
				buf.WriteByte('\n')
			case 'r':
				buf.WriteByte('\r')
			case 't':
				buf.WriteByte('\t')
			case '"':
				buf.WriteByte('"')
			case '\\':
				buf.WriteByte('\\')
			case '\'':
				buf.WriteByte('\'')
			case 'u':
				if i+5 >= len(s) {
					return 0, "", false
				}
				code, err := strconv.ParseInt(s[i+2:i+6], 16, 32)
				if err != nil {
					return 0, "", false
				}
				buf.WriteRune(rune(code))
				i += 4
			default:
				buf.WriteByte(s[i+1])
			}
			i += 2
			continue
		}
		buf.WriteByte(c)
		i++
	}
	return 0, "", false // unterminated
}

// emit produces a build-tagged Go file declaring one
// []formats.FormatInput per class.
func emit(pkg string, classes []scannedClass) ([]byte, error) {
	var b bytes.Buffer
	fmt.Fprintln(&b, "//go:build parity")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "// Code generated by scripts/okapi-test-scan. DO NOT EDIT.")
	fmt.Fprintln(&b)
	fmt.Fprintf(&b, "package %s\n\n", pkg)

	for _, c := range classes {
		varName := "Generated" + c.ClassName + "Inputs"
		fmt.Fprintf(&b, "// %s holds %d auto-extracted @Test fixtures from %s.\n",
			varName, len(c.Tests), c.ClassName)
		fmt.Fprintf(&b, "var %s = []FormatInput{\n", varName)
		for _, t := range c.Tests {
			fmt.Fprintf(&b, "\t{Name: %q, Content: ttext(%s), OkapiTest: %q, Informational: true},\n",
				"gen-"+t.Method, goRawOrQuoted(t.Snippet), c.ClassName+"#"+t.Method)
		}
		fmt.Fprintln(&b, "}")
		fmt.Fprintln(&b)
	}
	return format.Source(b.Bytes())
}

// goRawOrQuoted picks the most readable Go literal form for the
// snippet content. Backtick-quoted is preferred when the input has no
// backticks of its own; otherwise we fall back to %q which handles
// arbitrary bytes including embedded newlines and quotes.
func goRawOrQuoted(s string) string {
	if !strings.ContainsRune(s, '`') {
		return "`" + s + "`"
	}
	return strconv.Quote(s)
}

func splitCSV(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func die(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "okapi-test-scan: "+format+"\n", args...)
	os.Exit(1)
}
