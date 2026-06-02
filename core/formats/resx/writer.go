package resx

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
)

// Writer implements DataFormatWriter for .NET RESX / .resw files.
//
// Output strategy: the reader stores the original document bytes on the root
// Layer. The writer re-tokenizes those bytes and rewrites only the <value>
// content of string <data> entries whose corresponding Block carries a changed
// translation, preserving every other byte (the resheader boilerplate, the
// schema, typed/binary data, attribute order, whitespace, entity encoding)
// exactly. This guarantees byte-faithful round-trips for documents whose
// content was not modified. When no original is available (synthetic
// pipelines), the writer builds a canonical ResX document from scratch.
type Writer struct {
	format.BaseFormatWriter
	cfg           *Config
	skeletonStore *format.SkeletonStore
}

// Ensure Writer satisfies the skeleton-consumer contract. When a skeleton store
// is wired (kapi merge), the writer replays the source-captured skeleton —
// every byte verbatim except each <value> inner content, which is re-encoded
// from the resolved Block value looked up by ID.
var _ format.SkeletonStoreConsumer = (*Writer)(nil)

// SetSkeletonStore wires a skeleton store so the writer reconstructs output from
// the byte-exact skeleton captured at extract time, splicing each block's
// resolved value into its <value> ref. This is the path kapi merge drives.
func (w *Writer) SetSkeletonStore(store *format.SkeletonStore) {
	w.skeletonStore = store
}

// NewWriter creates a new RESX writer.
func NewWriter() *Writer {
	cfg := &Config{}
	cfg.Reset()
	return &Writer{
		BaseFormatWriter: format.BaseFormatWriter{
			FormatName: "resx",
		},
		cfg: cfg,
	}
}

// Config returns the writer's config for customization.
func (w *Writer) Config() *Config { return w.cfg }

// Write consumes Parts and writes the reconstructed RESX document.
func (w *Writer) Write(ctx context.Context, parts <-chan *model.Part) error {
	var original []byte
	// byName maps a <data>/@name to the output value for that entry.
	byName := make(map[string]string)
	// blocksByID maps a Block.ID to the block itself, for the skeleton path,
	// which resolves each <value> ref by ID rather than by @name.
	blocksByID := make(map[string]*model.Block)
	var order []string

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
					if raw, ok := layer.Properties["resx.original"]; ok {
						original = []byte(raw)
					}
				}
			case model.PartBlock:
				if block, ok := part.Resource.(*model.Block); ok {
					if _, seen := byName[block.Name]; !seen {
						order = append(order, block.Name)
					}
					byName[block.Name] = w.resolveValue(block)
					blocksByID[block.ID] = block
				}
			}
		}
	}
done:
	// Skeleton path (kapi merge): replay the source-captured skeleton, splicing
	// each block's resolved value into its <value> ref. Byte-exact when no
	// translation changed; otherwise only the <value> inner content differs.
	if w.skeletonStore != nil {
		if err := w.skeletonStore.Flush(); err != nil {
			return fmt.Errorf("resx writer: flush skeleton: %w", err)
		}
		return w.writeFromSkeleton(blocksByID)
	}
	if original != nil {
		out, err := w.rewrite(original, byName)
		if err != nil {
			return err
		}
		_, werr := w.Output.Write(out)
		return werr
	}
	return w.writeFromScratch(order, byName)
}

// writeFromSkeleton reconstructs the document from the skeleton store: text
// entries are written verbatim, and each ref entry is replaced by the
// XML-encoded resolved value of the referenced block (the same encodeText used
// to write <value> inner content elsewhere). A ref whose block is missing
// emits nothing, leaving the surrounding <value></value> tags empty rather than
// failing the merge.
func (w *Writer) writeFromSkeleton(blocksByID map[string]*model.Block) error {
	for {
		entry, err := w.skeletonStore.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("resx writer: read skeleton: %w", err)
		}
		switch entry.Type {
		case format.SkeletonText:
			if _, err := w.Output.Write(entry.Data); err != nil {
				return err
			}
		case format.SkeletonRef:
			block, ok := blocksByID[string(entry.Data)]
			if !ok {
				continue
			}
			if _, err := io.WriteString(w.Output, encodeText(w.resolveValue(block))); err != nil {
				return err
			}
		}
	}
	return nil
}

// resolveValue returns the output value for a block: the target text for the
// writer's locale when present, otherwise the source text. The value is the
// markup-preserving rendering so .NET placeholders (lifted to Ph runs by the
// reader) re-emit verbatim.
func (w *Writer) resolveValue(block *model.Block) string {
	if !w.Locale.IsEmpty() && block.HasTarget(w.Locale) {
		return model.RenderRunsWithData(block.TargetRuns(w.Locale))
	}
	return model.RenderRunsWithData(block.SourceRuns())
}

// rewrite re-tokenizes the original bytes and replaces the <value> content of
// each translatable string <data> entry with the resolved value when it
// differs from the original. Everything else is copied byte-for-byte.
func (w *Writer) rewrite(original []byte, byName map[string]string) ([]byte, error) {
	toks, err := newTokenizer(string(original)).tokenize()
	if err != nil {
		return nil, fmt.Errorf("resx: %w", err)
	}

	var out strings.Builder
	out.Grow(len(original))

	for i := 0; i < len(toks); i++ {
		t := toks[i]
		if t.kind != tokStartTag || t.name != "data" {
			out.WriteString(t.raw)
			continue
		}
		end := matchEnd(toks, i, "data")
		if end < 0 {
			out.WriteString(t.raw)
			continue
		}
		entryToks := toks[i : end+1]
		newVal, replace := w.replacementFor(t, byName)
		if replace {
			out.WriteString(rewriteEntryValue(entryToks, newVal))
		} else {
			for _, et := range entryToks {
				out.WriteString(et.raw)
			}
		}
		i = end
	}

	return []byte(out.String()), nil
}

// replacementFor decides whether the <data> entry whose start tag is t should
// have its <value> replaced, and with what string. Returns replace=false for
// non-translatable entries or entries with no collected block.
func (w *Writer) replacementFor(start token, byName map[string]string) (string, bool) {
	if _, ok := start.attrValue("type"); ok {
		return "", false
	}
	if _, ok := start.attrValue("mimetype"); ok {
		return "", false
	}
	name, ok := start.attrValue("name")
	if !ok {
		return "", false
	}
	if w.cfg.SkipNameDataReferences {
		if c, ok := firstRune(name); ok && c == '>' {
			return "", false
		}
	}
	val, ok := byName[name]
	if !ok {
		return "", false
	}
	return val, true
}

// rewriteEntryValue re-serialises a <data> entry's token span, replacing the
// inner character data of its first <value> child with the encoded newVal while
// preserving all other tokens (the start tag with its attributes, the
// surrounding whitespace, the <comment>, the </data> tag) exactly. If the
// encoded replacement equals the original inner bytes, the entry is emitted
// verbatim — keeping unchanged round-trips byte-identical.
func rewriteEntryValue(entryToks []token, newVal string) string {
	// Locate the <value> child element and its content span (the token
	// indices strictly between its start and matching end tag).
	valueStart, valueEnd := locateChild(entryToks, "value")
	if valueStart < 0 {
		// No <value> child — emit verbatim.
		var b strings.Builder
		for _, t := range entryToks {
			b.WriteString(t.raw)
		}
		return b.String()
	}

	encoded := encodeText(newVal)

	// Original inner bytes (between value start tag and value end tag).
	var origInner strings.Builder
	for i := valueStart + 1; i < valueEnd; i++ {
		origInner.WriteString(entryToks[i].raw)
	}
	// If the new encoded value is byte-identical to the original inner content,
	// no rewrite is needed.
	if origInner.String() == encoded {
		var b strings.Builder
		for _, t := range entryToks {
			b.WriteString(t.raw)
		}
		return b.String()
	}

	var b strings.Builder
	for i := range entryToks {
		switch {
		case i <= valueStart:
			b.WriteString(entryToks[i].raw)
		case i == valueStart+1:
			// First token after <value> start: emit the encoded replacement,
			// then skip the rest of the original inner tokens.
			b.WriteString(encoded)
		case i < valueEnd:
			// Skip remaining original inner tokens (already replaced).
		default:
			// valueEnd (the </value> end tag) and everything after.
			b.WriteString(entryToks[i].raw)
		}
	}
	return b.String()
}

// locateChild returns the token indices of the start tag and matching end tag
// of the first depth-1 child element with the given name inside entryToks
// (whose [0] is the parent <data> start tag). Returns (-1, -1) if absent or if
// the child is self-closing/empty in a way that has no inner span.
func locateChild(entryToks []token, name string) (int, int) {
	depth := 0
	for i := range entryToks {
		t := entryToks[i]
		switch t.kind {
		case tokStartTag:
			depth++
			if depth == 2 && t.name == name {
				// Find matching end tag at this depth.
				end := matchEnd(entryToks, i, name)
				if end > i {
					return i, end
				}
				return -1, -1
			}
		case tokSelfClose:
			// A self-closing <value/> has no inner span to replace.
			if depth == 1 && t.name == name {
				return -1, -1
			}
		case tokEndTag:
			depth--
		}
	}
	return -1, -1
}

// writeFromScratch builds a minimal canonical ResX document when no original is
// available (a synthetic pipeline produced blocks without first reading a RESX
// file). It emits the standard ResX 2.0 resheader boilerplate followed by one
// string <data> per collected entry.
func (w *Writer) writeFromScratch(order []string, byName map[string]string) error {
	var b strings.Builder
	b.WriteString(scratchHeader)
	for _, name := range order {
		val := byName[name]
		b.WriteString("  <data name=\"")
		b.WriteString(encodeAttr(name))
		b.WriteString("\" xml:space=\"preserve\">\n    <value>")
		b.WriteString(encodeText(val))
		b.WriteString("</value>\n  </data>\n")
	}
	b.WriteString("</root>\n")
	_, err := io.WriteString(w.Output, b.String())
	return err
}

// encodeAttr escapes a string for use as an XML attribute value (double-quoted).
func encodeAttr(s string) string {
	var b strings.Builder
	b.Grow(len(s) + 8)
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

// scratchHeader is the canonical ResX 2.0 prolog + resheader block emitted when
// writing a document from scratch (no original to preserve).
const scratchHeader = `<?xml version="1.0" encoding="utf-8"?>
<root>
  <resheader name="resmimetype">
    <value>text/microsoft-resx</value>
  </resheader>
  <resheader name="version">
    <value>2.0</value>
  </resheader>
  <resheader name="reader">
    <value>System.Resources.ResXResourceReader, System.Windows.Forms, Version=4.0.0.0, Culture=neutral, PublicKeyToken=b77a5c561934e089</value>
  </resheader>
  <resheader name="writer">
    <value>System.Resources.ResXResourceWriter, System.Windows.Forms, Version=4.0.0.0, Culture=neutral, PublicKeyToken=b77a5c561934e089</value>
  </resheader>
`
