package output

import (
	"fmt"
	"math"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// terminalBackgrounds is the set the palette has to survive: the extremes, plus
// the themes people actually run. One Dark and Dracula matter more than pure
// black — their backgrounds are *elevated*, which squeezes contrast for a
// mid-lightness foreground, so they, not #000000, are the binding constraint on
// the dark side. Solarized Light is the binding constraint on the light side.
var terminalBackgrounds = map[string]string{
	"pure white":      "#FFFFFF",
	"Solarized Light": "#FDF6E3",
	"pure black":      "#000000",
	"VS Code Dark":    "#1E1E1E",
	"One Dark":        "#282C34",
	"Solarized Dark":  "#002B36",
	"Dracula":         "#282A36",
}

// minContrast is the WCAG 2.1 bar for UI components and large text (1.4.11 /
// 1.4.3). Normal-body 4.5:1 is unreachable for *any* single color that must
// also work on the opposite background — the theoretical ceiling for a
// dual-background foreground is 4.58:1 against pure black and white, and less
// once elevated dark themes are in scope. 3:1 is the honest, achievable bar.
const minContrast = 3.0

// TestPaletteIsLegibleOnAnyBackground is the guard that lets kapi ship one
// palette and never ask the terminal what color it is.
//
// The whole no-detection design rests on the claim that these colors are legible
// everywhere. That claim is only worth as much as this test: without it, a
// well-meaning tweak to a hex value would silently reintroduce the light-terminal
// illegibility that detection was supposed to solve.
func TestPaletteIsLegibleOnAnyBackground(t *testing.T) {
	palette := map[string]lipgloss.Color{
		"accent":  colorAccent,
		"success": colorSuccess,
		"warn":    colorWarn,
		"error":   colorError,
		"muted":   colorMuted,
	}

	for name, c := range palette {
		for bgName, bg := range terminalBackgrounds {
			t.Run(fmt.Sprintf("%s_on_%s", name, bgName), func(t *testing.T) {
				got := contrastRatio(t, string(c), bg)
				assert.GreaterOrEqualf(t, got, minContrast,
					"%s (%s) on %s (%s) is %.2f:1, below the %.1f:1 bar — "+
						"this color is not legible on that terminal",
					name, string(c), bgName, bg, got, minContrast)
			})
		}
	}
}

// TestPaletteSurvives256Quantization checks the tier below truecolor.
//
// A 256-color terminal cannot render our hex; it snaps each color onto the fixed
// xterm-256 cube. That cube is a standard, so the result is predictable and the
// contrast guarantee can be checked rather than hoped for — which matters,
// because the snap moves a color and could push it under the bar.
//
// (The tier below *that*, a 16-color terminal, resolves our colors onto the
// user's own theme palette. Those are legible on their own background by
// construction, so there is nothing for us to verify.)
func TestPaletteSurvives256Quantization(t *testing.T) {
	for name, c := range map[string]lipgloss.Color{
		"accent": colorAccent, "success": colorSuccess, "warn": colorWarn,
		"error": colorError, "muted": colorMuted,
	} {
		q := quantize256(t, string(c))
		for bgName, bg := range terminalBackgrounds {
			got := contrastRatio(t, q, bg)
			assert.GreaterOrEqualf(t, got, minContrast,
				"%s quantizes to %s on a 256-color terminal, which is only %.2f:1 on %s",
				name, q, got, bgName)
		}
	}
}

// quantize256 returns the nearest xterm-256 cube/grayscale color, which is what
// a 256-color terminal substitutes for a truecolor request. The 16 system colors
// are excluded: they are theme-defined, so they have no fixed value to compare.
func quantize256(t *testing.T, hex string) string {
	t.Helper()
	r, g, b := parseHex(t, hex)

	var best string
	bestDist := math.MaxFloat64
	consider := func(cr, cg, cb int) {
		d := math.Pow(float64(int(r)-cr), 2) +
			math.Pow(float64(int(g)-cg), 2) +
			math.Pow(float64(int(b)-cb), 2)
		if d < bestDist {
			bestDist, best = d, fmt.Sprintf("#%02X%02X%02X", cr, cg, cb)
		}
	}

	levels := []int{0, 95, 135, 175, 215, 255} // the 6x6x6 color cube
	for _, cr := range levels {
		for _, cg := range levels {
			for _, cb := range levels {
				consider(cr, cg, cb)
			}
		}
	}
	for i := range 24 { // the 24-step grayscale ramp
		v := 8 + i*10
		consider(v, v, v)
	}
	return best
}

// TestMutedIsAchromatic pins the reason Muted can recede without going dim: it
// is the same luminance as the rest of the palette, and separates from Accent by
// chroma alone. A "muted" color that recedes by *darkening* would be the first
// thing to become unreadable on a dark terminal.
func TestMutedIsAchromatic(t *testing.T) {
	r, g, b := parseHex(t, string(colorMuted))
	assert.Equal(t, r, g, "muted must be neutral gray")
	assert.Equal(t, g, b, "muted must be neutral gray")

	// Same luminance band as a chromatic color, within a small tolerance.
	assert.InDelta(t,
		relativeLuminance(t, string(colorAccent)),
		relativeLuminance(t, string(colorMuted)),
		0.02, "muted should sit at the palette's luminance, not below it")
}

// contrastRatio is the WCAG 2.1 contrast formula. It is re-derived here rather
// than imported so the test cannot drift with a dependency.
func contrastRatio(t *testing.T, fg, bg string) float64 {
	t.Helper()
	a, b := relativeLuminance(t, fg), relativeLuminance(t, bg)
	if a < b {
		a, b = b, a
	}
	return (a + 0.05) / (b + 0.05)
}

func relativeLuminance(t *testing.T, hex string) float64 {
	t.Helper()
	r, g, b := parseHex(t, hex)
	lin := func(v uint8) float64 {
		c := float64(v) / 255
		if c <= 0.04045 {
			return c / 12.92
		}
		return math.Pow((c+0.055)/1.055, 2.4)
	}
	return 0.2126*lin(r) + 0.7152*lin(g) + 0.0722*lin(b)
}

func parseHex(t *testing.T, hex string) (r, g, b uint8) {
	t.Helper()
	require.Len(t, hex, 7, "expected #RRGGBB, got %q", hex)
	_, err := fmt.Sscanf(hex, "#%02x%02x%02x", &r, &g, &b)
	require.NoError(t, err, "parsing %q", hex)
	return r, g, b
}
