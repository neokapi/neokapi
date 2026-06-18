package ocr

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestCTCGreedyDecode(t *testing.T) {
	// dict[0]='a', [1]='b', [2]='c'; CTC classes: 0=blank, 1=a, 2=b, 3=c.
	dict := []string{"a", "b", "c"}
	// Sequence of argmax classes: a a blank a -> "aa" (repeat collapsed across
	// blank gives a, a). Build per-step prob rows with the winner at high prob.
	mk := func(winner int) []float32 {
		row := make([]float32, 4)
		for i := range row {
			row[i] = 0.01
		}
		row[winner] = 0.9
		return row
	}
	probs := [][]float32{mk(1), mk(1), mk(0), mk(1), mk(2), mk(2)} // a,a,blank,a,b,b
	text, conf := ctcGreedyDecode(probs, dict)
	if text != "aab" {
		t.Errorf("text = %q, want %q", text, "aab")
	}
	if conf < 0.8 || conf > 1.0 {
		t.Errorf("conf = %v, want ~0.9", conf)
	}

	if text, _ := ctcGreedyDecode(nil, dict); text != "" {
		t.Errorf("empty input → %q, want empty", text)
	}
}

func TestLoadDict(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "keys.txt")
	if err := os.WriteFile(p, []byte("x\ny\nz\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	d, err := loadDict(p)
	if err != nil {
		t.Fatal(err)
	}
	// trailing space appended per PP-OCR convention
	if !reflect.DeepEqual(d, []string{"x", "y", "z", " "}) {
		t.Errorf("dict = %v", d)
	}
}

func TestConnectedBoxes(t *testing.T) {
	// 10×6 mask with two separate filled rectangles.
	const w, h = 10, 6
	mask := make([]bool, w*h)
	set := func(x0, y0, x1, y1 int) {
		for y := y0; y <= y1; y++ {
			for x := x0; x <= x1; x++ {
				mask[y*w+x] = true
			}
		}
	}
	set(1, 0, 3, 1) // top-left box
	set(5, 3, 8, 4) // lower-right box
	boxes := connectedBoxes(mask, w, h, 2)
	if len(boxes) != 2 {
		t.Fatalf("boxes = %d, want 2: %+v", len(boxes), boxes)
	}
	// reading order: top box first
	if boxes[0] != (box{X0: 1, Y0: 0, X1: 3, Y1: 1}) {
		t.Errorf("box0 = %+v", boxes[0])
	}
	if boxes[1] != (box{X0: 5, Y0: 3, X1: 8, Y1: 4}) {
		t.Errorf("box1 = %+v", boxes[1])
	}
}

func TestConnectedBoxes_MinArea(t *testing.T) {
	const w, h = 5, 5
	mask := make([]bool, w*h)
	mask[0] = true // single pixel, area 1
	if got := connectedBoxes(mask, w, h, 4); len(got) != 0 {
		t.Errorf("single pixel should be filtered by minArea, got %+v", got)
	}
}
