package output

import (
	"io"
	"os"
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

// Renderer returns a lipgloss renderer bound to w, using the process-global
// color mode (set by Print via SetColorMode). Callers that resolve the mode
// themselves — help rendering, which runs before Print — should use
// RendererWithMode instead of mutating the global.
func Renderer(w io.Writer) *lipgloss.Renderer {
	return RendererWithMode(w, ColorMode(colorMode.Load()))
}

// RendererWithMode returns a lipgloss renderer bound to w for an explicitly
// supplied color mode, touching no package state.
//
// It is deliberately writer-scoped rather than lipgloss's package-level default
// renderer, which probes os.Stdout at init. host is linked into Kapi Desktop
// (Wails, no TTY) and the js/wasm CLI, and tests redirect output with
// cmd.SetOut, so a global stdout-derived profile would be wrong in all three.
func RendererWithMode(w io.Writer, mode ColorMode) *lipgloss.Renderer {
	r := lipgloss.NewRenderer(w)

	// Never let lipgloss resolve a background. Its AdaptiveColor path writes an
	// OSC-11 query to the terminal and reads the reply from stdin — which
	// collides with commands that read stdin (kcat -), leaves raw escape bytes
	// in the stream when nothing answers (CI, pipes, the screencasts we publish
	// as documentation), and costs a timeout at startup. The palette below is
	// background-independent, so there is nothing to detect: declaring the
	// background sets lipgloss's explicitBackgroundColor flag and the probe
	// never runs.
	r.SetHasDarkBackground(true)

	switch mode {
	case ColorNever:
		r.SetColorProfile(termenv.Ascii)
	case ColorAlways:
		// TrueColor, not ANSI256. Forcing color means writing somewhere termenv
		// cannot probe — a file, a pipe into `less -R`, a recorder — and the
		// palette below only carries its contrast guarantee when the exact hex
		// survives. A lower profile would quantize it onto whatever the reader's
		// 16- or 256-color palette happens to hold.
		if r.ColorProfile() == termenv.Ascii {
			r.SetColorProfile(termenv.TrueColor)
		}
	}
	return r
}

// The palette. One set of colors, legible on any terminal background.
//
// There is no light/dark variant and nothing to detect, which is the point.
// Every way of learning the terminal's background is either unavailable or
// harmful: COLORFGBG is unset by most modern terminals (iTerm2, Terminal.app,
// Alacritty, kitty, Ghostty, WezTerm, VS Code), the OS appearance setting is not
// the terminal's theme, and the OSC-11 query reads from stdin (see Renderer).
// Guessing is worse than not asking: a palette tuned for dark is close to
// invisible when the guess is wrong.
//
// So the colors are chosen not to care. Contrast against a background rises for
// one end of the lightness range and falls for the other, and the two curves
// cross: a foreground at that crossing has the best possible worst case. Solved
// against the real backgrounds people use — pure white through Solarized Light,
// pure black through One Dark (whose #282C34 is *elevated*, and is the binding
// constraint, not black) — the crossing sits at relative luminance ~0.22.
//
// Every color below is pinned to that luminance and pushed to maximum chroma at
// the brand's hue angles, so the amber accent still reads as the amber in
// packages/ui/src/styles/theme-colors.css. Worst case across every hue and every
// mainstream theme is 3.6:1, clearing the WCAG bar for UI text on all of them.
// TestPaletteIsLegibleOnAnyBackground holds the line.
//
// Muted is the same luminance with zero chroma. It recedes because it is
// achromatic next to a saturated accent, not because it is dimmer — so it stays
// as readable as everything else instead of degrading into gray mush.
var (
	colorAccent  = lipgloss.Color("#A37B00") // amber — the brand hue
	colorSuccess = lipgloss.Color("#039544")
	colorWarn    = lipgloss.Color("#C86703")
	colorError   = lipgloss.Color("#FF142F")
	colorMuted   = lipgloss.Color("#818181")
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

// Theme returns the style set bound to w, using the process-global color mode.
func Theme(w io.Writer) *Styles {
	return ThemeWithMode(w, ColorMode(colorMode.Load()))
}

// ThemeWithMode returns the style set bound to w for an explicitly supplied
// color mode, touching no package state. Use it when the caller resolves
// --color itself (e.g. help, which renders before Print sets the global).
func ThemeWithMode(w io.Writer, mode ColorMode) *Styles {
	r := RendererWithMode(w, mode)
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
