package openxml

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"errors"
	"io"
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
//nolint:unused // foundation; wire-up to parseRunProps in subsequent commit
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

// mergeProps applies non-zero values from src onto dst.
func mergeProps(dst *runProps, src runProps) {
	if src.bold {
		dst.bold = true
	}
	if src.italic {
		dst.italic = true
	}
	if src.underline != "" {
		dst.underline = src.underline
	}
	if src.strike {
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
					(*dst)[t.Name.Local] = true
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
					dst.bold = !hasAttrVal(t, "val", "0") && !hasAttrVal(t, "val", "false")
				}
			case "i":
				if dst := effectivePropsTarget(); dst != nil {
					dst.italic = !hasAttrVal(t, "val", "0") && !hasAttrVal(t, "val", "false")
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
					dst.strike = !hasAttrVal(t, "val", "0") && !hasAttrVal(t, "val", "false")
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
