package xliff

import (
	"encoding/xml"
	"strings"

	"github.com/neokapi/neokapi/core/model"
)

// parseNativeContent walks the inner XML of a <source>/<target>/
// <seg-source> body and builds the xliff-native IR. Unknown elements
// are dropped (their text content is kept) — okapi's reader behaves
// the same way.
func parseNativeContent(innerXML string) *NativeContent {
	if innerXML == "" {
		return &NativeContent{}
	}
	wrapped := "<root>" + innerXML + "</root>"
	dec := xml.NewDecoder(strings.NewReader(wrapped))
	dec.Strict = false

	// Skip the root start.
	for {
		tok, err := dec.Token()
		if err != nil {
			return &NativeContent{}
		}
		if _, ok := tok.(xml.StartElement); ok {
			break
		}
	}
	inls := parseInlines(dec, "root")
	return &NativeContent{Inlines: inls}
}

// parseInlines reads tokens until the matching EndElement for parent.
// It coalesces consecutive CharData into a single Text node.
func parseInlines(dec *xml.Decoder, parent string) []Inline {
	var out []Inline
	var buf strings.Builder
	flush := func() {
		if buf.Len() > 0 {
			out = append(out, Inline{Text: &Text{Content: buf.String()}})
			buf.Reset()
		}
	}
	for {
		tok, err := dec.Token()
		if err != nil {
			flush()
			return out
		}
		switch t := tok.(type) {
		case xml.CharData:
			buf.Write(t)
		case xml.StartElement:
			flush()
			if in, ok := startElementToInline(dec, t); ok {
				out = append(out, in)
			} else {
				// Unknown element — drop the wrapper, keep the text.
				inner := parseInlines(dec, t.Name.Local)
				for _, child := range inner {
					if child.Text != nil {
						buf.WriteString(child.Text.Content)
					}
				}
				flush()
			}
		case xml.EndElement:
			flush()
			return out
		}
	}
}

// startElementToInline converts a recognized xliff inline start tag to
// an Inline node. For container elements (g, bpt, ept, ph, it, mrk,
// sub), it recursively parses children. Returns false for unknown tags
// so the caller can decide how to handle them.
//
// Attrs are copied verbatim (preserving order and namespace prefix) so
// the writer round-trips every attribute, including custom-namespace
// ones like cms:translate that the well-known semantic fields don't
// surface.
func startElementToInline(dec *xml.Decoder, t xml.StartElement) (Inline, bool) {
	attrs := copyAttrs(t.Attr)
	switch t.Name.Local {
	case "g":
		g := &G{Attrs: attrs}
		g.Children = parseInlines(dec, "g")
		return Inline{G: g}, true

	case "x":
		// Self-closing or empty — drain any end token.
		drainToEnd(dec, "x")
		return Inline{X: &X{Attrs: attrs}}, true

	case "bx":
		drainToEnd(dec, "bx")
		return Inline{Bx: &Bx{Attrs: attrs}}, true

	case "ex":
		drainToEnd(dec, "ex")
		return Inline{Ex: &Ex{Attrs: attrs}}, true

	case "bpt":
		bpt := &Bpt{Attrs: attrs}
		bpt.Inner = parseInlines(dec, "bpt")
		return Inline{Bpt: bpt}, true

	case "ept":
		ept := &Ept{Attrs: attrs}
		ept.Inner = parseInlines(dec, "ept")
		return Inline{Ept: ept}, true

	case "ph":
		ph := &Ph{Attrs: attrs}
		ph.Inner = parseInlines(dec, "ph")
		return Inline{Ph: ph}, true

	case "it":
		it := &It{Attrs: attrs}
		it.Inner = parseInlines(dec, "it")
		return Inline{It: it}, true

	case "mrk":
		mrk := &Mrk{Attrs: attrs}
		mrk.Children = parseInlines(dec, "mrk")
		return Inline{Mrk: mrk}, true

	case "sub":
		sub := &Sub{Attrs: attrs}
		sub.Children = parseInlines(dec, "sub")
		return Inline{Sub: sub}, true
	}
	return Inline{}, false
}

// copyAttrs converts encoding/xml's attribute slice into our
// namespace-aware Attr slice, preserving source order.
func copyAttrs(in []xml.Attr) []Attr {
	if len(in) == 0 {
		return nil
	}
	out := make([]Attr, len(in))
	for i, a := range in {
		out[i] = Attr{Space: a.Name.Space, Local: a.Name.Local, Value: a.Value}
	}
	return out
}

// drainToEnd consumes tokens until the EndElement that closes the
// just-opened element. encoding/xml emits an end token for self-closing
// elements too, so this handles both `<x/>` and `<x></x>` forms.
func drainToEnd(dec *xml.Decoder, name string) {
	depth := 1
	for depth > 0 {
		tok, err := dec.Token()
		if err != nil {
			return
		}
		switch tok.(type) {
		case xml.StartElement:
			depth++
		case xml.EndElement:
			depth--
		}
	}
}

// nativeToRuns downconverts a native xliff inline tree to the generic
// model.Run sequence consumed by tools that don't speak xliff. It
// produces byte-identical output to parseInlineContent walking the same
// source XML, but reuses the already-parsed *NativeContent so each
// <source>/<target>/<seg-source> body is decoded only once (the reader
// no longer spins up a second xml.Decoder per segment just to build the
// generic Runs).
//
// The mapping is the same lossy one parseInlineContent applied: <g>/<mrk>
// become a PcOpen…PcClose pair, paired bpt/ept and singleton ph/x/it map
// to a single inline-code run carrying the recovered native code Data and
// equiv-text, and translatable <sub> sub-flow text rides through as
// trailing Text runs (one per sub Text node, in tree order).
func nativeToRuns(nc *NativeContent) []model.Run {
	if nc == nil || len(nc.Inlines) == 0 {
		return nil
	}
	var runs []model.Run
	var textBuf strings.Builder
	flushText := func() {
		if textBuf.Len() == 0 {
			return
		}
		runs = append(runs, model.Run{Text: &model.TextRun{Text: textBuf.String()}})
		textBuf.Reset()
	}
	appendInlines(&runs, &textBuf, flushText, nc.Inlines)
	flushText()
	return runs
}

// appendInlines walks one level of the inline tree, emitting runs that
// mirror parseInlineContent. textBuf coalesces adjacent text exactly as
// the streaming parser did; flushText drains it at every element
// boundary.
func appendInlines(runs *[]model.Run, textBuf *strings.Builder, flushText func(), inls []Inline) {
	for i := range inls {
		in := &inls[i]
		switch {
		case in.Text != nil:
			textBuf.WriteString(in.Text.Content)

		case in.Bpt != nil:
			data, subs := codeDataAndSubs(in.Bpt.Inner)
			flushText()
			*runs = append(*runs, model.Run{PcOpen: &model.PcOpenRun{
				ID: AttrLookup(in.Bpt.Attrs, "id"), Type: ctypeToSpanType(AttrLookup(in.Bpt.Attrs, "ctype")),
				Data: data, Equiv: AttrLookup(in.Bpt.Attrs, "equiv-text"),
			}})
			appendSubTexts(runs, subs)

		case in.Ept != nil:
			data, subs := codeDataAndSubs(in.Ept.Inner)
			flushText()
			*runs = append(*runs, model.Run{PcClose: &model.PcCloseRun{
				ID: AttrLookup(in.Ept.Attrs, "id"), Type: ctypeToSpanType(AttrLookup(in.Ept.Attrs, "ctype")),
				Data: data, Equiv: AttrLookup(in.Ept.Attrs, "equiv-text"),
			}})
			appendSubTexts(runs, subs)

		case in.Ph != nil:
			data, subs := codeDataAndSubs(in.Ph.Inner)
			flushText()
			*runs = append(*runs, model.Run{Ph: &model.PlaceholderRun{
				ID: AttrLookup(in.Ph.Attrs, "id"), Type: ctypeToSpanType(AttrLookup(in.Ph.Attrs, "ctype")),
				Data: data, Equiv: AttrLookup(in.Ph.Attrs, "equiv-text"),
			}})
			appendSubTexts(runs, subs)

		case in.X != nil:
			flushText()
			*runs = append(*runs, model.Run{Ph: &model.PlaceholderRun{
				ID: AttrLookup(in.X.Attrs, "id"), Type: ctypeToSpanType(AttrLookup(in.X.Attrs, "ctype")),
				Equiv: AttrLookup(in.X.Attrs, "equiv-text"),
			}})

		case in.Bx != nil:
			flushText()
			*runs = append(*runs, model.Run{PcOpen: &model.PcOpenRun{
				ID: AttrLookup(in.Bx.Attrs, "id"), Type: ctypeToSpanType(AttrLookup(in.Bx.Attrs, "ctype")),
				Equiv: AttrLookup(in.Bx.Attrs, "equiv-text"),
			}})

		case in.Ex != nil:
			flushText()
			*runs = append(*runs, model.Run{PcClose: &model.PcCloseRun{
				ID: AttrLookup(in.Ex.Attrs, "id"), Type: ctypeToSpanType(AttrLookup(in.Ex.Attrs, "ctype")),
				Equiv: AttrLookup(in.Ex.Attrs, "equiv-text"),
			}})

		case in.G != nil:
			id := AttrLookup(in.G.Attrs, "id")
			flushText()
			*runs = append(*runs, model.Run{PcOpen: &model.PcOpenRun{
				ID: id, Type: ctypeToSpanType(AttrLookup(in.G.Attrs, "ctype")),
				Equiv: AttrLookup(in.G.Attrs, "equiv-text"),
			}})
			appendInlines(runs, textBuf, flushText, in.G.Children)
			flushText()
			*runs = append(*runs, model.Run{PcClose: &model.PcCloseRun{ID: id}})

		case in.It != nil:
			data, subs := codeDataAndSubs(in.It.Inner)
			flushText()
			typ := ctypeToSpanType(AttrLookup(in.It.Attrs, "ctype"))
			equiv := AttrLookup(in.It.Attrs, "equiv-text")
			id := AttrLookup(in.It.Attrs, "id")
			switch AttrLookup(in.It.Attrs, "pos") {
			case "open":
				*runs = append(*runs, model.Run{PcOpen: &model.PcOpenRun{ID: id, Type: typ, Data: data, Equiv: equiv}})
			case "close":
				*runs = append(*runs, model.Run{PcClose: &model.PcCloseRun{ID: id, Type: typ, Data: data, Equiv: equiv}})
			default:
				*runs = append(*runs, model.Run{Ph: &model.PlaceholderRun{ID: id, Type: typ, Data: data, Equiv: equiv}})
			}
			appendSubTexts(runs, subs)

		case in.Mrk != nil:
			mid := AttrLookup(in.Mrk.Attrs, "mid")
			mtype := AttrLookup(in.Mrk.Attrs, "mtype")
			flushText()
			*runs = append(*runs, model.Run{PcOpen: &model.PcOpenRun{ID: mid, Type: "xliff:mrk:" + mtype}})
			appendInlines(runs, textBuf, flushText, in.Mrk.Children)
			flushText()
			*runs = append(*runs, model.Run{PcClose: &model.PcCloseRun{ID: mid, Type: "xliff:mrk"}})

		case in.Sub != nil:
			// A top-level <sub> contributes no run in parseInlineContent
			// (its content is consumed by readElementText and discarded);
			// nested <sub> text is surfaced through the parent code's
			// subTexts instead. Drop it here too.
		}
	}
}

// appendSubTexts emits one trailing Text run per recovered <sub> text
// fragment, mirroring parseInlineContent's per-sub-flow text emission.
func appendSubTexts(runs *[]model.Run, subs []string) {
	for _, s := range subs {
		*runs = append(*runs, model.Run{Text: &model.TextRun{Text: s}})
	}
}

// codeDataAndSubs recovers, from an inline code's Inner tree, the same
// (data, subTexts) pair readInlineCodeContent produced from the raw
// token stream:
//
//   - data is the concatenation of every text node anywhere inside the
//     code (tags dropped), the "native code" payload.
//   - subTexts is one entry per text node that sits directly inside a
//     <sub> sub-flow and not inside a further nested code element, in
//     tree order. Each such text node is a separate entry (the streaming
//     parser flushed at sub / nested-code boundaries; the IR already
//     coalesced adjacent CharData into one Text node, so one Text node
//     maps to one entry).
func codeDataAndSubs(inner []Inline) (string, []string) {
	var data strings.Builder
	var subs []string
	collectCodeData(&data, &subs, inner, false)
	return data.String(), subs
}

func collectCodeData(data *strings.Builder, subs *[]string, inls []Inline, inSub bool) {
	for i := range inls {
		in := &inls[i]
		switch {
		case in.Text != nil:
			data.WriteString(in.Text.Content)
			if inSub {
				*subs = append(*subs, in.Text.Content)
			}
		case in.G != nil:
			collectCodeData(data, subs, in.G.Children, inSub)
		case in.Mrk != nil:
			collectCodeData(data, subs, in.Mrk.Children, inSub)
		case in.Bpt != nil:
			collectCodeData(data, subs, in.Bpt.Inner, false)
		case in.Ept != nil:
			collectCodeData(data, subs, in.Ept.Inner, false)
		case in.Ph != nil:
			collectCodeData(data, subs, in.Ph.Inner, false)
		case in.It != nil:
			collectCodeData(data, subs, in.It.Inner, false)
		case in.Sub != nil:
			collectCodeData(data, subs, in.Sub.Children, true)
		}
	}
}
