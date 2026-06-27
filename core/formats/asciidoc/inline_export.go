package asciidoc

import (
	"strings"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/projection"
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
	sink := &adocInlineSink{}
	projection.WalkInline(runs, sink)
	sink.flush()
	return sink.sb.String()
}

// adocInlineSink maps the shared inline-run stream (projection.WalkInline) to
// AsciiDoc, owning the open-delimiter stack the paired-code close needs. It
// replaces the writer's former bespoke run loop; WalkInline now handles run
// decoding + plural/select 'other'-branch resolution.
type adocInlineSink struct {
	sb   strings.Builder
	open []string // stack of closing delimiters (or "" for dropped wrappers)
}

func (s *adocInlineSink) Text(t string) { s.sb.WriteString(t) }

func (s *adocInlineSink) Open(r *model.PcOpenRun) {
	switch r.Type {
	case "link:hyperlink":
		// link:href[text] — the link text is the paired content.
		s.sb.WriteString("link:" + r.Attr(model.AttrHref) + "[")
		s.open = append(s.open, "]")
	case "media:image", "link:image":
		// image:src[alt] — the alt text is the paired content.
		s.sb.WriteString("image:" + r.Attr(model.AttrSrc) + "[")
		s.open = append(s.open, "]")
	default:
		if m, ok := adocInlineTag[r.Type]; ok {
			s.sb.WriteString(m[0])
			s.open = append(s.open, m[1])
		} else {
			s.open = append(s.open, "")
		}
	}
}

func (s *adocInlineSink) Close(*model.PcCloseRun) {
	if n := len(s.open); n > 0 {
		s.sb.WriteString(s.open[n-1])
		s.open = s.open[:n-1]
	}
}

func (s *adocInlineSink) Placeholder(r *model.PlaceholderRun) {
	switch r.Type {
	case "media:image", "link:image":
		// Self-closing image (e.g. read from HTML <img>): alt lives in the run
		// attributes, not as paired content.
		s.sb.WriteString("image:" + r.Attr(model.AttrSrc) + "[" + r.Attr(model.AttrAlt) + "]")
	default:
		if r.Equiv != "" {
			s.sb.WriteString(r.Equiv)
		}
	}
}

func (s *adocInlineSink) flush() {
	for i := len(s.open) - 1; i >= 0; i-- {
		s.sb.WriteString(s.open[i])
	}
}
