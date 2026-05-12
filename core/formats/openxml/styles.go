package openxml

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"errors"
	"io"
	"strings"
)

// styleMap holds resolved style definitions from styles.xml.
// It maps style IDs to their effective run properties after
// resolving the basedOn inheritance chain. docDefaults captures the
// document-level default rPr from <w:docDefaults><w:rPrDefault>...
// (ECMA-376-1 §17.7.5.5) — the implicit base of every paragraph's
// run-property inheritance chain regardless of pStyle. Mirrors upstream
// WordStyleDefinitions.combinedRunProperties (WordStyleDefinitions.java
// lines 302-315): combinedRunProperties starts from documentDefaults
// and then layers paragraph-style → run-style → direct properties on
// top.
type styleMap struct {
	styles      map[string]*styleEntry
	docDefaults runProps
	// docDefaultsRPrChildNames is the set of WordprocessingML rPr
	// child element local names that appear in
	// <w:docDefaults><w:rPrDefault><w:rPr>...</w:rPr></w:rPrDefault>.
	// Used together with each styleEntry's rPrChildNames to compute
	// the resolved style-chain name set passed to minifyRPrChildren
	// (mirrors upstream Okapi's
	// `preCombined.contains(p.getName())` branch in
	// RunProperties.minified() — RunProperties.java:497-540).
	docDefaultsRPrChildNames map[string]bool
	// defaultParagraphStyleID is the styleId of the
	// <w:style w:type="paragraph" w:default="1" w:styleId="X"> entry
	// in styles.xml. Per ECMA-376-1 §17.3.1.10 (CT_P) a paragraph
	// with no <w:pStyle> implicitly inherits from this default
	// paragraph style. effectiveRPrChildNames falls back to this
	// id when the caller passes an empty paraStyleID — mirrors
	// upstream Okapi WordStyleDefinitions.Ids.defaultBased
	// (WordStyleDefinitions.java:485-491).
	defaultParagraphStyleID string
}

// styleEntry holds a single style definition.
type styleEntry struct {
	id      string
	basedOn string   // parent style ID
	props   runProps // run properties defined directly on this style
	// rPrChildNames is the set of WordprocessingML rPr child element
	// local names that appear DIRECTLY in this style's rPr (e.g.
	// "rtl", "b", "rFonts", "lang", "color"). Used by
	// minifyRPrChildren to honour upstream Okapi's
	// `preCombined.contains(p.getName())` branch in
	// RunProperties.minified() (RunProperties.java:497-540): an
	// explicit-off WPML toggle on a run is preserved when the run's
	// resolved style chain carries the toggle BY NAME, because it
	// is needed to clear the inherited toggle.
	rPrChildNames map[string]bool
	// rPrChildXMLs is the canonical XML serialisation of each rPr child
	// authored DIRECTLY on this style (e.g. "rFonts" →
	// `<w:rFonts w:ascii="Calibri" w:hAnsi="Calibri" w:cs="Calibri"
	// w:eastAsia="Calibri"/>`). Used by per-run rPr minification to
	// drop duplicate rPr children whose value matches the resolved
	// style chain — mirrors upstream Okapi RunProperties.minified()
	// `if (preCombined.contains(p))` branch which does Object.equals
	// on the Property (RunProperties.java:497-540 +
	// RunProperty.equalsProperty implementations). Without this, a
	// per-run `<w:rFonts ...>` that exactly matches the rStyle chain's
	// rFonts gets emitted twice on the wire (948-1.docx is the
	// canonical case: `Character1` style supplies the same rFonts that
	// every rStyle="Character1" run authors directly).
	rPrChildXMLs map[string]string
}

// effectiveProps returns the effective inherited run properties for a
// paragraph that uses paraStyleID. The result is the document-default
// rPr merged with the resolved basedOn-chain rPr of the named paragraph
// style. Mirrors upstream WordStyleDefinitions.combinedRunProperties
// (WordStyleDefinitions.java lines 302-315) at the granularity native
// presently tracks (no rStyle / linked-style branching — the few
// fixtures that exercise rStyle keep the rStyle on every <w:r> so the
// per-paragraph WSO commonRunProperties pass already excludes them).
//
// When sm is nil (caller did not enable style optimisation) or
// paraStyleID is empty AND docDefaults are zero-valued, the returned
// runProps is the zero value — equivalent to "no inheritance".
//
// Consumed by parseParagraph's hidden-text filter (allHidden gate in
// wml.go) so a paragraph whose <w:vanish/> travels via pStyle (e.g.
// PageBreak.docx after WSO promoted vanish into a synthesised
// pStyle) still gets filtered on re-read despite its runs no longer
// carrying direct vanish.
func (sm *styleMap) effectiveProps(paraStyleID string) runProps {
	if sm == nil {
		return runProps{}
	}
	resolved := sm.docDefaults
	if paraStyleID != "" {
		mergeProps(&resolved, sm.resolveProps(paraStyleID))
	}
	return resolved
}

// resolveProps returns the effective run properties for a style,
// walking up the basedOn chain to merge inherited properties.
// docDefaults are NOT included here — callers that need the full
// document-default + style-chain combined view use effectiveProps.
func (sm *styleMap) resolveProps(styleID string) runProps {
	if sm == nil || styleID == "" {
		return runProps{}
	}

	entry, ok := sm.styles[styleID]
	if !ok {
		return runProps{}
	}

	// Start with parent properties (if any)
	var resolved runProps
	if entry.basedOn != "" {
		resolved = sm.resolveProps(entry.basedOn)
	}

	// Override with this style's properties
	mergeProps(&resolved, entry.props)
	return resolved
}

// mergeChainNames returns the union of two chain-name maps. Used by
// the per-run minification path to combine the paragraph's style
// chain names with the run's rStyle chain names so
// minifyRPrChildren's preCombined.contains-by-name check sees
// properties contributed by either chain. Returns a fresh map even
// when one input is nil.
func mergeChainNames(a, b map[string]bool) map[string]bool {
	out := make(map[string]bool, len(a)+len(b))
	for k := range a {
		out[k] = true
	}
	for k := range b {
		out[k] = true
	}
	return out
}

// effectiveRPrChildXML returns the canonical XML serialisation of the
// named rPr child as supplied by the resolved style chain (basedOn
// walk from styleID). Returns "" if no style in the chain authors a
// child with that name. Used by per-run rPr minification to drop a
// run's rPr child when its value matches the chain — mirrors upstream
// Okapi RunProperties.minified() `if (preCombined.contains(p))` branch
// (RunProperties.java:497-540) which does Property.equals on the
// resolved chain (RunProperty.equalsProperty implementations).
//
// The walk is child-takes-first: the youngest style in the chain
// wins (i.e. the style itself, then its basedOn, then grandparent,
// …). docDefaults are NOT consulted — at the per-run minification
// site the caller should fold them in separately if needed.
func (sm *styleMap) effectiveRPrChildXML(styleID, name string) string {
	if sm == nil || styleID == "" {
		return ""
	}
	visited := make(map[string]bool)
	cursor := styleID
	for cursor != "" {
		if visited[cursor] {
			break
		}
		visited[cursor] = true
		entry, ok := sm.styles[cursor]
		if !ok {
			break
		}
		if v, ok := entry.rPrChildXMLs[name]; ok {
			return v
		}
		cursor = entry.basedOn
	}
	return ""
}

// formatRPrChildXML serialises an rPr child start element to its
// canonical self-closing w:-prefixed XML form. Used by parseStyles to
// populate styleEntry.rPrChildXMLs for value-bearing rPr children
// (color, sz, szCs, kern, position, vertAlign, lang, …) AND for
// no-attribute toggles (b, i, outline, shadow, strike, …) so per-run
// rPr minification can drop a run's child whose canonical XML matches
// what the resolved rStyle/pStyle chain supplies — mirrors upstream
// Okapi RunProperties.minified()'s `if (preCombined.contains(p))`
// branch (RunProperties.java:497-540) which performs Property.equals
// (RunProperty.equalsProperty) on the resolved chain.
//
// The serialisation matches what parseRunProps captures into
// rPrChildren via serializeWithCapture / serializeRPrChildElement
// (the wmlPrefixed form), so a byte-equal comparison correctly
// identifies redundant authoring regardless of attribute ordering
// quirks in the source XML — both sides walk t.Attr in source order
// and write `w:`-prefixed attribute names. For pure no-attribute
// toggles (`<w:b/>`, `<w:rtl/>`, `<w:outline/>`, `<w:shadow/>`,
// `<w:strike/>`, etc.) the output is the bare self-closing element.
//
// Per ECMA-376-1 §17.3.2 every rPr child element is identified by
// its name + attribute set (no character data content), so the
// self-closing `<w:NAME ...attrs.../>` shape is the canonical wire
// form for equality purposes.
func formatRPrChildXML(t xml.StartElement) string {
	var b strings.Builder
	b.WriteByte('<')
	// rPr children in styles.xml are bound to the WML namespace; the
	// element prefix is always "w:" regardless of the source's
	// declared prefix because parseStyles strips the xmlns dance.
	// Mirrors serializeRPrChildElement / writeStartTag in runprops.go
	// which use prefixForNamespace(Name.Space) — the WML URI maps to
	// "w:" — keeping byte-equal canonical XML across the parse-time
	// run capture and the styles-side chain capture.
	b.WriteString("w:")
	b.WriteString(t.Name.Local)
	for _, a := range t.Attr {
		// Strip xmlns attributes that some encoders surface on
		// child elements; rPr children always share the parent w:
		// namespace and the writer never emits xmlns on them.
		if a.Name.Local == "xmlns" || a.Name.Space == "xmlns" {
			continue
		}
		b.WriteByte(' ')
		// Mirror serializeWithCapture's per-attribute prefix
		// resolution: the attribute's effective prefix comes from
		// its namespace URI via prefixForNamespace, NOT from the
		// source's declared prefix. This keeps the chain XML
		// byte-equal to the per-run rPrChildren XML for byte
		// equality in the chain-value strip.
		b.WriteString(prefixForNamespace(a.Name.Space))
		b.WriteString(a.Name.Local)
		b.WriteString(`="`)
		b.WriteString(escapeAttrVal(a.Value))
		b.WriteString(`"`)
	}
	b.WriteString("/>")
	return b.String()
}

// extractRStyleID returns the value of the `<w:rStyle w:val="…"/>` child
// in the given rPrChildren slice, or "" if no rStyle is present. Used
// by the per-run rPr minification path to find the character style
// that should be merged into the resolved style chain when computing
// inherited rPr.
func extractRStyleID(children []rPrChild) string {
	for _, c := range children {
		if c.name != "rStyle" {
			continue
		}
		if v, ok := parseRPrChildVal(c.xml); ok {
			return v
		}
	}
	return ""
}

// effectiveRPrChildNames returns the union of rPr-child element local
// names contributed by docDefaults + every style in the basedOn chain
// of paraStyleID. Mirrors upstream Okapi
// WordStyleDefinitions.combinedRunProperties
// (WordStyleDefinitions.java:302-315) at the granularity of "which
// property NAMES would be in the pre-combined view for a run
// inheriting this paragraph style". Consumed by minifyRPrChildren
// to decide when an explicit-off WPML toggle must be preserved as a
// style-chain clearing override (RunProperties.java:497-540 —
// `preCombined.contains(p.getName())` branch).
//
// When paraStyleID is empty, the resolver falls back to
// defaultParagraphStyleID — ECMA-376-1 §17.3.1.10 (CT_P): a
// paragraph without a pStyle inherits from the default paragraph
// style (the <w:style w:default="1" w:type="paragraph"> entry in
// styles.xml). Upstream Okapi resolves this fallback via
// WordStyleDefinitions.Ids.defaultBased (WordStyleDefinitions.java
// :485-491) — the same default the WSO synthesised-style id-
// generator already honours.
//
// Returns nil when sm is nil so callers can use a nil-pointer fast
// path. Always returns a non-nil map otherwise (even for unknown
// paraStyleID — docDefaults still contribute).
func (sm *styleMap) effectiveRPrChildNames(paraStyleID string) map[string]bool {
	if sm == nil {
		return nil
	}
	out := make(map[string]bool, len(sm.docDefaultsRPrChildNames))
	for name := range sm.docDefaultsRPrChildNames {
		out[name] = true
	}
	effectiveID := paraStyleID
	if effectiveID == "" {
		effectiveID = sm.defaultParagraphStyleID
	}
	if effectiveID != "" {
		sm.collectRPrChildNames(effectiveID, out, make(map[string]bool))
	}
	return out
}

// collectRPrChildNames walks the basedOn chain of styleID, unioning
// each style's rPrChildNames into out. The visited set guards against
// cycles in malformed styles.xml.
func (sm *styleMap) collectRPrChildNames(styleID string, out map[string]bool, visited map[string]bool) {
	if sm == nil || styleID == "" || visited[styleID] {
		return
	}
	visited[styleID] = true
	entry, ok := sm.styles[styleID]
	if !ok {
		return
	}
	for name := range entry.rPrChildNames {
		out[name] = true
	}
	if entry.basedOn != "" {
		sm.collectRPrChildNames(entry.basedOn, out, visited)
	}
}

// mergeProps applies non-zero values from src onto dst. Explicit-off
// toggles authored by src (boldClear/italicClear/strikeClear) override
// the inherited bare-on form so the resolved style chain reflects the
// child style's clearing intent — mirrors ECMA-376-1 §17.3.2.1
// (CT_OnOff: explicit val="0"/"false"/"off" clears the toggle in the
// resolved chain) + upstream Okapi RunProperties.minified()
// (RunProperties.java:497-540). Without the explicit-off branch the
// resolved chain would silently keep the parent's bold/italic/strike
// and `subtractProps` would strip a per-run override that should have
// remained on the wire (canonical fixture
// `document-style-definitions.docx`: `Normal1` clears `Style1`'s
// inherited bold).
func mergeProps(dst *runProps, src runProps) {
	if src.boldClear {
		dst.bold = false
	} else if src.bold {
		dst.bold = true
	}
	if src.italicClear {
		dst.italic = false
	} else if src.italic {
		dst.italic = true
	}
	if src.underline != "" {
		dst.underline = src.underline
	}
	if src.strikeClear {
		dst.strike = false
	} else if src.strike {
		dst.strike = true
	}
	if src.vertAlign != "" {
		dst.vertAlign = src.vertAlign
	}
	if src.vanish {
		dst.vanish = true
	}
	if src.fontName != "" {
		dst.fontName = src.fontName
	}
}

// subtractProps removes properties from run that are already present
// in the resolved style (they're inherited and redundant).
func subtractProps(run *runProps, style runProps) {
	if run.bold && style.bold {
		run.bold = false
	}
	if run.italic && style.italic {
		run.italic = false
	}
	if run.underline == style.underline && style.underline != "" {
		run.underline = ""
	}
	if run.strike && style.strike {
		run.strike = false
	}
	if run.vertAlign == style.vertAlign && style.vertAlign != "" {
		run.vertAlign = ""
	}
}

// parseStyles parses word/styles.xml from the ZIP and builds a styleMap.
//
// Two state contexts feed the inheritance model:
//
//   - <w:docDefaults><w:rPrDefault><w:rPr>...</w:rPr></w:rPrDefault>
//     captures the document-level default run properties — these are
//     applied to every paragraph regardless of pStyle (ECMA-376-1
//     §17.7.5.5 docDefaults; WordStyleDefinitions.java line 304
//     `combinedRunProperties = this.documentDefaults.runProperties()`).
//
//   - <w:style w:styleId="..."><w:rPr>...</w:rPr><w:basedOn .../></w:style>
//     captures named style definitions used by paragraphs via
//     <w:pStyle w:val="..."> (ECMA-376-1 §17.7.4 Style Definitions).
//
// We only collect the toggle/font fields native maps onto the runProps
// struct — bold, italic, underline, strike, vertAlign, vanish, fontName.
// Other rPr children (color, sz, lang, …) flow through the model's
// rPrChildren list directly from each <w:r> and don't need a styles
// table because the writer's WSO post-pass operates on the rendered
// document.xml, not on the model.
func parseStyles(zr *zip.Reader) *styleMap {
	f := zipFileByName(zr, "word/styles.xml")
	if f == nil {
		return nil
	}

	data, err := readZipFile(f)
	if err != nil {
		return nil
	}

	sm := &styleMap{styles: make(map[string]*styleEntry)}
	d := xml.NewDecoder(bytes.NewReader(data))

	var inStyle bool
	var current *styleEntry
	var inRPr bool
	// docDefaults state: the chain is
	// <w:docDefaults><w:rPrDefault><w:rPr>...</w:rPr></w:rPrDefault></w:docDefaults>
	// — we only collect from <w:rPr> children when both inDocDefaults
	// and inRPrDefault are true.
	var inDocDefaults, inRPrDefault bool

	// effectivePropsTarget returns the runProps record currently being
	// filled (style entry's props OR docDefaults). nil when not inside
	// any rPr we care about — caller must guard.
	effectivePropsTarget := func() *runProps {
		switch {
		case inStyle && current != nil && inRPr:
			return &current.props
		case inDocDefaults && inRPrDefault && inRPr:
			return &sm.docDefaults
		}
		return nil
	}

	// rPrChildNameTarget returns the name-set being filled while we
	// are inside an rPr element. Captures every rPr child local name
	// regardless of whether the field-typed dispatch above recognises
	// it, so minifyRPrChildren can match upstream's
	// `preCombined.contains(p.getName())` check by name (not by
	// typed-field presence). ECMA-376-1 §17.3.2 specifies the
	// individual rPr children — only the union of names matters here,
	// not their values.
	rPrChildNameTarget := func() *map[string]bool {
		switch {
		case inStyle && current != nil && inRPr:
			if current.rPrChildNames == nil {
				current.rPrChildNames = make(map[string]bool)
			}
			return &current.rPrChildNames
		case inDocDefaults && inRPrDefault && inRPr:
			if sm.docDefaultsRPrChildNames == nil {
				sm.docDefaultsRPrChildNames = make(map[string]bool)
			}
			return &sm.docDefaultsRPrChildNames
		}
		return nil
	}

	for {
		tok, err := d.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return sm
		}

		switch t := tok.(type) {
		case xml.StartElement:
			// Capture rPr child canonical XML BEFORE the field-typed
			// dispatch below — the dispatch below only handles a few
			// toggle/font members but the per-run minification path
			// needs the full XML form for every rPr child the chain
			// might author. Mirrors upstream Okapi
			// RunProperties.minified()'s `if (preCombined.contains(p))`
			// branch (RunProperties.java:497-540) which compares run
			// properties to the FULL preCombined view by Property
			// equality. Captured for `<w:style>` definitions only —
			// docDefaults rPr also reach here but the chain-value
			// strip in wml.go currently consults named styles via
			// effectiveRPrChildXML (which walks the basedOn chain)
			// rather than docDefaults; capturing them on docDefaults
			// would require a separate lookup path and is left for a
			// follow-up if a fixture surfaces the gap.
			//
			// Canonical fixture: HiddenTablesApachePoi.docx — the
			// `Body` paragraph style authors `<w:outline w:val="0"/>`,
			// and every `Body`-styled run also authors
			// `<w:outline w:val="0"/>` directly. Without chain XML
			// capture the per-run minification cannot detect the
			// match and the WSO post-pass lifts outline=0 into the
			// synthesised `NF974E24F-Body1` style's rPr — a divergence
			// from upstream Okapi which RunProperties.minified() drops
			// the run-level entry as a Property.equals duplicate of
			// the inherited value before WSO computes commonRunProperties.
			if inStyle && current != nil && inRPr && t.Name.Local != "rPr" {
				if current.rPrChildXMLs == nil {
					current.rPrChildXMLs = make(map[string]string)
				}
				// Last-write-wins per-name (the source style
				// authoring is single-instance per name per
				// ECMA-376-1 §17.3.2; well-formed styles.xml never
				// repeats an rPr child within one rPr block).
				current.rPrChildXMLs[t.Name.Local] = formatRPrChildXML(t)
			}
			// Capture rPr child element names BEFORE the field-typed
			// dispatch below. The handlers below only inspect a few
			// toggle/font properties; minifyRPrChildren needs the
			// full set of property names so the style-chain "contains
			// p.getName()" check covers e.g. <w:rtl/>, <w:caps/>,
			// <w:lang>, <w:color>, <w:bidi> — none of which the
			// typed-field branches recognise yet but all of which
			// upstream Okapi's preCombined view exposes by name.
			if inRPr && t.Name.Local != "rPr" {
				if dst := rPrChildNameTarget(); dst != nil {
					// Mirror upstream Okapi's resolved-chain semantics:
					// a WPML toggle authored in explicit-off form
					// (val="0"/"false"/"off") collapses to OFF in the
					// resolved style — it does NOT contribute the
					// toggle's name to the preCombined view a downstream
					// run's minified() would see. Per ECMA-376-1
					// §17.3.2 (CT_OnOff): explicit-off authoring is
					// equivalent to omission, so the chain entry adds
					// no override-by-name to inheritors.
					//
					// Without this guard, lang.docx's `editform`
					// character style — which authors `<w:specVanish
					// w:val="0"/>` and `<w:webHidden w:val="0"/>` —
					// would mark `specVanish`/`webHidden` as chain
					// names; minifyRPrChildren would then PRESERVE the
					// per-run `<w:specVanish w:val="0"/>` clearing
					// override (treating chain-by-name as a hint that
					// the parent toggle is on), diverging from
					// upstream Okapi which treats both halves as
					// no-op and drops the run-level redundant entry.
					//
					// `vanish` itself stays in the chain set
					// unconditionally (it has no clearing-form variant
					// here — `<w:vanish/>` is the bare-on form), so the
					// late stripExplicitOffVanish branch in wml.go
					// continues to work for fixtures that author bare-
					// on vanish in their style chain.
					if wpmlToggleNames[t.Name.Local] {
						off := hasAttrVal(t, "val", "0") ||
							hasAttrVal(t, "val", "false") ||
							hasAttrVal(t, "val", "off")
						if off {
							goto afterRPrChildName
						}
					}
					(*dst)[t.Name.Local] = true
				afterRPrChildName:
				}
			}
			switch t.Name.Local {
			case "docDefaults":
				inDocDefaults = true
			case "rPrDefault":
				if inDocDefaults {
					inRPrDefault = true
				}
			case "style":
				inStyle = true
				current = &styleEntry{
					id: attrVal(t, "styleId"),
				}
				// Track the default paragraph style for the implicit
				// pStyle fallback used by effectiveRPrChildNames.
				// Mirrors upstream Okapi WordStyleDefinitions.Ids
				// .defaultBased (WordStyleDefinitions.java:485-491):
				// a <w:style w:type="paragraph" w:default="1"> entry
				// is the parent of every paragraph that doesn't
				// explicitly name a pStyle.
				if attrVal(t, "type") == "paragraph" && attrVal(t, "default") == "1" {
					sm.defaultParagraphStyleID = current.id
				}

			case "basedOn":
				if inStyle && current != nil {
					current.basedOn = attrVal(t, "val")
				}

			case "rPr":
				// rPr can appear directly under <w:style> OR under
				// <w:rPrDefault>. Mark the local context so the toggle
				// handlers route into the right runProps record.
				inRPr = true

			case "b":
				if dst := effectivePropsTarget(); dst != nil {
					off := hasAttrVal(t, "val", "0") || hasAttrVal(t, "val", "false") || hasAttrVal(t, "val", "off")
					dst.bold = !off
					if off {
						dst.boldClear = true
					}
				}
			case "i":
				if dst := effectivePropsTarget(); dst != nil {
					off := hasAttrVal(t, "val", "0") || hasAttrVal(t, "val", "false") || hasAttrVal(t, "val", "off")
					dst.italic = !off
					if off {
						dst.italicClear = true
					}
				}
			case "u":
				if dst := effectivePropsTarget(); dst != nil {
					val := attrVal(t, "val")
					if val != "" && val != "none" {
						dst.underline = val
					}
				}
			case "strike":
				if dst := effectivePropsTarget(); dst != nil {
					off := hasAttrVal(t, "val", "0") || hasAttrVal(t, "val", "false") || hasAttrVal(t, "val", "off")
					dst.strike = !off
					if off {
						dst.strikeClear = true
					}
				}
			case "vertAlign":
				if dst := effectivePropsTarget(); dst != nil {
					dst.vertAlign = attrVal(t, "val")
				}
			case "vanish":
				if dst := effectivePropsTarget(); dst != nil {
					// ECMA-376-1 §17.3.2.42: <w:vanish/> defaults to
					// true; an explicit `val="0"` / `val="false"` is the
					// off form. Same toggle semantics as the per-run
					// path in parseRunProps.
					dst.vanish = !hasAttrVal(t, "val", "0") && !hasAttrVal(t, "val", "false")
				}
			case "rFonts":
				if dst := effectivePropsTarget(); dst != nil {
					if v := attrVal(t, "ascii"); v != "" {
						dst.fontName = v
					} else if v := attrVal(t, "hAnsi"); v != "" {
						dst.fontName = v
					}
				}
				// Note: the canonical XML for `<w:rFonts>` (and every
				// other rPr child) is captured by the generic block
				// at the top of `case xml.StartElement`. ECMA-376-1
				// §17.3.2.26 (CT_Fonts): the element is identified by
				// its attribute set (ascii, hAnsi, cs, eastAsia,
				// *Theme, hint), not by content; canonicalising via
				// formatRPrChildXML keeps equality stable across
				// attribute order.
			}

		case xml.EndElement:
			switch t.Name.Local {
			case "docDefaults":
				inDocDefaults = false
			case "rPrDefault":
				inRPrDefault = false
			case "style":
				if inStyle && current != nil && current.id != "" {
					sm.styles[current.id] = current
				}
				inStyle = false
				current = nil
				inRPr = false
			case "rPr":
				inRPr = false
			}
		}
	}

	return sm
}
