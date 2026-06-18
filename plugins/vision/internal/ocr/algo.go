package ocr

import (
	"bufio"
	"os"
	"sort"
)

// algo.go holds the pure-Go OCR algorithms — CTC greedy decoding, the
// recognition dictionary, and connected-component box extraction from a
// detection probability map. They carry no native dependency and no build tag,
// so they are unit-tested directly; the onnx-tagged engine (engine_onnx.go)
// calls them around onnxruntime inference.

// box is an axis-aligned pixel box (inclusive corners).
type box struct{ X0, Y0, X1, Y1 int }

func (b box) w() int { return b.X1 - b.X0 + 1 }
func (b box) h() int { return b.Y1 - b.Y0 + 1 }

// loadDict reads a PP-OCR recognition dictionary (one character per line). The
// returned slice is indexed by (CTC class - 1): CTC class 0 is the blank, and a
// trailing space is appended per PP-OCR convention.
func loadDict(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	var chars []string
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for sc.Scan() {
		chars = append(chars, sc.Text())
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return append(chars, " "), nil
}

// ctcGreedyDecode collapses a recognition model's per-timestep class
// probabilities into text. probs is timesteps × classes (class 0 = CTC blank;
// class i>0 maps to dict[i-1]). It takes the argmax per step, drops blanks and
// consecutive repeats, and returns the text with the mean confidence of the
// emitted (kept) characters.
func ctcGreedyDecode(probs [][]float32, dict []string) (string, float64) {
	var out []byte
	var confSum float64
	var kept int
	prev := -1
	for _, step := range probs {
		cls, p := argmax(step)
		if cls != 0 && cls != prev {
			if idx := cls - 1; idx >= 0 && idx < len(dict) {
				out = append(out, dict[idx]...)
				confSum += float64(p)
				kept++
			}
		}
		prev = cls
	}
	conf := 0.0
	if kept > 0 {
		conf = confSum / float64(kept)
	}
	return string(out), conf
}

func argmax(v []float32) (int, float32) {
	best, bestV := 0, float32(-1)
	for i, x := range v {
		if x > bestV {
			best, bestV = i, x
		}
	}
	return best, bestV
}

// connectedBoxes extracts axis-aligned bounding boxes of 4-connected "true"
// regions in a w×h mask (the binarized detection probability map). Boxes smaller
// than minArea pixels are dropped (noise). Returned in reading order
// (top-to-bottom, then left-to-right).
func connectedBoxes(mask []bool, w, h, minArea int) []box {
	if w <= 0 || h <= 0 || len(mask) < w*h {
		return nil
	}
	seen := make([]bool, w*h)
	var boxes []box
	stack := make([]int, 0, 64)
	for start := 0; start < w*h; start++ {
		if !mask[start] || seen[start] {
			continue
		}
		// Flood-fill this component, tracking its extent.
		b := box{X0: start % w, Y0: start / w, X1: start % w, Y1: start / w}
		stack = stack[:0]
		stack = append(stack, start)
		seen[start] = true
		for len(stack) > 0 {
			idx := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			x, y := idx%w, idx/w
			if x < b.X0 {
				b.X0 = x
			}
			if x > b.X1 {
				b.X1 = x
			}
			if y < b.Y0 {
				b.Y0 = y
			}
			if y > b.Y1 {
				b.Y1 = y
			}
			// 4-neighbors
			if x > 0 && mask[idx-1] && !seen[idx-1] {
				seen[idx-1] = true
				stack = append(stack, idx-1)
			}
			if x < w-1 && mask[idx+1] && !seen[idx+1] {
				seen[idx+1] = true
				stack = append(stack, idx+1)
			}
			if y > 0 && mask[idx-w] && !seen[idx-w] {
				seen[idx-w] = true
				stack = append(stack, idx-w)
			}
			if y < h-1 && mask[idx+w] && !seen[idx+w] {
				seen[idx+w] = true
				stack = append(stack, idx+w)
			}
		}
		if b.w()*b.h() >= minArea {
			boxes = append(boxes, b)
		}
	}
	sortReadingOrder(boxes)
	return boxes
}

// sortReadingOrder sorts boxes top-to-bottom, then left-to-right, treating boxes
// whose vertical centers are within half the median height as the same line.
func sortReadingOrder(boxes []box) {
	if len(boxes) < 2 {
		return
	}
	heights := make([]int, len(boxes))
	for i, b := range boxes {
		heights[i] = b.h()
	}
	sort.Ints(heights)
	med := heights[len(heights)/2]
	tol := med / 2
	if tol < 1 {
		tol = 1
	}
	sort.SliceStable(boxes, func(i, j int) bool {
		ci := (boxes[i].Y0 + boxes[i].Y1) / 2
		cj := (boxes[j].Y0 + boxes[j].Y1) / 2
		if abs(ci-cj) > tol {
			return ci < cj
		}
		return boxes[i].X0 < boxes[j].X0
	})
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
