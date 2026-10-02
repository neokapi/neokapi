package xliff2

import (
	"strings"

	"github.com/beevik/etree"

	"github.com/neokapi/neokapi/core/model"
)

// This file renders one segment's <source> or <target> body as XLIFF 2 inline
// markup for the skeleton write path. The reader calls the same functions on
// the block it emits and pairs the result with the element's bytes in the
// skeleton, so the writer replays the document's own bytes for every segment
// that still renders the same, and writes the rendering for one that an edit
// changed.

// renderSourceRef renders the source of the segment a skeleton reference names.
func renderSourceRef(block *model.Block, segIdx int, segID string) string {
	segs := sourceSegsFromBlock(block)
	if !tiles(block.SourceSegmentation(), len(block.Source)) {
		segs = withTermMarks([]seg{{Runs: block.Source}}, block.OverlayOf(model.OverlayTerm))
	}
	s := segmentFor(segs, segIdx, segID)
	if s == nil {
		return ""
	}
	return segmentXML(s, blockCodes(block, s))
}

// renderTargetRef renders the target, in loc, of the segment a skeleton
// reference names: empty when the target holds nothing for that segment. ok is
// false when the block holds no target in loc at all.
func renderTargetRef(block *model.Block, loc model.LocaleID, segIdx int, segID string) (string, bool) {
	if loc.IsEmpty() || !block.HasTarget(loc) {
		return "", false
	}
	segs := targetSegsFromBlock(block, loc)
	key := model.Variant(loc)
	if runs := block.TargetRuns(loc); !tiles(block.SegmentationFor(&key), len(runs)) {
		segs = []seg{{Runs: runs}}
	}
	s := segmentFor(segs, segIdx, segID)
	if s == nil {
		return "", true
	}
	return segmentXML(s, blockCodes(block, s)), true
}

// tiles reports whether a segmentation still describes runs: its spans cover
// them end to end, each starting where the last ended, on run boundaries. An
// edit that rewrote the runs can leave spans that no longer do, because the
// edit text carries no segment boundaries; such runs are written as one
// segment rather than cut where the stale spans fall.
func tiles(o *model.Overlay, n int) bool {
	if o == nil || len(o.Spans) == 0 {
		return true
	}
	at := 0
	for _, sp := range o.Spans {
		r := sp.Range
		if r.Start.Run != at || r.Start.Offset != 0 || r.End.Offset != 0 || r.End.Run < at {
			return false
		}
		at = r.End.Run
	}
	return at == n
}

// segmentFor picks the segment a skeleton reference names: the one carrying its
// id. Runs no segmentation describes, such as a source an edit rewrote whole,
// form one anonymous segment, which the unit's first <segment> takes in full.
func segmentFor(segs []seg, segIdx int, segID string) *seg {
	if segID != "" {
		for i := range segs {
			if segs[i].ID == segID {
				return &segs[i]
			}
		}
	}
	if len(segs) == 1 && segs[0].ID == "" && segIdx == 0 {
		return &segs[0]
	}
	return nil
}

// segmentXML serializes a segment's body as inline markup. Text escapes only
// the characters XML requires, as xmlesc.Text does.
func segmentXML(s *seg, codes codeIndex) string {
	inls := segmentInlines(s, codes)
	root := etree.NewElement("inline")
	renderInlinesInto(root, inls)
	var b strings.Builder
	settings := &etree.WriteSettings{CanonicalText: true}
	for _, tok := range root.Child {
		tok.WriteTo(&b, settings)
	}
	return b.String()
}

// segmentInlines returns the inline IR a segment is written with. While the
// segment's runs still say what its IR says, the IR is the document's own
// structure and is written as read. Once the runs have been edited, the IR is
// rebuilt from them: the text from the runs, and each code with the attributes
// the document gave it. Runs that no XLIFF 2 markup can express are written
// as their text.
func segmentInlines(s *seg, codes codeIndex) []Inline {
	var inls []Inline
	switch {
	case s.Content != nil && irMatchesRuns(s.Content, s.Runs):
		inls = s.Content.Inlines
	default:
		rebuilt, ok := inlinesFromRuns(s.Runs, codes)
		if !ok {
			return []Inline{{Text: &Text{Content: model.RenderRunsWithData(s.Runs)}}}
		}
		inls = rebuilt
	}
	spliced, _ := spliceMarks(inls, s.Marks)
	return spliced
}

// irMatchesRuns reports whether ir still describes runs: the same text with the
// same codes in the same places.
func irMatchesRuns(ir *Content, runs []model.Run) bool {
	irRuns, _ := inlinesToRunsWithMarks(ir.Inlines)
	return model.RunsPlaceholderText(irRuns) == model.RunsPlaceholderText(runs)
}

// codeIndex holds the attributes the document gave each inline code of a unit,
// so a code an edit moved is written as the document wrote it.
type codeIndex struct {
	pc map[string]CodeAttrs // <pc> by id
	sc map[string]CodeAttrs // paired <sc> by id
	ec map[string]CodeAttrs // paired <ec> by startRef
	ph map[string]Inline    // <ph>, and isolated <sc>/<ec>, by id
}

// blockCodes indexes the codes of every segment IR the block carries, those of
// s first, so a target that holds no IR of its own (a translation added to the
// file) takes its codes' attributes from the source.
func blockCodes(block *model.Block, s *seg) codeIndex {
	ix := codeIndex{pc: map[string]CodeAttrs{}, sc: map[string]CodeAttrs{}, ec: map[string]CodeAttrs{}, ph: map[string]Inline{}}
	if s != nil && s.Content != nil {
		ix.add(s.Content.Inlines)
	}
	if ir := unitSegmentsIR(block); ir != nil {
		for _, c := range ir.Source {
			ix.add(c.Inlines)
		}
		for _, segs := range ir.Target {
			for _, c := range segs {
				ix.add(c.Inlines)
			}
		}
	}
	return ix
}

// add records the codes of inls; a code already recorded keeps its entry.
func (ix codeIndex) add(inls []Inline) {
	for _, in := range inls {
		switch {
		case in.Ph != nil:
			setOnce(ix.ph, in.Ph.ID, in)
		case in.Pc != nil:
			setOnce(ix.pc, in.Pc.ID, in.Pc.CodeAttrs)
			ix.add(in.Pc.Children)
		case in.Sc != nil && in.Sc.Isolated != "yes":
			setOnce(ix.sc, in.Sc.ID, in.Sc.CodeAttrs)
		case in.Ec != nil && in.Ec.StartRef != "":
			setOnce(ix.ec, in.Ec.StartRef, in.Ec.CodeAttrs)
		case in.Sc != nil:
			setOnce(ix.ph, in.Sc.ID, in)
		case in.Ec != nil:
			setOnce(ix.ph, in.Ec.ID, in)
		case in.Mrk != nil:
			ix.add(in.Mrk.Children)
		}
	}
}

func setOnce[V any](m map[string]V, key string, v V) {
	if _, ok := m[key]; !ok {
		m[key] = v
	}
}

// inlinesFromRuns rebuilds the inline IR of a run sequence: text from the
// runs, and each code from the index, so it keeps its element and attributes.
// A pair the document wrote as <pc> nests; one it wrote as <sc>/<ec> stays a
// pair of empty elements. A code the index does not know is written with its
// id alone. ok is false when the runs cannot be expressed as XLIFF 2 markup: a
// <pc> that closes out of order or never closes, or a run kind the reader
// never produces.
func inlinesFromRuns(runs []model.Run, codes codeIndex) ([]Inline, bool) {
	type frame struct {
		pc   *Pc
		kids []Inline
	}
	stack := []frame{{}}
	appendTo := func(in Inline) {
		top := &stack[len(stack)-1]
		if in.Text != nil {
			top.kids = appendText(top.kids, in.Text.Content)
			return
		}
		top.kids = append(top.kids, in)
	}
	for _, r := range runs {
		switch {
		case r.Text != nil:
			if r.Text.Text != "" {
				appendTo(Inline{Text: &Text{Content: r.Text.Text}})
			}
		case r.Ph != nil:
			if in, ok := codes.ph[r.Ph.ID]; ok {
				appendTo(in)
			} else {
				appendTo(Inline{Ph: &Ph{ID: r.Ph.ID}})
			}
		case r.PcOpen != nil:
			if attrs, ok := codes.sc[r.PcOpen.ID]; ok {
				appendTo(Inline{Sc: &Sc{CodeAttrs: attrs}})
				continue
			}
			attrs, ok := codes.pc[r.PcOpen.ID]
			if !ok {
				attrs = CodeAttrs{ID: r.PcOpen.ID}
			}
			stack = append(stack, frame{pc: &Pc{CodeAttrs: attrs}})
		case r.PcClose != nil:
			top := stack[len(stack)-1]
			if top.pc != nil && top.pc.ID == r.PcClose.ID {
				stack = stack[:len(stack)-1]
				top.pc.Children = top.kids
				appendTo(Inline{Pc: top.pc})
				continue
			}
			if attrs, ok := codes.ec[r.PcClose.ID]; ok {
				appendTo(Inline{Ec: &Ec{CodeAttrs: attrs}})
				continue
			}
			return nil, false
		default:
			return nil, false
		}
	}
	if len(stack) != 1 {
		return nil, false
	}
	return stack[0].kids, true
}
