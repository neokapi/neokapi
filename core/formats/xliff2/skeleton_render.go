package xliff2

import (
	"errors"
	"fmt"
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

// ErrSegmentsLost is the writer's refusal of a unit read with several
// segments whose content no longer divides into them: an edit rewrote the
// runs, and the segmentation that said where each segment ends no longer
// describes them. Writing such a unit would move words from one segment into
// another, or leave a source with no translation beside it.
var ErrSegmentsLost = errors.New("its segments no longer line up with its content")

// ErrCodesUnwritable is the writer's refusal of a segment whose inline codes
// no XLIFF 2 markup can express: a code pair that closes out of order or never
// closes, or a run kind the reader never produces.
var ErrCodesUnwritable = errors.New("its inline codes cannot be written as XLIFF 2 markup")

// checkUnit refuses, before the writer writes any byte, a unit it cannot write
// whole. Content read as several segments must still divide into them: a
// segmentation that no longer tiles the runs, or none at all, is what an edit
// that merged segments leaves, and on a translation it looks the same as a
// tool's translation of the whole unit. Either way the writer cannot tell
// which words belong to which segment. A translation of a unit that was read
// with at most one translated segment carries no such boundaries, and the
// unit's first segment takes it whole (see renderTargetRef). Every segment
// the write renders from its runs must be expressible as XLIFF 2 markup.
func checkUnit(block *model.Block, loc model.LocaleID) error {
	divides := func(ov *model.Overlay, runs []model.Run) bool {
		return ov != nil && len(ov.Spans) > 0 && tiles(ov, runs)
	}
	ir := unitSegmentsIR(block)
	if ir != nil && len(ir.Source) > 1 && !divides(block.SourceSegmentation(), block.Source) {
		return fmt.Errorf("xliff2 writer: unit %q: the source: %w", block.ID, ErrSegmentsLost)
	}
	hasTarget := !loc.IsEmpty() && block.HasTarget(loc)
	if hasTarget && ir != nil && len(ir.Target[loc]) > 1 {
		key := model.Variant(loc)
		if !divides(block.SegmentationFor(&key), block.TargetRuns(loc)) {
			return fmt.Errorf("xliff2 writer: unit %q: the target: %w", block.ID, ErrSegmentsLost)
		}
	}
	segs := writtenSourceSegs(block)
	if hasTarget {
		segs = append(segs, writtenTargetSegs(block, loc)...)
	}
	// Which codes a unit knows does not depend on the segment, so one index
	// serves every segment. Runs that rebuild as markup are writable; runs
	// that do not are writable only while the segment's IR still describes
	// them, since the IR is then written as read (segmentInlines).
	codes := blockCodes(block, nil)
	for i := range segs {
		s := &segs[i]
		if _, ok := inlinesFromRuns(s.Runs, codes); ok {
			continue
		}
		if s.Content == nil || !irMatchesRuns(s.Content, s.Runs) {
			return fmt.Errorf("xliff2 writer: unit %q: %w", block.ID, ErrCodesUnwritable)
		}
	}
	return nil
}

// writtenSourceSegs returns the source segments the writer writes: the
// segmentation's segments while it tiles the runs, and otherwise the runs as
// one anonymous segment.
func writtenSourceSegs(block *model.Block) []seg {
	if !tiles(block.SourceSegmentation(), block.Source) {
		return withMarks([]seg{{Runs: block.Source}}, block.Source, block, nil)
	}
	return sourceSegsFromBlock(block)
}

// writtenTargetSegs is writtenSourceSegs for the target in loc.
func writtenTargetSegs(block *model.Block, loc model.LocaleID) []seg {
	key := model.Variant(loc)
	if runs := block.TargetRuns(loc); !tiles(block.SegmentationFor(&key), runs) {
		return withMarks([]seg{{Runs: runs}}, runs, block, &key)
	}
	return targetSegsFromBlock(block, loc)
}

// renderSourceRef renders the source of the segment a skeleton reference names.
func renderSourceRef(block *model.Block, segIdx int, segID string) (string, error) {
	s := segmentFor(writtenSourceSegs(block), segIdx, refSegmentID(block, segIdx, segID))
	if s == nil {
		return "", nil
	}
	return segmentXML(s, blockCodes(block, s))
}

// renderTargetRef renders the target, in loc, of the segment a skeleton
// reference names: empty when the target holds nothing for that segment. ok is
// false when the block holds no target in loc at all. A target with no
// segmentation is one text for the unit, and the unit's first segment takes
// it; checkUnit has refused one that replaced a translation read segment by
// segment.
func renderTargetRef(block *model.Block, loc model.LocaleID, segIdx int, segID string) (body string, ok bool, err error) {
	if loc.IsEmpty() || !block.HasTarget(loc) {
		return "", false, nil
	}
	s := segmentFor(writtenTargetSegs(block, loc), segIdx, refSegmentID(block, segIdx, segID))
	if s == nil {
		return "", true, nil
	}
	body, err = segmentXML(s, blockCodes(block, s))
	return body, true, err
}

// refSegmentID returns the id of the segment a skeleton reference names. A
// reference spelled without one, as skeletons an earlier build persisted spell
// it, names its segment by position: segIdx counts the unit's <segment>
// elements, which the source segmentation lists in order.
func refSegmentID(block *model.Block, segIdx int, segID string) string {
	if segID != "" {
		return segID
	}
	if ov := block.SourceSegmentation(); ov != nil && segIdx >= 0 && segIdx < len(ov.Spans) {
		return ov.Spans[segIdx].ID
	}
	return ""
}

// tiles reports whether a segmentation still describes runs: its spans cover
// them end to end, each starting where the last ended (model.SpansTile). A
// boundary can fall inside a text run once an edit has joined two segments'
// text and rebased the spans onto it. An edit that rewrote the runs without
// rebasing them can leave spans that no longer tile, because the edit text
// carries no segment boundaries; such runs are written as one segment rather
// than cut where the stale spans fall.
func tiles(o *model.Overlay, runs []model.Run) bool {
	if o == nil || len(o.Spans) == 0 {
		return true
	}
	return model.SpansTile(o.Spans, runs)
}

// segmentFor picks the segment a skeleton reference names: the one carrying its
// id. Runs no segmentation describes, such as the source of a one-segment unit
// an edit rewrote whole or a translation a tool wrote as one text for the
// unit, form one anonymous segment, which the unit's first <segment> takes in
// full.
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
func segmentXML(s *seg, codes codeIndex) (string, error) {
	inls, err := segmentInlines(s, codes)
	if err != nil {
		return "", err
	}
	root := etree.NewElement("inline")
	renderInlinesInto(root, inls)
	var b strings.Builder
	settings := &etree.WriteSettings{CanonicalText: true}
	for _, tok := range root.Child {
		tok.WriteTo(&b, settings)
	}
	return b.String(), nil
}

// segmentInlines returns the inline IR a segment is written with. While the
// segment's runs still say what its IR says, the IR is the document's own
// structure and is written as read. Once the runs have been edited, the IR is
// rebuilt from them: the text from the runs, and each code with the attributes
// the document gave it. Runs that no XLIFF 2 markup can express are refused
// with ErrCodesUnwritable, since writing their text alone would drop every
// code.
func segmentInlines(s *seg, codes codeIndex) ([]Inline, error) {
	codes.native = true // the skeleton was read from this document
	inls, _, ok := segmentBody(s, codes)
	if !ok {
		return nil, ErrCodesUnwritable
	}
	return inls, nil
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
	// native says the unit was read from XLIFF 2, so a segment it holds with
	// no IR of its own is written as markup rebuilt from its runs.
	native bool
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
		ix.native = true
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
