package ts

import (
	"fmt"

	"github.com/neokapi/neokapi/core/model"
)

// applyCodeFinder walks every TextRun in `runs` and splits any text
// that matches okapi's TS-default `InlineCodeFinder` rules into
// inline-code Ph runs interleaved with the surrounding TextRuns. This
// mirrors okapi's `Parameters.reset()` defaults in TsFilter:
//
//	codeFinder.addRule("%(([-0+#]?)[-0+#]?)((\d\$)?)(([\d\*]*)(\.[\d\*]*)?)[dioxXucsfeEgGpn]");
//	codeFinder.addRule("(\\r\\n)|\\a|\\b|\\f|\\n|\\r|\\t|\\v");
//	codeFinder.addRule("\{\d[^\\]*?\}");
//
// Splitting these out as Ph runs keeps the pseudo-translation step
// (which only mutates TextRun content) from corrupting printf
// placeholders (`%s` -> `%ś`), C-style escapes (`\n` -> `\ń`), or
// MessageFormat-style positional markers (`{0}` -> `{0}` unchanged).
// The Data field carries the literal source bytes so the writer
// re-emits them verbatim via `runsToXML`.
func applyCodeFinder(runs []model.Run) []model.Run {
	if len(runs) == 0 {
		return runs
	}
	out := make([]model.Run, 0, len(runs))
	phID := 0
	for _, r := range runs {
		if r.Text == nil {
			out = append(out, r)
			continue
		}
		segs := splitCodeFinder(r.Text.Text, &phID)
		out = append(out, segs...)
	}
	return out
}

// splitCodeFinder walks `text` left-to-right, emitting TextRuns for
// the inter-match spans and Ph runs for each codeFinder match. The
// `phID` counter is shared across the whole runs slice so generated
// IDs are unique per block.
func splitCodeFinder(text string, phID *int) []model.Run {
	if text == "" {
		return nil
	}
	var out []model.Run
	i := 0
	for i < len(text) {
		// Try each rule in turn at position i. Rules are tried in
		// declaration order; the first one that matches wins.
		matchEnd := -1
		matchKind := ""
		if end := matchPrintfHere(text, i); end > i {
			matchEnd = end
			matchKind = "printf"
		} else if end := matchBackslashEscapeHere(text, i); end > i {
			matchEnd = end
			matchKind = "esc"
		} else if end := matchBracePlaceholderHere(text, i); end > i {
			matchEnd = end
			matchKind = "brace"
		}

		if matchEnd < 0 {
			// No rule matches at i — this byte is plain text. Find the
			// next rule-match (or EOF) and emit the in-between span as
			// a TextRun.
			next := findNextCodeFinderMatch(text, i+1)
			if next < 0 {
				next = len(text)
			}
			out = appendTSTextRun(out, text[i:next])
			i = next
			continue
		}

		*phID++
		out = append(out, model.Run{Ph: &model.PlaceholderRun{
			ID:   fmt.Sprintf("c%d", *phID),
			Type: matchKind,
			Data: text[i:matchEnd],
		}})
		i = matchEnd
	}
	return out
}

// findNextCodeFinderMatch returns the byte offset of the next
// codeFinder match at or after `start`, or -1 if none.
func findNextCodeFinderMatch(text string, start int) int {
	for i := start; i < len(text); i++ {
		if matchPrintfHere(text, i) > i {
			return i
		}
		if matchBackslashEscapeHere(text, i) > i {
			return i
		}
		if matchBracePlaceholderHere(text, i) > i {
			return i
		}
	}
	return -1
}

// matchPrintfHere returns the end offset of a printf-style format
// specifier starting at text[i], or i (no match) when text[i:] does
// not start a printf code. Implements okapi's regex:
//
//	%(([-0+#]?)[-0+#]?)((\d\$)?)(([\d\*]*)(\.[\d\*]*)?)[dioxXucsfeEgGpn]
//
// — `%` followed by 0..2 flag chars from `-0+#`, optional `\d$`
// positional argument, optional width (digits or `*`), optional
// `.precision` (digits or `*`), terminated by one of
// `dioxXucsfeEgGpn`. Returns i when there's no terminator.
func matchPrintfHere(text string, i int) int {
	if i >= len(text) || text[i] != '%' {
		return i
	}
	j := i + 1
	// Flags: zero, one, or two of `-0+#`.
	flagCount := 0
	for j < len(text) && flagCount < 2 && isPrintfFlag(text[j]) {
		j++
		flagCount++
	}
	// Optional positional argument: `\d$`.
	if j < len(text) && isASCIIDigit(text[j]) {
		k := j + 1
		for k < len(text) && isASCIIDigit(text[k]) {
			k++
		}
		if k < len(text) && text[k] == '$' {
			j = k + 1
		}
	}
	// Width: digits or `*`.
	for j < len(text) && (isASCIIDigit(text[j]) || text[j] == '*') {
		j++
	}
	// Optional `.precision`.
	if j < len(text) && text[j] == '.' {
		j++
		for j < len(text) && (isASCIIDigit(text[j]) || text[j] == '*') {
			j++
		}
	}
	// Terminator.
	if j >= len(text) {
		return i
	}
	switch text[j] {
	case 'd', 'i', 'o', 'x', 'X', 'u', 'c', 's', 'f', 'e', 'E', 'g', 'G', 'p', 'n':
		return j + 1
	}
	return i
}

// matchBackslashEscapeHere matches okapi's escape regex:
//
//	(\\r\\n)|\\a|\\b|\\f|\\n|\\r|\\t|\\v
//
// at text[i]. Returns the end offset, or i when no match.
func matchBackslashEscapeHere(text string, i int) int {
	if i+1 >= len(text) || text[i] != '\\' {
		return i
	}
	// Two-byte combo first: `\r\n` (literal text `\\r\\n`).
	if i+3 < len(text) && text[i+1] == 'r' && text[i+2] == '\\' && text[i+3] == 'n' {
		return i + 4
	}
	switch text[i+1] {
	case 'a', 'b', 'f', 'n', 'r', 't', 'v':
		return i + 2
	}
	return i
}

// matchBracePlaceholderHere matches okapi's MessageFormat-style
// placeholder regex `\{\d[^\\]*?\}` at text[i]. The `[^\\]*?` is
// non-greedy "any char except backslash"; the close `}` ends the
// match. Returns the end offset (just past `}`), or i when no match.
func matchBracePlaceholderHere(text string, i int) int {
	if i+2 >= len(text) || text[i] != '{' || !isASCIIDigit(text[i+1]) {
		return i
	}
	j := i + 2
	for j < len(text) {
		c := text[j]
		if c == '\\' {
			return i // escape inside breaks the match
		}
		if c == '}' {
			return j + 1
		}
		j++
	}
	return i
}

func isPrintfFlag(b byte) bool {
	switch b {
	case '-', '0', '+', '#':
		return true
	}
	return false
}

func isASCIIDigit(b byte) bool {
	return b >= '0' && b <= '9'
}
