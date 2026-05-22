// Package openxml — DrawingML paragraph-default-run style optimisation.
//
// Implements upstream Okapi's StyleOptimisation.Default for DrawingML
// paragraphs (`<a:p>`).
//
// Unlike the WordprocessingML side — where native is FAITHFUL and does
// NOT synthesise paragraph styles (the former WSO post-pass was deleted;
// equivalence with Okapi's compact form is proved in the parity
// comparator) — DrawingML payloads are captured as opaque XML by the WML
// reader and replayed verbatim, so the only place to reproduce Okapi's
// `<a:defRPr>` hoist is here, during the always-on post-skeleton flush.
// This pass therefore runs unconditionally on every captured drawing,
// regardless of any writer style-optimisation setting.
//
// Per ECMA-376-1 §21.1.2.2.6 (DrawingML CT_TextParagraph) a paragraph
// may carry a single `<a:pPr>` whose `<a:defRPr>` child holds the
// default run properties applied to runs that omit their own `<a:rPr>`.
// When every `<a:r>` in the paragraph shares the same `<a:rPr>` shape,
// upstream Okapi hoists those common run properties up into the
// paragraph's `<a:pPr><a:defRPr/></a:pPr>` and strips them from each
// run — see StyleOptimisation.Default.applyTo
// (okapi/filters/openxml/src/main/java/net/sf/okapi/filters/openxml/
// StyleOptimisation.java:96-129) wired in from RunParser.java:386-389
// where the `Namespace.PREFIX_A` branch builds a fresh
// StyleOptimisation.Default with `ParagraphBlockProperties.PPR` /
// `RunProperties.DEF_RPR` as the block + inner-property names and an
// empty exclusion list (Collections.emptyList() at line 388).
//
// Native's WML reader captures the entire `<w:drawing>` payload as
// opaque XML and replays it via writeDrawingXMLToSkel (wml.go), so the
// optimisation has to run post-skeleton like stripDMLRunPropertyAttrs.
// We piggyback on the same per-`<a:p>` walk in stripDMLRunPropertyAttrs
// — see the call from writer.go just after stripEmptyDMLEndParaRPr.
//
// Fixture: DrawingML_Test.docx — the `Important` textbox paragraph has
// a single `<a:r>` whose `<a:rPr sz="2000" b="1"><a:solidFill>…
// </a:solidFill></a:rPr>` matches the paragraph's endParaRPr shape.
// Upstream hoists those props into `<a:pPr algn="ctr"><a:defRPr sz=
// "2000" b="1"><a:solidFill>…</a:solidFill></a:defRPr></a:pPr>` and
// leaves the run bare (`<a:r><a:t>Important</a:t></a:r>`). Without this
// pass native diverges with a literal `<a:r><a:rPr …>…</a:rPr><a:t>…
// </a:t></a:r>` shape.

package openxml

import (
	"regexp"
	"sort"
	"strings"
)

// dmlRunOpenTagRE matches the start of an `<a:r>` element (the opening
// tag of a DrawingML run). The trailing word boundary keeps the regex
// from matching `<a:rPr>` / `<a:rng>` / `<a:rgb>` etc.
var dmlRunOpenTagRE = regexp.MustCompile(`<a:r(?:\s[^>]*)?>`)

// dmlRunCloseTag is the literal `</a:r>` closing tag.
const dmlRunCloseTag = "</a:r>"

// dmlAttrRE captures one ` name="value"` attribute pair (with leading
// whitespace). Used to compare run-property attribute sets after they
// have already had Okapi's strippable attributes removed.
var dmlAttrRE = regexp.MustCompile(`\s+([a-zA-Z_:][\w:-]*)="([^"]*)"`)

// optimiseDMLBlockProperties applies upstream Okapi's
// StyleOptimisation.Default for DrawingML paragraphs to one already-
// stripped `<a:p>` body. The input is the slice BETWEEN the `<a:p…>`
// opening tag and the `</a:p>` closing tag — same shape stripped by
// stripDMLRunPropertyAttrs in the same loop.
//
// Behaviour mirrors StyleOptimisation.Default.applyTo:
//
//  1. Locate every direct `<a:r>` run inside the paragraph and capture
//     its `<a:rPr>` (attrs + children). Runs without an rPr (empty
//     direct properties) abort optimisation per
//     commonRunPropertiesOf line 225-228.
//  2. Compute the intersection of run-property "items" across every
//     run. An item is either a single attribute name=value pair on
//     `<a:rPr>` or one direct child element of `<a:rPr>`. Two items
//     match when their canonical serialisation is identical. Mirrors
//     Property.equals which compares the underlying XMLEvent name +
//     value.
//  3. If the intersection is empty, bail (nothing to optimise).
//  4. Construct `<a:defRPr ...>...</a:defRPr>` from the common items
//     and insert it as the first child of `<a:pPr>`. When the paragraph
//     has no `<a:pPr>`, synthesise one. Mirrors
//     paragraphBlockPropertiesOf (StyleOptimisation.java:158-185) +
//     refine (lines 120-124) which call updateOrAddBlockProperties on
//     the first markup chunk.
//  5. Remove the common items from each run's `<a:rPr>`; when that
//     leaves the rPr fully empty (no attrs, no children), drop the
//     `<a:rPr>` element entirely. Mirrors refineRunProperties via
//     Run.refineRunProperties (Run.java) which strips the matching
//     properties from each run's direct property pair.
//
// The exclusion list is empty for DrawingML (RunParser.java line 388
// passes Collections.emptyList()), so we do not need to check for
// excluded properties before optimising.
func optimiseDMLBlockProperties(blockBody string) string {
	// Cheap reject: need at least one `<a:r>` AND one `<a:rPr>` to be
	// a candidate. A paragraph with only a `<a:pPr>` and an
	// `<a:endParaRPr>` (no runs at all, or empty runs without rPr) is
	// always a no-op.
	if !strings.Contains(blockBody, "<a:r") || !strings.Contains(blockBody, "<a:rPr") {
		return blockBody
	}

	runs := findDMLRunsWithRPr(blockBody)
	if len(runs) == 0 {
		return blockBody
	}

	// Compute the intersection across every run's rPr. A single run
	// with rPr is a valid candidate (its entire rPr becomes the
	// hoisted defRPr); two runs with disjoint rPrs leave nothing to
	// hoist; etc.
	commonAttrs, commonChildren := intersectDMLRunProperties(runs)
	if len(commonAttrs) == 0 && len(commonChildren) == 0 {
		return blockBody
	}

	// Build the defRPr from the common items, ordered to match
	// upstream Okapi's RunProperty serialisation: attributes appear on
	// the start tag in the order they were captured, children appear
	// in the order they appear in the first run's rPr. Since we copy
	// from the first run's parsed rPr, the relative order is
	// preserved.
	defRPr := buildDMLDefRPr(commonAttrs, commonChildren)

	// Hoist defRPr into the paragraph's `<a:pPr>` (creating one when
	// missing).
	withPPr := insertDMLDefRPrIntoPPr(blockBody, defRPr)

	// Strip the hoisted items from each run's `<a:rPr>`.
	withRefinedRuns := refineDMLRuns(withPPr, commonAttrs, commonChildren)

	return withRefinedRuns
}

// dmlRunWithRPr captures one `<a:r>` element's `<a:rPr>` shape — the
// parsed attribute list and the raw child XML fragments needed to
// compute the intersection across runs. Offsets are not stored because
// refineDMLRuns re-walks the paragraph after the pPr insertion has
// shifted positions.
type dmlRunWithRPr struct {
	// Parsed attributes from the rPr start tag (name → value).
	attrs map[string]string
	// Stable ordering of attribute keys, matching the source order on
	// the start tag.
	attrOrder []string
	// Direct child elements of `<a:rPr>` as raw XML fragments (one
	// per child, in source order). For the intersection we compare
	// the canonical serialisation of each fragment.
	children []string
}

// findDMLRunsWithRPr returns one entry per `<a:r>` run in the
// paragraph body that has a direct `<a:rPr>` child. Runs without rPr
// abort the optimisation upstream (commonRunPropertiesOf
// StyleOptimisation.java:225-228 clears the common set when a run has
// no direct properties); when we encounter such a run we return an
// empty slice so callers bail.
func findDMLRunsWithRPr(blockBody string) []dmlRunWithRPr {
	var out []dmlRunWithRPr
	pos := 0
	for pos < len(blockBody) {
		// Locate next `<a:r>` (run) start. Use the precompiled regex
		// to discriminate `<a:r>` / `<a:r …>` from `<a:rPr…>` etc.
		loc := dmlRunOpenTagRE.FindStringIndex(blockBody[pos:])
		if loc == nil {
			return out
		}
		_ = pos + loc[0] // run-open start; unused but illustrative
		runOpenEnd := pos + loc[1]
		// Locate matching `</a:r>`. `<a:r>` elements do not nest in
		// the DrawingML content model (ECMA-376-1 §21.1.2.2.7
		// CT_RegularTextRun allows only `<a:rPr>` + `<a:t>` children),
		// so a plain index suffices.
		closeRel := strings.Index(blockBody[runOpenEnd:], dmlRunCloseTag)
		if closeRel < 0 {
			return nil // unbalanced run — bail out of the whole optimisation
		}
		runBodyEnd := runOpenEnd + closeRel
		runBody := blockBody[runOpenEnd:runBodyEnd]

		// Find direct `<a:rPr>` child. It is always the first element
		// in the run body when present (DrawingML §21.1.2.2.7 ordering
		// is `<a:rPr>?, <a:t>`).
		_, _, attrs, attrOrder, children, ok := parseDMLRPr(runBody)
		if !ok {
			// Run has no rPr — empty direct properties, optimisation
			// must abort per upstream commonRunPropertiesOf.
			return nil
		}
		out = append(out, dmlRunWithRPr{
			attrs:     attrs,
			attrOrder: attrOrder,
			children:  children,
		})
		pos = runBodyEnd + len(dmlRunCloseTag)
	}
	return out
}

// parseDMLRPr locates the leading `<a:rPr>` inside a run body and
// extracts its attribute pairs + raw child XML fragments. Returns
// offsets (relative to runBody) that delimit the rPr element, the
// parsed attrs (map + ordered keys), the list of child element XML
// fragments, and an ok flag. ok=false when runBody has no `<a:rPr>`.
func parseDMLRPr(runBody string) (rprStart, rprEnd int, attrs map[string]string, attrOrder []string, children []string, ok bool) {
	idx := strings.Index(runBody, "<a:rPr")
	if idx < 0 {
		return 0, 0, nil, nil, nil, false
	}
	// Tag end ('>' or '/>')
	tagEnd := strings.Index(runBody[idx:], ">")
	if tagEnd < 0 {
		return 0, 0, nil, nil, nil, false
	}
	tagEndAbs := idx + tagEnd + 1
	openTag := runBody[idx:tagEndAbs]

	// Parse attrs from openTag (excluding the leading `<a:rPr` and
	// trailing `>` or `/>`).
	attrs = make(map[string]string)
	for _, m := range dmlAttrRE.FindAllStringSubmatch(openTag, -1) {
		name, value := m[1], m[2]
		attrs[name] = value
		attrOrder = append(attrOrder, name)
	}

	// Self-closing rPr — no children.
	if strings.HasSuffix(openTag, "/>") {
		return idx, tagEndAbs, attrs, attrOrder, nil, true
	}

	// Find matching `</a:rPr>`. The rPr body holds only well-known
	// child elements (a:ln, a:noFill, a:solidFill, a:gradFill,
	// a:blipFill, a:pattFill, a:effectLst, a:effectDag, a:highlight,
	// a:uLnTx, a:uLn, a:uFillTx, a:uFill, a:latin, a:ea, a:cs, a:sym,
	// a:hlinkClick, a:hlinkMouseOver, a:rtl, a:extLst per ECMA-376-1
	// §21.1.2.3.9 CT_TextCharacterProperties). None of these contain a
	// nested `</a:rPr>`, so a plain index suffices.
	closeIdx := strings.Index(runBody[tagEndAbs:], "</a:rPr>")
	if closeIdx < 0 {
		return 0, 0, nil, nil, nil, false
	}
	bodyEnd := tagEndAbs + closeIdx
	rprBody := runBody[tagEndAbs:bodyEnd]

	// Split rprBody into top-level child element fragments. Each
	// fragment is a single XML element (self-closing or open+close).
	children = splitDMLChildren(rprBody)

	return idx, bodyEnd + len("</a:rPr>"), attrs, attrOrder, children, true
}

// splitDMLChildren splits an rPr body into the sequence of top-level
// child element fragments. Whitespace between fragments is discarded
// (DrawingML canon does not emit pretty-printed rPr bodies). Each
// returned string is the verbatim XML of one child element including
// its open and close tags (or self-closing form).
func splitDMLChildren(body string) []string {
	var out []string
	pos := 0
	for pos < len(body) {
		// Skip whitespace.
		for pos < len(body) && (body[pos] == ' ' || body[pos] == '\n' || body[pos] == '\r' || body[pos] == '\t') {
			pos++
		}
		if pos >= len(body) {
			break
		}
		if body[pos] != '<' {
			// Stray text — not a valid rPr child. Skip to next '<'.
			next := strings.Index(body[pos:], "<")
			if next < 0 {
				break
			}
			pos += next
			continue
		}
		// Read element name.
		nameStart := pos + 1
		nameEnd := nameStart
		for nameEnd < len(body) {
			c := body[nameEnd]
			if c == ' ' || c == '>' || c == '/' || c == '\t' || c == '\n' || c == '\r' {
				break
			}
			nameEnd++
		}
		name := body[nameStart:nameEnd]
		// Find tag end.
		tagEnd := strings.Index(body[pos:], ">")
		if tagEnd < 0 {
			break
		}
		tagEndAbs := pos + tagEnd + 1
		openTag := body[pos:tagEndAbs]
		if strings.HasSuffix(openTag, "/>") {
			out = append(out, openTag)
			pos = tagEndAbs
			continue
		}
		// Need matching `</name>`.
		closeTag := "</" + name + ">"
		closeIdx := findMatchingCloseTag(body[tagEndAbs:], name)
		if closeIdx < 0 {
			break
		}
		closeStart := tagEndAbs + closeIdx
		fragmentEnd := closeStart + len(closeTag)
		out = append(out, body[pos:fragmentEnd])
		pos = fragmentEnd
	}
	return out
}

// findMatchingCloseTag returns the offset within s of the closing tag
// `</name>` that balances the opening tag whose body s begins. Handles
// arbitrary nesting depth (Okapi-generated rPr children commonly
// contain `<a:solidFill><a:srgbClr/></a:solidFill>` nesting).
func findMatchingCloseTag(s, name string) int {
	openTag := "<a:" + strings.TrimPrefix(name, "a:")
	if !strings.HasPrefix(name, "a:") {
		openTag = "<" + name
	}
	closeTag := "</" + name + ">"
	depth := 1
	pos := 0
	for pos < len(s) {
		nextOpen := strings.Index(s[pos:], openTag)
		nextClose := strings.Index(s[pos:], closeTag)
		if nextClose < 0 {
			return -1
		}
		if nextOpen >= 0 && nextOpen < nextClose {
			// Could be a nested self-closing or open. Check the byte
			// right after the open tag — '>' means open (depth++),
			// '/' means self-closing (skip, no depth change). Use the
			// position after openTag to find that byte.
			after := pos + nextOpen + len(openTag)
			// Skip attributes.
			tagClose := strings.Index(s[after:], ">")
			if tagClose < 0 {
				return -1
			}
			tagEndAbs := after + tagClose + 1
			if tagEndAbs >= 2 && s[tagEndAbs-2] != '/' {
				depth++
			}
			pos = tagEndAbs
			continue
		}
		depth--
		if depth == 0 {
			return pos + nextClose
		}
		pos += nextClose + len(closeTag)
	}
	return -1
}

// intersectDMLRunProperties computes the intersection of run-property
// items across every run's rPr. Items are split into two buckets:
// attribute name=value pairs (on the rPr start tag) and child element
// XML fragments. Both buckets are intersected by exact string match.
//
// The intersection is non-empty when there is at least one item that
// appears identically across every run. Order is preserved from the
// first run (so canonical Okapi attribute order survives when present).
func intersectDMLRunProperties(runs []dmlRunWithRPr) (commonAttrs map[string]string, commonChildren []string) {
	if len(runs) == 0 {
		return nil, nil
	}
	first := runs[0]
	// Attribute intersection: start with first run's attrs, drop any
	// that another run lacks or mismatches.
	attrCommon := make(map[string]string, len(first.attrs))
	for k, v := range first.attrs {
		attrCommon[k] = v
	}
	for _, r := range runs[1:] {
		for k, v := range attrCommon {
			if rv, ok := r.attrs[k]; !ok || rv != v {
				delete(attrCommon, k)
			}
		}
	}

	// Child intersection: a child is in the intersection iff every
	// run carries the exact same fragment string. Preserve first
	// run's order. Use a set lookup for O(N) per run.
	childCommon := make([]string, 0, len(first.children))
	for _, c := range first.children {
		matchedAll := true
		for _, r := range runs[1:] {
			if !containsString(r.children, c) {
				matchedAll = false
				break
			}
		}
		if matchedAll {
			childCommon = append(childCommon, c)
		}
	}

	return attrCommon, childCommon
}

// containsString reports whether s appears in ss.
func containsString(ss []string, s string) bool {
	for _, v := range ss {
		if v == s {
			return true
		}
	}
	return false
}

// buildDMLDefRPr serialises the common attributes + children as an
// `<a:defRPr>` element. When no children are present we emit the
// self-closing form `<a:defRPr ... />`; otherwise we emit the open+
// close form.
//
// Attributes are sorted alphabetically by name to keep the output
// deterministic across runs whose rPr attribute order differs (Okapi
// emits attributes in the same source-order Java reads them; ours
// matches by canonical key sort to avoid order-dependent failures).
func buildDMLDefRPr(commonAttrs map[string]string, commonChildren []string) string {
	var sb strings.Builder
	sb.WriteString("<a:defRPr")
	keys := make([]string, 0, len(commonAttrs))
	for k := range commonAttrs {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		sb.WriteString(" ")
		sb.WriteString(k)
		sb.WriteString(`="`)
		sb.WriteString(commonAttrs[k])
		sb.WriteString(`"`)
	}
	if len(commonChildren) == 0 {
		sb.WriteString("/>")
		return sb.String()
	}
	sb.WriteString(">")
	for _, c := range commonChildren {
		sb.WriteString(c)
	}
	sb.WriteString("</a:defRPr>")
	return sb.String()
}

// insertDMLDefRPrIntoPPr inserts defRPr as the first child of the
// paragraph's `<a:pPr>`. When the paragraph has no `<a:pPr>` we
// synthesise one. The synthesised pPr is inserted as the first child
// of `<a:p>` per the DrawingML content model
// (ECMA-376-1 §21.1.2.2.6 CT_TextParagraph).
//
// The block body input is the slice BETWEEN `<a:p…>` and `</a:p>` —
// matching the slice operated on by stripDMLRunPropertyAttrs.
func insertDMLDefRPrIntoPPr(blockBody, defRPr string) string {
	pprOpenIdx := strings.Index(blockBody, "<a:pPr")
	if pprOpenIdx < 0 {
		// No pPr — synthesise one at the start of the paragraph body.
		return "<a:pPr>" + defRPr + "</a:pPr>" + blockBody
	}
	// Find the end of the pPr open tag.
	openTagEnd := strings.Index(blockBody[pprOpenIdx:], ">")
	if openTagEnd < 0 {
		return blockBody // malformed — pass through
	}
	openTagEndAbs := pprOpenIdx + openTagEnd + 1
	openTag := blockBody[pprOpenIdx:openTagEndAbs]

	if strings.HasSuffix(openTag, "/>") {
		// Self-closing `<a:pPr.../>`. Expand to `<a:pPr...>defRPr
		// </a:pPr>`.
		openAttrs := openTag[len("<a:pPr") : len(openTag)-2]
		expanded := "<a:pPr" + openAttrs + ">" + defRPr + "</a:pPr>"
		return blockBody[:pprOpenIdx] + expanded + blockBody[openTagEndAbs:]
	}
	// Open form — insert defRPr right after the start tag (as first
	// child).
	return blockBody[:openTagEndAbs] + defRPr + blockBody[openTagEndAbs:]
}

// refineDMLRuns walks every `<a:r>` run in the paragraph body and
// removes the hoisted attributes + children from each run's `<a:rPr>`.
// When a run's `<a:rPr>` becomes structurally empty (no attrs and no
// children remain) the entire `<a:rPr>` element is dropped, matching
// upstream Okapi's BlockProperties.getEvents which omits empty
// envelopes (BlockProperties.java:169-172).
//
// We re-walk the paragraph after the pPr insertion above so the offsets
// captured by findDMLRunsWithRPr (taken from the un-mutated body)
// don't have to be tracked through the insertion.
func refineDMLRuns(blockBody string, commonAttrs map[string]string, commonChildren []string) string {
	if len(commonAttrs) == 0 && len(commonChildren) == 0 {
		return blockBody
	}
	var out strings.Builder
	out.Grow(len(blockBody))
	pos := 0
	for pos < len(blockBody) {
		loc := dmlRunOpenTagRE.FindStringIndex(blockBody[pos:])
		if loc == nil {
			out.WriteString(blockBody[pos:])
			return out.String()
		}
		runOpenEnd := pos + loc[1]
		out.WriteString(blockBody[pos:runOpenEnd])
		closeRel := strings.Index(blockBody[runOpenEnd:], dmlRunCloseTag)
		if closeRel < 0 {
			out.WriteString(blockBody[runOpenEnd:])
			return out.String()
		}
		runBodyEnd := runOpenEnd + closeRel
		runBody := blockBody[runOpenEnd:runBodyEnd]
		refinedRunBody := refineDMLRunBody(runBody, commonAttrs, commonChildren)
		out.WriteString(refinedRunBody)
		out.WriteString(dmlRunCloseTag)
		pos = runBodyEnd + len(dmlRunCloseTag)
	}
	return out.String()
}

// refineDMLRunBody removes the hoisted properties from the run body's
// `<a:rPr>`. When the rPr becomes empty, it is dropped entirely.
func refineDMLRunBody(runBody string, commonAttrs map[string]string, commonChildren []string) string {
	rprStart, rprEnd, attrs, attrOrder, children, ok := parseDMLRPr(runBody)
	if !ok {
		return runBody
	}
	// Filter attrs: drop any whose value matches the hoisted value.
	remainingAttrs := make([]string, 0, len(attrOrder))
	for _, k := range attrOrder {
		if cv, ok := commonAttrs[k]; ok && cv == attrs[k] {
			continue
		}
		remainingAttrs = append(remainingAttrs, k)
	}
	// Filter children: keep only those not in commonChildren.
	commonSet := make(map[string]bool, len(commonChildren))
	for _, c := range commonChildren {
		commonSet[c] = true
	}
	remainingChildren := make([]string, 0, len(children))
	for _, c := range children {
		if commonSet[c] {
			continue
		}
		remainingChildren = append(remainingChildren, c)
	}

	// If everything is hoisted, drop the entire `<a:rPr>` element.
	if len(remainingAttrs) == 0 && len(remainingChildren) == 0 {
		return runBody[:rprStart] + runBody[rprEnd:]
	}

	// Otherwise rebuild rPr with the remaining attrs and children.
	var sb strings.Builder
	sb.WriteString("<a:rPr")
	for _, k := range remainingAttrs {
		sb.WriteString(" ")
		sb.WriteString(k)
		sb.WriteString(`="`)
		sb.WriteString(attrs[k])
		sb.WriteString(`"`)
	}
	if len(remainingChildren) == 0 {
		sb.WriteString("/>")
	} else {
		sb.WriteString(">")
		for _, c := range remainingChildren {
			sb.WriteString(c)
		}
		sb.WriteString("</a:rPr>")
	}
	return runBody[:rprStart] + sb.String() + runBody[rprEnd:]
}
