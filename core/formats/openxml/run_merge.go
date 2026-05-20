package openxml

import "bytes"

// run_merge.go — production cross-source run-envelope fusion (#602/#603).
//
// The WML skeleton write path emits one <w:r> per source run (runToXML /
// emitRunEnvelopes). Okapi's RunMerger additionally fuses ADJACENT source
// runs whose RunProperties are mergeable. emitRunEnvelopes covers the
// same-source split and the br→text cross-source case structurally; the
// three remaining cross-source fusions — handled here — arise in the
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
