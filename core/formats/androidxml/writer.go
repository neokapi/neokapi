package androidxml

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
)

// Writer implements DataFormatWriter for Android string resources.
//
// Output strategy: the reader stores the original document bytes on the root
// Layer. The writer re-tokenizes those bytes and rewrites only the inner content
// of translatable values whose corresponding Block carries a changed
// translation, preserving every other byte (the prolog, comments, whitespace,
// attribute order, entity encoding, CDATA, xliff:g markup) exactly. This
// guarantees byte-faithful round-trips for documents whose content was not
// modified. When no original is available (synthetic pipelines), the writer
// builds a canonical resources document from scratch.
type Writer struct {
	format.BaseFormatWriter
	cfg *Config
}

// NewWriter creates a new Android string-resources writer.
func NewWriter() *Writer {
	cfg := &Config{}
	cfg.Reset()
	return &Writer{
		BaseFormatWriter: format.BaseFormatWriter{
			FormatName: "androidxml",
		},
		cfg: cfg,
	}
}

// Config returns the writer's config for customization.
func (w *Writer) Config() *Config { return w.cfg }

// valueKey uniquely addresses a translatable value inside a resources document.
type valueKey struct {
	kind     string // "string", "string-array", "plurals"
	name     string // entry @name
	index    int    // string-array item index (0 for others)
	quantity string // plurals item quantity (empty for others)
}

// keyFromBlock reconstructs the value key a Block maps to, from its properties.
func keyFromBlock(b *model.Block) (valueKey, bool) {
	kind := b.Properties["androidxml.kind"]
	switch kind {
	case "string":
		return valueKey{kind: kind, name: b.Name}, true
	case "string-array":
		name := b.Properties["androidxml.arrayName"]
		idx, _ := strconv.Atoi(b.Properties["androidxml.index"])
		return valueKey{kind: kind, name: name, index: idx}, true
	case "plurals":
		name := b.Properties["androidxml.pluralsName"]
		return valueKey{kind: kind, name: name, quantity: b.Properties["androidxml.quantity"]}, true
	}
	return valueKey{}, false
}

// Write consumes Parts and writes the reconstructed resources document.
func (w *Writer) Write(ctx context.Context, parts <-chan *model.Part) error {
	var original []byte
	byKey := make(map[valueKey]string)
	// orderedStrings preserves emission order for the from-scratch path.
	var order []valueKey

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case part, ok := <-parts:
			if !ok {
				goto done
			}
			switch part.Type {
			case model.PartLayerStart:
				if layer, ok := part.Resource.(*model.Layer); ok && layer.IsRoot() {
					if raw, ok := layer.Properties["androidxml.original"]; ok {
						original = []byte(raw)
					}
				}
			case model.PartBlock:
				if block, ok := part.Resource.(*model.Block); ok {
					key, ok := keyFromBlock(block)
					if !ok {
						continue
					}
					if _, seen := byKey[key]; !seen {
						order = append(order, key)
					}
					byKey[key] = w.resolveValue(block)
				}
			}
		}
	}
done:
	if original != nil {
		out, err := w.rewrite(original, byKey)
		if err != nil {
			return err
		}
		_, werr := w.Output.Write(out)
		return werr
	}
	return w.writeFromScratch(order, byKey)
}

// resolveValue returns the encoded XML element content for a Block: the target
// runs for the writer's locale when present, otherwise the source runs. Plain
// TextRun content is entity-encoded; inline codes (printf args, xliff:g spans,
// CDATA) re-emit their Data verbatim (it already carries the exact source
// bytes), so markup is never double-escaped. This is the inverse of the reader's
// buildRuns and produces directly-spliceable element content.
func (w *Writer) resolveValue(block *model.Block) string {
	runs := block.SourceRuns()
	if !w.Locale.IsEmpty() && block.HasTarget(w.Locale) {
		runs = block.TargetRuns(w.Locale)
	}
	return renderRunsToXML(runs)
}

// renderRunsToXML serialises a Run sequence to Android resource element content.
// TextRun text is entity-encoded ('&', '<', '>' → entities); placeholder and
// paired-code Data is emitted verbatim.
func renderRunsToXML(runs []model.Run) string {
	var b strings.Builder
	for _, r := range runs {
		switch {
		case r.Text != nil:
			b.WriteString(encodeText(r.Text.Text))
		case r.Ph != nil:
			b.WriteString(r.Ph.Data)
		case r.PcOpen != nil:
			b.WriteString(r.PcOpen.Data)
		case r.PcClose != nil:
			b.WriteString(r.PcClose.Data)
		case r.Sub != nil:
			b.WriteString(r.Sub.Ref)
		default:
			// Plural/Select are not produced by this reader; render via the
			// shared helper as a defensive fallback.
			b.WriteString(model.RenderRunsWithData([]model.Run{r}))
		}
	}
	return b.String()
}

// rewrite re-tokenizes the original bytes and replaces the inner content of each
// translatable value with its resolved string when it differs. Everything else
// is copied byte-for-byte.
func (w *Writer) rewrite(original []byte, byKey map[valueKey]string) ([]byte, error) {
	toks, err := newTokenizer(string(original)).tokenize()
	if err != nil {
		return nil, fmt.Errorf("androidxml: %w", err)
	}

	var out strings.Builder
	out.Grow(len(original))

	for i := 0; i < len(toks); i++ {
		t := toks[i]
		if t.kind != tokStartTag {
			out.WriteString(t.raw)
			continue
		}
		switch t.name {
		case "string":
			end := matchEnd(toks, i, "string")
			if end < 0 {
				out.WriteString(t.raw)
				continue
			}
			entry := toks[i : end+1]
			name, _ := t.attrValue("name")
			if newVal, ok := byKey[valueKey{kind: "string", name: name}]; ok {
				out.WriteString(rewriteElementInner(entry, newVal))
			} else {
				writeAll(&out, entry)
			}
			i = end
		case "string-array":
			end := matchEnd(toks, i, "string-array")
			if end < 0 {
				out.WriteString(t.raw)
				continue
			}
			name, _ := t.attrValue("name")
			out.WriteString(w.rewriteItems(toks[i:end+1], "string-array", name, byKey))
			i = end
		case "plurals":
			end := matchEnd(toks, i, "plurals")
			if end < 0 {
				out.WriteString(t.raw)
				continue
			}
			name, _ := t.attrValue("name")
			out.WriteString(w.rewriteItems(toks[i:end+1], "plurals", name, byKey))
			i = end
		default:
			out.WriteString(t.raw)
		}
	}

	return []byte(out.String()), nil
}

// rewriteItems re-serialises a <string-array>/<plurals> entry span, splicing the
// inner content of each top-level <item> whose key has a replacement. entry[0]
// is the container start tag and entry[len-1] its end tag. Items are matched at
// depth 1 relative to the container so an <item> nested inside inline markup is
// never mistaken for a list item.
func (w *Writer) rewriteItems(entry []token, kind, name string, byKey map[valueKey]string) string {
	var out strings.Builder
	idx := 0
	depth := 0
	for i := 0; i < len(entry); i++ {
		t := entry[i]
		switch t.kind {
		case tokStartTag:
			depth++
			if depth == 2 && t.name == "item" {
				end := matchEnd(entry, i, "item")
				if end < 0 {
					out.WriteString(t.raw)
					depth--
					continue
				}
				itemSpan := entry[i : end+1]
				key := valueKey{kind: kind, name: name}
				if kind == "string-array" {
					key.index = idx
				} else {
					key.quantity, _ = t.attrValue("quantity")
				}
				if newVal, ok := byKey[key]; ok {
					out.WriteString(rewriteElementInner(itemSpan, newVal))
				} else {
					writeAll(&out, itemSpan)
				}
				idx++
				// The item's matching end tag is the last token of itemSpan; it
				// closes the depth we just opened, so settle back to depth 1 and
				// resume after the item span.
				depth--
				i = end
				continue
			}
		case tokEndTag:
			depth--
		}
		out.WriteString(t.raw)
	}
	return out.String()
}

// rewriteElementInner replaces the inner content of an element span (span[0] =
// start tag, span[len-1] = end tag) with newVal — which is already encoded XML
// element content (see resolveValue) — while preserving the start tag, its
// attributes, and the end tag exactly. If the replacement equals the original
// inner bytes, the span is emitted verbatim so unchanged values stay
// byte-identical.
func rewriteElementInner(span []token, newVal string) string {
	if len(span) < 2 {
		return concatRaw(span)
	}
	// Self-closing element has no inner span.
	if span[0].kind == tokSelfClose {
		return concatRaw(span)
	}

	var origInner strings.Builder
	for i := 1; i < len(span)-1; i++ {
		origInner.WriteString(span[i].raw)
	}
	if origInner.String() == newVal {
		return concatRaw(span)
	}

	var b strings.Builder
	b.WriteString(span[0].raw) // start tag
	b.WriteString(newVal)      // new (already-encoded) inner content
	b.WriteString(span[len(span)-1].raw)
	return b.String()
}

func concatRaw(toks []token) string {
	var b strings.Builder
	for _, t := range toks {
		b.WriteString(t.raw)
	}
	return b.String()
}

func writeAll(b *strings.Builder, toks []token) {
	for _, t := range toks {
		b.WriteString(t.raw)
	}
}

// writeFromScratch builds a minimal canonical resources document when no
// original is available (a synthetic pipeline produced blocks without first
// reading a resources file). It groups blocks by entry and emits <string>,
// <string-array>, and <plurals> in first-seen order.
func (w *Writer) writeFromScratch(order []valueKey, byKey map[valueKey]string) error {
	var b strings.Builder
	b.WriteString("<?xml version=\"1.0\" encoding=\"utf-8\"?>\n<resources>\n")

	emittedArrays := map[string]bool{}
	emittedPlurals := map[string]bool{}

	// Values in byKey are already encoded XML element content (resolveValue).
	for _, key := range order {
		switch key.kind {
		case "string":
			b.WriteString("    <string name=\"")
			b.WriteString(encodeAttr(key.name))
			b.WriteString("\">")
			b.WriteString(byKey[key])
			b.WriteString("</string>\n")
		case "string-array":
			if emittedArrays[key.name] {
				continue
			}
			emittedArrays[key.name] = true
			b.WriteString("    <string-array name=\"")
			b.WriteString(encodeAttr(key.name))
			b.WriteString("\">\n")
			for i := 0; ; i++ {
				v, ok := byKey[valueKey{kind: "string-array", name: key.name, index: i}]
				if !ok {
					break
				}
				b.WriteString("        <item>")
				b.WriteString(v)
				b.WriteString("</item>\n")
			}
			b.WriteString("    </string-array>\n")
		case "plurals":
			if emittedPlurals[key.name] {
				continue
			}
			emittedPlurals[key.name] = true
			b.WriteString("    <plurals name=\"")
			b.WriteString(encodeAttr(key.name))
			b.WriteString("\">\n")
			for _, q := range []string{"zero", "one", "two", "few", "many", "other"} {
				v, ok := byKey[valueKey{kind: "plurals", name: key.name, quantity: q}]
				if !ok {
					continue
				}
				b.WriteString("        <item quantity=\"")
				b.WriteString(q)
				b.WriteString("\">")
				b.WriteString(v)
				b.WriteString("</item>\n")
			}
			b.WriteString("    </plurals>\n")
		}
	}

	b.WriteString("</resources>\n")
	_, err := io.WriteString(w.Output, b.String())
	return err
}
