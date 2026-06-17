package openxml

import (
	"bytes"
	"encoding/xml"
	"regexp"
	"strconv"
	"strings"

	"github.com/neokapi/neokapi/core/model"
)

// paraRole is the semantic role a paragraph style (or numbering) implies,
// recorded as additive stand-off metadata on a Block (WS1/WS2). It is NEVER
// serialized back into the .docx — byte-faithful round-trip is unaffected —
// and exists purely so semantic export (e.g. DOCX → clean Markdown) and the
// visual editor can read the document's structure.
type paraRole struct {
	role  string // model.Role* constant; "" when the style implies no role
	level int    // heading level (1-9); 0 otherwise
}

// styleRoleMap maps a paragraph styleId to the semantic role it implies. It is
// built from word/styles.xml by resolving each paragraph style's w:name,
// w:outlineLvl, and basedOn chain against the well-known heading/title
// conventions (only styles that resolve to a role are recorded). A nil/empty
// map is valid: roleForParaStyle then falls back to the language-independent
// built-in styleId heuristic, so headings still resolve when styles.xml is
// absent or was not loaded.
type styleRoleMap map[string]paraRole

// rawStyle is the subset of a <w:style> entry that role resolution needs.
type rawStyle struct {
	id         string
	typ        string // CT_Style@type; "" defaults to "paragraph" (ECMA-376-1 §17.7.4.17)
	name       string // <w:name w:val> (localized, e.g. "heading 1")
	basedOn    string // <w:basedOn w:val>
	outlineLvl int    // <w:pPr><w:outlineLvl w:val>; -1 when absent
}

// headingNumRE matches both the language-independent built-in heading styleId
// "Heading1".."Heading9" (ECMA-376 fixed identifiers Word emits regardless of
// UI language) and the localized English style name "heading 1". The optional
// whitespace covers the name form; the captured digit is the level.
var headingNumRE = regexp.MustCompile(`(?i)^heading\s*([1-9])$`)

// buildStyleRoleMap parses a styles.xml part and returns the styleId → role
// map. It streams the XML, collecting each <w:style>'s id/type/name/basedOn/
// outlineLvl, then resolves every paragraph style to a role (built-in styleId
// convention → localized name → explicit outlineLvl → basedOn inheritance).
// Returns nil when the input is empty or no style resolves to a role.
func buildStyleRoleMap(stylesXML []byte) styleRoleMap {
	if len(stylesXML) == 0 {
		return nil
	}
	dec := xml.NewDecoder(bytes.NewReader(stylesXML))
	raws := make(map[string]*rawStyle)
	var cur *rawStyle
	for {
		tok, err := dec.Token()
		if err != nil {
			break // EOF or malformed tail — resolve whatever we collected
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "style":
				cur = &rawStyle{outlineLvl: -1}
				for _, a := range t.Attr {
					switch a.Name.Local {
					case "type":
						cur.typ = a.Value
					case "styleId":
						cur.id = a.Value
					}
				}
			case "name":
				if cur != nil {
					cur.name = attrLocalVal(t, "val")
				}
			case "basedOn":
				if cur != nil {
					cur.basedOn = attrLocalVal(t, "val")
				}
			case "outlineLvl":
				if cur != nil {
					if n, err := strconv.Atoi(attrLocalVal(t, "val")); err == nil {
						cur.outlineLvl = n
					}
				}
			}
		case xml.EndElement:
			if t.Name.Local == "style" && cur != nil {
				if cur.id != "" {
					raws[cur.id] = cur
				}
				cur = nil
			}
		}
	}
	if len(raws) == 0 {
		return nil
	}
	out := make(styleRoleMap)
	for id := range raws {
		if r := resolveStyleRole(id, raws, make(map[string]bool)); r.role != "" {
			out[id] = r
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// resolveStyleRole resolves the role a single styleId implies, walking the
// basedOn chain when the style itself carries no heading/title signal. The
// visited set guards against cycles in malformed styles.xml.
func resolveStyleRole(id string, raws map[string]*rawStyle, visited map[string]bool) paraRole {
	if id == "" || visited[id] {
		return paraRole{}
	}
	visited[id] = true
	r, ok := raws[id]
	if !ok {
		// Style referenced (e.g. via basedOn) but not defined here — fall
		// back to the built-in styleId convention.
		return builtinStyleRole(id)
	}
	// Only paragraph styles map to block roles. CT_Style@type defaults to
	// "paragraph" when omitted.
	if r.typ != "" && r.typ != "paragraph" {
		return paraRole{}
	}
	if br := builtinStyleRole(id); br.role != "" {
		return br
	}
	if nr := nameRole(r.name); nr.role != "" {
		return nr
	}
	if r.outlineLvl >= 0 {
		return paraRole{role: model.RoleHeading, level: min(r.outlineLvl+1, 9)}
	}
	if r.basedOn != "" {
		return resolveStyleRole(r.basedOn, raws, visited)
	}
	return paraRole{}
}

// builtinStyleRole resolves the well-known, language-independent built-in
// paragraph styleIds (Heading1..Heading9, Title) to a role. Returns the zero
// paraRole for any other styleId.
func builtinStyleRole(styleID string) paraRole {
	if m := headingNumRE.FindStringSubmatch(styleID); m != nil {
		lvl, _ := strconv.Atoi(m[1])
		return paraRole{role: model.RoleHeading, level: lvl}
	}
	if strings.EqualFold(styleID, "Title") {
		return paraRole{role: model.RoleTitle}
	}
	return paraRole{}
}

// nameRole resolves a localized style name ("heading 1", "Title") to a role.
func nameRole(name string) paraRole {
	name = strings.TrimSpace(name)
	if name == "" {
		return paraRole{}
	}
	if m := headingNumRE.FindStringSubmatch(name); m != nil {
		lvl, _ := strconv.Atoi(m[1])
		return paraRole{role: model.RoleHeading, level: lvl}
	}
	if strings.EqualFold(name, "Title") {
		return paraRole{role: model.RoleTitle}
	}
	return paraRole{}
}

// roleForParaStyle returns the role a paragraph's styleId implies, consulting
// the resolved styleRoleMap first and falling back to the built-in styleId
// heuristic (so headings resolve even when styles.xml was not loaded).
func roleForParaStyle(styleID string, m styleRoleMap) paraRole {
	if styleID == "" {
		return paraRole{}
	}
	if m != nil {
		if r, ok := m[styleID]; ok {
			return r
		}
	}
	return builtinStyleRole(styleID)
}

// paraHasNumbering reports whether a captured <w:pPr> raw string declares a
// <w:numPr> (the signal that a paragraph is a list item). The numbering
// definition itself (numbering.xml) is not consulted — presence of numPr is a
// sufficient list-item signal for semantic export.
func paraHasNumbering(rawParaProps string) bool {
	return strings.Contains(rawParaProps, "<w:numPr") || strings.Contains(rawParaProps, "<numPr")
}

// attrLocalVal returns the value of the attribute whose local name matches
// local (namespace-prefix-agnostic), or "" when absent.
func attrLocalVal(start xml.StartElement, local string) string {
	for _, a := range start.Attr {
		if a.Name.Local == local {
			return a.Value
		}
	}
	return ""
}

// applyParagraphRole records the semantic role a paragraph implies on its
// Block: a heading/title style wins (numbered headings keep their heading
// role), otherwise a numbering declaration marks the block as a list item, and
// failing both a note part (footnotes/endnotes) supplies a footnote role. It
// also records the part's plane (header/footer → furniture) and per-paragraph
// visibility (a fully hidden paragraph → hidden) — the §8 structure facets.
// All additive stand-off metadata; never serialized back, so byte-faithful
// round-trip is unaffected.
func (p *wmlParser) applyParagraphRole(block *model.Block, paraStyleID, rawParaProps string, hidden bool) {
	if block == nil {
		return
	}
	if r := roleForParaStyle(paraStyleID, p.roleStyles); r.role != "" {
		block.SetSemanticRole(r.role, r.level)
	} else if paraHasNumbering(rawParaProps) {
		block.SetSemanticRole(model.RoleListItem, 0)
	} else if p.partNoteRole != "" {
		block.SetSemanticRole(p.partNoteRole, 0)
	}
	if p.partPlane != "" {
		block.SetLayoutLayer(p.partPlane)
	}
	if hidden {
		block.SetVisibility(model.VisibilityHidden)
	}
}

// docxPartStructure classifies a WordprocessingML part path into the plane and
// fallback note-role it implies: running headers/footers are furniture; the
// footnotes/endnotes parts give their paragraphs a footnote role.
func docxPartStructure(partPath string) (plane, noteRole string) {
	base := partPath
	if i := strings.LastIndex(base, "/"); i >= 0 {
		base = base[i+1:]
	}
	switch {
	case strings.HasPrefix(base, "header"):
		return model.LayerFurniture, ""
	case strings.HasPrefix(base, "footer"):
		return model.LayerFurniture, ""
	case base == "footnotes.xml" || base == "endnotes.xml":
		return "", model.RoleFootnote
	}
	return "", ""
}
