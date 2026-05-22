//go:build parity

package roundtrip

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"errors"
	"io"
	"sort"
	"strings"
)

// wmlNS is the WordprocessingML Transitional namespace URI (ECMA-376-1
// §17, the "2006" namespace most .docx files use). encoding/xml decodes
// the source's `w:`-prefixed names into Name.Space, so we match by
// namespace URI + local name and never by the source's declared prefix.
const wmlNS = "http://schemas.openxmlformats.org/wordprocessingml/2006/main"

// wmlNSStrict is the WordprocessingML Strict (ISO/IEC 29500 "purl.oclc.org")
// namespace URI. Okapi sometimes re-serialises a document into the Strict
// namespace while native preserves the source's Transitional namespace.
// Both are the same WordprocessingML vocabulary (ISO/IEC 29500-1 Annex A);
// the resolver must treat elements in either namespace identically so the
// two sides' run properties compare equal.
const wmlNSStrict = "http://purl.oclc.org/ooxml/wordprocessingml/main"

// isWMLNamespace reports whether a namespace URI is one of the
// WordprocessingML namespaces (Transitional or Strict), or empty (which
// the decoder produces for elements in scope of an unprefixed default
// declaration matching the WML default).
func isWMLNamespace(space string) bool {
	return space == wmlNS || space == wmlNSStrict || space == ""
}

// OpenXMLEffectiveRPr is the effective-rPr canonical normalizer — the
// keystone of the openxml faithful-writer parity story.
//
// Native ships faithful output (OptimiseWordStyles=false): each run keeps
// its source run properties inline. Okapi ships a compact form: it lifts a
// paragraph's common run properties into a synthesised paragraph style
// (`<w:pStyle w:val="NF…-Normal">` + a `<w:style>` def in word/styles.xml)
// and drops those properties from the runs. ECMA-376-1 §17.7 (style
// resolution) makes the two forms equivalent: a conforming consumer
// resolves docDefaults → style chain (basedOn) → paragraph pStyle → run
// rStyle → direct rPr to the same EFFECTIVE per-run formatting, then
// renders. Inline (native) and style-based (Okapi) representations are
// equally valid producer choices that resolve identically.
//
// This normalizer applies the §17.7 resolution SYMMETRICALLY to both
// sides so the inline form and the synth-style form canonicalise to the
// same thing:
//
//  1. Parse word/styles.xml → docDefaults rPr + each style's own rPr,
//     basedOn parent, style type, and the default style per type.
//  2. For every word/*.xml part, walk each paragraph: resolve every rPr
//     (run rPr and the paragraph-mark rPr in pPr) to its effective set by
//     layering, in order, docDefaults → paragraph-pStyle chain → run-rStyle
//     chain → the rPr's own direct children. Layering uses Okapi's
//     `combineDistinct` rule (RunProperties.java:620 — replace any earlier
//     property whose element name matches, append the rest), which is the
//     §17.7 cascade for non-toggle properties and direct overrides.
//     Inline the resolved children onto the rPr and drop the now-redundant
//     `<w:pStyle>`/`<w:rStyle>` references.
//  3. For word/styles.xml, drop every `<w:style>` definition and the
//     docDefaults rPr — once the formatting is inlined on the runs the
//     style defs carry nothing the comparison needs, and dropping them on
//     BOTH sides erases the asymmetry of Okapi's added synth styles.
//
// After this pass the existing XMLCanonical chain (attr-sort, child-sort,
// revision-id strip, …) compares the resolved trees. Native's inline rPr
// and Okapi's pStyle-resolved rPr then match.
//
// Grounded in: ECMA-376-1 / ISO/IEC 29500-1 §17.7 (style hierarchy),
// §17.3.2.1 (CT_R), §17.7.5.5 (docDefaults); and upstream Okapi
// WordStyleDefinitions.combinedRunProperties (WordStyleDefinitions.java
// :302-314) + RunProperties.combineDistinct (RunProperties.java:620-650).
//
// Parity-only — not shipped in the product writer. Symmetric, principled,
// reusable: it is the spec's own equivalence relation, not Okapi-mimicry.
type OpenXMLEffectiveRPr struct{}

// Name implements Normalizer.
func (OpenXMLEffectiveRPr) Name() string { return "openxml-effective-rpr" }

// Normalize implements Normalizer. It expects a zip (.docx) archive and
// rewrites the word/*.xml parts in place. Non-zip or unparsable input is
// returned unchanged so the normalizer chains safely.
func (n OpenXMLEffectiveRPr) Normalize(in []byte) ([]byte, error) {
	r, err := zip.NewReader(bytes.NewReader(in), int64(len(in)))
	if err != nil {
		// Not a zip — leave it to the caller / next chain step.
		return in, nil
	}

	// Pass 1: read every entry, locate + parse word/styles.xml.
	type entry struct {
		name string
		body []byte
	}
	var entries []entry
	var styleTable *wmlStyleTable
	for _, f := range r.File {
		if f.FileInfo().IsDir() {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return in, nil
		}
		raw, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			return in, nil
		}
		entries = append(entries, entry{name: f.Name, body: raw})
		if f.Name == "word/styles.xml" {
			styleTable = parseWMLStyleTable(raw)
		}
	}
	if styleTable == nil {
		styleTable = &wmlStyleTable{}
	}

	// Pass 2: rewrite the word/*.xml parts (and styles.xml itself).
	var out bytes.Buffer
	w := zip.NewWriter(&out)
	for _, e := range entries {
		body := e.body
		switch {
		case e.name == "word/styles.xml":
			if rewritten, ok := stripWMLStyleDefs(e.body); ok {
				body = rewritten
			}
		case isWMLContentPart(e.name):
			if rewritten, ok := resolveWMLEffectiveRPr(e.body, styleTable); ok {
				body = rewritten
			}
		}
		hdr := &zip.FileHeader{Name: e.name, Method: zip.Store}
		zw, err := w.CreateHeader(hdr)
		if err != nil {
			return in, nil
		}
		if _, err := zw.Write(body); err != nil {
			return in, nil
		}
	}
	if err := w.Close(); err != nil {
		return in, nil
	}
	return out.Bytes(), nil
}

// isWMLContentPart reports whether the zip entry is a WordprocessingML
// content part whose runs carry resolvable formatting: the main document,
// headers/footers, foot/endnotes, and comments. These are exactly the
// parts whose paragraphs reference styles.xml for pStyle/rStyle. The
// styles.xml part itself is handled separately.
func isWMLContentPart(name string) bool {
	if !strings.HasPrefix(name, "word/") || !strings.HasSuffix(name, ".xml") {
		return false
	}
	base := strings.TrimPrefix(name, "word/")
	switch {
	case base == "document.xml",
		base == "footnotes.xml",
		base == "endnotes.xml",
		base == "comments.xml":
		return true
	case strings.HasPrefix(base, "header") && strings.HasSuffix(base, ".xml"):
		return true
	case strings.HasPrefix(base, "footer") && strings.HasSuffix(base, ".xml"):
		return true
	}
	return false
}

// ── Style table (ECMA-376-1 §17.7) ────────────────────────────────────

// wmlStyleEntry is one <w:style> definition's resolution-relevant state:
// its own rPr children (canonicalised), the basedOn parent id, and the
// style type ("paragraph"/"character"/…).
type wmlStyleEntry struct {
	id        string
	basedOn   string
	styleType string
	// rPrChildren are the rPr child elements authored directly on this
	// style, captured as canonical nodes (one per element). Order is
	// source order; equality/replacement is by element name.
	rPrChildren []rPrNode
}

// wmlStyleTable holds the resolved style universe for one document.
type wmlStyleTable struct {
	styles map[string]*wmlStyleEntry
	// docDefaults are the <w:docDefaults><w:rPrDefault><w:rPr> children —
	// the implicit base of every run's resolution chain (§17.7.5.5).
	docDefaults []rPrNode
	// defaultParaStyle / defaultCharStyle are the styleIds of the
	// <w:style w:default="1"> entries per type. A paragraph with no
	// pStyle inherits from the default paragraph style (§17.3.1.10).
	defaultParaStyle string
	defaultCharStyle string
}

// rPrNode is a single rPr child element (e.g. <w:rFonts>, <w:sz>, <w:b>)
// kept as its decoded start element + descendants. rPr children have no
// element content beyond optional nested elements (none of the props we
// resolve nest), so we keep the start element (carrying the attributes,
// which ARE the value per §17.3.2) plus any raw children verbatim.
type rPrNode struct {
	start    xml.StartElement
	children []xmlChild
	end      xml.EndElement
}

// localName returns the rPr child's WML local name (e.g. "rFonts", "sz").
func (p rPrNode) localName() string { return p.start.Name.Local }

// parseWMLStyleTable parses word/styles.xml into a wmlStyleTable. Returns
// a non-nil (possibly empty) table even on parse error so callers always
// have a usable resolver.
func parseWMLStyleTable(data []byte) *wmlStyleTable {
	st := &wmlStyleTable{styles: map[string]*wmlStyleEntry{}}
	tree, ok := decodeXMLTree(data)
	if !ok {
		return st
	}
	// The synthetic root holds the XML declaration ProcInst and the
	// <w:styles> document element; styles + docDefaults are children of
	// <w:styles>, one level below the synthetic root.
	stylesRoot := findChildElem(tree, "styles")
	if stylesRoot == nil {
		return st
	}
	for _, c := range stylesRoot.children {
		if c.sub == nil {
			continue
		}
		switch c.sub.start.Name.Local {
		case "docDefaults":
			st.docDefaults = extractDocDefaultsRPr(c.sub)
		case "style":
			e := parseWMLStyleEntry(c.sub)
			if e == nil || e.id == "" {
				continue
			}
			st.styles[e.id] = e
			if attrLocal(c.sub.start, "default") == "1" {
				switch e.styleType {
				case "paragraph":
					st.defaultParaStyle = e.id
				case "character":
					st.defaultCharStyle = e.id
				}
			}
		}
	}
	return st
}

// extractDocDefaultsRPr returns the rPr children of
// <w:docDefaults><w:rPrDefault><w:rPr>…</w:rPr></w:rPrDefault>.
func extractDocDefaultsRPr(docDefaults *xmlNode) []rPrNode {
	rPrDefault := findChildElem(docDefaults, "rPrDefault")
	if rPrDefault == nil {
		return nil
	}
	rPr := findChildElem(rPrDefault, "rPr")
	if rPr == nil {
		return nil
	}
	return collectRPrChildren(rPr)
}

// parseWMLStyleEntry parses one <w:style> element.
func parseWMLStyleEntry(style *xmlNode) *wmlStyleEntry {
	e := &wmlStyleEntry{
		id:        attrLocal(style.start, "styleId"),
		styleType: attrLocal(style.start, "type"),
	}
	if b := findChildElem(style, "basedOn"); b != nil {
		e.basedOn = attrLocal(b.start, "val")
	}
	if rPr := findChildElem(style, "rPr"); rPr != nil {
		e.rPrChildren = collectRPrChildren(rPr)
	}
	return e
}

// collectRPrChildren returns each element child of an rPr node as an
// rPrNode (non-element children — whitespace CharData — are dropped; they
// carry no formatting). Style-reference children (pStyle/rStyle) are
// retained here because the resolver decides per-context what to do with
// them (a run's rStyle points into the chain; it is not a formatting prop).
func collectRPrChildren(rPr *xmlNode) []rPrNode {
	var out []rPrNode
	for _, c := range rPr.children {
		if c.sub == nil {
			continue
		}
		out = append(out, rPrNode{
			start:    c.sub.start,
			children: c.sub.children,
			end:      c.sub.end,
		})
	}
	return out
}

// resolveStyleChain returns the combined rPr children contributed by the
// basedOn chain of styleID (root-most first, youngest last), so that
// combineDistinct layering yields youngest-wins semantics. docDefaults are
// NOT included (the caller layers them first).
func (st *wmlStyleTable) resolveStyleChain(styleID string) []rPrNode {
	if st == nil || styleID == "" {
		return nil
	}
	// Walk to the root collecting ids, guarding cycles.
	var chain []string
	visited := map[string]bool{}
	cur := styleID
	for cur != "" && !visited[cur] {
		visited[cur] = true
		e, ok := st.styles[cur]
		if !ok {
			break
		}
		chain = append(chain, cur)
		cur = e.basedOn
	}
	// Apply root-most first so the youngest style overrides. combineDistinct
	// replaces by element name, so iterating from the root down and combining
	// each style's own rPr produces the §17.7 "closest wins" result.
	var combined []rPrNode
	for i := len(chain) - 1; i >= 0; i-- {
		combined = combineDistinctRPr(combined, st.styles[chain[i]].rPrChildren)
	}
	return combined
}

// ── Effective-rPr resolution (the §17.7 cascade) ──────────────────────

// resolveWMLEffectiveRPr rewrites a WML content part so every rPr carries
// its fully-resolved effective formatting inline and no pStyle/rStyle
// references remain. Returns (rewritten, true) on success, (nil, false)
// when the part can't be parsed (caller keeps the original bytes).
func resolveWMLEffectiveRPr(data []byte, st *wmlStyleTable) ([]byte, bool) {
	tree, ok := decodeXMLTree(data)
	if !ok {
		return nil, false
	}
	resolveParagraphsInNode(tree, st, "")
	encoded, ok := encodeXMLTree(tree)
	if !ok {
		return nil, false
	}
	return encoded, true
}

// resolveParagraphsInNode walks node depth-first. When it meets a <w:p> it
// resolves that paragraph (using its own pStyle, falling back to the
// inherited default paragraph style), then keeps descending so paragraphs
// nested inside the paragraph's runs (textbox content under
// <w:drawing>/<w:txbxContent>/<mc:AlternateContent>) are also resolved.
// Tables, sdt wrappers, and other containers recurse the same way.
func resolveParagraphsInNode(node *xmlNode, st *wmlStyleTable, inheritedParaStyle string) {
	for _, c := range node.children {
		if c.sub == nil {
			continue
		}
		if isWMLElem(c.sub.start, "p") {
			resolveParagraph(c.sub, st)
			// Keep descending: a paragraph's runs may embed textbox
			// content with its own nested paragraphs.
			resolveParagraphsInNode(c.sub, st, inheritedParaStyle)
			continue
		}
		resolveParagraphsInNode(c.sub, st, inheritedParaStyle)
	}
}

// resolveParagraph resolves every rPr inside one <w:p>: the paragraph-mark
// rPr in <w:pPr><w:rPr> and each run's <w:rPr>. The paragraph's pStyle
// (explicit, or the document default paragraph style) seeds the chain.
func resolveParagraph(p *xmlNode, st *wmlStyleTable) {
	paraStyle := paragraphStyleID(p)
	if paraStyle == "" {
		paraStyle = st.defaultParaStyle
	}

	// Effective formatting contributed by docDefaults + the paragraph
	// style chain — the shared base for both the paragraph-mark rPr and
	// every run that does not add its own rStyle.
	base := combineDistinctRPr(cloneRPrNodes(st.docDefaults), st.resolveStyleChain(paraStyle))

	pPr := findChildElem(p, "pPr")
	if pPr != nil {
		// The paragraph-mark rPr (<w:pPr><w:rPr>) carries paragraph-mark
		// formatting; resolve it against base then drop pStyle. We DO NOT
		// strip pStyle from pPr's element list here — pStyle is a pPr
		// child (sibling of rPr), removed below.
		if rPr := findChildElem(pPr, "rPr"); rPr != nil {
			inlineResolvedRPr(rPr, base, nil)
			dropEmptyRPr(pPr)
		}
		// Drop the <w:pStyle> reference from pPr (its formatting is now
		// inlined wherever it mattered, and dropping it symmetrically on
		// both sides erases the synth-pStyle-vs-inline asymmetry).
		removeChildElems(pPr, "pStyle")
	}

	// Resolve every run in the paragraph's content. Runs are not always
	// direct children of <w:p>: ECMA-376-1 §17.3.2.1 (CT_R) runs may be
	// wrapped by inline-level containers (hyperlink, smartTag, ins, del,
	// bdo, dir, …). Walk the content recursively, descending through such
	// wrappers but NOT into a nested <w:p> (textbox paragraphs carry their
	// own pStyle context and are handled by resolveParagraphsInNode).
	resolveRunsInContent(p, st, base)

	// Drop an empty <w:pPr> (no element children left after pStyle
	// removal). One side may carry a bare <w:pPr/> wrapper around just a
	// pStyle while the other has no pPr at all; an element-empty pPr is
	// inert (ECMA-376-1 §17.3.1.10), so dropping it symmetrically erases
	// that asymmetry.
	if pPr != nil && !hasElementChild(pPr) {
		removeChildNode(p, pPr)
	}
}

// resolveRunsInContent resolves the rPr of every <w:r> reachable from node
// without crossing a nested <w:p> boundary. base is the paragraph's
// docDefaults+pStyle effective formatting. Inline-level wrappers
// (hyperlink, smartTag, ins, del, …) are descended; <w:pPr> is skipped
// (already resolved by the caller); a nested <w:p> stops the walk.
func resolveRunsInContent(node *xmlNode, st *wmlStyleTable, base []rPrNode) {
	for _, c := range node.children {
		if c.sub == nil {
			continue
		}
		switch {
		case isWMLElem(c.sub.start, "r"):
			resolveRun(c.sub, st, base)
		case isWMLElem(c.sub.start, "p"):
			// Nested paragraph (textbox) — its own context; do not descend.
			continue
		case isWMLElem(c.sub.start, "pPr"):
			// Already resolved by resolveParagraph; skip.
			continue
		default:
			resolveRunsInContent(c.sub, st, base)
		}
	}
}

// resolveRun resolves one <w:r>'s effective rPr against base and the run's
// own rStyle chain, then drops the rPr if it resolves to empty.
func resolveRun(run *xmlNode, st *wmlStyleTable, base []rPrNode) {
	rPr := findChildElem(run, "rPr")
	if rPr == nil {
		// A run with no rPr inherits purely from base; synthesise an rPr
		// only if base is non-empty so it compares equal to a side that
		// carries the same props inline.
		if len(base) == 0 {
			return
		}
		injectResolvedRPr(run, cloneRPrNodes(base))
		return
	}
	runStyle := rStyleID(rPr)
	var runChain []rPrNode
	if runStyle != "" {
		runChain = st.resolveStyleChain(runStyle)
	}
	inlineResolvedRPr(rPr, base, runChain)
	dropEmptyRPr(run)
}

// dropEmptyRPr removes the <w:rPr> child of parent when it has no element
// children. An element-empty rPr applies no formatting (§17.3.2.1); one
// side may emit a bare <w:rPr/> while the other omits it entirely.
func dropEmptyRPr(parent *xmlNode) {
	rPr := findChildElem(parent, "rPr")
	if rPr != nil && !hasElementChild(rPr) {
		removeChildNode(parent, rPr)
	}
}

// hasElementChild reports whether node has at least one element child.
func hasElementChild(node *xmlNode) bool {
	for _, c := range node.children {
		if c.sub != nil {
			return true
		}
	}
	return false
}

// removeChildNode drops the first child whose sub pointer == target.
func removeChildNode(parent, target *xmlNode) {
	kept := parent.children[:0]
	for _, c := range parent.children {
		if c.sub == target {
			continue
		}
		kept = append(kept, c)
	}
	parent.children = kept
}

// inlineResolvedRPr replaces rPr's children with the resolved effective
// formatting: base (docDefaults + paragraph-style chain) → run-style chain
// → the rPr's own direct formatting children (excluding the style-ref
// children pStyle/rStyle, which are pointers, not formatting). pStyle and
// rStyle are dropped from the output. Result children are deterministically
// ordered by element name so the later XMLCanonical pass compares cleanly.
//
// The run's text content is intentionally NOT consulted here. Okapi's
// content-category clarification (§17.3.2.26: dropping complex-script
// bCs/iCs/szCs/rFonts-cs on runs with no complex-script text, and the
// non-complex mirror) is left as documented cosmetic divergence
// (851/multiple_tabs/br2/…): it is the spec's content-category
// equivalence, NOT style indirection, and faithfully replicating Okapi's
// RunParser.canBeSkipped decision (which gates on the per-run pre-combined
// view, applied BEFORE the synth-style lift) cannot be approximated
// symmetrically in the comparator without dropping properties Okapi keeps
// as genuine overrides — every approximation tried proved net-regressive
// against the WSO-off baseline.
func inlineResolvedRPr(rPr *xmlNode, base, runChain []rPrNode) {
	direct := directFormattingChildren(rPr)
	eff := combineDistinctRPr(cloneRPrNodes(base), runChain)
	eff = combineDistinctRPr(eff, direct)
	setRPrChildren(rPr, eff)
}

// injectResolvedRPr inserts a fresh <w:rPr> with the given children as the
// first child of run, mirroring ECMA-376-1 §17.3.2.1 (rPr MUST be the
// first child of <w:r> when present). children are ordered by element name.
func injectResolvedRPr(run *xmlNode, children []rPrNode) {
	rPr := &xmlNode{
		start: xml.StartElement{Name: xml.Name{Space: wmlNS, Local: "rPr"}},
		end:   xml.EndElement{Name: xml.Name{Space: wmlNS, Local: "rPr"}},
	}
	setRPrChildren(rPr, children)
	run.children = append([]xmlChild{{sub: rPr}}, run.children...)
}

// directFormattingChildren returns rPr's own formatting children, EXCLUDING
// the style-reference children (rStyle/pStyle), which are resolution
// pointers rather than formatting (Okapi's combinedRunProperties filters
// out StyleRunProperty before combining — WordStyleDefinitions.java:309).
func directFormattingChildren(rPr *xmlNode) []rPrNode {
	all := collectRPrChildren(rPr)
	out := all[:0:0]
	for _, p := range all {
		switch p.localName() {
		case "rStyle", "pStyle":
			continue
		}
		out = append(out, p)
	}
	return out
}

// setRPrChildren replaces rPr's element children with the given resolved
// nodes, after applying spec-equivalent value clarifications, sorted by
// element name (stable). Preserving any non-element children at the front
// is unnecessary because rPr has none of interest.
func setRPrChildren(rPr *xmlNode, children []rPrNode) {
	sorted := make([]rPrNode, len(children))
	copy(sorted, children)
	sorted = clarifyRPrChildren(sorted)
	sort.SliceStable(sorted, func(i, j int) bool {
		return sorted[i].localName() < sorted[j].localName()
	})
	rPr.children = rPr.children[:0]
	for i := range sorted {
		rPr.children = append(rPr.children, xmlChild{sub: &xmlNode{
			start:    sorted[i].start,
			children: sorted[i].children,
			end:      sorted[i].end,
		}})
	}
}

// wmlToggleProps is the set of WordprocessingML run-property toggle
// element local names (ECMA-376-1 §17.3.2, CT_OnOff). Each is a boolean
// formatting flag whose ABSENCE means "off" and whose bare element form
// (or w:val="1"/"true"/"on") means "on". Mirrors upstream Okapi's
// RunPropertyFactory.WpmlTogglePropertyName enum
// (RunPropertyFactory.java:201-222) plus the core b/i/strike toggles.
var wmlToggleProps = map[string]bool{
	"b": true, "bCs": true, "i": true, "iCs": true,
	"caps": true, "smallCaps": true, "strike": true, "dstrike": true,
	"outline": true, "shadow": true, "emboss": true, "imprint": true,
	"vanish": true, "webHidden": true, "specVanish": true,
	"noProof": true, "snapToGrid": true, "oMath": true,
	"cs": true, "rtl": true,
}

// isToggleOff reports whether a toggle element is in its explicit-OFF form
// (w:val="0"/"false"/"off"). A bare element or w:val="1"/"true"/"on" is ON.
func isToggleOff(s xml.StartElement) bool {
	v := attrLocal(s, "val")
	switch v {
	case "0", "false", "off":
		return true
	}
	return false
}

// clarifyRPrChildren applies spec-equivalent run-property value
// clarifications in place and returns the filtered slice (some entries may
// be dropped). These are unambiguous ECMA-376 equivalences, not
// Okapi-specific heuristics:
//
//   - <w:color w:val="auto"> ≡ <w:color w:val="000000"> for the foreground
//     run property. ECMA-376-1 §17.3.2.6 (CT_Color): `auto` means "the
//     consumer determines the colour automatically"; for body text the
//     automatic foreground colour is black (000000). Okapi resolves the
//     automatic value to its concrete RGB on round-trip while native
//     preserves the source token; both denote the same rendered colour.
//
//   - Toggle properties (§17.3.2, CT_OnOff: b, i, caps, rtl, …) whose
//     EFFECTIVE value is OFF are dropped, because in WML an absent toggle
//     and an explicit `w:val="0"/"false"/"off"` toggle are equivalent
//     ("off" is the property's default). Since the resolver has already
//     layered the full style chain into the effective set, a toggle still
//     present with an off value carries no formatting and is equivalent to
//     omitting it — exactly what Okapi's RunProperties.minified() does
//     (RunProperties.java:497-540). Toggles whose effective value is ON are
//     normalised to the bare element form (<w:b/>) so the w:val="1"/"true"
//     spelling variants compare equal.
func clarifyRPrChildren(children []rPrNode) []rPrNode {
	out := children[:0]
	for _, c := range children {
		name := c.localName()
		if name == "color" {
			c.start = setAttrLocal(c.start, "val", func(v string) (string, bool) {
				if v == "auto" {
					return "000000", true
				}
				return v, false
			})
		}
		if wmlToggleProps[name] {
			if isToggleOff(c.start) {
				// Effective-off toggle ≡ absent; drop it.
				continue
			}
			// Effective-on toggle: normalise to the bare element form so
			// <w:b/>, <w:b w:val="1"/>, and <w:b w:val="true"/> compare equal.
			c.start.Attr = stripAttrLocal(c.start.Attr, "val")
		}
		out = append(out, c)
	}
	return out
}

// stripAttrLocal returns the attribute slice with the named attribute
// removed (matched by local name regardless of namespace prefix). Returns
// a fresh slice when a removal happens so shared storage is not mutated.
func stripAttrLocal(attrs []xml.Attr, local string) []xml.Attr {
	for i, a := range attrs {
		if a.Name.Local == local {
			out := make([]xml.Attr, 0, len(attrs)-1)
			out = append(out, attrs[:i]...)
			out = append(out, attrs[i+1:]...)
			return out
		}
	}
	return attrs
}

// setAttrLocal returns a copy of s with the named attribute's value
// transformed by fn. fn returns (newValue, changed); when changed is
// false the start element is returned unmodified. The attribute is matched
// by local name regardless of namespace prefix.
func setAttrLocal(s xml.StartElement, local string, fn func(string) (string, bool)) xml.StartElement {
	for i, a := range s.Attr {
		if a.Name.Local != local {
			continue
		}
		nv, changed := fn(a.Value)
		if !changed {
			return s
		}
		// Copy the attribute slice so we don't mutate shared storage.
		attrs := make([]xml.Attr, len(s.Attr))
		copy(attrs, s.Attr)
		attrs[i].Value = nv
		s.Attr = attrs
		return s
	}
	return s
}

// combineDistinctRPr layers add onto base using Okapi's combineDistinct
// rule (RunProperties.java:620-650): for each property already in base,
// replace it with the same-named property from add (if present); append
// any add property whose name is not already in base. The result reflects
// "later layer wins by element name", which is the §17.7 cascade for
// non-toggle properties and for direct overrides of inherited values.
//
// base is mutated and returned; callers that need to preserve base pass a
// clone (cloneRPrNodes).
func combineDistinctRPr(base, add []rPrNode) []rPrNode {
	if len(add) == 0 {
		return base
	}
	used := make([]bool, len(add))
	for i := range base {
		name := base[i].localName()
		for j := range add {
			if used[j] {
				continue
			}
			if add[j].localName() == name {
				base[i] = add[j]
				used[j] = true
				break
			}
		}
	}
	for j := range add {
		if !used[j] {
			base = append(base, add[j])
		}
	}
	return base
}

// cloneRPrNodes returns a shallow copy of the slice (the rPrNode values
// share the underlying start/children, which are treated as immutable by
// the resolver — combineDistinct only re-slices and replaces entries).
func cloneRPrNodes(in []rPrNode) []rPrNode {
	if len(in) == 0 {
		return nil
	}
	out := make([]rPrNode, len(in))
	copy(out, in)
	return out
}

// paragraphStyleID returns the <w:pStyle w:val="…"> of a paragraph, or "".
func paragraphStyleID(p *xmlNode) string {
	pPr := findChildElem(p, "pPr")
	if pPr == nil {
		return ""
	}
	if ps := findChildElem(pPr, "pStyle"); ps != nil {
		return attrLocal(ps.start, "val")
	}
	return ""
}

// rStyleID returns the <w:rStyle w:val="…"> of an rPr, or "".
func rStyleID(rPr *xmlNode) string {
	if rs := findChildElem(rPr, "rStyle"); rs != nil {
		return attrLocal(rs.start, "val")
	}
	return ""
}

// ── styles.xml symmetric strip ─────────────────────────────────────────

// stripWMLStyleDefs removes every <w:style> definition and the
// <w:docDefaults> rPr from word/styles.xml. Once the effective formatting
// is inlined onto runs (resolveWMLEffectiveRPr), the style defs carry no
// formatting the comparison needs; dropping them on BOTH sides erases the
// asymmetry between Okapi (which adds synthesised NF… styles) and native
// (which keeps only source styles). Non-style children (latentStyles, …)
// are left untouched. Returns (rewritten, true) on success.
func stripWMLStyleDefs(data []byte) ([]byte, bool) {
	tree, ok := decodeXMLTree(data)
	if !ok {
		return nil, false
	}
	// Find the <w:styles> root element among the tree's children.
	for _, c := range tree.children {
		if c.sub == nil || !isWMLElem(c.sub.start, "styles") {
			continue
		}
		styles := c.sub
		kept := styles.children[:0]
		for _, sc := range styles.children {
			if sc.sub != nil {
				switch sc.sub.start.Name.Local {
				case "style", "docDefaults":
					continue
				}
			}
			kept = append(kept, sc)
		}
		styles.children = kept
	}
	encoded, ok := encodeXMLTree(tree)
	if !ok {
		return nil, false
	}
	return encoded, true
}

// ── XML tree helpers (shared with XMLCanonical's xmlNode model) ────────

// decodeXMLTree decodes XML into the synthetic-root xmlNode tree used by
// the canonical transforms. Returns (tree, true) on success.
func decodeXMLTree(data []byte) (*xmlNode, bool) {
	dec := xml.NewDecoder(bytes.NewReader(data))
	var toks []xml.Token
	for {
		t, err := dec.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, false
		}
		toks = append(toks, xml.CopyToken(t))
	}
	idx := 0
	tree := buildXMLNode(toks, &idx, true)
	return tree, true
}

// encodeXMLTree serialises the synthetic-root xmlNode tree back to bytes.
func encodeXMLTree(tree *xmlNode) ([]byte, bool) {
	var toks []xml.Token
	emitXMLNode(tree, &toks, true, false)
	var buf bytes.Buffer
	enc := xml.NewEncoder(&buf)
	for _, t := range toks {
		if err := enc.EncodeToken(t); err != nil {
			return nil, false
		}
	}
	if err := enc.Flush(); err != nil {
		return nil, false
	}
	return buf.Bytes(), true
}

// isWMLElem reports whether a start element is the named WML element in
// either the Transitional or Strict WordprocessingML namespace.
func isWMLElem(s xml.StartElement, local string) bool {
	return s.Name.Local == local && isWMLNamespace(s.Name.Space)
}

// findChildElem returns the first direct child element with the given WML
// local name, or nil.
func findChildElem(node *xmlNode, local string) *xmlNode {
	if node == nil {
		return nil
	}
	for _, c := range node.children {
		if c.sub != nil && isWMLElem(c.sub.start, local) {
			return c.sub
		}
	}
	return nil
}

// removeChildElems drops every direct child element with the given WML
// local name from node.
func removeChildElems(node *xmlNode, local string) {
	kept := node.children[:0]
	for _, c := range node.children {
		if c.sub != nil && isWMLElem(c.sub.start, local) {
			continue
		}
		kept = append(kept, c)
	}
	node.children = kept
}

// attrLocal returns the value of the first attribute whose local name
// matches (regardless of namespace prefix), or "".
func attrLocal(s xml.StartElement, local string) string {
	for _, a := range s.Attr {
		if a.Name.Local == local {
			return a.Value
		}
	}
	return ""
}
