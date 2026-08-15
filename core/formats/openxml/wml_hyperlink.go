// Hyperlinks: resolving a w:hyperlink's r:id through the part's relationships
// to the canonical run attributes, and wrapping the linked runs in the
// sentinel pair that carries the link through a block.

package openxml

import (
	"encoding/xml"
	"strings"

	"github.com/neokapi/neokapi/core/internal/xmlesc"
	"github.com/neokapi/neokapi/core/model"
)

// wrapHyperlinkRuns wraps runs in hyperlink opening/closing markers.
//
// The emitted <w:hyperlink> start tag mirrors upstream Okapi's preserved
// startMarkup (RunContainer.java:97-99, getEvents() lines 168-176): every
// non-`r:id` attribute on the source <w:hyperlink> survives the round-
// trip, including w:tooltip, w:history, w:anchor, w:docLocation, and
// w:tgtFrame (ECMA-376-1 \u00A717.16.22 CT_Hyperlink). The native pipeline
// previously reconstructed the tag from `relID` alone and synthesised a
// non-OOXML `href=...` attribute, which dropped tooltip/history and
// added a spurious href that the reference output never carries
// (830-7.docx, 952-1.docx, 952-2.docx, hyperlink.docx,
// external_hyperlink.docx, 1341-textbox-with-a-hyperlink.docx).
// hyperlinkAttrs resolves a parsed `<w:hyperlink>` start element to the
// canonical run attributes core/projection consumes: model.AttrHref for the
// destination, model.AttrTitle for w:tooltip.
//
// The destination is not in the element. `r:id` names a relationship in the
// part's .rels, and only that lookup turns it into a URL. wrapHyperlinkRuns
// preserves the start tag verbatim and deliberately does not synthesise an
// `href` attribute onto it — upstream Okapi's reference output never carries
// one, and the skeleton write path replays those bytes. That left the
// *canonical* layer empty too, so every cross-format export produced `[text]()`
// with no destination. This fills the canonical layer without touching the
// markup.
//
// An internal jump carries no relationship: `w:anchor` names a bookmark in the
// same document, which becomes a `#fragment` destination.
func (p *wmlParser) hyperlinkAttrs(relID string, extraAttrs []xml.Attr) map[string]string {
	attrs := make(map[string]string, 2)
	if relID != "" {
		if rel, ok := p.rels[relID]; ok && rel.Target != "" {
			attrs[model.AttrHref] = rel.Target
		}
	}
	for _, a := range extraAttrs {
		switch a.Name.Local {
		case "anchor":
			if _, ok := attrs[model.AttrHref]; !ok && a.Value != "" {
				attrs[model.AttrHref] = "#" + a.Value
			}
		case "tooltip":
			if a.Value != "" {
				attrs[model.AttrTitle] = a.Value
			}
		}
	}
	if len(attrs) == 0 {
		return nil
	}
	return attrs
}

func (p *wmlParser) wrapHyperlinkRuns(runs []textRun, relID string, extraAttrs []xml.Attr) []textRun {
	// Build <w:hyperlink> opening tag preserving every captured
	// attribute. The relID feeds the r:id attribute; the remaining
	// attributes come from extraAttrs in source order.
	var b strings.Builder
	b.WriteString("<w:hyperlink")
	if relID != "" {
		b.WriteString(` r:id="`)
		b.WriteString(xmlesc.Attr(relID))
		b.WriteString(`"`)
	}
	for _, a := range extraAttrs {
		b.WriteString(" ")
		writeAttrName(&b, a.Name)
		b.WriteString(`="`)
		b.WriteString(xmlesc.Attr(a.Value))
		b.WriteString(`"`)
	}
	b.WriteString(">")
	data := b.String()

	// Create wrapper with sentinel markers
	var result []textRun
	result = append(result, textRun{
		text:  "\uE103:" + data,
		props: runProps{},
		attrs: p.hyperlinkAttrs(relID, extraAttrs),
	}) // hyperlink open sentinel
	result = append(result, runs...)
	result = append(result, textRun{text: "\uE104:" + data, props: runProps{}}) // hyperlink close sentinel
	return result
}
