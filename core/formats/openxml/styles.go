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
}

// styleEntry holds a single style definition.
type styleEntry struct {
	id      string
	basedOn string   // parent style ID
	props   runProps // run properties defined directly on this style
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
