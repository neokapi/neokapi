package output

import (
	"io"
	"os"
	"strconv"
	"strings"
	"sync/atomic"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// ColorMode is the resolved --color setting for the process.
type ColorMode int32

const (
	// ColorAuto colorizes only when the destination is a terminal.
	ColorAuto ColorMode = iota
	// ColorAlways forces color even when the destination is not a terminal.
	ColorAlways
	// ColorNever disables color entirely.
	ColorNever
)

// colorMode carries the --color flag across the TextFormatter boundary.
//
// FormatText only receives an io.Writer, so it cannot reach the command's
// flags. Print resolves --color once (see SetColorMode) and stashes it here for
// the renderer to read. NO_COLOR and CLICOLOR_FORCE need no plumbing: termenv
// honors both when it probes the writer.
var colorMode atomic.Int32

// SetColorMode records the resolved --color setting. Print calls this; tests
// call it to pin a profile.
func SetColorMode(m ColorMode) { colorMode.Store(int32(m)) }

// Renderer returns a lipgloss renderer bound to w.
//
// It is deliberately writer-scoped rather than lipgloss's package-level default
// renderer, which probes os.Stdout at init. host is linked into Kapi Desktop
// (Wails, no TTY) and the js/wasm CLI, and tests redirect output with
// cmd.SetOut, so a global stdout-derived profile would be wrong in all three.
func Renderer(w io.Writer) *lipgloss.Renderer {
	r := lipgloss.NewRenderer(w)

	// Pin the background explicitly. Left to itself, lipgloss resolves
	// AdaptiveColor by writing an OSC-11 query to the terminal and reading the
	// reply from stdin. That is unsafe for us on three counts: kapi commands
	// read stdin themselves (kcat -), a terminal that never answers leaves the
	// raw query bytes in the output stream, and those bytes would be baked into
	// the harness screencasts we publish as documentation. SetHasDarkBackground
	// sets lipgloss's explicitBackgroundColor flag, which skips the probe.
	r.SetHasDarkBackground(hasDarkBackground())

	switch ColorMode(colorMode.Load()) {
	case ColorNever:
		r.SetColorProfile(termenv.Ascii)
	case ColorAlways:
		if r.ColorProfile() == termenv.Ascii {
			r.SetColorProfile(termenv.ANSI256)
		}
	}
	return r
}

// hasDarkBackground reports whether to render for a dark terminal.
//
// KAPI_THEME wins (the video harness records light and dark variants of every
// demo and needs to force each). Otherwise COLORFGBG, which terminals set as
// "<fg>;<bg>" with an ANSI index for the background: 0-6 and 8 are the dark
// half of the palette. Absent both, assume dark — the common default, and the
// safer guess, since our light-background colors are the low-lightness ones and
// would be near-invisible if we guessed light and were wrong.
func hasDarkBackground() bool {
	switch strings.ToLower(os.Getenv("KAPI_THEME")) {
	case "light":
		return false
	case "dark":
		return true
	}
	if fgbg := os.Getenv("COLORFGBG"); fgbg != "" {
		if _, bg, ok := strings.Cut(fgbg, ";"); ok {
			// Some terminals report a trailing "default"; only trust an index.
			if n, err := strconv.Atoi(strings.TrimSpace(bg)); err == nil {
				return n <= 6 || n == 8
			}
		}
	}
	return true
}

// Brand palette, converted from the design tokens in
// packages/ui/src/styles/theme-colors.css so the CLI, the desktop app, and the
// web app read as one product.
//
// The Light/Dark pairs are foreground colors chosen for contrast *against* that
// background, so they are not a direct copy of the CSS values: --accent is a
// fill color there, and the light-theme fill (#FFD061) is unreadable as text on
// white. Amber therefore darkens on light terminals and brightens on dark ones.
var (
	colorAccent  = lipgloss.AdaptiveColor{Light: "#8A6A12", Dark: "#FFD061"}
	colorMuted   = lipgloss.AdaptiveColor{Light: "#6E6E68", Dark: "#8D8D85"}
	colorSuccess = lipgloss.AdaptiveColor{Light: "#2E7D46", Dark: "#5DD08A"}
	colorWarn    = lipgloss.AdaptiveColor{Light: "#8A5A00", Dark: "#E2A93B"}
	colorError   = lipgloss.AdaptiveColor{Light: "#C0322F", Dark: "#DF4343"}
)

// Styles is the semantic style set for text output, bound to one writer.
// Callers name the *meaning* of a cell (Accent for the identifier you would
// copy-paste, Muted for chrome) rather than a color, so the palette stays
// changeable in one place.
type Styles struct {
	r *lipgloss.Renderer

	// Title heads a block of output, e.g. "Available formats".
	Title lipgloss.Style
	// Header is a table column heading.
	Header lipgloss.Style
	// Cell is an ordinary table cell.
	Cell lipgloss.Style
	// Accent marks the identifier in a row — the format id, the tool name —
	// the thing a reader will retype into the next command.
	Accent lipgloss.Style
	// Muted is secondary chrome: totals, hints, absent values.
	Muted lipgloss.Style
	// Success, Warn and Error carry status.
	Success lipgloss.Style
	Warn    lipgloss.Style
	Error   lipgloss.Style
}

// Theme returns the style set bound to w.
func Theme(w io.Writer) *Styles {
	r := Renderer(w)
	return &Styles{
		r:       r,
		Title:   r.NewStyle().Bold(true),
		Header:  r.NewStyle().Bold(true).Foreground(colorMuted),
		Cell:    r.NewStyle(),
		Accent:  r.NewStyle().Foreground(colorAccent),
		Muted:   r.NewStyle().Foreground(colorMuted),
		Success: r.NewStyle().Foreground(colorSuccess),
		Warn:    r.NewStyle().Foreground(colorWarn),
		Error:   r.NewStyle().Foreground(colorError),
	}
}

// Renderer exposes the underlying renderer for callers that need to build a
// style the semantic set does not cover.
func (s *Styles) Renderer() *lipgloss.Renderer { return s.r }

// Dim renders v as muted, or an em dash when empty. Use it for secondary
// metadata — a source, a type, a size — that should recede behind the row's
// primary content.
func (s *Styles) Dim(v string) string {
	if v == "" {
		return s.Muted.Render("—")
	}
	return s.Muted.Render(v)
}

// Text renders v in the default foreground, or a muted em dash when empty.
//
// Use it for primary content that happens to be optional — a description, a
// definition. Dim would be wrong here: it is the thing the reader came to read,
// so it should not recede just because the column can be empty.
func (s *Styles) Text(v string) string {
	if v == "" {
		return s.Muted.Render("—")
	}
	return s.Cell.Render(v)
}

// YesNo renders a boolean as a checkmark or a muted dash.
func (s *Styles) YesNo(v bool) string {
	if v {
		return s.Success.Render("✓")
	}
	return s.Muted.Render("—")
}

// isTerminal reports whether w is an interactive terminal. Unlike the ad-hoc
// isatty calls scattered through host, this asks about the actual destination.
func isTerminal(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	return termenv.NewOutput(f).Profile != termenv.Ascii
}
