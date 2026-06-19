package av

import (
	"bytes"
	"image"
	"math/bits"

	_ "image/jpeg" // decoders for sampled frames
	_ "image/png"
)

// aHash computes a 64-bit average hash (aHash) of an image: downscale to 8×8
// grayscale, then set each bit where the pixel is at or above the mean. Two
// near-identical frames produce hashes a small Hamming distance apart, so a
// burned-in subtitle that persists across frames is easy to detect and drop.
func aHash(img image.Image) uint64 {
	const n = 8
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w == 0 || h == 0 {
		return 0
	}
	var gray [n * n]float64
	var sum float64
	for gy := range n {
		for gx := range n {
			// Nearest-neighbour sample of the n×n grid over the source.
			sx := b.Min.X + gx*w/n
			sy := b.Min.Y + gy*h/n
			r, g, bl, _ := img.At(sx, sy).RGBA()
			// Rec. 601 luma; RGBA() returns 16-bit channels.
			lum := 0.299*float64(r) + 0.587*float64(g) + 0.114*float64(bl)
			gray[gy*n+gx] = lum
			sum += lum
		}
	}
	mean := sum / float64(n*n)
	var hash uint64
	for i, v := range gray {
		if v >= mean {
			hash |= 1 << uint(i)
		}
	}
	return hash
}

// aHashBytes decodes an encoded image (PNG/JPEG) and returns its aHash.
func aHashBytes(data []byte) (uint64, error) {
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return 0, err
	}
	return aHash(img), nil
}

// hammingDistance counts differing bits between two hashes.
func hammingDistance(a, b uint64) int {
	return bits.OnesCount64(a ^ b)
}

// dedupKeep returns the indices of frames to keep: a frame is dropped when its
// aHash is within maxDistance bits of the last kept frame's hash (i.e. it looks
// the same — e.g. a subtitle that hasn't changed). The first frame is always
// kept. maxDistance of 0 keeps only exact-hash-change frames; a small value
// (≈5) tolerates compression noise. This collapses the per-frame OCR cost on
// video where text persists for seconds (AD-030 §"sample, don't decode every
// frame").
func dedupKeep(hashes []uint64, maxDistance int) []int {
	if len(hashes) == 0 {
		return nil
	}
	keep := []int{0}
	last := hashes[0]
	for i := 1; i < len(hashes); i++ {
		if hammingDistance(hashes[i], last) > maxDistance {
			keep = append(keep, i)
			last = hashes[i]
		}
	}
	return keep
}
