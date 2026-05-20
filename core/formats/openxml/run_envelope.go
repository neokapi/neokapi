package openxml

import (
	"strings"
)

// run_envelope.go — native <w:r> envelope emission (#602).
//
// neokapi's textRun is finer-grained than OOXML's <w:r>: a single source
// <w:r> that carries several children under one <w:rPr> (e.g.
// <w:r><w:rPr/><w:tab/><w:t>x</w:t></w:r>) is parsed into multiple
// textRuns. The reader records the envelope boundary on each textRun via
// srcRunStart (true on the FIRST child of a fresh source <w:r>, false on
// later children of the same source <w:r>).
//
// Historically the skeleton write path emitted one <w:r> per textRun
// (runToXML) and then re-stitched same-envelope children with a battery
// of post-serialization fuse regexes in writer.go. emitRunEnvelopes makes
// the <w:r> envelope a first-class emission unit: it groups consecutive
// "simple" run-child textRuns that share a source <w:r> (srcRunStart ==
// false continues the open envelope) and emits one <w:r> with a single
// shared <w:rPr> around all of them.
//
// It is deliberately conservative. Children are not re-serialized here;
// each child's XML is derived by stripping the "<w:r>" + rPr prefix and
// "</w:r>" suffix off runToXML(r), so a single-run envelope is byte-for-
// byte identical to runToXML and the grouped form is exactly the
// concatenation of those children under the first run's shared rPr.
// Anything emitRunEnvelopes does not recognize as a groupable simple
// run-child — paragraph-level sentinels (math, bookmarks, comment-range,
// complex fields, paired-code wrappers) and any other PUA sentinel — is
// emitted standalone via runToXML and forces an envelope boundary, so its
// output is unchanged. emitRunEnvelopes can therefore only ever FUSE
// same-source simple children; it never alters how an unrecognized run
// serializes.

// Sentinel code points used by the WML reader, named here so this file
// can reference them without embedding raw Private-Use-Area bytes. The
// \uE1xx escapes compile to the same bytes the reader writes into
// textRun.text (see wml.go: tab , image , note-ref ,
// paragraph-opaque , paired-code /).
const (
	sentinelTab         = "\uE100"  // <w:tab/>
	sentinelImage       = "\uE101"  // <w:drawing>/<w:pict>/<w:object>/run-level mc:AlternateContent
	sentinelNoteRefPfx  = "\uE102:" // footnote/endnote reference (":f:id" / ":e:id")
	sentinelParaOpaque  = "\uE105"  // math / paragraph-level mc:AlternateContent (direct <w:p> child)
	sentinelPairedOpen  = "\uE10E:" // generic paired-code OPEN wrapper (<w:ins>/<w:moveTo>/<w:sdt>)
	sentinelPairedClose = "\uE10F:" // generic paired-code CLOSE wrapper
)

// runIsParagraphLevel reports whether a textRun serializes as a direct
// <w:p> child (no <w:r> wrapper): math / paragraph-level AlternateContent,
// bookmarks, comment-range markers, complex-field markup, and the generic
// paired-code wrappers. Mirrors the early-return cases at the top of
// runToXML.
func runIsParagraphLevel(r textRun) bool {
	return strings.HasPrefix(r.text, sentinelParaOpaque) ||
		isBookmarkSentinel(r.text) ||
		isCommentRangeSentinel(r.text) ||
		isFieldSentinel(r.text) ||
		strings.HasPrefix(r.text, sentinelPairedOpen) ||
		strings.HasPrefix(r.text, sentinelPairedClose)
}

// simpleRunChild reports whether r is a run-child textRun that runToXML
// wraps in "<w:r>" + rPr + CHILD + "</w:r>" and that is safe to fuse into
// a shared envelope: plain text, tab, line break, drawing/pict/object
// payload, or footnote/endnote reference. Paragraph-level sentinels and
// any other sentinel return false (emitted standalone, forcing a
// boundary).
func simpleRunChild(r textRun) bool {
	if runIsParagraphLevel(r) {
		return false
	}
	switch {
	case r.text == sentinelTab:
		return true
	case r.text == "\n":
		return true
	case strings.HasPrefix(r.text, sentinelNoteRefPfx):
		return true
	case isSentinel(r.text):
		// Drawing/pict/object (sentinelImage), hyperlink, raw-XML, and any
		// other sentinel are NOT simple: drawings need writeDrawingXMLToSkel
		// for docPr/@name attribute extraction, and the others carry opaque
		// payloads. Non-simple runs are emitted standalone via runToXML and
		// force an envelope boundary.
		return false
	default:
		return true // plain text run
	}
}

// runChildXMLViaStrip returns the <w:r>-child XML of a simple run-child by
// stripping the "<w:r>" + rPr prefix and "</w:r>" suffix off runToXML(r).
// This guarantees byte-identity with runToXML's own child serialization —
// no separate child emitter to drift out of sync. Callers must only pass
// runs for which simpleRunChild(r) is true.
func runChildXMLViaStrip(r textRun) string {
	full := runToXML(r)
	prefix := "<w:r>" + serializeFullRPrXML(r.props)
	full = strings.TrimPrefix(full, prefix)
	full = strings.TrimSuffix(full, "</w:r>")
	return full
}

// emitRunEnvelopes serializes a post-mergeRuns []textRun into <w:r>
// envelopes, fusing consecutive simple run-children that originated in the
// same source <w:r> (srcRunStart == false continues the open envelope)
// under the first run's shared <w:rPr>. Non-simple runs are emitted
// standalone via runToXML and force a boundary. Replaces the per-textRun
// skeleton emit + post-serialization fuse regexes for the same-source-
// split case (#602).
func emitRunEnvelopes(runs []textRun) string {
	var buf strings.Builder
	i := 0
	for i < len(runs) {
		if !simpleRunChild(runs[i]) {
			buf.WriteString(runToXML(runs[i]))
			i++
			continue
		}
		buf.WriteString("<w:r>")
		buf.WriteString(serializeFullRPrXML(runs[i].props))
		buf.WriteString(runChildXMLViaStrip(runs[i]))
		envProps := runs[i].props
		lastWasBreak := runs[i].text == "\n"
		j := i + 1
		for j < len(runs) && simpleRunChild(runs[j]) {
			cont := !runs[j].srcRunStart
			// Cross-source RunMerger fusion: a plain text run that
			// begins a fresh source <w:r> still fuses INTO the open
			// envelope when the envelope's most recent child was a
			// <w:br/> and both runs carry no <w:rPr>. Mirrors Okapi
			// RunMerger fusing `<w:r><w:br/></w:r><w:r><w:t>…</w:t></w:r>`
			// → `<w:r><w:br/><w:t>…</w:t></w:r>` (apissue.docx). The
			// reverse adjacency `[text][br]` is intentionally NOT fused
			// — a <w:br/> that opens a new source <w:r> keeps its own
			// envelope (textRun.srcRunStart / 1421-line-break.docx).
			if !cont && lastWasBreak && isPlainTextRun(runs[j]) &&
				envProps.isEmpty() && runs[j].props.isEmpty() {
				cont = true
			}
			if !cont {
				break
			}
			buf.WriteString(runChildXMLViaStrip(runs[j]))
			lastWasBreak = runs[j].text == "\n"
			j++
		}
		buf.WriteString("</w:r>")
		i = j
	}
	return buf.String()
}

// isPlainTextRun reports whether r is an ordinary <w:t> text run (not a
// sentinel, not a line break). Used to gate cross-source break→text
// fusion in emitRunEnvelopes.
func isPlainTextRun(r textRun) bool {
	return !isSentinel(r.text) && r.text != "\n"
}
