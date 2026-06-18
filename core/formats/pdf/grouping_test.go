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

func TestGroupRuns_Empty(t *testing.T) {
	if r := GroupRuns(nil); r != nil {
		t.Errorf("GroupRuns(nil) = %v, want nil", r)
	}
	if r := GroupRuns([]RectIn{{Text: "   "}}); len(r) != 0 {
		t.Errorf("whitespace-only rects should yield no runs, got %v", r)
	}
}
