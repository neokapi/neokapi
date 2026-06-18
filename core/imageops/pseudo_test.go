package imageops

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"
)

func solidPNG(t *testing.T, w, h int, c color.RGBA) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.SetRGBA(x, y, c)
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func decode(t *testing.T, b []byte) *image.RGBA {
	t.Helper()
	im, _, err := image.Decode(bytes.NewReader(b))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	bb := im.Bounds()
	out := image.NewRGBA(image.Rect(0, 0, bb.Dx(), bb.Dy()))
	for y := range bb.Dy() {
		for x := range bb.Dx() {
			out.Set(x, y, im.At(bb.Min.X+x, bb.Min.Y+y))
		}
	}
	return out
}

func TestPseudoLocalize(t *testing.T) {
	white := color.RGBA{255, 255, 255, 255}
	src := solidPNG(t, 120, 80, white)

	out, err := PseudoLocalize(src, PseudoOptions{}) // zero → Defaults()
	if err != nil {
		t.Fatalf("PseudoLocalize: %v", err)
	}
	img := decode(t, out)
	if img.Bounds().Dx() != 120 || img.Bounds().Dy() != 80 {
		t.Fatalf("dimensions changed: %v", img.Bounds())
	}

	// A center pixel must be tinted away from pure white (the magenta wash dims G).
	c := img.RGBAAt(60, 40)
	if c.G >= 250 {
		t.Errorf("center not tinted (G=%d, expected reduced by the wash)", c.G)
	}

	// The corner is inside the border frame → should be the border color (red).
	corner := img.RGBAAt(0, 0)
	if corner.R < 200 || corner.G > 60 || corner.B > 60 {
		t.Errorf("border corner not red: %+v", corner)
	}
}

func TestPseudoLocalize_BadInput(t *testing.T) {
	if _, err := PseudoLocalize([]byte("not an image"), PseudoOptions{}); err == nil {
		t.Error("expected decode error for garbage input")
	}
}

func TestPseudoLocalize_CustomTint(t *testing.T) {
	src := solidPNG(t, 40, 40, color.RGBA{0, 0, 0, 255})
	out, err := PseudoLocalize(src, PseudoOptions{
		Tint: color.RGBA{0, 255, 0, 255}, TintAlpha: 1.0, BorderPx: 1, Border: color.RGBA{0, 0, 255, 255},
	})
	if err != nil {
		t.Fatal(err)
	}
	img := decode(t, out)
	// Full-alpha green tint over black → center is green.
	c := img.RGBAAt(20, 20)
	if c.G < 200 || c.R > 60 {
		t.Errorf("center not green-tinted: %+v", c)
	}
}
