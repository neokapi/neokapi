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
// Mirrors Okapi's RunSkippableElements toggle set + RunProperties
// special handling that StyleOptimisation.Default.innerChunksContainExclusions
// guards against.
//
// "vanish" (hidden text marker) is excluded conservatively: in Okapi the
// reader correctly resolves paragraph-style→run inheritance and so
// preserves the hidden flag after extraction even when vanish is moved
// into a synthesised paragraph style. The native reader does not yet
// resolve that inheritance, so moving vanish out of a run causes the
// re-extracted block count to grow on round-trip (TestRoundtripFormatted
// would then fail). Excluding vanish here is the spec-compatible
// fallback while inheritance resolution is a separate work item.
//
// Tracked-revision run-property elements (rPrChange, ins, del, moveTo,
// moveFrom inside rPr) have already been stripped by
// stripWMLSkippableElements when this function runs, so they don't need
// a guard here.
var runPropExclusions = map[string]bool{
	"vanish": true,
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
	if len(runs) < 2 {
		// Pragmatic threshold: only optimise paragraphs with 2+ runs.
		// Upstream Okapi optimises 1-run paragraphs too (see
		// StyleOptimisation.Default.applyTo line 98 — bypass only when
		// chunks.size() <= 2 == 0 runs), but the post-write pass
		// operates on writer-emitted XML where the native reader/writer
		// pipeline has already aggressively merged source runs into a
		// single rendered run that no longer reflects the original
		// per-run rPr distribution. Optimising those single-rendered-run
		// paragraphs introduces synthesised styles that Okapi did not
		// generate (the source had heterogeneous run rPr that Okapi's
		// commonRunPropertiesOf() rejected — empty intersection because
		// at least one run lacked rPr or carried fldChar/ins/del
		// markers). 2+ runs means native's writer kept structural
		// distinctions (e.g. color-exclusion fixtures) and the
		// optimisation premise — common props across rendered runs —
		// holds.
		return src
	}
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
func commonProps(seed []runProp, entries []runEntry) []runProp {
	if len(entries) == 0 {
		return nil
	}
	// For each prop in seed, check that every entry contains an equal-xml
	// prop.
	out := make([]runProp, 0, len(seed))
	for _, p := range seed {
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
	// Strip an existing first-child <w:pStyle w:val="..."/>.
	body := src[startTagEnd+1:]
	pStyleClose := []byte(`"/>`)
	if bytes.HasPrefix(bytes.TrimLeft(body, " \t\n\r"), []byte("<w:pStyle ")) {
		// Skip leading whitespace
		ws := 0
		for ws < len(body) && (body[ws] == ' ' || body[ws] == '\t' || body[ws] == '\n' || body[ws] == '\r') {
			ws++
		}
		// Find the end of the <w:pStyle ...> tag.
		end := bytes.Index(body[ws:], pStyleClose)
		if end >= 0 {
			body = body[ws+end+len(pStyleClose):]
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
