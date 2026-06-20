package txml

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

// Writer implements DataFormatWriter for Wordfast Pro TXML files.
//
// Two output paths:
//
//   - Skeleton-store mode (when SetSkeletonStore was called): the
//     reader recorded byte positions for every <source>/<target>
//     content region; we splice translations back into the original
//     bytes for byte-exact roundtrips.
//   - Direct mode: synthesize a fresh document from the streamed
//     parts. Used by tools that produce TXML from non-TXML inputs.
type Writer struct {
	format.BaseFormatWriter
	cfg           *Config
	skeletonStore *format.SkeletonStore
	sourceLocale  string
	targetLocale  string
	blocks        []*model.Block
}

// Ensure Writer implements SkeletonStoreConsumer.
var _ format.SkeletonStoreConsumer = (*Writer)(nil)

// NewWriter creates a new TXML writer.
func NewWriter() *Writer {
	cfg := &Config{}
	cfg.Reset()
	return &Writer{
		BaseFormatWriter: format.BaseFormatWriter{
			FormatName:  "txml",
			Interchange: true,
		},
		cfg: cfg,
	}
}

// Config returns the writer configuration for external modification.
func (w *Writer) Config() *Config { return w.cfg }

// SetSkeletonStore sets the skeleton store for byte-exact output.
func (w *Writer) SetSkeletonStore(store *format.SkeletonStore) {
	w.skeletonStore = store
}

// Write consumes Parts from a channel and writes reconstructed TXML.
func (w *Writer) Write(ctx context.Context, parts <-chan *model.Part) error {
	if w.skeletonStore != nil {
		// Collect all parts, then write from skeleton.
		for {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case part, ok := <-parts:
				if !ok {
					return w.writeFromSkeleton()
				}
				if part.Type == model.PartBlock {
					if block, ok := part.Resource.(*model.Block); ok {
						w.blocks = append(w.blocks, block)
					}
				}
			}
		}
	}

	headerWritten := false
	translatableOpen := false

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case part, ok := <-parts:
			if !ok {
				if translatableOpen {
					if _, err := io.WriteString(w.Output, "</translatable>\n"); err != nil {
						return err
					}
				}
				if headerWritten {
					if _, err := io.WriteString(w.Output, "</txml>\n"); err != nil {
						return err
					}
				}
				return nil
			}
			if part.Type == model.PartLayerStart {
				layer, ok := part.Resource.(*model.Layer)
				if !ok {
					continue
				}
				w.sourceLocale = string(layer.Locale)
				if tl, ok := layer.Properties["target-locale"]; ok {
					w.targetLocale = tl
				}
				if !headerWritten {
					if err := w.writeHeader(); err != nil {
						return err
					}
					headerWritten = true
				}
				continue
			}
			if !headerWritten {
				if err := w.writeHeader(); err != nil {
					return err
				}
				headerWritten = true
			}
			if part.Type == model.PartBlock {
				if translatableOpen {
					if _, err := io.WriteString(w.Output, "</translatable>\n"); err != nil {
						return err
					}
				}
				if err := w.writeBlock(part); err != nil {
					return err
				}
				translatableOpen = true
			}
		}
	}
}

// writeFromSkeleton reads skeleton entries and fills in block content.
// Refs are of the form "<blockIdx>:<segIdx>:<source|target>".
func (w *Writer) writeFromSkeleton() error {
	if err := w.skeletonStore.Flush(); err != nil {
		return fmt.Errorf("txml writer: flush skeleton: %w", err)
	}

	for {
		entry, err := w.skeletonStore.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("txml writer: read skeleton: %w", err)
		}
		switch entry.Type {
		case format.SkeletonText:
			if _, err := w.Output.Write(entry.Data); err != nil {
				return err
			}
		case format.SkeletonRef:
			refID := string(entry.Data)
			// lookupRef returns content already serialized as TXML
			// inner XML (text escaped, <ut> inline codes reconstructed),
			// so it must be written verbatim — re-escaping here would
			// double-escape entities and destroy the <ut> markup.
			xml, ok := w.lookupRef(refID)
			if !ok {
				continue
			}
			if _, err := io.WriteString(w.Output, xml); err != nil {
				return err
			}
		}
	}
	return nil
}

// lookupRef resolves a "blockIdx:segIdx:elemType" ref to the TXML
// inner XML that should be spliced back into the document at that
// position. The returned string is already escaped and carries any
// reconstructed <ut> inline codes, so the caller writes it verbatim.
func (w *Writer) lookupRef(refID string) (string, bool) {
	first, rest, ok := strings.Cut(refID, ":")
	if !ok {
		return "", false
	}
	blockIdx, err := strconv.Atoi(first)
	if err != nil || blockIdx < 0 || blockIdx >= len(w.blocks) {
		return "", false
	}
	second, elemType, ok := strings.Cut(rest, ":")
	if !ok {
		return "", false
	}
	segIdx, err := strconv.Atoi(second)
	if err != nil || segIdx < 0 {
		return "", false
	}
	block := w.blocks[blockIdx]

	switch elemType {
	case "source":
		if segIdx >= block.SourceSegmentCount() {
			return "", false
		}
		return renderTXMLInline(block.SourceSegmentRuns(segIdx)), true
	case "target":
		// Replacing the content of an existing <target> element. Try the
		// recorded targetlocale first; fall back to the writer's
		// configured locale, then any available target.
		if inner, ok := w.targetInnerXML(block, segIdx); ok {
			return inner, true
		}
		// No target available — preserve the source as a fallback so
		// the document stays well-formed.
		if segIdx < block.SourceSegmentCount() {
			return renderTXMLInline(block.SourceSegmentRuns(segIdx)), true
		}
		return "", true
	case "target-insert":
		// Zero-width splice point for a segment that had no original
		// <target>. Emit a full <target>…</target> only when the block
		// carries a translated target segment for this index, matching
		// Okapi's TXMLSkeletonWriter (which writes <target> iff
		// trgSeg != null for the output locale —
		// TXMLSkeletonWriter.java:167-176). When there is no translation
		// we emit nothing, preserving the source-only segment verbatim
		// and honoring allowEmptyOutputTarget=true (no empty <target/>).
		if inner, ok := w.targetInnerXML(block, segIdx); ok {
			return "<target>" + inner + "</target>", true
		}
		return "", true
	}
	return "", false
}

// targetInnerXML resolves the translated target segment for the given
// segment index, returning its rendered TXML inner XML. It prefers the
// recorded targetlocale, then the writer's configured locale, then any
// available target locale. Returns ok=false when the block carries no
// target segment for this index in any locale.
//
// The Block holds one flat target Run sequence per locale plus a target
// segmentation overlay; segIdx indexes the overlay's spans (a dense
// list — only segments that carried a <target> appear), matching the
// reader's per-segment target spans.
func (w *Writer) targetInnerXML(block *model.Block, segIdx int) (string, bool) {
	if inner, ok := targetSegmentXML(block, model.LocaleID(w.targetLocale), segIdx); ok {
		return inner, true
	}
	if inner, ok := targetSegmentXML(block, w.Locale, segIdx); ok {
		return inner, true
	}
	for _, locale := range block.TargetLocales() {
		if inner, ok := targetSegmentXML(block, locale, segIdx); ok {
			return inner, true
		}
	}
	return "", false
}

// targetSegmentXML returns the rendered TXML inner XML of the idx-th
// target segment for a locale, splitting the locale's target runs by
// its target segmentation overlay. With no overlay the whole target is
// one segment. Returns ok=false when the locale has no target or the
// index is out of range.
func targetSegmentXML(block *model.Block, locale model.LocaleID, idx int) (string, bool) {
	if locale.IsEmpty() || !block.HasTarget(locale) {
		return "", false
	}
	runs := block.TargetRuns(locale)
	key := model.Variant(locale)
	ov := block.SegmentationFor(&key)
	if ov == nil {
		if idx == 0 {
			return renderTXMLInline(runs), true
		}
		return "", false
	}
	if idx < 0 || idx >= len(ov.Spans) {
		return "", false
	}
	return renderTXMLInline(ov.Spans[idx].Range.ExtractRuns(runs)), true
}

func (w *Writer) writeHeader() error {
	if _, err := io.WriteString(w.Output, `<?xml version="1.0" encoding="utf-8"?>`+"\n"); err != nil {
		return err
	}
	sourceLocale := w.sourceLocale
	if sourceLocale == "" {
		sourceLocale = "en-US"
	}
	targetLocale := w.targetLocale
	if targetLocale == "" && !w.Locale.IsEmpty() {
		targetLocale = string(w.Locale)
	}
	if _, err := fmt.Fprintf(w.Output, `<txml locale="%s" targetlocale="%s" version="1.0" datatype="xml">`+"\n",
		xmlEscape(sourceLocale), xmlEscape(targetLocale)); err != nil {
		return err
	}
	return nil
}

func (w *Writer) writeBlock(part *model.Part) error {
	block, ok := part.Resource.(*model.Block)
	if !ok {
		return errors.New("txml writer: expected Block resource")
	}

	blockID := block.ID
	if blockID == "" {
		blockID = "tu1"
	}
	datatype := block.Properties["datatype"]
	if datatype == "" {
		datatype = "xml"
	}

	if _, err := fmt.Fprintf(w.Output, "<translatable blockId=\"%s\" datatype=\"%s\">\n",
		xmlEscape(blockID), xmlEscape(datatype)); err != nil {
		return err
	}

	targetLocale := model.LocaleID(w.targetLocale)
	if targetLocale.IsEmpty() {
		targetLocale = w.Locale
	}

	// Walk source segments via the segmentation overlay; pair each with
	// its same-indexed target segment (dense overlay span) when one
	// exists.
	srcSeg := block.SourceSegmentation()
	srcCount := block.SourceSegmentCount()
	var (
		trgRuns []model.Run
		trgOv   *model.Overlay
	)
	if !targetLocale.IsEmpty() {
		trgRuns = block.TargetRuns(targetLocale)
		key := model.Variant(targetLocale)
		trgOv = block.SegmentationFor(&key)
	}

	for i := range srcCount {
		segID := fmt.Sprintf("s%d", i+1)
		if srcSeg != nil && i < len(srcSeg.Spans) && srcSeg.Spans[i].ID != "" {
			segID = srcSeg.Spans[i].ID
		}
		if _, err := fmt.Fprintf(w.Output, "<segment segmentId=\"%s\">", xmlEscape(segID)); err != nil {
			return err
		}
		// renderTXMLInline already escapes text and reconstructs <ut>
		// inline codes, so the result is written verbatim.
		sourceXML := renderTXMLInline(block.SourceSegmentRuns(i))
		if _, err := fmt.Fprintf(w.Output, "<source>%s</source>", sourceXML); err != nil {
			return err
		}
		var targetXML string
		hasTarget := false
		if segRuns, ok := directTargetSegmentRuns(trgRuns, trgOv, i); ok {
			targetXML = renderTXMLInline(segRuns)
			hasTarget = targetXML != ""
		}
		if hasTarget {
			if _, err := fmt.Fprintf(w.Output, "<target>%s</target>", targetXML); err != nil {
				return err
			}
		} else if w.cfg.AllowEmptyOutputTarget {
			if _, err := io.WriteString(w.Output, "<target/>"); err != nil {
				return err
			}
		}
		if _, err := io.WriteString(w.Output, "</segment>\n"); err != nil {
			return err
		}
	}
	return nil
}

// directTargetSegmentRuns returns the runs of the idx-th target segment
// for the direct (non-skeleton) writer path. With a target
// segmentation overlay idx indexes the (dense) overlay spans; without
// one the whole target is one segment (idx 0). Returns ok=false when
// there is no target segment for the index.
func directTargetSegmentRuns(runs []model.Run, ov *model.Overlay, idx int) ([]model.Run, bool) {
	if len(runs) == 0 {
		return nil, false
	}
	if ov == nil {
		if idx == 0 {
			return runs, true
		}
		return nil, false
	}
	if idx < 0 || idx >= len(ov.Spans) {
		return nil, false
	}
	return ov.Spans[idx].Range.ExtractRuns(runs), true
}

// renderTXMLInline serializes a Run sequence to TXML inner-XML form
// for splicing inside a <source> or <target> element. TextRun content
// is XML-escaped; PlaceholderRun (inline-code) runs are reconstructed
// as <ut x="..." type="...">escaped-data</ut> elements, mirroring how
// Wordfast Pro and Okapi's TXMLFilter store inline markup.
//
// The reader stores the *inner* text of each <ut> in PlaceholderRun.Data
// (entity-decoded by the XML parser). Re-emitting the <ut> wrapper here
// is what lets inline codes survive a read → write → read cycle; without
// it the codes would collapse into plain (re-escaped) text on rewrite —
// see TestRoundTripPreservesInlineCodes.
func renderTXMLInline(runs []model.Run) string {
	var b strings.Builder
	for _, r := range runs {
		switch {
		case r.Text != nil:
			b.WriteString(xmlEscape(r.Text.Text))
		case r.Ph != nil:
			b.WriteString("<ut")
			if r.Ph.ID != "" {
				b.WriteString(` x="`)
				b.WriteString(xmlEscape(r.Ph.ID))
				b.WriteString(`"`)
			}
			if r.Ph.Type != "" {
				b.WriteString(` type="`)
				b.WriteString(xmlEscape(r.Ph.Type))
				b.WriteString(`"`)
			}
			b.WriteString(">")
			b.WriteString(xmlEscape(r.Ph.Data))
			b.WriteString("</ut>")
		default:
			// Other run kinds (paired codes, plural/select, sub) are not
			// produced by the TXML reader; fall back to the generic
			// data-preserving rendering, escaped, to stay well-formed.
			b.WriteString(xmlEscape(model.RenderRunsWithData([]model.Run{r})))
		}
	}
	return b.String()
}

// xmlEscape escapes XML special characters.
func xmlEscape(s string) string {
	var buf []byte
	for i := range len(s) {
		switch s[i] {
		case '&':
			buf = append(buf, []byte("&amp;")...)
		case '<':
			buf = append(buf, []byte("&lt;")...)
		case '>':
			buf = append(buf, []byte("&gt;")...)
		case '"':
			buf = append(buf, []byte("&quot;")...)
		default:
			buf = append(buf, s[i])
		}
	}
	return string(buf)
}
