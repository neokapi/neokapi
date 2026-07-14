package output

import (
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
	"golang.org/x/term"
)

// Table builds an aligned table for text output.
//
// It replaces the three widths schemes that grew up side by side in this
// package: text/tabwriter, hardcoded "%-26s" format strings, and manual len()
// scans. All three measured in bytes, so a wide rune (CJK, emoji) or an
// over-long cell shifted every column after it — and because the headers are
// translated (see i18n.go), the hardcoded widths could not survive a locale
// change either. lipgloss measures display cells and derives every width from
// the content, so neither can happen.
type Table struct {
	s       *Styles
	w       io.Writer
	headers []string
	rows    [][]string
	// accentCol is the column holding the row's identifier, rendered in the
	// brand accent. -1 disables.
	accentCol int
}

// NewTable starts a table written to w.
func NewTable(w io.Writer) *Table {
	return &Table{s: Theme(w), w: w, accentCol: -1}
}

// Headers sets the column headings.
func (t *Table) Headers(h ...string) *Table {
	t.headers = h
	return t
}

// Accent marks column i as the row identifier, rendered in the brand accent.
// By convention this is the value the reader will retype into the next command
// (a format id, a tool name, a plugin name).
func (t *Table) Accent(i int) *Table {
	t.accentCol = i
	return t
}

// Row appends a row. Cells may already carry styling; widths are measured in
// display cells, so ANSI escapes are not counted.
func (t *Table) Row(cells ...string) *Table {
	t.rows = append(t.rows, cells)
	return t
}

// Rowf appends a row from any values, formatted with %v.
func (t *Table) Rowf(cells ...any) *Table {
	s := make([]string, len(cells))
	for i, c := range cells {
		s[i] = fmt.Sprint(c)
	}
	return t.Row(s...)
}

// Styles exposes the theme so callers can style individual cells before adding
// them (t.Styles().Success.Render("ok")).
func (t *Table) Styles() *Styles { return t.s }

// Render writes the table. It is a no-op when there are no rows, so callers
// decide what an empty result says rather than emitting a bare header.
func (t *Table) Render() {
	if len(t.rows) == 0 {
		return
	}
	fmt.Fprintln(t.w, t.build())
}

// String renders the table without writing it.
func (t *Table) String() string {
	if len(t.rows) == 0 {
		return ""
	}
	return t.build()
}

const (
	// colGap is the space between two columns.
	colGap = 2
	// minCol is the narrowest a column may be squeezed before we stop taking
	// cells from it; below this a truncated value tells you nothing.
	minCol = 6
)

func (t *Table) build() string {
	headers, rows := t.headers, t.rows
	if avail := termWidth(t.w) - len(tableIndent); avail > 0 {
		headers, rows = shrinkToFit(headers, rows, avail)
	}

	// A borderless table indented two spaces, matching how the rest of the CLI
	// indents block content. Columns are separated by padding rather than
	// rules: this is a listing, not a spreadsheet, and box drawing would fight
	// the restrained register the brand guidance asks for.
	hdr := t.s.Header.PaddingRight(colGap)
	cell := t.s.Cell.PaddingRight(colGap)
	accent := t.s.Accent.PaddingRight(colGap)

	tbl := table.New().
		Border(lipgloss.HiddenBorder()).
		BorderTop(false).BorderBottom(false).
		BorderLeft(false).BorderRight(false).
		BorderColumn(false).BorderRow(false).BorderHeader(false).
		StyleFunc(func(row, col int) lipgloss.Style {
			if row == table.HeaderRow {
				return hdr
			}
			if col == t.accentCol {
				return accent
			}
			return cell
		}).
		Headers(headers...).
		Rows(rows...)

	// Width is deliberately not set on the table. lipgloss would satisfy a
	// width budget by *wrapping* cells, turning every row into several lines
	// and destroying the one-row-per-record scannability a listing exists for.
	// shrinkToFit has already truncated cells so the natural width fits.
	return indent(strings.TrimRight(tbl.Render(), "\n"), tableIndent)
}

const tableIndent = "  "

// shrinkToFit truncates cells so the table fits avail display cells on one line
// per row.
//
// It repeatedly takes a cell from whichever column is currently widest, so the
// columns that give up space are the bloated ones (a MIME-type list) rather
// than the narrow ones that carry the identity of the row (a format id). Values
// that get cut are never lost: --json prints them in full, as does the
// corresponding `info` command.
func shrinkToFit(headers []string, rows [][]string, avail int) ([]string, [][]string) {
	n := len(headers)
	if n == 0 {
		return headers, rows
	}

	// Natural width of each column: the widest cell it holds.
	width := make([]int, n)
	for i, h := range headers {
		width[i] = lipgloss.Width(h)
	}
	for _, r := range rows {
		for i := 0; i < n && i < len(r); i++ {
			width[i] = max(width[i], lipgloss.Width(r[i]))
		}
	}

	total := func() int {
		sum := colGap * (n - 1)
		for _, w := range width {
			sum += w
		}
		return sum
	}

	// Take from the widest column until the table fits, or until every column
	// has been squeezed to minCol and there is nothing left to give.
	for total() > avail {
		widest, wi := 0, -1
		for i, w := range width {
			if w > widest && w > minCol {
				widest, wi = w, i
			}
		}
		if wi < 0 {
			break // everything is at the floor; overflow rather than mangle
		}
		width[wi]--
	}

	fit := func(cells []string) []string {
		out := make([]string, len(cells))
		copy(out, cells)
		for i := 0; i < n && i < len(out); i++ {
			if lipgloss.Width(out[i]) > width[i] {
				out[i] = truncate(out[i], width[i])
			}
		}
		return out
	}

	outRows := make([][]string, len(rows))
	for i, r := range rows {
		outRows[i] = fit(r)
	}
	return fit(headers), outRows
}

// indent prefixes each line and strips the trailing cell padding lipgloss adds
// to square the last column. Trailing whitespace is invisible on a terminal but
// shows up in diffs, golden files, and anything piped through grep.
func indent(s, prefix string) string {
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		l = strings.TrimRight(l, " \t")
		if l != "" {
			l = prefix + l
		}
		lines[i] = l
	}
	return strings.Join(lines, "\n")
}

// termWidth returns the terminal width of w, or 0 when w is not a terminal (a
// pipe, a test buffer, the wasm CLI). Returning 0 leaves the table unbounded,
// which is what a consumer reading piped output wants.
func termWidth(w io.Writer) int {
	if !isTerminal(w) {
		return 0
	}
	type fder interface{ Fd() uintptr }
	f, ok := w.(fder)
	if !ok {
		return 0
	}
	cols, _, err := term.GetSize(int(f.Fd()))
	if err != nil || cols <= 0 {
		return 0
	}
	return cols
}

// Title writes a section heading followed by a blank line.
func Title(w io.Writer, text string) {
	fmt.Fprintln(w, Theme(w).Title.Render(text))
	fmt.Fprintln(w)
}

// Note writes a muted trailing line, e.g. a total or a next-step hint.
func Note(w io.Writer, format string, args ...any) {
	fmt.Fprintln(w, Theme(w).Muted.Render(fmt.Sprintf(format, args...)))
}
