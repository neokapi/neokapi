// Package imageops holds dependency-free raster transforms used by the
// localization pipeline. It imports only the standard library so any layer
// (tools, formats) can use it without pulling in format or tool packages.
package imageops

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	_ "image/gif"  // register GIF decoder
	_ "image/jpeg" // register JPEG decoder
	"image/png"    // PNG decoder + encoder
)

// PseudoOptions controls the pseudo-localization transform. The zero value is
// usable; Defaults() fills sensible visible values.
type PseudoOptions struct {
	// Tint is blended over the whole raster (a "raster-color" wash) at TintAlpha.
	Tint      color.RGBA
	TintAlpha float64 // 0..1 (fraction of the tint mixed in)
	// Border is a solid frame drawn around the edge, BorderPx thick (0 = auto).
	Border   color.RGBA
	BorderPx int
	// Band draws a diagonal stripe across the image when true.
	Band bool
}

// Defaults returns clearly-visible pseudo-localization settings: a magenta wash,
// a red frame, and a diagonal band — unmistakable in a UI or build artifact.
func Defaults() PseudoOptions {
	return PseudoOptions{
		Tint:      color.RGBA{R: 255, G: 0, B: 255, A: 255},
		TintAlpha: 0.28,
		Border:    color.RGBA{R: 255, G: 0, B: 0, A: 255},
		BorderPx:  0,
		Band:      true,
	}
}

// PseudoLocalize decodes a PNG/JPEG/GIF image, applies a clearly-visible
// pseudo-localization watermark (a color tint, a solid border, and a diagonal
// band), and re-encodes it as PNG. It is deterministic and dependency-free, so
// a pseudo-localized image is obviously not the source — the visual analog of
// text pseudo-translation. opts.Zero uses Defaults().
func PseudoLocalize(src []byte, opts PseudoOptions) ([]byte, error) {
	if opts == (PseudoOptions{}) {
		opts = Defaults()
	}
	if opts.TintAlpha < 0 {
		opts.TintAlpha = 0
	}
	if opts.TintAlpha > 1 {
		opts.TintAlpha = 1
	}
	srcImg, _, err := image.Decode(bytes.NewReader(src))
	if err != nil {
		return nil, fmt.Errorf("imageops: decode: %w", err)
	}
	b := srcImg.Bounds()
	w, h := b.Dx(), b.Dy()
	if w == 0 || h == 0 {
		return nil, errors.New("imageops: empty image")
	}
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(dst, dst.Bounds(), srcImg, b.Min, draw.Src)

	tintAndBand(dst, opts)
	if px := opts.BorderPx; px != 0 || opts.Border.A != 0 {
		drawBorder(dst, borderWidth(opts.BorderPx, w, h), opts.Border)
	}

	var out bytes.Buffer
	if err := png.Encode(&out, dst); err != nil {
		return nil, fmt.Errorf("imageops: encode: %w", err)
	}
	return out.Bytes(), nil
}

func borderWidth(px, w, h int) int {
	if px > 0 {
		return px
	}
	// Auto: ~2.5% of the shorter side, at least 3px.
	m := min(h, w)
	bw := max(m/40, 3)
	return bw
}

// tintAndBand blends the tint over every pixel and, optionally, darkens a
// diagonal band so the wash is unmistakable even on busy images.
func tintAndBand(img *image.RGBA, opts PseudoOptions) {
	w, h := img.Bounds().Dx(), img.Bounds().Dy()
	a := opts.TintAlpha
	band := opts.Band
	bandHalf := (w + h) / 16 // band thickness (in the x+y diagonal metric)
	for y := range h {
		for x := range w {
			c := img.RGBAAt(x, y)
			r := blend(c.R, opts.Tint.R, a)
			g := blend(c.G, opts.Tint.G, a)
			bl := blend(c.B, opts.Tint.B, a)
			// Diagonal band: a stronger tint along the anti-diagonal.
			if band {
				d := x + y
				center := (w + h) / 2
				if d > center-bandHalf && d < center+bandHalf {
					r = blend(r, opts.Tint.R, 0.5)
					g = blend(g, opts.Tint.G, 0.5)
					bl = blend(bl, opts.Tint.B, 0.5)
				}
			}
			img.SetRGBA(x, y, color.RGBA{R: r, G: g, B: bl, A: c.A})
		}
	}
}

// drawBorder fills a frame of width px around the image edge.
func drawBorder(img *image.RGBA, px int, c color.RGBA) {
	w, h := img.Bounds().Dx(), img.Bounds().Dy()
	if px*2 >= w || px*2 >= h {
		px = (min(w, h) - 1) / 2
	}
	fill := func(x0, y0, x1, y1 int) {
		draw.Draw(img, image.Rect(x0, y0, x1, y1), &image.Uniform{C: c}, image.Point{}, draw.Src)
	}
	fill(0, 0, w, px)   // top
	fill(0, h-px, w, h) // bottom
	fill(0, 0, px, h)   // left
	fill(w-px, 0, w, h) // right
}

// blend mixes v toward t by fraction a (0..1).
func blend(v, t uint8, a float64) uint8 {
	return uint8(float64(v)*(1-a) + float64(t)*a + 0.5)
}
