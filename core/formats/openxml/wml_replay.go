// Paragraph replay for WordprocessingML: a paragraph whose content nothing
// changed goes back as the bytes the source wrote it with, and a paragraph
// that did change keeps the source bytes of every direct child that did not.

package openxml

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
)

// The reader brackets a paragraph's skeleton with marker refs, the device the
// part boundaries already use (skelPartStartPrefix), so the writer can put the
// paragraph's source bytes back in place of what it rendered. A ref that names
// no block is inert to a writer that does not know the marker.
//
//   - skelWMLRegionStart: opens a replay region ahead of a paragraph the
//     reader holds back until it knows where a complex field ends, so the
//     region also covers the skeleton written between that paragraph and
//     the next while it was held.
//   - skelWMLParaPrefix + span: the paragraph whose skeleton follows was read
//     from these source bytes. Opens a replay region when none is open.
//   - skelWMLParaBlockPrefix + span: the same, for a paragraph whose skeleton
//     is its frame around one block ref; the writer can also replay the
//     paragraph's unchanged children into a rendered block.
//   - skelWMLRegionEnd: closes the region. The writer replays the span of the
//     single paragraph in it when every block inside rendered to what its
//     source runs render to.
//   - skelWMLRegionEndSpanPrefix + span: closes a region holding several
//     paragraphs, which a complex field crossing paragraph boundaries makes,
//     with the source bytes of the whole stretch.
//   - skelWMLRegionSkip: closes the region with no replay, because a paragraph
//     in it holds revision markup the writer accepts, holds a <w:t> whose
//     whitespace the writer repairs, was merged into a neighbour, or was
//     dropped.
const (
	skelWMLRegionStart         = "@@SKEL_REGION_START@@"
	skelWMLParaPrefix          = "@@SKEL_PARA@@"
	skelWMLParaBlockPrefix     = "@@SKEL_PARA_BLOCK@@"
	skelWMLRegionEnd           = "@@SKEL_REGION_END@@"
	skelWMLRegionEndSpanPrefix = "@@SKEL_REGION_END_SPAN@@"
	skelWMLRegionSkip          = "@@SKEL_REGION_SKIP@@"
)

// isSkelMarkerRef reports whether a skeleton ref is one of the markers the
// OpenXML readers write for the writer rather than the id of a block.
func isSkelMarkerRef(id string) bool {
	return strings.HasPrefix(id, "@@SKEL_")
}

// wmlRevisionMarkupRE matches the revision elements the reader unwraps or the
// writer strips when revisions are accepted: the content wrappers and the
// paragraph-mark markers, every property-change snapshot, and the move-range
// markers. Replaying a paragraph that holds one would put the revision back.
var wmlRevisionMarkupRE = regexp.MustCompile(`<w:(?:ins|del|moveTo|moveFrom|[A-Za-z]+Change|move(?:To|From)Range(?:Start|End))[\s/>]`)

// paragraphReplayable reports whether a paragraph read from span can go back
// as those bytes: it holds no revision markup while revisions are accepted,
// and every <w:t> in it declares the whitespace it needs.
func (p *wmlParser) paragraphReplayable(span []byte) bool {
	if p.skeletonStore == nil {
		return false
	}
	if p.cfg != nil && p.cfg.AutomaticallyAcceptRevisions && wmlRevisionMarkupRE.Match(span) {
		return false
	}
	return wmlTextIsSpaceSafe(span)
}

// wmlTextIsSpaceSafe reports whether every <w:t> in the bytes declares
// xml:space="preserve" where its text has whitespace at an edge. ECMA-376-1
// §17.3.3.31 leaves the default to XML, and a consumer is free to collapse
// the space, so the writer adds the attribute where it is missing; a
// paragraph that takes that repair is rendered rather than replayed over it.
// SpreadsheetML has the same rule in smlTextIsSpaceSafe.
func wmlTextIsSpaceSafe(span []byte) bool {
	d := newRawDecoder(span)
	for {
		tok, err := d.Token()
		if err != nil {
			return true
		}
		t, ok := tok.(xml.StartElement)
		if !ok || t.Name.Local != "t" || !isWMLPrefixed(t) {
			continue
		}
		preserve := attrVal(t, "space") == "preserve"
		text, err := readCharData(d)
		if err != nil {
			return true
		}
		if !preserve && strings.Trim(text, xmlWhitespace) != text {
			return false
		}
	}
}

// isWMLPrefixed reports whether an element decoded from a fragment belongs to
// WordprocessingML: the namespace resolved when the fragment declared it, or
// the bare `w` prefix when it did not.
func isWMLPrefixed(el xml.StartElement) bool {
	return el.Name.Space == "w" || isWML(el) || isWMLNoNS(el)
}

// wmlParagraphOpenTag returns the start tag a rendered paragraph reopens with:
// the source's own tag, attributes and all. A self-closing slash is dropped
// because the writer closes the element itself.
func wmlParagraphOpenTag(raw string) string {
	if tag, ok := strings.CutSuffix(raw, "/>"); ok {
		return strings.TrimRight(tag, " \t\r\n") + ">"
	}
	return raw
}

// replayParagraphBegin records that a paragraph's skeleton follows. A replay
// region opens on the first paragraph and stays open while a complex field is
// open across the paragraph's end, so the paragraphs the field crosses are
// replayed together or not at all. An ineligible paragraph makes the region it
// belongs to ineligible; on its own, outside any field, it opens no region.
func (p *wmlParser) replayParagraphBegin(start, end int64, span []byte, eligible, block, fieldOpen bool) {
	if p.skeletonStore == nil {
		return
	}
	if !p.regionOpen {
		if !eligible && !fieldOpen {
			return
		}
		p.regionOpen = true
		p.regionStart = start
		p.regionEligible = true
		p.regionParas = 0
	}
	p.regionParas++
	p.regionEnd = end
	if !eligible {
		p.regionEligible = false
		return
	}
	prefix := skelWMLParaPrefix
	if block {
		prefix = skelWMLParaBlockPrefix
	}
	p.skelRef(prefix + string(span))
}

// replayRegionHold opens a region for a paragraph the reader defers, before
// any skeleton the part writes while the paragraph waits. The region's
// source range starts at the deferred paragraph, and the writer's mark sits
// ahead of the deferred skeleton, so the replayed bytes land in source
// order.
func (p *wmlParser) replayRegionHold(start int64) {
	if p.skeletonStore == nil || p.regionOpen {
		return
	}
	p.regionOpen = true
	p.regionStart = start
	p.regionEligible = true
	p.regionParas = 0
	p.skelRef(skelWMLRegionStart)
}

// replayParagraphDropped records a paragraph whose bytes the reader emitted
// nowhere. A region it belongs to cannot be replayed, since the region's span
// would put the paragraph back.
func (p *wmlParser) replayParagraphDropped(d *rawDecoder, fieldOpen bool) {
	if !p.regionOpen {
		return
	}
	p.regionEligible = false
	if !fieldOpen {
		p.replayRegionEnd(d)
	}
}

// replayRegionEnd closes the open region, if any, with the marker the writer
// acts on.
func (p *wmlParser) replayRegionEnd(d *rawDecoder) {
	if !p.regionOpen {
		return
	}
	p.regionOpen = false
	switch {
	case !p.regionEligible:
		p.skelRef(skelWMLRegionSkip)
	case p.regionParas == 1:
		p.skelRef(skelWMLRegionEnd)
	default:
		p.skelRef(skelWMLRegionEndSpanPrefix + d.RangeString(p.regionStart, p.regionEnd))
	}
}

// wmlReplayState is the writer's side of the markers: where the open region
// began in the part buffer, whether any block inside rendered differently
// from its source runs, the span to replay, and the spans already replayed in
// the part, which stand in the buffer as placeholders until the part's
// post-passes have run.
type wmlReplayState struct {
	open      bool
	mark      int
	mismatch  bool
	span      []byte
	blockSpan []byte
	replays   [][]byte
}

// start opens a region at the buffer's current length when none is open.
func (s *wmlReplayState) start(buf *bytes.Buffer) {
	if s.open {
		return
	}
	s.open = true
	s.mark = buf.Len()
	s.mismatch = false
	s.span = nil
}

// para handles a paragraph marker: it opens a region when none is open, keeps
// the first paragraph's span for a region that closes with none of its own,
// and keeps the span for the block that follows when the marker says the
// paragraph's skeleton is a frame around one.
func (s *wmlReplayState) para(buf *bytes.Buffer, span []byte, block bool) {
	s.start(buf)
	if s.span == nil {
		s.span = span
	}
	s.blockSpan = nil
	if block {
		s.blockSpan = span
	}
}

// end closes the region. When nothing inside rendered differently from its
// source, the rendered bytes are replaced by a placeholder for span, which is
// the paragraph's own span when the marker carries none.
func (s *wmlReplayState) end(buf *bytes.Buffer, span []byte) {
	if !s.open {
		return
	}
	s.open = false
	s.blockSpan = nil
	if s.mismatch {
		return
	}
	if span == nil {
		span = s.span
	}
	buf.Truncate(s.mark)
	fmt.Fprintf(buf, "<!--kapi-replay-%d-->", len(s.replays))
	s.replays = append(s.replays, span)
}

// skip closes the region with nothing replayed.
func (s *wmlReplayState) skip() {
	s.open = false
	s.blockSpan = nil
}

// wmlReplayPlaceholderRE matches the placeholder end leaves in the part buffer.
var wmlReplayPlaceholderRE = regexp.MustCompile(`<!--kapi-replay-(\d+)-->`)

// restoreWMLReplays puts the replayed spans back in place of their placeholders
// once the part's post-passes have run, so no pass rewrites bytes the source
// wrote.
func restoreWMLReplays(data []byte, replays [][]byte) []byte {
	if len(replays) == 0 {
		return data
	}
	return wmlReplayPlaceholderRE.ReplaceAllFunc(data, func(m []byte) []byte {
		i, err := strconv.Atoi(string(m[len("<!--kapi-replay-") : len(m)-len("-->")]))
		if err != nil || i < 0 || i >= len(replays) {
			return m
		}
		return replays[i]
	})
}

// wmlRenderedParagraphChildren names the direct children of <w:p> the reader
// renders. Everything else under a paragraph, the proofing marks and
// permission ranges among them, is dropped on read, and <w:pPr> is the frame
// the skeleton carries. A revision wrapper is listed under its own name even
// though the reader unwraps it: the rendered child is then a run, the names
// disagree, and the wrapper's bytes stay out of the output.
var wmlRenderedParagraphChildren = map[string]bool{
	"r":                 true,
	"hyperlink":         true,
	"bookmarkStart":     true,
	"bookmarkEnd":       true,
	"commentRangeStart": true,
	"commentRangeEnd":   true,
	"sdt":               true,
	"smartTag":          true,
	"ins":               true,
	"moveTo":            true,
	"oMathPara":         true,
	"oMath":             true,
	"AlternateContent":  true,
	"fldSimple":         true,
}

// wmlChild is one direct child of a paragraph: its local name and where it
// sits in the bytes it was found in.
type wmlChild struct {
	name       string
	start, end int
}

// wmlParagraphChildren lists the direct children of the first element in
// data, a <w:p>, that the reader renders, in order. It reports false for
// bytes it cannot walk to the end, so a caller falls back to the rendered
// content rather than replaying misaligned spans. The `_GoBack` bookmark Word
// leaves behind is skipped with its matching end, as the reader skips it.
func wmlParagraphChildren(data []byte) ([]wmlChild, bool) {
	d := newRawDecoder(data)
	var out []wmlChild
	depth := 0
	goBackIDs := map[string]bool{}
	for {
		tok, err := d.Token()
		if err != nil {
			return out, errors.Is(err, io.EOF) && depth == 0
		}
		switch t := tok.(type) {
		case xml.StartElement:
			depth++
			if depth != 2 {
				continue
			}
			start := int(d.Offset())
			name := t.Name.Local
			// Consume the child so the next token is its sibling.
			if err := skipElement(d); err != nil {
				return nil, false
			}
			depth--
			if !wmlRenderedParagraphChildren[name] {
				continue
			}
			if name == "bookmarkStart" && attrVal(t, "name") == "_GoBack" {
				goBackIDs[attrVal(t, "id")] = true
				continue
			}
			if name == "bookmarkEnd" && goBackIDs[attrVal(t, "id")] {
				continue
			}
			out = append(out, wmlChild{name: name, start: start, end: int(d.EndOffset())})
		case xml.EndElement:
			depth--
		}
	}
}

// replayUnchangedChildren returns rendered, the content of a paragraph the
// writer produced from its target runs, with every direct child that rendered
// the same from the source runs replaced by the bytes the source wrote it
// with. The three child lists must align one to one by position and by name;
// when they do not, which is what a run the reader merged with its neighbour
// or a wrapper it unwrapped produces, the rendered content comes back as is.
func replayUnchangedChildren(rendered, sourceRendered string, span []byte) string {
	src, ok := wmlParagraphChildren(span)
	if !ok || len(src) == 0 {
		return rendered
	}
	const open, close = "<w:p>", "</w:p>"
	got, ok := wmlParagraphChildren([]byte(open + rendered + close))
	if !ok || len(got) != len(src) {
		return rendered
	}
	want, ok := wmlParagraphChildren([]byte(open + sourceRendered + close))
	if !ok || len(want) != len(src) {
		return rendered
	}
	var out strings.Builder
	prev := 0
	for i := range got {
		gs, ge := got[i].start-len(open), got[i].end-len(open)
		ws, we := want[i].start-len(open), want[i].end-len(open)
		if gs < prev || ge > len(rendered) || ws < 0 || we > len(sourceRendered) {
			return rendered
		}
		out.WriteString(rendered[prev:gs])
		child := rendered[gs:ge]
		if got[i].name == src[i].name && want[i].name == src[i].name && child == sourceRendered[ws:we] {
			out.Write(span[src[i].start:src[i].end])
		} else {
			out.WriteString(child)
		}
		prev = ge
	}
	out.WriteString(rendered[prev:])
	return out.String()
}
