package openxml

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
)

// wmlLangElementRE matches the WordprocessingML <w:lang> element in
// both self-closing (`<w:lang .../>`) and open/close (`<w:lang ...>
// </w:lang>`) forms. Source documents almost always self-close this
// element, but the encoding/xml-driven reader/skeleton path can
// re-emit it in open/close form, in which case the self-closing-only
// regex would silently fail to strip it.
//
// <w:lang> is stripped by upstream Okapi's RunSkippableElements
// (lines 50-62 of RunSkippableElements.java) keyed on the
// TRANSITIONAL WPML namespace QName — see stripWMLSkippableElements
// for the Strict-OOXML gate that mirrors upstream's QName semantics.
var wmlLangElementRE = regexp.MustCompile(
	`<w:lang\b[^>]*/>` +
		`|<w:lang\b[^>]*></w:lang>`,
)

// wmlBidiVisualElementRE matches the WordprocessingML <w:bidiVisual>
// paragraph-property element (RTL visual hint, ECMA-376-1 §17.3.1.7)
// in both forms. Stripped unconditionally by upstream Okapi's
// BlockPropertiesFactory via SkippableElements.Default
// (BLOCK_PROPERTY_BIDI_VISUAL).
var wmlBidiVisualElementRE = regexp.MustCompile(
	`<w:bidiVisual\b[^>]*/>` +
		`|<w:bidiVisual\b[^>]*></w:bidiVisual>`,
)

// wmlMoveRangeStrippableElementRE matches the cross-structure
// revision-tracking range markers that okapi unconditionally drops
// during round-trip when bPreferenceAutomaticallyAcceptRevisions=true
// (the default). Each marker is overwhelmingly emitted in self-closing
// form (`<w:moveToRangeStart .../>`) but the schema permits an empty
// open/close pair; both forms are matched. Element list:
//   - <w:moveToRangeStart>   / <w:moveToRangeEnd>
//   - <w:moveFromRangeStart> / <w:moveFromRangeEnd>
//
// See SkippableElement.RevisionCrossStructure (lines 143-173 of
// okapi/filters/openxml/src/main/java/net/sf/okapi/filters/openxml/SkippableElement.java)
// and the wiring in SkippableElements.RevisionCrossStructure /
// MoveFromRevisionCrossStructure (lines 336-410 of
// okapi/filters/openxml/src/main/java/net/sf/okapi/filters/openxml/SkippableElements.java),
// BlockSkippableElements (lines 64-78 of BlockSkippableElements.java),
// and StyledTextPart (lines 212-225 of StyledTextPart.java).
var wmlMoveRangeStrippableElementRE = regexp.MustCompile(
	`<w:(?:moveToRangeStart|moveToRangeEnd|moveFromRangeStart|moveFromRangeEnd)\b[^>]*/>` +
		`|<w:(?:moveToRangeStart|moveToRangeEnd|moveFromRangeStart|moveFromRangeEnd)\b[^>]*>\s*</w:(?:moveToRangeStart|moveToRangeEnd|moveFromRangeStart|moveFromRangeEnd)>`,
)

// wmlEmptyPropertiesContainerRE matches WordprocessingML run-property and
// paragraph-property containers that have no attributes and no element
// children. Okapi's RunProperties.Default.getEvents (line 580 of
// okapi/filters/openxml/src/main/java/net/sf/okapi/filters/openxml/RunProperties.java)
// returns Collections.emptyList() when properties().isEmpty(), and
// BlockProperties.Default.getEvents (line 169-180 of BlockProperties.java)
// returns Collections.emptyList() when isEmpty() (no attributes AND no
// properties; isEmpty at line 203-205). The element is therefore omitted
// entirely from the round-trip output. Both <w:rPr> and <w:pPr> are
// container-only in the WML schema and never carry attributes in
// okapi-testdata fixtures, so the empty-with-attributes case need not
// be considered. Stripping must be iterated because removing an empty
// <w:rPr/> can leave its parent <w:pPr/> empty and itself eligible.
//
// The body matcher [\s]* tolerates any whitespace between the open and
// close tags — encoding/xml may emit indented or newline-padded XML
// which would otherwise survive the strip and force the empty container
// into the output (and into the canonical comparison, where it diverges
// against the okapi reference's omitted-entirely encoding).
var wmlEmptyPropertiesContainerRE = regexp.MustCompile(
	`<w:rPr>\s*</w:rPr>` +
		`|<w:rPr\s*/>` +
		`|<w:pPr>\s*</w:pPr>` +
		`|<w:pPr\s*/>`,
)

// wmlNoProofRE matches the run-property <w:noProof> element (no
// spelling/grammar marker) in both self-closing and open/close form.
// Okapi strips this from rPr in run/style contexts via
// RunSkippableElements (line 55 of okapi/filters/openxml/src/main/java/
// net/sf/okapi/filters/openxml/RunSkippableElements.java) and the
// RunProperties parser. The element is container-free in practice
// (carries no attributes either; the schema permits w:val but no fixture
// corpus uses it), so a simple element-only regex is sufficient.
var wmlNoProofRE = regexp.MustCompile(
	`<w:noProof\b[^>]*/>` +
		`|<w:noProof\b[^>]*></w:noProof>`,
)

// bareBrThenBareTextRunRE matches a bare `<w:r><w:br [attrs]/></w:r>`
// envelope IMMEDIATELY followed by a bare
// `<w:r><w:t [attrs]>content</w:t></w:r>` envelope. Both envelopes
// must lack `<w:rPr>` so the fuse is rPr-equivalent. Captures:
//
//	$1 = the br element verbatim (including any w:type / w:clear attrs)
//	$2 = the t element verbatim (open tag + body + close tag)
//
// The replacement collapses the pair into a single `<w:r>` carrying
// both children. Used by fuseBareBrAndTextRuns; see the call site
// for the upstream Okapi RunMerger citation and apissue.docx fixture
// rationale.
var bareBrThenBareTextRunRE = regexp.MustCompile(
	`<w:r>(<w:br\b[^/>]*/>)</w:r><w:r>(<w:t\b[^>]*>[^<]*</w:t>)</w:r>`)

// fuseBareBrAndTextRuns collapses adjacent bare `<w:r><w:br/></w:r>`
// + `<w:r><w:t>…</w:t></w:r>` envelopes into one `<w:r>` envelope
// carrying both children. Both envelopes must lack `<w:rPr>` so the
// fuse is rPr-equivalent. See bareBrThenBareTextRunRE for the regex
// shape and the postNonWSOForName call site for the upstream Okapi
// RunMerger citation.
//
// Per ECMA-376-1 §17.3.2.1 (CT_R) a single `<w:r>` may carry both
// `<w:br>` and `<w:t>` children alongside one shared `<w:rPr>`
// (here: empty / absent). Per §17.3.3.1 (CT_Br) the break element's
// w:type/w:clear attrs are independent of the run's text bytes, so
// preserving the captured br tag verbatim survives the round-trip.
//
// apissue.docx is the canonical fixture: page-break + space text
// run pairs in non-translatable paragraphs reach this writer via
// the wml.go skeleton path's per-run runToXML emit, which always
// wraps each source `<w:r>` in its own envelope. Bridge's
// RunMerger fuses these on the way out; this post-pass mirrors
// that for parity.
func fuseBareBrAndTextRuns(data []byte) []byte {
	if !bytes.Contains(data, []byte(`<w:r><w:br`)) {
		return data
	}
	return bareBrThenBareTextRunRE.ReplaceAll(data, []byte(`<w:r>$1$2</w:r>`))
}

// altContentRunThenBareBrRunRE matches an `<mc:AlternateContent>` host
// run's closing `</mc:AlternateContent></w:r>` IMMEDIATELY followed by a
// bare `<w:r><w:br[...]/>...` run envelope (with optional `<w:t>` body
// from a prior fuseBareBrAndTextRuns fusion). The following run must
// lack `<w:rPr>` so the boundary is rPr-equivalent — when the source's
// post-image run carries a non-empty rPr the structural per-run skeleton
// emit preserves the boundary naturally. Captures:
//
//	$1 = the trailing run envelope verbatim (preserved unchanged), starting
//	     at `<w:r>` and continuing through `<w:br[...]/>` (with optional
//	     `<w:t>` body).
//
// The replacement keeps the trailing run unchanged and inserts a fresh
// empty `<w:r></w:r>` placeholder between the AlternateContent host run
// and the trailing br-bearing run. Used by
// emitEmptyRunAfterAltContentPostImageBoundary; see the call site for
// the upstream-Okapi citation and the graphicdata.docx fixture
// rationale.
var altContentRunThenBareBrRunRE = regexp.MustCompile(
	`</mc:AlternateContent></w:r>(<w:r><w:br\b[^/>]*/>)`)

// emitEmptyRunAfterAltContentPostImageBoundary inserts an empty
// `<w:r></w:r>` placeholder run between an `<mc:AlternateContent>`
// host run's closing `</mc:AlternateContent></w:r>` and the
// IMMEDIATELY-following bare `<w:r><w:br[...]/>...` run envelope.
//
// Rationale: upstream Okapi's RunBuilder.flushRunStart/flushRunEnd
// cycle (RunBuilder.java) emits an empty placeholder `<w:r></w:r>`
// envelope on the boundary between a complex image run (a `<w:drawing>`
// / `<w:pict>` / `<w:mc:AlternateContent>` envelope) and the next
// `<w:r>` envelope whose body begins with a `<w:br/>` markup chunk.
// The placeholder is the visible artefact of the post-image flush
// cycle in BlockParser.parse — the same pattern that produces the
// cross-paragraph `fieldStraddle` artefact, but scoped to post-image
// boundaries. Per ECMA-376-1 §11.3 (CT_AlternateContent) the
// AlternateContent element is a Markup compatibility wrapper that
// carries no rendering effect; per §17.3.2.1 (CT_R) an empty `<w:r/>`
// run is well-formed.
//
// Native's writer emits the AlternateContent host run + the bare
// br-bearing run as adjacent siblings without the boundary placeholder,
// so a byte-level diff at the post-image boundary shows up as a missing
// `<w:r></w:r>` envelope. This post-pass closes that gap.
//
// The br-bearing run must lack `<w:rPr>` so the boundary is rPr-
// equivalent — when the source's post-image run carries a non-empty
// rPr the structural per-run skeleton emit preserves the boundary
// naturally and Okapi's flush cycle does not synthesise a separate
// placeholder.
//
// graphicdata.docx is the canonical fixture: a textbox-shape +
// AlternateContent host run is immediately followed by a bare-rPr
// `<w:br/>` run (post-rPr-strip) and then a bare-rPr text run; the
// br + text runs fuse via fuseBareBrAndTextRuns leaving the post-image
// boundary as a single `</mc:AlternateContent></w:r><w:r><w:br/>...`
// junction that upstream Okapi punctuates with the empty placeholder.
//
// The br match accepts both self-closing (`<w:br ...>`) and bare
// (`<w:br/>`) shapes — encoding/xml may re-emit either form depending
// on payload provenance. The trailing capture `$1` is preserved
// verbatim, which means the inserted placeholder lands BEFORE any
// `<w:t>` body the br-bearing run may carry post-fuse.
func emitEmptyRunAfterAltContentPostImageBoundary(data []byte) []byte {
	if !bytes.Contains(data, []byte(`</mc:AlternateContent></w:r><w:r><w:br`)) {
		return data
	}
	return altContentRunThenBareBrRunRE.ReplaceAll(
		data, []byte(`</mc:AlternateContent></w:r><w:r></w:r>$1`))
}

// bareFldCharEndAfterTextThenBareTextRunRE matches a bare
// `<w:r><w:t ...>...</w:t></w:r>` envelope (display text from an
// extractable complex field) IMMEDIATELY followed by a bare
// `<w:r><w:fldChar w:fldCharType="end"/></w:r>` envelope and then a
// bare `<w:r><w:t [attrs]>content</w:t></w:r>` envelope. All three
// envelopes must lack `<w:rPr>` so the fuse is rPr-equivalent.
// Captures:
//
//	$1 = the preceding text run verbatim (preserved unchanged)
//	$2 = the fldChar-end element verbatim (self-closing or open/close form)
//	$3 = the trailing t element verbatim (open tag + body + close tag)
//
// The replacement keeps the preceding text run as-is and collapses the
// fldChar-end + trailing text pair into a single `<w:r>` carrying both
// children. The leading-text gate distinguishes EXTRACTABLE fields
// (whose display text upstream emits as RunText body chunks subject to
// RunMerger fusion) from NON-EXTRACTABLE fields like XE index markers
// (whose preceding sibling is `<w:instrText>` rather than `<w:t>`, and
// whose fldChar-end + trailing text upstream keeps split per
// canMergeWith's containsComplexFields gate at RunMerger.java:147-149).
// Used by fuseBareFldCharEndAndTextRuns; see the call site for the
// upstream Okapi RunMerger citation and the 830-4.docx vs docxtest.docx
// fixture pair rationale.
//
// The fldChar match accepts both self-closing
// (`<w:fldChar .../>`) and open/close (`<w:fldChar ...></w:fldChar>`)
// shapes — encoding/xml may re-emit either form depending on payload
// provenance.
var bareFldCharEndAfterTextThenBareTextRunRE = regexp.MustCompile(
	`(<w:r><w:t\b[^>]*>[^<]*</w:t></w:r>)<w:r>(<w:fldChar\b[^>]*\bw:fldCharType="end"[^>]*(?:/>|></w:fldChar>))</w:r><w:r>(<w:t\b[^>]*>[^<]*</w:t>)</w:r>`)

// fuseBareFldCharEndAndTextRuns collapses adjacent bare
// `<w:r><w:fldChar fldCharType="end"/></w:r>` + `<w:r><w:t>…</w:t></w:r>`
// envelopes into one `<w:r>` envelope carrying both children, but ONLY
// when the fldChar-end run is itself preceded by another bare
// `<w:r><w:t>…</w:t></w:r>` envelope (the field's extracted display
// text). All three envelopes must lack `<w:rPr>` so the fuse is
// rPr-equivalent — when any side carries an `<w:rPr>` the structural
// fldChar-end + text merge path inside writeWMLBlock (see the
// inFieldEndRun branch around the `case r.Text != nil:` arm) handles
// the join with full effective-rPr comparison.
//
// Mirrors upstream Okapi RunMerger.mergeRunBodyChunks
// (RunMerger.java:402-441) fusing a Markup chunk (the fldChar-end)
// followed by a RunText chunk (the trailing text) when the containing
// source runs share rPr per canRunPropertiesBeMerged
// (RunMerger.java:156-229). Per ECMA-376-1 §17.3.2.1 (CT_R) a single
// `<w:r>` may carry both `<w:fldChar>` and `<w:t>` children, and per
// §17.16.5 (Complex Fields) the fldChar elements bookend a single
// semantic run regardless of intervening syntactic-run boundaries.
//
// The leading-text gate exists because non-extractable complex fields
// (e.g. XE index markers) flow through parseComplexField's non-
// extractable branch (RunParser.java:501-507) which adds the entire
// field — including the fldChar-end — to a single RunBuilder's markup
// chunk; the trailing text is a SEPARATE RunBuilder whose merge with
// the field run is blocked by canMergeWith's containsComplexFields
// gate (RunMerger.java:147-149). For these the source emits an
// `<w:instrText>` run immediately before the fldChar-end run, so the
// regex's `<w:t>` precondition naturally filters them out. Fixture
// pair: 830-4.docx (extractable COMMENTS field — fuse) vs
// docxtest.docx (non-extractable XE field — keep split).
//
// 830-4.docx is the canonical fuse fixture: a COMMENTS field
// straddling multiple paragraphs ends with a bare-rPr fldChar-end run
// immediately followed by a bare-rPr "." text run; upstream merges
// them into one `<w:r>` while native's per-run skeleton emit preserves
// the source envelope split. The structural inFieldEndRun fast path
// doesn't fire here because the fldChar-end Ph payload arrives via
// the skeleton reconstruction (writeWMLBlock isn't called for this
// block since the surrounding paragraph is non-translatable apart
// from the field's stripped display text), so the fix is applied at
// the post-pass layer where the per-run emit has already produced the
// envelope triplet.
func fuseBareFldCharEndAndTextRuns(data []byte) []byte {
	if !bytes.Contains(data, []byte(`<w:r><w:fldChar`)) {
		return data
	}
	return bareFldCharEndAfterTextThenBareTextRunRE.ReplaceAll(
		data, []byte(`$1<w:r>$2$3</w:r>`))
}

// stripFldCharBeginRunRPrWhenInheritedFromFollowingRun elides the
// `<w:rPr>` from a `<w:r ...><w:rPr>X</w:rPr><w:fldChar
// w:fldCharType="begin"/></w:r>` envelope when the IMMEDIATELY-following
// run is an `<w:r ...><w:rPr>Y</w:rPr><w:instrText ...>` envelope whose
// rPr Y contains the rPr X verbatim as a prefix or interior substring.
//
// Rationale: upstream Okapi RunMerger composes a complex field as a
// single semantic run carrying the begin/instrText/separate/end markup
// chunks plus any display text under one shared RunProperties — see
// RunMerger.mergeRunBodyChunks (RunMerger.java:402-441) and the
// canRunPropertiesBeMerged gate (RunMerger.java:156-229). After the
// merge, RunProperties.minified() strips redundant inherited toggles
// from the per-chunk rPr slots so that only the SUPERSET rPr survives
// on the chunk that carries the most distinguishing properties — in
// practice the instrText carrier — while the fldChar-begin emerges
// with a bare-body `<w:r><w:fldChar/></w:r>` envelope.
//
// Per ECMA-376-1 §17.16.5 (Complex Fields) the fldChar elements bookend
// a single semantic field; per §17.3.2.1 (CT_R) a single `<w:r>` may
// carry both `<w:fldChar>` and `<w:instrText>` children with one shared
// rPr — but when the source splits the begin and instrText into two
// separate `<w:r>` envelopes (as Practice2.docx does), the rPr on the
// begin run is redundant when the instrText run already carries a
// rPr that's a SUPERSET of the begin's rPr.
//
// The superset test uses verbatim substring containment of the rPr
// inner text. This is a conservative match: it fires only when every
// child element of the begin's rPr (in document order) appears as a
// contiguous run inside the instrText's rPr children. False negatives
// (different child ordering) leave the rPr in place — safe.
//
// Practice2.docx footer2.xml / footer3.xml is the canonical fixture:
// the source `<w:r><w:rPr><w:b/></w:rPr><w:fldChar begin/></w:r>` is
// followed by `<w:r><w:rPr>{rFonts,b,sz,szCs}</w:rPr><w:instrText>
// PAGE</w:instrText></w:r>`. Bridge's RunMerger fuses the field and
// minified() drops the redundant <w:b/> from the begin slot; native's
// per-run skeleton emit preserves the source per-run rPr, leaving a
// diff that this post-pass closes.
// canonicalRPrChildren parses the inner content of `<w:rPr>` and
// returns each child element normalised to its self-closing form
// (`<w:NAME ATTRS/>`), preserving attribute order verbatim. This
// canonicalisation lets the strip's superset comparison ignore the
// self-closing vs paired difference that encoding/xml may introduce
// on re-emit, while preserving attribute distinctions (e.g. two
// `<w:rFonts>` children with different attrs remain distinct).
//
// Children that don't parse as `<w:NAME ...>` are returned verbatim
// — the caller's substring containment then degrades gracefully to
// no-match (safer than over-stripping).
//
// Per ECMA-376-1 §17.3.2.28 (CT_RPr) the rPr children are all
// individual property elements; this function preserves their
// document order but the caller's `rPrChildrenSubset` does an
// order-insensitive set containment check.
func canonicalRPrChildren(inner []byte) []string {
	if len(inner) == 0 {
		return nil
	}
	var out []string
	pos := 0
	for pos < len(inner) {
		// Skip whitespace.
		for pos < len(inner) && (inner[pos] == ' ' || inner[pos] == '\t' || inner[pos] == '\n' || inner[pos] == '\r') {
			pos++
		}
		if pos >= len(inner) {
			break
		}
		if inner[pos] != '<' {
			// Unexpected text content — bail.
			return nil
		}
		tagClose := bytes.IndexByte(inner[pos:], '>')
		if tagClose < 0 {
			return nil
		}
		tagCloseAbs := pos + tagClose
		tag := inner[pos : tagCloseAbs+1]
		if len(tag) >= 2 && tag[len(tag)-2] == '/' {
			// Self-closing form already.
			out = append(out, string(tag))
			pos = tagCloseAbs + 1
			continue
		}
		// Paired form: find matching `</w:NAME>`.
		// Extract the element name (between `<` and the first space or `>`).
		nameEnd := pos + 1
		for nameEnd < tagCloseAbs && inner[nameEnd] != ' ' && inner[nameEnd] != '\t' && inner[nameEnd] != '\n' && inner[nameEnd] != '\r' {
			nameEnd++
		}
		name := inner[pos+1 : nameEnd]
		closingTag := append([]byte("</"), name...)
		closingTag = append(closingTag, '>')
		closeIdx := bytes.Index(inner[tagCloseAbs+1:], closingTag)
		if closeIdx < 0 {
			return nil
		}
		bodyEnd := tagCloseAbs + 1 + closeIdx
		body := inner[tagCloseAbs+1 : bodyEnd]
		// Only treat empty-body paired form as self-closing equivalent.
		// Non-empty body (rare in rPr children) preserves the paired form.
		if len(bytes.TrimSpace(body)) == 0 {
			// Convert `<w:NAME ATTRS></w:NAME>` to `<w:NAME ATTRS/>`.
			normalised := make([]byte, 0, len(tag)+1)
			normalised = append(normalised, tag[:len(tag)-1]...)
			normalised = append(normalised, '/', '>')
			out = append(out, string(normalised))
		} else {
			out = append(out, string(inner[pos:bodyEnd+len(closingTag)]))
		}
		pos = bodyEnd + len(closingTag)
	}
	return out
}

// rPrChildrenSubset reports whether every canonical child of subset
// appears in superset. Order-insensitive set inclusion. Used by
// stripFldCharBeginRunRPrWhenInheritedFromFollowingRun to verify that
// the fldChar-begin run's rPr is fully covered by the following
// instrText run's rPr before stripping.
func rPrChildrenSubset(subset, superset []byte) bool {
	subC := canonicalRPrChildren(subset)
	if subC == nil {
		return false
	}
	superC := canonicalRPrChildren(superset)
	if superC == nil {
		return false
	}
	superSet := make(map[string]struct{}, len(superC))
	for _, c := range superC {
		superSet[c] = struct{}{}
	}
	for _, c := range subC {
		if _, ok := superSet[c]; !ok {
			return false
		}
	}
	return true
}

// countRPrChildren returns the number of distinguishable child
// elements inside the rPr inner content. Used to enforce strict
// superset (more children in superset than subset) so identical-rPr
// pairs leave the strip OFF.
func countRPrChildren(inner []byte) int {
	return len(canonicalRPrChildren(inner))
}

func stripFldCharBeginRunRPrWhenInheritedFromFollowingRun(data []byte) []byte {
	if !bytes.Contains(data, []byte(`fldCharType="begin"`)) {
		return data
	}
	if !bytes.Contains(data, []byte(`<w:instrText`)) {
		return data
	}
	out := make([]byte, 0, len(data))
	pos := 0
	for pos < len(data) {
		// Find next `<w:r` open tag.
		runOpenIdx := bytes.Index(data[pos:], []byte(`<w:r`))
		if runOpenIdx < 0 {
			out = append(out, data[pos:]...)
			break
		}
		runOpenAbs := pos + runOpenIdx
		// Require `<w:r>` or `<w:r ` (not `<w:rPr` or `<w:rStyle` etc).
		next := data[runOpenAbs+len("<w:r")]
		if next != '>' && next != ' ' && next != '\t' && next != '\n' && next != '\r' {
			out = append(out, data[pos:runOpenAbs+1]...)
			pos = runOpenAbs + 1
			continue
		}
		// Find end of run open tag.
		openClose := bytes.IndexByte(data[runOpenAbs:], '>')
		if openClose < 0 {
			out = append(out, data[pos:]...)
			break
		}
		runOpenEndAbs := runOpenAbs + openClose + 1
		// Expect `<w:rPr>` immediately following.
		rprPrefix := []byte(`<w:rPr>`)
		if !bytes.HasPrefix(data[runOpenEndAbs:], rprPrefix) {
			out = append(out, data[pos:runOpenEndAbs]...)
			pos = runOpenEndAbs
			continue
		}
		rprInnerStart := runOpenEndAbs + len(rprPrefix)
		// Find `</w:rPr>`.
		rprClose := bytes.Index(data[rprInnerStart:], []byte(`</w:rPr>`))
		if rprClose < 0 {
			out = append(out, data[pos:rprInnerStart]...)
			pos = rprInnerStart
			continue
		}
		rprInnerEnd := rprInnerStart + rprClose
		rprEndAbs := rprInnerEnd + len(`</w:rPr>`)
		// Expect `<w:fldChar` next, with type="begin", in either
		// self-closing or paired form, then `</w:r>`.
		fldOpen := []byte(`<w:fldChar`)
		if !bytes.HasPrefix(data[rprEndAbs:], fldOpen) {
			out = append(out, data[pos:rprEndAbs]...)
			pos = rprEndAbs
			continue
		}
		// Locate end of fldChar element. Look for `/>` or `></w:fldChar>`.
		fldStart := rprEndAbs
		fldTagClose := bytes.IndexByte(data[fldStart:], '>')
		if fldTagClose < 0 {
			out = append(out, data[pos:fldStart]...)
			pos = fldStart
			continue
		}
		fldTagCloseAbs := fldStart + fldTagClose
		// Check attrs include w:fldCharType="begin".
		fldOpenTag := data[fldStart : fldTagCloseAbs+1]
		if !bytes.Contains(fldOpenTag, []byte(`w:fldCharType="begin"`)) {
			out = append(out, data[pos:fldTagCloseAbs+1]...)
			pos = fldTagCloseAbs + 1
			continue
		}
		// Determine fldChar element end.
		var fldEndAbs int
		if fldTagCloseAbs > 0 && data[fldTagCloseAbs-1] == '/' {
			// Self-closing `<w:fldChar .../>`.
			fldEndAbs = fldTagCloseAbs + 1
		} else {
			// Paired `<w:fldChar ...></w:fldChar>`.
			closingTag := []byte(`</w:fldChar>`)
			if !bytes.HasPrefix(data[fldTagCloseAbs+1:], closingTag) {
				out = append(out, data[pos:fldTagCloseAbs+1]...)
				pos = fldTagCloseAbs + 1
				continue
			}
			fldEndAbs = fldTagCloseAbs + 1 + len(closingTag)
		}
		// Expect `</w:r>` next.
		runCloseTag := []byte(`</w:r>`)
		if !bytes.HasPrefix(data[fldEndAbs:], runCloseTag) {
			out = append(out, data[pos:fldEndAbs]...)
			pos = fldEndAbs
			continue
		}
		fldRunEndAbs := fldEndAbs + len(runCloseTag)
		// We've identified a `<w:r ...><w:rPr>X</w:rPr><w:fldChar begin
		// .../></w:r>` envelope. Now look at what follows: it must be a
		// `<w:r ...><w:rPr>Y</w:rPr><w:instrText ...>` envelope.
		innerRPr := data[rprInnerStart:rprInnerEnd]
		// Compute the post-envelope position.
		nextRunPos := fldRunEndAbs
		nextRunOpenIdx := bytes.Index(data[nextRunPos:], []byte(`<w:r`))
		stripped := false
		if nextRunOpenIdx == 0 && len(innerRPr) > 0 {
			// Verify the next run starts with rPr + instrText.
			nextRunOpenAbs := nextRunPos
			nextOpenClose := bytes.IndexByte(data[nextRunOpenAbs:], '>')
			if nextOpenClose > 0 {
				nextRunOpenEndAbs := nextRunOpenAbs + nextOpenClose + 1
				// First, ensure this is `<w:r>` or `<w:r `, not
				// `<w:rPr` etc.
				ch := data[nextRunOpenAbs+len("<w:r")]
				if ch == '>' || ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r' {
					if bytes.HasPrefix(data[nextRunOpenEndAbs:], rprPrefix) {
						nextRprInnerStart := nextRunOpenEndAbs + len(rprPrefix)
						nextRprClose := bytes.Index(data[nextRprInnerStart:], []byte(`</w:rPr>`))
						if nextRprClose >= 0 {
							nextRprInnerEnd := nextRprInnerStart + nextRprClose
							nextRprEndAbs := nextRprInnerEnd + len(`</w:rPr>`)
							instrPrefix := []byte(`<w:instrText`)
							if bytes.HasPrefix(data[nextRprEndAbs:], instrPrefix) {
								// Quick check the byte after the
								// `<w:instrText` is space or `>` (not
								// part of a longer name).
								after := data[nextRprEndAbs+len(instrPrefix)]
								if after == ' ' || after == '>' || after == '\t' || after == '\n' || after == '\r' {
									innerInstrRPr := data[nextRprInnerStart:nextRprInnerEnd]
									// Strict superset: instrText's rPr must
									// carry all of fldChar's rPr children,
									// PLUS at least one additional child.
									// Identical rPrs leave the strip OFF —
									// reference Okapi keeps the fldChar rPr
									// when the two carry exactly the same
									// children (no minified() collapse to
									// trigger). Fixture 1102.docx is the
									// canonical guard: both rPrs are
									// `<w:b/>` and the reference retains
									// the rPr on the begin run.
									if rPrChildrenSubset(innerRPr, innerInstrRPr) &&
										countRPrChildren(innerInstrRPr) > countRPrChildren(innerRPr) {
										// Emit the begin-run with rPr stripped.
										runOpenTag := data[runOpenAbs:runOpenEndAbs]
										// Copy everything up to runOpenAbs.
										out = append(out, data[pos:runOpenAbs]...)
										out = append(out, runOpenTag...)
										out = append(out, data[fldStart:fldEndAbs]...)
										out = append(out, runCloseTag...)
										pos = fldRunEndAbs
										stripped = true
									}
								}
							}
						}
					}
				}
			}
		}
		if !stripped {
			// Preserve verbatim up through the end of this fld-begin run.
			out = append(out, data[pos:fldRunEndAbs]...)
			pos = fldRunEndAbs
		}
	}
	return out
}

// bareTextThenBarePTabRunRE matches a bare `<w:r><w:t [attrs]>content</w:t></w:r>`
// envelope IMMEDIATELY followed by a bare `<w:r><w:ptab .../></w:r>` envelope.
// Both envelopes must lack `<w:rPr>` so the fuse is rPr-equivalent. The ptab
// element is logically empty per ECMA-376-1 §17.3.3.27 (CT_PTab) and is
// matched in both self-closing and open/close (re-emitted by encoding/xml)
// shapes. Captures:
//
//	$1 = the t element verbatim (open tag + body + close tag)
//	$2 = the ptab element verbatim (self-closing or open/close form)
//
// Used by fuseBareTextAndPTabRuns; see the call site for the upstream Okapi
// RunMerger citation and the OpenXML_text_reference_v1_2.docx fixture
// rationale.
var bareTextThenBarePTabRunRE = regexp.MustCompile(
	`<w:r>(<w:t\b[^>]*>[^<]*</w:t>)</w:r><w:r>(<w:ptab\b[^>]*(?:/>|></w:ptab>))</w:r>`)

// barePTabThenBareTextRunRE matches a bare `<w:r><w:ptab .../></w:r>`
// envelope IMMEDIATELY followed by a bare `<w:r><w:t [attrs]>content</w:t></w:r>`
// envelope. Both envelopes must lack `<w:rPr>` so the fuse is rPr-equivalent.
// Captures:
//
//	$1 = the ptab element verbatim (self-closing or open/close form)
//	$2 = the t element verbatim (open tag + body + close tag)
//
// Used by fuseBareTextAndPTabRuns to handle the reverse adjacency (ptab
// followed by text) once the leading-text-then-ptab fuse has folded the
// first ptab into the leading text envelope.
var barePTabThenBareTextRunRE = regexp.MustCompile(
	`<w:r>(<w:ptab\b[^>]*(?:/>|></w:ptab>))</w:r><w:r>(<w:t\b[^>]*>[^<]*</w:t>)</w:r>`)

// bareTextPTabEnvelopePairRE matches a `<w:r>` envelope whose body is a
// sequence of one or more bare `<w:t>` and `<w:ptab/>` children (no rPr)
// immediately followed by another `<w:r>` envelope of the same shape.
// Both envelopes must lack `<w:rPr>` so the fuse is rPr-equivalent. The
// first envelope is the already-fused leading chunk produced by the
// initial t+ptab fold; this iterative regex absorbs adjacent same-shape
// envelopes into the same `<w:r>`. Captures:
//
//	$1 = the leading envelope body (one or more t and/or ptab children)
//	$2 = the trailing envelope body (one or more t and/or ptab children)
//
// Used by fuseBareTextAndPTabRuns to walk a t/ptab/t/ptab/t alternation
// (or any superset) into a single envelope.
var bareTextPTabEnvelopePairRE = regexp.MustCompile(
	`<w:r>((?:<w:t\b[^>]*>[^<]*</w:t>|<w:ptab\b[^>]*(?:/>|></w:ptab>))+)</w:r>` +
		`<w:r>((?:<w:t\b[^>]*>[^<]*</w:t>|<w:ptab\b[^>]*(?:/>|></w:ptab>))+)</w:r>`)

// sameRPrAnnotationRefThenTextRunRE matches a `<w:r><w:rPr>…</w:rPr>
// <w:annotationRef/></w:r>` envelope immediately followed by a
// `<w:r><w:rPr>…</w:rPr><w:t [attrs]>content</w:t></w:r>` envelope
// where both `<w:rPr>` bodies match a single rStyle pointing at the
// CommentReference style (the canonical comment-marker rPr per
// ECMA-376-1 §17.13.4.1 / §17.13.4.5). Both rStyle inner elements may
// appear in self-closing (`<w:rStyle .../>`) or open/close
// (`<w:rStyle ...></w:rStyle>`) form — encoding/xml may re-emit either
// shape depending on the captured rawMarkup provenance, but per
// ECMA-376-1 §17.7.4.5 (CT_String) the element is logically empty in
// both. Captures:
//
//	$1 = the leading run's rPr/rStyle val attribute (re-emitted verbatim)
//	$2 = the trailing t element verbatim (open tag + body + close tag)
//
// The replacement fuses the two runs by emitting one `<w:r>` envelope
// carrying the canonical self-closing rStyle inside the rPr, followed
// by `<w:annotationRef/>` and the captured `<w:t>` child. Per
// ECMA-376-1 §17.3.2.1 (CT_R) a single `<w:r>` may carry both
// `<w:annotationRef>` and `<w:t>` children alongside one shared
// `<w:rPr>`.
//
// Used by fuseSameRPrAnnotationRefAndTextRuns; see the call site for
// the upstream Okapi RunMerger citation and the
// OpenXML_text_reference_v1_2.docx (comments.xml) fixture rationale.
var sameRPrAnnotationRefThenTextRunRE = regexp.MustCompile(
	`<w:r><w:rPr><w:rStyle w:val="CommentReference"(?:/>|></w:rStyle>)</w:rPr><w:annotationRef/></w:r>` +
		`<w:r><w:rPr><w:rStyle w:val="CommentReference"(?:/>|></w:rStyle>)</w:rPr>(<w:t\b[^>]*>[^<]*</w:t>)</w:r>`)

// fuseSameRPrAnnotationRefAndTextRuns collapses an
// `<w:r><w:rPr><w:rStyle w:val="CommentReference"/></w:rPr><w:annotationRef/></w:r>`
// envelope followed by an
// `<w:r><w:rPr><w:rStyle w:val="CommentReference"/></w:rPr><w:t>…</w:t></w:r>`
// envelope into a single `<w:r>` carrying the rPr, the annotationRef
// and the t children together. Mirrors upstream Okapi RunMerger
// (RunMerger.java:402-441 fusing a Markup + RunText body chunk pair
// when the containing runs share rPr per canRunPropertiesBeMerged at
// 156-229). The two source rPrs are equal modulo `<w:rStyle>` self-close
// vs open/close form — the regex accepts either shape, and the
// replacement always emits the canonical self-closing form.
//
// Per ECMA-376-1 §17.13.4.1 (annotation comment body) every
// `<w:comment>` body opens with a marker run carrying the
// CommentReference rStyle plus an `<w:annotationRef/>` child. The
// comment's display text appears in the immediately-following same-rPr
// run; bridge's RunMerger fuses these two source runs into one
// envelope, while native's per-run skeleton emit preserves the source's
// envelope split. OpenXML_text_reference_v1_2.docx (comments.xml) is
// the canonical fixture exercising this path.
func fuseSameRPrAnnotationRefAndTextRuns(data []byte) []byte {
	if !bytes.Contains(data, []byte(`<w:annotationRef/>`)) {
		return data
	}
	return sameRPrAnnotationRefThenTextRunRE.ReplaceAll(data,
		[]byte(`<w:r><w:rPr><w:rStyle w:val="CommentReference"/></w:rPr><w:annotationRef/>$1</w:r>`))
}

// fuseBareTextAndPTabRuns collapses adjacent bare `<w:r><w:t>…</w:t></w:r>`
// and `<w:r><w:ptab/></w:r>` envelopes into a single `<w:r>` envelope
// carrying all `<w:t>` and `<w:ptab/>` children side-by-side. All source
// envelopes must lack `<w:rPr>` so the fuse is rPr-equivalent. Mirrors
// upstream Okapi RunMerger.mergeRunBodyChunks (RunMerger.java:402-441)
// fusing adjacent same-rPr runs: a sequence of bare `<w:r>` envelopes
// with matching (here: empty) RunProperties merges into one RunBuilder
// whose body chunks list intersperses RunText (the <w:t>) and Markup
// (the <w:ptab/>) chunks. Per ECMA-376-1 §17.3.2.1 (CT_R) a single
// `<w:r>` may carry both `<w:t>` and `<w:ptab>` children alongside one
// shared `<w:rPr>` (here: empty / absent). Per §17.3.3.27 (CT_PTab) the
// positional-tab element's attrs (alignment/leader/relativeTo) are
// independent of the run's text bytes, so preserving the captured ptab
// tag verbatim survives the round-trip.
//
// OpenXML_text_reference_v1_2.docx (header2.xml) is the canonical
// fixture: a header paragraph with the alternation
// `<w:r><w:t>left</w:t></w:r><w:r><w:ptab center/></w:r><w:r><w:t>center</w:t></w:r><w:r><w:ptab right/></w:r><w:r><w:t>right</w:t></w:r>`
// where bridge's RunMerger fuses the five runs into one envelope while
// native's per-run skeleton emit keeps them split. The fuse runs in
// postNonWSOForName (after skeleton reconstruction + WSO) so it sees the
// post-WSO wire shape where the rPr-less envelopes have already been
// emitted.
//
// The fuse is applied iteratively (loop until no further change) so a
// long alternation like t/ptab/t/ptab/t collapses into a single envelope
// in one call. The two regexes handle either starting adjacency
// (text-first or ptab-first) and the iterative version absorbs the next
// bare text run into the growing envelope.
func fuseBareTextAndPTabRuns(data []byte) []byte {
	if !bytes.Contains(data, []byte(`<w:ptab`)) {
		return data
	}
	for {
		next := bareTextThenBarePTabRunRE.ReplaceAll(data, []byte(`<w:r>$1$2</w:r>`))
		next = barePTabThenBareTextRunRE.ReplaceAll(next, []byte(`<w:r>$1$2</w:r>`))
		next = bareTextPTabEnvelopePairRE.ReplaceAll(next, []byte(`<w:r>$1$2</w:r>`))
		if bytes.Equal(next, data) {
			return data
		}
		data = next
	}
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
// rPr X verbatim from the second slot.
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
// In addition to the structural fuse, this function also strips the
// `w:hAnsi="…"` attribute from the fused rPr's `<w:rFonts>` when the
// rFonts element carries `w:ascii` with the SAME value. This mirrors
// upstream Okapi's RunProperties.minified() collapse of redundant
// rFonts attributes against the pPr.rPr-inherited rFonts context:
// when the paragraph mark's rFonts already declares hAnsi at the
// same value, the run's hAnsi is redundant and is dropped, leaving
// only the ascii attribute on the run-level rFonts (which Okapi
// retains as the "primary" Latin font marker). Fixture: Hangs.docx
// header1 paragraph at offset ~413K, where a bare pict run wrapping
// the s1059 shape is followed by a text run carrying
// `<w:rFonts w:ascii="Times New Roman" w:hAnsi="Times New Roman"/>`
// and the bridge output emits the fused run with
// `<w:rFonts w:ascii="Times New Roman"/>` only.
//
// The pict body is matched non-greedily up to the FIRST occurrence of
// `</w:pict></w:r>` to keep the regex bounded; nested `<w:pict>` does
// not occur in the OOXML fixture corpus (VML pict bodies hold
// shape/textbox children but never another `<w:pict>` per ECMA-376-1
// §17.3.3.9 / §M.6.2). The text body matches non-greedily up to
// `</w:t></w:r>`.
//
// The leading-bare-r gate (`<w:r><w:pict>`, no rPr) distinguishes
// pict runs that already share rPr with neighbors (where the
// structural fuse path inside writeWMLBlock handles the join) from
// the SKELETON-EMITTED case where the source's bare-rPr pict run is
// re-emitted verbatim through the runToXML path. Hangs.docx exercises
// the skeleton path: the surrounding paragraph is non-translatable
// apart from a short text run, so the runs flow through skeleton
// reconstruction and arrive at this post-pass as the un-fused
// envelope pair.
func fuseBarePictAndRPrTextRuns(data []byte) []byte {
	if !bytes.Contains(data, []byte(`<w:r><w:pict>`)) {
		return data
	}
	// Non-regex walker — pict bodies contain heavy markup (textbox,
	// imagedata, OLEObject) whose regex non-greedy walk over megabytes
	// of XML would be slow. The walker pairs each `<w:r><w:pict>` open
	// with the NEAREST `</w:pict></w:r>` (no nested pict in OOXML
	// fixtures per ECMA-376-1 §17.3.3.9) and checks if the suffix is a
	// bare-rPr text run envelope.
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
		// the regex/walker assumes a single text leaf).
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
		// them split. Fixture: Hangs.docx header1 s1059 (rPr =
		// rFonts ascii+hAnsi only → fuse) vs s1062 (rPr = rFonts +
		// sz → keep split).
		rPrInner := data[rPrBodyStart:rPrCloseAt]
		if !isMergeableBareRPrRFonts(rPrInner) {
			out = append(out, data[openAt:afterPict]...)
			pos = afterPict
			continue
		}
		// Build the fused envelope. The rPr body is taken verbatim
		// from the second run, then minified by stripping
		// `w:hAnsi="X"` when `w:ascii="X"` is present with the same
		// value (see function comment for the rationale).
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
	asciiRE := regexp.MustCompile(`w:ascii="([^"]+)"`)
	hAnsiRE := regexp.MustCompile(`w:hAnsi="([^"]+)"`)
	asciiMatch := asciiRE.FindSubmatch(tag)
	hAnsiMatch := hAnsiRE.FindSubmatch(tag)
	if asciiMatch == nil || hAnsiMatch == nil {
		return false
	}
	if !bytes.Equal(asciiMatch[1], hAnsiMatch[1]) {
		return false
	}
	// Reject if there are extra attributes beyond ascii+hAnsi (e.g.
	// hint="eastAsia", cs, eastAsia, etc.). Counting attributes:
	// the rFonts element must have exactly 2 attributes.
	attrRE := regexp.MustCompile(`\bw:[A-Za-z]+="`)
	return len(attrRE.FindAll(tag, -1)) == 2
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
	asciiRE := regexp.MustCompile(`w:ascii="([^"]+)"`)
	asciiMatch := asciiRE.FindSubmatch(tag)
	if asciiMatch == nil {
		return rPrBody
	}
	asciiVal := asciiMatch[1]
	hAnsiPattern := []byte(fmt.Sprintf(` w:hAnsi="%s"`, asciiVal))
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

// wmlRevisionParagraphMarkRE matches the EMPTY-BODY forms of the
// paragraph-mark revision elements that appear INSIDE <w:rPr>:
//   - <w:ins .../>           (RUN_PROPERTY_INSERTED_PARAGRAPH_MARK)
//   - <w:ins ...></w:ins>    (same, re-emitted by encoding/xml)
//   - <w:del .../>           (RUN_PROPERTY_DELETED_PARAGRAPH_MARK)
//   - <w:del ...></w:del>    (same)
//   - <w:moveTo .../>        (RUN_PROPERTY_MOVED_PARAGRAPH_TO)
//   - <w:moveTo ...></w:moveTo>
//   - <w:moveFrom .../>      (RUN_PROPERTY_MOVED_PARAGRAPH_FROM)
//   - <w:moveFrom ...></w:moveFrom>
//
// These are skipped by okapi's SkippableElements.RevisionProperty when
// AutomaticallyAcceptRevisions=true (the default — line 819 of
// ConditionalParameters.java). Per okapi's SkippableElement.java lines
// 231-234.
//
// Only the empty-body form is matched: the content-wrapping form
// (<w:ins><w:r>...</w:r></w:ins> as inline-content marker — child
// element present) is handled differently by okapi (the wrapper is
// unwrapped, children kept). The fixture corpus uses self-closing/empty
// form for paragraph-mark revisions universally; the empty-body open/
// close form arises only when encoding/xml re-emits a previously
// self-closing tag as open/close.
var wmlRevisionParagraphMarkRE = regexp.MustCompile(
	`<w:ins\b[^>]*/>` +
		`|<w:ins\b[^>]*></w:ins>` +
		`|<w:del\b[^>]*/>` +
		`|<w:del\b[^>]*></w:del>` +
		`|<w:moveTo\b[^>]*/>` +
		`|<w:moveTo\b[^>]*></w:moveTo>` +
		`|<w:moveFrom\b[^>]*/>` +
		`|<w:moveFrom\b[^>]*></w:moveFrom>`,
)

// wmlRevisionPropertyChangeNames are the WordprocessingML
// revision-property "change tracking" elements that okapi strips when
// AutomaticallyAcceptRevisions=true (the default — see line 819 of
// okapi/filters/openxml/src/main/java/net/sf/okapi/filters/openxml/
// ConditionalParameters.java) via SkippableElements.RevisionProperty
// (lines 506-569 of SkippableElements.java). All carry nested
// <w:rPr>/<w:pPr>/etc. snapshots of pre-revision properties; stripping
// them preserves only the post-revision (current) state.
//
// Per okapi's SkippableElement.RevisionProperty enum (lines 229-245 of
// SkippableElement.java):
//   - pPrChange    (PARAGRAPH_PROPERTIES_CHANGE)
//   - rPrChange    (RUN_PROPERTIES_CHANGE)
//   - sectPrChange (SECTION_PROPERTIES_CHANGE)
//   - tblGridChange (TABLE_GRID_CHANGE)
//   - tblPrChange  (TABLE_PROPERTIES_CHANGE)
//   - tblPrExChange (TABLE_PROPERTIES_EXCEPTIONS_CHANGE)
//   - tcPrChange   (TABLE_CELL_PROPERTIES_CHANGE)
//   - trPrChange   (TABLE_ROW_PROPERTIES_CHANGE)
//
// Note: <w:ins> and <w:del> when used as paragraph-mark revision markers
// inside <w:rPr> (RUN_PROPERTY_INSERTED/DELETED_PARAGRAPH_MARK) are also
// in the same enum but require context-aware stripping (only inside
// <w:rPr>, not as content wrappers); they are intentionally NOT included
// in the unconditional regex to avoid stripping content-wrapper <w:ins>/
// <w:del> elements that have legitimate text payload. Most fixtures with
// these don't reach the canonical-equal tier for other reasons anyway.
var wmlRevisionPropertyChangeNames = []string{
	"pPrChange",
	"rPrChange",
	"sectPrChange",
	"tblGridChange",
	"tblPrChange",
	"tblPrExChange",
	"tcPrChange",
	"trPrChange",
}

// wmlFldCharEndRE matches a `<w:fldChar w:fldCharType="end"/>` element
// in both self-closing (`.../>`) and open/close
// (`<w:fldChar ...></w:fldChar>`) forms. Per ECMA-376-1 §17.16.5 the
// element is always logically empty; the captured payload from the
// reader can be either shape depending on the encoding/xml emit path.
// Used to detect a fldChar-end inside a captured field run's payload
// so the writer can merge the field-end `<w:r>` with a following
// same-rPr text run, mirroring upstream Okapi RunMerger
// (RunMerger.java:402-441 mergeRunBodyChunks concatenates a Markup
// chunk followed by a RunText chunk when their containing runs share
// rPr per canRunPropertiesBeMerged at RunMerger.java:156-229). Per
// ECMA-376-1 §17.3.2.1 (CT_R), a single `<w:r>` may carry both
// fldChar and `<w:t>` children.
var wmlFldCharEndRE = regexp.MustCompile(
	`<w:fldChar\b[^>]*\bw:fldCharType="end"[^>]*/>` +
		`|<w:fldChar\b[^>]*\bw:fldCharType="end"[^>]*></w:fldChar>`,
)

// wmlFldCharBeginRE matches a `<w:fldChar w:fldCharType="begin"/>` element
// in both self-closing and open/close forms. Per ECMA-376-1 §17.16.5 the
// element is always logically empty. Used by the writer to detect a
// fldChar-begin Ph following a same-rPr text run so the two can be fused
// into a single `<w:r>` carrying both `<w:t>…</w:t>` and `<w:fldChar/>`.
// Mirrors the symmetric fldChar-end merge path (wmlFldCharEndRE), modelled
// on upstream Okapi RunMerger (RunMerger.add at RunMerger.java:83-95 +
// canRunPropertiesBeMerged at RunMerger.java:156-229) — adjacent same-rPr
// runs fuse before serialisation, so the source's plain text run
// preceding the fldChar-begin run is emitted inside the same `<w:r>` as
// the fldChar. Per ECMA-376-1 §17.3.2.1 (CT_R) a single `<w:r>` may
// carry both `<w:fldChar>` and `<w:t>` children. Fixtures
// 1083-*-hyperlink-* exercise this path.
var wmlFldCharBeginRE = regexp.MustCompile(
	`<w:fldChar\b[^>]*\bw:fldCharType="begin"[^>]*/>` +
		`|<w:fldChar\b[^>]*\bw:fldCharType="begin"[^>]*></w:fldChar>`,
)

// wmlRunStartTagRE matches the leading `<w:r ...>` opening tag of a
// captured field payload. Used by the fldChar-begin merge path to
// identify the inner span between the `<w:r>` open and the first child
// element so the writer can verify no extraneous siblings are present
// before fusing the field-begin into the preceding text run.
var wmlRunStartTagRE = regexp.MustCompile(`^\s*<w:r\b[^>]*>`)

// wmlRPrInRunPayloadRE captures the `<w:rPr>...</w:rPr>` element inside
// a captured `<w:r>...</w:r>` field payload. Used to extract the
// run-prop fragment so it can be compared against the next text run's
// effectiveRPr to decide whether to merge them into a single `<w:r>`.
// Per ECMA-376-1 §17.3.2.1 (CT_R) a `<w:rPr>` is the first child of
// `<w:r>` when present.
var wmlRPrInRunPayloadRE = regexp.MustCompile(
	`<w:rPr\b[^>]*>([\s\S]*?)</w:rPr>` +
		`|<w:rPr\b[^>]*/>`,
)

// wmlCloseRunTagRE matches the trailing `</w:r>` of a captured field
// payload. Used by the fldChar-end + text merge path to drop the
// close-run tag so the following text can append `<w:t>` inside the
// still-open `<w:r>`.
var wmlCloseRunTagRE = regexp.MustCompile(`</w:r>\s*$`)

// fldCharEndMergeInfo holds the parts of a captured fldChar-end `<w:r>`
// payload we need to inspect when deciding whether to merge it with a
// following same-rPr text run.
type fldCharEndMergeInfo struct {
	// rprChildren is the children-only contents of the embedded
	// `<w:rPr>` (empty string when the payload has no rPr or a
	// self-closing `<w:rPr/>`).
	rprChildren string
	// truncated is the payload with its trailing `</w:r>` stripped so
	// the writer can keep the run open and append `<w:t>` from the
	// following text run.
	truncated string
	// ok is true only when the payload contains exactly one
	// `<w:fldChar w:fldCharType="end"/>` and ends with `</w:r>`.
	ok bool
}

// detectFldCharEndForMerge inspects a TypeField Ph payload (the raw
// captured `<w:r>...</w:r>` shell wrapping a fldChar). If the payload
// is a fldChar-end run with a well-formed shape it returns a populated
// fldCharEndMergeInfo with ok=true; otherwise ok=false and the caller
// emits the payload verbatim.
//
// The rPr extraction normalises whitespace between siblings and
// collapses open/close empty elements (`<w:b></w:b>` → `<w:b/>`) so
// the result can be compared byte-for-byte against the
// writer-synthesised rPr (which uses self-closing toggle markers like
// `<w:b/>`). Mirrors upstream Okapi's mergeRunBodyChunks
// (RunMerger.java:402-441) fusing a Markup chunk followed by a
// RunText chunk when their containing runs share rPr per
// canRunPropertiesBeMerged (RunMerger.java:156-229). Per ECMA-376-1
// §17.3.2.1 (CT_R), a single `<w:r>` may carry both `<w:fldChar>`
// and `<w:t>` children.
func detectFldCharEndForMerge(payload string) fldCharEndMergeInfo {
	if !wmlFldCharEndRE.MatchString(payload) {
		return fldCharEndMergeInfo{}
	}
	if !wmlCloseRunTagRE.MatchString(payload) {
		return fldCharEndMergeInfo{}
	}
	rpr := ""
	if m := wmlRPrInRunPayloadRE.FindStringSubmatch(payload); m != nil {
		rpr = normaliseRPrChildrenFragment(m[1])
		// The reader's stripFieldRPrSkippables stamps the
		// fieldRPrKeepEmptyMarker (an XML comment) inside an otherwise
		// empty rPr to prevent the writer's stripWMLSkippableElements
		// pass from collapsing it. The marker is stripped from the
		// final wire by postNonWSOForName, so the EFFECTIVE rPr we
		// compare against is the empty string. Without this
		// normalisation the merge gate would never fire on
		// post-strip-empty rPr field-end runs (the 1083-*-hyperlink-*
		// cluster). See fieldRPrKeepEmptyMarker in wml.go for the
		// upstream-Okapi rationale.
		if rpr == fieldRPrKeepEmptyMarker {
			rpr = ""
		}
	}
	truncated := wmlCloseRunTagRE.ReplaceAllString(payload, "")
	return fldCharEndMergeInfo{
		rprChildren: rpr,
		truncated:   truncated,
		ok:          true,
	}
}

// fldCharBeginMergeInfo holds the inspection result for a captured
// fldChar-begin `<w:r>` payload when the writer is considering fusing it
// with the immediately-preceding same-rPr text run. The inner fldChar
// element (without the surrounding `<w:r>...</w:r>` shell) is appended
// inside the still-open text run, mirroring upstream Okapi RunMerger
// behavior for adjacent same-rPr runs (RunMerger.add at
// RunMerger.java:83-95 + canRunPropertiesBeMerged at
// RunMerger.java:156-229). Per ECMA-376-1 §17.3.2.1 (CT_R) a single
// `<w:r>` may carry both `<w:t>` and `<w:fldChar>` children.
type fldCharBeginMergeInfo struct {
	// rprChildren is the children-only contents of the embedded
	// `<w:rPr>` after normalisation (empty when the payload has no rPr
	// or a self-closing `<w:rPr/>`).
	rprChildren string
	// innerFldChar is the `<w:fldChar w:fldCharType="begin"/>` element
	// alone, ready to be appended inside the open `<w:r>` after the
	// closing `</w:t>` is emitted.
	innerFldChar string
	// ok is true only when the payload is a minimal `<w:r>` shell
	// wrapping (optionally) an rPr and a single fldChar-begin child.
	ok bool
}

// detectFldCharBeginForMerge inspects a TypeField Ph payload to decide
// whether the writer can fuse it with a same-rPr text run emitted just
// before it. Returns ok=true only for a minimal
// `<w:r>[<w:rPr>…</w:rPr>]<w:fldChar w:fldCharType="begin"/></w:r>`
// shape — payloads carrying adjacent siblings (e.g. an extra
// `<w:instrText>`) are rejected because the fldChar-begin run is the
// syntactic start of the complex field and is always followed by its
// own run boundary in well-formed Word output. Mirrors upstream Okapi
// RunMerger.add (RunMerger.java:83-95) + canRunPropertiesBeMerged
// (RunMerger.java:156-229) for the case where a plain text run
// immediately preceding a fldChar-begin run carries the same rPr and is
// fused into the field's leading run (see the 1083-*-hyperlink-*
// fixtures — upstream output places `<w:t>A Text</w:t>` and
// `<w:fldChar fldCharType="begin"/>` inside one `<w:r>`). Per
// ECMA-376-1 §17.3.2.1 (CT_R) a `<w:r>` may carry both `<w:t>` and
// `<w:fldChar>` children; §17.16.5 (Complex Fields) classifies fldChar
// as a run child.
func detectFldCharBeginForMerge(payload string) fldCharBeginMergeInfo {
	loc := wmlFldCharBeginRE.FindStringIndex(payload)
	if loc == nil {
		return fldCharBeginMergeInfo{}
	}
	closeLoc := wmlCloseRunTagRE.FindStringIndex(payload)
	if closeLoc == nil {
		return fldCharBeginMergeInfo{}
	}
	runStart := wmlRunStartTagRE.FindStringIndex(payload)
	if runStart == nil {
		return fldCharBeginMergeInfo{}
	}
	// Inspect everything between the `<w:r ...>` open and the fldChar
	// start: it must be either empty or an `<w:rPr>...</w:rPr>` (which
	// belongs to the carrier run, not to a sibling). Anything else
	// (instrText, t, drawing, …) is a multi-child run that upstream
	// Okapi keeps as its own envelope, so abort the fusion.
	beforeFld := payload[runStart[1]:loc[0]]
	if rprMatch := wmlRPrInRunPayloadRE.FindStringIndex(payload); rprMatch != nil &&
		rprMatch[0] >= runStart[1] && rprMatch[1] <= loc[0] {
		beforeFld = payload[runStart[1]:rprMatch[0]] + payload[rprMatch[1]:loc[0]]
	}
	if strings.TrimSpace(beforeFld) != "" {
		return fldCharBeginMergeInfo{}
	}
	// Tail between fldChar-begin and the run's `</w:r>` must be empty
	// (no siblings allowed inside the carrier run).
	if strings.TrimSpace(payload[loc[1]:closeLoc[0]]) != "" {
		return fldCharBeginMergeInfo{}
	}
	rpr := ""
	if m := wmlRPrInRunPayloadRE.FindStringSubmatch(payload); m != nil {
		rpr = normaliseRPrChildrenFragment(m[1])
	}
	return fldCharBeginMergeInfo{
		rprChildren:  rpr,
		innerFldChar: payload[loc[0]:loc[1]],
		ok:           true,
	}
}

// isExtractableFldCharBeginRun reports whether the TypeField Ph at the
// given index is the begin marker of an EXTRACTABLE complex field. The
// determination scans forward through the run list until the matching
// fldChar-end is found (tracking nesting depth so nested fields don't
// confuse the count): when any Text run appears inside the field's
// scope, the field's display text was promoted to translatable text by
// the reader, which only happens for fields whose code is in
// ComplexFieldDefinitionsToExtract (e.g. HYPERLINK). Mirrors upstream
// Okapi RunParser.parseComplexField (lines 461-542 of RunParser.java)
// where parseContent (line 537) routes display events to RunText only
// when `extractable && atComplexFieldResult` is true — otherwise events
// land in runBuilder.addToMarkup (line 505) and stay as opaque markup
// on the field's RunBuilder, never becoming Text in the block.
//
// Used by the writer's fldChar-begin merge path to decide whether to
// fuse the begin run with a preceding same-rPr text run. For
// non-extractable fields (e.g. DATE) upstream's RunMerger refuses the
// fusion (canMergeWith returns false on containsComplexFields, line
// 147 of RunMerger.java), so we must not fuse either. Fixture
// 1083-date-and-hyperlink-instructions.docx is the canonical
// non-extractable case.
func isExtractableFldCharBeginRun(runs []model.Run, beginIdx int) bool {
	depth := 1
	for j := beginIdx + 1; j < len(runs); j++ {
		nr := runs[j]
		if nr.Text != nil {
			// A Text run anywhere inside this field's nesting scope
			// signals the reader extracted display text — i.e. the
			// (innermost) field is extractable and we're past the
			// separator. The outer field carrying this run is also
			// extractable per upstream's recursive parseComplexField.
			return true
		}
		if nr.Ph == nil || nr.Ph.Type != TypeField {
			continue
		}
		// fldSimple sentinels (SubTypeFieldSimple) are self-contained
		// and don't contribute to begin/end nesting depth.
		if nr.Ph.SubType == SubTypeFieldSimple {
			continue
		}
		data := nr.Ph.Data
		switch {
		case wmlFldCharBeginRE.MatchString(data):
			depth++
		case wmlFldCharEndRE.MatchString(data):
			depth--
			if depth == 0 {
				return false
			}
		}
	}
	return false
}

// isExtractableFldCharEndRun reports whether the TypeField Ph at the
// given index closes an EXTRACTABLE complex field. Symmetric to
// isExtractableFldCharBeginRun but scans BACKWARD to find the matching
// fldChar-begin Ph: when any Text run appears between the begin and
// end, the field's display text was promoted to translatable text by
// the reader (extractable). Mirrors upstream Okapi
// RunParser.parseComplexField (RunParser.java:461-542) where
// parseContent (line 537) routes display events to RunText only when
// `extractable && atComplexFieldResult` is true.
//
// Used by the writer's fldChar-end + text merge to gate the fusion:
// for non-extractable fields (e.g. DATE) upstream keeps the field-end
// run as opaque markup separate from any following translatable text,
// matching the source's run boundary. Fixture
// 1083-date-and-hyperlink-instructions.docx is the canonical
// non-extractable case where merging would diverge from upstream.
func isExtractableFldCharEndRun(runs []model.Run, endIdx int) bool {
	depth := 1
	for j := endIdx - 1; j >= 0; j-- {
		nr := runs[j]
		if nr.Text != nil {
			return true
		}
		if nr.Ph == nil || nr.Ph.Type != TypeField {
			continue
		}
		if nr.Ph.SubType == SubTypeFieldSimple {
			continue
		}
		data := nr.Ph.Data
		switch {
		case wmlFldCharEndRE.MatchString(data):
			depth++
		case wmlFldCharBeginRE.MatchString(data):
			depth--
			if depth == 0 {
				return false
			}
		}
	}
	return false
}

// emptyElementOpenCloseRE matches an XML element with an empty body
// in open/close form (e.g. `<w:b></w:b>` or `<w:i  attr="x"></w:i>`).
// Used by normaliseRPrChildrenFragment to collapse the open/close
// form emitted by Go's encoding/xml into the self-closing form
// authored by Word and emitted by the toggle-writer's addWMLProp.
// Go's regexp doesn't support backreferences, so the post-processing
// step verifies name equality in code.
var emptyElementOpenCloseRE = regexp.MustCompile(
	`<(w:[A-Za-z][A-Za-z0-9]*)\b([^>]*)></w:([A-Za-z][A-Za-z0-9]*)>`,
)

// betweenTagsWhitespaceRE matches whitespace between tags, used to
// strip the layout whitespace encoding/xml inserts when re-emitting
// nested elements (e.g. `</w:rPr>\n  <w:fldChar...>` between rPr and
// fldChar siblings). The rPr children fragment carries no significant
// inter-tag whitespace per ECMA-376-1 §17.3.2 — toggle and property
// elements have no mixed content.
var betweenTagsWhitespaceRE = regexp.MustCompile(`>\s+<`)

// leadingTrailingWhitespaceRE strips leading and trailing whitespace.
var leadingTrailingWhitespaceRE = regexp.MustCompile(`^\s+|\s+$`)

// normaliseRPrChildrenFragment normalises a `<w:rPr>` children-only
// fragment to the canonical form the writer's toggle-writer
// (`addWMLProp`) emits: self-closing tags with no inter-tag
// whitespace. Per ECMA-376-1 §17.3.2 the rPr children are empty
// elements with attributes; the canonical serialization is
// `<w:b/><w:i/>...` regardless of whether the source used the
// self-closing or open/close form.
func normaliseRPrChildrenFragment(s string) string {
	if s == "" {
		return s
	}
	s = betweenTagsWhitespaceRE.ReplaceAllString(s, `><`)
	s = leadingTrailingWhitespaceRE.ReplaceAllString(s, "")
	s = emptyElementOpenCloseRE.ReplaceAllStringFunc(s, func(m string) string {
		sub := emptyElementOpenCloseRE.FindStringSubmatch(m)
		if len(sub) != 4 {
			return m
		}
		// sub[1] is `w:NAME`, sub[3] is `NAME` (the closing tag's
		// local name without the `w:` prefix). Verify the local
		// names agree before collapsing.
		if sub[1][2:] != sub[3] {
			return m
		}
		return `<` + sub[1] + sub[2] + `/>`
	})
	return s
}

// sdtPropertiesContainerNames lists the SDT-envelope property containers
// whose inner contents must NOT be touched by the writer-side
// RunSkippableElements-mirroring strips. Both elements carry an inner
// `<w:rPr>` per ECMA-376-1 §17.5.2.40 (CT_SdtPr) and §17.5.2.38
// (CT_SdtEndPr) — the placeholder and post-content run properties for
// the SDT preview — and upstream Okapi routes them through
// BlockParser.parseRunContainer:155-166 with `emptySkippableElements`,
// so noProof/lang are preserved inside.
var sdtPropertiesContainerNames = [...]string{"sdtPr", "sdtEndPr"}

// applyREOutsideSDTPr applies a regex strip to `data` everywhere EXCEPT
// inside `<w:sdtPr>...</w:sdtPr>` and `<w:sdtEndPr>...</w:sdtEndPr>`
// ranges. Mirrors upstream Okapi's scoped instantiation of
// RunSkippableElements: that strip set is wired into the `<w:r>`
// parsing path (BlockParser.processRun → RunPropertiesParser →
// RunSkippableElements; RunSkippableElements.java:50-62), but the SDT
// envelope's properties blocks route through
// BlockParser.parseRunContainer:155-166 with `emptySkippableElements`
// — i.e. NO `<w:lang>` / `<w:noProof>` stripping. The end-state diff
// shows up on `<w:sdt><w:sdtPr><w:rPr><w:noProof/><w:sz/></w:rPr></w:sdtPr>`
// (956.docx footer1.xml) and `<w:sdtEndPr><w:rPr><w:noProof/></w:rPr>
// </w:sdtEndPr>` (956.docx footer2.xml) where upstream preserves the
// noProof and our writer-side post-pass must leave it alone too. Per
// ECMA-376-1 §17.5.2.40 / §17.5.2.38 the rPr child holds placeholder
// run properties — the non-run rPr context the schema explicitly
// carves out.
//
// sdtPr/sdtEndPr are non-nested in practice (the schema doesn't allow
// either to contain itself), so the scan only needs a flat open/close
// pair match. The implementation walks the buffer and applies the regex
// piecewise to the slices outside of any matched range.
func applyREOutsideSDTPr(data []byte, re *regexp.Regexp) []byte {
	hasContainer := false
	for _, n := range sdtPropertiesContainerNames {
		if bytes.Contains(data, []byte("<w:"+n)) {
			hasContainer = true
			break
		}
	}
	if !hasContainer {
		return re.ReplaceAll(data, nil)
	}
	out := make([]byte, 0, len(data))
	cursor := 0
	for cursor < len(data) {
		// Find the nearest occurrence of any container open tag.
		nearestStart := -1
		nearestName := ""
		for _, n := range sdtPropertiesContainerNames {
			open := []byte("<w:" + n)
			i := bytes.Index(data[cursor:], open)
			if i < 0 {
				continue
			}
			abs := cursor + i
			if nearestStart < 0 || abs < nearestStart {
				nearestStart = abs
				nearestName = n
			}
		}
		if nearestStart < 0 {
			out = append(out, re.ReplaceAll(data[cursor:], nil)...)
			break
		}
		openTag := []byte("<w:" + nearestName)
		closeTag := []byte("</w:" + nearestName + ">")
		boundaryIdx := nearestStart + len(openTag)
		if boundaryIdx >= len(data) {
			out = append(out, re.ReplaceAll(data[cursor:], nil)...)
			break
		}
		b := data[boundaryIdx]
		if b != '>' && b != '/' && b != ' ' && b != '\t' && b != '\n' && b != '\r' {
			// False positive — apply the strip up to and including the
			// rejected boundary byte, then continue scanning past it.
			out = append(out, re.ReplaceAll(data[cursor:boundaryIdx+1], nil)...)
			cursor = boundaryIdx + 1
			continue
		}
		tagEnd := bytes.IndexByte(data[boundaryIdx:], '>')
		if tagEnd < 0 {
			out = append(out, re.ReplaceAll(data[cursor:], nil)...)
			break
		}
		startTagEnd := boundaryIdx + tagEnd + 1
		// Apply strip to the prelude before the container.
		out = append(out, re.ReplaceAll(data[cursor:nearestStart], nil)...)
		// Self-closing form `<w:sdtPr.../>` (no body — copy verbatim).
		if startTagEnd >= 2 && data[startTagEnd-2] == '/' {
			out = append(out, data[nearestStart:startTagEnd]...)
			cursor = startTagEnd
			continue
		}
		// Open form — find matching close.
		closeIdx := bytes.Index(data[startTagEnd:], closeTag)
		if closeIdx < 0 {
			// Unbalanced — preserve the rest as-is to be safe.
			out = append(out, data[nearestStart:]...)
			break
		}
		rangeEnd := startTagEnd + closeIdx + len(closeTag)
		out = append(out, data[nearestStart:rangeEnd]...)
		cursor = rangeEnd
	}
	return out
}

// stripBalancedElement removes every occurrence of <w:NAME ...>...</w:NAME>
// (and the self-closing form <w:NAME .../>) from data, where NAME is the
// supplied local name. The matcher is non-nested — the *Change elements
// in the okapi-testdata corpus never embed themselves recursively, and
// the schema doesn't allow it either. Returns the original slice if the
// element name doesn't appear at all (cheap fast path).
func stripBalancedElement(data []byte, name string) []byte {
	startPrefix := []byte("<w:" + name)
	if !bytes.Contains(data, startPrefix) {
		return data
	}
	endTag := []byte("</w:" + name + ">")
	out := make([]byte, 0, len(data))
	for {
		i := bytes.Index(data, startPrefix)
		if i < 0 {
			out = append(out, data...)
			break
		}
		// Confirm the element-name boundary so "<w:noProofX" doesn't match
		// a longer element name. The next byte must be `>`, `/`, or
		// whitespace.
		j := i + len(startPrefix)
		if j >= len(data) {
			out = append(out, data...)
			break
		}
		b := data[j]
		if b != '>' && b != '/' && b != ' ' && b != '\t' && b != '\n' && b != '\r' {
			out = append(out, data[:j+1]...)
			data = data[j+1:]
			continue
		}
		// Find element terminator within the start tag.
		k := bytes.IndexByte(data[j:], '>')
		if k < 0 {
			out = append(out, data...)
			break
		}
		startEnd := j + k
		out = append(out, data[:i]...)
		// Self-closing form <w:NAME .../>: skip the tag.
		if startEnd > 0 && data[startEnd-1] == '/' {
			data = data[startEnd+1:]
			continue
		}
		// Open form: find matching close tag.
		closeIdx := bytes.Index(data[startEnd+1:], endTag)
		if closeIdx < 0 {
			// Unbalanced — bail out, append remainder unchanged.
			out = append(out, data[i:]...)
			break
		}
		data = data[startEnd+1+closeIdx+len(endTag):]
	}
	return out
}

// stripWMLSkippableElements removes WordprocessingML elements from an
// XML part to mirror okapi's BlockProperties/RunProperties,
// RevisionCrossStructure, and RevisionProperty stripping. Returns the
// original slice if nothing was matched (cheap fast paths).
//
// <w:lang> stripping is gated on the document's WordprocessingML
// namespace URI. Upstream Okapi's RunSkippableElements identifies lang
// by QName — Namespaces.WordProcessingML.getQName("lang") — keyed on
// the TRANSITIONAL WPML URI ("http://schemas.openxmlformats.org/
// wordprocessingml/2006/main", Namespaces.java:26). For Strict OOXML
// documents using "http://purl.oclc.org/ooxml/wordprocessingml/main"
// (858.docx — Word's "Save As → Strict Open XML Document" output),
// the QName does NOT match — the SkippableElements.Default contains
// check at SkippableElements.java:122 returns false — so upstream
// PRESERVES <w:lang> through round-trip. The reference output for
// 858.docx keeps <w:lang> in the paragraph mark rPr (inside pPr) AND
// in the WSO-synthesised paragraph style's rPr.
//
// Native mirrors this: when the document binds the "w" prefix to the
// strict URI, the lang strip is skipped. Both prefix and URI are
// observed in the part itself — the writer doesn't track which doc
// the part came from, but every WPML XML part declares the prefix
// binding on its root element. ECMA-376 Part 1 §A.1 / ISO/IEC 29500-1
// §A.1 (the two URIs).
func stripWMLSkippableElements(data []byte) []byte {
	// stripLang AND stripNoProof are gated on the transitional WPML
	// namespace for the same reason: upstream Okapi binds the
	// RUN_PROPERTY_LANGUAGE / RUN_PROPERTY_NO_SPELLING_OR_GRAMMAR
	// QNames to Namespaces.WordProcessingML which is the transitional
	// URI ("http://schemas.openxmlformats.org/wordprocessingml/2006/
	// main", Namespaces.java:26 + SkippableElement.java:205-207). For
	// Strict OOXML documents the QName does NOT match upstream's
	// skippable set, so both `<w:lang>` AND `<w:noProof>` are PRESERVED
	// through round-trip. 859.docx is the canonical fixture — its
	// drawing-bearing run carries `<w:rPr><w:noProof/><w:lang
	// w:eastAsia="ru-RU"/></w:rPr>` which must round-trip on the wire
	// AND lift into the WSO-synthesised paragraph style's rPr.
	strict := bytes.Contains(data, []byte(wmlStrictNamespace))
	if !strict && bytes.Contains(data, []byte("<w:lang")) {
		// Scope the strip to OUTSIDE `<w:sdtPr>...</w:sdtPr>` /
		// `<w:sdtEndPr>...</w:sdtEndPr>` — upstream Okapi's
		// RunSkippableElements is only wired into the `<w:r>` parsing
		// path; the SDT properties block routes through
		// BlockParser.parseRunContainer:155-166 with
		// `emptySkippableElements`, so `<w:lang>` survives there.
		data = applyREOutsideSDTPr(data, wmlLangElementRE)
	}
	if bytes.Contains(data, []byte("<w:bidiVisual")) {
		data = wmlBidiVisualElementRE.ReplaceAll(data, nil)
	}
	if bytes.Contains(data, []byte("<w:moveToRange")) || bytes.Contains(data, []byte("<w:moveFromRange")) {
		data = wmlMoveRangeStrippableElementRE.ReplaceAll(data, nil)
	}
	if !strict && bytes.Contains(data, []byte("<w:noProof")) {
		// Same SDT scoping as `<w:lang>` — see comment above. Canonical
		// fixture: 956.docx footer1.xml/footer2.xml, Page Numbers
		// (Bottom of Page) SDT — `<w:sdtPr><w:rPr><w:noProof/>
		// <w:sz w:val="14"/></w:rPr>...</w:sdtPr>` and
		// `<w:sdtEndPr><w:rPr><w:noProof/></w:rPr></w:sdtEndPr>` must
		// round-trip with noProof intact.
		data = applyREOutsideSDTPr(data, wmlNoProofRE)
	}
	if bytes.Contains(data, []byte("<w:ins")) ||
		bytes.Contains(data, []byte("<w:del")) ||
		bytes.Contains(data, []byte("<w:moveTo")) ||
		bytes.Contains(data, []byte("<w:moveFrom")) {
		data = wmlRevisionParagraphMarkRE.ReplaceAll(data, nil)
	}
	for _, name := range wmlRevisionPropertyChangeNames {
		data = stripBalancedElement(data, name)
	}
	// Iterate empty <w:rPr>/<w:pPr> stripping until fixpoint: removing an
	// empty <w:rPr></w:rPr> nested inside an otherwise-empty <w:pPr>
	// leaves the parent eligible on the next pass. The fixture corpus
	// requires at most two iterations (<w:p><w:pPr><w:rPr><w:lang/></w:rPr></w:pPr>
	// becomes <w:p/> after lang+rPr+pPr strips), but the loop terminates
	// generally because each pass strictly shrinks the buffer.
	// Loop until the empty-container regex stops shrinking the buffer.
	// The fast-path bytes.Contains gate looks at "<w:rPr" and "<w:pPr"
	// substrings — matches any potentially-stripable form including
	// whitespace-padded variants — which a more specific gate would
	// miss after encoding/xml indented re-emission.
	for bytes.Contains(data, []byte("<w:rPr")) ||
		bytes.Contains(data, []byte("<w:pPr")) {
		next := wmlEmptyPropertiesContainerRE.ReplaceAll(data, nil)
		if len(next) == len(data) {
			break
		}
		data = next
	}
	return data
}

// shouldStripWMLLang reports whether the given ZIP entry path is a
// WordprocessingML XML part where okapi's lang/bidiVisual and
// RevisionCrossStructure (moveTo/moveFrom range) stripping applies.
// Other parts (drawings, themes, settings.xml) are untouched.
//
// `word/glossary/<name>.xml` mirrors the main-document set: ECMA-376-1
// §17.12.7 (Glossary Document Part) defines a glossary as a parallel
// WordprocessingML package whose parts mirror the main document's
// structure (its own document.xml, settings.xml, styles.xml, etc.).
// Okapi's filter walks the glossary document via the same
// XmlEventStreamingPart path as the main document
// (OpenXMLFilter.openZip + DocumentParts.glossary), so the same
// `<w:lang>` strip applies. 834.docx is the canonical fixture.
func shouldStripWMLLang(name string) bool {
	if !strings.HasPrefix(name, "word/") || !strings.HasSuffix(name, ".xml") {
		return false
	}
	switch {
	case name == "word/document.xml",
		name == "word/styles.xml",
		name == "word/footnotes.xml",
		name == "word/endnotes.xml",
		name == "word/comments.xml":
		return true
	case strings.HasPrefix(name, "word/header") && strings.HasSuffix(name, ".xml"),
		strings.HasPrefix(name, "word/footer") && strings.HasSuffix(name, ".xml"):
		return true
	}
	if strings.HasPrefix(name, "word/glossary/") && strings.HasSuffix(name, ".xml") {
		switch name {
		case "word/glossary/document.xml",
			"word/glossary/styles.xml",
			"word/glossary/footnotes.xml",
			"word/glossary/endnotes.xml",
			"word/glossary/comments.xml":
			return true
		}
	}
	return false
}

// wmlLangValAttrRE matches the w:val attribute on a <w:lang ...> or
// <w:themeFontLang ...> element and captures the existing value.
// Submatches: 1=tag name (lang|themeFontLang), 2=quote char, 3=value.
//
// The match is anchored on the opening "<w:lang" or "<w:themeFontLang"
// followed by a word boundary (so it doesn't accept "<w:langfoo>"), then
// scans up to the element terminator (`>` or `/>`) for any w:val=
// attribute. Single and double quotes are both supported. The character
// class for the value side excludes the quote so we don't cross attribute
// boundaries.
var wmlLangValAttrRE = regexp.MustCompile(
	`(<w:(lang|themeFontLang)\b[^>]*?\bw:val=)(["'])([^"']*)(["'])`,
)

// shouldRewriteWMLLangVal reports whether the given ZIP entry path is a
// WordprocessingML XML part where okapi rewrites <w:lang>/<w:themeFontLang>
// w:val attributes from the source locale to the target locale on
// round-trip (mirroring GenericSkeletonWriter's Property.LANGUAGE
// rewriting; see writer.go SetSourceLocale godoc).
//
// The set is the strip set plus settings.xml — okapi's
// RUN_PROPERTY_LANGUAGE skippable list strips <w:lang/> from rPr in
// document/styles/footnotes/endnotes/comments/header/footer parts (so any
// surviving <w:lang> there must have been outside an rPr and is rewritten
// in the same way), while <w:themeFontLang/> sits in settings.xml only and
// is preserved by okapi but with its w:val retargeted.
func shouldRewriteWMLLangVal(name string) bool {
	if name == "word/settings.xml" || name == "word/glossary/settings.xml" {
		return true
	}
	return shouldStripWMLLang(name)
}

// rewriteWMLLangVal rewrites the w:val attribute on every <w:lang> and
// <w:themeFontLang> element when its existing value's primary language
// matches the source locale's primary language. The replacement value
// is the target locale string verbatim (okapi uses LocaleId#toString,
// which is the BCP-47 form).
//
// This mirrors okapi/core/src/main/java/net/sf/okapi/common/skeleton/
// GenericSkeletonWriter.java lines 808-816:
//
//	if ( Property.LANGUAGE.equals(name) ) {
//	    LocaleId locId = LocaleId.fromString(value);
//	    if ( locId.sameLanguageAs(inputLoc) ) {
//	        value = outputLoc.toString();
//	    }
//	}
//
// in combination with okapi/filters/openxml/src/main/java/net/sf/okapi/
// filters/openxml/ContentFilter.java lines 527-537, where the openxml
// filter normalizes the attribute name on <w:lang> and <w:themeFontLang>
// to Property.LANGUAGE so the writer's retargeting kicks in.
//
// Returns the original slice if no eligible attribute was found (so the
// caller can avoid recompressing pass-through entries).
func rewriteWMLLangVal(data []byte, sourceLocale, targetLocale model.LocaleID) []byte {
	if targetLocale.IsEmpty() {
		return data
	}
	// Strict OOXML namespace: upstream's QName-keyed Property.LANGUAGE
	// rewrite does NOT match elements bound to the strict URI
	// "http://purl.oclc.org/ooxml/wordprocessingml/main" — the rewrite
	// hook lives on ContentFilter (lines 527-537) and only fires when
	// the openxml filter has classified the element as a
	// Property.LANGUAGE-carrying WordProcessingML element, which is
	// QName-keyed by the transitional URI (Namespaces.java:26). 858.docx
	// reference output keeps <w:lang w:val="en-US"/> through round-trip
	// even with target=fr, so the native rewrite must also skip strict
	// parts.
	if bytes.Contains(data, []byte(wmlStrictNamespace)) {
		return data
	}
	src := primaryLangOf(sourceLocale)
	if src == "" {
		// Default to "en" — matches okapi OpenXMLFilter's behaviour when
		// no source locale was supplied via setOptions.
		src = "en"
	}
	if !bytes.Contains(data, []byte("<w:lang")) && !bytes.Contains(data, []byte("<w:themeFontLang")) {
		return data
	}
	tgt := []byte(string(targetLocale))
	return wmlLangValAttrRE.ReplaceAllFunc(data, func(match []byte) []byte {
		sub := wmlLangValAttrRE.FindSubmatch(match)
		if sub == nil {
			return match
		}
		// sub[1]=prefix incl. "w:val=", sub[3]=open quote, sub[4]=value, sub[5]=close quote
		existing := string(sub[4])
		if primaryLangOf(model.LocaleID(existing)) != src {
			return match
		}
		out := make([]byte, 0, len(sub[1])+len(sub[3])+len(tgt)+len(sub[5]))
		out = append(out, sub[1]...)
		out = append(out, sub[3]...)
		out = append(out, tgt...)
		out = append(out, sub[5]...)
		return out
	})
}

// primaryLangOf returns the lower-cased primary language subtag of a
// BCP-47 locale ID. Mirrors okapi LocaleId.sameLanguageAs comparison
// semantics (region/script ignored).
func primaryLangOf(l model.LocaleID) string {
	s := strings.ToLower(string(l))
	if i := strings.IndexAny(s, "-_"); i >= 0 {
		s = s[:i]
	}
	return s
}

// Writer implements DataFormatWriter for OpenXML files.
type Writer struct {
	format.BaseFormatWriter
	cfg             *Config
	skeletonStore   *format.SkeletonStore
	originalContent []byte

	// sourceLocale records the input/source locale supplied to the writer
	// (defaults to "en" — okapi's LocaleId.EMPTY default for OpenXMLFilter).
	// Used by the WordprocessingML lang-attribute rewriter to decide whether
	// an existing <w:lang>/<w:themeFontLang> w:val matches the source
	// language and should be retargeted to w.Locale.
	sourceLocale model.LocaleID

	// mediaReplacements maps ZIP entry paths (e.g., "word/media/image1.png")
	// to replacement binary content for locale-variant media substitution (Bowrain AD-007).
	mediaReplacements map[string][]byte

	// blocks holds the current Write call's block index, populated by
	// Write() before invoking renderBlock and consumed by
	// expandDrawingMarkers when renderWMLBlock's TypeImage handler
	// substitutes <!--KAPI-PROP:tu123--> / <!--KAPI-PARA:tu123-->
	// markers inside captured drawing payloads (set by the WML
	// reader via extractDrawingTranslations). Reset at the end of
	// each Write call.
	blocks map[string]*model.Block
}

var _ format.SkeletonStoreConsumer = (*Writer)(nil)
var _ format.OriginalContentSetter = (*Writer)(nil)
var _ format.SourceLocaleSetter = (*Writer)(nil)

// SetMediaReplacement registers a locale-variant media file to substitute
// during output reconstruction. The zipPath should match the original
// entry path (e.g., "word/media/image1.png").
func (w *Writer) SetMediaReplacement(zipPath string, data []byte) {
	if w.mediaReplacements == nil {
		w.mediaReplacements = make(map[string][]byte)
	}
	w.mediaReplacements[zipPath] = data
}

// NewWriter creates a new OpenXML writer.
func NewWriter() *Writer {
	cfg := &Config{}
	cfg.Reset()
	return &Writer{
		BaseFormatWriter: format.BaseFormatWriter{
			FormatName: "openxml",
		},
		cfg: cfg,
	}
}

// SetSkeletonStore sets the skeleton store for streaming reconstruction.
func (w *Writer) SetSkeletonStore(store *format.SkeletonStore) {
	w.skeletonStore = store
}

// SetOriginalContent sets the original document bytes for reconstruction.
func (w *Writer) SetOriginalContent(content []byte) {
	w.originalContent = content
}

// SetSourceLocale records the source/input locale. Used by the
// WordprocessingML lang-attribute rewriter (mirrors okapi's
// GenericSkeletonWriter behavior at lines 808-816 of okapi/core/src/
// main/java/net/sf/okapi/common/skeleton/GenericSkeletonWriter.java
// which retargets Property.LANGUAGE-named attributes from inputLoc to
// outputLoc when sameLanguageAs(inputLoc) holds).
func (w *Writer) SetSourceLocale(locale model.LocaleID) {
	w.sourceLocale = locale
}

// Write consumes Parts and writes the reconstructed OpenXML document.
func (w *Writer) Write(ctx context.Context, parts <-chan *model.Part) error {
	// Collect all blocks keyed by ID
	blocks := make(map[string]*model.Block)
	for part := range parts {
		if part.Type == model.PartBlock {
			if b, ok := part.Resource.(*model.Block); ok {
				blocks[b.ID] = b
			}
		}
	}
	w.blocks = blocks
	defer func() { w.blocks = nil }()

	if w.originalContent == nil {
		return errors.New("openxml: writer requires original content for reconstruction")
	}

	// Open original ZIP
	origZR, err := zip.NewReader(bytes.NewReader(w.originalContent), int64(len(w.originalContent)))
	if err != nil {
		return fmt.Errorf("openxml: invalid original ZIP: %w", err)
	}

	// Parse container
	info, err := parseContainer(origZR, w.cfg)
	if err != nil {
		return err
	}

	// Create output ZIP
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)

	// If we have a skeleton store, use skeleton-based reconstruction
	if w.skeletonStore != nil {
		if err := w.skeletonStore.Flush(); err != nil {
			return fmt.Errorf("openxml: skeleton flush: %w", err)
		}
		if err := w.writeFromSkeleton(origZR, zw, &buf, info, blocks); err != nil {
			return err
		}
		_, err = w.Output.Write(buf.Bytes())
		return err
	}

	// Fallback: copy original unchanged
	if err := w.writeFromReparse(origZR, zw, &buf, blocks); err != nil {
		return err
	}
	_, err = w.Output.Write(buf.Bytes())
	return err
}

// writeFromSkeleton reconstructs translatable XML parts using the skeleton store.
// The skeleton stream contains part-boundary markers (skelPartStartPrefix/skelPartEndPrefix)
// that delimit each XML part's skeleton content. The writer collects each part's
// reconstructed bytes, then writes the output ZIP with replacements.
func (w *Writer) writeFromSkeleton(origZR *zip.Reader, zw *zip.Writer, buf *bytes.Buffer,
	info *containerInfo, blocks map[string]*model.Block) error {

	// Pre-load source styles.xml flags BEFORE the per-block renderBlock
	// loop below — adjustRPrForRunText / stripToggleMirrorChildren are
	// invoked during renderBlock and consult these module-level flags
	// to gate per-run bCs/iCs preservation against the docDefaults'
	// explicit-off form. Per ECMA-376-1 §17.7.5.5 (docDefaults) the
	// preCombined rPr a run inherits via the default chain comes from
	// docDefaults' rPrDefault; when docDefaults declares `<w:bCs val="0"/>`
	// and a run authors a bare-on `<w:bCs/>`, upstream's
	// RunParser.canBeSkipped (RunParser.java:240-250) refuses the strip
	// because pcrp.equals(rp) is false. Native must mirror that gate
	// at write time. Mirrors the WSO pre-pass below (lines ~1159+)
	// which sets the same vars for the post-pass; the docDefaults
	// detection itself is style-set-invariant, so both passes consult
	// the same single source-of-truth.
	for _, f := range origZR.File {
		if f.Name == "word/styles.xml" {
			data, err := readZipFile(f)
			if err == nil {
				currentDocDefaultsBCsExplicitOff = extractDocDefaultsToggleExplicitOff(data, "bCs")
				currentDocDefaultsICsExplicitOff = extractDocDefaultsToggleExplicitOff(data, "iCs")
			}
			break
		}
	}

	// Read all skeleton entries, splitting by part-boundary markers
	partContents := make(map[string][]byte)
	var currentPart string
	var currentBuf bytes.Buffer

	for {
		entry, err := w.skeletonStore.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("openxml: reading skeleton: %w", err)
		}

		switch entry.Type {
		case format.SkeletonText:
			if currentPart != "" {
				currentBuf.Write(entry.Data)
			}

		case format.SkeletonRef:
			refID := string(entry.Data)

			// Check for part-boundary markers
			if strings.HasPrefix(refID, skelPartStartPrefix) {
				currentPart = strings.TrimPrefix(refID, skelPartStartPrefix)
				currentBuf.Reset()
				continue
			}
			if strings.HasPrefix(refID, skelPartEndPrefix) {
				partPath := strings.TrimPrefix(refID, skelPartEndPrefix)
				if currentBuf.Len() > 0 {
					partContents[partPath] = append([]byte{}, currentBuf.Bytes()...)
				}
				currentPart = ""
				currentBuf.Reset()
				continue
			}

			// Regular block ref — render and write
			if currentPart != "" {
				if block, ok := blocks[refID]; ok {
					currentBuf.WriteString(w.renderBlock(block, info.docType))
				}
			}
		}
	}

	// Write output ZIP: replace translatable parts with skeleton-reconstructed content,
	// and substitute locale-variant media files (Bowrain AD-007).
	isDOCX := info.docType == docTypeDOCX

	// AllowWordStyleOptimisation post-pass: applied per WML part that
	// participates in style synthesis. Styles synthesised across all
	// parts are accumulated in a single map keyed by styleId, then
	// injected into word/styles.xml at the end. This mirrors Okapi's
	// single-IdGenerator-per-filter-invocation scope (see
	// WordStyleDefinitions.readWith line 114).
	var (
		synthesised             map[string]synthesisedStyle
		orderedIDs              []string
		idCounter               int
		existingIDs             map[string]bool
		defaultParagraphStyleID string
		pendingStyles           map[string]pendingStylesEntry
		// hasStylesPart records whether word/styles.xml exists in the
		// source ZIP. When it does NOT, upstream Okapi instantiates
		// `StyleDefinitions.Empty` for the missing styles part
		// (WordDocument.java:115-119, calling styleDefinitions(EMPTY)
		// → new StyleDefinitions.Empty()). The optimiser still runs —
		// inserting <w:pStyle> in pPr and stripping common rPr props
		// from runs — but Empty.place(…) is a no-op and Empty.placedId()
		// returns null (StyleDefinitions.java:53-59), so the inserted
		// pStyle carries an empty w:val and no <w:style> is appended to
		// a styles part (none exists to append to). Per ECMA-376-1
		// §17.7.4, when no styles part is present no style hierarchy
		// exists; the empty-val pStyle is upstream's surfaced form of
		// "synthesis ran but produced no id."
		hasStylesPart bool
	)
	if isDOCX && w.cfg.OptimiseWordStyles {
		synthesised = make(map[string]synthesisedStyle)
		// Pre-load existing styleIds AND the default paragraph styleId
		// from the source styles.xml so generated NF974E24F-* ids don't
		// collide AND so synthesised styles' basedOn (and id parent
		// fragment) point at the document's actual default paragraph
		// style — mirroring upstream WordStyleDefinitions.Ids.defaultBased
		// (WordStyleDefinitions.java:485-491).
		for _, f := range origZR.File {
			if f.Name == "word/styles.xml" {
				hasStylesPart = true
				data, err := readZipFile(f)
				if err == nil {
					existingIDs = extractExistingStyleIDs(data)
					defaultParagraphStyleID = extractDefaultParagraphStyleID(data)
					// Hand the rtl-bearing chain set off to the WSO
					// strip pass via the package-level
					// currentRTLChainStyles. Consumed by
					// stripToggleMirrorsFromCommon's `case "rtl":` —
					// see style_optimization.go for the 899.docx-vs-
					// 830-2.docx rationale.
					currentRTLChainStyles = extractRTLChainStyleIDs(data)
					// Detect docDefaults' explicit-off bCs/iCs so the
					// WSO common-rPr strip + the per-run sidecar's
					// stripToggleMirrorChildren preserve a run's bCs/
					// iCs when upstream's canBeSkipped (RunParser.java:
					// 240-250) would refuse the strip due to a
					// preCombined-vs-run value disagreement.
					currentDocDefaultsBCsExplicitOff = extractDocDefaultsToggleExplicitOff(data, "bCs")
					currentDocDefaultsICsExplicitOff = extractDocDefaultsToggleExplicitOff(data, "iCs")
					// Capture the default character style's bare-on
					// toggles so the WSO common-rPr lift can drop
					// duplicates per ECMA-376-1 §17.7.4 (default
					// character style applies implicitly to runs
					// without rStyle). document-style-definitions
					// fixture: Emphasis is the default character style
					// with `<w:i/>` — runs that author `<w:i/>` have
					// it dropped from the synthesised pStyle's rPr
					// because the implicit Emphasis chain already
					// supplies it.
					currentDefaultCharacterStyleToggles = extractDefaultCharacterStyleToggles(data)
					// Hand the source paragraph style set off to the
					// WSO matcher via currentSourceParagraphStyles.
					// Consumed by findMatchingStyle to mirror upstream
					// WordStyleDefinitions.Ids.parentBased
					// (WordStyleDefinitions.java:462-475) which walks
					// BOTH source and in-pass synthesised styles when
					// looking for a re-use candidate.
					currentSourceParagraphStyles = extractSourceParagraphStyles(data)
				}
				break
			}
		}
		if existingIDs == nil {
			existingIDs = make(map[string]bool)
		}
		// Reset the rtl-chain handoff after the WSO pass below so it
		// doesn't leak into another Writer's invocation. Tests that
		// invoke optimizeWMLPart directly leave currentRTLChainStyles
		// nil — preserving the pre-fix drop behaviour for fixtures
		// whose chain has no rtl-bearing styles.
		defer func() {
			currentRTLChainStyles = nil
			currentSourceParagraphStyles = nil
			currentDocDefaultsBCsExplicitOff = false
			currentDocDefaultsICsExplicitOff = false
			currentDefaultCharacterStyleToggles = nil
		}()
	}

	// wsoOptimised stashes the WSO-rewritten bytes for each
	// shouldOptimiseWMLPart part (keyed by ZIP entry name). It is
	// populated in the WSO PRE-PASS below, in canonical Okapi processing
	// order (mainPart first, then headers/footers/footnotes/endnotes/
	// comments — see ZipEntryComparator + reorderedPartPaths in
	// WordDocument.java:74-97). The shared idCounter ticks in that order
	// so the synthesised styleId sequence (NF974E24F-{parent}{N}) lines
	// up with the upstream filter's IdGenerator stream.
	wsoOptimised := map[string][]byte{}
	// Helper: post-process a WML XML payload (after lang strip + lang
	// retargeting, before recompression). WSO is applied in a separate
	// pre-pass — postNonWSOForName runs the field-marker reversal that
	// must always happen, then defers to wsoOptimised when the part has
	// already been WSO'd.
	postNonWSOForName := func(data []byte) []byte {
		if !isDOCX {
			return data
		}
		// Strip the field-rPr keep-empty marker the reader inserted to
		// prevent stripWMLSkippableElements from collapsing
		// `<w:rPr></w:rPr>` inside complex-field runs (see
		// fieldRPrKeepEmptyMarker in wml.go for the upstream-Okapi
		// citation). The marker only appears inside word/*.xml parts;
		// the contains-check is the cheap fast path.
		if bytes.Contains(data, []byte(fieldRPrKeepEmptyMarker)) {
			data = bytes.ReplaceAll(data, []byte(fieldRPrKeepEmptyMarker), nil)
		}
		// Reverse protectFieldPayloadFromStripping (see wml.go for the
		// upstream-Okapi citation): any element renamed with the keep
		// suffix is restored to its original WordprocessingML name now
		// that stripWMLSkippableElements has already run. The contains
		// check is the cheap fast path.
		if bytes.Contains(data, []byte(fieldKeepElementSuffix)) {
			for _, name := range fieldKeepElementNames {
				data = bytes.ReplaceAll(data, []byte("<w:"+name+fieldKeepElementSuffix), []byte("<w:"+name))
				data = bytes.ReplaceAll(data, []byte("</w:"+name+fieldKeepElementSuffix+">"), []byte("</w:"+name+">"))
			}
		}
		// Strip DrawingML run-property strippable attributes (lang,
		// altLang, dirty, smtClean, err, noProof) from any
		// <a:rPr>/<a:endParaRPr>/<a:defRPr> embedded inside a
		// <a:p> paragraph block in a captured <w:drawing> payload.
		// The WML reader writes drawings to the skeleton verbatim
		// (writeDrawingXMLToSkel in wml.go) so the strip has to
		// happen here, after skeleton reconstruction. Mirrors
		// upstream Okapi's StrippableAttributes.DrawingRunProperties
		// which is unconditionally applied during DrawingML block
		// parsing — see stripDMLRunPropertyAttrs for the citation
		// and scope rationale (list-style/table-style defaults are
		// preserved). Fixture: DrawingML_Test.docx <a:endParaRPr
		// lang="en-US" dirty="0">.
		if bytes.Contains(data, []byte("<a:p")) {
			data = []byte(stripDMLRunPropertyAttrs(string(data)))
		}
		// Pull a leading `<w:fldChar w:fldCharType="end"/>` run out
		// of its host paragraph and append it to the immediately
		// preceding paragraph when that previous paragraph carries
		// an open complex-field begin/separate without a matching
		// end. Mirrors upstream Okapi RunParser.parseComplexField
		// (RunParser.java:461-542) + BlockParser.parse (lines
		// 221-228): when a complex field straddles paragraph
		// boundaries, the field-end event is held in deferredEvents
		// alongside the intervening paragraph-ends and gets
		// re-anchored to whichever paragraph closes first inside
		// the field's run consumption — see the
		// `endComplexFieldParsing` branch invoked at line 476 +
		// `goesAfterAnotherRun` deferred-events check at line 612.
		// The visible end-state is that an isolated leading fld-end
		// in a paragraph migrates to the previous paragraph during
		// field consumption. Per ECMA-376-1 §17.16.5 (complex
		// fields) the fldChar elements bookend a single semantic
		// run regardless of paragraph layout. Fixtures: 830-1.docx,
		// 830-3.docx, 830-5.docx, 830-6.docx.
		if bytes.Contains(data, []byte(`fldCharType="end"`)) {
			data = pullLeadingFldCharEndIntoPrevParagraph(data)
		}
		// Same fld-end migration scoped to each
		// `<w:txbxContent>...</w:txbxContent>` region. The document-
		// level pass above doesn't reach paragraphs nested inside
		// textbox bodies because the outer paragraph's `</w:p>`
		// search short-circuits on the FIRST inner textbox `</w:p>`.
		// Per upstream Okapi the textbox body is parsed as its own
		// IBlock event stream so its complex-field deferredEvents
		// flush runs independently — see
		// pullLeadingFldCharEndIntoPrevParagraphInTxbxContents for the
		// full citation. Fixture: 1341-textbox-with-a-hyperlink.docx.
		if bytes.Contains(data, []byte("<w:txbxContent")) &&
			bytes.Contains(data, []byte(`fldCharType="end"`)) {
			data = pullLeadingFldCharEndIntoPrevParagraphInTxbxContents(data)
		}
		// Fuse a bare `<w:r><w:br .../></w:r>` envelope with the
		// IMMEDIATELY-following bare `<w:r><w:t ...>…</w:t></w:r>`
		// envelope into a single `<w:r>` carrying both the br and
		// the t children. Both source envelopes must lack `<w:rPr>`
		// so the fuse is rPr-equivalent. Mirrors upstream Okapi
		// RunMerger.add → canMergeWith (RunMerger.java:83-95 +
		// canRunPropertiesBeMerged at 156-229): adjacent source
		// `<w:r>` envelopes with matching (here: empty)
		// RunProperties merge into one RunBuilder; the resulting
		// `<w:r>` carries the br Markup body chunk and the t
		// RunText body chunk side by side. Per ECMA-376-1
		// §17.3.2.1 (CT_R) a single `<w:r>` may carry both
		// `<w:br/>` and `<w:t>` children.
		//
		// The fuse runs in postNonWSOForName (after skeleton
		// reconstruction + WSO) so it sees the post-WSO wire shape
		// where the rPr-less envelopes have already been emitted.
		// The skeleton path's `runToXML` (wml.go) emits each
		// source `<w:r>` envelope verbatim, so the source's
		// `<w:r><w:br type="page"/></w:r><w:r><w:t> </w:t></w:r>`
		// pattern arrives here intact when the paragraph is
		// non-translatable (skeleton-only). apissue.docx is the
		// canonical fixture: page-break + space text run pairs in
		// otherwise-empty paragraphs that bridge fuses but the
		// per-run skeleton emit splits.
		if bytes.Contains(data, []byte(`<w:br`)) {
			data = fuseBareBrAndTextRuns(data)
		}
		// Fuse a bare `<w:r><w:fldChar fldCharType="end"/></w:r>`
		// envelope with the IMMEDIATELY-following bare
		// `<w:r><w:t ...>…</w:t></w:r>` envelope into a single `<w:r>`
		// carrying both children. See fuseBareFldCharEndAndTextRuns
		// for the upstream Okapi RunMerger citation and the 830-4.docx
		// fixture rationale. Both source envelopes must lack `<w:rPr>`
		// so the fuse is rPr-equivalent.
		if bytes.Contains(data, []byte(`fldCharType="end"`)) {
			data = fuseBareFldCharEndAndTextRuns(data)
		}
		// Fuse an alternation of bare `<w:r><w:t>…</w:t></w:r>` and
		// `<w:r><w:ptab/></w:r>` envelopes into a single `<w:r>`
		// envelope carrying all the t and ptab children side by side.
		// All source envelopes must lack `<w:rPr>` so the fuse is
		// rPr-equivalent. See fuseBareTextAndPTabRuns for the upstream
		// Okapi RunMerger citation and the
		// OpenXML_text_reference_v1_2.docx (header2.xml) fixture
		// rationale.
		if bytes.Contains(data, []byte(`<w:ptab`)) {
			data = fuseBareTextAndPTabRuns(data)
		}
		// Fuse the `<w:r>` envelope carrying the comment-body marker
		// (`<w:annotationRef/>` with CommentReference rStyle) with the
		// immediately-following same-rPr text run. See
		// fuseSameRPrAnnotationRefAndTextRuns for the upstream Okapi
		// RunMerger citation and the OpenXML_text_reference_v1_2.docx
		// (comments.xml) fixture rationale.
		if bytes.Contains(data, []byte(`<w:annotationRef/>`)) {
			data = fuseSameRPrAnnotationRefAndTextRuns(data)
		}
		return data
	}
	postWML := func(name string, data []byte) []byte {
		data = postNonWSOForName(data)
		if isDOCX && w.cfg.OptimiseWordStyles && shouldOptimiseWMLPart(name) {
			if optimised, ok := wsoOptimised[name]; ok {
				data = optimised
			}
		}
		// Strip `<w:rPr>` from a `<w:r ...><w:rPr>X</w:rPr><w:fldChar
		// w:fldCharType="begin"/></w:r>` envelope when the IMMEDIATELY-
		// following `<w:r ...><w:rPr>Y</w:rPr><w:instrText>…</w:instrText>
		// </w:r>` carries an rPr Y that contains X verbatim. Mirrors
		// upstream Okapi RunMerger + RunProperties.minified() collapse of
		// the field's per-chunk rPr to the superset rPr on the instrText
		// carrier. Runs AFTER WSO so it doesn't perturb style synthesis
		// (WSO inspects per-run rPr to choose synth styleIds). See
		// stripFldCharBeginRunRPrWhenInheritedFromFollowingRun for the
		// citation and the Practice2.docx (footer2.xml / footer3.xml)
		// fixture rationale.
		if isDOCX && bytes.Contains(data, []byte(`fldCharType="begin"`)) {
			data = stripFldCharBeginRunRPrWhenInheritedFromFollowingRun(data)
		}
		// Fuse a bare `<w:r><w:pict>…</w:pict></w:r>` envelope with
		// the IMMEDIATELY-following bare-rPr text run envelope. Runs
		// AFTER WSO so the per-run rPr/pict structure WSO inspects to
		// choose synth styleIds is the un-fused source shape — fusing
		// before WSO would shift the IdGenerator counter and break the
		// styleId-sequence parity with upstream. See
		// fuseBarePictAndRPrTextRuns for the upstream Okapi RunMerger
		// citation and the Hangs.docx (header1.xml) fixture rationale.
		if isDOCX && bytes.Contains(data, []byte(`<w:r><w:pict>`)) {
			data = fuseBarePictAndRPrTextRuns(data)
		}
		// Insert an empty `<w:r></w:r>` placeholder run between an
		// `<mc:AlternateContent>` host run's closing
		// `</mc:AlternateContent></w:r>` and the IMMEDIATELY-following
		// bare `<w:r><w:br[...]/>...` run envelope. Mirrors upstream
		// Okapi RunBuilder.flushRunStart/flushRunEnd's post-image flush
		// cycle that emits an empty placeholder run on the boundary
		// between a complex image run (drawing/pict/AlternateContent)
		// and the next br-bearing run. Runs AFTER WSO (which strips the
		// post-image run's rPr when its members are inherited from the
		// surrounding paragraph style), so the regex sees the rPr-less
		// `<w:r><w:br/>...` envelope. See
		// emitEmptyRunAfterAltContentPostImageBoundary for the citation
		// and the graphicdata.docx fixture rationale.
		if isDOCX && bytes.Contains(data, []byte(`</mc:AlternateContent></w:r><w:r><w:br`)) {
			data = emitEmptyRunAfterAltContentPostImageBoundary(data)
		}
		return data
	}

	// WSO pre-pass: visit WSO-eligible parts in the order Okapi's
	// ZipEntryComparator produces (mainPart first; see Okapi
	// WordDocument.java:74-90 / ZipEntryComparator.java:39-44). For each
	// part, fetch the same bytes the file-emit loop would feed to postWML
	// (skeleton-reconstructed content if present, otherwise the raw ZIP
	// content with strip-lang and lang-retarget applied), apply the
	// non-WSO post-processing, and run optimizeWMLPart with the SHARED
	// idCounter so styleId sequence numbers stay in lockstep with the
	// upstream IdGenerator stream.
	if isDOCX && w.cfg.OptimiseWordStyles {
		wsoNames := wsoPartOrder(origZR, info)
		for _, name := range wsoNames {
			f := zipFileByName(origZR, name)
			if f == nil {
				continue
			}
			var data []byte
			if content, ok := partContents[name]; ok && len(content) > 0 {
				data = content
				if shouldStripWMLLang(name) {
					data = stripWMLSkippableElements(data)
				}
				if shouldRewriteWMLLangVal(name) {
					data = rewriteWMLLangVal(data, w.sourceLocale, w.Locale)
				}
			} else if shouldStripWMLLang(name) || shouldRewriteWMLLangVal(name) {
				raw, err := readZipFile(f)
				if err != nil {
					continue
				}
				data = raw
				if shouldStripWMLLang(name) {
					data = stripWMLSkippableElements(data)
				}
				if shouldRewriteWMLLangVal(name) {
					data = rewriteWMLLangVal(data, w.sourceLocale, w.Locale)
				}
			} else {
				continue
			}
			data = postNonWSOForName(data)
			partStrict := bytes.Contains(data, []byte(wmlStrictNamespace))
			// Read the SOURCE bytes (pre-read, pre-strip) so optimizeWMLPart
			// can bypass paragraphs whose ENTIRE content was inside
			// tracked-revision wrappers (<w:ins>/<w:del>/<w:moveTo>/
			// <w:moveFrom>) before the auto-accept-revisions unwrap at
			// READ time removed the wrappers. See the
			// optimizeWMLPartWithSource docstring + 847-3.docx fixture.
			// readZipFile failure is non-fatal: fall back to source-less
			// WSO (preserves pre-fix behaviour).
			var srcXML []byte
			if rawSrc, err := readZipFile(f); err == nil {
				srcXML = rawSrc
			}
			data = optimizeWMLPartWithSource(data, srcXML, existingIDs, defaultParagraphStyleID, hasStylesPart, partStrict, &idCounter, synthesised, &orderedIDs)
			wsoOptimised[name] = data
		}
	}

	for _, f := range origZR.File {
		if content, ok := partContents[f.Name]; ok && len(content) > 0 {
			// Replace with skeleton-reconstructed content
			if isDOCX && shouldStripWMLLang(f.Name) {
				content = stripWMLSkippableElements(content)
			}
			if isDOCX && shouldRewriteWMLLangVal(f.Name) {
				content = rewriteWMLLangVal(content, w.sourceLocale, w.Locale)
			}
			content = postWML(f.Name, content)
			fh := f.FileHeader
			fh.Method = zip.Deflate
			// Clear data descriptor fields to avoid checksum issues
			fh.CompressedSize64 = 0
			fh.UncompressedSize64 = 0
			fh.CRC32 = 0
			fw, err := zw.CreateHeader(&fh)
			if err != nil {
				return err
			}
			if _, err := fw.Write(content); err != nil {
				return err
			}
		} else if replacement, ok := w.mediaReplacements[f.Name]; ok {
			// Replace with locale-variant media (Bowrain AD-007).
			fh := f.FileHeader
			fh.Method = zip.Deflate
			fh.CompressedSize64 = 0
			fh.UncompressedSize64 = 0
			fh.CRC32 = 0
			fw, err := zw.CreateHeader(&fh)
			if err != nil {
				return err
			}
			if _, err := fw.Write(replacement); err != nil {
				return err
			}
		} else if isDOCX && (shouldStripWMLLang(f.Name) || shouldRewriteWMLLangVal(f.Name)) {
			// Pass-through WordprocessingML part (e.g. word/styles.xml,
			// word/settings.xml) that needs okapi-style lang/bidiVisual
			// stripping and/or <w:lang>/<w:themeFontLang> w:val
			// retargeting. Read, transform, re-emit with a recompressed
			// header.
			data, err := readZipFile(f)
			if err != nil {
				return err
			}
			if shouldStripWMLLang(f.Name) {
				data = stripWMLSkippableElements(data)
			}
			if shouldRewriteWMLLangVal(f.Name) {
				data = rewriteWMLLangVal(data, w.sourceLocale, w.Locale)
			}
			data = postWML(f.Name, data)
			// Defer styles.xml emission until all paragraph parts have
			// been visited so we know the synthesised set. We instead
			// stash the post-strip bytes in a sentinel map that's
			// flushed after the loop.
			if w.cfg.OptimiseWordStyles && isDOCX && f.Name == "word/styles.xml" {
				if pendingStyles == nil {
					pendingStyles = map[string]pendingStylesEntry{}
				}
				pendingStyles[f.Name] = pendingStylesEntry{header: f.FileHeader, data: data}
				continue
			}
			fh := f.FileHeader
			fh.Method = zip.Deflate
			fh.CompressedSize64 = 0
			fh.UncompressedSize64 = 0
			fh.CRC32 = 0
			fw, err := zw.CreateHeader(&fh)
			if err != nil {
				return err
			}
			if _, err := fw.Write(data); err != nil {
				return err
			}
		} else {
			// Copy unchanged — use raw copy to preserve CRC/data descriptors
			if err := zw.Copy(f); err != nil {
				return err
			}
		}
	}

	// Late-emit styles.xml with synthesised <w:style> entries appended.
	if w.cfg.OptimiseWordStyles && isDOCX && pendingStyles != nil {
		for name, ps := range pendingStyles {
			data := ps.data
			if name == "word/styles.xml" && len(orderedIDs) > 0 {
				data = injectSynthesisedStyles(data, synthesised, orderedIDs)
			}
			fh := ps.header
			fh.Method = zip.Deflate
			fh.CompressedSize64 = 0
			fh.UncompressedSize64 = 0
			fh.CRC32 = 0
			fw, err := zw.CreateHeader(&fh)
			if err != nil {
				return err
			}
			if _, err := fw.Write(data); err != nil {
				return err
			}
		}
	}

	return zw.Close()
}

// pendingStylesEntry holds a WML part (currently only styles.xml) that
// must be deferred until after all other parts have been post-processed
// — the synthesised-style set isn't complete until then.
type pendingStylesEntry struct {
	header zip.FileHeader
	data   []byte
}

// wsoPartOrder returns the ordered list of WSO-eligible part paths,
// in the canonical order Okapi's ZipEntryComparator produces (see
// WordDocument.java:74-97 / ZipEntryComparator.java:39-44 — main
// document part comes first; the rest in original ZIP order).
//
// The shared idCounter must increment in this order so that the
// synthesised styleIds (NF974E24F-{parent}{N}) match upstream's
// IdGenerator stream — otherwise headers/footers that come before
// document.xml in raw ZIP order would consume the low sequence numbers
// the upstream filter assigns to document.xml first.
func wsoPartOrder(origZR *zip.Reader, info *containerInfo) []string {
	var out []string
	seen := make(map[string]struct{})
	// Main part first (Okapi reorderedPartPaths places mainPartPath
	// after relsPath, which is not WSO-eligible — so mainPart is the
	// first WSO target after the early non-WSO entries).
	if info.mainDocumentPart != "" && shouldOptimiseWMLPart(info.mainDocumentPart) {
		out = append(out, info.mainDocumentPart)
		seen[info.mainDocumentPart] = struct{}{}
	}
	// Then everything else in ZIP order.
	for _, f := range origZR.File {
		if _, dup := seen[f.Name]; dup {
			continue
		}
		if shouldOptimiseWMLPart(f.Name) {
			out = append(out, f.Name)
			seen[f.Name] = struct{}{}
		}
	}
	return out
}

// shouldOptimiseWMLPart reports whether a WML XML part participates in
// AllowWordStyleOptimisation (paragraphs are walked, common rPr is
// extracted into synthesised paragraph styles). Mirrors the set of
// parts Okapi's openxml filter routes through WordPart processing.
func shouldOptimiseWMLPart(name string) bool {
	if !strings.HasPrefix(name, "word/") || !strings.HasSuffix(name, ".xml") {
		return false
	}
	switch {
	case name == "word/document.xml",
		name == "word/footnotes.xml",
		name == "word/endnotes.xml",
		name == "word/comments.xml":
		return true
	case strings.HasPrefix(name, "word/header") && strings.HasSuffix(name, ".xml"),
		strings.HasPrefix(name, "word/footer") && strings.HasSuffix(name, ".xml"):
		return true
	}
	return false
}

// writeFromReparse copies the original ZIP, substituting locale-variant media (Bowrain AD-007).
func (w *Writer) writeFromReparse(origZR *zip.Reader, zw *zip.Writer, buf *bytes.Buffer,
	blocks map[string]*model.Block) error {

	for _, f := range origZR.File {
		if replacement, ok := w.mediaReplacements[f.Name]; ok {
			fh := f.FileHeader
			fh.Method = zip.Deflate
			fh.CompressedSize64 = 0
			fh.UncompressedSize64 = 0
			fh.CRC32 = 0
			fw, err := zw.CreateHeader(&fh)
			if err != nil {
				return err
			}
			if _, err := fw.Write(replacement); err != nil {
				return err
			}
		} else {
			if err := zw.Copy(f); err != nil {
				return err
			}
		}
	}

	return zw.Close()
}

// renderBlock converts a block's content back to the appropriate XML dialect.
func (w *Writer) renderBlock(block *model.Block, dt docType) string {
	runs := w.preferredRuns(block)
	if runs == nil {
		return ""
	}


	// Core properties and table column names are plain text (no XML wrapping needed).
	if block.Type == "property" || block.Type == "table-column" {
		return xmlEscapeAttr(model.FlattenRuns(runs))
	}

	// Chart and diagram parts inside a DOCX are DrawingML, not WML —
	// they declare the drawingml/2006/main namespace under the `a:`
	// prefix and use <a:r>/<a:t>. Routing them through renderWMLBlock
	// would emit <w:r>/<w:t>, but the `w:` prefix is undeclared in the
	// chart/diagram XML and the round-trip output would mis-bind it
	// (see TranchartAmpersand.docx / Transmart_art.docx golds, which
	// produce <a:r>/<a:t> inside chart and diagram parts). Keying on
	// partPath keeps the wml writer for body/header/footer parts and
	// switches to the dml writer for chart and diagram parts.
	if dt == docTypeDOCX {
		if pp := block.Properties["partPath"]; isChartPartPath(pp) || isDiagramDataPartPath(pp) {
			return w.renderDMLBlock(runs)
		}
	}

	// Per-source-run rPr preservation (#592). The reader stashes the
	// per-paragraph common rPr children under
	// openxmlSourceRPrAnnotationKey when the source had at least one
	// non-toggle rPr child; the writer prepends this XML to every
	// emitted <w:r>'s <w:rPr>. The WSO post-pass (style_optimization.go)
	// then lifts the redundant rPr into a synthesised paragraph style
	// when the optimisation conditions hold.
	sourceRPr := blockSourceRPrXML(block)

	// Per-text-run rPr sidecar (Phase 2 of the per-run rPr work — see
	// PARITY_NOTES.md "1083-*" cluster). When non-empty the writer
	// prefers each text-run's specific rPr over the paragraph-common
	// sourceRPr, mirroring upstream Okapi RunBuilder.java lines 73-188
	// + RunMerger.java lines 156-229 (per ECMA-376-1 §17.3.2): every
	// source run keeps its full rPr verbatim. When all fragments are
	// identical (or after dedupe-on-collapse), output matches the
	// previous common-rPr path. When heterogeneous, per-run divergences
	// (e.g. rStyle on hyperlink display text) are preserved.
	perRunRPr := blockPerRunRPrFragments(block)
	perRunSrcStart := blockPerRunSrcRunStartFlags(block)
	perRunInFieldDisplay := blockPerRunInFieldDisplayFlags(block)
	perRunSourceHadRPr := blockPerRunSourceHadRPrFlags(block)
	// Cross-paragraph field straddle marker set by wml.go's
	// flushPendingFieldBlock — see "openxml:field-straddle" property
	// comment there. When true the writer mirrors upstream Okapi
	// BlockTextUnitWriter.flush(Run.Markup) lines 238-251: an empty
	// `<w:r/>` placeholder is emitted before every TypeBreak Ph that
	// began a fresh source `<w:r>`, capturing the artifact of the
	// open-then-close `<w:r>` cycle that Okapi's flush performs when
	// the first MarkupComponent of the outer-Run's body chunk is a
	// `<w:br>` Component.Start.
	fieldStraddle := false
	if block.Properties != nil && block.Properties["openxml:field-straddle"] == "true" {
		fieldStraddle = true
	}

	switch dt {
	case docTypeDOCX:
		return w.renderWMLBlock(runs, sourceRPr, perRunRPr, perRunSrcStart, perRunInFieldDisplay, perRunSourceHadRPr, fieldStraddle)
	case docTypePPTX:
		return w.renderDMLBlock(runs)
	case docTypeXLSX:
		return w.renderSMLBlock(runs, block)
	default:
		return w.renderWMLBlock(runs, sourceRPr, perRunRPr, perRunSrcStart, perRunInFieldDisplay, perRunSourceHadRPr, fieldStraddle)
	}
}

// blockSourceRPrXML extracts the per-paragraph common rPr children
// XML from the block annotation populated by the WML reader (#592).
// Returns the empty string when no annotation is present (the writer
// falls through to its toggle-only rPr path).
func blockSourceRPrXML(block *model.Block) string {
	if block == nil || block.Annotations == nil {
		return ""
	}
	a, ok := block.Annotations[openxmlSourceRPrAnnotationKey]
	if !ok {
		return ""
	}
	g, ok := a.(*model.GenericAnnotation)
	if !ok || g == nil || g.Fields == nil {
		return ""
	}
	v, ok := g.Fields["xml"].(string)
	if !ok {
		return ""
	}
	return v
}

// blockPerRunRPrFragments extracts the per-text-run rPr children XML
// fragments from the block annotation populated by the WML reader
// (Phase 1 — see source_rpr.go openxmlPerRunRPrAnnotationKey).
//
// The slice has one entry per text-bearing source run BEFORE
// mergeRuns coalescing — adjacent identical fragments correspond to
// runs that mergeRuns combined into a single model TextRun. The
// writer dedupes adjacent identical fragments at emit time so the
// remaining slice aligns 1:1 with the post-merge model TextRun
// stream emitted by renderWMLBlock.
//
// Returns nil when no annotation is present (the writer falls
// through to the paragraph-common sourceRPr path).
func blockPerRunRPrFragments(block *model.Block) []string {
	if block == nil || block.Annotations == nil {
		return nil
	}
	a, ok := block.Annotations[openxmlPerRunRPrAnnotationKey]
	if !ok {
		return nil
	}
	g, ok := a.(*model.GenericAnnotation)
	if !ok || g == nil || g.Fields == nil {
		return nil
	}
	v, ok := g.Fields["fragments"].([]string)
	if !ok {
		return nil
	}
	// The per-run sidecar is returned raw here. The text-aware
	// bCs/iCs strip — which mirrors upstream Okapi RunParser.java
	// :219-229 (strip bCs/iCs/szCs when runFonts has no detected
	// complex-script content categories) — is applied per text
	// run inside renderWMLBlock where the post-pseudo run text is
	// known. ContentCategoriesDetection (upstream
	// ContentCategoriesDetection.java:134-138) classifies runText
	// against the complex-script Unicode block (U+0590..U+074F
	// plus a few extensions; see containsComplexScriptText for
	// the full inventory derived from ECMA-376-1 §17.3.2.16
	// (bCs) and §17.3.2.17 (iCs)).
	//
	// dedupeAdjacent is NOT applied here. The reader's mergeRuns
	// (wml.go) already coalesces adjacent text runs whose toggle +
	// non-toggle rPr are fully equal, so the sidecar arrives with
	// one entry per surviving model text-run. Adjacent fragments
	// that happen to be byte-equal (e.g. two HYPERLINK display runs
	// separated by an intervening fldChar / PcClose) must NOT be
	// collapsed — they belong to distinct model runs and the writer
	// needs the slot at each text-run index. Prior code ran
	// dedupeAdjacent and triggered the alignment guard in
	// renderWMLBlock (sidecar suppressed → fall back to sourceRPr),
	// which dropped rStyle from hyperlink-internal runs in
	// 830-7.docx, external_hyperlink.docx, etc.
	return v
}

// blockPerRunSrcRunStartFlags extracts the per-text-run "starts new
// source <w:r>" boolean sidecar from the block annotation populated
// by the WML reader. See source_rpr.go
// openxmlPerRunSrcRunStartAnnotationKey for the contract.
//
// Returns nil when the annotation is absent.
func blockPerRunSrcRunStartFlags(block *model.Block) []bool {
	if block == nil || block.Annotations == nil {
		return nil
	}
	a, ok := block.Annotations[openxmlPerRunSrcRunStartAnnotationKey]
	if !ok {
		return nil
	}
	g, ok := a.(*model.GenericAnnotation)
	if !ok || g == nil || g.Fields == nil {
		return nil
	}
	v, ok := g.Fields["flags"].([]bool)
	if !ok {
		return nil
	}
	return v
}

// blockPerRunInFieldDisplayFlags extracts the per-text-run "inside a
// complex field's display text" boolean sidecar from the block
// annotation populated by the WML reader. See source_rpr.go
// openxmlPerRunInFieldDisplayAnnotationKey for the contract.
//
// Returns nil when the annotation is absent.
func blockPerRunInFieldDisplayFlags(block *model.Block) []bool {
	if block == nil || block.Annotations == nil {
		return nil
	}
	a, ok := block.Annotations[openxmlPerRunInFieldDisplayAnnotationKey]
	if !ok {
		return nil
	}
	g, ok := a.(*model.GenericAnnotation)
	if !ok || g == nil || g.Fields == nil {
		return nil
	}
	v, ok := g.Fields["flags"].([]bool)
	if !ok {
		return nil
	}
	return v
}

// blockPerRunSourceHadRPrFlags extracts the per-text-run "source had
// rPr" boolean sidecar from the block annotation populated by the WML
// reader. See source_rpr.go openxmlPerRunSourceHadRPrAnnotationKey
// for the contract.
//
// Returns nil when the annotation is absent.
func blockPerRunSourceHadRPrFlags(block *model.Block) []bool {
	if block == nil || block.Annotations == nil {
		return nil
	}
	a, ok := block.Annotations[openxmlPerRunSourceHadRPrAnnotationKey]
	if !ok {
		return nil
	}
	g, ok := a.(*model.GenericAnnotation)
	if !ok || g == nil || g.Fields == nil {
		return nil
	}
	v, ok := g.Fields["flags"].([]bool)
	if !ok {
		return nil
	}
	return v
}

// stripToggleMirrorChildren removes <w:bCs/> and <w:iCs/> elements
// (with or without attributes) from an rPr children-only XML
// fragment. These are complex-script toggle mirrors that upstream
// Okapi strips at parse time when the run text has NO detected
// complex-script content categories
// (okapi/filters/openxml/RunParser.java:219-229 — when
// !runFonts.containsDetectedComplexScriptContentCategories the
// RUN_PROPERTY_COMPLEX_SCRIPT_BOLD/ITALICS/FONT_SIZE elements are
// added to skippableProperties and dropped from the run's rPr).
//
// Callers in the per-run sidecar path must gate this on the
// post-pseudo run text — see containsComplexScriptText. When the
// text contains complex-script characters, bCs/iCs must be preserved
// per ECMA-376-1 §17.3.2.16 (CT_OnOff bCs — complex-script bold)
// and §17.3.2.17 (CT_OnOff iCs — complex-script italics): each is
// the independent toggle for the complex-script side of the run's
// font triple, and stripping them when text is complex-script-
// bearing would drop legitimate formatting (cluster 1200-*).
//
// Paired-toggle preservation: an explicit-off
// `<w:bCs w:val="false"/>` is KEPT when the SAME fragment also carries
// an explicit-off `<w:b w:val="false"/>` (and likewise iCs ↔ i). This
// mirrors upstream Okapi RunParser.canBeSkipped (RunParser.java:240-
// 250): bCs is skippable only when preCombined and runProperties have
// EQUAL bCs values. When the inherited style chain has `<w:bCs/>`
// (bare-on, the natural way pStyles like Heading2 set the toggle) and
// the run has `<w:bCs w:val="false"/>`, the values disagree and the
// strip cannot fire. Native lacks the preCombined view at write time
// but DOES see the b↔bCs / i↔iCs explicit-off pairing in the rPr —
// authoring tools emit both halves of the pair only when the
// inherited chain has them ON, so the pairing is a faithful proxy.
// Fixture 1311.docx (Heading2 → bCs/) is the canonical case.
func stripToggleMirrorChildren(s string) string {
	if s == "" {
		return s
	}
	// Paired-toggle preservation: keep bCs/iCs when the strip cannot
	// fire per upstream Okapi RunParser.canBeSkipped (RunParser.java:
	// 240-250), which requires `preCombined.bCs.equals(run.bCs)` for
	// the strip to apply. Two known cases of preCombined-vs-run
	// disagreement:
	//
	//   (a) Explicit-off pair authored on the run rPr:
	//       `<w:b w:val="0"/><w:bCs w:val="0"/>` — paragraph clears
	//       an inherited bold AND its complex-script mirror
	//       (1311.docx Heading2 / highlights_block.docx Caption).
	//   (b) Bare-on pair authored on the run rPr against a
	//       docDefaults that clears the toggle:
	//       `<w:b/><w:bCs/>` + docDefaults `<w:bCs w:val="0"/>`
	//       (992.docx footer). preCombined's bCs is val=0 and run's
	//       bCs is bare-on (val implicit true) → disagree → no strip.
	//
	// Native consults two signals:
	//   - The pairing in the rPr itself (always available).
	//   - currentDocDefaultsBCsExplicitOff / *ICsExplicitOff set by
	//     the writer's WSO pre-pass from styles.xml.
	//
	// Bare-on pair only preserves when docDefaults has the explicit-
	// off form — otherwise the strip is correct (large-attribute.docx
	// has no docDefaults bCs, so its bare-on `<w:b/><w:bCs/>` runs
	// hit upstream's `else { v = true; }` branch at RunParser.java:
	// 247-248 and ARE stripped).
	keepBCs := stripBCsBlockedByPairing(s, "b", "bCs", currentDocDefaultsBCsExplicitOff)
	keepICs := stripBCsBlockedByPairing(s, "i", "iCs", currentDocDefaultsICsExplicitOff)
	if !keepBCs {
		s = stripWMLElement(s, "bCs")
	}
	if !keepICs {
		s = stripWMLElement(s, "iCs")
	}
	return s
}

// stripBCsBlockedByPairing reports whether the rPr fragment s should
// KEEP its bCs/iCs (i.e. the strip is blocked) given:
//
//   - latinName: "b" / "i" — the mirror partner element
//   - mirrorName: "bCs" / "iCs" — the toggle mirror to gate
//   - docDefaultsExplicitOff: whether docDefaults rPr authors
//     `<w:NAME w:val="0"/>` for the mirror
//
// Rules:
//   - Explicit-off pair (`<w:b w:val="0"/>` + `<w:bCs w:val="0"/>`)
//     → preserve.
//   - Bare-on pair (`<w:b/>` + `<w:bCs/>`) AND docDefaults has the
//     explicit-off mirror → preserve.
//   - Otherwise → strip.
//
// Per ECMA-376-1 §17.3.2.16 (CT_OnOff bCs) and §17.3.2.17 (CT_OnOff
// iCs).
func stripBCsBlockedByPairing(s, latinName, mirrorName string, docDefaultsExplicitOff bool) bool {
	hasMirror := hasToggleMirrorPartner(s, mirrorName)
	if !hasMirror {
		return false
	}
	// Branch 1: explicit-off pair on the same rPr. Mirrors the case
	// where the source authored `<w:b w:val="0"/><w:bCs w:val="0"/>`
	// to clear an inherited bold's complex-script half (1311.docx
	// Heading2 / highlights_block.docx Caption).
	hasLatin := hasToggleMirrorPartner(s, latinName)
	if hasLatin && hasExplicitOffElement(s, latinName) && hasExplicitOffElement(s, mirrorName) {
		return true
	}
	// Branch 2: docDefaults declares an explicit-off form of the
	// mirror, AND the run authors a bare-on (or any non-explicit-off)
	// form. preCombined's value (val=0) disagrees with run's value
	// (val implicit true) so upstream's canBeSkipped at
	// RunParser.java:244-245 returns false. The bare-on b mirror
	// partner is NOT required for this branch — the disagreement is
	// solely between preCombined.bCs and run.bCs; the bold half can
	// be present, absent, or asserted via a separate <w:b/> outside
	// the rPr-children fragment (the writer's toggle path emits
	// <w:b/> from props.bold rather than from the captured
	// rPrChildren XML).
	if docDefaultsExplicitOff && !hasExplicitOffElement(s, mirrorName) {
		return true
	}
	return false
}

// hasToggleMirrorPartner reports whether the rPr fragment s contains
// a `<w:NAME...>` element (the Latin half of a b↔bCs / i↔iCs pair) in
// any form — bare-on, explicit-on, or explicit-off. Used by
// stripToggleMirrorChildren to detect the paired-toggle preservation
// signal. The trailing space after the name in the search prefix
// enforces a strict element-name boundary so `<w:bCs/>` does not
// match a search for `<w:b/>` (the prefix is `<w:b ` or `<w:b/`,
// neither of which is a prefix of `<w:bCs`).
func hasToggleMirrorPartner(s, name string) bool {
	openWithSpace := "<w:" + name + " "
	openSelfClose := "<w:" + name + "/"
	openWithGT := "<w:" + name + ">"
	if strings.Contains(s, openWithSpace) {
		return true
	}
	if strings.Contains(s, openSelfClose) {
		return true
	}
	if strings.Contains(s, openWithGT) {
		return true
	}
	return false
}

// hasExplicitOffElement reports whether the rPr-children fragment s
// contains a `<w:NAME w:val="0"/>` / `"false"` / `"off"` element. Used
// by stripToggleMirrorChildren to detect the paired-toggle preservation
// signal (b ↔ bCs / i ↔ iCs).
func hasExplicitOffElement(s, name string) bool {
	prefix := "<w:" + name + " "
	idx := 0
	for {
		i := strings.Index(s[idx:], prefix)
		if i < 0 {
			return false
		}
		// Find element end (`/>` or `>`).
		start := idx + i
		end := strings.IndexAny(s[start:], ">")
		if end < 0 {
			return false
		}
		head := s[start : start+end]
		// Skip if this is a longer name (e.g. "<w:bCs " when looking for "<w:b ").
		// The trailing space after the name in `prefix` already enforces a
		// boundary, so this is fine — but we also want to skip `<w:bCs ` when
		// looking for `<w:b ` is not possible because prefix has trailing
		// space. Good.
		if strings.Contains(head, ` w:val="0"`) ||
			strings.Contains(head, ` w:val="false"`) ||
			strings.Contains(head, ` w:val="off"`) {
			return true
		}
		idx = start + end + 1
		if idx >= len(s) {
			return false
		}
	}
}

// containsComplexScriptText reports whether s contains any Unicode
// code point that upstream Okapi's
// ContentCategoriesDetection.Default classifies as a complex-script
// content category (ContentCategoriesDetection.java:71-74,
// 134-138). The ranges mirror Microsoft's "Office Open XML Themes,
// Schemes and Fonts" guidance for the complex-script font slot
// referenced by ECMA-376-1 §17.3.2.16 / .17 / .27.
//
// When this returns false for the post-pseudo run text, upstream
// Okapi strips the complex-script run-property toggle mirrors
// (bCs/iCs/szCs) at parse time — so the writer must do the same
// on the per-run sidecar to round-trip byte-equally with the
// reference.
//
// References:
//   - okapi/filters/openxml/ContentCategoriesDetection.java:71-74,
//     134-138 — COMPLEX_SCRIPT_CHARACTERS Pattern + detection rule.
//   - okapi/filters/openxml/RunParser.java:219-229 — skip
//     bCs/iCs/szCs when no detected CS categories.
//   - ECMA-376-1 §17.3.2.16 (bCs), §17.3.2.17 (iCs).
func containsComplexScriptText(s string) bool {
	for _, r := range s {
		switch {
		case r >= 0x0590 && r <= 0x074F: // Hebrew, Arabic, Syriac, …
			return true
		case r >= 0x0780 && r <= 0x07BF: // Thaana
			return true
		case r >= 0x0900 && r <= 0x109F: // Devanagari … Myanmar
			return true
		case r >= 0x1780 && r <= 0x18AF: // Khmer … Mongolian
			return true
		case r >= 0x200C && r <= 0x200F: // ZWJ / ZWNJ / LRM / RLM
			return true
		case r >= 0x202A && r <= 0x202F: // bidi formatting + NNBSP
			return true
		case r >= 0x2670 && r <= 0x2671: // misc symbols
			return true
		case r >= 0xFB1D && r <= 0xFB4F: // Hebrew presentation forms
			return true
		}
	}
	return false
}

// adjustRPrForRunText returns the per-run rPr fragment with bCs/iCs
// removed when the run text has no complex-script characters. This
// mirrors upstream Okapi's parse-time strip
// (okapi/filters/openxml/RunParser.java:219-229) which removes the
// complex-script toggle mirrors when
// !runFonts.containsDetectedComplexScriptContentCategories. We
// apply it at write time because the post-pseudo run text is what
// upstream's ContentCategoriesDetection runs against
// (ContentCategoriesDetection.java:111 — performFor receives the
// run's effective text).
//
// Per ECMA-376-1 §17.3.2.16 (bCs) and §17.3.2.17 (iCs) the
// complex-script side of the bold / italic toggle pair applies
// independently to complex-script runs of the run's text. When
// none of the text classifies as complex-script the toggle mirror
// is a no-op by definition and gets stripped.
func adjustRPrForRunText(fragment, text string) string {
	if fragment == "" {
		return fragment
	}
	if containsComplexScriptText(text) {
		return fragment
	}
	return stripToggleMirrorChildren(fragment)
}

// stripWMLElement removes every occurrence of <w:NAME ...?/> (self-
// closing) from s. The per-run rPr fragments use WML "w:" prefixes
// throughout and the b/i toggle mirrors are always self-closing.
//
// Match terminates at the next ">" after a strict element-name
// boundary (whitespace, "/", or ">") to avoid partial-prefix
// matches like <w:bCsExtension/>.
func stripWMLElement(s, name string) string {
	open := "<w:" + name
	for {
		i := strings.Index(s, open)
		if i < 0 {
			return s
		}
		boundary := i + len(open)
		if boundary >= len(s) {
			return s
		}
		next := s[boundary]
		if next != ' ' && next != '\t' && next != '\n' && next != '\r' && next != '/' && next != '>' {
			// Not an exact element-name match (e.g. matched <w:bCsX/>
			// while looking for <w:bCs/>). Skip past this prefix and
			// keep searching.
			s = s[:i+1] + stripWMLElement(s[i+1:], name)
			return s
		}
		end := strings.Index(s[boundary:], ">")
		if end < 0 {
			return s
		}
		end += boundary + 1
		s = s[:i] + s[end:]
	}
}

// runsHaveInlineCodes reports whether the run sequence contains any
// non-text runs (placeholders or paired codes). The fast path for a
// plain-text block short-circuits the walker below.
func runsHaveInlineCodes(runs []model.Run) bool {
	for _, r := range runs {
		if r.Text == nil {
			return true
		}
	}
	return false
}

// countTextRuns returns the number of text-bearing model runs (one
// per non-nil r.Text with non-empty Text). Used to gate the per-run
// rPr sidecar alignment guard in renderWMLBlock — when this count
// matches len(perRunRPr) the writer can index slot-by-slot; when
// it doesn't, the sidecar is suppressed.
func countTextRuns(runs []model.Run) int {
	n := 0
	for _, r := range runs {
		if r.Text != nil && r.Text.Text != "" {
			n++
		}
	}
	return n
}

// renderWMLBlock renders a run sequence as WordprocessingML runs.
//
// sourceRPr is the per-paragraph common rPr children XML stashed by
// the reader (#592). When non-empty it is prepended on every emitted
// <w:r>'s <w:rPr>, mirroring upstream Okapi RunBuilder.java which
// keeps the source's full rPr per run, and giving the WSO post-pass
// (style_optimization.go) material to lift into a synthesised
// paragraph style.
//
// perRunRPr is the per-text-run rPr fragments sidecar (Phase 2 of
// the per-run rPr work — see PARITY_NOTES.md "1083-*" cluster).
// When non-empty AND the slot for the current text-run index is
// non-empty, this fragment REPLACES sourceRPr on that <w:r>; runs
// for which the sidecar slot is empty fall back to sourceRPr. This
// preserves heterogeneous-rPr runs (e.g. hyperlink display runs
// carrying <w:rStyle val="Hyperlink"/> alongside surrounding
// non-hyperlink text) per upstream Okapi RunBuilder.java lines
// 73-188 + RunMerger.java lines 156-229 (RunProperties.equals
// gates run fusion, so heterogeneous rPr surfaces multiple <w:r>
// elements rather than collapsing to a single rPr-less <w:r>).
//
// Per ECMA-376-1 §17.3.2.
func (w *Writer) renderWMLBlock(runs []model.Run, sourceRPr string, perRunRPr []string, perRunSrcStart []bool, perRunInFieldDisplay []bool, perRunSourceHadRPr []bool, fieldStraddle bool) string {
	// Alignment guard: the per-run sidecar is one fragment per
	// text-bearing source run AFTER dedupe-on-collapse. The writer
	// emits one <w:r> per text-bearing model.Run.Text. When the two
	// counts disagree, the sidecar cannot be aligned 1:1 to model
	// runs (typically because mergeRuns coalesced source runs whose
	// non-toggle rPr differed — see runProps.equal: it ignores
	// non-toggle children, mergeRuns merges across them). In that
	// case fall back to sourceRPr-only mode rather than risk
	// emitting wrong-rPr-on-wrong-run. This preserves the previous
	// common-rPr behaviour for ambiguous paragraphs.
	if len(perRunRPr) != countTextRuns(runs) {
		perRunRPr = nil
	}
	// Mirror the same alignment guard for the srcRunStart sidecar:
	// it must have one entry per post-merge text-bearing run. If the
	// length disagrees, drop it — the writer falls back to the
	// pre-#592 behaviour (every standalone <w:br/>/<w:tab/> closes
	// the <w:r> envelope immediately, never reused by following text).
	if len(perRunSrcStart) != countTextRuns(runs) {
		perRunSrcStart = nil
	}
	// Same alignment guard for the inFieldDisplay sidecar — drop the
	// flags when they disagree with the run count, otherwise the
	// writer would force-split runs at the wrong indices.
	if len(perRunInFieldDisplay) != countTextRuns(runs) {
		perRunInFieldDisplay = nil
	}
	// Same alignment guard for the sourceHadRPr sidecar.
	if len(perRunSourceHadRPr) != countTextRuns(runs) {
		perRunSourceHadRPr = nil
	}

	// textRunTexts holds the post-pseudo text per text-bearing model
	// run, aligned with the text-run-idx the writer assigns below.
	// It lets effectiveRPr apply upstream Okapi's parse-time
	// bCs/iCs strip (okapi/filters/openxml/RunParser.java:219-229)
	// against the actual run text, rather than blanket-stripping
	// every occurrence — which would drop legitimate complex-script
	// formatting on Arabic/Hebrew/… runs (cluster 1200-*; see
	// PARITY_NOTES.md for the divergence inventory).
	var textRunTexts []string
	for _, r := range runs {
		if r.Text != nil && r.Text.Text != "" {
			textRunTexts = append(textRunTexts, r.Text.Text)
		}
	}

	// effectiveRPr returns the per-run rPr to emit at text-run index
	// idx (0-based, counting only text-bearing model.Run.Text
	// emissions).
	//
	// When the per-run sidecar is present (alignment guard at line
	// ~3206 hasn't dropped it), the slot is authoritative — including
	// when it is the empty string, which means the source `<w:r>` had
	// no rPr at all. Falling back to sourceRPr in that case would
	// inject the paragraph-wide common rPr into a bare source run,
	// fusing it with neighbouring runs that share the common subset.
	// Per upstream Okapi RunBuilder.java:73-188 each source run keeps
	// its rPr verbatim; bare runs surface as `<w:r><w:t>…</w:t></w:r>`
	// with no rPr, distinct from neighbours per
	// RunMerger.canRunPropertiesBeMerged (RunMerger.java:156-229).
	//
	// sourceRPr is only consulted when the sidecar was dropped (the
	// alignment guard set perRunRPr to nil) or when the index is out
	// of range — both defensive paths preserving pre-#592 behaviour
	// for paragraphs the sidecar cannot describe.
	//
	// bCs/iCs are stripped on-the-fly when the corresponding run
	// text contains no complex-script characters (mirrors upstream
	// Okapi RunParser.java:219-229 + ContentCategoriesDetection.java
	// :134-138; ECMA-376-1 §17.3.2.16 / .17). When the text DOES
	// carry complex-script content, the source's bCs/iCs survive
	// verbatim — fixing the 1200-* RTL synthesis cluster.
	effectiveRPr := func(idx int) string {
		var base string
		switch {
		case perRunRPr == nil:
			base = sourceRPr
		case idx < 0 || idx >= len(perRunRPr):
			base = sourceRPr
		default:
			base = perRunRPr[idx]
		}
		if base == "" {
			return base
		}
		var text string
		if idx >= 0 && idx < len(textRunTexts) {
			text = textRunTexts[idx]
		}
		return adjustRPrForRunText(base, text)
	}

	// textSrcStart returns true iff the text-run at idx began a fresh
	// source <w:r>. Defaults to true when the sidecar is absent so
	// the writer never accidentally fuses heterogeneous source-run
	// origins (false would invite previously-separate runs to merge
	// into a preceding standalone <w:br/>/<w:tab/>'s <w:r>).
	textSrcStart := func(idx int) bool {
		if idx < 0 || idx >= len(perRunSrcStart) {
			return true
		}
		return perRunSrcStart[idx]
	}

	// textInFieldDisplay returns true iff the text-run at idx was
	// emitted from inside an extractable complex field's display text
	// region. Defaults to false when the sidecar is absent (i.e. the
	// paragraph carries no such runs). See
	// openxmlPerRunInFieldDisplayAnnotationKey for the upstream-Okapi
	// rationale — runs inside the field display area must keep their
	// per-source-<w:r> envelopes distinct.
	textInFieldDisplay := func(idx int) bool {
		if idx < 0 || idx >= len(perRunInFieldDisplay) {
			return false
		}
		return perRunInFieldDisplay[idx]
	}

	// Fast paths below collapse the entire run sequence into a single
	// <w:r> via model.FlattenRuns. They are only valid when there is
	// at most one text-bearing model run — otherwise per-run rPr
	// boundaries (Phase 4 split on rPrChildren divergence; sidecar
	// slots from Phase 1/2) would be erased. When countTextRuns > 1
	// fall through to the slow path which emits one <w:r> per text
	// run with the correct effectiveRPr(idx). Mirrors upstream Okapi
	// RunBuilder.java lines 73-188 + RunMerger.java lines 156-229:
	// each distinct rPr boundary becomes a distinct <w:r>. Per
	// ECMA-376-1 §17.3.2.
	if countTextRuns(runs) <= 1 {
		// Fast path: no inline codes AND no rPr at all → single
		// <w:r><w:t> with the flattened text. Pre-#592 behaviour for
		// truly plain paragraphs (e.g. "Heading 1" inside a paragraph
		// whose style already supplies all formatting).
		if !runsHaveInlineCodes(runs) && sourceRPr == "" && effectiveRPr(0) == "" {
			return `<w:r><w:t xml:space="preserve">` + xmlEscape(model.FlattenRuns(runs)) + `</w:t></w:r>`
		}

		// Fast path: no inline codes but we DO have rPr → single
		// <w:r><w:rPr>{rPr}</w:rPr><w:t>. Prefer the per-run sidecar
		// slot 0 over sourceRPr. Mirrors Okapi's "RunMerger merges
		// adjacent same-rPr runs into one <w:r> carrying the shared
		// rPr" behaviour for paragraphs that extracted as a single
		// TextRun (after font-mapping + subtractProps + mergeRuns).
		if !runsHaveInlineCodes(runs) {
			return `<w:r><w:rPr>` + effectiveRPr(0) + `</w:rPr><w:t xml:space="preserve">` +
				xmlEscape(model.FlattenRuns(runs)) + `</w:t></w:r>`
		}
	}

	var buf strings.Builder
	var inRun bool
	// inRunNoText flags an open <w:r> that has emitted a standalone
	// <w:br/> / <w:tab/> but no <w:t> yet. The next text Run, if it
	// shares the same rPr, joins this <w:r> by opening <w:t> inside
	// it rather than spawning a new <w:r>. This preserves the source
	// shape `<w:r><w:br/><w:t>...</w:t></w:r>` (run 3 of
	// 1421-line-break.docx) which upstream Okapi RunBuilder keeps as
	// a single <w:r> per ECMA-376-1 §17.3.2.1 (CT_R: rPr followed by
	// run children — <w:br/> and <w:t> may both appear inside one
	// run). When the next Run is NOT a same-rPr text Run (different
	// rPr, another Ph, or PcOpen/PcClose), this <w:r> closes via
	// closeRunNoText so the open envelope doesn't leak.
	var inRunNoText bool
	// inRunNoTextRPr is the effective rPr the still-open `<w:r>` carries
	// (sourceRPr+runProps OR an inherited-from-neighbour rPr when the
	// open run was emitted by the Tab Ph inherit-from-next-text fuse).
	// The text-side join check compares the next text's effectiveRPr
	// against THIS, not against the paragraph-wide sourceRPr — so an
	// inherited-rPr open run can still fuse same-rPr text. When empty,
	// fall back to the original sourceRPr-only comparison.
	var inRunNoTextRPr string
	// pendingTReopen marks a speculative `<w:t xml:space="preserve">`
	// opened by the inline-tab/br branch right after `<w:tab/>` /
	// `<w:br/>` in anticipation of more text in the same <w:r>. If no
	// character data follows before closeRun fires, the open <w:t>
	// must NOT receive a closing tag — the source had
	// `<w:r><w:t>…</w:t><w:tab/></w:r>` with nothing after the tab
	// (e.g. 992.docx footer1.xml's "Be Shaping The Future " run).
	// Upstream Okapi RunBuilder.java:73-188 preserves the source-run
	// envelope and never authors a trailing empty <w:t/>. Per
	// ECMA-376-1 §17.3.3.31 (<w:tab/>) and §17.3.3.1 (<w:br/>),
	// tab/break are run children whose effect is independent of any
	// following <w:t>; emitting an empty <w:t/> would be a
	// meaningless artefact that diverges from upstream byte output.
	var pendingTReopen bool
	// inFieldEndRun flags an open <w:r> emitted from a fldChar-end Ph
	// whose closing `</w:r>` has been stripped because the next Text
	// run carries matching rPr. When the following Text arrives, it
	// appends `<w:t xml:space="preserve">…</w:t></w:r>` inside this
	// run rather than spawning a new <w:r>. fieldEndRPrEffective stores
	// the FULL post-toggle rPr (sidecar base + runProps) the field run
	// carried so the join decision at the next Text can compare it
	// against the equivalent fragment that emitRPr would produce for
	// that text. Mirrors upstream Okapi RunMerger.mergeRunBodyChunks
	// (RunMerger.java:402-441), which fuses a Markup chunk (the
	// captured fldChar end) followed by a RunText chunk (the
	// following text) into a single Run when their containing source
	// runs share rPr per canRunPropertiesBeMerged
	// (RunMerger.java:156-229). Per ECMA-376-1 §17.3.2.1 (CT_R) and
	// §17.16.5 (complex fields), a single `<w:r>` may carry
	// `<w:fldChar>` and `<w:t>` children. Fixtures: 768.docx,
	// 768-2.docx.
	var inFieldEndRun bool
	var fieldEndRPrEffective string
	var runProps string
	// toggleOpenForms tracks the source serialisation of each
	// currently-open toggle (TypeBold → `<w:b w:val="1"/>` or
	// `<w:b/>`, TypeItalic → `<w:i ...>`, etc.) so the matching
	// PcClose can strip the same byte sequence from runProps. Per
	// ECMA-376-1 §17.3.2.1 (CT_OnOff <w:b>) the bare element and
	// val="1"/"true"/"on" are equivalent ON states, but upstream
	// Okapi preserves the source form across the round-trip — this
	// map carries the per-PcOpen Data string through to the
	// matching PcClose so the writer can do the same.
	toggleOpenForms := map[string]string{}
	textRunIdx := -1 // pre-increment on each new <w:r> for r.Text

	closeRun := func() {
		if inRun {
			if pendingTReopen {
				trail := `<w:t xml:space="preserve">`
				if cur := buf.String(); strings.HasSuffix(cur, trail) {
					buf.Reset()
					buf.WriteString(cur[:len(cur)-len(trail)])
				}
				pendingTReopen = false
				buf.WriteString(`</w:r>`)
			} else {
				buf.WriteString(`</w:t></w:r>`)
			}
			inRun = false
		}
		if inRunNoText {
			buf.WriteString(`</w:r>`)
			inRunNoText = false
			inRunNoTextRPr = ""
		}
		if inFieldEndRun {
			buf.WriteString(`</w:r>`)
			inFieldEndRun = false
			fieldEndRPrEffective = ""
		}
	}

	// emitRPr concatenates the per-text-run rPr (slot at the given
	// idx, falling back to sourceRPr) with the toggle rPr the model
	// accumulated from PcOpen/PcClose runs. The combined block is
	// wrapped in a single <w:rPr>...</w:rPr> per emitted <w:r>.
	emitRPr := func(idx int) {
		base := effectiveRPr(idx)
		if base == "" && runProps == "" {
			// When the text run sits inside a cross-paragraph
			// extractable field's display region AND the source `<w:r>`
			// declared an `<w:rPr>` child (even if all of its children
			// got stripped by RunSkippableElements), preserve an empty
			// `<w:rPr></w:rPr>` wrapper. Mirrors upstream Okapi
			// BlockTextUnitWriter.flush(Run.Markup) at lines 238-251 —
			// the source rPr open/close events flow verbatim through
			// the field's outer Run body chunks. The keep-empty marker
			// guards the wrapper against the writer's
			// stripWMLSkippableElements / wmlEmptyPropertiesContainerRE
			// post-pass; postWML strips the marker before emit. Per
			// ECMA-376-1 §17.3.2.1 (CT_R) an empty `<w:rPr>` is
			// rendering-neutral but byte-distinguishable from an
			// absent `<w:rPr>`. Fixture 1102.docx P4 runs:
			// `<w:r><w:rPr><w:lang/></w:rPr><w:t>...</w:t></w:r>`
			// source → `<w:r><w:rPr/><w:t>...</w:t></w:r>` after the
			// lang strip. Excluded: runs whose source had NO rPr
			// (e.g. 1172.docx P2's bare `<w:r><w:t>...</w:t></w:r>`)
			// must emit without an rPr wrapper.
			if idx >= 0 && idx < len(perRunInFieldDisplay) && perRunInFieldDisplay[idx] &&
				idx < len(perRunSourceHadRPr) && perRunSourceHadRPr[idx] {
				buf.WriteString(`<w:rPr>`)
				buf.WriteString(fieldRPrKeepEmptyMarker)
				buf.WriteString(`</w:rPr>`)
			}
			return
		}
		buf.WriteString(`<w:rPr>`)
		buf.WriteString(base)
		buf.WriteString(runProps)
		buf.WriteString(`</w:rPr>`)
	}

	// emitNonTextRPr is used by Ph runs (br/tab/footnoteRef) that
	// emit their own <w:r> wrapper. They reuse the paragraph-common
	// sourceRPr (NOT a per-text-run slot) because they are not
	// text-bearing and don't consume a sidecar slot — the sidecar
	// is aligned to text runs only (perRunRPrFragments skips lone
	// "\n" line breaks and sentinel runs by construction; see
	// source_rpr.go).
	emitNonTextRPr := func() {
		if sourceRPr == "" && runProps == "" {
			return
		}
		buf.WriteString(`<w:rPr>`)
		buf.WriteString(sourceRPr)
		buf.WriteString(runProps)
		buf.WriteString(`</w:rPr>`)
	}

	for runIdx := 0; runIdx < len(runs); runIdx++ {
		r := runs[runIdx]
		switch {
		case r.Text != nil:
			// Snapshot whether a prior inline-tab/br left a speculative
			// `<w:t xml:space="preserve">` open inside the surrounding
			// `<w:r>` (pendingTReopen=true). When this text Run lands its
			// chars into that open `<w:t>` (instead of spawning a fresh
			// `<w:r>`), no later increment of textRunIdx happens, so
			// subsequent per-run-rPr comparisons would lag by one slot.
			// The flag drives a textRunIdx bump AFTER the rPr check —
			// the check still wants to compare the previously-emitted
			// run's rPr against the candidate's rPr, so we mustn't bump
			// before it.
			//
			// Fixture TestLTinsideBoxFails.docx footer1.xml is the
			// canonical case: source paragraph has tabbed run "Last
			// Updated: …" followed by a trailing space run carrying only
			// `<w:rPr><w:lang/></w:rPr>` (whose lang gets parse-time
			// stripped, leaving rPrChildren empty). The lagging index
			// caused the writer to compare the space run's slot against
			// "Last Updated"'s slot — both `<w:sz/>` — so it fused.
			// Upstream Okapi RunMerger.canRunPropertiesBeMerged
			// (RunMerger.java:156-229) correctly sees the space run's
			// rPr as empty and refuses the merge.
			willJoinViaPendingTReopen := inRun && pendingTReopen
			// When the next text run's effectiveRPr differs from the
			// currently-open <w:r>'s rPr, close the current run and open
			// a new one so per-run rPr boundaries (Phase 1-5 sidecar) are
			// preserved on the wire. Mirrors RunBuilder.java:73-188 +
			// RunMerger.canRunPropertiesBeMerged (RunMerger.java:156-229)
			// per ECMA-376-1 §17.3.2.1: each distinct rPr boundary is a
			// distinct <w:r>.
			//
			// Additionally: when BOTH this run and the next come from
			// an extractable complex field's display text region, the
			// per-source-<w:r> boundary must round-trip even when the
			// rPr matches. Upstream Okapi parseComplexField captures
			// each source <w:r> as its own RunText body chunk with
			// the surrounding </w:r><w:r> events preserved in adjacent
			// Markup chunks (RunParser.java:537 + 815) — the chunks do
			// NOT pass through RunMerger.canMergeWith so neighbours
			// stay as separate <w:r> envelopes. Fixtures
			// 1083-empty-and-hyperlink-instructions.docx (and the two
			// hyperlink-and-* siblings) carry " "+"with" pairs that
			// must emerge as two `<w:r>` elements. Per ECMA-376-1
			// §17.16.5 (Complex Fields) the field's display text
			// retains the source's run grouping.
			if inRun {
				nextRPr := effectiveRPr(textRunIdx + 1)
				curRPr := effectiveRPr(textRunIdx)
				// Force-close on a fresh source <w:r> boundary when the
				// per-run rPr sidecar has been nilled by the alignment
				// guard (writer.go:3206 — count mismatch from mergeRuns
				// reducing the text-run population). In that fallback
				// state effectiveRPr returns sourceRPr (the paragraph-
				// wide common subset) for ALL runs, so two source <w:r>
				// envelopes that mergeRuns REFUSED to fuse (genuinely
				// different rPr per RunMerger.canRunPropertiesBeMerged,
				// RunMerger.java:156-229) would silently fuse here and
				// lose the rPr distinction on the wire. With the sidecar
				// alive we already split on perRunRPr inequality via the
				// nextRPr != curRPr check above; this clause covers the
				// nilled-sidecar fallback. Fixture: delTextAmp.docx
				// footer1 — single-char `<w:t>t</w:t>` with `<w:spacing
				// w:val="-2"/>` would otherwise fuse with the next bare-
				// rPr text run, losing character-kerning per ECMA-376-1
				// §17.3.2.35.
				if nextRPr != curRPr ||
					(textInFieldDisplay(textRunIdx) && textInFieldDisplay(textRunIdx+1) && textSrcStart(textRunIdx+1)) ||
					(perRunRPr == nil && textSrcStart(textRunIdx+1)) {
					closeRun()
				}
			}
			// If the comparison above didn't close the run AND we're
			// joining via the pendingTReopen path, advance textRunIdx
			// now so subsequent comparisons (and the per-run-rPr writes
			// inside emitRPr) reference the correct slot. The other
			// "joining without close" paths (inRunNoText, inFieldEndRun)
			// each bump textRunIdx themselves; pendingTReopen had no
			// matching bump until this fix.
			if willJoinViaPendingTReopen && inRun && len(r.Text.Text) > 0 {
				textRunIdx++
			}
			// If a prior standalone <w:br/> / <w:tab/> left an <w:r>
			// open without a <w:t>, decide whether this text joins it.
			// Two conditions must hold:
			//   (a) the text was NOT marked as starting a new source
			//       <w:r> (perRunSrcStart sidecar from the reader); and
			//   (b) the text's effectiveRPr matches the rPr the Ph
			//       emitted via emitNonTextRPr (sourceRPr + runProps).
			// Both true → reuse the open <w:r> by opening <w:t> in
			// it, preserving `<w:r><w:br/><w:t>…</w:t></w:r>` (run 3
			// of 1421-line-break.docx). Otherwise close the no-text
			// <w:r> first so the text emits in a fresh <w:r> carrying
			// its own rPr. Per upstream Okapi RunBuilder (lines
			// 73-188) and ECMA-376-1 §17.3.2.1 (CT_R), the <w:r>
			// envelope is preserved per source run.
			if inRunNoText {
				nextRPr := effectiveRPr(textRunIdx + 1)
				// Fuse the next text into the still-open <w:r> from a
				// prior standalone <w:br/>/<w:tab/> when the text's
				// effective rPr matches what the Ph emitted via
				// emitNonTextRPr (sourceRPr + runProps). Mirrors
				// upstream Okapi RunMerger.canMergeWith
				// (RunMerger.java:126-154 → canRunPropertiesBeMerged
				// at lines 156-229) which fuses adjacent same-rPr
				// source runs into one <w:r> regardless of source-run
				// boundary; the fused runBody carries both the br/tab
				// Markup chunk and the following RunText chunk side by
				// side (canRunBodyChunksBeMerged at lines 438-441 keeps
				// distinct chunk types adjacent inside one <w:r>). Per
				// ECMA-376-1 §17.3.2.1 (CT_R) a single <w:r> may carry
				// both `<w:br/>` and `<w:t>` children with one shared
				// rPr.
				//
				// Previously the join also required
				// !textSrcStart(textRunIdx+1) — an over-conservative
				// guard added to preserve `<w:r><w:br/></w:r>` followed
				// by `<w:r><w:br/><w:t>...</w:t></w:r>` (1421-line-
				// break.docx). That case still works without the
				// textSrcStart gate: source-run-3's br is itself
				// SubTypeBreakStandalone (srcRunStart=true), so it
				// arrives in step 3 closing the prior inRunNoText and
				// opening its own; only at step 4 does the text join
				// — and step 4's text has srcRunStart=false (it's the
				// SECOND child of source-run-3) so the gate would have
				// allowed the join anyway. The gate was active for
				// fixtures where a standalone-br source-run is followed
				// by a separate text source-run with matching rPr (e.g.
				// OpenXmlRoundtripSoftLineBreaksDoNotTranslateTest
				// CharacterStyle.docx, special-chars-and-linebreaks.
				// docx) — bridge fuses these per RunMerger's rPr-only
				// gate, so dropping the textSrcStart guard restores
				// parity without regressing the 1421 case.
				// Compare against the actual rPr the open `<w:r>`
				// carries. When the open run was emitted by the Tab
				// Ph inherit-from-next-text fuse path, the rPr is
				// inRunNoTextRPr (the neighbour's effectiveRPr); the
				// canonical case is sourceRPr+runProps emitted via
				// emitNonTextRPr (then inRunNoTextRPr is "" and we
				// fall back to sourceRPr). Without this check an
				// inherited-rPr open run never fuses with the
				// following text — AlternateContentTest.docx footer1
				// tab-after-hyperlink case.
				openRPr := inRunNoTextRPr
				if openRPr == "" {
					openRPr = sourceRPr
				}
				if nextRPr == openRPr {
					textRunIdx++
					buf.WriteString(`<w:t xml:space="preserve">`)
					inRun = true
					inRunNoText = false
					inRunNoTextRPr = ""
				} else {
					buf.WriteString(`</w:r>`)
					inRunNoText = false
					inRunNoTextRPr = ""
				}
			}
			// If a prior fldChar-end Ph left an <w:r> open (its
			// trailing `</w:r>` stripped), join it when the next
			// text's emitted rPr matches the field run's rPr. Mirrors
			// upstream Okapi mergeRunBodyChunks (RunMerger.java:402-
			// 441): a Markup chunk followed by a RunText chunk fuses
			// when the containing source runs share rPr per
			// canRunPropertiesBeMerged (RunMerger.java:156-229).
			// ECMA-376-1 §17.3.2.1 (CT_R) and §17.16.5 (complex
			// fields) allow a single `<w:r>` to carry `<w:fldChar>`
			// and `<w:t>` children. Fixtures: 768.docx, 768-2.docx.
			if inFieldEndRun {
				nextRPr := effectiveRPr(textRunIdx+1) + runProps
				if nextRPr == fieldEndRPrEffective {
					textRunIdx++
					buf.WriteString(`<w:t xml:space="preserve">`)
					inRun = true
					inFieldEndRun = false
					fieldEndRPrEffective = ""
				} else {
					buf.WriteString(`</w:r>`)
					inFieldEndRun = false
					fieldEndRPrEffective = ""
				}
			}
			for _, ch := range r.Text.Text {
				if !inRun {
					textRunIdx++
					buf.WriteString(`<w:r>`)
					emitRPr(textRunIdx)
					buf.WriteString(`<w:t xml:space="preserve">`)
					inRun = true
				}
				// Any character data clears the speculative-<w:t>
				// flag — the <w:t> now has content so closeRun must
				// emit its closing tag normally.
				pendingTReopen = false
				xmlEscapeRune(&buf, ch)
			}

		case r.PcOpen != nil:
			if r.PcOpen.Type == TypeHyperlink || r.PcOpen.Type == TypeSmartTag || r.PcOpen.Type == TypeRevisionIns || r.PcOpen.Type == TypeSDT {
				// Opaque paired-code open: emit captured raw XML
				// (the <w:hyperlink ...>, <w:smartTag ...>,
				// strict-OOXML <w:ins ...>/<w:moveTo ...>, or
				// <w:sdt><w:sdtPr>...</w:sdtPr><w:sdtEndPr/>
				// <w:sdtContent> start element) verbatim, paired
				// with the matching close data emitted by the
				// corresponding PcClose. Per upstream Okapi
				// RunContainer (RunContainer.java lines 29-43,
				// 97-176, 187-191) hyperlink, smartTag, and sdt are
				// transparent run-containers preserved as a single
				// pair of codes around their inner runs; ECMA-376-1
				// §17.13.5.16 (CT_RunTrackChange) gives the same
				// shape to <w:ins>/<w:moveTo> in strict OOXML where
				// upstream's RevisionInline skippable QName does not
				// match (transitional binding only;
				// SkippableElement.java:209-212). ECMA-376-1 §17.5.2
				// (Structured Document Tags) for <w:sdt>.
				closeRun()
				buf.WriteString(r.PcOpen.Data)
			} else {
				// A toggle change closes the current <w:r> so the
				// next text emits with the updated rPr — mirrors
				// upstream Okapi RunMerger.canRunPropertiesBeMerged
				// (RunMerger.java lines 156-229), which prevents
				// merging across rPr boundaries. PcOpen.Data carries
				// the source serialisation of the toggle (e.g.
				// `<w:b w:val="1"/>` vs the canonical bare
				// `<w:b/>`); track it on toggleOpenForms so the
				// matching PcClose can strip the same form on close.
				closeRun()
				runProps = w.addWMLPropForm(runProps, r.PcOpen.Type, r.PcOpen.Data, toggleOpenForms)
			}

		case r.PcClose != nil:
			if r.PcClose.Type == TypeHyperlink || r.PcClose.Type == TypeSmartTag || r.PcClose.Type == TypeRevisionIns || r.PcClose.Type == TypeSDT {
				closeRun()
				buf.WriteString(r.PcClose.Data)
			} else {
				closeRun()
				runProps = w.removeWMLPropForm(runProps, r.PcClose.Type, toggleOpenForms)
			}

		case r.Ph != nil:
			// Inline <w:tab/> / <w:br/> into the open <w:r> when its rPr
			// matches what a free-standing tab/break would use (sourceRPr +
			// runProps). Mirrors upstream Okapi RunBuilder.java:73-188
			// which keeps tab/break as Markup chunks inside the surrounding
			// run rather than spawning a new <w:r>. Per ECMA-376-1
			// §17.3.3.31 (<w:tab/>) and §17.3.3.1 (<w:br/>), both are run
			// children that share the enclosing <w:r>'s rPr context.
			//
			// The SubTypeBreakStandalone / SubTypeTabStandalone subtypes
			// tag Ph chunks that began a fresh source <w:r> (the reader
			// sets textRun.srcRunStart on the first emission of each
			// <w:r> and buildBlock propagates it through the SubType).
			// Those MUST close the current run before emitting so the
			// source-run envelope round-trips intact — RunMerger does
			// not collapse break-bearing runs across <w:r> boundaries
			// (RunMerger.java:156-229). 1421-line-break.docx is the
			// canonical fixture.
			canInline := (r.Ph.Type == TypeTab && r.Ph.SubType != SubTypeTabStandalone) ||
				(r.Ph.Type == TypeBreak && r.Ph.SubType != SubTypeBreakStandalone)
			if canInline && inRun {
				// Compare the open <w:r>'s emitted rPr to what a
				// standalone tab/br <w:r> would emit. Both derive from
				// the SAME source <w:r> (the tab/br was authored
				// alongside the surrounding text in one <w:r>), so the
				// source rPr is identical — but the writer applies
				// bCs/iCs stripping to the text-run's rPr based on the
				// text's complex-script content
				// (adjustRPrForRunText / RunParser.java:219-229). For
				// the tab/br to inline correctly we need the post-strip
				// view of BOTH sides to match. A direct compare of the
				// raw fragments fails when one side carries bCs/iCs and
				// the other had them stripped. Per ECMA-376-1
				// §17.3.2.16/.17 (bCs/iCs) the complex-script toggle
				// mirrors are no-ops on non-complex-script text, so
				// applying the strip to sourceRPr here is semantically
				// equivalent to the strip the writer applies on the
				// text run's <w:r>. Mirrors upstream Okapi RunMerger
				// (RunMerger.java:156-229 + RunBuilder.java:73-188):
				// the source authored the tab/br alongside the text in
				// ONE <w:r>; RunMerger's canRunPropertiesBeMerged gate
				// operates on already-script-resolved RunProperties so
				// the merge is preserved. Fixture 992 footer1.xml is
				// the canonical case: a Calibri-bold run authored as
				// <w:r><w:rPr>{Calibri, b, bCs, …}</w:rPr>
				//   <w:t>Be Shaping ...</w:t><w:tab/></w:r>
				// where the text is Latin-script (bCs strips off the
				// text-run's <w:r> rPr); the tab would otherwise emit a
				// standalone <w:r> with bCs still present, losing the
				// source-run envelope.
				curRPr := effectiveRPr(textRunIdx)
				var text string
				if textRunIdx >= 0 && textRunIdx < len(textRunTexts) {
					text = textRunTexts[textRunIdx]
				}
				adjSrc := adjustRPrForRunText(sourceRPr, text)
				// Also accept inline when the Ph carries an embedded
				// source-`<w:r>` rPr (encoded as a `<w:rPr>...</w:rPr>`
				// prefix on Ph.Data) and that rPr matches the open run's
				// effective rPr. This covers the per-source-`<w:r>` tab/br
				// case where the paragraph-common sourceRPr is empty (so
				// adjSrc=="" diverges from the open run's perRun rPr).
				// Reader-side: wml.go buildBlock's TypeTab and TypeBreak
				// branches prepend `serializeFullRPrXML(run.props)` to
				// Ph.Data when the tab/br carried per-run rPr. Mirrors
				// upstream Okapi RunMerger.canRunPropertiesBeMerged
				// (RunMerger.java:156-229) which fuses adjacent same-rPr
				// source `<w:r>` envelopes — tab-bearing or text-bearing —
				// into one RunBuilder. AlternateContentTest.docx footer1
				// is the canonical fixture (FontStyle18-only after WSO,
				// per-run sidecar on the text vs empty paragraph-common).
				embeddedTabRPr := ""
				if r.Ph.Type == TypeTab && strings.HasPrefix(r.Ph.Data, "<w:rPr>") {
					if end := strings.Index(r.Ph.Data, "</w:rPr>"); end >= 0 {
						embeddedTabRPr = r.Ph.Data[len("<w:rPr>"):end]
					}
				}
				if curRPr == adjSrc || (embeddedTabRPr != "" && curRPr == embeddedTabRPr) {
					// If the previous tab/br already opened a speculative
					// <w:t> via pendingTReopen and no character data has
					// landed inside it (the buffer still ends with the
					// open `<w:t xml:space="preserve">`), strip the open
					// `<w:t>` instead of closing it — otherwise we emit a
					// stray empty `<w:t></w:t>` between adjacent tab/br
					// pairs (tabstyles.docx: `<w:r><w:t>has</w:t>
					// <w:tab/><w:tab/><w:t>tabs</w:t></w:r>` round-trips
					// with `<w:t/>` between the two tabs). Mirrors
					// upstream Okapi RunBuilder.java:73-188 — adjacent
					// tab/br Markup chunks live next to each other
					// inside the surrounding `<w:r>` with no synthetic
					// `<w:t/>` separator.
					// Re-emit br with its captured attrs (Ph.Data) when
					// available; tab is always self-closing with no
					// attributes per ECMA-376-1 §17.3.3.31.
					phData := r.Ph.Data
					if r.Ph.Type == TypeBreak && phData == "" {
						phData = "<w:br/>"
					} else if r.Ph.Type == TypeTab {
						phData = "<w:tab/>"
					}
					trail := `<w:t xml:space="preserve">`
					if pendingTReopen {
						if cur := buf.String(); strings.HasSuffix(cur, trail) {
							buf.Reset()
							buf.WriteString(cur[:len(cur)-len(trail)])
							pendingTReopen = false
							buf.WriteString(phData + `<w:t xml:space="preserve">`)
							pendingTReopen = true
							continue
						}
					}
					buf.WriteString(`</w:t>` + phData + `<w:t xml:space="preserve">`)
					// Speculative <w:t> opened in case more text
					// follows in the same source <w:r>. closeRun
					// strips the trailing <w:t/> if no character
					// data arrives first.
					pendingTReopen = true
					continue
				}
			}
			// Symmetric counterpart to the fldChar-end + text merge
			// (TypeField branch below). When the current open <w:r>
			// is a plain text run AND the next Ph is a fldChar-BEGIN
			// whose rPr matches what the open run emitted, fuse them
			// into a single <w:r> carrying both `<w:t>` and
			// `<w:fldChar/>` children. Mirrors upstream Okapi
			// RunMerger (RunMerger.add + canRunPropertiesBeMerged,
			// RunMerger.java:83-95 + 156-229): the source's plain
			// text run preceding the field-begin run is fused into
			// the field-begin's <w:r> when their RunProperties match
			// AND containsComplexFields is false on both sides — the
			// latter is true for the begin run of an EXTRACTABLE field
			// only (see isExtractableFldCharBeginRun for the
			// determination). Per ECMA-376-1 §17.3.2.1 (CT_R) a single
			// `<w:r>` may carry both `<w:t>` and `<w:fldChar>` children;
			// §17.16.5 (Complex Fields) classifies fldChar as a run
			// child. Fixtures: 1083-empty-and-hyperlink-instructions.
			// docx, 1083-hyperlink-and-date-instructions.docx,
			// 1083-hyperlink-and-empty-instructions.docx.
			if r.Ph.Type == TypeField && inRun && !pendingTReopen {
				if info := detectFldCharBeginForMerge(r.Ph.Data); info.ok {
					if isExtractableFldCharBeginRun(runs, runIdx) {
						curRPr := effectiveRPr(textRunIdx) + runProps
						if curRPr == info.rprChildren {
							buf.WriteString(`</w:t>`)
							buf.WriteString(info.innerFldChar)
							buf.WriteString(`</w:r>`)
							inRun = false
							continue
						}
					}
				}
			}
			// Fuse a TypeImage Ph (drawing/pict/object/AlternateContent)
			// into the still-open <w:r> when both the open run and the
			// drawing carry no rPr. Mirrors upstream Okapi RunMerger
			// (RunMerger.java:83-95 + 156-229): when the source <w:r>
			// wrapping the drawing carries the same RunProperties as the
			// preceding text-run, the runs are merged into one
			// RunBuilder; the resulting `<w:r>` carries both the
			// `<w:t>` text body chunk and the drawing Markup chunk
			// inside one envelope. Per ECMA-376-1 §17.3.2.1 (CT_R) a
			// single <w:r> may carry both <w:t> and <w:drawing> /
			// <w:pict> / <w:object> children with one shared rPr.
			//
			// The narrow case implemented here covers the
			// no-rPr-on-either-side scenario: the drawing's Ph.Data
			// does not start with `<w:rPr>` (the reader's image path
			// only prepends rPr when the source run had its own rPr —
			// see wml.go::buildBlock TypeImage emit), and the open
			// <w:r> carries no per-text-run rPr (effectiveRPr is empty)
			// AND no active toggles (runProps empty). Under those
			// conditions both sides have rPr-empty, so the merge is
			// rPr-equivalent.
			//
			// Fixture gettysburg_en.docx P3 source authors
			// `<w:r><w:rPr/><w:t>N</w:t><w:drawing>...</w:drawing></w:r>
			// <w:r><w:rPr/><w:t>ow we are...</w:t></w:r>` — bridge fuses
			// across the source-run boundary into one <w:r> with text +
			// drawing + text. Without this fusion native splits into
			// three separate <w:r>s.
			if r.Ph.Type == TypeImage && r.Ph.SubType == SubTypeImageInline &&
				inRun && !pendingTReopen &&
				effectiveRPr(textRunIdx) == "" && runProps == "" &&
				!strings.HasPrefix(r.Ph.Data, "<w:rPr>") {
				expanded := w.expandDrawingMarkers(r.Ph.Data)
				buf.WriteString(`</w:t>`)
				buf.WriteString(expanded)
				// Speculative <w:t> opened in case more text follows in
				// this <w:r>. closeRun strips the trailing <w:t/> if no
				// character data arrives first (the same pendingTReopen
				// machinery used by the inline tab/br path). Leave inRun
				// true so the next text continues inside this envelope.
				buf.WriteString(`<w:t xml:space="preserve">`)
				pendingTReopen = true
				continue
			}
			// Fuse a TypeRawRunMarkup chunk (<w:noBreakHyphen/> per
			// ECMA-376-1 §17.3.3.18, <w:softHyphen/> per §17.3.3.30)
			// into the still-open <w:r> from a prior standalone <w:br/>
			// or <w:tab/>. Mirrors upstream Okapi BlockTextUnitWriter:
			// the noBreakHyphen / softHyphen Markup is emitted via the
			// flushMarkup path that does NOT call flushRunEnd /
			// flushRunStart (BlockTextUnitWriter.java:240-251 only
			// terminates the run on <w:br>); meanwhile the surrounding
			// br/tab opens its own <w:r> via writeLineBreakAt
			// (BlockTextUnitWriter.java:349-371). The combined effect:
			// `<w:r><w:br/></w:r><w:r><w:noBreakHyphen/></w:r>` source
			// emerges as `<w:r><w:br/><w:noBreakHyphen/></w:r>` because
			// the noBreakHyphen Markup chunk attaches to the still-open
			// run from the br. Per ECMA-376-1 §17.3.2.1 (CT_R) a single
			// <w:r> may carry both <w:br/> and <w:noBreakHyphen/>
			// children with one shared rPr. Fixture
			// special-chars-and-linebreaks.docx authors br + noBreakHyphen
			// and br + softHyphen pairs that round-trip as one <w:r>
			// each in the bridge reference output.
			//
			// Br/tab Ph chunks are NOT fused here. Bridge's
			// writeLineBreakAt always closes and reopens the run around
			// each break (BlockTextUnitWriter.java:365-366) — adjacent
			// brs end up in separate <w:r> envelopes even when their
			// source runs share rPr. Fixtures 1421-line-break.docx and
			// br.docx both show `<w:r><w:br/></w:r><w:r><w:br/></w:r>`
			// for two adjacent standalone brs.
			if r.Ph.Type == TypeRawRunMarkup && inRunNoText {
				buf.WriteString(r.Ph.Data)
				continue
			}
			// Fuse a follow-on <w:tab/> Ph into the still-open <w:r>
			// from a prior standalone tab. Mirrors upstream Okapi
			// BlockTextUnitWriter.flush(Run.Markup) at
			// BlockTextUnitWriter.java:238-251, which appends each
			// MarkupComponent into xmlEvents inside the already-open
			// run and only calls flushRunEnd/flushRunStart for
			// <w:br>/<a:br> elements (line 243's
			// isLineBreakStartEvent). Tab markup elements pass
			// straight through into the open run. Per ECMA-376-1
			// §17.3.2.1 (CT_R) a single `<w:r>` may carry multiple
			// `<w:tab/>` children sharing one rPr.
			//
			// Pre-condition: inRunNoText is set ONLY by previous
			// tab/br emissions that left the run open expecting more
			// content in the same source <w:r>. Since both the open
			// run and this tab inherit the paragraph-wide source rPr
			// + active toggles (Ph chunks don't consume per-run
			// sidecar slots), the rPr context is identical. Mirrors
			// RunMerger.canRunPropertiesBeMerged (RunMerger.java:
			// 156-229) — same rPr → adjacent runs fuse on the way to
			// the writer.
			//
			// Fixture apissue.docx header2.xml authors three adjacent
			// source <w:r>s: `<w:r>…<w:tab/></w:r><w:r>…<w:tab/></w:r>
			// <w:r>…<w:t>F150 USB…</w:t></w:r>`, all carrying the
			// same Arial/40 rPr. Bridge fuses them into a single
			// `<w:r><w:tab/><w:tab/><w:t>F150 USB…</w:t></w:r>`. The
			// follow-on text run picks up the still-open <w:r> via
			// the existing inRunNoText branch (text path).
			if r.Ph.Type == TypeTab && inRunNoText {
				buf.WriteString(`<w:tab/>`)
				// Leave inRunNoText set so a subsequent tab or
				// same-rPr text continues to fuse into this run.
				continue
			}
			// Symmetric fusion path for <w:br/>: adjacent same-rPr
			// br runs collapse into one `<w:r>` envelope with
			// multiple `<w:br/>` children. Mirrors upstream Okapi
			// RunMerger.add → canMergeWith (RunMerger.java:83-95 +
			// 126-154 → canRunPropertiesBeMerged at 156-229): two
			// source `<w:r>` envelopes each carrying a single
			// `<w:br/>` Markup chunk with byte-equal RunProperties
			// merge into one RunBuilder whose body chunks list
			// retains both br Markup chunks side by side
			// (canRunBodyChunksBeMerged at lines 438-441 keeps
			// distinct chunk types — and like-typed Markup chunks —
			// adjacent inside one run). Per ECMA-376-1 §17.3.2.1
			// (CT_R) a single `<w:r>` may carry multiple `<w:br/>`
			// children under one shared rPr; §17.3.3.1 (CT_Br)
			// classifies `<w:br/>` as a run child.
			//
			// Pre-condition: the prior br emission set inRunNoText
			// (SubTypeBreakStandalone branch at the same TypeBreak
			// case below). Since Ph runs don't consume per-text-run
			// sidecar slots, both br Markup chunks inherit the
			// paragraph-wide source rPr + active toggles — the
			// equality of effective rPr is implicit. Native's
			// per-run sidecar already pruned divergent-rPr neighbours
			// into distinct br Phs at the parser level via the
			// `run.srcRunStart && !activeProps.equal(run.props)`
			// guard in buildBlock (wml.go ~line 4630), so by the
			// time two TypeBreak Phs land adjacent here their
			// source rPr already matched.
			//
			// Fixture br.docx: source authors
			// `<w:r><w:rPr>{rFonts,color,szCs,lang}</w:rPr><w:br/>
			// </w:r><w:r><w:rPr>{same}</w:rPr><w:br/></w:r>` —
			// bridge fuses to `<w:r><w:br/><w:br/></w:r>` (rPr
			// stripped to empty by StyleOptimisation since none
			// of the children survive WSO's no-content drop on a
			// br-only run). br2.docx and EndGroup.docx exhibit the
			// same shape with shd/sz variants.
			if r.Ph.Type == TypeBreak && inRunNoText {
				// Refuse to fuse when the about-to-fuse br itself
				// began a source `<w:r>` that ALSO carries trailing
				// content in the SAME envelope (text or another br
				// followed by text). In that case the new br's source
				// `<w:r>` must round-trip intact — fusing into the
				// prior `<w:r>` would split the br away from the
				// text/markup it shared a source envelope with, and
				// upstream Okapi RunMerger / BlockTextUnitWriter
				// preserves the source's `<w:br/>+<w:t>` boundary.
				//
				// 1421-line-break.docx is the canonical fixture: source
				// authors `<w:r><w:rPr>{lang}</w:rPr><w:br/></w:r>
				// <w:r><w:rPr>{lang}</w:rPr><w:br/><w:t>...</w:t></w:r>`.
				// Bridge keeps the two source envelopes — the first
				// br emerges as `<w:r><w:br/></w:r>`, the second as
				// `<w:r><w:br/><w:t>...</w:t></w:r>`. The lookahead
				// here detects "next model run is a Text whose sidecar
				// flags it as continuing this br's source <w:r>"
				// (textSrcStart(textRunIdx+1) == false) and bails out
				// of the fuse so the next-iteration TypeBreak handler
				// closes the prior inRunNoText and opens a fresh `<w:r>`
				// for the br + trailing text.
				//
				// Similarly, when the immediate successor is another
				// TypeBreak Ph but THAT br's source `<w:r>` carries
				// trailing text (lookahead: runs[runIdx+2].Text with
				// !textSrcStart), the same source-envelope-preservation
				// rule applies. Otherwise (next br is also "lone" or
				// is followed by a srcRunStart text — i.e., a fresh
				// source `<w:r>`) the fuse is correct. Fixture
				// special-chars-and-linebreaks.docx exercises the
				// adjacent same-rPr `<w:r><w:br/></w:r>` pairs where
				// each br is the SOLE child of its source `<w:r>`,
				// matching br.docx's shape.
				if runIdx+1 < len(runs) &&
					runs[runIdx+1].Text != nil &&
					!textSrcStart(textRunIdx+1) {
					// Next model run is a Text continuing THIS br's
					// source <w:r>. Don't fuse — let the TypeBreak
					// branch below close prior inRunNoText and open
					// a fresh <w:r> for the br + trailing text.
				} else if runIdx+1 < len(runs) &&
					runs[runIdx+1].Ph != nil &&
					runs[runIdx+1].Ph.Type == TypeBreak &&
					runIdx+2 < len(runs) &&
					runs[runIdx+2].Text != nil &&
					!textSrcStart(textRunIdx+1) {
					// Next br + text in the new br's source envelope.
					// Mirror the single-Text case above.
				} else {
					brXMLFuse := r.Ph.Data
					if brXMLFuse == "" {
						brXMLFuse = "<w:br/>"
					}
					// Strip the embedded `<w:rPr>...</w:rPr>` prefix
					// (added by the reader at wml.go ~line 4683 to
					// preserve the source <w:r>'s rPr): the fusion
					// reuses the prior open `<w:r>`'s rPr context,
					// so dropping the duplicate rPr is correct. If
					// the rPrs disagreed the lookahead guards above
					// would have refused the fuse — but we don't
					// repeat the rPr comparison here because
					// inRunNoText was set by the prior br emission
					// which already gated on its own rPr.
					if strings.HasPrefix(brXMLFuse, "<w:rPr>") {
						if end := strings.Index(brXMLFuse, "</w:rPr>"); end >= 0 {
							brXMLFuse = brXMLFuse[end+len("</w:rPr>"):]
						}
					}
					buf.WriteString(brXMLFuse)
					// Leave inRunNoText set so a subsequent br, tab,
					// or same-rPr text continues to fuse into this run.
					continue
				}
			}
			// Fuse a Tab Ph into the still-open <w:r> from a prior
			// text run when the surrounding text runs share the same
			// effective rPr. Mirrors upstream Okapi RunMerger
			// (RunMerger.java:156-229): adjacent source <w:r>
			// envelopes with matching RunProperties fuse into one
			// RunBuilder; tab MarkupComponents inside such a run
			// stay inline with the text body chunks. Per ECMA-376-1
			// §17.3.2.1 (CT_R) and §17.3.3.31 (`<w:tab/>`) a single
			// `<w:r>` may carry both `<w:t>` and `<w:tab/>` children
			// under one shared rPr.
			//
			// Pre-conditions:
			//   - inRun: the preceding text opened a `<w:r>` that's
			//     still emitting `<w:t>` content (pendingTReopen
			//     might be set; we'll override the trailing `<w:t
			//     xml:space="preserve">` placeholder).
			//   - The next model run is a Text whose effectiveRPr
			//     matches the currently-open run's effectiveRPr
			//     (textRunIdx points to the last emitted text run,
			//     so its effectiveRPr is the open run's rPr).
			// When both hold, emit `</w:t><w:tab/>` to close the
			// current text body and stitch the tab in-place, then
			// open `<w:t xml:space="preserve">` as the new text body
			// (pendingTReopen=true) so the next text continues
			// inside the same `<w:r>`.
			//
			// The tab's own source-rPr is dropped (sidecars are
			// aligned to text runs only, so we don't have its
			// per-run slot). For the fuse to be semantically
			// correct the surrounding texts must share rPr — when
			// they do, the dropped tab rPr is equivalent to what
			// the surrounding runs carry, so the fusion is
			// content-preserving. Fixture AlternateContentTest.docx
			// footer1: the source has 8+ source `<w:r>` envelopes
			// all carrying `<w:rStyle val="FontStyle18"/><w:lang
			// val="cs-CZ"/>` rPr, including tab-bearing runs and
			// text-bearing runs alike. Upstream RunMerger fuses
			// them into one `<w:r>` with multiple `<w:t>` and
			// `<w:tab/>` body chunks under the shared rPr;
			// pre-fix native opened a fresh empty-rPr `<w:r>` per
			// tab.
			if r.Ph.Type == TypeTab && inRun &&
				runIdx+1 < len(runs) && runs[runIdx+1].Text != nil &&
				effectiveRPr(textRunIdx) == effectiveRPr(textRunIdx+1) {
				if pendingTReopen {
					trail := `<w:t xml:space="preserve">`
					if cur := buf.String(); strings.HasSuffix(cur, trail) {
						buf.Reset()
						buf.WriteString(cur[:len(cur)-len(trail)])
					}
					pendingTReopen = false
				} else {
					buf.WriteString(`</w:t>`)
				}
				buf.WriteString(`<w:tab/>`)
				buf.WriteString(`<w:t xml:space="preserve">`)
				pendingTReopen = true
				// inRun stays true; the next text branch sees the
				// open `<w:t xml:space="preserve">` and continues
				// writing into it.
				continue
			}
			closeRun()
			switch r.Ph.Type {
			case TypeBreak:
				// A <w:br/> inside a run inherits the surrounding
				// rPr in upstream Okapi (RunBuilder treats <w:br/>
				// as a Markup chunk inside the same <w:r>). For
				// the native renderer we wrap it in its own <w:r>
				// for symmetry with the existing pipeline; the
				// surrounding text runs carry their own rPr.
				//
				// When the Ph is SubTypeBreakStandalone (began a
				// fresh source <w:r>), leave the <w:r> OPEN
				// (inRunNoText=true) so a following text run that
				// originated in the same source <w:r> can join it
				// by opening <w:t> inside this <w:r>. Otherwise
				// close immediately. Mirrors upstream Okapi
				// RunBuilder (RunBuilder.java:73-188) which keeps
				// each source <w:r>'s br + text together in one
				// envelope. 1421-line-break.docx is the canonical
				// fixture (run 3: `<w:r><w:br/><w:t>…</w:t></w:r>`).
				// Prefer Ph.Data (captured `<w:br ...>/>` with attrs)
				// over the literal `<w:br/>` so w:type="page" /
				// w:type="column" / w:clear survive. Per ECMA-376-1
				// §17.3.3.1 (CT_Br) the type attribute distinguishes
				// textWrap, page, and column break semantics.
				brXMLBranch := r.Ph.Data
				if brXMLBranch == "" {
					brXMLBranch = "<w:br/>"
				}
				// When the Ph data starts with <w:rPr> the reader
				// embedded the source <w:r>'s rPr alongside the br
				// (wml.go ~line 4683) — emit the rPr inside this
				// run's envelope instead of the paragraph-wide
				// sourceRPr + active toggles. Mirrors the existing
				// TypeImage / TypeFootnoteRef embedded-rPr pattern
				// (writer.go ~line 3060). Per ECMA-376-1 §17.3.2.1
				// (CT_R) <w:rPr> precedes the run's other children.
				// EndGroup.docx is the canonical fixture: a
				// `<w:r><w:rPr>{szCs val=21}</w:rPr><w:br/></w:r>`
				// must round-trip with its szCs intact even when
				// the surrounding text runs have different rPr (so
				// the common-rPr is empty and would otherwise drop
				// the szCs sidecar).
				// Cross-paragraph field straddle: prepend an empty
				// `<w:r></w:r>` envelope before every standalone-br Ph.
				// Mirrors upstream Okapi BlockTextUnitWriter.flush(
				// Run.Markup) lines 238-251: when the outer field
				// Run's body chunk's first Component.Start is a
				// `<w:br>`, flushRunStart opens an `<w:r>` and the
				// loop's flushRunEnd immediately closes it before
				// flushRunStart re-opens a fresh `<w:r>` for the
				// br's events. The closed-immediately envelope
				// becomes an empty `<w:r></w:r>` in the wire output.
				//
				// Native preserves the source's per-`<w:r>` boundaries
				// per textRun, so the equivalent artifact must be
				// synthesised explicitly here. Only SubTypeBreakStandalone
				// (a br that began a fresh source `<w:r>`) qualifies —
				// the in-run br shape (`<w:br/>` followed by `<w:t>`
				// inside the same source `<w:r>`) already lives in a
				// single envelope via the inRunNoText fuse path above.
				//
				// Fixture 1172.docx P2 is the canonical case: the
				// HYPERLINK field's display area spans P1 (begin/
				// instrText/separate), P2 (text + br + br+text), and
				// P3 (fldChar end). Reference output emits an empty
				// `<w:r/>` before each of the br-only and br+text
				// source runs.
				if fieldStraddle && r.Ph.SubType == SubTypeBreakStandalone {
					closeRun() // belt-and-braces; closeRun was already invoked above
					buf.WriteString(`<w:r></w:r>`)
				}
				if strings.HasPrefix(brXMLBranch, "<w:rPr>") {
					if r.Ph.SubType == SubTypeBreakStandalone {
						buf.WriteString(`<w:r>`)
						buf.WriteString(brXMLBranch)
						inRunNoText = true
					} else {
						buf.WriteString(`<w:r>` + brXMLBranch + `</w:r>`)
					}
				} else if r.Ph.SubType == SubTypeBreakStandalone {
					buf.WriteString(`<w:r>`)
					emitNonTextRPr()
					buf.WriteString(brXMLBranch)
					inRunNoText = true
				} else if sourceRPr != "" || runProps != "" {
					buf.WriteString(`<w:r>`)
					emitNonTextRPr()
					buf.WriteString(brXMLBranch + `</w:r>`)
				} else {
					buf.WriteString(`<w:r>` + brXMLBranch + `</w:r>`)
				}
			case TypeTab:
				// Look ahead: if the NEXT model run is a Text whose
				// per-text-run sidecar marks it as continuing the SAME
				// source <w:r> as this tab (textSrcStart=false), leave
				// the new <w:r> open as inRunNoText so the text fuses
				// inside this run via the inRunNoText branch above.
				// Mirrors upstream Okapi RunBuilder.java:73-188 — a
				// source <w:r> may carry both <w:tab/> and <w:t>
				// children; per ECMA-376-1 §17.3.3.31 (<w:tab/>) +
				// §17.3.2.1 (CT_R) the tab and following text share the
				// enclosing <w:r>'s rPr context. Fixture
				// Document-with-tabs-5.docx P2: source
				// `<w:r><w:tab/><w:t>Text after tab.</w:t></w:r>` must
				// round-trip as one <w:r>, not two.
				// Lookahead extension: also keep the new <w:r> open
				// when the next run is another TypeTab Ph that will
				// itself fuse via the TypeTab + inRunNoText branch
				// above. Mirrors upstream Okapi
				// BlockTextUnitWriter.flush(Run.Markup) lines 238-251
				// — adjacent <w:tab/> markup chunks live inside the
				// same open <w:r> envelope. Fixture apissue.docx
				// header2.xml authors `<w:r>…<w:tab/></w:r>
				// <w:r>…<w:tab/></w:r><w:r>…<w:t>F150…</w:t></w:r>`
				// all carrying Arial/40 rPr; bridge fuses to one
				// `<w:r><w:tab/><w:tab/><w:t>F150…</w:t></w:r>`.
				keepOpen := runIdx+1 < len(runs) &&
					runs[runIdx+1].Text != nil &&
					!textSrcStart(textRunIdx+1)
				if !keepOpen && runIdx+1 < len(runs) &&
					runs[runIdx+1].Ph != nil &&
					runs[runIdx+1].Ph.Type == TypeTab {
					keepOpen = true
				}
				// Mirrors upstream Okapi RunMerger.canMergeWith
				// (RunMerger.java:126-154 → canRunPropertiesBeMerged at
				// 156-229) which fuses adjacent source `<w:r>` envelopes
				// when their RunProperties match — independent of the
				// source-run boundary. The tab's Ph carries no per-text-
				// run sidecar slot (sidecars cover text only), so its
				// effective rPr is sourceRPr + active toggles
				// (`runProps`); we compare that to the next text's
				// effective rPr. When equal, keep the `<w:r>` open so
				// the text fuses inside via the `inRunNoText` text
				// branch above — emerging as
				// `<w:r><w:rPr.../><w:tab/><w:t>...</w:t></w:r>`,
				// matching upstream's RunMerger output. Per ECMA-376-1
				// §17.3.2.1 (CT_R) and §17.3.3.31 (`<w:tab/>`) a single
				// `<w:r>` may carry both `<w:tab/>` and `<w:t>` children
				// under one shared `<w:rPr>`. Fixture
				// AlternateContentTest.docx footer1: a paragraph whose
				// text-tab-text-tab-text source runs all share the same
				// `<w:rStyle val="FontStyle18"/><w:lang val="cs-CZ"/>`
				// rPr — bridge fuses them into ONE `<w:r>` with multiple
				// `<w:t>` and `<w:tab/>` body chunks; pre-fix native
				// kept them split because the textSrcStart guard above
				// blocked the join.
				// Lookahead: when the next text run's effective rPr
				// matches what this tab WOULD carry on its own (sourceRPr
				// + active toggles), fuse them into one <w:r>.
				if !keepOpen && runIdx+1 < len(runs) && runs[runIdx+1].Text != nil {
					nextRPr := effectiveRPr(textRunIdx + 1)
					tabRPr := sourceRPr + runProps
					if nextRPr == tabRPr {
						keepOpen = true
					}
				}
				// Inherit-from-neighbour rPr: when the tab Ph is
				// sandwiched between two text runs that SHARE the same
				// effective rPr AND that rPr differs from
				// sourceRPr+runProps, emit the tab inside an `<w:r>`
				// carrying that shared rPr. The tab in the source
				// carried the same per-run rPr override (a property of
				// the surrounding style context — typically a shared
				// `<w:rStyle>`); reconstructing it from the neighbours
				// matches upstream Okapi's RunMerger fusion behaviour
				// (RunMerger.canMergeWith at RunMerger.java:126-154 +
				// canRunPropertiesBeMerged at 156-229). Per ECMA-376-1
				// §17.3.2.1 (CT_R) the tab `<w:r>`'s rPr is
				// independent; a shared neighbour rPr is strong
				// evidence the tab carried the same. Both-sides match
				// guard avoids mis-fusing when the tab sits between
				// runs with DIFFERENT rPr (e.g.
				// AlternateContentTest.docx: a tab between
				// `<w:t>46 70 82 19</w:t>` (FontStyle18) and
				// `<w:t>DIC</w:t>` (FontStyle25) — the surrounding
				// runs differ so the tab keeps its sourceRPr-only
				// emit and the DIC text opens its own `<w:r>`).
				//
				// Fixture AlternateContentTest.docx footer1: a tab
				// between a `<w:hyperlink>` close and a `<w:r>` with
				// `<w:rStyle val="FontStyle18"/>` rPr. The previous
				// (extracted) text was inside the hyperlink and ALSO
				// carried `<w:rStyle val="FontStyle18"/>` — so
				// textRunIdx and textRunIdx+1 share effectiveRPr.
				// Inheriting matches upstream's fused
				// `<w:r><w:rPr><w:rStyle val="FontStyle18"/></w:rPr>
				// <w:tab/><w:t>IC: 46 70 82 19</w:t><w:tab/></w:r>`
				// (the next-next run is another tab that the
				// adjacent-tab fuse handles).
				inheritedNextRPr := ""
				if !keepOpen && runIdx+1 < len(runs) && runs[runIdx+1].Text != nil {
					nextRPr := effectiveRPr(textRunIdx + 1)
					prevRPr := effectiveRPr(textRunIdx)
					tabRPr := sourceRPr + runProps
					if nextRPr != tabRPr && nextRPr != "" && nextRPr == prevRPr {
						inheritedNextRPr = nextRPr
						keepOpen = true
					}
				}
				// When the initial keepOpen fired from !textSrcStart (tab
				// and the following text share a source `<w:r>`) AND
				// sourceRPr+runProps is empty, the tab would otherwise
				// emit as a bare `<w:r><w:tab/>` whose `inRunNoTextRPr`
				// is "" — and the following text's join check would fail
				// (next rPr is non-empty, openRPr is empty). Inherit the
				// next text's effective rPr so the tab carries the same
				// `<w:rPr>` the source had and the join succeeds.
				// Mirrors upstream Okapi RunBuilder.java:73-188 — the
				// tab and following text live inside one source `<w:r>`
				// whose rPr applies to both. Without this, fixture
				// TestLTinsideBoxFails.docx footer1 splits the tab away
				// from its accompanying `Last Updated…` text after a
				// preceding `<w:fldSimple>` closes the prior run.
				if keepOpen && inheritedNextRPr == "" &&
					runIdx+1 < len(runs) && runs[runIdx+1].Text != nil &&
					!textSrcStart(textRunIdx+1) {
					nextRPr := effectiveRPr(textRunIdx + 1)
					tabRPr := sourceRPr + runProps
					if nextRPr != "" && nextRPr != tabRPr {
						inheritedNextRPr = nextRPr
					}
				}
				// Extract embedded source-rPr from Ph.Data, if any. Reader
				// embeds `<w:rPr>...</w:rPr><w:tab/>` when the source
				// `<w:r>` carried per-run rPr; the writer uses this as the
				// fallback rPr when no surrounding context drove an
				// inherited rPr. Mirrors upstream Okapi RunMerger which
				// preserves the tab's source `<w:r>` rPr when fusion
				// doesn't apply.
				embeddedTabRPr := ""
				if strings.HasPrefix(r.Ph.Data, "<w:rPr>") {
					if end := strings.Index(r.Ph.Data, "</w:rPr>"); end >= 0 {
						embeddedTabRPr = r.Ph.Data[len("<w:rPr>"):end]
					}
				}
				if r.Ph.SubType == SubTypeTabStandalone {
					buf.WriteString(`<w:r>`)
					emitNonTextRPr()
					buf.WriteString(`<w:tab/>`)
					inRunNoText = true
				} else if inheritedNextRPr != "" {
					buf.WriteString(`<w:r><w:rPr>`)
					buf.WriteString(inheritedNextRPr)
					buf.WriteString(`</w:rPr><w:tab/>`)
					inRunNoText = true
					inRunNoTextRPr = inheritedNextRPr
				} else if embeddedTabRPr != "" && sourceRPr == "" && runProps == "" {
					// Reader supplied the source `<w:r>`'s rPr; prefer it
					// over the empty paragraph-common rPr so the tab's
					// envelope round-trips its source styling. WSO has
					// already stripped redundant children at the model
					// layer, so the embedded fragment is what the bridge
					// would emit.
					buf.WriteString(`<w:r><w:rPr>`)
					buf.WriteString(embeddedTabRPr)
					buf.WriteString(`</w:rPr><w:tab/>`)
					if keepOpen {
						inRunNoText = true
						inRunNoTextRPr = embeddedTabRPr
					} else {
						buf.WriteString(`</w:r>`)
					}
				} else if sourceRPr != "" || runProps != "" {
					buf.WriteString(`<w:r>`)
					emitNonTextRPr()
					buf.WriteString(`<w:tab/>`)
					if keepOpen {
						inRunNoText = true
					} else {
						buf.WriteString(`</w:r>`)
					}
				} else {
					buf.WriteString(`<w:r><w:tab/>`)
					if keepOpen {
						inRunNoText = true
					} else {
						buf.WriteString(`</w:r>`)
					}
				}
			case TypeImage:
				// Drawings/pict/object are opaque — never wrap with
				// our paragraph-synthesised rPr because the captured
				// payload is the original <w:r>'s body verbatim (see
				// the reader's textRun{text:"", data:raw}).
				//
				// When the source <w:r> wrapping the drawing carried
				// its own <w:rPr>, the buildBlock image path
				// (serializeFullRPrXML) prepends the serialised
				// `<w:rPr>...</w:rPr>` fragment to Ph.Data so the
				// writer threads it back into the run envelope here.
				// Per ECMA-376-1 §17.3.2.1 (CT_R) <w:rPr> is the first
				// child of <w:r>, preceding `<w:drawing>` /
				// `<w:pict>` / `<w:object>`, so the embedded fragment
				// is in document order; we just emit Ph.Data as-is
				// inside the <w:r> wrapper. Mirrors upstream Okapi's
				// RunBuilder which materialises the source
				// RunProperties on every emitted run, drawing-bearing
				// or not. 859.docx fixture: a strict-OOXML drawing
				// run carrying
				// `<w:rPr><w:noProof/><w:lang w:eastAsia="ru-RU"/></w:rPr>
				// <w:drawing>` round-trips with both rPr and drawing
				// preserved.
				//
				// extractDrawingTranslations may have replaced
				// translatable sites (drawing-name attributes,
				// vml-textpath strings, txbx-content paragraph
				// bodies) with <!--KAPI-PROP:tu123--> /
				// <!--KAPI-PARA:tu123--> markers; expand those
				// against the per-Write blocks index here so the
				// captured payload picks up translated content.
				expanded := w.expandDrawingMarkers(r.Ph.Data)
				buf.WriteString(`<w:r>` + expanded + `</w:r>`)
			case TypeRawRunMarkup:
				// Raw run-child markup chunk (<w:noBreakHyphen/> per
				// ECMA-376-1 §17.3.3.18, <w:softHyphen/> per §17.3.3.30,
				// <w:cr/> per §17.3.3.4, schema-misplaced <w:bidi>
				// per §17.3.1.6+§17.3.2.1). The Ph.Data field holds the
				// literal element XML; wrap it in a <w:r> with the
				// surrounding paragraph's source rPr context
				// (sourceRPr + active toggles in runProps) so the markup
				// inherits the right formatting envelope the same way
				// upstream Okapi RunParser (RunParser.java:752-766)
				// routes the element to runBuilder.addToMarkup, where
				// it lives inside the containing RunBuilder's <w:r> on
				// output.
				//
				// SubTypeCR + same-source-<w:r> lookahead: when the cr
				// markup originates from a source `<w:r>` that ALSO
				// carries a `<w:t>` sibling (e.g. MissingPara.docx
				// para 1: `<w:r><w:rPr>...</w:rPr><w:cr/>
				// <w:t>FRIENDLY</w:t></w:r>` — legal per
				// ECMA-376-1 §17.3.2.1 CT_R), the reader emits two
				// adjacent textRuns: [cr-rawmarkup with srcRunStart=
				// true, text with srcRunStart=false]. The default
				// emit-and-close path above splits the source envelope,
				// producing `<w:r>...<w:cr/></w:r><w:r>...<w:t>...
				// </w:t></w:r>`. Upstream Okapi RunBuilder
				// (RunBuilder.java:73-188) keeps the cr Markup chunk
				// and following RunText chunk inside one <w:r>; per
				// ECMA-376-1 §17.3.3.4 (CT_Empty for <w:cr/>) the cr
				// has no content of its own and inherits the
				// containing <w:r>'s rPr context.
				//
				// SubTypeBidi is the schema-misplaced `<w:bidi>` that
				// appears as a DIRECT child of `<w:r>` (between `<w:r>`
				// start and `<w:rPr>`). Upstream Okapi RunParser
				// preserves it via runBuilder.addToMarkup
				// (RunParser.java:815) and emits it alongside the run's
				// text inside the same `<w:r>` envelope. Fixture:
				// 899.docx.
				//
				// Both subtypes leave the `<w:r>` OPEN
				// (inRunNoText=true) so the next text run joins via
				// the existing inRunNoText branch above. Same
				// rPr-equality guard applies: the text fuses only when
				// its effectiveRPr matches the open run's rPr
				// (inRunNoTextRPr || sourceRPr).
				if r.Ph.SubType == SubTypeCR &&
					runIdx+1 < len(runs) && runs[runIdx+1].Text != nil &&
					!textSrcStart(textRunIdx+1) {
					if strings.HasPrefix(r.Ph.Data, `<w:rPr>`) {
						// Embedded per-run rPr (reader sets this when
						// commonRPrXML is empty so the cr's source
						// `<w:rPr>` survives — wml.go ~line 4742).
						// Stash the embedded rPr text as inRunNoTextRPr
						// so the join check at line ~2697 compares the
						// next text's effectiveRPr against the SAME
						// rPr the cr carried.
						embeddedRPr := ""
						if end := strings.Index(r.Ph.Data, `</w:rPr>`); end >= 0 {
							// Extract inner text between <w:rPr> and </w:rPr>.
							embeddedRPr = r.Ph.Data[len(`<w:rPr>`):end]
						}
						buf.WriteString(`<w:r>` + r.Ph.Data)
						inRunNoText = true
						inRunNoTextRPr = embeddedRPr
					} else if sourceRPr != "" || runProps != "" {
						buf.WriteString(`<w:r>`)
						emitNonTextRPr()
						buf.WriteString(r.Ph.Data)
						inRunNoText = true
					} else {
						buf.WriteString(`<w:r>` + r.Ph.Data)
						inRunNoText = true
					}
					continue
				}
				if r.Ph.SubType == SubTypeBidi {
					if sourceRPr != "" || runProps != "" {
						buf.WriteString(`<w:r>`)
						emitNonTextRPr()
						buf.WriteString(r.Ph.Data)
					} else {
						buf.WriteString(`<w:r>` + r.Ph.Data)
					}
					inRunNoText = true
				} else if sourceRPr != "" || runProps != "" {
					buf.WriteString(`<w:r>`)
					emitNonTextRPr()
					buf.WriteString(r.Ph.Data + `</w:r>`)
				} else {
					buf.WriteString(`<w:r>` + r.Ph.Data + `</w:r>`)
				}
			case TypeFootnoteRef:
				// When the Ph data starts with <w:rPr> the reader
				// embedded the run-specific rPr (e.g.
				// <w:rStyle w:val="FootnoteReference"/>) alongside the
				// marker so the writer keeps the marker inside the same
				// <w:r> as that rPr — mirrors upstream Okapi RunBuilder
				// which never splits the marker from its rPr. Per
				// ECMA-376 Part 1 §17.3.2.1 (CT_R) <w:rPr> precedes the
				// run's other children, so the embedded fragment is
				// already in document order.
				if strings.HasPrefix(r.Ph.Data, `<w:rPr>`) {
					buf.WriteString(`<w:r>` + r.Ph.Data + `</w:r>`)
				} else if sourceRPr != "" || runProps != "" {
					buf.WriteString(`<w:r>`)
					emitNonTextRPr()
					buf.WriteString(r.Ph.Data + `</w:r>`)
				} else {
					buf.WriteString(`<w:r>` + r.Ph.Data + `</w:r>`)
				}
			case TypeField:
				// Complex field markup (fldChar / instrText) and
				// fldSimple — the captured payload already carries its
				// own <w:r>...</w:r> or <w:fldSimple>...</w:fldSimple>
				// wrapper plus rPr, so we emit verbatim with no extra
				// wrapping or per-paragraph rPr injection. Per upstream
				// Okapi (RunParser.parseComplexField, lines 461-542 of
				// okapi/filters/openxml/src/main/java/net/sf/okapi/
				// filters/openxml/RunParser.java; BlockParser.parse for
				// fldSimple, lines 242-250) the run that hosts a field
				// marker is preserved as a single opaque markup chunk.
				//
				// Special-case for fldChar="end": when the next Text
				// run (skipping intervening PcOpen/PcClose toggles)
				// shares rPr with this field run, upstream Okapi
				// fuses them into a single <w:r> carrying both the
				// fldChar-end and the following text
				// (mergeRunBodyChunks at RunMerger.java:402-441 +
				// canRunPropertiesBeMerged at RunMerger.java:156-229).
				// ECMA-376-1 §17.3.2.1 (CT_R) and §17.16.5 (complex
				// fields) both permit a run with multiple body
				// children. We strip the trailing `</w:r>` from the
				// payload, advance past the intervening toggle codes
				// (folding them into runProps directly so the writer's
				// PcOpen/PcClose handler can't fire its closeRun),
				// and set inFieldEndRun so the next matching Text
				// branch appends `<w:t xml:space="preserve">…</w:t>
				// </w:r>` inside the still-open run. Fixtures:
				// 768.docx, 768-2.docx.
				if info := detectFldCharEndForMerge(r.Ph.Data); info.ok && isExtractableFldCharEndRun(runs, runIdx) {
					anticipatedRunProps := runProps
					// Clone toggleOpenForms so the speculative
					// add/remove pass doesn't mutate live state used
					// by the outer PcOpen / PcClose branches.
					anticipatedOpens := make(map[string]string, len(toggleOpenForms))
					for k, v := range toggleOpenForms {
						anticipatedOpens[k] = v
					}
					nextTextAt := -1
					mergeable := true
					for j := runIdx + 1; j < len(runs); j++ {
						nr := runs[j]
						switch {
						case nr.Text != nil:
							nextTextAt = j
						case nr.PcOpen != nil:
							if nr.PcOpen.Type == TypeHyperlink || nr.PcOpen.Type == TypeSmartTag || nr.PcOpen.Type == TypeRevisionIns || nr.PcOpen.Type == TypeSDT {
								mergeable = false
							} else {
								anticipatedRunProps = w.addWMLPropForm(anticipatedRunProps, nr.PcOpen.Type, nr.PcOpen.Data, anticipatedOpens)
								continue
							}
						case nr.PcClose != nil:
							if nr.PcClose.Type == TypeHyperlink || nr.PcClose.Type == TypeSmartTag || nr.PcClose.Type == TypeRevisionIns || nr.PcClose.Type == TypeSDT {
								mergeable = false
							} else {
								anticipatedRunProps = w.removeWMLPropForm(anticipatedRunProps, nr.PcClose.Type, anticipatedOpens)
								continue
							}
						default:
							// Any Ph between fldChar-end and the next
							// text breaks the merge (the Ph wants its
							// own <w:r> envelope).
							mergeable = false
						}
						break
					}
					if mergeable && nextTextAt > runIdx {
						nextRPr := effectiveRPr(textRunIdx+1) + anticipatedRunProps
						if nextRPr == info.rprChildren {
							// Commit the merge: write the truncated
							// payload, fold the intervening PcOpen/
							// PcClose codes into runProps directly,
							// and advance runIdx so the outer loop
							// resumes at the next Text.
							buf.WriteString(info.truncated)
							inFieldEndRun = true
							fieldEndRPrEffective = info.rprChildren
							runProps = anticipatedRunProps
							toggleOpenForms = anticipatedOpens
							runIdx = nextTextAt - 1
							continue
						}
					}
				}
				buf.WriteString(r.Ph.Data)
			case TypeOpaqueParaChild:
				// Paragraph-level opaque element captured by the
				// reader's `case "oMathPara", "oMath":` and
				// `case "AlternateContent":` arms (paragraph-level
				// dispatch in parseParagraph). The captured payload
				// is the entire `<m:oMath>` / `<m:oMathPara>`
				// (ECMA-376 Part 1 §22.1) or paragraph-level
				// `<mc:AlternateContent>` (ECMA-376 Part 3 §10)
				// subtree — a direct `<w:p>` child rather than a
				// `<w:r>` child. Emit Ph.Data raw at paragraph level
				// (closeRun above already terminated any open
				// `<w:r>`) so the source position survives the
				// round-trip. Mirrors upstream Okapi BlockParser
				// (BlockParser.java:240-260) which gathers these
				// events into a markup chunk that the writer
				// re-emits verbatim around the paragraph's
				// translatable runs. Canonical fixture:
				// OpenXML_text_reference_v1_2.docx.
				buf.WriteString(r.Ph.Data)
			default:
				buf.WriteString(r.Ph.Data)
			}
		}
	}

	closeRun()
	return buf.String()
}

// pullLeadingFldCharEndIntoPrevParagraph is the post-skeleton WML
// pass that re-anchors a leading `<w:fldChar w:fldCharType="end"/>`
// run to the immediately preceding paragraph when that previous
// paragraph carries an open complex-field begin/separate without a
// matching end inside its own body.
//
// Mirrors upstream Okapi RunParser.parseComplexField
// (RunParser.java:461-542) plus BlockParser.parse lines 221-228:
// the field-end event is held in deferredEvents alongside the
// intervening paragraph-end events, and when the field finally
// closes the deferred chunks attach to the run/paragraph that
// finished consuming them — visibly migrating an isolated leading
// fld-end into the previous paragraph. Per ECMA-376-1 §17.16.5
// (complex fields) the fldChar elements bookend a single semantic
// run regardless of paragraph layout, so moving a stray leading
// end into the previous paragraph is content-preserving.
//
// Scope: only matches the simple case where the host paragraph's
// FIRST translatable run is `<w:r>...<w:fldChar w:fldCharType=
// "end"/>...</w:r>` (optionally preceded by a `<w:r><w:rPr>...
// </w:rPr></w:r>` empty-rPr placeholder run) and the previous
// paragraph contains an unmatched `<w:fldChar w:fldCharType=
// "begin"/>` (begin count > end count when scanning the previous
// paragraph's body). Other fld-end runs (mid-paragraph, after text)
// are left in place. Empty-rPr placeholder runs that precede the
// fld-end stay with their original paragraph (the reference output
// shows them remaining there alongside the moved end's slot).
//
// Fixtures: 830-1.docx, 830-3.docx, 830-5.docx, 830-6.docx (the
// canonical fld-end paragraph movement cluster).
func pullLeadingFldCharEndIntoPrevParagraph(data []byte) []byte {
	// The `<w:r>` envelope around a fld-end may carry rsid* /
	// rsidDel / rsidR / rsidRPr attributes that survive into the
	// post-WSO output (the rsid strip is only applied by the
	// canonical normalizer, not by stripWMLSkippableElements).
	// Use a regex that accepts any `<w:r ...>` open tag and either
	// the paired `<w:fldChar.../></w:fldChar>` or the empty
	// self-closing `<w:fldChar.../>` body. Per ECMA-376-1
	// §17.3.2.1 (CT_R) and §17.16.5.6 (CT_FldChar), an isolated
	// fld-end run carries no rPr or other body chunks — match
	// only the bare-body shape.
	if !bytes.Contains(data, []byte(`fldCharType="end"`)) {
		return data
	}
	src := string(data)
	var out strings.Builder
	out.Grow(len(src))

	pos := 0
	// Track the byte offset within `out` where the previous
	// paragraph's `</w:p>` was emitted, so we can splice the
	// migrated fld-end run in just before it.
	prevParaCloseInOut := -1
	// Track the CUMULATIVE open/close balance across all
	// paragraphs seen so far. A POSITIVE balance means more
	// begins than ends across the document body so far — i.e.
	// there is at least one unmatched begin somewhere upstream
	// awaiting an end. Per upstream Okapi RunParser.
	// parseComplexField (RunParser.java:461-542) the field
	// consumption is parser-wide, so the destination paragraph
	// for a migrated end is the IMMEDIATELY preceding paragraph
	// regardless of whether IT carries the begin: the begin may
	// live arbitrarily far upstream with empty paragraphs in
	// between (830-3.docx is the canonical case: para 1 has the
	// unmatched begin/separate, para 2 is empty, para 3 holds
	// the stray end — the end migrates to para 2).
	cumulativeFldBalance := 0
	// Track whether the immediately-preceding paragraph's body
	// (after pPr) is "migration-eligible" — either empty (no
	// runs) or carrying an unmatched fld-begin/separate of its
	// own. When the previous paragraph is non-empty and not
	// itself carrying the open field, leaving the fld-end in
	// its source paragraph matches upstream Okapi: the
	// deferredEvents flush in parseComplexField anchors the
	// end to whichever paragraph the parser's stream cursor
	// is in at the time of the end event, and full text-bearing
	// paragraphs with no field content of their own do NOT
	// receive a migrated end via that mechanism. Fixture
	// 830-5.docx para 6 is the canonical guard: a bare `<w:p>`
	// holds a leading fld-end + space, the immediately-
	// preceding paragraph (00000006) holds plain text
	// "paragraphs.", and upstream LEAVES the end in the bare
	// paragraph.
	prevParaMigrationEligible := false

	for pos < len(src) {
		// Find next paragraph-open / paragraph-close.
		nextOpen := indexFromOf(src, pos, "<w:p>", "<w:p ")
		if nextOpen < 0 {
			out.WriteString(src[pos:])
			break
		}
		// Copy up to the paragraph open verbatim.
		out.WriteString(src[pos:nextOpen])
		// Locate the end of this paragraph open tag (`>` after
		// `<w:p` — covers both `<w:p>` and `<w:p attr="…">`).
		tagClose := strings.IndexByte(src[nextOpen:], '>')
		if tagClose < 0 {
			out.WriteString(src[nextOpen:])
			break
		}
		paraOpenEnd := nextOpen + tagClose + 1
		// Find this paragraph's `</w:p>` — search forward.
		paraEnd := strings.Index(src[paraOpenEnd:], "</w:p>")
		if paraEnd < 0 {
			out.WriteString(src[nextOpen:])
			break
		}
		paraEndAbs := paraOpenEnd + paraEnd
		paraCloseEndAbs := paraEndAbs + len("</w:p>")
		paraOpenTag := src[nextOpen:paraOpenEnd]
		paraBody := src[paraOpenEnd:paraEndAbs]

		// Skip past the optional <w:pPr>...</w:pPr> at the head
		// of the body — pPr is paragraph properties, not a
		// translatable run.
		bodyStart := 0
		if strings.HasPrefix(paraBody, "<w:pPr>") || strings.HasPrefix(paraBody, "<w:pPr ") {
			pprEnd := strings.Index(paraBody, "</w:pPr>")
			if pprEnd >= 0 {
				bodyStart = pprEnd + len("</w:pPr>")
			} else if i := strings.Index(paraBody, "/>"); i >= 0 && strings.HasPrefix(paraBody, "<w:pPr") {
				// Self-closing <w:pPr/>.
				bodyStart = i + 2
			}
		}

		// Try to migrate the leading fld-end run upward.
		moved := false
		if prevParaCloseInOut >= 0 && cumulativeFldBalance > 0 && prevParaMigrationEligible {
			// The leading run must be a bare-body fld-end run
			// (no rPr, no other body chunks) wrapped in any
			// `<w:r ...>` envelope. The wrapper may carry
			// rsid* attrs that survived the WSO pass; rPr-bearing
			// fld-ends (different shape) are intentionally left
			// in place because they're not the simple deferred-
			// flush shape the migration models. Per ECMA-376-1
			// §17.3.2.1 (CT_R) and §17.16.5.6 (CT_FldChar) an
			// isolated fld-end carries no rPr or body chunks
			// other than the fldChar itself.
			leading := paraBody[bodyStart:]
			matchedRun := matchLeadingFldEndRun(leading)
			if matchedRun != "" {
				// Splice the run at the recorded `</w:p>` of
				// the previous paragraph in `out`. The previous
				// paragraph thereby acquires a fld-end that
				// closes the upstream open begin.
				existing := out.String()
				newOut := existing[:prevParaCloseInOut] + matchedRun + existing[prevParaCloseInOut:]
				out.Reset()
				out.WriteString(newOut)
				cumulativeFldBalance--
				// Drop the leading run from the current body.
				newBody := paraBody[:bodyStart] + paraBody[bodyStart+len(matchedRun):]
				// Re-emit this paragraph WITHOUT the leading run.
				out.WriteString(paraOpenTag)
				out.WriteString(newBody)
				out.WriteString("</w:p>")
				prevParaCloseInOut = out.Len() - len("</w:p>")
				cumulativeFldBalance += countFldBeginEndBalance(newBody)
				prevParaMigrationEligible = paraMigrationEligible(newBody, bodyStart)
				moved = true
			}
		}

		if !moved {
			out.WriteString(paraOpenTag)
			out.WriteString(paraBody)
			out.WriteString("</w:p>")
			prevParaCloseInOut = out.Len() - len("</w:p>")
			cumulativeFldBalance += countFldBeginEndBalance(paraBody)
			prevParaMigrationEligible = paraMigrationEligible(paraBody, bodyStart)
		}
		pos = paraCloseEndAbs
	}

	return []byte(out.String())
}

// pullLeadingFldCharEndIntoPrevParagraphInTxbxContents applies the
// same fld-end migration as pullLeadingFldCharEndIntoPrevParagraph
// but scoped to each `<w:txbxContent>...</w:txbxContent>` region,
// and matches BOTH the bare-body and the rPr-bearing leading
// fld-end run shapes (see matchLeadingFldEndRunInTxbx).
//
// Textbox bodies are XML-nested inside `<w:drawing>` / `<w:pict>`
// envelopes which themselves sit inside an outer `<w:r>` of a
// surrounding `<w:p>`. The document-level migration pass
// (pullLeadingFldCharEndIntoPrevParagraph) scans top-level `<w:p>`
// boundaries and, because of the nesting, never identifies the
// inner textbox paragraphs as standalone units — the first
// `</w:p>` it locates after the outer paragraph open is actually
// the inner textbox paragraph's close, so the outer-paragraph body
// it computes spans only as far as that inner close. This pass
// walks the file looking for txbxContent regions and applies the
// migration to the paragraphs they enclose, with the upstream-
// Okapi-correct rPr-bearing-run shape allowed.
//
// Mirrors upstream Okapi: the textbox body is parsed as a separate
// IBlock event stream (TextboxContentExtraction + BlockParser.parse
// over the inner WML), so the deferredEvents/complex-field flush
// in RunParser.parseComplexField (RunParser.java:461-542) runs
// independently inside the txbxContent scope. A HYPERLINK opened
// in textbox paragraph N with its matching end in paragraph N+1
// produces — after the deferredEvents flush — an output where the
// end run (with its rPr) lives at the tail of paragraph N and
// paragraph N+1's body is empty. Fixture:
// 1341-textbox-with-a-hyperlink.docx.
//
// Idempotent: matchLeadingFldEndRunInTxbx only matches leading
// fld-end runs in a paragraph body, so running this pass twice is
// safe.
func pullLeadingFldCharEndIntoPrevParagraphInTxbxContents(data []byte) []byte {
	if !bytes.Contains(data, []byte("<w:txbxContent")) {
		return data
	}
	if !bytes.Contains(data, []byte(`fldCharType="end"`)) {
		return data
	}
	src := string(data)
	var out strings.Builder
	out.Grow(len(src))
	pos := 0
	const closeTok = "</w:txbxContent>"
	for pos < len(src) {
		nextOpen := indexFromOf(src, pos, "<w:txbxContent>", "<w:txbxContent ")
		if nextOpen < 0 {
			out.WriteString(src[pos:])
			break
		}
		// Locate the end of the open tag (`>` after `<w:txbxContent`).
		tagClose := strings.IndexByte(src[nextOpen:], '>')
		if tagClose < 0 {
			out.WriteString(src[pos:])
			break
		}
		openEnd := nextOpen + tagClose + 1
		closeAt := strings.Index(src[openEnd:], closeTok)
		if closeAt < 0 {
			out.WriteString(src[pos:])
			break
		}
		innerStart := openEnd
		innerEnd := openEnd + closeAt
		// Copy everything up to and including the txbxContent open tag.
		out.WriteString(src[pos:openEnd])
		// Apply the inner-scope migration to the txbxContent contents.
		inner := []byte(src[innerStart:innerEnd])
		migrated := pullLeadingFldCharEndIntoPrevParagraphTxbxScope(inner)
		out.Write(migrated)
		// Copy the closing tag verbatim.
		out.WriteString(closeTok)
		pos = innerEnd + len(closeTok)
	}
	return []byte(out.String())
}

// pullLeadingFldCharEndIntoPrevParagraphTxbxScope is the
// txbxContent-scoped migration loop. Same algorithm as
// pullLeadingFldCharEndIntoPrevParagraph except the leading-run
// match accepts an rPr-bearing fld-end run as well as the bare
// form (see matchLeadingFldEndRunInTxbx for the upstream-Okapi
// rationale). It also handles the self-closing `<w:p ... />`
// paragraph shape that appears inside textboxes that originally
// had no pPr and ended up with an empty body after WSO.
func pullLeadingFldCharEndIntoPrevParagraphTxbxScope(data []byte) []byte {
	src := string(data)
	var out strings.Builder
	out.Grow(len(src))

	pos := 0
	prevParaCloseInOut := -1
	cumulativeFldBalance := 0
	prevParaMigrationEligible := false

	for pos < len(src) {
		nextOpen := indexFromOf(src, pos, "<w:p>", "<w:p ")
		if nextOpen < 0 {
			out.WriteString(src[pos:])
			break
		}
		out.WriteString(src[pos:nextOpen])
		tagClose := strings.IndexByte(src[nextOpen:], '>')
		if tagClose < 0 {
			out.WriteString(src[nextOpen:])
			break
		}
		// Self-closing `<w:p .../>` paragraph — copy verbatim. The
		// self-closing form has no body to receive a migrated run.
		if tagClose > 0 && src[nextOpen+tagClose-1] == '/' {
			paraCloseEndAbs := nextOpen + tagClose + 1
			out.WriteString(src[nextOpen:paraCloseEndAbs])
			// Treat as an empty paragraph: not a valid migration
			// destination in the next iteration (no `</w:p>` to
			// splice before), and resets eligibility.
			prevParaCloseInOut = -1
			prevParaMigrationEligible = false
			pos = paraCloseEndAbs
			continue
		}
		paraOpenEnd := nextOpen + tagClose + 1
		paraEnd := strings.Index(src[paraOpenEnd:], "</w:p>")
		if paraEnd < 0 {
			out.WriteString(src[nextOpen:])
			break
		}
		paraEndAbs := paraOpenEnd + paraEnd
		paraCloseEndAbs := paraEndAbs + len("</w:p>")
		paraOpenTag := src[nextOpen:paraOpenEnd]
		paraBody := src[paraOpenEnd:paraEndAbs]

		bodyStart := 0
		if strings.HasPrefix(paraBody, "<w:pPr>") || strings.HasPrefix(paraBody, "<w:pPr ") {
			pprEnd := strings.Index(paraBody, "</w:pPr>")
			if pprEnd >= 0 {
				bodyStart = pprEnd + len("</w:pPr>")
			} else if i := strings.Index(paraBody, "/>"); i >= 0 && strings.HasPrefix(paraBody, "<w:pPr") {
				bodyStart = i + 2
			}
		}

		moved := false
		if prevParaCloseInOut >= 0 && cumulativeFldBalance > 0 && prevParaMigrationEligible {
			leading := paraBody[bodyStart:]
			matchedRun := matchLeadingFldEndRunInTxbx(leading)
			if matchedRun != "" {
				existing := out.String()
				newOut := existing[:prevParaCloseInOut] + matchedRun + existing[prevParaCloseInOut:]
				out.Reset()
				out.WriteString(newOut)
				cumulativeFldBalance--
				newBody := paraBody[:bodyStart] + paraBody[bodyStart+len(matchedRun):]
				out.WriteString(paraOpenTag)
				out.WriteString(newBody)
				out.WriteString("</w:p>")
				prevParaCloseInOut = out.Len() - len("</w:p>")
				cumulativeFldBalance += countFldBeginEndBalance(newBody)
				prevParaMigrationEligible = paraMigrationEligible(newBody, bodyStart)
				moved = true
			}
		}

		if !moved {
			out.WriteString(paraOpenTag)
			out.WriteString(paraBody)
			out.WriteString("</w:p>")
			prevParaCloseInOut = out.Len() - len("</w:p>")
			cumulativeFldBalance += countFldBeginEndBalance(paraBody)
			prevParaMigrationEligible = paraMigrationEligible(paraBody, bodyStart)
		}
		pos = paraCloseEndAbs
	}

	return []byte(out.String())
}

// leadingFldEndRunRE matches a `<w:r ...><w:fldChar
// w:fldCharType="end"/></w:r>` (or paired body) at the head of a
// paragraph body. The wrapper attrs are arbitrary (rsid* survive
// the WSO pass); the fld-end body must carry no rPr or other
// children — only the fldChar itself, in either self-closing or
// paired form. Anchored to the start with ^ so it only fires when
// the run is the FIRST token of the post-pPr body. Per ECMA-376-1
// §17.3.2.1 (CT_R) and §17.16.5.6 (CT_FldChar).
var leadingFldEndRunRE = regexp.MustCompile(
	`^<w:r(?:\s[^>]*)?><w:fldChar w:fldCharType="end"(?:/>|></w:fldChar>)</w:r>`,
)

// leadingRunOpenRE matches just the `<w:r ...>` open tag at the head
// of a body. Used by matchLeadingFldEndRunInTxbx to peel the wrapper
// before inspecting the run body shape, since Go's RE2 regexp engine
// can't match a `<w:rPr>...</w:rPr>` child with arbitrary inner XML
// via a single pattern.
var leadingRunOpenRE = regexp.MustCompile(`^<w:r(?:\s[^>]*)?>`)

// matchLeadingFldEndRun returns the byte slice of the leading
// fld-end run when present at the head of body, or "" otherwise.
// The returned slice is suitable for splicing verbatim into the
// destination paragraph and removing from the source body.
func matchLeadingFldEndRun(body string) string {
	m := leadingFldEndRunRE.FindString(body)
	return m
}

// matchLeadingFldEndRunInTxbx returns the byte slice of a leading
// fld-end run at the head of body, accepting EITHER the bare-body
// form (no rPr) or the rPr-bearing form (one `<w:rPr>...</w:rPr>`
// preceding the fldChar). Used by the txbxContent-scoped migration
// pass where upstream Okapi preserves the fld-end run's rPr when
// the run migrates across the paragraph boundary inside a textbox
// body. Per ECMA-376-1 §17.3.2.1 (CT_R), `<w:rPr>` is the first
// optional child of `<w:r>`. Fixture:
// 1341-textbox-with-a-hyperlink.docx — the textbox's fld-end run
// carries `<w:rPr><w:b/><w:bCs/><w:sz w:val="32"/>
// <w:szCs w:val="28"/></w:rPr>` and that rPr survives migration.
//
// The RE2 regexp engine can't match a `<w:rPr>...</w:rPr>` child
// containing arbitrary inner WML elements via a single bounded
// pattern, so this routine peels the `<w:r ...>` open tag, scans
// for `</w:rPr>` to delimit the optional rPr child, then matches
// the fld-end + `</w:r>` tail.
func matchLeadingFldEndRunInTxbx(body string) string {
	if m := leadingFldEndRunRE.FindString(body); m != "" {
		return m
	}
	openLoc := leadingRunOpenRE.FindStringIndex(body)
	if openLoc == nil {
		return ""
	}
	rest := body[openLoc[1]:]
	if !strings.HasPrefix(rest, "<w:rPr>") && !strings.HasPrefix(rest, "<w:rPr ") {
		return ""
	}
	rprEnd := strings.Index(rest, "</w:rPr>")
	if rprEnd < 0 {
		return ""
	}
	tail := rest[rprEnd+len("</w:rPr>"):]
	const fldEndSelfClose = `<w:fldChar w:fldCharType="end"/></w:r>`
	const fldEndPaired = `<w:fldChar w:fldCharType="end"></w:fldChar></w:r>`
	if strings.HasPrefix(tail, fldEndSelfClose) {
		return body[:openLoc[1]+rprEnd+len("</w:rPr>")+len(fldEndSelfClose)]
	}
	if strings.HasPrefix(tail, fldEndPaired) {
		return body[:openLoc[1]+rprEnd+len("</w:rPr>")+len(fldEndPaired)]
	}
	return ""
}

// paraMigrationEligible reports whether a paragraph's emitted body
// is a valid destination for a migrated leading fld-end run from
// the immediately-following paragraph. The eligibility rules
// approximate upstream Okapi's deferredEvents flush behaviour
// (RunParser.parseComplexField + BlockParser.parse lines 221-228):
// the field-end is anchored to whichever paragraph the parser's
// stream cursor is in at the time of the end event, which in
// practice means
//
//   - an EMPTY paragraph (no runs, or only empty placeholder runs
//     such as `<w:r><w:rPr><w:rtl w:val="0"/></w:rPr></w:r>`)
//     between the begin/separate paragraph and the source
//     paragraph that originally held the end gets the end
//     attached (the parser's stream cursor lands in the
//     placeholder paragraph after consuming it during deferred
//     flush). Fixtures 830-3.docx and 830-2.docx (para 7 with
//     a single empty rtl-only run) are the canonical cases.
//   - a paragraph that itself carries an unmatched fld-begin/separate
//     gets the end appended (the field's local close happens in the
//     same paragraph as its open). Fixture 830-1.docx is the
//     canonical case.
//   - a paragraph whose body carries text-bearing runs (`<w:t>`
//     children) but NO open fld-begin/separate of its own is NOT
//     eligible — leaving the end in the source paragraph matches
//     upstream's behaviour where the field doesn't reach back
//     through arbitrary text content. Fixture 830-5.docx para 6
//     is the canonical guard: a bare `<w:p>` holds a leading
//     fld-end + space, the immediately-preceding paragraph
//     (00000006) holds plain text "paragraphs.", and upstream
//     LEAVES the end in the bare paragraph.
//
// bodyStart is the offset within body of the first non-pPr byte —
// passed in from the caller's bookkeeping.
func paraMigrationEligible(body string, bodyStart int) bool {
	rest := body[bodyStart:]
	// Treat as empty when there are no `<w:t>` (text) children.
	// Empty placeholder runs (e.g. `<w:r><w:rPr><w:rtl w:val=
	// "0"/></w:rPr></w:r>` in 830-2.docx para 7) carry no
	// translatable text and let the parser's deferredEvents
	// flush attach the migrated end after them. Per
	// ECMA-376-1 §17.3.2.1 (CT_R) a `<w:t>` child marks the
	// run as text-bearing.
	if !strings.Contains(rest, "<w:t>") && !strings.Contains(rest, "<w:t ") {
		return true
	}
	// Paragraph carrying its own unmatched fld-begin: eligible.
	const beginTok = `w:fldCharType="begin"`
	const endTok = `w:fldCharType="end"`
	begins := strings.Count(rest, beginTok)
	ends := strings.Count(rest, endTok)
	return begins > ends
}

// countFldBeginEndBalance counts the difference between
// `<w:fldChar w:fldCharType="begin"/>` and `<w:fldChar
// w:fldCharType="end"/>` occurrences in body, where `body` is the
// inner XML of a `<w:p>` element (after pPr stripping is optional —
// pPr never carries a fldChar). Positive means more begins than
// ends — an unmatched begin awaiting an end.
func countFldBeginEndBalance(body string) int {
	const beginTok = `w:fldCharType="begin"`
	const endTok = `w:fldCharType="end"`
	begins := strings.Count(body, beginTok)
	ends := strings.Count(body, endTok)
	return begins - ends
}

// indexFromOf returns the smallest non-negative index >= start of
// any of the substrings, or -1 when none occur.
func indexFromOf(s string, start int, subs ...string) int {
	best := -1
	for _, sub := range subs {
		i := strings.Index(s[start:], sub)
		if i < 0 {
			continue
		}
		if best < 0 || i < best {
			best = i
		}
	}
	if best < 0 {
		return -1
	}
	return start + best
}

// addWMLProp adds a formatting property element to the accumulated rPr content.
func (w *Writer) addWMLProp(current, spanType string) string {
	switch spanType {
	case TypeBold:
		return current + "<w:b/>"
	case TypeItalic:
		return current + "<w:i/>"
	case TypeUnderline:
		return current + `<w:u w:val="single"/>`
	case TypeStrikethrough:
		return current + "<w:strike/>"
	case TypeSuperscript:
		return current + `<w:vertAlign w:val="superscript"/>`
	case TypeSubscript:
		return current + `<w:vertAlign w:val="subscript"/>`
	}
	return current
}

// removeWMLProp removes a formatting property from the accumulated rPr content.
func (w *Writer) removeWMLProp(current, spanType string) string {
	switch spanType {
	case TypeBold:
		return strings.ReplaceAll(current, "<w:b/>", "")
	case TypeItalic:
		return strings.ReplaceAll(current, "<w:i/>", "")
	case TypeUnderline:
		return strings.ReplaceAll(current, `<w:u w:val="single"/>`, "")
	case TypeStrikethrough:
		return strings.ReplaceAll(current, "<w:strike/>", "")
	case TypeSuperscript:
		return strings.ReplaceAll(current, `<w:vertAlign w:val="superscript"/>`, "")
	case TypeSubscript:
		return strings.ReplaceAll(current, `<w:vertAlign w:val="subscript"/>`, "")
	}
	return current
}

// addWMLPropForm appends the source-preserving toggle serialisation
// for spanType to current and stashes that form in opens so the
// matching close can strip the same byte sequence. data is the
// PcOpen.Data captured by appendOpeningRuns: the bare element form
// (`<w:b/>`) when the source authored it, or the explicit-on form
// (`<w:b w:val="1"/>`) when the source carried `val="1"` /
// `val="true"` / `val="on"`. data="" or any non-toggle spanType
// falls back to addWMLProp's hardcoded canonical form to preserve
// the legacy code path for non-bold/italic toggles.
//
// Per ECMA-376-1 §17.3.2.1 (CT_OnOff <w:b>) and §17.3.2.13
// (CT_OnOff <w:i>) the bare element and val="1"/"true"/"on" are
// equivalent ON states, but upstream Okapi preserves the source
// form across the round-trip (RunProperties.minified() retains the
// captured RunProperty's exact QName + attributes;
// RunProperties.java:497-540). 830-2.docx and 830-6.docx are the
// canonical fixtures.
func (w *Writer) addWMLPropForm(current, spanType, data string, opens map[string]string) string {
	switch spanType {
	case TypeBold, TypeItalic:
		form := data
		if form == "" {
			form = w.addWMLProp("", spanType)
		}
		opens[spanType] = form
		return current + form
	}
	return w.addWMLProp(current, spanType)
}

// removeWMLPropForm strips the toggle form previously stashed by
// addWMLPropForm (or, when no captured form is present, falls back
// to removeWMLProp's hardcoded canonical form).
func (w *Writer) removeWMLPropForm(current, spanType string, opens map[string]string) string {
	switch spanType {
	case TypeBold, TypeItalic:
		if form, ok := opens[spanType]; ok && form != "" {
			delete(opens, spanType)
			return strings.ReplaceAll(current, form, "")
		}
	}
	return w.removeWMLProp(current, spanType)
}

// dmlRunPropertyStartTagRE matches the start tag of a DrawingML
// run-property container — `<a:rPr>`, `<a:endParaRPr>`, or
// `<a:defRPr>` — including any attributes. Used by
// stripDMLRunPropertyAttrs to scrub Okapi's strippable attribute
// set without disturbing the element body or any inner children
// (`<a:solidFill>`, `<a:latin>`, `<a:hlinkClick>`, …).
//
// The (?s) flag is unnecessary — start tags do not contain
// newlines in any encoder we feed (encoding/xml, captureRawElement,
// hand-built strings) — but the `[^>]*` body is naturally
// dot-equivalent because `[^>]` matches newlines.
var dmlRunPropertyStartTagRE = regexp.MustCompile(`<a:(?:rPr|endParaRPr|defRPr)\b[^>]*>`)

// dmlStrippableAttrRE matches a single attribute (preceded by
// whitespace) inside a DrawingML run-property start tag whose name
// matches Okapi's StrippableAttributes.DrawingRunProperties set —
// `err`, `noProof`, `dirty`, `smtClean`, `lang`, `altLang`. These
// six attribute names are unconditionally dropped from any
// `<a:rPr>` / `<a:endParaRPr>` / `<a:defRPr>` element by upstream
// Okapi (StrippableAttributes.java lines 67-100, RunProperty enum
// at lines 258-277, BlockParser.java instantiating the stripper
// for every block-properties parse).
//
// Per ECMA-376-1 §22.1.2 (DrawingML EG_RPrBase / CT_TextCharacter
// PropertiesType): `lang` defaults to "en-US" at runtime, `dirty`
// defaults to false (revision-tracking hint), `smtClean` defaults
// to false (smart-tag hint), `err` / `noProof` are spell/grammar
// hints with implementation-defined defaults — none of these
// affect rendered text shape, so dropping them is content-
// preserving.
var dmlStrippableAttrRE = regexp.MustCompile(` (?:err|noProof|dirty|smtClean|lang|altLang)="[^"]*"`)

// dmlBlockOpenTagRE matches an `<a:p>` (paragraph) opening tag,
// either self-closing or with attributes. The opening tag scopes a
// block of content where Okapi's RunParser /
// BlockPropertiesFactory pipeline applies
// StrippableAttributes.DrawingRunProperties to every `<a:rPr>` /
// `<a:endParaRPr>` / `<a:defRPr>` it encounters. Outside this scope
// (e.g. inside `<a:lstStyle>` paragraph defaults per ECMA-376-1
// §21.1.2.4.4) the stripper is NOT invoked and the source attribute
// set survives verbatim — see the BlockParser / RunParser citation
// in stripDMLRunPropertyAttrs.
var dmlBlockOpenTagRE = regexp.MustCompile(`<a:p\b[^>]*>`)

// dmlBlockCloseTag is the literal `</a:p>` closing tag.
const dmlBlockCloseTag = "</a:p>"

// stripDMLRunPropertyAttrs scans payload for DrawingML run-property
// start tags (<a:rPr>, <a:endParaRPr>, <a:defRPr>) that appear
// INSIDE a `<a:p>` paragraph block and removes the six attributes
// Okapi unconditionally strips during paragraph parsing
// (StrippableAttributes.DrawingRunProperties). Body and child
// elements pass through unchanged. Run-property elements outside
// `<a:p>` (e.g. inside `<a:lstStyle><a:defPPr><a:defRPr/>`
// list-style defaults per ECMA-376-1 §21.1.2.4.4) are left alone
// because upstream Okapi's stripper is only attached to
// BlockParser / RunParser / ParagraphBlockProperties (see
// BlockParser.java:163, RunParser.java:525,
// ParagraphBlockProperties.java:664) — list-style and table-style
// defaults bypass this pipeline.
//
// The fast-path skips payloads that obviously contain no <a:p>.
//
// This is the post-write equivalent of upstream Okapi's
// BlockPropertiesFactory + BlockParser pipeline applying
// drawingRunPropertiesStrippableAttributes to every <a:rPr>,
// <a:endParaRPr>, <a:defRPr> start element observed inside a
// paragraph block.
//
// Native's WML drawing path captures the entire <w:drawing>
// payload as opaque XML in extractDrawingTranslations and writes
// it back verbatim through writeDrawingXMLToSkel (see wml.go).
// Without this strip, source-side `<a:endParaRPr lang="en-US"
// dirty="0">` survives round-trip, diverging against upstream
// canon (DrawingML_Test.docx fixture).
func stripDMLRunPropertyAttrs(payload string) string {
	if !strings.Contains(payload, "<a:p") {
		return payload
	}
	var out strings.Builder
	out.Grow(len(payload))
	pos := 0
	for pos < len(payload) {
		loc := dmlBlockOpenTagRE.FindStringIndex(payload[pos:])
		if loc == nil {
			out.WriteString(payload[pos:])
			return out.String()
		}
		openEnd := pos + loc[1]
		out.WriteString(payload[pos:openEnd])
		// <a:p> elements do not nest in any fixture corpus
		// (DrawingML §21.1.2.2.6 CT_TextParagraph allows <a:r>/
		// <a:fld>/<a:br> children but not nested <a:p>), so a
		// flat strings.Index is sufficient to find the matching
		// </a:p>.
		closeRel := strings.Index(payload[openEnd:], dmlBlockCloseTag)
		if closeRel < 0 {
			// Unbalanced — emit rest verbatim and bail. Defensive
			// against captured-payload truncation.
			out.WriteString(payload[openEnd:])
			return out.String()
		}
		blockEnd := openEnd + closeRel
		block := payload[openEnd:blockEnd]
		// Strip attributes from any <a:rPr>/<a:endParaRPr>/<a:defRPr>
		// start tag inside the paragraph body.
		stripped := dmlRunPropertyStartTagRE.ReplaceAllStringFunc(block, func(tag string) string {
			return dmlStrippableAttrRE.ReplaceAllString(tag, "")
		})
		// After attribute strip, drop any `<a:endParaRPr/>` (or
		// `<a:endParaRPr></a:endParaRPr>`) that no longer carries any
		// attribute or child content. Upstream Okapi's
		// ParagraphBlockProperties.getEvents
		// (BlockProperties.java:169-172) drops empty paragraph-property
		// envelopes — the same rule applies to
		// `<a:endParaRPr>` (the paragraph-mark run-property in
		// DrawingML, ECMA-376-1 §21.1.2.2.3). Without this drop, a
		// source `<a:endParaRPr lang="en-CA"/>` (only-strippable-attrs
		// shape) survives the round-trip as `<a:endParaRPr/>` while
		// upstream drops it entirely. apissue.docx has 5 such
		// occurrences in lockedCanvas chart fragments — keeping them
		// adds ~57KB of phantom markup vs the reference output.
		stripped = stripEmptyDMLEndParaRPr(stripped)
		// Hoist common run properties into a paragraph-default-run
		// (`<a:defRPr>` inside `<a:pPr>`) — see dml_style_optimization.go
		// for the upstream Okapi RunParser.java:386-389 +
		// StyleOptimisation.java:96-129 citation. Fixture:
		// DrawingML_Test.docx single-run textbox paragraph.
		stripped = optimiseDMLBlockProperties(stripped)
		out.WriteString(stripped)
		out.WriteString(dmlBlockCloseTag)
		pos = blockEnd + len(dmlBlockCloseTag)
	}
	return out.String()
}

// stripEmptyDMLEndParaRPr removes every empty `<a:endParaRPr/>` (or
// `<a:endParaRPr></a:endParaRPr>` with no children/attrs) from the
// paragraph body. Mirrors upstream Okapi's
// ParagraphBlockProperties.getEvents (BlockProperties.java:169-172)
// empty-envelope drop applied to the DML paragraph-mark rPr — see
// stripDMLRunPropertyAttrs for the apissue.docx canonical case.
//
// We only strip the empty form. A `<a:endParaRPr>` carrying child
// elements (a:solidFill, a:latin, …) is preserved verbatim because
// those children encode rendered formatting per ECMA-376-1
// §21.1.2.2.3 and dropping them would alter the paragraph mark's
// appearance.
func stripEmptyDMLEndParaRPr(block string) string {
	if !strings.Contains(block, "<a:endParaRPr") {
		return block
	}
	var out strings.Builder
	out.Grow(len(block))
	pos := 0
	for pos < len(block) {
		i := strings.Index(block[pos:], "<a:endParaRPr")
		if i < 0 {
			out.WriteString(block[pos:])
			return out.String()
		}
		start := pos + i
		out.WriteString(block[pos:start])
		// Find the end of the tag's open-tag (first '>').
		tagEnd := strings.Index(block[start:], ">")
		if tagEnd < 0 {
			// Malformed — pass through.
			out.WriteString(block[start:])
			return out.String()
		}
		tagEndAbs := start + tagEnd + 1
		openTag := block[start:tagEndAbs]
		// Distinguish self-closing `<a:endParaRPr.../>` from the
		// open-form `<a:endParaRPr...>...</a:endParaRPr>`.
		if strings.HasSuffix(openTag, "/>") {
			// Self-closing. Drop iff every attribute was stripped
			// (the tag is now exactly `<a:endParaRPr/>`).
			if openTag == "<a:endParaRPr/>" {
				pos = tagEndAbs
				continue
			}
			out.WriteString(openTag)
			pos = tagEndAbs
			continue
		}
		// Open form. Look for `</a:endParaRPr>`. If the body between
		// open and close is empty AND the open tag has no attributes,
		// drop the whole envelope.
		closeIdx := strings.Index(block[tagEndAbs:], "</a:endParaRPr>")
		if closeIdx < 0 {
			out.WriteString(block[start:])
			return out.String()
		}
		body := block[tagEndAbs : tagEndAbs+closeIdx]
		closeEnd := tagEndAbs + closeIdx + len("</a:endParaRPr>")
		if openTag == "<a:endParaRPr>" && strings.TrimSpace(body) == "" {
			pos = closeEnd
			continue
		}
		out.WriteString(block[start:closeEnd])
		pos = closeEnd
	}
	return out.String()
}

// expandDrawingMarkers replaces <!--KAPI-PROP:id--> /
// <!--KAPI-PARA:id--> marker comments inside a captured drawing
// payload with rendered translations from the current Write call's
// blocks index. PROP markers (set in place of an attribute value
// at READ time) expand to the property block's xml-attr-escaped
// text. PARA markers (set in place of a textbox-body paragraph's
// runs) expand to the paragraph block's renderWMLBlock output —
// `<w:r><w:t>...</w:t></w:r>` plus any inline-code wrapping.
//
// When a marker has no matching block (defensive: e.g. the reader
// emitted blocks but they were filtered out before reaching the
// writer) the marker is replaced with the empty string. This is
// the same behaviour the skeleton flush has for unresolved refs.
func (w *Writer) expandDrawingMarkers(payload string) string {
	if !strings.Contains(payload, drawingMarkerPropPrefix) &&
		!strings.Contains(payload, drawingMarkerParaPrefix) &&
		!strings.Contains(payload, drawingMarkerTextPrefix) {
		return payload
	}
	return drawingMarkerRE.ReplaceAllStringFunc(payload, func(match string) string {
		m := drawingMarkerRE.FindStringSubmatch(match)
		if len(m) != 3 {
			return ""
		}
		kind, id := m[1], m[2]
		block, ok := w.blocks[id]
		if !ok || block == nil {
			return ""
		}
		runs := w.preferredRuns(block)
		if runs == nil {
			return ""
		}
		switch kind {
		case "PROP":
			return xmlEscapeAttr(model.FlattenRuns(runs))
		case "PARA":
			fieldStraddle := block.Properties != nil && block.Properties["openxml:field-straddle"] == "true"
			return w.renderWMLBlock(runs, blockSourceRPrXML(block), blockPerRunRPrFragments(block), blockPerRunSrcRunStartFlags(block), blockPerRunInFieldDisplayFlags(block), blockPerRunSourceHadRPrFlags(block), fieldStraddle)
		case "TEXT":
			// Character-data marker: emit xml-escaped text only,
			// without the run/text-element wrapper. Used for bare
			// <w:t> elements that live inside opaque markup
			// (e.g. <mc:Choice><w:t>...</w:t></mc:Choice>) where
			// the <w:t> tag itself is preserved verbatim and only
			// its character data needs the translation. Differs
			// from PROP only in the escape rules — text contexts
			// don't need the quote escape that attribute contexts
			// require.
			return xmlEscape(model.FlattenRuns(runs))
		default:
			return ""
		}
	})
}

// renderDMLBlock renders a run sequence as DrawingML runs.
func (w *Writer) renderDMLBlock(runs []model.Run) string {
	if !runsHaveInlineCodes(runs) {
		return `<a:r><a:t>` + xmlEscape(model.FlattenRuns(runs)) + `</a:t></a:r>`
	}

	var buf strings.Builder
	var inRun bool
	var runPropsAttrs []string

	closeRun := func() {
		if inRun {
			buf.WriteString(`</a:t></a:r>`)
			inRun = false
		}
	}

	for _, r := range runs {
		switch {
		case r.Text != nil:
			for _, ch := range r.Text.Text {
				if !inRun {
					buf.WriteString(`<a:r>`)
					if len(runPropsAttrs) > 0 {
						buf.WriteString(`<a:rPr `)
						buf.WriteString(strings.Join(runPropsAttrs, " "))
						buf.WriteString(`/>`)
					}
					buf.WriteString(`<a:t>`)
					inRun = true
				}
				xmlEscapeRune(&buf, ch)
			}

		case r.PcOpen != nil:
			runPropsAttrs = w.addDMLProp(runPropsAttrs, r.PcOpen.Type)

		case r.PcClose != nil:
			closeRun()
			runPropsAttrs = w.removeDMLProp(runPropsAttrs, r.PcClose.Type)

		case r.Ph != nil:
			closeRun()
			if r.Ph.Type == TypeBreak {
				buf.WriteString(`<a:br/>`)
			} else {
				buf.WriteString(r.Ph.Data)
			}
		}
	}

	closeRun()
	return buf.String()
}

func (w *Writer) addDMLProp(attrs []string, spanType string) []string {
	switch spanType {
	case TypeBold:
		return append(attrs, `b="1"`)
	case TypeItalic:
		return append(attrs, `i="1"`)
	case TypeUnderline:
		return append(attrs, `u="sng"`)
	case TypeStrikethrough:
		return append(attrs, `strike="sngStrike"`)
	case TypeSuperscript:
		return append(attrs, `baseline="30000"`)
	case TypeSubscript:
		return append(attrs, `baseline="-25000"`)
	}
	return attrs
}

func (w *Writer) removeDMLProp(attrs []string, spanType string) []string {
	var target string
	switch spanType {
	case TypeBold:
		target = `b="1"`
	case TypeItalic:
		target = `i="1"`
	case TypeUnderline:
		target = `u="sng"`
	case TypeStrikethrough:
		target = `strike="sngStrike"`
	case TypeSuperscript:
		target = `baseline="30000"`
	case TypeSubscript:
		target = `baseline="-25000"`
	default:
		return attrs
	}
	var result []string
	for _, a := range attrs {
		if a != target {
			result = append(result, a)
		}
	}
	return result
}

// renderSMLBlock renders a run sequence as SpreadsheetML content.
func (w *Writer) renderSMLBlock(runs []model.Run, block *model.Block) string {
	if block.Type == "shared-string" {
		return w.renderSMLSharedString(runs)
	}

	// Cell content — wrap in <v> element as inline string type. Flatten
	// to plain text: inline codes in cell values are rare and the legacy
	// path stripped markers via Fragment.Text().
	return `<v>` + xmlEscape(model.FlattenRuns(runs)) + `</v>`
}

// renderSMLSharedString renders a run sequence as shared string <si> content.
func (w *Writer) renderSMLSharedString(runs []model.Run) string {
	if !runsHaveInlineCodes(runs) {
		return `<t>` + xmlEscape(model.FlattenRuns(runs)) + `</t>`
	}

	// Rich text shared string — emit <r> elements
	var buf strings.Builder
	var inRun bool
	var currentProps []string

	closeRun := func() {
		if inRun {
			buf.WriteString(`</t></r>`)
			inRun = false
		}
	}

	for _, r := range runs {
		switch {
		case r.Text != nil:
			for _, ch := range r.Text.Text {
				if !inRun {
					buf.WriteString(`<r>`)
					if len(currentProps) > 0 {
						buf.WriteString(`<rPr>`)
						for _, p := range currentProps {
							buf.WriteString(p)
						}
						buf.WriteString(`</rPr>`)
					}
					buf.WriteString(`<t>`)
					inRun = true
				}
				xmlEscapeRune(&buf, ch)
			}

		case r.PcOpen != nil:
			closeRun()
			currentProps = w.addSMLProp(currentProps, r.PcOpen.Type)

		case r.PcClose != nil:
			closeRun()
			currentProps = w.removeSMLProp(currentProps, r.PcClose.Type)

		case r.Ph != nil:
			// Placeholders are skipped in shared strings (legacy behaviour).
		}
	}

	closeRun()
	return buf.String()
}

func (w *Writer) addSMLProp(props []string, spanType string) []string {
	switch spanType {
	case TypeBold:
		return append(props, `<b/>`)
	case TypeItalic:
		return append(props, `<i/>`)
	case TypeUnderline:
		return append(props, `<u/>`)
	case TypeStrikethrough:
		return append(props, `<strike/>`)
	case TypeSuperscript:
		return append(props, `<vertAlign val="superscript"/>`)
	case TypeSubscript:
		return append(props, `<vertAlign val="subscript"/>`)
	}
	return props
}

func (w *Writer) removeSMLProp(props []string, spanType string) []string {
	var target string
	switch spanType {
	case TypeBold:
		target = `<b/>`
	case TypeItalic:
		target = `<i/>`
	case TypeUnderline:
		target = `<u/>`
	case TypeStrikethrough:
		target = `<strike/>`
	case TypeSuperscript:
		target = `<vertAlign val="superscript"/>`
	case TypeSubscript:
		target = `<vertAlign val="subscript"/>`
	default:
		return props
	}
	var result []string
	for _, p := range props {
		if p != target {
			result = append(result, p)
		}
	}
	return result
}

// preferredRuns returns the target runs for the writer's locale when
// present, falling back to the source runs. Returns nil if neither is
// available, matching the earlier getFragment contract.
func (w *Writer) preferredRuns(block *model.Block) []model.Run {
	if !w.Locale.IsEmpty() && block.HasTarget(w.Locale) {
		segs := block.Targets[w.Locale]
		if len(segs) > 0 && len(segs[0].Runs) > 0 {
			return segs[0].Runs
		}
	}
	if len(block.Source) > 0 && len(block.Source[0].Runs) > 0 {
		return block.Source[0].Runs
	}
	return nil
}
