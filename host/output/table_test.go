package output

import (
	"bytes"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// lines splits rendered output, dropping the trailing empty element.
func lines(s string) []string {
	return strings.Split(strings.TrimRight(s, "\n"), "\n")
}

// cellIndex returns the display-cell offset at which sub starts in s.
//
// Not strings.Index: that is a *byte* offset, and a cell holding an em dash (3
// bytes, 1 cell) or a CJK glyph (3 bytes, 2 cells) makes the two disagree. An
// assertion built on strings.Index would reproduce, inside the test, the exact
// byte-vs-cell confusion these tests exist to rule out.
func cellIndex(t *testing.T, s, sub string) int {
	t.Helper()
	i := strings.Index(s, sub)
	require.GreaterOrEqual(t, i, 0, "%q not found in %q", sub, s)
	return lipgloss.Width(s[:i])
}

// TestTableAlignsColumnsToContent is the regression test for the bug this
// package was built to kill: the formats table hardcoded "%-6s" for a column
// whose values include "faithful" (8 cells), so every column after it shifted.
// Widths must come from the content.
func TestTableAlignsColumnsToContent(t *testing.T) {
	var buf bytes.Buffer
	SetColorMode(ColorNever)

	NewTable(&buf).
		Headers("FORMAT", "EDIT", "SOURCE").
		Row("androidxml", "faithful", "built-in").
		Row("archive", "—", "built-in").
		Render()

	got := lines(buf.String())
	require.Len(t, got, 3)

	// SOURCE must start at the same display column in the header and in every
	// row — including the row whose EDIT cell is an em dash rather than the
	// 8-cell "faithful" that used to overflow the hardcoded %-6s.
	col := cellIndex(t, got[0], "SOURCE")
	for _, l := range got[1:] {
		assert.Equal(t, col, cellIndex(t, l, "built-in"), "column misaligned in %q", l)
	}
}

// TestTableMeasuresDisplayCellsNotBytes covers the other half of the old bug:
// %-*s pads by bytes, so a wide rune (two cells, three bytes) or an accented
// Latin letter (one cell, two bytes) threw the alignment off. Headers here are
// translated at runtime, so this is not hypothetical.
func TestTableMeasuresDisplayCellsNotBytes(t *testing.T) {
	var buf bytes.Buffer
	SetColorMode(ColorNever)

	NewTable(&buf).
		Headers("NAME", "MARK").
		Row("日本語フォーマット", "x").      // 9 runes, 18 cells, 27 bytes
		Row("Blåbærsyltetøy", "x"). // 14 runes, 14 cells, 18 bytes
		Row("plain", "x").
		Render()

	got := lines(buf.String())
	require.Len(t, got, 4)

	// The MARK column must land at the same display-cell offset on every line.
	// Byte-based padding puts it at three different offsets here, because the
	// three names have byte lengths of 27, 18 and 5 but cell widths of 18, 14
	// and 5.
	col := cellIndex(t, got[0], "MARK")
	for _, l := range got[1:] {
		assert.Equal(t, col, cellIndex(t, l, "x"), "row misaligned: %q", l)
	}
}

// TestTableTruncatesToTerminalWidthWithoutWrapping asserts shrinkToFit keeps one
// row per record. lipgloss's own Width() would satisfy the budget by wrapping
// cells, which turns a listing into an unscannable block.
func TestTableShrinkToFitKeepsOneRowPerRecord(t *testing.T) {
	headers := []string{"ID", "MIME"}
	rows := [][]string{
		{"archive", "application/zip, application/x-tar, application/gzip"},
		{"csv", "text/csv"},
	}

	gotH, gotR := shrinkToFit(headers, rows, 30)

	assert.Len(t, gotR, 2, "no row was split")
	for _, r := range gotR {
		for _, c := range r {
			assert.NotContains(t, c, "\n", "cell wrapped instead of truncating")
		}
	}
	// The bloated column gives up the space, not the identity column.
	assert.Equal(t, "archive", gotR[0][0])
	assert.Contains(t, gotR[0][1], "…")
	assert.Equal(t, headers, gotH, "headers fit, so they are untouched")
}

// TestTableTruncationPreservesANSI guards against color bleed: cells are styled
// before they are measured, so a byte-naive truncate could cut a cell mid-escape
// and leave the reset code off, spilling the color across the rest of the line.
func TestTableTruncationPreservesANSI(t *testing.T) {
	SetColorMode(ColorAlways)
	defer SetColorMode(ColorNever)

	s := Theme(&bytes.Buffer{})
	styled := s.Accent.Render("application/zip, application/x-tar, application/gzip")
	require.Contains(t, styled, "\x1b[", "precondition: cell is styled")

	_, rows := shrinkToFit([]string{"ID", "MIME"}, [][]string{{"archive", styled}}, 30)
	got := rows[0][1]

	assert.Less(t, lipgloss.Width(got), lipgloss.Width(styled), "cell was truncated")
	assert.True(t, strings.HasSuffix(got, "\x1b[0m") || !strings.Contains(got, "\x1b["),
		"truncated cell must end with a reset, got %q", got)
}

// TestNoColorWhenNotATerminal: FormatText writes to whatever writer it is given.
// A bytes.Buffer (a test, a pipe, the wasm CLI) must get plain text.
func TestNoColorWhenNotATerminal(t *testing.T) {
	SetColorMode(ColorAuto)
	var buf bytes.Buffer

	NewTable(&buf).Headers("A").Row("b").Render()

	assert.NotContains(t, buf.String(), "\x1b[", "ANSI leaked into a non-terminal writer")
}

// TestColorNeverDisablesStyling covers --color=never / NO_COLOR.
func TestColorNeverDisablesStyling(t *testing.T) {
	SetColorMode(ColorNever)
	defer SetColorMode(ColorAuto)

	var buf bytes.Buffer
	s := Theme(&buf)
	assert.Equal(t, "ok", s.Success.Render("ok"))
	assert.Equal(t, "—", s.Dim(""))
}

// TestEmptyTableRendersNothing: the caller decides what "no results" says, so an
// empty table must not emit a lone header row.
func TestEmptyTableRendersNothing(t *testing.T) {
	var buf bytes.Buffer
	NewTable(&buf).Headers("A", "B").Render()
	assert.Empty(t, buf.String())
}

// TestNoTrailingWhitespace: invisible on a terminal, but it shows up in golden
// files and breaks `grep -c ' $'`-style checks.
func TestNoTrailingWhitespace(t *testing.T) {
	SetColorMode(ColorNever)
	var buf bytes.Buffer

	NewTable(&buf).
		Headers("A", "LONG_HEADER").
		Row("x", "y").
		Render()

	for _, l := range lines(buf.String()) {
		assert.Equal(t, l, strings.TrimRight(l, " \t"), "trailing whitespace in %q", l)
	}
}

// TestTruncateIsRuneSafe: the previous implementation sliced bytes, which could
// cut a multi-byte rune in half and emit invalid UTF-8.
func TestTruncateIsRuneSafe(t *testing.T) {
	for _, in := range []string{"日本語フォーマット", "Blåbærsyltetøy", "emoji 🎉 tail"} {
		for n := 1; n <= lipgloss.Width(in); n++ {
			got := truncate(in, n)
			assert.True(t, utf8ValidString(got), "truncate(%q, %d) = %q is not valid UTF-8", in, n, got)
			assert.LessOrEqual(t, lipgloss.Width(got), n, "truncate(%q, %d) overflowed", in, n)
		}
	}
}

func utf8ValidString(s string) bool {
	for _, r := range s {
		if r == '�' {
			return false
		}
	}
	return true
}
