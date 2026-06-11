package openxml

import (
	"bytes"
	"fmt"
	"regexp"
)

// run_merge.go — production cross-source run-envelope fusion (#602/#603).
//
// The WML skeleton write path emits one <w:r> per source run (runToXML /
// emitRunEnvelopes). Okapi's RunMerger additionally fuses ADJACENT source
// runs whose RunProperties are mergeable. emitRunEnvelopes covers the
// same-source split and the br→text cross-source case structurally; the
// four remaining cross-source fusions — handled here — arise in the
// skeleton path where the runs flow through skeleton reconstruction rather
// than writeWMLBlock and so reach the post-pass as separate envelopes:
//
//  1. fuseFldCharEndText        — a bare fldChar-end run fuses with the
//     FOLLOWING bare text run, gated by an immediately-preceding bare text
//     run (the extractable field's display text). RunMerger.java:402-441 +
//     the containsComplexFields gate at 147-149.
//  2. fuseTextPtabEnvelopes     — an alternation of bare <w:t> and <w:ptab/>
//     runs collapses into one envelope. RunMerger.java:402-441.
//  3. fuseAnnotationRefText      — a CommentReference-rPr <w:annotationRef/>
//     run fuses with the following same-rPr text run. RunMerger.java:402-441.
//  4. fuseBarePictAndRPrTextRuns — a bare (rPr-less) <w:pict> run fuses with
//     the FOLLOWING rFonts-only text run, carrying the text run's minified
//     rPr. RunMerger.java:402-441 + RunFonts.canBeMerged (RunFonts.java:
//     190-247).
//
// All four are applied during the faithful flush (the writer routes the
// part bytes through postNonWSOForName), reproducing the run shape and
// count upstream Okapi's RunMerger produces. The bare-pict run carries no
// rPr (a drawing-only Markup chunk); folding it into the following run's
// RunProperties is what upstream Okapi's RunMerger does.
//
// These replace the equivalent post-serialization REGEXES (retired): the
// matchers below reproduce ReplaceAll's left-to-right, non-overlapping
// semantics by SPLICING original bytes — no decode/re-encode through
// encoding/xml (which would mangle self-closing tags / attribute order /
// namespace prefixes and defeat byte parity). Each is a single forward
// pass; the ptab fuse in particular drops the old fixpoint re-scan loop,
// which was the worst regex backtracking cliff (BenchmarkRegexFuseStack:
// 76× slower / 23× more memory than the streaming form on fuse-heavy
// input). Output is byte-identical to the regexes (gated on the 185-fixture
// openxml parity suite: native 175 canon / 10 div must hold).

// isNameBoundary reports whether c terminates an XML element name, mirroring
// a regex `\b` after a tag name (so `<w:t` matches `<w:t>`/`<w:t ...>` but
// not `<w:tab>`/`<w:tbl>`).
func isNameBoundary(c byte) bool {
	switch c {
	case '>', ' ', '\t', '\n', '\r', '/':
		return true
	default:
		return false
	}
}

// matchTextElement matches `<w:t\b[^>]*>[^<]*</w:t>` at data[i:] and returns
// the index just past `</w:t>`, or -1. The element must carry a body (the
// self-closing `<w:t/>` form is not matched, mirroring the regex).
func matchTextElement(data []byte, i int) int {
	const open = "<w:t"
	if !bytes.HasPrefix(data[i:], []byte(open)) {
		return -1
	}
	nameEnd := i + len(open)
	if nameEnd >= len(data) || !isNameBoundary(data[nameEnd]) {
		return -1
	}
	gt := bytes.IndexByte(data[nameEnd:], '>')
	if gt < 0 {
		return -1
	}
	openEnd := nameEnd + gt + 1
	if data[openEnd-2] == '/' { // <w:t .../> — no body
		return -1
	}
	rel := bytes.Index(data[openEnd:], []byte("</w:t>"))
	if rel < 0 {
		return -1
	}
	if bytes.IndexByte(data[openEnd:openEnd+rel], '<') >= 0 { // [^<]* body
		return -1
	}
	return openEnd + rel + len("</w:t>")
}

// matchPtabElement matches `<w:ptab\b[^>]*(?:/>|></w:ptab>)` at data[i:] and
// returns the index just past the element, or -1.
func matchPtabElement(data []byte, i int) int {
	const open = "<w:ptab"
	if !bytes.HasPrefix(data[i:], []byte(open)) {
		return -1
	}
	nameEnd := i + len(open)
	if nameEnd >= len(data) || !isNameBoundary(data[nameEnd]) {
		return -1
	}
	gt := bytes.IndexByte(data[nameEnd:], '>')
	if gt < 0 {
		return -1
	}
	tagEnd := nameEnd + gt + 1
	if data[tagEnd-2] == '/' { // self-closing <w:ptab .../>
		return tagEnd
	}
	rel := bytes.Index(data[tagEnd:], []byte("</w:ptab>")) // open/close form
	if rel != 0 {
		return -1
	}
	return tagEnd + len("</w:ptab>")
}

// matchFldCharEndElement matches `<w:fldChar\b[^>]*\bw:fldCharType="end"[^>]*
// (?:/>|></w:fldChar>)` at data[i:] and returns the index just past the
// element, or -1.
func matchFldCharEndElement(data []byte, i int) int {
	const open = "<w:fldChar"
	if !bytes.HasPrefix(data[i:], []byte(open)) {
		return -1
	}
	nameEnd := i + len(open)
	if nameEnd >= len(data) || !isNameBoundary(data[nameEnd]) {
		return -1
	}
	gt := bytes.IndexByte(data[nameEnd:], '>')
	if gt < 0 {
		return -1
	}
	tagEnd := nameEnd + gt + 1
	// The fldChar open tag's attributes must declare fldCharType="end".
	if !bytes.Contains(data[nameEnd:tagEnd], []byte(`w:fldCharType="end"`)) {
		return -1
	}
	if data[tagEnd-2] == '/' { // self-closing <w:fldChar .../>
		return tagEnd
	}
	rel := bytes.Index(data[tagEnd:], []byte("</w:fldChar>"))
	if rel != 0 {
		return -1
	}
	return tagEnd + len("</w:fldChar>")
}

// matchBareEnvelope matches `<w:r>` + (one element matched by `child`) +
// `</w:r>` at data[i:], returning (childStart, childEnd, end) where end is
// just past `</w:r>`, or ok=false. The envelope must carry no <w:rPr> (the
// child element follows `<w:r>` directly).
func matchBareEnvelope(data []byte, i int, child func([]byte, int) int) (childStart, childEnd, end int, ok bool) {
	const ropen = "<w:r>"
	if !bytes.HasPrefix(data[i:], []byte(ropen)) {
		return 0, 0, 0, false
	}
	cs := i + len(ropen)
	ce := child(data, cs)
	if ce < 0 {
		return 0, 0, 0, false
	}
	const rclose = "</w:r>"
	if !bytes.HasPrefix(data[ce:], []byte(rclose)) {
		return 0, 0, 0, false
	}
	return cs, ce, ce + len(rclose), true
}

// fuseFldCharEndText collapses `[bare-text][bare-fldChar-end][bare-text]`
// into `[bare-text]<w:r>[fldChar-end][text]</w:r>` — keeping the leading
// display-text run separate and fusing the field-end markup with the
// following text run. Faithful single-pass port of the retired
// bareFldCharEndAfterTextThenBareTextRunRE; see run_merge.go header and
// RunMerger.java:402-441 / 147-149.
func fuseFldCharEndText(data []byte) []byte {
	if !bytes.Contains(data, []byte("<w:r><w:fldChar")) {
		return data
	}
	out := make([]byte, 0, len(data))
	pos := 0
	for pos < len(data) {
		rel := bytes.Index(data[pos:], []byte("<w:r><w:t"))
		if rel < 0 {
			out = append(out, data[pos:]...)
			break
		}
		aStart := pos + rel
		// A: the leading bare text envelope (the gate — extractable field
		// display text; XE markers carry <w:instrText> here instead).
		_, _, aEnd, aOK := matchBareEnvelope(data, aStart, matchTextElement)
		if !aOK {
			out = append(out, data[pos:aStart+1]...)
			pos = aStart + 1
			continue
		}
		// B: the bare fldChar-end envelope.
		bcStart, bcEnd, bEnd, bOK := matchBareEnvelope(data, aEnd, matchFldCharEndElement)
		if !bOK {
			out = append(out, data[pos:aEnd]...)
			pos = aEnd
			continue
		}
		// C: the trailing bare text envelope.
		ccStart, ccEnd, cEnd, cOK := matchBareEnvelope(data, bEnd, matchTextElement)
		if !cOK {
			out = append(out, data[pos:aEnd]...)
			pos = aEnd
			continue
		}
		// Splice: A verbatim, then <w:r>[fldChar-end][text]</w:r>.
		out = append(out, data[pos:aEnd]...)
		out = append(out, "<w:r>"...)
		out = append(out, data[bcStart:bcEnd]...)
		out = append(out, data[ccStart:ccEnd]...)
		out = append(out, "</w:r>"...)
		pos = cEnd
	}
	return out
}

// fuseTextPtabEnvelopes collapses a maximal run of two-or-more consecutive
// bare `<w:t>`/`<w:ptab/>` envelopes into one `<w:r>` carrying all children
// side by side. Faithful single-pass port of the retired
// fuseBareTextAndPTabRuns (which looped its three regexes to a fixpoint);
// see run_merge.go header and RunMerger.java:402-441.
func fuseTextPtabEnvelopes(data []byte) []byte {
	if !bytes.Contains(data, []byte("<w:ptab")) {
		return data
	}
	// matchTextOrPtab returns the children byte span and end of a bare
	// text-or-ptab envelope at data[i:], or ok=false.
	matchTextOrPtab := func(i int) (cs, ce, end int, ok bool) {
		if cs, ce, end, ok = matchBareEnvelope(data, i, matchTextElement); ok {
			return
		}
		return matchBareEnvelope(data, i, matchPtabElement)
	}
	out := make([]byte, 0, len(data))
	pos := 0
	for pos < len(data) {
		ti := bytes.Index(data[pos:], []byte("<w:r><w:t"))
		pi := bytes.Index(data[pos:], []byte("<w:r><w:ptab"))
		rel := ti
		if rel < 0 || (pi >= 0 && pi < rel) {
			rel = pi
		}
		if rel < 0 {
			out = append(out, data[pos:]...)
			break
		}
		gStart := pos + rel
		cs, ce, end, ok := matchTextOrPtab(gStart)
		if !ok {
			out = append(out, data[pos:gStart+1]...)
			pos = gStart + 1
			continue
		}
		out = append(out, data[pos:gStart]...) // verbatim gap before group
		// Accumulate consecutive members.
		type span struct{ cs, ce int }
		members := []span{{cs, ce}}
		p := end
		for {
			ncs, nce, nend, nok := matchTextOrPtab(p)
			if !nok {
				break
			}
			members = append(members, span{ncs, nce})
			p = nend
		}
		if len(members) >= 2 {
			out = append(out, "<w:r>"...)
			for _, m := range members {
				out = append(out, data[m.cs:m.ce]...)
			}
			out = append(out, "</w:r>"...)
		} else {
			out = append(out, data[gStart:p]...) // single member, verbatim
		}
		pos = p
	}
	return out
}

// fuseAnnotationRefText collapses a CommentReference-rPr `<w:annotationRef/>`
// run followed by a same-rPr text run into one `<w:r>` carrying the rPr, the
// annotationRef and the text. Faithful single-pass port of the retired
// fuseSameRPrAnnotationRefAndTextRuns; see run_merge.go header and
// RunMerger.java:402-441.
func fuseAnnotationRefText(data []byte) []byte {
	if !bytes.Contains(data, []byte("<w:annotationRef/>")) {
		return data
	}
	// commentRefRPr matches `<w:rPr><w:rStyle w:val="CommentReference"
	// (?:/>|></w:rStyle>)</w:rPr>` at data[i:], returning the index just
	// past `</w:rPr>`, or -1.
	commentRefRPr := func(i int) int {
		const a = `<w:rPr><w:rStyle w:val="CommentReference"`
		if !bytes.HasPrefix(data[i:], []byte(a)) {
			return -1
		}
		j := i + len(a)
		const selfClose = `/></w:rPr>`
		const openClose = `></w:rStyle></w:rPr>`
		switch {
		case bytes.HasPrefix(data[j:], []byte(selfClose)):
			return j + len(selfClose)
		case bytes.HasPrefix(data[j:], []byte(openClose)):
			return j + len(openClose)
		default:
			return -1
		}
	}
	const marker = `<w:r><w:rPr><w:rStyle w:val="CommentReference"`
	out := make([]byte, 0, len(data))
	pos := 0
	for pos < len(data) {
		rel := bytes.Index(data[pos:], []byte(marker))
		if rel < 0 {
			out = append(out, data[pos:]...)
			break
		}
		mStart := pos + rel
		// Marker run: <w:r> + CommentReference rPr + <w:annotationRef/> + </w:r>.
		rprEnd := commentRefRPr(mStart + len("<w:r>"))
		if rprEnd < 0 {
			out = append(out, data[pos:mStart+1]...)
			pos = mStart + 1
			continue
		}
		const annot = "<w:annotationRef/></w:r>"
		if !bytes.HasPrefix(data[rprEnd:], []byte(annot)) {
			out = append(out, data[pos:mStart+1]...)
			pos = mStart + 1
			continue
		}
		markerEnd := rprEnd + len(annot)
		// Text run: <w:r> + CommentReference rPr + <w:t>…</w:t> + </w:r>.
		if !bytes.HasPrefix(data[markerEnd:], []byte("<w:r>")) {
			out = append(out, data[pos:mStart+1]...)
			pos = mStart + 1
			continue
		}
		tRPrEnd := commentRefRPr(markerEnd + len("<w:r>"))
		if tRPrEnd < 0 {
			out = append(out, data[pos:mStart+1]...)
			pos = mStart + 1
			continue
		}
		tEnd := matchTextElement(data, tRPrEnd)
		if tEnd < 0 || !bytes.HasPrefix(data[tEnd:], []byte("</w:r>")) {
			out = append(out, data[pos:mStart+1]...)
			pos = mStart + 1
			continue
		}
		// Splice: canonical self-closing rStyle + annotationRef + the text.
		out = append(out, data[pos:mStart]...)
		out = append(out, `<w:r><w:rPr><w:rStyle w:val="CommentReference"/></w:rPr><w:annotationRef/>`...)
		out = append(out, data[tRPrEnd:tEnd]...)
		out = append(out, "</w:r>"...)
		pos = tEnd + len("</w:r>")
	}
	return out
}

// fuseBarePictAndRPrTextRuns collapses an adjacent
// `<w:r><w:pict>…</w:pict></w:r><w:r><w:rPr>…</w:rPr><w:t …>…</w:t></w:r>`
// pair into a single `<w:r>` envelope carrying the second run's rPr,
// followed by the pict and t children: `<w:r><w:rPr>X</w:rPr>
// <w:pict>…</w:pict><w:t …>…</w:t></w:r>`.
//
// The first run must be a bare `<w:r><w:pict>` envelope (no rPr) and
// the second must be a `<w:r><w:rPr>X</w:rPr><w:t>…</w:t></w:r>`
// envelope. The fuse is rPr-equivalent on the first slot (empty rPr →
// inherited from pPr), and the post-fuse `<w:rPr>X</w:rPr>` is the
// rPr X from the second slot, minified (see below).
//
// Mirrors upstream Okapi RunMerger.mergeRunBodyChunks
// (RunMerger.java:402-441) fusing adjacent runs whose rPr's are
// "compatible" per canRunPropertiesBeMerged (RunMerger.java:156-229):
// an empty rPr is treated as compatible with any other rPr, and the
// merged run carries the non-empty rPr. The result is a single
// `<w:r>` carrying both a Markup body chunk (the pict) and a RunText
// body chunk (the t) under one shared RunProperties — per ECMA-376-1
// §17.3.2.1 (CT_R) a single `<w:r>` may carry both `<w:pict>` and
// `<w:t>` children alongside one `<w:rPr>`.
//
// In addition to the structural fuse, this strips the `w:hAnsi="…"`
// attribute from the fused rPr's `<w:rFonts>` when the rFonts element
// carries `w:ascii` with the SAME value. This mirrors upstream Okapi's
// RunProperties.minified() collapse of redundant rFonts attributes
// against the pPr.rPr-inherited rFonts context: when the paragraph
// mark's rFonts already declares hAnsi at the same value, the run's
// hAnsi is redundant and is dropped, leaving only the ascii attribute
// on the run-level rFonts (which Okapi retains as the "primary" Latin
// font marker). Fixture: Hangs.docx document.xml, where a bare pict run
// wrapping the s1059 shape is followed by a text run carrying
// `<w:rFonts w:ascii="Times New Roman" w:hAnsi="Times New Roman"/>` and
// the reference output emits the fused run with
// `<w:rFonts w:ascii="Times New Roman"/>` only.
//
// The walker (not a regex — pict bodies hold heavy textbox/imagedata/
// OLEObject markup whose non-greedy regex walk over megabytes of XML
// would be slow) pairs each `<w:r><w:pict>` open with the NEAREST
// `</w:pict></w:r>`; nested `<w:pict>` does not occur in the OOXML
// fixture corpus (VML pict bodies hold shape/textbox children but never
// another `<w:pict>` per ECMA-376-1 §17.3.3.9 / §M.6.2).
//
// The leading-bare-r gate (`<w:r><w:pict>`, no rPr) distinguishes pict
// runs that already share rPr with neighbors (where emitRunEnvelopes'
// same-source join handles the merge) from the SKELETON-EMITTED case
// where the source's rPr-less pict run is re-emitted verbatim through
// the runToXML path. Hangs.docx exercises the skeleton path: the
// surrounding paragraph is non-translatable apart from a short text
// run, so the runs flow through skeleton reconstruction and arrive here
// as the un-fused envelope pair. Applied pre-WSO (postNonWSOForName) so
// WordStyleOptimisation sees the same fused run upstream's RunMerger
// produced before its own WSO pass — see the run_merge.go header.
func fuseBarePictAndRPrTextRuns(data []byte) []byte {
	if !bytes.Contains(data, []byte(`<w:r><w:pict>`)) {
		return data
	}
	out := make([]byte, 0, len(data))
	const openSeq = "<w:r><w:pict>"
	const closeSeq = "</w:pict></w:r>"
	const rprOpen = "<w:r><w:rPr>"
	const rprClose = "</w:rPr>"
	const tOpen = "<w:t"
	const tClose = "</w:t></w:r>"
	pos := 0
	for pos < len(data) {
		idx := bytes.Index(data[pos:], []byte(openSeq))
		if idx < 0 {
			out = append(out, data[pos:]...)
			break
		}
		// Copy bytes up to the open of `<w:r>`.
		openAt := pos + idx
		out = append(out, data[pos:openAt]...)
		// Locate matching `</w:pict></w:r>` (first occurrence — no
		// nested pict in OOXML).
		bodyStart := openAt + len(openSeq)
		closeRel := bytes.Index(data[bodyStart:], []byte(closeSeq))
		if closeRel < 0 {
			// Malformed — emit verbatim and bail.
			out = append(out, data[openAt:]...)
			break
		}
		closeAt := bodyStart + closeRel
		afterPict := closeAt + len(closeSeq)
		// Check if the immediately-following bytes are
		// `<w:r><w:rPr>…</w:rPr><w:t …>…</w:t></w:r>`.
		if !bytes.HasPrefix(data[afterPict:], []byte(rprOpen)) {
			out = append(out, data[openAt:afterPict]...)
			pos = afterPict
			continue
		}
		rPrBodyStart := afterPict + len(rprOpen)
		rPrCloseRel := bytes.Index(data[rPrBodyStart:], []byte(rprClose))
		if rPrCloseRel < 0 {
			out = append(out, data[openAt:afterPict]...)
			pos = afterPict
			continue
		}
		rPrCloseAt := rPrBodyStart + rPrCloseRel
		afterRPr := rPrCloseAt + len(rprClose)
		if !bytes.HasPrefix(data[afterRPr:], []byte(tOpen)) {
			out = append(out, data[openAt:afterPict]...)
			pos = afterPict
			continue
		}
		tCloseRel := bytes.Index(data[afterRPr:], []byte(tClose))
		if tCloseRel < 0 {
			out = append(out, data[openAt:afterPict]...)
			pos = afterPict
			continue
		}
		tEndAt := afterRPr + tCloseRel + len(tClose)
		// Reject if the t body contains nested `<w:t>` (defensive —
		// the walker assumes a single text leaf).
		tBody := data[afterRPr:tEndAt]
		if bytes.Count(tBody, []byte("<w:t>")) > 0 || bytes.Count(tBody, []byte("<w:t ")) > 1 {
			out = append(out, data[openAt:afterPict]...)
			pos = afterPict
			continue
		}
		// Only fuse when the second run's rPr is EXACTLY a single
		// <w:rFonts ... /> element carrying the ascii+hAnsi attribute
		// pair with the SAME value. Mirrors upstream Okapi's
		// canRunPropertiesBeMerged + RunFonts.canBeMerged
		// (RunMerger.java:156-229 + RunFonts.java:190-247): a bare
		// (rPr-less) pict run merges with a following text run ONLY
		// when the text-run rPr is reducible to a rFonts-only
		// signature whose attributes are the merge-compatible
		// ascii=hAnsi pair. Text runs carrying additional rPr
		// children (sz, color, b, …) fail the merge — bridge keeps
		// them split. Fixture: Hangs.docx document.xml s1059 (rPr =
		// rFonts ascii+hAnsi only → fuse) vs s1062 (rPr = rFonts +
		// sz → keep split).
		rPrInner := data[rPrBodyStart:rPrCloseAt]
		if !isMergeableBareRPrRFonts(rPrInner) {
			out = append(out, data[openAt:afterPict]...)
			pos = afterPict
			continue
		}
		// Build the fused envelope. The rPr body is taken from the
		// second run, then minified by stripping `w:hAnsi="X"` when
		// `w:ascii="X"` is present with the same value (see function
		// comment for the rationale).
		rPrBody := minifyRunRFontsHAnsi(rPrInner)
		pictBody := data[bodyStart:closeAt]
		out = append(out, []byte("<w:r><w:rPr>")...)
		out = append(out, rPrBody...)
		out = append(out, []byte("</w:rPr><w:pict>")...)
		out = append(out, pictBody...)
		out = append(out, []byte("</w:pict>")...)
		// Emit the t element verbatim (between afterRPr and tEndAt
		// minus the closing `</w:r>` so we can wrap it in our own).
		// tClose is `</w:t></w:r>`; we need to keep `</w:t>` but drop
		// the `</w:r>` since we append our own.
		tBodyEnd := tEndAt - len("</w:r>")
		out = append(out, data[afterRPr:tBodyEnd]...)
		out = append(out, []byte("</w:r>")...)
		pos = tEndAt
	}
	return out
}

// isMergeableBareRPrRFonts reports whether an rPr inner body
// (the bytes BETWEEN `<w:rPr>` and `</w:rPr>`) consists of EXACTLY one
// `<w:rFonts …/>` element carrying `w:ascii="X"` and `w:hAnsi="X"` for
// the SAME value X, with no other attributes and no sibling rPr
// children. Used by fuseBarePictAndRPrTextRuns to gate the fuse — see
// the call site for the upstream Okapi canRunPropertiesBeMerged
// rationale.
func isMergeableBareRPrRFonts(rPrInner []byte) bool {
	trimmed := bytes.TrimSpace(rPrInner)
	if !bytes.HasPrefix(trimmed, []byte("<w:rFonts")) {
		return false
	}
	// Accept both self-closing and open/close empty forms.
	var tag []byte
	switch {
	case bytes.HasSuffix(trimmed, []byte("/>")):
		tag = trimmed
	case bytes.HasSuffix(trimmed, []byte("</w:rFonts>")):
		// Open + close form — extract the open tag and verify the
		// body is empty.
		openEnd := bytes.IndexByte(trimmed, '>')
		if openEnd < 0 {
			return false
		}
		body := trimmed[openEnd+1 : len(trimmed)-len("</w:rFonts>")]
		if len(bytes.TrimSpace(body)) != 0 {
			return false
		}
		tag = trimmed[:openEnd+1]
	default:
		return false
	}
	asciiMatch := rFontsASCIIRE.FindSubmatch(tag)
	hAnsiMatch := rFontsHAnsiRE.FindSubmatch(tag)
	if asciiMatch == nil || hAnsiMatch == nil {
		return false
	}
	if !bytes.Equal(asciiMatch[1], hAnsiMatch[1]) {
		return false
	}
	// Reject if there are extra attributes beyond ascii+hAnsi (e.g.
	// hint="eastAsia", cs, eastAsia, etc.). Counting attributes:
	// the rFonts element must have exactly 2 attributes.
	return len(rFontsAttrRE.FindAll(tag, -1)) == 2
}

// minifyRunRFontsHAnsi strips a redundant `w:hAnsi="X"` attribute from
// the FIRST `<w:rFonts …/>` element inside an rPr body when the same
// element also carries `w:ascii="X"` with the same value. Mirrors
// upstream Okapi RunProperties.minified() collapse of inherited
// rFonts attributes — see fuseBarePictAndRPrTextRuns for the citation
// and Hangs.docx fixture rationale.
//
// Conservative match: only fires when the rFonts element carries
// EXACTLY `w:ascii="X"` and `w:hAnsi="X"` for the SAME value X, with
// no other attribute between them or that would make the merge
// unsafe. Returns the input unchanged when the pattern doesn't match.
func minifyRunRFontsHAnsi(rPrBody []byte) []byte {
	rFontsStart := bytes.Index(rPrBody, []byte("<w:rFonts"))
	if rFontsStart < 0 {
		return rPrBody
	}
	// Locate the rFonts element end (self-closing `/>` or open-tag
	// `>`).
	tagEnd := bytes.IndexAny(rPrBody[rFontsStart:], ">")
	if tagEnd < 0 {
		return rPrBody
	}
	tagEnd += rFontsStart
	tag := rPrBody[rFontsStart : tagEnd+1]
	// Match `w:ascii="X"` and `w:hAnsi="X"` and drop the hAnsi.
	asciiMatch := rFontsASCIIRE.FindSubmatch(tag)
	if asciiMatch == nil {
		return rPrBody
	}
	asciiVal := asciiMatch[1]
	hAnsiPattern := fmt.Appendf(nil, ` w:hAnsi="%s"`, asciiVal)
	if !bytes.Contains(tag, hAnsiPattern) {
		return rPrBody
	}
	// Strip the hAnsi attribute (including the preceding space).
	newTag := bytes.Replace(tag, hAnsiPattern, nil, 1)
	out := make([]byte, 0, len(rPrBody))
	out = append(out, rPrBody[:rFontsStart]...)
	out = append(out, newTag...)
	out = append(out, rPrBody[tagEnd+1:]...)
	return out
}

// rFonts attribute matchers used by the bare-pict + text run fuse to
// gate (isMergeableBareRPrRFonts) and minify (minifyRunRFontsHAnsi) the
// fused run's `<w:rFonts>` per upstream Okapi RunProperties.minified() +
// RunFonts.canBeMerged. Compiled once at package init rather than per
// call (the originals re-MustCompile'd on every fuse).
var (
	rFontsASCIIRE = regexp.MustCompile(`w:ascii="([^"]+)"`)
	rFontsHAnsiRE = regexp.MustCompile(`w:hAnsi="([^"]+)"`)
	rFontsAttrRE  = regexp.MustCompile(`\bw:[A-Za-z]+="`)
)
