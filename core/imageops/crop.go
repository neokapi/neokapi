package imageops

import (
	"bytes"
	"fmt"
	"image"
	"image/png"
)

// Crop decodes a PNG/JPEG/GIF raster and returns the given sub-rectangle as PNG
// bytes. The rectangle is in pixel coordinates with a top-left origin; it is
// clamped to the image bounds. An empty intersection is an error. Crop is the
// pixel-slice operation behind the image MediaSlicer (AD-030): a low-confidence
// OCR line's bounding box becomes a small crop sent to the refinement LLM, so the
// whole raster never travels to the provider.
func Crop(src []byte, x, y, w, h int) ([]byte, error) {
	img, _, err := image.Decode(bytes.NewReader(src))
	if err != nil {
		return nil, fmt.Errorf("imageops: decode for crop: %w", err)
	}
	want := image.Rect(x, y, x+w, y+h)
	region := want.Intersect(img.Bounds())
	if region.Empty() {
		return nil, fmt.Errorf("imageops: crop region %v does not intersect image bounds %v", want, img.Bounds())
	}

	// SubImage shares pixels where supported; fall back to a copy otherwise.
	type subImager interface {
		SubImage(r image.Rectangle) image.Image
	}
	var out image.Image
	if si, ok := img.(subImager); ok {
		out = si.SubImage(region)
	} else {
		dst := image.NewRGBA(image.Rect(0, 0, region.Dx(), region.Dy()))
		for yy := region.Min.Y; yy < region.Max.Y; yy++ {
			for xx := region.Min.X; xx < region.Max.X; xx++ {
				dst.Set(xx-region.Min.X, yy-region.Min.Y, img.At(xx, yy))
			}
		}
		out = dst
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, out); err != nil {
		return nil, fmt.Errorf("imageops: encode crop: %w", err)
	}
	return buf.Bytes(), nil
}
