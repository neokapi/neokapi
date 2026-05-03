package xliff

import (
	"encoding/xml"
	"strings"
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
// model.Run sequence consumed by tools that don't speak xliff. The
// mapping is the same lossy one the old parseInlineContent did, kept
// here for backward compat — but the writer reads from the native IR,
// so the downconversion no longer has to round-trip.
//
// (Imported lazily by reader.go via the existing parseInlineContent
// indirection.)
