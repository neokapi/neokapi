package androidxml

import (
	"context"
	"errors"
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

	// skeletonStore, when non-nil, drives byte-exact output for `kapi merge`:
	// the verbatim structure replays from SkeletonText entries and each
	// SkeletonRef is filled by re-serialising the matching block's runs via
	// renderRunsToXML. This is the only path used on merge — the
	// androidxml.original layer property is not consulted.
	skeletonStore *format.SkeletonStore
}

// Ensure Writer implements SkeletonStoreConsumer so `kapi merge` can splice
// blocks into the source-captured skeleton.
var _ format.SkeletonStoreConsumer = (*Writer)(nil)

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

// SetSkeletonStore wires a skeleton store so the writer reconstructs output from
// the source-captured skeleton (verbatim structure + per-block content refs)
// instead of re-tokenizing an androidxml.original layer property. This is the
// path `kapi merge` drives.
func (w *Writer) SetSkeletonStore(store *format.SkeletonStore) {
	w.skeletonStore = store
}

// valueKey uniquely addresses a translatable value inside a resources document.
type valueKey struct {
	kind     string // "string", "string-array", "plurals"
	name     string // entry @name
	product  string // entry @product qualifier (disambiguates same-@name <string>s)
	index    int    // string-array item index (0 for others)
	quantity string // plurals item quantity (empty for others)
}

// keyFromBlock reconstructs the value key a Block maps to, from its properties.
func keyFromBlock(b *model.Block) (valueKey, bool) {
	kind := b.Properties["androidxml.kind"]
	switch kind {
	case "string":
		// Use the raw @name (the Block.Name may carry a "@product" suffix to keep
		// same-@name siblings distinct) plus the @product qualifier so the writer
		// matches the exact source element.
		name := b.Properties["androidxml.name"]
		if name == "" {
			name = b.Name
		}
		return valueKey{kind: kind, name: name, product: b.Properties["androidxml.product"]}, true
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
	if w.skeletonStore != nil {
		return w.writeWithSkeletonStore(ctx, parts)
	}

	var original []byte
	// overrides holds only entries the writer should splice over the original:
	// blocks carrying a real target translation for the writer's locale. Entries
	// without such a target are never spliced, so an unchanged document — and any
	// unchanged entry within a partially-translated one — round-trips
	// byte-for-byte regardless of how its source bytes were originally escaped.
	overrides := make(map[valueKey]string)
	// scratch holds every block's resolved value (target if present, else source)
	// for the from-scratch path, where there is no original to preserve.
	scratch := make(map[valueKey]string)
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
					if _, seen := scratch[key]; !seen {
						order = append(order, key)
					}
					scratch[key] = w.resolveValue(block)
					// Only a real target for the writer's locale on a TRANSLATABLE
					// block becomes a splice override against the preserved original.
					// A translatable="false" entry surfaced as non-translatable
					// content (issue #928 A) must always pass through verbatim, even
					// if a tool erroneously attached a target to it.
					if block.Translatable && !w.Locale.IsEmpty() && block.HasTarget(w.Locale) {
						overrides[key] = renderRunsToXML(block.TargetRuns(w.Locale))
					}
				}
			}
		}
	}
done:
	if original != nil {
		out, err := w.rewrite(original, overrides)
		if err != nil {
			return err
		}
		_, werr := w.Output.Write(out)
		return werr
	}
	return w.writeFromScratch(order, scratch)
}

// writeWithSkeletonStore collects every block by ID, then reconstructs output by
// replaying the source-captured skeleton: SkeletonText entries pass through
// verbatim, SkeletonRef entries are filled with the matching block's content.
func (w *Writer) writeWithSkeletonStore(ctx context.Context, parts <-chan *model.Part) error {
	blocksByID := make(map[string]*model.Block)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case part, ok := <-parts:
			if !ok {
				goto done
			}
			if part.Type == model.PartBlock {
				if block, ok := part.Resource.(*model.Block); ok {
					blocksByID[block.ID] = block
				}
			}
		}
	}
done:
	if err := w.skeletonStore.Flush(); err != nil {
		return fmt.Errorf("androidxml writer: flush skeleton: %w", err)
	}
	return w.writeFromSkeleton(blocksByID)
}

// writeFromSkeleton reads skeleton entries and reconstructs the document. Text
// entries replay verbatim; ref entries are replaced by the matching block's
// inner content, re-serialised via renderRunsToXML (target runs when the writer
// has a locale and the block carries one, otherwise source runs) so inline
// xliff:g/CDATA/printf markup re-emits faithfully and text is XML-escaped via
// encodeText. An untranslated roundtrip is therefore byte-exact: each ref's
// rendered source equals the original inner bytes.
func (w *Writer) writeFromSkeleton(blocks map[string]*model.Block) error {
	for {
		entry, err := w.skeletonStore.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("androidxml writer: read skeleton: %w", err)
		}
		switch entry.Type {
		case format.SkeletonText:
			if _, err := w.Output.Write(entry.Data); err != nil {
				return err
			}
		case format.SkeletonRef:
			if block, ok := blocks[string(entry.Data)]; ok {
				if _, err := io.WriteString(w.Output, w.resolveValue(block)); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// resolveValue returns the encoded XML element content for a Block: the target
// runs for the writer's locale when present, otherwise the source runs. Plain
// TextRun content is entity-encoded; inline codes (printf args, xliff:g spans,
// CDATA) re-emit their Data verbatim (it already carries the exact source
// bytes), so markup is never double-escaped. This is the inverse of the reader's
// buildRuns and produces directly-spliceable element content.
func (w *Writer) resolveValue(block *model.Block) string {
	runs := block.SourceRuns()
	// A non-translatable content block (a surfaced translatable="false" entry)
	// always renders its source verbatim — its value is never replaced by a
	// translation, so the skeleton ref fills with the original bytes.
	if block.Translatable && !w.Locale.IsEmpty() && block.HasTarget(w.Locale) {
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
			product, _ := t.attrValue("product")
			if newVal, ok := byKey[valueKey{kind: "string", name: name, product: product}]; ok {
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
			b.WriteString("\"")
			if key.product != "" {
				b.WriteString(" product=\"")
				b.WriteString(encodeAttr(key.product))
				b.WriteString("\"")
			}
			b.WriteString(">")
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
