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
// resolving the basedOn inheritance chain.
type styleMap struct {
	styles map[string]*styleEntry
}

// styleEntry holds a single style definition.
type styleEntry struct {
	id      string
	basedOn string   // parent style ID
	props   runProps // run properties defined directly on this style
}

// resolveProps returns the effective run properties for a style,
// walking up the basedOn chain to merge inherited properties.
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
				if inStyle && current != nil {
					inRPr = true
				}

			case "b":
				if inRPr && current != nil {
					current.props.bold = !hasAttrVal(t, "val", "0") && !hasAttrVal(t, "val", "false")
				}
			case "i":
				if inRPr && current != nil {
					current.props.italic = !hasAttrVal(t, "val", "0") && !hasAttrVal(t, "val", "false")
				}
			case "u":
				if inRPr && current != nil {
					val := attrVal(t, "val")
					if val != "" && val != "none" {
						current.props.underline = val
					}
				}
			case "strike":
				if inRPr && current != nil {
					current.props.strike = !hasAttrVal(t, "val", "0") && !hasAttrVal(t, "val", "false")
				}
			case "vertAlign":
				if inRPr && current != nil {
					current.props.vertAlign = attrVal(t, "val")
				}
			case "vanish":
				if inRPr && current != nil {
					current.props.vanish = true
				}
			case "rFonts":
				if inRPr && current != nil {
					if v := attrVal(t, "ascii"); v != "" {
						current.props.fontName = v
					} else if v := attrVal(t, "hAnsi"); v != "" {
						current.props.fontName = v
					}
				}
			}

		case xml.EndElement:
			switch t.Name.Local {
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
