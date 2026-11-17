package openxml

import (
	"archive/zip"
	"encoding/xml"
	"strconv"
	"strings"

	"github.com/neokapi/neokapi/core/safeio"
)

// emuBox is a DrawingML transform's offset and extent, in EMU.
type emuBox struct {
	x, y, cx, cy int64
}

// placeholderSet holds the positioned placeholders of one slide layout or
// slide master, indexed both ways a slide placeholder can name its parent.
type placeholderSet struct {
	byIdx  map[string]emuBox
	byType map[string]emuBox
}

// placeholderGeometry resolves the position of a placeholder shape that
// carries no <a:xfrm> of its own. PresentationML lets a slide placeholder
// inherit its frame from the matching placeholder on the slide's layout, and a
// layout placeholder from the matching one on the layout's master
// (ECMA-376-1 §19.3.1.36). Decks place most of their text this way: a title
// or body placeholder on a slide usually states no position at all.
type placeholderGeometry struct {
	// sets holds the positioned placeholders of each layout and master part.
	sets map[string]*placeholderSet
	// parent maps a slide part to its layout and a layout part to its master.
	parent map[string]string
}

// parsePlaceholderGeometry reads the placeholders of every slide layout and
// slide master in the package. Layouts and masters are small template parts;
// a part that cannot be read contributes no positions, and the shapes that
// would inherit from it stay unplaced. Nil when the package has no layouts.
func parsePlaceholderGeometry(zr *zip.Reader, info *containerInfo) *placeholderGeometry {
	g := &placeholderGeometry{sets: make(map[string]*placeholderSet), parent: make(map[string]string)}
	for relsPath, rels := range info.relationships {
		source := relsSourcePart(relsPath)
		for _, rel := range rels {
			if rel.Type != relTypeSlideLayout && rel.Type != relTypeSlideMaster {
				continue
			}
			target := resolveRelTarget(relsPath, rel.Target)
			// A slide names its layout and a layout its master. A master also
			// lists its layouts, and presentation.xml its masters; those point
			// downwards and are not inheritance.
			switch {
			case strings.HasPrefix(source, "ppt/slides/") && rel.Type == relTypeSlideLayout,
				strings.HasPrefix(source, "ppt/slideLayouts/") && rel.Type == relTypeSlideMaster:
				g.parent[source] = target
			}
			if _, done := g.sets[target]; !done {
				g.sets[target] = readPlaceholderSet(zr, target)
			}
		}
	}
	if len(g.parent) == 0 {
		return nil
	}
	return g
}

// relsSourcePart is the part a .rels file describes: "ppt/slides/_rels/slide1.xml.rels"
// describes "ppt/slides/slide1.xml".
func relsSourcePart(relsPath string) string {
	dir, base := "", relsPath
	if i := strings.LastIndex(relsPath, "/"); i >= 0 {
		dir, base = relsPath[:i+1], relsPath[i+1:]
	}
	dir = strings.TrimSuffix(dir, "_rels/")
	return dir + strings.TrimSuffix(base, ".rels")
}

// resolve returns the inherited frame of a placeholder on slidePart, given the
// placeholder's own type and idx attributes as the slide wrote them. A slide
// placeholder matches the layout placeholder with the same idx, or failing
// that the same type; a layout placeholder matches its master by type.
func (g *placeholderGeometry) resolve(slidePart, phType, phIdx string) (emuBox, bool) {
	if g == nil {
		return emuBox{}, false
	}
	layout := g.parent[slidePart]
	if set := g.sets[layout]; set != nil {
		if phIdx != "" {
			if b, ok := set.byIdx[phIdx]; ok {
				return b, true
			}
		}
		if b, ok := set.byType[placeholderTypeKey(phType)]; ok {
			return b, true
		}
	}
	if set := g.sets[g.parent[layout]]; set != nil {
		if b, ok := set.byType[masterPlaceholderType(phType)]; ok {
			return b, true
		}
	}
	return emuBox{}, false
}

// placeholderTypeKey normalises an absent type to "body", the reading the
// parser already gives an untyped placeholder.
func placeholderTypeKey(t string) string {
	if t == "" {
		return "body"
	}
	return t
}

// masterPlaceholderType maps a placeholder type to the master placeholder it
// inherits from. A master carries title, body, date, footer and slide-number
// placeholders only: a centred title is drawn from the title, a subtitle and
// every content placeholder (object, chart, table, picture, …) from the body.
func masterPlaceholderType(t string) string {
	switch t {
	case "title", "ctrTitle":
		return "title"
	case "dt", "ftr", "sldNum", "hdr":
		return t
	default:
		return "body"
	}
}

// readPlaceholderSet collects the placeholders of a layout or master part that
// state a full transform. Shapes inside a group are skipped: their transforms
// are in the group's child space.
func readPlaceholderSet(zr *zip.Reader, part string) *placeholderSet {
	f := zipFileByName(zr, part)
	if f == nil {
		return nil
	}
	rc, err := safeio.DefaultZipLimits.OpenEntry(f)
	if err != nil {
		return nil
	}
	defer rc.Close()

	set := &placeholderSet{byIdx: make(map[string]emuBox), byType: make(map[string]emuBox)}
	dec := xml.NewDecoder(rc)
	var (
		inShape, isPH, inSpPr bool
		hasOff, hasExt        bool
		groupDepth            int
		phType, phIdx         string
		box                   emuBox
	)
	for {
		tok, err := dec.Token()
		if err != nil {
			return set
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "grpSp":
				groupDepth++
			case "sp":
				if groupDepth == 0 {
					inShape, isPH, inSpPr, hasOff, hasExt = true, false, false, false, false
					phType, phIdx, box = "", "", emuBox{}
				}
			case "ph":
				if inShape {
					isPH = true
					phType, phIdx = attrVal(t, "type"), attrVal(t, "idx")
				}
			case "spPr":
				inSpPr = inShape
			case "off":
				if inSpPr && t.Name.Space == dmlNamespace {
					box.x, _ = strconv.ParseInt(attrVal(t, "x"), 10, 64)
					box.y, _ = strconv.ParseInt(attrVal(t, "y"), 10, 64)
					hasOff = true
				}
			case "ext":
				if inSpPr && t.Name.Space == dmlNamespace {
					box.cx, _ = strconv.ParseInt(attrVal(t, "cx"), 10, 64)
					box.cy, _ = strconv.ParseInt(attrVal(t, "cy"), 10, 64)
					hasExt = true
				}
			}
		case xml.EndElement:
			switch t.Name.Local {
			case "grpSp":
				groupDepth--
			case "spPr":
				inSpPr = false
			case "sp":
				if inShape && isPH && hasOff && hasExt {
					if phIdx != "" {
						if _, dup := set.byIdx[phIdx]; !dup {
							set.byIdx[phIdx] = box
						}
					}
					key := placeholderTypeKey(phType)
					if _, dup := set.byType[key]; !dup {
						set.byType[key] = box
					}
				}
				inShape = false
			}
		}
	}
}
