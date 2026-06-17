package idml

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/neokapi/neokapi/core/model"
)

// Native intrinsic geometry for IDML (Phase 3 / A2 of the structure-geometry
// plan). InDesign stores each TextFrame's GeometricBounds ("y1 x1 y2 x2", in
// the item's own coordinate space) and an ItemTransform affine ("a b c d tx ty")
// mapping that space to the spread (pasteboard). The page-space box is the
// axis-aligned bounding box of the four transformed corners, in points,
// top-left origin. The geometry lives in Spreads/*.xml while text lives in
// Stories/*.xml, so a pre-scan maps each story's Self id to its anchoring
// frame's box; parseStory then attaches it to every block of that story.
//
// v1 limits (documented, mirroring the #864 PPTX/XLSX geometry pass): a story
// threaded through multiple linked frames uses its chain-start frame
// (PreviousTextFrame=="n"), falling back to the first frame seen; the spread
// index serves as the page number (a spread ≈ a page/page-pair). Additive
// stand-off metadata the writer never reads, so byte-faithful round-trip is
// unaffected.

// frameBox is a TextFrame's axis-aligned bounding box in spread coordinates
// (points, top-left origin). ok is false when the frame lacked usable bounds.
type frameBox struct {
	x, y, w, h float64
	page       int // 1-based spread index; 0 = unknown
	ok         bool
}

// scanStoryGeometry walks the Spreads/ parts and maps each story Self id to the
// page-space box of its anchoring TextFrame.
func (r *Reader) scanStoryGeometry(zr *zip.Reader) (map[string]frameBox, error) {
	var spreadNames []string
	for _, f := range zr.File {
		if strings.HasPrefix(f.Name, "Spreads/") && strings.HasSuffix(f.Name, ".xml") {
			spreadNames = append(spreadNames, f.Name)
		}
	}
	sort.Strings(spreadNames)

	out := map[string]frameBox{}
	chainStart := map[string]bool{} // story has a chain-start box recorded

	for i, name := range spreadNames {
		page := i + 1
		zf := zipFileByName(zr, name)
		if zf == nil {
			continue
		}
		data, err := readZipFile(zf)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", name, err)
		}
		dec := xml.NewDecoder(bytes.NewReader(data))
		for {
			tok, err := dec.Token()
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				return nil, fmt.Errorf("parse %s: %w", name, err)
			}
			se, ok := tok.(xml.StartElement)
			if !ok || se.Name.Local != "TextFrame" {
				continue
			}
			storyID := attrVal(se.Attr, "ParentStory")
			if storyID == "" {
				continue
			}
			fb, ok := frameBoxFromAttrs(se.Attr, page)
			if !ok {
				continue
			}
			prev := attrVal(se.Attr, "PreviousTextFrame")
			isChainStart := prev == "" || prev == "n"
			switch {
			case isChainStart:
				// Chain-start always wins (and the first chain-start sticks).
				if !chainStart[storyID] {
					out[storyID] = fb
					chainStart[storyID] = true
				}
			case !chainStart[storyID]:
				// Mid-chain/anchor frame: only as a fallback before any
				// chain-start is seen, and only the first such frame.
				if _, seen := out[storyID]; !seen {
					out[storyID] = fb
				}
			}
		}
	}
	return out, nil
}

// frameBoxFromAttrs derives the page-space bounding box from a TextFrame's
// GeometricBounds + ItemTransform attributes.
func frameBoxFromAttrs(attrs []xml.Attr, page int) (frameBox, bool) {
	gb := parseFloats(attrVal(attrs, "GeometricBounds"))
	if len(gb) != 4 {
		return frameBox{}, false
	}
	// GeometricBounds = [y1 x1 y2 x2] = [top left bottom right].
	top, left, bottom, right := gb[0], gb[1], gb[2], gb[3]

	// ItemTransform = [a b c d tx ty]; default identity.
	a, b, c, d, tx, ty := 1.0, 0.0, 0.0, 1.0, 0.0, 0.0
	if tf := parseFloats(attrVal(attrs, "ItemTransform")); len(tf) == 6 {
		a, b, c, d, tx, ty = tf[0], tf[1], tf[2], tf[3], tf[4], tf[5]
	}

	minX, minY := math.Inf(1), math.Inf(1)
	maxX, maxY := math.Inf(-1), math.Inf(-1)
	for _, lx := range []float64{left, right} {
		for _, ly := range []float64{top, bottom} {
			px := a*lx + c*ly + tx
			py := b*lx + d*ly + ty
			minX, maxX = math.Min(minX, px), math.Max(maxX, px)
			minY, maxY = math.Min(minY, py), math.Max(maxY, py)
		}
	}
	return frameBox{x: minX, y: minY, w: maxX - minX, h: maxY - minY, page: page, ok: true}, true
}

// parseFloats splits a space-separated numeric attribute (GeometricBounds,
// ItemTransform) into floats. Returns nil if any field fails to parse.
func parseFloats(s string) []float64 {
	fields := strings.Fields(s)
	if len(fields) == 0 {
		return nil
	}
	out := make([]float64, 0, len(fields))
	for _, f := range fields {
		v, err := strconv.ParseFloat(f, 64)
		if err != nil {
			return nil
		}
		out = append(out, v)
	}
	return out
}

// applyStoryGeometry attaches a story's frame box to a block as a
// GeometryAnnotation. No-op when the box is absent.
func applyStoryGeometry(block *model.Block, fb frameBox) {
	if block == nil || !fb.ok {
		return
	}
	block.SetGeometry(&model.GeometryAnnotation{
		Page:   fb.page,
		BBox:   model.Rect{X: fb.x, Y: fb.y, W: fb.w, H: fb.h},
		Origin: "top-left",
	})
}
