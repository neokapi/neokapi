// Package openxml — Word style optimisation
//
// Implements Okapi's AllowWordStyleOptimisation transform on
// WordprocessingML output, mirroring upstream Java behaviour:
//
//   - Per paragraph, compute the run-property elements (rPr children) that
//     are present and identical across every <w:r> in the paragraph.
//   - If the common set is non-empty AND no run carries an "exclusion"
//     property (toggle/highlight/etc.), synthesise a paragraph style with
//     basedOn=<paragraph's pStyle, or "Normal" by default> and
//     rPr=<common props>. The styleId follows Okapi's IdGenerator pattern:
//     "NF974E24F-{parentBase}{N}" where {N} is the per-parent sequence
//     starting at 1.
//   - Add <w:pStyle w:val="<id>"/> to the paragraph's pPr.
//   - Strip the common props from each run's rPr.
//
// Upstream references:
//   - StyleOptimisation.java (lines 96-129) — the Default.applyTo loop.
//   - WordStyleDefinitions.java (lines 148-185) — place() of a
//     synthesised style with basedOn/rPr.
//   - WordStyleDefinitions.Ids (lines 445-516) — parentBased/defaultBased
//     ID lookup-or-generate.
//   - IdGenerator.java + Util.makeId — the "NF974E24F" prefix is the
//     hex-formatted Java string-hash of the literal "style" via
//     Util.makeId("style"), used as the IdGenerator root for the
//     openxml-filter style id stream.
//
// This implementation runs as a POST-pass on the writer-emitted
// document.xml, headerN.xml, footerN.xml etc. — it does not change the
// reader/skeleton paths. The set of synthesised styles is collected and
// injected into word/styles.xml via injectSynthesisedStyles.

package openxml

import (
	"bytes"
	"fmt"
	"sort"
	"strings"
)

// styleHashRoot is the hex form of Util.makeId("style") — the literal
// IdGenerator root used by Okapi's WordStyleDefinitions for synthesised
// paragraph styles. This is hard-coded because every Okapi reference
// run uses "style" as the root for the openxml id generator (see
// WordStyleDefinitions.readWith line 114 — IdGenerator(STYLE, STYLE)).
const styleHashRoot = "NF974E24F"

// runPropExclusions are local-names of <w:rPr> children that BLOCK a
// paragraph from being optimised at all when ANY run in it carries one.
//
// Upstream Okapi's WSO exclusion list for WordprocessingML is JUST
// rStyle (see okapi/filters/openxml/WordDocument.java:335-337 — the
// styleOptimisationsFor() factory passes
// Collections.singletonList(rStyle) as the exclusion set when building
// StyleOptimisation.Default for a WML part).
//
// rStyle (character style reference) is run-scoped semantics: it points
// to a character style by id and must remain on each <w:r>. ECMA-376-1
// §17.7.4 (Character Style Definitions). Lifting it into a synthesised
// PARAGRAPH style would silently change the rendered result.
//
// Other rPr children that might look like exclusion candidates:
//   - <w:lang>, <w:noProof>, <w:rPrChange> are stripped from rPr at
//     parse time (parseRunProps) and at writer post-pass time
//     (stripWMLSkippableElements) — mirroring upstream RunSkippableElements
//     (RunSkippableElements.java:50-62). They never reach this map.
//   - Tracked-revision run-property elements (rPrChange, ins, del,
//     moveTo, moveFrom inside rPr) have already been stripped by
//     stripWMLSkippableElements when this function runs.
//
// Native-only over-exclusions (compensating for missing upstream
// behaviour the native pipeline does not yet implement):
//
//   - <w:vanish> (hidden text marker, ECMA-376-1 §17.3.2.42) is excluded
//     pending paragraph-style→run inheritance support in the native
//     reader. Upstream Okapi resolves pStyle inheritance so a hidden run
//     stays hidden after extraction even when vanish is promoted into a
//     synthesised paragraph style; the native reader does not yet do
//     that resolution, so promoting vanish causes a previously hidden
//     run to become extracted as translatable on round-trip
//     (TestRoundtripFormatted regresses without this guard).
//
//   - <w:rtl> (WpmlToggleRunProperty per RunPropertyFactory.java:219)
//     is excluded pending RunProperty.minified() support in the native
//     parser. Upstream's RunPropertiesParser path runs every direct rPr
//     through RunProperties.minified(combined) (RunParser.java:280-294,
//     RunProperties.java:497-540), which strips toggle properties whose
//     value is the no-op default (false) when not in the inherited
//     style hierarchy — so `<w:rtl w:val="0"/>` is removed from the
//     run's rPr BEFORE WSO sees it. Native does not yet implement that
//     minification, so without this exclusion neokapi promotes the
//     redundant `<w:rtl w:val="0"/>` into a synthesised pStyle that
//     upstream does not generate (reordered-zip.docx fixture). The
//     proper fix is to add the minified() pass in parseRunProps and
//     drop this exclusion.
var runPropExclusions = map[string]bool{
	"rStyle": true,
	"vanish": true,
	"rtl":    true,
}

// runProp is a single <w:rPr> child element captured by name and raw
// XML serialization. Two runProps are "equal" when their canonical
// serialization matches (attribute order is preserved from the source —
// canonicalisation happens at the parity layer).
type runProp struct {
	name string // local element name without "w:" prefix (e.g. "rFonts", "lang")
	xml  string // raw element XML, e.g. `<w:rFonts w:ascii="Arial"/>`
}

// synthesisedStyle is a paragraph-style placeholder that was created by
// the optimisation pass. parentID is the basedOn target ("Normal" by
// default).
type synthesisedStyle struct {
	id       string
	parentID string
	rPrXML   string // children only
}

// rawParagraph is a slice into the document buffer covering one
// <w:p>...</w:p> element (start-tag offset to end-tag offset+len).
type rawParagraph struct {
	start int // index of opening "<w:p" in src
	end   int // index just past "</w:p>"
}

// optimizeWMLPart applies AllowWordStyleOptimisation to a
// WordprocessingML XML part. It is the entry point used by the writer
// post-pass for word/document.xml, word/header*.xml, word/footer*.xml,
// word/footnotes.xml, word/endnotes.xml.
//
// idCounters is updated in-place across calls so that styleId
// sequence numbers continue across multi-part documents (matching
// Okapi's single IdGenerator scope for the entire openxml filter
// invocation).
//
// existingStyleIDs contains the styleIds present in the source
// word/styles.xml; an existing match short-circuits new-id generation.
// This is consulted before idCounters because Okapi's parentBased()
// re-uses an existing matching style if one is found.
func optimizeWMLPart(
	src []byte,
	existingStyleIDs map[string]bool,
	idCounters map[string]int,
	synthesised map[string]synthesisedStyle,
	orderedIDs *[]string,
) []byte {
	if len(src) == 0 {
		return src
	}
	if !bytes.Contains(src, []byte("<w:p")) {
		return src
	}

	paragraphs := findParagraphs(src)
	if len(paragraphs) == 0 {
		return src
	}

	var out bytes.Buffer
	out.Grow(len(src) + 1024)
	cursor := 0
	for _, para := range paragraphs {
		out.Write(src[cursor:para.start])
		rewritten := optimizeParagraph(
			src[para.start:para.end],
			existingStyleIDs, idCounters, synthesised, orderedIDs,
		)
		out.Write(rewritten)
		cursor = para.end
	}
	out.Write(src[cursor:])
	return out.Bytes()
}

// findParagraphs walks src and returns the byte ranges of every
// top-level <w:p>...</w:p> element. Paragraphs nested inside <w:tbl>,
// <w:txbxContent> etc. are also found because the matcher is purely
// structural — every "<w:p" with a balanced "</w:p>" qualifies. Self-
// closing <w:p/> paragraphs are skipped (they have no runs).
func findParagraphs(src []byte) []rawParagraph {
	var out []rawParagraph
	openTag := []byte("<w:p")
	closeTag := []byte("</w:p>")
	i := 0
	for i < len(src) {
		idx := bytes.Index(src[i:], openTag)
		if idx < 0 {
			break
		}
		start := i + idx
		// Confirm element-name boundary — next char must be `>`, ` ` or `/`.
		// Reject "<w:pPr", "<w:pgSz", "<w:pgMar", "<w:pStyle", etc.
		j := start + len(openTag)
		if j >= len(src) {
			break
		}
		b := src[j]
		if b != '>' && b != ' ' && b != '\t' && b != '\n' && b != '\r' && b != '/' {
			i = j
			continue
		}
		// Find the start tag's terminator
		k := bytes.IndexByte(src[j:], '>')
		if k < 0 {
			break
		}
		startTagEnd := j + k
		// Self-closing <w:p/>?
		if startTagEnd > 0 && src[startTagEnd-1] == '/' {
			i = startTagEnd + 1
			continue
		}
		// Find matching </w:p> — must skip nested <w:p> (which can occur
		// in textbox content). Simple depth counter on top-level w:p
		// open/close tags.
		depth := 1
		scan := startTagEnd + 1
		for depth > 0 && scan < len(src) {
			ni := bytes.Index(src[scan:], openTag)
			ci := bytes.Index(src[scan:], closeTag)
			if ci < 0 {
				return out // unbalanced — bail
			}
			// If a nested <w:p starts before the next </w:p>, increase depth.
			if ni >= 0 && ni < ci {
				abs := scan + ni
				bj := abs + len(openTag)
				if bj < len(src) {
					bb := src[bj]
					if bb == '>' || bb == ' ' || bb == '\t' || bb == '\n' || bb == '\r' || bb == '/' {
						// Confirm not self-closing
						k := bytes.IndexByte(src[bj:], '>')
						if k >= 0 {
							se := bj + k
							if !(se > 0 && src[se-1] == '/') {
								depth++
							}
							scan = se + 1
							continue
						}
					}
				}
				scan = bj
				continue
			}
			// Match a </w:p>
			depth--
			scan = scan + ci + len(closeTag)
		}
		out = append(out, rawParagraph{start: start, end: scan})
		i = scan
	}
	return out
}

// runEntry captures a parsed <w:r>...</w:r> with its rPr properties.
type runEntry struct {
	runStart, runEnd int // <w:r ...> ... </w:r> range
	rPrStart, rPrEnd int // <w:rPr> ... </w:rPr> range (or zero if no rPr)
	hasRPr           bool
	props            []runProp
	excluded         bool // run carries an exclusion property
}

// optimizeParagraph rewrites a single <w:p>...</w:p> block applying
// AllowWordStyleOptimisation. Returns the original bytes if no
// optimisation is applicable (or if structure is too unusual to
// safely transform).
func optimizeParagraph(
	src []byte,
	existingStyleIDs map[string]bool,
	idCounters map[string]int,
	synthesised map[string]synthesisedStyle,
	orderedIDs *[]string,
) []byte {
	// Find pPr (or its absence). pPr must be the first child if present.
	pPrStart, pPrEnd, hasPPr := findFirstChild(src, "pPr")
	pStyleID := ""
	if hasPPr {
		// Extract any <w:pStyle w:val="..."/> already present.
		pStyleID = extractAttrVal(src[pPrStart:pPrEnd], "pStyle", "w:val")
	}

	// Bail if paragraph contains tracked-revision content wrappers
	// (<w:ins> or <w:del> at content level, NOT inside <w:rPr>) — these
	// confuse the inner-chunks/exclusion checks in Okapi's
	// StyleOptimisation and the safe path is bypass. Native's
	// stripWMLSkippableElements has already removed the empty-form
	// (paragraph-mark) <w:ins>/<w:del> from inside <w:rPr>, so any
	// surviving instance is a content wrapper.
	if containsContentRevisionWrapper(src, pPrStart, pPrEnd, hasPPr) {
		return src
	}

	// Collect run rPr blocks (and the runs themselves so we can rewrite).
	runs := findRuns(src)
	if len(runs) < 1 {
		// Empty paragraph (no runs at all) — nothing to optimise.
		return src
	}
	// Threshold note: upstream Okapi optimises 1-run paragraphs too
	// (StyleOptimisation.Default.applyTo line 98 bypasses only when
	// chunks.size() <= 2 — i.e. 0 runs in addition to outer markup).
	// With #592 the native writer now preserves per-source-run rPr on
	// every emitted <w:r>, so 1-run paragraphs carry the same rPr
	// payload Okapi sees and the optimisation premise — common props
	// across rendered runs — holds for them too. Pre-#592 the native
	// reader/writer aggressively collapsed source runs into a single
	// rPr-less <w:r>, so a 2+ threshold was used as a safety net to
	// avoid synthesising styles upstream did not.
	entries := make([]runEntry, 0, len(runs))
	for _, r := range runs {
		e := runEntry{runStart: r.start, runEnd: r.end}
		rps, rpe, has := findFirstChild(src[r.start:r.end], "rPr")
		if has {
			e.hasRPr = true
			e.rPrStart = r.start + rps
			e.rPrEnd = r.start + rpe
			e.props = parseRunPropElements(src[e.rPrStart:e.rPrEnd])
			for _, p := range e.props {
				if runPropExclusions[p.name] {
					e.excluded = true
					break
				}
			}
		}
		entries = append(entries, e)
	}

	// Compute common props across all runs. If any run has empty rPr,
	// commons is empty (per Okapi: "if direct properties empty,
	// commonRunProperties.clear()").
	for _, e := range entries {
		if e.excluded {
			return src // bypass per StyleOptimisation.innerChunksContainExclusions
		}
		if !e.hasRPr || len(e.props) == 0 {
			return src
		}
	}
	common := commonProps(entries[0].props, entries)
	if len(common) == 0 {
		return src
	}

	// Build the synthesised style id.
	parentID := pStyleID
	if parentID == "" {
		parentID = "Normal"
	}
	commonRPrXML := buildRPrXML(common)
	matchedID := findMatchingStyle(parentID, commonRPrXML, synthesised, *orderedIDs)
	if matchedID == "" {
		// Generate a fresh id "NF974E24F-<parentID><N>"
		idCounters[parentID]++
		seq := idCounters[parentID]
		for {
			candidate := fmt.Sprintf("%s-%s%d", styleHashRoot, parentID, seq)
			if !existingStyleIDs[candidate] {
				matchedID = candidate
				break
			}
			idCounters[parentID]++
			seq = idCounters[parentID]
		}
		synthesised[matchedID] = synthesisedStyle{
			id:       matchedID,
			parentID: parentID,
			rPrXML:   commonRPrXML,
		}
		*orderedIDs = append(*orderedIDs, matchedID)
		existingStyleIDs[matchedID] = true
	}

	// Rewrite paragraph: insert pStyle into pPr (or create pPr) and
	// strip common props from each run's rPr.
	var out bytes.Buffer
	out.Grow(len(src) + 256)
	cursor := 0

	// Insert/update pPr.
	if hasPPr {
		out.Write(src[:pPrStart])
		newPPr := insertPStyle(src[pPrStart:pPrEnd], matchedID)
		out.Write(newPPr)
		cursor = pPrEnd
	} else {
		// Find the start-tag end of <w:p ...> — pPr goes immediately after.
		startTagEnd := bytes.IndexByte(src, '>')
		if startTagEnd < 0 {
			return src
		}
		out.Write(src[:startTagEnd+1])
		out.WriteString(`<w:pPr><w:pStyle w:val="`)
		out.WriteString(matchedID)
		out.WriteString(`"/></w:pPr>`)
		cursor = startTagEnd + 1
	}

	// Now iterate runs, stripping common props from each rPr.
	commonNames := make(map[string]bool, len(common))
	for _, p := range common {
		commonNames[p.name] = true
	}
	for _, e := range entries {
		if e.runStart < cursor {
			// Should not happen, but guard.
			continue
		}
		out.Write(src[cursor:e.runStart])
		runBuf := src[e.runStart:e.runEnd]
		if e.hasRPr && len(commonNames) > 0 {
			runBuf = stripPropsFromRun(runBuf, commonNames)
		}
		out.Write(runBuf)
		cursor = e.runEnd
	}
	out.Write(src[cursor:])
	return out.Bytes()
}

// findFirstChild returns the byte range of the first <w:NAME>...</w:NAME>
// (or self-closing <w:NAME/>) element appearing inside the parent element
// represented by src. start/end are relative to src. The element must
// appear before any other content (children-only matcher won't see
// elements past the first sub-element of a different name — safe enough
// for pPr/rPr lookup which is always the first child if present).
func findFirstChild(src []byte, name string) (int, int, bool) {
	open := []byte("<w:" + name)
	close := []byte("</w:" + name + ">")
	i := bytes.Index(src, open)
	if i < 0 {
		return 0, 0, false
	}
	// Confirm name boundary.
	j := i + len(open)
	if j >= len(src) {
		return 0, 0, false
	}
	b := src[j]
	if b != '>' && b != ' ' && b != '\t' && b != '\n' && b != '\r' && b != '/' {
		return 0, 0, false
	}
	k := bytes.IndexByte(src[j:], '>')
	if k < 0 {
		return 0, 0, false
	}
	startTagEnd := j + k
	// Self-closing form.
	if startTagEnd > 0 && src[startTagEnd-1] == '/' {
		return i, startTagEnd + 1, true
	}
	// Open form — find matching close (no nesting in pPr/rPr in WML).
	ci := bytes.Index(src[startTagEnd+1:], close)
	if ci < 0 {
		return 0, 0, false
	}
	return i, startTagEnd + 1 + ci + len(close), true
}

// extractAttrVal scans src for an element <w:elem ... attr="VAL"...>
// and returns VAL. Returns "" if not found.
func extractAttrVal(src []byte, elemName, attr string) string {
	open := []byte("<w:" + elemName)
	i := bytes.Index(src, open)
	if i < 0 {
		return ""
	}
	j := i + len(open)
	if j >= len(src) {
		return ""
	}
	b := src[j]
	if b != '>' && b != ' ' && b != '\t' && b != '\n' && b != '\r' && b != '/' {
		return ""
	}
	k := bytes.IndexByte(src[j:], '>')
	if k < 0 {
		return ""
	}
	startTag := string(src[j : j+k])
	// Locate attr=
	ai := strings.Index(startTag, attr+"=")
	if ai < 0 {
		return ""
	}
	rest := startTag[ai+len(attr)+1:]
	if len(rest) == 0 {
		return ""
	}
	q := rest[0]
	if q != '"' && q != '\'' {
		return ""
	}
	end := strings.IndexByte(rest[1:], q)
	if end < 0 {
		return ""
	}
	return rest[1 : 1+end]
}

// rawRun is a byte range covering one <w:r>...</w:r> element.
type rawRun struct {
	start, end int
}

// findRuns returns every top-level <w:r>...</w:r> element inside src.
// "Top-level" here means it doesn't recurse into nested runs (which
// don't exist — runs cannot contain runs in WML), but it DOES find
// runs nested inside hyperlinks/sdt/ins-content-wrappers because the
// scan is purely sequential.
func findRuns(src []byte) []rawRun {
	var out []rawRun
	open := []byte("<w:r")
	close := []byte("</w:r>")
	i := 0
	for i < len(src) {
		idx := bytes.Index(src[i:], open)
		if idx < 0 {
			return out
		}
		start := i + idx
		j := start + len(open)
		if j >= len(src) {
			return out
		}
		b := src[j]
		if b != '>' && b != ' ' && b != '\t' && b != '\n' && b != '\r' && b != '/' {
			// Not a w:r — could be w:rPr, w:rFonts, w:rStyle, etc.
			i = j
			continue
		}
		k := bytes.IndexByte(src[j:], '>')
		if k < 0 {
			return out
		}
		startTagEnd := j + k
		// Self-closing <w:r/> — empty run, skip.
		if startTagEnd > 0 && src[startTagEnd-1] == '/' {
			i = startTagEnd + 1
			continue
		}
		ci := bytes.Index(src[startTagEnd+1:], close)
		if ci < 0 {
			return out
		}
		end := startTagEnd + 1 + ci + len(close)
		out = append(out, rawRun{start: start, end: end})
		i = end
	}
	return out
}

// parseRunPropElements parses the children of a <w:rPr>...</w:rPr>
// block (src includes the enclosing tags). It returns a slice of
// runProp records preserving source order. Each runProp captures both
// the local element name and the literal serialization (so order-
// sensitive attribute equality works).
func parseRunPropElements(src []byte) []runProp {
	// Strip the wrapping <w:rPr>...</w:rPr> or <w:rPr/>.
	open := []byte("<w:rPr")
	close := []byte("</w:rPr>")
	i := bytes.Index(src, open)
	if i < 0 {
		return nil
	}
	startTagEnd := bytes.IndexByte(src[i:], '>')
	if startTagEnd < 0 {
		return nil
	}
	// Self-closing rPr — no children.
	if startTagEnd > 0 && src[i+startTagEnd-1] == '/' {
		return nil
	}
	body := src[i+startTagEnd+1:]
	// Trim trailing </w:rPr>.
	if ci := bytes.Index(body, close); ci >= 0 {
		body = body[:ci]
	}
	// Now scan body for child elements (no text content expected;
	// every child is a property element). Each element is either
	// self-closing or open/close-balanced (no nesting in rPr).
	var out []runProp
	for j := 0; j < len(body); {
		if body[j] != '<' {
			j++
			continue
		}
		// Must be of form <w:NAME ...
		if !bytes.HasPrefix(body[j:], []byte("<w:")) {
			// Non-w: child — skip the tag (could be e.g. <w14:foo/>
			// extension).
			tagEnd := bytes.IndexByte(body[j:], '>')
			if tagEnd < 0 {
				break
			}
			// Self-closing? Skip it. Otherwise skip up to matching close —
			// rare; conservative implementation just records the tag verbatim.
			selfClose := tagEnd > 0 && body[j+tagEnd-1] == '/'
			if selfClose {
				out = append(out, runProp{name: extractLocal(body[j : j+tagEnd+1]), xml: string(body[j : j+tagEnd+1])})
				j = j + tagEnd + 1
				continue
			}
			// Find balanced close.
			localName := extractLocal(body[j : j+tagEnd+1])
			if localName == "" {
				j = j + tagEnd + 1
				continue
			}
			// Look for the closest matching </prefix:localName>
			closeNeedle := []byte("</" + extractPrefixedName(body[j:j+tagEnd+1]) + ">")
			endIdx := bytes.Index(body[j+tagEnd+1:], closeNeedle)
			if endIdx < 0 {
				j = j + tagEnd + 1
				continue
			}
			elemEnd := j + tagEnd + 1 + endIdx + len(closeNeedle)
			out = append(out, runProp{name: localName, xml: string(body[j:elemEnd])})
			j = elemEnd
			continue
		}
		// <w:NAME ...
		tagEnd := bytes.IndexByte(body[j:], '>')
		if tagEnd < 0 {
			break
		}
		nameEnd := bytes.IndexAny(body[j+3:], " \t\n\r/>")
		if nameEnd < 0 {
			break
		}
		name := string(body[j+3 : j+3+nameEnd])
		// Self-closing?
		if tagEnd > 0 && body[j+tagEnd-1] == '/' {
			out = append(out, runProp{name: name, xml: string(body[j : j+tagEnd+1])})
			j = j + tagEnd + 1
			continue
		}
		// Open form — find matching </w:NAME>.
		closeNeedle := []byte("</w:" + name + ">")
		endIdx := bytes.Index(body[j+tagEnd+1:], closeNeedle)
		if endIdx < 0 {
			j = j + tagEnd + 1
			continue
		}
		elemEnd := j + tagEnd + 1 + endIdx + len(closeNeedle)
		out = append(out, runProp{name: name, xml: string(body[j:elemEnd])})
		j = elemEnd
	}
	return out
}

// extractLocal returns the local element name from a tag like
// "<w:rFonts ..." or "<w14:foo ..." → "rFonts", "foo".
func extractLocal(tag []byte) string {
	if len(tag) < 2 || tag[0] != '<' {
		return ""
	}
	s := tag[1:]
	if i := bytes.IndexByte(s, ':'); i >= 0 {
		s = s[i+1:]
	}
	end := bytes.IndexAny(s, " \t\n\r/>")
	if end < 0 {
		return string(s)
	}
	return string(s[:end])
}

// extractPrefixedName returns "prefix:local" from a tag like
// "<w14:foo ..." → "w14:foo".
func extractPrefixedName(tag []byte) string {
	if len(tag) < 2 || tag[0] != '<' {
		return ""
	}
	s := tag[1:]
	end := bytes.IndexAny(s, " \t\n\r/>")
	if end < 0 {
		return string(s)
	}
	return string(s[:end])
}

// commonProps returns the run-property elements present and equal
// (by exact xml serialization) in EVERY run-entry. Order is preserved
// from the first run.
//
// <w:rFonts> is special-cased: the common rFonts is the per-attribute
// intersection of every run's rFonts (an attribute is kept iff every
// run that has rFonts agrees on the value AND every run has rFonts).
// This mirrors upstream Okapi's behaviour: RunMerger fuses adjacent
// runs whose rFonts are mergeable (RunFonts.canBeMerged + RunFonts.merge)
// BEFORE StyleOptimisation runs, so by the time WSO sees the runs, all
// rFonts are already the merged consensus. We don't have RunMerger in
// the post-write pass, so we compute the consensus here. The intersection
// rule is the safe approximation of Okapi's merge logic for plain-text
// runs (where the COMPLEX_SCRIPT/EAST_ASIAN content categories aren't
// "detected" and thus don't carry extra attributes through the merge).
//
// Per ECMA-376-1 §17.3.2.26 (rFonts), the ascii, hAnsi, cs, eastAsia
// (and corresponding theme variants) attributes are independent: an
// rFonts element may carry any subset. The intersection of attribute/
// value pairs is therefore a valid rFonts and a faithful per-run common
// font specification.
//
// References:
//   - okapi/filters/openxml/RunFonts.java lines 190-247 (canBeMerged,
//     mergeContentCategories) — upstream merge contract.
//   - okapi/filters/openxml/StyleOptimisation.java lines 204-238
//     (commonRunPropertiesOf) — exact-equality List.retainAll on
//     post-merge runs.
//   - ECMA-376-1 4th ed §17.3.2.26 (rFonts).
func commonProps(seed []runProp, entries []runEntry) []runProp {
	if len(entries) == 0 {
		return nil
	}
	out := make([]runProp, 0, len(seed))
	rFontsHandled := false
	for _, p := range seed {
		if p.name == "rFonts" {
			if rFontsHandled {
				continue
			}
			rFontsHandled = true
			if merged, ok := commonRFonts(entries); ok {
				out = append(out, runProp{name: "rFonts", xml: merged})
			}
			continue
		}
		all := true
		for _, e := range entries {
			found := false
			for _, q := range e.props {
				if q.name == p.name && q.xml == p.xml {
					found = true
					break
				}
			}
			if !found {
				all = false
				break
			}
		}
		if all {
			out = append(out, p)
		}
	}
	return out
}

// commonRFonts computes the per-attribute intersection of every run's
// <w:rFonts>. Returns the synthesised rFonts XML (with attribute order
// matching the first run that has rFonts) and true iff the intersection
// is non-empty AND every run has an rFonts.
//
// Attribute equality is by exact (name, value) pair. Attribute name
// uses the namespace-prefixed form as it appears in the source (e.g.
// "w:ascii"); the value is compared after stripping its quote
// character. Both forms are preserved in the emitted rFonts.
func commonRFonts(entries []runEntry) (string, bool) {
	if len(entries) == 0 {
		return "", false
	}
	// Every entry must have exactly one rFonts (the typical case;
	// duplicate rFonts within a single rPr is invalid per ECMA-376
	// schema and would indicate malformed input — skip optimisation).
	var firstAttrs []rfontsAttr
	allAttrSets := make([]map[string]string, len(entries))
	for i, e := range entries {
		var rfonts *runProp
		for k := range e.props {
			if e.props[k].name == "rFonts" {
				if rfonts != nil {
					return "", false // duplicate rFonts in one rPr
				}
				rfonts = &e.props[k]
			}
		}
		if rfonts == nil {
			return "", false // a run lacks rFonts → not common
		}
		attrs, ok := parseRFontsAttrs(rfonts.xml)
		if !ok {
			return "", false
		}
		if i == 0 {
			firstAttrs = attrs
		}
		m := make(map[string]string, len(attrs))
		for _, a := range attrs {
			m[a.name] = a.value
		}
		allAttrSets[i] = m
	}
	// Walk the first run's attribute order; keep an attribute iff every
	// other run has the same name with the same value.
	var kept []rfontsAttr
	for _, a := range firstAttrs {
		ok := true
		for j := 1; j < len(allAttrSets); j++ {
			v, present := allAttrSets[j][a.name]
			if !present || v != a.value {
				ok = false
				break
			}
		}
		if ok {
			kept = append(kept, a)
		}
	}
	if len(kept) == 0 {
		return "", false
	}
	// Re-emit. Preserve the source rFonts element name prefix (likely
	// "w:rFonts" but could differ).
	prefix := extractRFontsElemNameFromProps(entries[0].props)
	if prefix == "" {
		prefix = "w:rFonts"
	}
	var b strings.Builder
	b.WriteByte('<')
	b.WriteString(prefix)
	for _, a := range kept {
		b.WriteByte(' ')
		b.WriteString(a.name)
		b.WriteByte('=')
		q := a.quote
		if q == 0 {
			q = '"'
		}
		b.WriteByte(q)
		b.WriteString(a.value)
		b.WriteByte(q)
	}
	b.WriteString("/>")
	return b.String(), true
}

// extractRFontsElemNameFromProps returns the prefixed element name of the first
// rFonts found in props, e.g. "w:rFonts". Returns "" if not found.
func extractRFontsElemNameFromProps(props []runProp) string {
	for _, p := range props {
		if p.name != "rFonts" {
			continue
		}
		// Tag is like "<w:rFonts ...>" — extract up to first space/slash/>.
		if len(p.xml) < 2 || p.xml[0] != '<' {
			return ""
		}
		end := strings.IndexAny(p.xml[1:], " \t\n\r/>")
		if end < 0 {
			return ""
		}
		return p.xml[1 : 1+end]
	}
	return ""
}

// rfontsAttr captures one parsed rFonts attribute.
type rfontsAttr struct {
	name  string // prefixed name as in source, e.g. "w:ascii"
	value string // unescaped value (quotes stripped)
	quote byte
}

// parseRFontsAttrs parses attributes of a self-closing or open-form
// <w:rFonts ...> element. Returns the attribute list in source order.
// Returns false if the element is malformed.
func parseRFontsAttrs(xmlStr string) ([]rfontsAttr, bool) {
	if len(xmlStr) < 2 || xmlStr[0] != '<' {
		return nil, false
	}
	// Skip element name.
	nameEnd := strings.IndexAny(xmlStr[1:], " \t\n\r/>")
	if nameEnd < 0 {
		return nil, false
	}
	rest := xmlStr[1+nameEnd:]
	// Find end of start-tag.
	tagEnd := strings.IndexByte(rest, '>')
	if tagEnd < 0 {
		return nil, false
	}
	body := rest[:tagEnd]
	if len(body) > 0 && body[len(body)-1] == '/' {
		body = body[:len(body)-1]
	}
	var attrs []rfontsAttr
	i := 0
	for i < len(body) {
		// Skip whitespace.
		for i < len(body) && (body[i] == ' ' || body[i] == '\t' || body[i] == '\n' || body[i] == '\r') {
			i++
		}
		if i >= len(body) {
			break
		}
		// Read name up to '='.
		eq := strings.IndexByte(body[i:], '=')
		if eq < 0 {
			return nil, false
		}
		name := strings.TrimRight(body[i:i+eq], " \t\n\r")
		i += eq + 1
		// Skip whitespace.
		for i < len(body) && (body[i] == ' ' || body[i] == '\t' || body[i] == '\n' || body[i] == '\r') {
			i++
		}
		if i >= len(body) {
			return nil, false
		}
		q := body[i]
		if q != '"' && q != '\'' {
			return nil, false
		}
		i++
		end := strings.IndexByte(body[i:], q)
		if end < 0 {
			return nil, false
		}
		val := body[i : i+end]
		i += end + 1
		attrs = append(attrs, rfontsAttr{name: name, value: val, quote: q})
	}
	return attrs, true
}

// buildRPrXML emits the children-only serialization of the common
// props (no enclosing <w:rPr>...</w:rPr>).
func buildRPrXML(props []runProp) string {
	if len(props) == 0 {
		return ""
	}
	var b strings.Builder
	for _, p := range props {
		b.WriteString(p.xml)
	}
	return b.String()
}

// findMatchingStyle searches both the existing-source and the
// in-progress synthesised set for a paragraph style with the same
// parent and identical rPr body. Returns the styleId or "" if none.
//
// Mirrors WordStyleDefinitions.Ids.parentBased() — Okapi's optimiser
// re-uses an existing matching synthesised style instead of creating a
// new one.
//
// We don't search the source's existing styles (they're paragraph
// styles whose rPr would need a full-tree comparison). The
// optimisation only checks in-pass synthesised styles.
func findMatchingStyle(
	parentID string,
	rPrXML string,
	synthesised map[string]synthesisedStyle,
	orderedIDs []string,
) string {
	for _, id := range orderedIDs {
		s := synthesised[id]
		if s.parentID == parentID && s.rPrXML == rPrXML {
			return id
		}
	}
	return ""
}

// insertPStyle returns a new <w:pPr>...</w:pPr> block with
// <w:pStyle w:val="<id>"/> inserted as the FIRST child. Okapi places
// pStyle as the first child of pPr (per ParagraphBlockProperties.refine).
//
// If the existing pPr already has a pStyle, it is REPLACED with the
// new one (Okapi's refine() overrides the paragraphStyle slot).
func insertPStyle(src []byte, id string) []byte {
	// Self-closing <w:pPr/> — convert to open/close with pStyle child.
	if bytes.HasSuffix(bytes.TrimSpace(src), []byte("/>")) {
		// Find "/>" and replace.
		idx := bytes.LastIndex(src, []byte("/>"))
		if idx < 0 {
			return src
		}
		var b bytes.Buffer
		b.Write(src[:idx])
		b.WriteString(`><w:pStyle w:val="`)
		b.WriteString(id)
		b.WriteString(`"/></w:pPr>`)
		return b.Bytes()
	}
	// Open form — find start-tag end.
	startTagEnd := bytes.IndexByte(src, '>')
	if startTagEnd < 0 {
		return src
	}
	// Strip an existing first-child <w:pStyle ...>. The captured pPr
	// may carry pStyle in either self-closing form ("<w:pStyle w:val=
	// \"...\"/>") OR open/close form ("<w:pStyle w:val=\"...\"></w:pStyle>"
	// ; encoding/xml's Decoder/Encoder cycle re-emits captureRawElement
	// payloads in the latter form even when the source was self-closing
	// — which exposes the strip-only-self-closing path as a #592
	// regression for fixtures whose pPr was lifted into a synthesised
	// pStyle by the WSO post-pass).
	body := src[startTagEnd+1:]
	if bytes.HasPrefix(bytes.TrimLeft(body, " \t\n\r"), []byte("<w:pStyle")) {
		// Skip leading whitespace
		ws := 0
		for ws < len(body) && (body[ws] == ' ' || body[ws] == '\t' || body[ws] == '\n' || body[ws] == '\r') {
			ws++
		}
		// pStyle character after the prefix must be a name boundary
		// (' ', '/', '>') — otherwise we matched something like
		// <w:pStyleId> by accident.
		boundaryAt := ws + len("<w:pStyle")
		if boundaryAt < len(body) {
			b := body[boundaryAt]
			if b == ' ' || b == '\t' || b == '\n' || b == '\r' || b == '/' || b == '>' {
				// Find the start tag's '>' terminator.
				tagEnd := bytes.IndexByte(body[ws:], '>')
				if tagEnd >= 0 {
					absTagEnd := ws + tagEnd
					// Self-closing — element ends here.
					if absTagEnd > 0 && body[absTagEnd-1] == '/' {
						body = body[absTagEnd+1:]
					} else {
						// Open form — skip past matching </w:pStyle>.
						closeNeedle := []byte("</w:pStyle>")
						closeIdx := bytes.Index(body[absTagEnd+1:], closeNeedle)
						if closeIdx >= 0 {
							body = body[absTagEnd+1+closeIdx+len(closeNeedle):]
						}
					}
				}
			}
		}
	}
	var b bytes.Buffer
	b.Write(src[:startTagEnd+1])
	b.WriteString(`<w:pStyle w:val="`)
	b.WriteString(id)
	b.WriteString(`"/>`)
	b.Write(body)
	return b.Bytes()
}

// stripPropsFromRun removes named property elements from the <w:rPr>
// inside a <w:r>...</w:r> block. If the resulting rPr is empty, the
// rPr container itself is removed (matching the
// wmlEmptyPropertiesContainerRE post-pass).
func stripPropsFromRun(runSrc []byte, names map[string]bool) []byte {
	rps, rpe, has := findFirstChild(runSrc, "rPr")
	if !has {
		return runSrc
	}
	rPrSrc := runSrc[rps:rpe]
	props := parseRunPropElements(rPrSrc)
	var kept []runProp
	for _, p := range props {
		if !names[p.name] {
			kept = append(kept, p)
		}
	}
	var newRPr bytes.Buffer
	if len(kept) == 0 {
		// Remove rPr entirely.
		var out bytes.Buffer
		out.Write(runSrc[:rps])
		out.Write(runSrc[rpe:])
		return out.Bytes()
	}
	// Re-emit rPr with kept props, preserving the original opening tag
	// (which may carry namespace declarations).
	openEnd := bytes.IndexByte(rPrSrc, '>')
	if openEnd < 0 {
		return runSrc
	}
	newRPr.Write(rPrSrc[:openEnd+1])
	for _, p := range kept {
		newRPr.WriteString(p.xml)
	}
	newRPr.WriteString(`</w:rPr>`)
	var out bytes.Buffer
	out.Write(runSrc[:rps])
	out.Write(newRPr.Bytes())
	out.Write(runSrc[rpe:])
	return out.Bytes()
}

// injectSynthesisedStyles inserts synthesised <w:style> elements into
// word/styles.xml just before the closing </w:styles> tag. Order is
// the orderedIDs slice (insertion order).
//
// Mirrors WordStyleDefinitions.asMarkup (lines 429-443) — synthesised
// styles append to the end of the styles list as their place() calls
// occur.
func injectSynthesisedStyles(stylesXML []byte, synthesised map[string]synthesisedStyle, orderedIDs []string) []byte {
	if len(orderedIDs) == 0 {
		return stylesXML
	}
	closeTag := []byte("</w:styles>")
	idx := bytes.LastIndex(stylesXML, closeTag)
	if idx < 0 {
		return stylesXML
	}
	var inj bytes.Buffer
	for _, id := range orderedIDs {
		s := synthesised[id]
		inj.WriteString(`<w:style w:type="paragraph" w:styleId="`)
		inj.WriteString(s.id)
		inj.WriteString(`"><w:name w:val="`)
		inj.WriteString(s.id)
		inj.WriteString(`"/><w:basedOn w:val="`)
		inj.WriteString(s.parentID)
		inj.WriteString(`"/>`)
		if s.rPrXML != "" {
			inj.WriteString(`<w:rPr>`)
			inj.WriteString(s.rPrXML)
			inj.WriteString(`</w:rPr>`)
		}
		inj.WriteString(`</w:style>`)
	}
	var out bytes.Buffer
	out.Grow(len(stylesXML) + inj.Len())
	out.Write(stylesXML[:idx])
	out.Write(inj.Bytes())
	out.Write(stylesXML[idx:])
	return out.Bytes()
}

// extractExistingStyleIDs scans word/styles.xml for every
// w:styleId="..." attribute and returns the set. This is consulted
// during id generation to avoid colliding with a pre-existing styleId.
func extractExistingStyleIDs(stylesXML []byte) map[string]bool {
	out := make(map[string]bool)
	cursor := 0
	needle := []byte(`w:styleId="`)
	for {
		idx := bytes.Index(stylesXML[cursor:], needle)
		if idx < 0 {
			break
		}
		start := cursor + idx + len(needle)
		end := bytes.IndexByte(stylesXML[start:], '"')
		if end < 0 {
			break
		}
		out[string(stylesXML[start:start+end])] = true
		cursor = start + end + 1
	}
	return out
}

// _ keeps sort imported for future use (sorted-id traversal).
var _ = sort.Strings

// containsContentRevisionWrapper reports whether the paragraph src has
// any <w:ins>...</w:ins> or <w:del>...</w:del> CONTENT wrapper (i.e.
// outside of pPr's rPr, where the empty-form paragraph-mark variants
// would be — those have already been stripped by
// stripWMLSkippableElements in our pipeline). When this is true,
// StyleOptimisation must bypass the paragraph: the content wrappers
// imply tracked-revision inserted/deleted runs whose rPr should not
// participate in common-property extraction (Okapi's
// auto-accept-revisions semantics handle these specially via Block /
// RunBuilder routing that the post-write pass cannot replicate).
func containsContentRevisionWrapper(src []byte, pPrStart, pPrEnd int, hasPPr bool) bool {
	// Scan for "<w:ins" or "<w:del" outside the pPr range.
	scan := func(needle []byte) bool {
		i := 0
		for i < len(src) {
			idx := bytes.Index(src[i:], needle)
			if idx < 0 {
				return false
			}
			at := i + idx
			i = at + len(needle)
			// Skip if inside the pPr.
			if hasPPr && at >= pPrStart && at < pPrEnd {
				continue
			}
			// Confirm element-name boundary.
			if at+len(needle) >= len(src) {
				return false
			}
			b := src[at+len(needle)]
			if b == '>' || b == ' ' || b == '\t' || b == '\n' || b == '\r' || b == '/' {
				return true
			}
		}
		return false
	}
	return scan([]byte("<w:ins")) || scan([]byte("<w:del")) ||
		scan([]byte("<w:moveTo")) || scan([]byte("<w:moveFrom"))
}
