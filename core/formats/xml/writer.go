package xml

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
)

// Writer implements DataFormatWriter for XML files.
type Writer struct {
	format.BaseFormatWriter
	resolver      format.SubfilterResolver
	skeletonStore *format.SkeletonStore
	cfg           *WriterCfg
}

// Ensure Writer implements SubfilterAware, SkeletonStoreConsumer, and
// WriterConfigurable.
var _ format.SubfilterAware = (*Writer)(nil)
var _ format.SkeletonStoreConsumer = (*Writer)(nil)
var _ format.WriterConfigurable = (*Writer)(nil)

// NewWriter creates a new XML writer.
func NewWriter() *Writer {
	return &Writer{
		BaseFormatWriter: format.BaseFormatWriter{
			FormatName: "xml",
		},
		cfg: NewWriterCfg(),
	}
}

// WriterConfig returns the writer's serialization config.
func (w *Writer) WriterConfig() format.DataFormatConfig { return w.cfg }

// SetSubfilterResolver sets the resolver for creating sub-format writers.
func (w *Writer) SetSubfilterResolver(resolver format.SubfilterResolver) {
	w.resolver = resolver
}

// SetSkeletonStore sets the skeleton store for byte-exact output.
func (w *Writer) SetSkeletonStore(store *format.SkeletonStore) {
	w.skeletonStore = store
}

// Write consumes Parts from a channel and writes reconstructed XML.
func (w *Writer) Write(ctx context.Context, parts <-chan *model.Part) error {
	if w.skeletonStore != nil {
		return w.writeWithSkeletonStore(ctx, parts)
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case part, ok := <-parts:
			if !ok {
				return nil
			}
			if part.Type == model.PartBlock {
				block, ok := part.Resource.(*model.Block)
				if !ok {
					continue
				}
				text := w.blockText(block)
				if _, err := fmt.Fprint(w.Output, text); err != nil {
					return err
				}
			}
			if part.Type == model.PartLayerStart {
				if layer, ok := part.Resource.(*model.Layer); ok && layer.IsEmbedded() {
					val, err := w.writeChildLayer(ctx, layer, parts)
					if err != nil {
						return fmt.Errorf("xml: writing child layer %s: %w", layer.Name, err)
					}
					if _, err := fmt.Fprint(w.Output, val); err != nil {
						return err
					}
				}
			}
		}
	}
}

// writeWithSkeletonStore collects all blocks, then reconstructs output from skeleton entries.
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
		return fmt.Errorf("xml writer: flush skeleton: %w", err)
	}
	return w.writeFromSkeleton(blocksByID)
}

// writeFromSkeleton reads skeleton entries and fills in block content.
// This produces byte-exact output — only translated text differs from the original.
//
// When cfg.EmitDeclaration is set and the source skeleton's leading
// bytes don't already contain an `<?xml ?>` prologue, one is injected
// at the start of output. Source documents that already begin with a
// declaration pass through unchanged.
//
// `blocks` is also used to expand inline-attribute reference markers
// (see writeRunsXML / expandInlineAttrRefs).
func (w *Writer) writeFromSkeleton(blocks map[string]*model.Block) error {
	first := true
	for {
		entry, err := w.skeletonStore.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("xml writer: read skeleton: %w", err)
		}
		switch entry.Type {
		case format.SkeletonText:
			data := entry.Data
			if first && w.cfg != nil && w.cfg.EmitDeclaration {
				// EmitDeclaration mode rewrites the source's prologue
				// to a fresh canonical declaration: any existing
				// declaration is stripped, then a new one is emitted.
				// This matches the behavior of tools that always emit
				// a normalized prologue (e.g. upstream Okapi) where
				// a source `<?xml version="1.0"?>` becomes
				// `<?xml version="1.0" encoding="UTF-8"?>`.
				bom, rest := splitLeadingBOM(data)
				rest = stripLeadingXMLDeclaration(rest)
				if len(bom) > 0 {
					if _, err := w.Output.Write(bom); err != nil {
						return err
					}
				}
				decl := fmt.Sprintf("<?xml version=\"%s\" encoding=\"%s\"?>\n",
					w.cfg.DeclarationVersion, w.cfg.DeclarationEncoding)
				if _, err := io.WriteString(w.Output, decl); err != nil {
					return err
				}
				data = rest
			}
			first = false
			if _, err := w.Output.Write(data); err != nil {
				return err
			}
		case format.SkeletonRef:
			first = false
			if block, ok := blocks[string(entry.Data)]; ok {
				text := w.renderBlockXML(block, blocks)
				if _, err := io.WriteString(w.Output, text); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// stripLeadingXMLDeclaration returns data with any leading `<?xml ... ?>`
// declaration removed (along with the trailing newline that typically
// follows it). Whitespace before the declaration is preserved. If no
// declaration is present, data is returned unchanged.
func stripLeadingXMLDeclaration(data []byte) []byte {
	i := 0
	for i < len(data) && (data[i] == ' ' || data[i] == '\t' || data[i] == '\r' || data[i] == '\n') {
		i++
	}
	if !bytes.HasPrefix(data[i:], []byte("<?xml")) {
		return data
	}
	end := bytes.Index(data[i:], []byte("?>"))
	if end < 0 {
		return data
	}
	cut := i + end + 2
	// Consume one trailing newline after the declaration if present
	// — declarations conventionally sit on their own line, and the
	// replacement we emit ends with `\n` already.
	if cut < len(data) && data[cut] == '\n' {
		cut++
	} else if cut+1 < len(data) && data[cut] == '\r' && data[cut+1] == '\n' {
		cut += 2
	}
	return data[cut:]
}

// splitLeadingBOM returns (bom, rest) where bom is the UTF-8 byte-order
// mark if present at the start of data, and rest is the remainder.
// Used to inject an XML declaration after the BOM rather than before.
func splitLeadingBOM(data []byte) (bom, rest []byte) {
	if bytes.HasPrefix(data, []byte("\xef\xbb\xbf")) {
		return data[:3], data[3:]
	}
	return nil, data
}

// renderBlockXML renders a block's text for XML output. Text parts are XML-escaped
// while inline span markup (from span Data) is written as-is since it's already valid XML.
//
// When a target translation is being rendered (not the source) and the
// block isn't marked PreserveWhitespace, runs are passed through
// collapseRunsWhitespace first. Skeleton-mode reading keeps source runs
// verbatim so byte-equal round-trip works when nothing is translated;
// once a target replaces the source, we need to mirror okapi's
// whitespace collapsing inside translatable text containers.
//
// blocks is the project-wide block map used to resolve inline-attribute
// reference markers embedded by the reader in inline placeholder data
// (see reader.go's injectInlineAttrRefs). Pass nil to disable
// substitution; markers will then be stripped to keep output well-formed.
func (w *Writer) renderBlockXML(block *model.Block, blocks map[string]*model.Block) string {
	runs := block.Source
	useTarget := !w.Locale.IsEmpty() && block.HasTarget(w.Locale)
	if useTarget {
		runs = block.TargetRuns(w.Locale)
	}
	var buf strings.Builder
	escape := xmlEscapeString
	if block.Type == "attribute" {
		// Attribute values only need to escape `&`, `<`, and the
		// delimiter quote (we use `"`); leaving `>` and `'` literal
		// matches okapi's reference writer.
		escape = xmlEscapeAttrValue
	}
	if useTarget && !block.PreserveWhitespace && block.Type != "attribute" {
		runs = collapseRenderWhitespace(runs)
	}
	writeRunsXML(&buf, runs, escape, blocks, w)
	return buf.String()
}

// writeRunsXML walks a Run sequence, applying `escape` to TextRun
// content and writing inline-code Data verbatim (already valid XML).
//
// Inline-code Data may contain inline-attribute reference markers
// `\x01REF:<id>\x02` injected by the reader. expandInlineAttrRefs
// replaces them with the referenced block's translated attribute value
// (rendered via the same writer for consistent escaping / locale
// selection).
func writeRunsXML(buf *strings.Builder, runs []model.Run, escape func(string) string, blocks map[string]*model.Block, w *Writer) {
	for _, r := range runs {
		switch {
		case r.Text != nil:
			buf.WriteString(escape(r.Text.Text))
		case r.Ph != nil:
			buf.WriteString(expandInlineAttrRefs(r.Ph.Data, blocks, w))
		case r.PcOpen != nil:
			buf.WriteString(expandInlineAttrRefs(r.PcOpen.Data, blocks, w))
		case r.PcClose != nil:
			buf.WriteString(r.PcClose.Data)
		case r.Sub != nil:
			buf.WriteString(r.Sub.Ref)
		case r.Plural != nil:
			if form, ok := r.Plural.Forms[model.PluralOther]; ok {
				writeRunsXML(buf, form, escape, blocks, w)
			}
		case r.Select != nil:
			if form, ok := r.Select.Cases["other"]; ok {
				writeRunsXML(buf, form, escape, blocks, w)
			}
		}
	}
}

// expandInlineAttrRefs walks data looking for inline-attribute
// reference markers (`\x01REF:<id>\x02`). For each marker, the
// referenced attribute block is rendered through renderBlockXML — this
// gives the translated value with proper attribute-value escaping
// applied. Markers whose target block isn't present in the map are
// stripped (silently dropped) so the resulting XML stays well-formed.
func expandInlineAttrRefs(data string, blocks map[string]*model.Block, w *Writer) string {
	if !strings.ContainsRune(data, '\x01') {
		return data
	}
	var b strings.Builder
	b.Grow(len(data))
	i := 0
	for i < len(data) {
		start := strings.IndexByte(data[i:], '\x01')
		if start < 0 {
			b.WriteString(data[i:])
			break
		}
		start += i
		end := strings.IndexByte(data[start+1:], '\x02')
		if end < 0 {
			// Malformed marker (shouldn't happen): emit verbatim.
			b.WriteString(data[i:])
			break
		}
		end += start + 1
		// Marker payload: between `\x01` and `\x02`. Format: `REF:<id>`.
		payload := data[start+1 : end]
		b.WriteString(data[i:start])
		if strings.HasPrefix(payload, "REF:") && blocks != nil {
			id := payload[len("REF:"):]
			if ref, ok := blocks[id]; ok {
				b.WriteString(w.renderBlockXML(ref, blocks))
			}
		}
		i = end + 1
	}
	return b.String()
}

// xmlEscapeString escapes the four XML special characters (&, <, >, ")
// that may appear in element-text content. The double quote isn't
// strictly required by XML 1.0 inside element text, but okapi's
// reference writer emits it as the &quot; entity when it appears in
// extracted content — escaping here keeps round-trip parity. The
// apostrophe stays unescaped because okapi leaves it literal even
// when the source used &apos;. Whitespace (newlines, tabs) is preserved
// for byte-exact skeleton roundtrip.
func xmlEscapeString(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch r {
		case '&':
			b.WriteString("&amp;")
		case '<':
			b.WriteString("&lt;")
		case '>':
			b.WriteString("&gt;")
		case '"':
			b.WriteString("&quot;")
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// collapseRenderWhitespace returns a run slice whose TextRun contents
// have ASCII whitespace runs collapsed to a single space, matching
// okapi's serialization for translatable text inside non-preserve-space
// containers. Inline-code runs (Ph/PcOpen/PcClose/Sub) pass through
// unchanged. The collapse spans run boundaries: a TextRun ending in
// whitespace, followed by an inline code, followed by a TextRun
// starting with whitespace, becomes single-space + code + content with
// the leading whitespace dropped.
//
// Leading whitespace at the very start of the runs collapses to a
// single space (preserved, not dropped) when the source had any
// leading whitespace. Trailing whitespace likewise collapses to a
// single space. This matches okapi's serialization for elements like
// `<string>   Be aware ...   </string>` where okapi outputs
// ` Be aware ... ` (one leading + one trailing space).
func collapseRenderWhitespace(runs []model.Run) []model.Run {
	if len(runs) == 0 {
		return runs
	}
	// Detect leading and trailing whitespace before collapsing —
	// okapi preserves a single space at each end when the source
	// had any whitespace there.
	leadingWS := runsStartWithWhitespace(runs)
	trailingWS := runsEndWithWhitespace(runs)

	out := make([]model.Run, 0, len(runs))
	pendingSpace := false
	started := false
	for _, r := range runs {
		if r.Text == nil {
			if pendingSpace && started {
				out = appendSpaceTo(out)
				pendingSpace = false
			}
			out = append(out, r)
			started = true
			continue
		}
		s := r.Text.Text
		var b strings.Builder
		b.Grow(len(s))
		for _, ch := range s {
			if ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r' {
				pendingSpace = true
				continue
			}
			if pendingSpace && started {
				b.WriteByte(' ')
			}
			pendingSpace = false
			b.WriteRune(ch)
			started = true
		}
		if b.Len() > 0 {
			out = append(out, model.Run{Text: &model.TextRun{Text: b.String()}})
		}
	}

	// Re-attach a single leading/trailing space when the source had
	// whitespace there. Without this, `<string>   Be aware   </string>`
	// becomes `<string>Be aware</string>` (no spaces) — okapi keeps
	// `<string> Be aware </string>` (one space at each end).
	if leadingWS && len(out) > 0 {
		if first := out[0]; first.Text != nil {
			out[0] = model.Run{Text: &model.TextRun{Text: " " + first.Text.Text}}
		} else {
			out = append([]model.Run{{Text: &model.TextRun{Text: " "}}}, out...)
		}
	}
	if trailingWS && len(out) > 0 {
		out = appendSpaceTo(out)
	}
	return out
}

// runsStartWithWhitespace reports whether the first textual character
// in the run sequence is ASCII whitespace. Inline-code runs without
// any text in front are skipped — okapi's behavior depends on whether
// the content text starts with whitespace, not on whether the very
// first run is a code.
func runsStartWithWhitespace(runs []model.Run) bool {
	for _, r := range runs {
		if r.Text == nil {
			continue
		}
		if len(r.Text.Text) == 0 {
			continue
		}
		c := r.Text.Text[0]
		return c == ' ' || c == '\t' || c == '\n' || c == '\r'
	}
	return false
}

// runsEndWithWhitespace reports whether the last textual character in
// the run sequence is ASCII whitespace. Mirrors runsStartWithWhitespace
// for the trailing edge.
func runsEndWithWhitespace(runs []model.Run) bool {
	for i := len(runs) - 1; i >= 0; i-- {
		r := runs[i]
		if r.Text == nil {
			continue
		}
		s := r.Text.Text
		if len(s) == 0 {
			continue
		}
		c := s[len(s)-1]
		return c == ' ' || c == '\t' || c == '\n' || c == '\r'
	}
	return false
}

// appendSpaceTo appends a single-space TextRun, coalescing with the
// previous TextRun if present.
func appendSpaceTo(runs []model.Run) []model.Run {
	if n := len(runs); n > 0 && runs[n-1].Text != nil {
		runs[n-1].Text.Text += " "
		return runs
	}
	return append(runs, model.Run{Text: &model.TextRun{Text: " "}})
}

// xmlEscapeAttrValue escapes the characters required inside a
// double-quoted XML attribute value: `&`, `<`, and the `"` delimiter
// only. `>` and `'` stay literal — XML 1.0 §2.4 doesn't require their
// escaping inside an attribute value, and okapi-bridge's reference
// round-trip output for ITS test01.xml shows literal `>` and `'`
// inside attribute values.
func xmlEscapeAttrValue(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch r {
		case '&':
			b.WriteString("&amp;")
		case '<':
			b.WriteString("&lt;")
		case '"':
			b.WriteString("&quot;")
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// writeChildLayer collects parts until the matching PartLayerEnd and writes them
// through the appropriate sub-format writer.
func (w *Writer) writeChildLayer(ctx context.Context, layer *model.Layer, parts <-chan *model.Part) (string, error) {
	var childParts []*model.Part
	for {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case part, ok := <-parts:
			if !ok {
				return "", fmt.Errorf("unexpected end of parts stream in child layer %s", layer.ID)
			}
			if part.Type == model.PartLayerEnd {
				if endLayer, ok := part.Resource.(*model.Layer); ok && endLayer.ID == layer.ID {
					goto collected
				}
			}
			childParts = append(childParts, part)
		}
	}

collected:
	if w.resolver == nil {
		return w.fallbackChildText(childParts), nil
	}

	subWriter, err := w.resolver.ResolveWriter(layer.Format)
	if err != nil {
		return w.fallbackChildText(childParts), nil
	}

	var buf bytes.Buffer
	if err := subWriter.SetOutputWriter(&buf); err != nil {
		return "", err
	}
	subWriter.SetLocale(w.Locale)

	childCh := make(chan *model.Part, len(childParts))
	for _, p := range childParts {
		childCh <- p
	}
	close(childCh)

	if err := subWriter.Write(ctx, childCh); err != nil {
		return "", err
	}
	if err := subWriter.Close(); err != nil {
		return "", err
	}

	return buf.String(), nil
}

// fallbackChildText concatenates block texts when no sub-writer is available.
func (w *Writer) fallbackChildText(parts []*model.Part) string {
	var sb strings.Builder
	for _, p := range parts {
		if p.Type == model.PartBlock {
			if block, ok := p.Resource.(*model.Block); ok {
				sb.WriteString(w.blockText(block))
			}
		}
	}
	return sb.String()
}

func (w *Writer) blockText(block *model.Block) string {
	if !w.Locale.IsEmpty() && block.HasTarget(w.Locale) {
		return model.RenderRunsWithData(block.TargetRuns(w.Locale))
	}
	return model.RenderRunsWithData(block.Source)
}
