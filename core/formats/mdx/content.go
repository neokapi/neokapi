package mdx

import (
	"bytes"
	"context"
	"fmt"

	"github.com/neokapi/neokapi/core/model"
)

// content.go implements MDX-specific non-translatable content surfacing
// (#928, treatment A). Block-level JSX text children and GFM table cell prose
// are surfaced as Translatable:false content blocks — visible to ingestion,
// skipped by MT — while the surrounding structure (tags, attributes,
// {expressions}, pipes, padding, the table delimiter row, and all inter-token
// whitespace) stays in the skeleton, so an untranslated read→write reproduces
// the source byte-for-byte.
//
// Each surfacing is SELF-VERIFYING: the splitter partitions the region into
// segments whose concatenation must equal the region exactly. If it does not
// (an ambiguous/unexpected shape), the splitter reports failure and the caller
// falls back to emitting the whole region opaque — exactly as before — so the
// byte-faithful round-trip (the primary acceptance bar) is never at risk.
//
// Surfacing is gated on BOTH r.cfg.ExtractNonTranslatableContent() (default
// on; parity forces it off) AND the presence of a skeleton store (the faithful
// write path). With the flag off the emitted part stream is byte-identical to
// the opaque-only baseline.

// contentSeg is one byte-contiguous slice of a region: either structural
// skeleton (isChild=false) or a surfaceable non-translatable child text run
// (isChild=true).
type contentSeg struct {
	text    []byte
	isChild bool
}

// isASCIISpaceByte reports whether b is ASCII inter-token whitespace.
func isASCIISpaceByte(b byte) bool {
	return b == ' ' || b == '\t' || b == '\n' || b == '\r'
}

// anyChild reports whether any segment is a surfaceable child run.
func anyChild(segs []contentSeg) bool {
	for _, s := range segs {
		if s.isChild {
			return true
		}
	}
	return false
}

// segsReconstruct reports whether concatenating the segments reproduces region
// byte-for-byte. The fail-safe behind every surfacing.
func segsReconstruct(segs []contentSeg, region []byte) bool {
	var buf bytes.Buffer
	for _, s := range segs {
		buf.Write(s.text)
	}
	return bytes.Equal(buf.Bytes(), region)
}

// splitJSXSegments partitions a block-level JSX region into structural
// skeleton segments (tags, attributes, {expression} containers, and all
// inter-tag whitespace) and surfaceable text-child segments (the trimmed prose
// between tags). It returns ok=false if the region is not cleanly partitionable
// (a stray `<`, an unbalanced construct, or a reconstruction mismatch), in
// which case the caller preserves the region verbatim/opaque.
//
// JSX text children are surfaced VERBATIM (a single run, no inline parse) and
// trimmed of leading/trailing ASCII whitespace, with that whitespace kept in
// the skeleton so round-trip stays byte-exact.
func splitJSXSegments(span []byte) ([]contentSeg, bool) {
	var segs []contentSeg
	n := len(span)
	structStart := 0
	flushStruct := func(upto int) {
		if upto > structStart {
			segs = append(segs, contentSeg{text: span[structStart:upto]})
		}
	}

	i := 0
	for i < n {
		switch span[i] {
		case '<':
			sc := &jsxScanner{body: span, pos: i}
			tok, _ := sc.consumeTag()
			if tok == jsxOther || sc.pos <= i {
				// Not a clean JSX tag (e.g. a literal `<` in text) — bail and
				// let the caller keep the region opaque.
				return nil, false
			}
			i = sc.pos
		case '{':
			js := &jsScanner{body: span, pos: i + 1}
			js.skipBraces()
			if js.pos <= i {
				return nil, false
			}
			i = js.pos
		default:
			// A text run extends to the next tag or expression container.
			textStart := i
			for i < n && span[i] != '<' && span[i] != '{' {
				i++
			}
			ls := textStart
			for ls < i && isASCIISpaceByte(span[ls]) {
				ls++
			}
			if ls == i {
				// Pure whitespace — leave it folded into the skeleton run.
				continue
			}
			te := i
			for te > ls && isASCIISpaceByte(span[te-1]) {
				te--
			}
			flushStruct(ls)
			segs = append(segs, contentSeg{text: span[ls:te], isChild: true})
			structStart = te
		}
	}
	flushStruct(n)

	if !segsReconstruct(segs, span) {
		return nil, false
	}
	return segs, true
}

// splitTableSegments partitions a GFM table region into structural skeleton
// segments (pipes, cell padding, the delimiter row, and line breaks) and
// surfaceable cell-text segments (the trimmed prose of each non-empty cell in
// the header and body rows). Cell text is surfaced VERBATIM (a single run, no
// inline parse) so embedded inline markup (`**bold**`, `code spans`) rides
// back exactly. Returns ok=false on a reconstruction mismatch.
func splitTableSegments(region []byte) ([]contentSeg, bool) {
	var segs []contentSeg
	n := len(region)
	structStart := 0
	flushStruct := func(upto int) {
		if upto > structStart {
			segs = append(segs, contentSeg{text: region[structStart:upto]})
		}
	}

	i := 0
	for i < n {
		lineStart := i
		lineEnd := lineEndAt(region, lineStart)
		line := region[lineStart:lineEnd]

		// The delimiter row (and any all-dash/colon row) carries no prose —
		// keep it wholly in the skeleton.
		if !isTableDelimiterRow(line) {
			p := lineStart
			for p < lineEnd {
				if region[p] == '|' && !pipeEscaped(region, lineStart, p) {
					p++ // pipe stays skeleton
					continue
				}
				cellStart := p
				for p < lineEnd && !(region[p] == '|' && !pipeEscaped(region, lineStart, p)) {
					p++
				}
				cellEnd := p
				cs := cellStart
				for cs < cellEnd && (region[cs] == ' ' || region[cs] == '\t') {
					cs++
				}
				ce := cellEnd
				for ce > cs && (region[ce-1] == ' ' || region[ce-1] == '\t') {
					ce--
				}
				if ce > cs {
					flushStruct(cs)
					segs = append(segs, contentSeg{text: region[cs:ce], isChild: true})
					structStart = ce
				}
			}
		}

		i = lineEnd
		if i < n && region[i] == '\n' {
			i++
		}
		if i == lineStart {
			// No progress guard (degenerate input).
			break
		}
	}
	flushStruct(n)

	if !segsReconstruct(segs, region) {
		return nil, false
	}
	return segs, true
}

// pipeEscaped reports whether the `|` at index p (within the line beginning at
// lineStart) is backslash-escaped (an odd number of immediately preceding
// backslashes).
func pipeEscaped(region []byte, lineStart, p int) bool {
	bs := 0
	for j := p - 1; j >= lineStart && region[j] == '\\'; j-- {
		bs++
	}
	return bs%2 == 1
}

// emitContentSegs replays an ordered segment partition: structural segments go
// to the skeleton as text; child segments are surfaced as Translatable:false
// content blocks whose verbatim body rides a skeleton ref. Returns false only
// on context cancellation.
func (r *Reader) emitContentSegs(ctx context.Context, ch chan<- model.PartResult,
	segs []contentSeg, role, blockType, namePrefix string, locale model.LocaleID) bool {

	for _, s := range segs {
		if !s.isChild {
			r.skelText(s.text)
			continue
		}
		r.blockCounter++
		id := fmt.Sprintf("tu%d", r.blockCounter)
		block := model.NewBlock(id, string(s.text)) // single verbatim run
		block.Name = fmt.Sprintf("%s%d", namePrefix, r.blockCounter)
		block.Type = blockType
		block.SourceLocale = locale
		block.Translatable = false
		block.PreserveWhitespace = true
		if role != "" {
			block.SetSemanticRole(role, 0)
		}
		r.skelRef(id)
		if !r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block}) {
			return false
		}
	}
	return true
}

// emitJSX surfaces a block-level JSX region's text children as
// Translatable:false content blocks when surfacing is enabled and feasible;
// otherwise it preserves the region opaque (verbatim skeleton + Data),
// identical to the prior behaviour.
func (r *Reader) emitJSX(ctx context.Context, ch chan<- model.PartResult, span []byte, locale model.LocaleID) bool {
	if r.skeletonStore != nil && r.cfg.ExtractNonTranslatableContent() {
		if segs, ok := splitJSXSegments(span); ok && anyChild(segs) {
			return r.emitContentSegs(ctx, ch, segs, "", "jsx-text", "jsx-text", locale)
		}
	}
	r.emitOpaque(ctx, ch, span, "jsx")
	return true
}

// emitTable surfaces a GFM table region's cell prose as Translatable:false
// content blocks when surfacing is enabled and feasible; otherwise it
// preserves the table opaque (verbatim skeleton + Data), identical to the
// prior behaviour.
func (r *Reader) emitTable(ctx context.Context, ch chan<- model.PartResult, span []byte, locale model.LocaleID) bool {
	if r.skeletonStore != nil && r.cfg.ExtractNonTranslatableContent() {
		if segs, ok := splitTableSegments(span); ok && anyChild(segs) {
			return r.emitContentSegs(ctx, ch, segs, model.RoleTableCell, "table-cell", "cell", locale)
		}
	}
	r.emitOpaque(ctx, ch, span, "table")
	return true
}
