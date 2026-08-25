package mdx

import (
	"bytes"
	"context"
	"fmt"
	"strings"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/translatability"
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
	// scope groups consecutive child segments that share an enclosing
	// structure — the cells of one table row. Every change of value opens a
	// fresh naming scope, so a cell is addressed by its row and column rather
	// than by a count across the whole table. Zero means no such structure.
	scope int
	// element is the innermost JSX element this text sits in, in its source
	// spelling, or "" at the top level of a region or inside a fragment.
	element string
	// translatable is the splitter's answer, not a default this struct
	// supplies. emitContentSegs serves both the JSX and table paths, and a
	// shared default is how the table's cells — which name no element — would
	// quietly inherit whatever the JSX path decided.
	translatable bool
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
// jsxTextTranslatable answers whether text directly inside element belongs to
// the translator, from the W3C table the JSX transform uses.
//
// An element the table does not classify is a container, and every React
// component arrives that way. Containers are PROMOTED rather than skipped:
// dropping the text of an unfamiliar component is how a page ends up half
// translated with nothing to say why. Each promotion is recorded so the
// inference is reported, exactly as the TypeScript transform reports it.
//
// Text with no enclosing element is at the top level of a JSX region or inside
// a fragment, which is prose in a component's body.
func jsxTextTranslatable(element string, promoted *[]string) bool {
	if element == "" {
		return true
	}
	switch translatability.Classify(strings.ToLower(element)) {
	case translatability.Yes:
		return true
	case translatability.No:
		return false
	default:
		*promoted = append(*promoted, element)
		return true
	}
}

// The second return names the elements promoted while splitting, so the
// caller can report the inference.
func splitJSXSegments(span []byte) ([]contentSeg, []string, bool) {
	var segs []contentSeg
	n := len(span)
	structStart := 0
	flushStruct := func(upto int) {
		if upto > structStart {
			segs = append(segs, contentSeg{text: span[structStart:upto]})
		}
	}

	// stack holds the open element names, innermost last, so a text run can
	// be attributed to the element it sits in.
	var stack []string
	// promotedHere collects elements the table does not classify whose text
	// was taken as translatable, for the caller to report.
	var promotedHere []string
	enclosing := func() string {
		if len(stack) == 0 {
			return ""
		}
		return stack[len(stack)-1]
	}

	i := 0
	for i < n {
		// A fenced code block is structure, not child prose: its bytes ride
		// the skeleton whole, and nothing inside it is read as a tag.
		if end, ok := fenceRegionAt(span, i); ok {
			i = end
			continue
		}
		switch span[i] {
		case '<':
			name := jsxTagNameAt(span, i)
			sc := &jsxScanner{body: span, pos: i}
			tok, selfClosing := sc.consumeTag()
			if tok == jsxOther || sc.pos <= i {
				// Not a clean JSX tag (e.g. a literal `<` in text) — bail and
				// let the caller keep the region opaque.
				return nil, nil, false
			}
			if tok == jsxStartTag {
				// An attribute's value is copy the reader sees, so it leaves
				// the skeleton and becomes a child of the tag it sits in.
				for _, av := range jsxTranslatableAttrValues(span[i:sc.pos], name) {
					vs, ve := i+av.start, i+av.end
					flushStruct(vs)
					segs = append(segs, contentSeg{
						text: span[vs:ve], isChild: true, element: name, translatable: true,
					})
					structStart = ve
				}
			}
			switch {
			case tok == jsxStartTag && !selfClosing:
				stack = append(stack, name)
			case tok == jsxEndTag && len(stack) > 0:
				stack = stack[:len(stack)-1]
			}
			i = sc.pos
		case '{':
			js := &jsScanner{body: span, pos: i + 1}
			js.skipBraces()
			if js.pos <= i {
				return nil, nil, false
			}
			i = js.pos
		default:
			// A text run extends to the next tag, expression container, or
			// fence opener. Inline code spans ride inside the run: their
			// angle brackets and braces are code, not structure.
			textStart := i
			for i < n && span[i] != '<' && span[i] != '{' {
				if _, isFence := fenceRegionAt(span, i); isFence {
					break
				}
				if end, ok := codeSpanEnd(span, i); ok {
					i = end
					continue
				}
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
			el := enclosing()
			segs = append(segs, contentSeg{
				text: span[ls:te], isChild: true, element: el,
				translatable: jsxTextTranslatable(el, &promotedHere),
			})
			structStart = te
		}
	}
	flushStruct(n)

	if !segsReconstruct(segs, span) {
		return nil, nil, false
	}
	return segs, promotedHere, true
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
	row := 0
	for i < n {
		lineStart := i
		lineEnd := lineEndAt(region, lineStart)
		line := region[lineStart:lineEnd]

		// The delimiter row (and any all-dash/colon row) carries no prose —
		// keep it wholly in the skeleton.
		if !isTableDelimiterRow(line) {
			row++
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
					// A table cell holds prose a reader reads, so it belongs to
					// the translator like any paragraph. The cell's padding and
					// the pipes stay in the skeleton, and BlockPropVerbatim
					// keeps the writer from re-escaping what it emits.
					segs = append(segs, contentSeg{
						text: region[cs:ce], isChild: true, scope: row, translatable: true,
					})
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
//
// scopeKind, when set, names the structure that groups consecutive child
// segments (a table row); each group opens its own naming scope so a cell is
// addressed by row and column.
func (r *Reader) emitContentSegs(ctx context.Context, ch chan<- model.PartResult,
	segs []contentSeg, role, blockType, nameKind, scopeKind string, locale model.LocaleID) bool {

	openScope := 0
	popScope := func() {}
	defer func() { popScope() }()

	for _, s := range segs {
		if !s.isChild {
			r.skelText(s.text)
			continue
		}
		if scopeKind != "" && s.scope != openScope {
			popScope()
			popScope = r.naming.Push(scopeKind)
			openScope = s.scope
		}
		r.blockCounter++
		id := fmt.Sprintf("tu%d", r.blockCounter)
		block := model.NewBlock(id, string(s.text)) // single verbatim run
		block.Name = r.naming.Name(nameKind)
		block.Type = blockType
		block.SourceLocale = locale
		block.Translatable = s.translatable
		block.PreserveWhitespace = true
		// The segment is a byte slice of the region, so the writer must emit
		// it unchanged — see BlockPropVerbatim.
		block.Properties[BlockPropVerbatim] = "1"
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

// notePromotion records, once per element, that an unclassified element's text
// was taken as translatable.
func (r *Reader) notePromotion(element string) {
	if r.promoted == nil {
		r.promoted = map[string]bool{}
	}
	if r.promoted[element] {
		return
	}
	r.promoted[element] = true
	r.AddDiagnostic(format.Diagnostic{
		Severity: format.SeverityNeutral,
		Category: "structure.jsx-promoted",
		Message: "<" + element + "> is not in the translatability table, so its " +
			"text was taken as translatable. Add translate=\"no\" to the element " +
			"if it holds markup rather than prose.",
	})
}

// emitJSX surfaces a block-level JSX region's text children as
// Translatable:false content blocks when surfacing is enabled and feasible;
// otherwise it preserves the region opaque (verbatim skeleton + Data),
// identical to the prior behaviour.
func (r *Reader) emitJSX(ctx context.Context, ch chan<- model.PartResult, span []byte, locale model.LocaleID) bool {
	if r.skeletonStore != nil && r.cfg.ExtractNonTranslatableContent() {
		if segs, promoted, ok := splitJSXSegments(span); ok && anyChild(segs) {
			for _, el := range promoted {
				r.notePromotion(el)
			}
			// The element is the structure its text children sit in, so it
			// scopes their ordinals — as a list scopes its items.
			defer r.naming.Push("jsx")()
			return r.emitContentSegs(ctx, ch, segs, "", "jsx-text", "text", "", locale)
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
			defer r.naming.Push("table")()
			return r.emitContentSegs(ctx, ch, segs, model.RoleTableCell, "table-cell", "cell", "row", locale)
		}
	}
	r.emitOpaque(ctx, ch, span, "table")
	return true
}
