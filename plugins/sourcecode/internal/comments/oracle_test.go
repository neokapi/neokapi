package comments_test

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/neokapi/neokapi/core/comment/commenttest"
)

// The oracle for TypeScript, TSX and JavaScript is the comment spans
// @babel/parser reports, written beside each fixture by
// testdata/babel-goldens.mjs. Grouping and directives are decided here from the
// bytes, with rules written for this test and nothing shared with the provider.

// golden is one fixture's Babel reading.
type golden struct {
	fixture string
	source  string
	sha256  string
	spans   [][4]int
}

// readGolden parses a fixture's .babel file.
func readGolden(fixture string) (golden, error) {
	g := golden{fixture: fixture}
	f, err := os.Open(fixture + ".babel")
	if err != nil {
		return g, fmt.Errorf("no Babel golden beside %s; run testdata/babel-goldens.mjs: %w", filepath.Base(fixture), err)
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
				return g, fmt.Errorf("%s.babel: malformed span %q", filepath.Base(fixture), line)
			}
			var span [4]int
			for i, field := range fields {
				n, err := strconv.Atoi(field)
				if err != nil {
					return g, fmt.Errorf("%s.babel: malformed span %q: %w", filepath.Base(fixture), line, err)
				}
				span[i] = n
			}
			g.spans = append(g.spans, span)
		}
	}
	if g.sha256 == "" {
		return g, fmt.Errorf("%s.babel records no sha256", filepath.Base(fixture))
	}
	return g, sc.Err()
}

func sum(src []byte) string {
	h := sha256.Sum256(src)
	return hex.EncodeToString(h[:])
}

// oracleUnits turns Babel's spans into the units the conformance suite holds a
// provider to. Line comments share a group while each sits alone on its line
// and the next follows on the very next line. Every other comment is a group
// of its own.
func oracleUnits(src []byte, spans [][4]int) []commenttest.Unit {
	units := make([]commenttest.Unit, len(spans))
	group := 0
	for i, s := range spans {
		if i > 0 && !continuesGroup(src, spans[i-1], s) {
			group++
		}
		units[i] = commenttest.Unit{
			Start: s[0], End: s[1], Open: s[2], Close: s[3],
			Group: group, Directive: oracleDirective(src[s[0]:s[1]]),
		}
	}
	return units
}

func continuesGroup(src []byte, prev, cur [4]int) bool {
	if !isLineComment(src, prev) || !isLineComment(src, cur) || !aloneOnLine(src, prev[0]) || !aloneOnLine(src, cur[0]) {
		return false
	}
	gap := src[prev[1]:cur[0]]
	return bytes.Count(gap, []byte("\n")) == 1 && len(bytes.TrimSpace(gap)) == 0
}

func isLineComment(src []byte, s [4]int) bool {
	return s[3] == 0 && bytes.HasPrefix(src[s[0]:s[1]], []byte("//"))
}

func aloneOnLine(src []byte, start int) bool {
	lineStart := bytes.LastIndexByte(src[:start], '\n') + 1
	return len(bytes.TrimLeft(src[lineStart:start], " \t")) == 0
}

// oracleDirectives are the comment forms JavaScript and TypeScript tools read,
// matched against the comment as written.
var oracleDirectives = []*regexp.Regexp{
	regexp.MustCompile(`^#!`),
	regexp.MustCompile(`^//\s*(eslint-disable|eslint-enable\b|eslint-env\b|oxlint-(disable|enable)|biome-ignore|tslint:|@ts-(expect-error|ignore|nocheck|check)\b|prettier-ignore|(istanbul|c8|v8) ignore\b|@vite-ignore\b|[#@] ?source(Mapping)?URL=|@(vitest|jest)-environment\b|@jsx(ImportSource|Frag|Runtime)?\b)`),
	regexp.MustCompile(`^///\s*<(reference|amd-module|amd-dependency)`),
	regexp.MustCompile(`^/\*\*?\s*(eslint-disable|eslint-enable\b|eslint-env\b|eslint\s+[@\w/-]+\s*:|globals?\s+[\w$]|oxlint-(disable|enable)|biome-ignore|tslint:|@ts-(expect-error|ignore|nocheck|check)\b|prettier-ignore|(istanbul|c8|v8) ignore\b|[#@]__(PURE|NO_SIDE_EFFECTS)__\s*\*/$|@vite-ignore\b|webpack[A-Z]\w*\s*:|turbopackIgnore\s*:|[#@] ?source(Mapping)?URL=)`),
	regexp.MustCompile(`(?m)^\s*(/\*\*?|\*)?\s*@(vitest-environment|jest-environment|jsx|jsxImportSource|jsxFrag|jsxRuntime)\b`),
}

func oracleDirective(text []byte) bool {
	for i, re := range oracleDirectives {
		// The docblock pragma rule reads every line of a block comment.
		if i == len(oracleDirectives)-1 && !bytes.HasPrefix(text, []byte("/*")) {
			continue
		}
		if re.Match(text) {
			return true
		}
	}
	return false
}
