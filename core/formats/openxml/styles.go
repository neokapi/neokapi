package openxml

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
// When paraStyleID is empty, the resolver falls back to
// defaultParagraphStyleID — ECMA-376-1 §17.3.1.10 (CT_P): a paragraph
// without a pStyle inherits from the default paragraph style (the
// <w:style w:default="1" w:type="paragraph"> entry in styles.xml).
// Mirrors upstream Okapi WordStyleDefinitions.Ids.defaultBased
// (WordStyleDefinitions.java:485-491) — the same fallback the
// effectiveRPrChildNames sister method already honours, keeping the
// "names" view and the "props" view consistent for paragraphs that
// inherit purely through the default paragraph style.
//
// When sm is nil (caller did not enable style optimisation) or
// paraStyleID is empty AND there is no defaultParagraphStyleID AND
// docDefaults are zero-valued, the returned runProps is the zero
// value — equivalent to "no inheritance".
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
	effectiveID := paraStyleID
	if effectiveID == "" {
		effectiveID = sm.defaultParagraphStyleID
	}
	if effectiveID != "" {
		mergeProps(&resolved, sm.resolveProps(effectiveID))
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
