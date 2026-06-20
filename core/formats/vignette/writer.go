package vignette

import (
	"context"
	"errors"
	"fmt"
	"html"
	"io"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
)

// Writer implements DataFormatWriter for Vignette CMS export/import XML.
//
// The writer requires a SkeletonStore to round-trip the original XML
// envelope. Without a skeleton (legacy single-pass mode) it falls back
// to writing the extracted block payloads alone — useful for tests and
// debugging but not a faithful CMS file.
type Writer struct {
	format.BaseFormatWriter
	skeletonStore *format.SkeletonStore
	useCDATA      bool
}

// Ensure Writer implements SkeletonStoreConsumer.
var _ format.SkeletonStoreConsumer = (*Writer)(nil)

// NewWriter creates a new Vignette CMS XML writer.
func NewWriter() *Writer {
	return &Writer{
		BaseFormatWriter: format.BaseFormatWriter{
			FormatName: "vignette",
		},
		useCDATA: true,
	}
}

// SetSkeletonStore sets the skeleton store for byte-exact output.
func (w *Writer) SetSkeletonStore(store *format.SkeletonStore) {
	w.skeletonStore = store
}

// SetUseCDATA flips CDATA wrapping on or off (default: on, mirroring
// the upstream `useCDATA` parameter).
func (w *Writer) SetUseCDATA(b bool) { w.useCDATA = b }

// Write consumes Parts from a channel and writes reconstructed XML.
func (w *Writer) Write(ctx context.Context, parts <-chan *model.Part) error {
	if w.skeletonStore != nil {
		return w.writeWithSkeleton(ctx, parts)
	}
	return w.writeFallback(ctx, parts)
}

// writeWithSkeleton collects all blocks, then reconstructs output from skeleton entries.
func (w *Writer) writeWithSkeleton(ctx context.Context, parts <-chan *model.Part) error {
	blocksByID := make(map[string]*model.Block)

	collect := func() error {
		for {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case part, ok := <-parts:
				if !ok {
					return nil
				}
				if part == nil {
					continue
				}
				if part.Type == model.PartBlock {
					if block, ok := part.Resource.(*model.Block); ok {
						blocksByID[block.ID] = block
					}
				}
			}
		}
	}
	if err := collect(); err != nil {
		return err
	}

	if err := w.skeletonStore.Flush(); err != nil {
		return fmt.Errorf("vignette writer: flush skeleton: %w", err)
	}
	return w.writeFromSkeleton(blocksByID)
}

// writeFromSkeleton reads skeleton entries and fills in block content.
func (w *Writer) writeFromSkeleton(blocks map[string]*model.Block) error {
	for {
		entry, err := w.skeletonStore.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("vignette writer: read skeleton: %w", err)
		}
		switch entry.Type {
		case format.SkeletonText:
			if _, err := w.Output.Write(entry.Data); err != nil {
				return err
			}
		case format.SkeletonRef:
			block, ok := blocks[string(entry.Data)]
			if !ok {
				continue
			}
			payload := w.payloadFor(block)
			if _, err := io.WriteString(w.Output, payload); err != nil {
				return err
			}
		}
	}
	return nil
}

// payloadFor returns the block's text encoded for the destination
// `<valueString>` / `<valueCLOB>` element. The reader records the
// element kind and sub-filter on the block's Properties; the writer
// re-encodes accordingly:
//
//   - okf_html: the source text was decoded HTML; re-wrap in `<p>` if
//     the reader stripped one (`wrappedP=true`), otherwise emit verbatim.
//     The CLOB body is wrapped in CDATA when useCDATA is on.
//   - default / non-CLOB: write text with XML entities escaped so the
//     output remains well-formed.
//
// The CDATA wrap toggle is honored only for valueCLOB content (matching
// the upstream `useCDATA` parameter).
func (w *Writer) payloadFor(block *model.Block) string {
	// Non-translatable content blocks (surfaced for ingestion from skipped,
	// non-source-locale instances) carry the literal source bytes of their
	// payload region. Write them back verbatim — no re-encoding — so the
	// skeleton ref round-trips byte-for-byte exactly as the prior skeleton
	// text did.
	if block.Properties["rawVerbatim"] == "true" {
		return block.SourceText()
	}

	text := block.SourceText()
	if !w.Locale.IsEmpty() && block.HasTarget(w.Locale) {
		text = block.TargetText(w.Locale)
	}
	subFilter := block.Properties["subfilter"]
	valueElem := block.Properties["valueElement"]

	switch subFilter {
	case "okf_html":
		htmlBody := text
		if block.Properties["wrappedP"] == "true" {
			htmlBody = "<p>" + text + "</p>"
		}
		if valueElem == "valueCLOB" && w.useCDATA {
			return "<![CDATA[" + htmlBody + "]]>"
		}
		return xmlEscape(htmlBody)
	default:
		if valueElem == "valueCLOB" && w.useCDATA {
			return "<![CDATA[" + text + "]]>"
		}
		return xmlEscape(text)
	}
}

// xmlEscape escapes the five XML special characters.
func xmlEscape(s string) string {
	// html.EscapeString covers <, >, & and quotes — fine for embedding
	// in attribute / element character data.
	return html.EscapeString(s)
}

// writeFallback writes only block payloads (one per line) when no
// skeleton is configured. This is intentionally lossy — callers that
// need round-trip fidelity must wire a SkeletonStore.
func (w *Writer) writeFallback(ctx context.Context, parts <-chan *model.Part) error {
	first := true
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case part, ok := <-parts:
			if !ok {
				return nil
			}
			if part == nil || part.Type != model.PartBlock {
				continue
			}
			block, ok := part.Resource.(*model.Block)
			if !ok {
				continue
			}
			text := block.SourceText()
			if !w.Locale.IsEmpty() && block.HasTarget(w.Locale) {
				text = block.TargetText(w.Locale)
			}
			if !first {
				if _, err := fmt.Fprint(w.Output, "\n"); err != nil {
					return err
				}
			}
			first = false
			if _, err := fmt.Fprint(w.Output, text); err != nil {
				return err
			}
		}
	}
}
