package odf

import (
	"encoding/xml"
	"strconv"
	"strings"

	"github.com/neokapi/neokapi/core/model"
)

// Native intrinsic geometry for ODF (Phase 3 / A3 of the structure-geometry
// plan). ODF draw:frame elements carry their position and size directly as
// svg:x / svg:y / svg:width / svg:height physical lengths — no transform matrix
// and no cross-part lookup, unlike IDML/PDF. We track the enclosing frame (and,
// for presentations, the draw:page index) during the walk and attach a
// GeometryAnnotation to every block emitted inside the frame. Coordinates are
// normalized to points, top-left origin. Additive stand-off metadata: the
// writer never reads it, so byte-faithful round-trip is unaffected.

const (
	nsDraw = "urn:oasis:names:tc:opendocument:xmlns:drawing:1.0"
	nsSvg  = "urn:oasis:names:tc:opendocument:xmlns:svg-compatible:1.0"
)

// frameBox is a parsed draw:frame bounding box in points (top-left origin).
// ok is false when the frame did not declare usable svg coordinates.
type frameBox struct {
	x, y, w, h float64
	ok         bool
}

// parseFrameBox reads svg:x/y/width/height off a draw:frame start element and
// returns the box in points. Missing position defaults to 0; a frame with no
// width and height yields ok=false (nothing useful to place).
func parseFrameBox(t xml.StartElement) frameBox {
	var fb frameBox
	var haveW, haveH bool
	for _, a := range t.Attr {
		if a.Name.Space != nsSvg {
			continue
		}
		v, ok := parseODFLength(a.Value)
		if !ok {
			continue
		}
		switch a.Name.Local {
		case "x":
			fb.x = v
		case "y":
			fb.y = v
		case "width":
			fb.w, haveW = v, true
		case "height":
			fb.h, haveH = v, true
		}
	}
	fb.ok = haveW && haveH
	return fb
}

// odfLengthUnits maps an ODF length unit suffix to its size in points
// (1in = 72pt). ODF lengths are physical (XSL-FO) units; px is uncommon and
// approximated at 96dpi.
var odfLengthUnits = map[string]float64{
	"pt": 1,
	"in": 72,
	"cm": 72.0 / 2.54,
	"mm": 72.0 / 25.4,
	"pc": 12, // pica
	"px": 72.0 / 96.0,
}

// parseODFLength parses an ODF physical length ("2.54cm", "1in", "72pt") into
// points. Returns ok=false for an empty, unitless, or unrecognized value.
func parseODFLength(s string) (float64, bool) {
	s = strings.TrimSpace(s)
	if len(s) < 3 {
		return 0, false
	}
	unit := s[len(s)-2:]
	factor, ok := odfLengthUnits[unit]
	if !ok {
		return 0, false
	}
	num, err := strconv.ParseFloat(strings.TrimSpace(s[:len(s)-2]), 64)
	if err != nil {
		return 0, false
	}
	return num * factor, true
}

// applyFrameGeometry attaches the enclosing frame's box (and page number, when
// known) to a block as a GeometryAnnotation. No-op when there is no enclosing
// frame with usable coordinates.
func applyFrameGeometry(block *model.Block, frameStack []frameBox, page int) {
	if block == nil || len(frameStack) == 0 {
		return
	}
	fb := frameStack[len(frameStack)-1]
	if !fb.ok {
		return
	}
	block.SetGeometry(&model.GeometryAnnotation{
		Page:   page,
		BBox:   model.Rect{X: fb.x, Y: fb.y, W: fb.w, H: fb.h},
		Origin: "top-left",
	})
}
