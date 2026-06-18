package pdf

import (
	"reflect"
	"testing"
)

// Bottom-left coords (Top > Bottom). Line 1 sits higher on the page (larger Y)
// than line 2. Within line 1: two adjacent fragments merge into one run, and a
// far-right fragment (big gap) becomes a separate run (a column/cell). A lone
// glyph with large gaps on both sides stays its own run.
func TestGroupRuns(t *testing.T) {
	rects := []RectIn{
		// line 1, left: "Hello" + "World" (small gap → merge with a space)
		{Text: "Hello", L: 10, R: 40, T: 100, B: 90},
		{Text: "World", R: 73, L: 43, T: 100, B: 90},
		// line 1, right column (big gap → separate run)
		{Text: "Col2", L: 200, R: 230, T: 100, B: 90},
		// line 2 (lower): a styled single glyph then the rest, adjacent → merge
		{Text: "T", L: 10, R: 18, T: 80, B: 70},
		{Text: "itle", L: 18, R: 45, T: 80, B: 70},
	}
	runs := GroupRuns(rects)
	got := make([]string, len(runs))
	for i, r := range runs {
		got[i] = r.Text
	}
	want := []string{"Hello World", "Col2", "Title"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("runs = %q, want %q", got, want)
	}
	// "Hello World" unions both fragments' boxes and lists both members.
	if runs[0].L != 10 || runs[0].R != 73 {
		t.Errorf("merged run bbox L/R = %v/%v, want 10/73", runs[0].L, runs[0].R)
	}
	if len(runs[0].Members) != 2 {
		t.Errorf("merged run members = %d, want 2", len(runs[0].Members))
	}
	// "Title" merged the lone "T" glyph back into the word (no single-char block).
	if len(runs[2].Members) != 2 {
		t.Errorf("Title members = %d, want 2", len(runs[2].Members))
	}
}

// Rects arriving bottom-to-top (PDFium does not guarantee reading order) must
// still come out top-of-page first, then left-to-right within a line.
func TestGroupRuns_ReadingOrder(t *testing.T) {
	rects := []RectIn{
		// deliberately out of order: lower line first, right-of-line before left
		{Text: "two", L: 10, R: 40, T: 80, B: 70},      // line 2 (lower)
		{Text: "right", L: 100, R: 130, T: 100, B: 90}, // line 1, right
		{Text: "left", L: 10, R: 40, T: 100, B: 90},    // line 1, left
	}
	runs := GroupRuns(rects)
	got := make([]string, len(runs))
	for i, r := range runs {
		got[i] = r.Text
	}
	// line 1 (top) before line 2; within line 1 a big horizontal gap splits the
	// two fragments into separate runs, left before right.
	want := []string{"left", "right", "two"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("runs = %q, want %q (top-to-bottom, then left-to-right)", got, want)
	}
}

// Two fragments whose centers differ by less than half the line height belong to
// the same line and merge; a larger vertical offset starts a new line.
func TestGroupRuns_LineGrouping(t *testing.T) {
	// height 10 → threshold 0.5*10 = 5. Centers at 95 and 92 (Δ3 ≤ 5) → same line.
	same := GroupRuns([]RectIn{
		{Text: "a", L: 10, R: 20, T: 100, B: 90}, // cy 95
		{Text: "b", L: 22, R: 32, T: 97, B: 87},  // cy 92
	})
	if len(same) != 1 {
		t.Fatalf("Δcy=3 within line height should merge to 1 run, got %d: %+v", len(same), same)
	}
	// Centers at 95 and 80 (Δ15 > 5) → two lines.
	diff := GroupRuns([]RectIn{
		{Text: "a", L: 10, R: 20, T: 100, B: 90}, // cy 95
		{Text: "b", L: 10, R: 20, T: 85, B: 75},  // cy 80
	})
	if len(diff) != 2 {
		t.Fatalf("Δcy=15 beyond line height should be 2 runs, got %d: %+v", len(diff), diff)
	}
}

func TestGroupRuns_Empty(t *testing.T) {
	if r := GroupRuns(nil); r != nil {
		t.Errorf("GroupRuns(nil) = %v, want nil", r)
	}
	if r := GroupRuns([]RectIn{{Text: "   "}}); len(r) != 0 {
		t.Errorf("whitespace-only rects should yield no runs, got %v", r)
	}
}
