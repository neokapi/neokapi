package vision

import (
	"context"
	"fmt"
	"sort"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/structure"
)

// Region is one detected layout region on a page image, in top-left pixel
// coordinates. Role is a model.Role* value (heading, paragraph, table, figure,
// caption, list, …). ReadingOrder is a 0-based index in the page's reading flow.
type Region struct {
	Role         string
	BBox         model.Rect
	ReadingOrder int
	Confidence   float64
}

// LayoutOptions tunes layout detection. All fields are advisory.
type LayoutOptions struct {
	Lang string
}

// LayoutEngine is an OPTIONAL capability an Engine may also implement: ML layout
// analysis over a page image (path-based, like OCR — the host never loads the
// image bytes). Callers type-assert for it:
//
//	if le, ok := eng.(vision.LayoutEngine); ok { regions, err := le.Layout(...) }
//
// Keeping it separate from Engine means OCR-only backends (and the non-ONNX
// stub) need not implement it; layout is simply absent when unsupported.
type LayoutEngine interface {
	Layout(ctx context.Context, imagePath string, opts LayoutOptions) ([]Region, error)
}

// SortReadingOrder assigns each region a ReadingOrder index by a deterministic
// heuristic: cluster regions into columns by horizontal-center proximity, order
// columns left-to-right, and within each column order top-to-bottom. It is used
// when the layout model does not emit an explicit reading order. Returns the
// regions sorted in that order (and stamps ReadingOrder in place).
func SortReadingOrder(regions []Region) []Region {
	if len(regions) <= 1 {
		for i := range regions {
			regions[i].ReadingOrder = i
		}
		return regions
	}
	// Median region width sets the column-merge tolerance.
	widths := make([]float64, len(regions))
	for i, r := range regions {
		widths[i] = r.BBox.W
	}
	sort.Float64s(widths)
	tol := widths[len(widths)/2] * 0.5
	if tol < 1 {
		tol = 1
	}
	// Column buckets keyed by left-edge proximity, ordered left-to-right.
	type col struct {
		x     float64
		items []Region
	}
	var cols []*col
	// Seed columns from left-sorted regions so a region joins the nearest
	// already-seen column.
	byLeft := append([]Region(nil), regions...)
	sort.SliceStable(byLeft, func(i, j int) bool { return byLeft[i].BBox.X < byLeft[j].BBox.X })
	for _, r := range byLeft {
		placed := false
		for _, c := range cols {
			if abs(c.x-r.BBox.X) <= tol {
				c.items = append(c.items, r)
				placed = true
				break
			}
		}
		if !placed {
			cols = append(cols, &col{x: r.BBox.X, items: []Region{r}})
		}
	}
	sort.SliceStable(cols, func(i, j int) bool { return cols[i].x < cols[j].x })
	out := make([]Region, 0, len(regions))
	for _, c := range cols {
		sort.SliceStable(c.items, func(i, j int) bool { return c.items[i].BBox.Y < c.items[j].BBox.Y })
		out = append(out, c.items...)
	}
	for i := range out {
		out[i].ReadingOrder = i
	}
	return out
}

func abs(f float64) float64 {
	if f < 0 {
		return -f
	}
	return f
}

// PartsFromLayout builds a structured Part stream from layout regions plus OCR
// lines: each OCR line is assigned to the region that contains its center (the
// best-overlap region), regions are emitted in reading order, and each region's
// lines become Blocks carrying the region's semantic role. Lines matching no
// region are appended as paragraphs at the end (nothing is dropped). This is the
// tier-3 path — authoritative roles + reading order from the layout model,
// versus the geometric tier-2 (core/structure.Analyze).
//
// A table region's lines are reconstructed into row/column structure (see
// regionParts → structure.Gridify) so downstream writers render a real table.
func PartsFromLayout(regions []Region, res *OCRResult, counter, groupCounter *int) []*model.Part {
	if res == nil {
		return nil
	}
	ordered := append([]Region(nil), regions...)
	sort.SliceStable(ordered, func(i, j int) bool { return ordered[i].ReadingOrder < ordered[j].ReadingOrder })

	assigned := make([]bool, len(res.Lines))
	var parts []*model.Part

	for _, reg := range ordered {
		var lines []OCRLine
		for i, ln := range res.Lines {
			if assigned[i] || ln.Text == "" {
				continue
			}
			if contains(reg.BBox, center(ln.BBox)) {
				assigned[i] = true
				lines = append(lines, ln)
			}
		}
		if len(lines) == 0 {
			continue
		}
		sortLines(lines)
		parts = append(parts, regionParts(reg, lines, counter, groupCounter)...)
	}

	// Unassigned lines (outside every region) → trailing paragraphs.
	for i, ln := range res.Lines {
		if assigned[i] || ln.Text == "" {
			continue
		}
		parts = append(parts, blockPart(ln, model.RoleParagraph, counter))
	}
	return parts
}

// regionParts emits one region's lines. A table region reconstructs row/column
// cell structure from the lines' geometry (reusing the tier-2 grid clustering);
// other roles emit role-tagged blocks.
func regionParts(reg Region, lines []OCRLine, counter, groupCounter *int) []*model.Part {
	if reg.Role == model.RoleTable {
		cells := make([]*model.Block, 0, len(lines))
		for _, ln := range lines {
			*counter++
			b := model.NewBlock(fmt.Sprintf("tu%d", *counter), ln.Text)
			if ln.BBox.W > 0 || ln.BBox.H > 0 {
				b.SetGeometry(&model.GeometryAnnotation{BBox: ln.BBox, Origin: "top-left"})
			}
			cells = append(cells, b)
		}
		return structure.TableToParts(structure.Gridify(cells), groupCounter)
	}
	role := reg.Role
	if role == "" {
		role = model.RoleParagraph
	}
	parts := make([]*model.Part, 0, len(lines))
	for _, ln := range lines {
		parts = append(parts, blockPart(ln, role, counter))
	}
	return parts
}

func blockPart(ln OCRLine, role string, counter *int) *model.Part {
	*counter++
	b := model.NewBlock(fmt.Sprintf("tu%d", *counter), ln.Text)
	if ln.BBox.W > 0 || ln.BBox.H > 0 {
		b.SetGeometry(&model.GeometryAnnotation{BBox: ln.BBox, Origin: "top-left"})
	}
	if role != "" {
		level := 0
		if role == model.RoleHeading {
			level = 1
		}
		b.SetSemanticRole(role, level)
	}
	return &model.Part{Type: model.PartBlock, Resource: b}
}

func sortLines(lines []OCRLine) {
	sort.SliceStable(lines, func(i, j int) bool {
		ci, cj := center(lines[i].BBox), center(lines[j].BBox)
		if abs(ci.Y-cj.Y) > lines[i].BBox.H*0.5 {
			return ci.Y < cj.Y
		}
		return ci.X < cj.X
	})
}

type pt struct{ X, Y float64 }

func center(r model.Rect) pt { return pt{X: r.X + r.W/2, Y: r.Y + r.H/2} }

func contains(r model.Rect, p pt) bool {
	return p.X >= r.X && p.X <= r.X+r.W && p.Y >= r.Y && p.Y <= r.Y+r.H
}
