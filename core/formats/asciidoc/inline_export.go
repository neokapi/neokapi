package asciidoc

import (
	"strings"

	"github.com/neokapi/neokapi/core/model"
)

// adocInlineTag maps a canonical inline run Type to its AsciiDoc delimiters
// (open, close). This is the AsciiDoc format's own projection of the shared
// canonical type vocabulary (core/model/vocabularies), used by the normalized
// (cross-format) write path so a foreign document's inline formatting renders
// as AsciiDoc markup rather than echoing the source format's captured Data.
// link:hyperlink and media:image are handled separately in renderInlineAsciidoc
// because they carry attributes. TestAsciidocInlineTagCoverage asserts this
// table covers every attribute-free formatting type the vocabulary defines that
// AsciiDoc can express, so a newly added type cannot silently fall through.
//
// Types with no AsciiDoc inline equivalent (bidi, handwriting) intentionally
// have no entry — their content survives, the wrapper is dropped.
var adocInlineTag = map[string][2]string{
	"fmt:bold":          {"*", "*"},
	"fmt:italic":        {"_", "_"},
	"fmt:code":          {"`", "`"},
	"fmt:strikethrough": {"[.line-through]#", "#"},
	"fmt:highlight":     {"#", "#"},
	"fmt:underline":     {"[.underline]#", "#"},
	"fmt:superscript":   {"^", "^"},
	"fmt:subscript":     {"~", "~"},
}

// renderInlineAsciidoc renders a run sequence as AsciiDoc inline content from
// each run's canonical Type — text verbatim, formatting from adocInlineTag,
// links as `link:href[text]`, images as `image:src[alt]`. It never consults a
// run's source-format Data, so the same AsciiDoc results whatever the source
// format. This is the cross-format projection; byte-exact round-trips use the
// skeleton path (model.RenderRunsWithData) instead.
func renderInlineAsciidoc(runs []model.Run) string {
	var sb strings.Builder
	var open []string // stack of closing delimiters (or "" for dropped wrappers)
	for _, r := range runs {
		switch {
		case r.Text != nil:
			sb.WriteString(r.Text.Text)
		case r.PcOpen != nil:
			switch r.PcOpen.Type {
			case "link:hyperlink":
				// link:href[text] — the link text is the paired content.
				sb.WriteString("link:" + r.PcOpen.Attr(model.AttrHref) + "[")
				open = append(open, "]")
			case "media:image", "link:image":
				// image:src[alt] — the alt text is the paired content.
				sb.WriteString("image:" + r.PcOpen.Attr(model.AttrSrc) + "[")
				open = append(open, "]")
			default:
				if m, ok := adocInlineTag[r.PcOpen.Type]; ok {
					sb.WriteString(m[0])
					open = append(open, m[1])
				} else {
					open = append(open, "")
				}
			}
		case r.PcClose != nil:
			if n := len(open); n > 0 {
				sb.WriteString(open[n-1])
				open = open[:n-1]
			}
		case r.Ph != nil:
			switch r.Ph.Type {
			case "media:image", "link:image":
				// Self-closing image (e.g. read from HTML <img>): alt lives in
				// the run attributes, not as paired content.
				sb.WriteString("image:" + r.Ph.Attr(model.AttrSrc) + "[" + r.Ph.Attr(model.AttrAlt) + "]")
			default:
				if r.Ph.Equiv != "" {
					sb.WriteString(r.Ph.Equiv)
				}
			}
		}
	}
	for i := len(open) - 1; i >= 0; i-- {
		sb.WriteString(open[i])
	}
	return sb.String()
}
