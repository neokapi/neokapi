package pdf

import (
	"math"
	"sort"
	"strings"
)

// RectIn is a PDFium text rectangle in bottom-left coordinates (Top > Bottom),
// as returned by FPDFText_GetRect. It is the neutral input to GroupRuns, shared
// by the native plugin reader (go-pdfium responses) and the browser wasm bridge
// (JS values), so the line-grouping logic lives in one place.
type RectIn struct {
	Text             string
	L, T, R, B       float64
}

// RunOut is a grouped line-run: the merged text of one visual run plus its union
// bounding box (bottom-left coords). Members lists the indices (into the input
// rects slice) that were merged into this run, so a caller can gather each run's
// per-rect data (e.g. glyph boxes).
type RunOut struct {
	Text       string
	L, T, R, B float64
	Members    []int
}

// GroupRuns merges PDFium's per-rect fragments into line-level runs. PDFium emits
// a separate rect whenever the font/style changes mid-line, so a styled or
// isolated glyph becomes its own one-character rect; grouping restores coherent
// text. Rects sharing a line (vertical-center overlap) are joined left-to-right
// and split into a new run only at a large horizontal gap — a column or table-
// cell boundary — so multi-column layouts and tables don't collapse into one
// block. Returns runs in reading order (top-to-bottom, then left-to-right).
func GroupRuns(rects []RectIn) []RunOut {
	type item struct {
		text       string
		l, t, r, b float64
		cy, h      float64
		idx        int // index into the input rects slice
	}
	items := make([]item, 0, len(rects))
	for i, rc := range rects {
		if strings.TrimSpace(rc.Text) == "" {
			continue
		}
		l, r := math.Min(rc.L, rc.R), math.Max(rc.L, rc.R)
		t, b := math.Max(rc.T, rc.B), math.Min(rc.T, rc.B)
		items = append(items, item{rc.Text, l, t, r, b, (t + b) / 2, t - b, i})
	}
	if len(items) == 0 {
		return nil
	}
	// Reading order: top of page first (larger center-Y), ties left-to-right.
	// Plain comparator (no epsilon) so the ordering stays transitive.
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].cy != items[j].cy {
			return items[i].cy > items[j].cy
		}
		return items[i].l < items[j].l
	})

	var runs []RunOut
	// Walk sorted rects, accumulating the current line; flush it into runs when a
	// rect drops to a new line.
	line := []item{items[0]}
	lineCy, lineH := items[0].cy, items[0].h
	flush := func(ln []item) {
		if len(ln) == 0 {
			return
		}
		sort.SliceStable(ln, func(i, j int) bool { return ln[i].l < ln[j].l })
		// Average run height for gap thresholds.
		var sumH float64
		for _, it := range ln {
			sumH += it.h
		}
		avgH := sumH / float64(len(ln))
		var cur *RunOut
		var curRight float64
		for _, it := range ln {
			if cur == nil || it.l-curRight > avgH { // large gap → new run (column/cell)
				runs = append(runs, RunOut{Text: strings.TrimSpace(it.text), L: it.l, T: it.t, R: it.r, B: it.b, Members: []int{it.idx}})
				cur = &runs[len(runs)-1]
				curRight = it.r
				continue
			}
			sep := ""
			if it.l-curRight > 0.15*avgH && !strings.HasSuffix(cur.Text, " ") && !strings.HasPrefix(it.text, " ") {
				sep = " "
			}
			cur.Text += sep + it.text
			cur.L, cur.R = math.Min(cur.L, it.l), math.Max(cur.R, it.r)
			cur.T, cur.B = math.Max(cur.T, it.t), math.Min(cur.B, it.b)
			cur.Members = append(cur.Members, it.idx)
			curRight = it.r
		}
	}
	for _, it := range items[1:] {
		if math.Abs(it.cy-lineCy) <= 0.5*math.Max(lineH, it.h) {
			line = append(line, it)
			// Track the line's representative center/height (running max height).
			lineH = math.Max(lineH, it.h)
			continue
		}
		flush(line)
		line = []item{it}
		lineCy, lineH = it.cy, it.h
	}
	flush(line)

	for i := range runs {
		runs[i].Text = strings.TrimSpace(runs[i].Text)
	}
	return runs
}
