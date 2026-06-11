package xliff

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"golang.org/x/text/encoding"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
)

// Writer implements DataFormatWriter for XLIFF 1.2 files.
type Writer struct {
	format.BaseFormatWriter
	cfg           *Config
	skeletonStore *format.SkeletonStore
	parts         []*model.Part
	blocks        []*model.Block
	sourceLang    model.LocaleID
	targetLang    model.LocaleID
	fileName      string
}

// Ensure Writer implements SkeletonStoreConsumer.
var _ format.SkeletonStoreConsumer = (*Writer)(nil)

// NewWriter creates a new XLIFF 1.2 writer with default config.
func NewWriter() *Writer {
	cfg := &Config{}
	cfg.Reset()
	return &Writer{
		BaseFormatWriter: format.BaseFormatWriter{
			FormatName: "xliff",
		},
		cfg: cfg,
	}
}

// SetConfig replaces the writer's config — used to enable
// OkapiCompatConfig flags from parity tests.
func (w *Writer) SetConfig(cfg *Config) {
	if cfg != nil {
		w.cfg = cfg
	}
}

// WriterConfig implements format.WriterConfigurable, exposing the
// writer's xliff Config so the parity harness (and CLI introspection,
// recipe loading) can apply OkapiCompatConfig flags via ApplyMap.
func (w *Writer) WriterConfig() format.DataFormatConfig {
	if w.cfg == nil {
		w.cfg = &Config{}
		w.cfg.Reset()
	}
	return w.cfg
}

// okapiCompat returns the writer's OkapiCompatConfig (zero value if
// the writer was constructed without a config).
func (w *Writer) okapiCompat() OkapiCompatConfig {
	if w.cfg == nil {
		return OkapiCompatConfig{}
	}
	return w.cfg.OkapiCompat
}

// transUnitsWithoutSourceTarget returns a position-indexed bitmask:
// out[i] is true when the i-th trans-unit (in document order, matching
// w.blocks order) had no `<target>` element in the source. Indexing by
// position rather than by ID is required because XLIFF allows duplicate
// trans-unit ids across the document (SF-12-Test03 has two trans-units
// with id="1"), and the strip rule must apply per-occurrence based on
// each trans-unit's own source target presence — not on whether ANY
// trans-unit with that id had a target.
//
// Used by the StripApprovedWhenNoSourceTarget post-process. The reader
// sets the `xliff:target-attrs` annotation only when a `<target>` was
// present; absence of the annotation signals "strip approved here".
func (w *Writer) transUnitsWithoutSourceTarget() []bool {
	out := make([]bool, 0, len(w.blocks))
	for _, block := range w.blocks {
		if block == nil {
			out = append(out, false)
			continue
		}
		if _, ok := block.Anno("xliff:target-attrs"); !ok {
			out = append(out, true)
		} else {
			out = append(out, false)
		}
	}
	return out
}

// transUnitsWithDivergentSegSource returns a position-indexed bitmask:
// out[i] is true when the i-th trans-unit (in document order, matching
// w.blocks order) carries the `xliff:divergent-segsource` annotation
// the reader sets when it dropped a `<seg-source>` whose content
// disagreed with `<source>`. Used by the seg-source unwrap post-process
// to drop the literal seg-source bytes that still come through from the
// skeleton.
func (w *Writer) transUnitsWithDivergentSegSource() []bool {
	out := make([]bool, 0, len(w.blocks))
	for _, block := range w.blocks {
		if block == nil {
			out = append(out, false)
			continue
		}
		_, ok := block.Anno("xliff:divergent-segsource")
		out = append(out, ok)
	}
	return out
}

// encoderForOkapiCompat returns the `golang.org/x/text` Encoder the
// writer should use to drive okapi-compat encoding-conditional entity
// escaping, or nil to disable escaping. Returns non-nil when both the
// EscapeBeyondLatin1AsEntities flag is on AND the source declared a
// non-UTF-8 encoding (recorded by the reader in the layer's
// `xliff:source-encoding` property).
//
// Mirrors okapi XMLEncoder.setOptions (XMLEncoder.java:101-110): the
// encoder is only constructed when output encoding is non-UTF-8/16,
// and the encoder's canEncode determines per-char whether to escape.
func (w *Writer) encoderForOkapiCompat() *encoding.Encoder {
	if !w.okapiCompat().EscapeBeyondLatin1AsEntities {
		return nil
	}
	for _, part := range w.parts {
		if part.Type != model.PartLayerStart {
			continue
		}
		layer, ok := part.Resource.(*model.Layer)
		if !ok || layer == nil {
			continue
		}
		if name, ok := layer.Properties["xliff:source-encoding"]; ok {
			return encoderForCharset(name)
		}
	}
	return nil
}

// SetSkeletonStore sets the skeleton store for byte-exact output.
func (w *Writer) SetSkeletonStore(store *format.SkeletonStore) {
	w.skeletonStore = store
}

// Write consumes Parts from a channel and writes XLIFF 1.2 output.
func (w *Writer) Write(ctx context.Context, parts <-chan *model.Part) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case part, ok := <-parts:
			if !ok {
				if w.skeletonStore != nil {
					return w.writeFromSkeleton()
				}
				return w.flush()
			}
			w.parts = append(w.parts, part)
			if part.Type == model.PartBlock {
				if block, ok := part.Resource.(*model.Block); ok {
					w.blocks = append(w.blocks, block)
				}
			}
			if part.Type == model.PartLayerStart {
				if layer, ok := part.Resource.(*model.Layer); ok {
					w.sourceLang = layer.Locale
					w.fileName = layer.Name
					if tl, ok := layer.Properties["target-language"]; ok {
						w.targetLang = model.LocaleID(tl)
					}
				}
			}
		}
	}
}

// writeFromSkeleton reads skeleton entries and fills in block content.
// The return value is named so the deferred post-process flush can
// surface a failed terminal Write (e.g. disk full / broken pipe)
// instead of dropping it and reporting success on a truncated file.
func (w *Writer) writeFromSkeleton() (retErr error) {
	if err := w.skeletonStore.Flush(); err != nil {
		return fmt.Errorf("xliff writer: flush skeleton: %w", err)
	}

	targetLang := w.targetLang
	if !w.Locale.IsEmpty() {
		targetLang = w.Locale
	}

	// injectLang is what we write into synthesized <target xml:lang="…">
	// and into the <file target-language="…"> patch. okapi keeps the
	// file's existing target-language verbatim on round-trip — even when
	// the runtime target differs — so prefer the file's target-language
	// when present and fall back to the writer locale otherwise.
	injectLang := w.targetLang
	if injectLang.IsEmpty() {
		injectLang = targetLang
	}
	if w.okapiCompat().LowercaseLangSubtag {
		injectLang = model.LocaleID(canonicalBCP47(string(injectLang)))
	}

	// If any okapi-compat post-processing flag is on, buffer the whole
	// output so we can rewrite it before flushing. Otherwise write
	// straight through.
	compat := w.okapiCompat()
	needsPostProcess := compat.HoistAltTransNotes ||
		compat.ReorderHeaderToolToEnd ||
		compat.UnwrapSingleSegMrk ||
		compat.StripApprovedWhenNoSourceTarget ||
		compat.StripAltTransSegSource
	finalOut := w.Output
	var postBuf *bytes.Buffer
	if needsPostProcess {
		postBuf = &bytes.Buffer{}
		w.Output = postBuf
		defer func() {
			w.Output = finalOut
			rewritten := postBuf.Bytes()
			if compat.UnwrapSingleSegMrk {
				rewritten = unwrapSingleSegMrkWhenSourceDiffers(rewritten, w.transUnitsWithDivergentSegSource())
			}
			if compat.StripAltTransSegSource {
				rewritten = stripAltTransSegSource(rewritten)
			}
			if compat.HoistAltTransNotes {
				rewritten = hoistAltTransNotes(rewritten)
			}
			if compat.ReorderHeaderToolToEnd {
				rewritten = reorderHeaderToolToEnd(rewritten)
			}
			if compat.StripApprovedWhenNoSourceTarget {
				rewritten = stripApprovedFromTransUnits(rewritten, w.transUnitsWithoutSourceTarget())
			}
			// Surface a failed terminal write. Don't clobber an earlier
			// error returned by the body; only set retErr when it's nil.
			if _, err := finalOut.Write(rewritten); err != nil && retErr == nil {
				retErr = fmt.Errorf("xliff writer: flush output: %w", err)
			}
		}()
	}

	// Wrap output to inject `target-language="..."` into the first
	// `<file ...>` start tag if the source didn't have one. okapi's
	// xliff writer emits target-language regardless of source presence,
	// so this keeps native canonical-equal on round-trip.
	out := newFileTagInjector(w.Output, string(injectLang))

	for {
		entry, err := w.skeletonStore.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("xliff writer: read skeleton: %w", err)
		}
		switch entry.Type {
		case format.SkeletonText:
			data := entry.Data
			compat := w.okapiCompat()
			if compat.StripTransUnitApprovedAttr || compat.StripPhaseDateAttr {
				data = applySkeletonAttrStripping(data, compat)
			}
			if _, err := out.Write(data); err != nil {
				return err
			}
		case format.SkeletonRef:
			refID := string(entry.Data)
			// Ref ID is "blockIdx:elemType"
			idxStr, refSuffix, ok := strings.Cut(refID, ":")
			if !ok {
				continue
			}
			blockIdx, err := strconv.Atoi(idxStr)
			if err != nil || blockIdx < 0 || blockIdx >= len(w.blocks) {
				continue
			}
			block := w.blocks[blockIdx]
			elemType := refSuffix

			var text string
			switch elemType {
			case "source":
				text = w.sourceText(block)
			case "target":
				// elemPos for "target" now spans the FULL element
				// (open tag + content + close tag), not just inner
				// content. Re-emit the complete <target ...>...</target>
				// using stored attrs so empty/self-closing targets
				// (<target state="…" />) get a properly-nested
				// translated body instead of having content land
				// outside the element.
				text = w.fullTargetElement(block, targetLang, injectLang)
			case "target-inject":
				// No <target> element existed in the source; synthesize
				// a complete one. okapi's filter writes a target on
				// round-trip even when the source has only <source>.
				// Only inject when an explicit writer locale is set
				// (translation mode); skip for pure passthrough.
				if w.Locale.IsEmpty() {
					continue
				}
				inj := w.targetText(block, targetLang)
				// okapi emits "<target...>...</target>\n" right before
				// </trans-unit>, so the close tag stays on its own line.
				// The reader places this ref at the offset of `<` in
				// </trans-unit>; appending a trailing newline matches
				// okapi's whitespace exactly. xml:lang uses injectLang
				// (file's target-language preferred over writer locale)
				// to match okapi's "preserve declared target" rule.
				text = fmt.Sprintf("<target xml:lang=\"%s\">%s</target>\n",
					xmlEscapeAttr(string(injectLang)), inj)
				if _, err := io.WriteString(out, text); err != nil {
					return err
				}
				continue
			case "target-inject-seg":
				// Same as target-inject but the trans-unit had a
				// <seg-source> with no sibling <target>; okapi
				// synthesizes a *segmented* target wrapping each
				// segment in <mrk mtype="seg" mid="…"> matching the
				// seg-source mids (XLIFFFilter.java emits this when
				// alignSeg/segmentation produces target=null).
				if w.Locale.IsEmpty() {
					continue
				}
				segs, base := w.pickTargetSegments(block, targetLang)
				if len(segs) == 0 {
					continue
				}
				inj := wrapSegmentsAsMrk(segs, base)
				text = fmt.Sprintf("<target xml:lang=\"%s\">%s</target>\n",
					xmlEscapeAttr(string(injectLang)), inj)
				if _, err := io.WriteString(out, text); err != nil {
					return err
				}
				continue
			}

			// renderXliffRuns already escapes TextRun content and emits
			// inline-code wrappers, so write `text` verbatim here.
			if _, err := io.WriteString(out, text); err != nil {
				return err
			}
		}
	}
	return out.Flush()
}

// fileTagInjector wraps an io.Writer to ensure the first `<file ...>`
// start tag in the stream carries a `target-language="..."` attribute.
// okapi's xliff writer always emits target-language; native preserves
// the source skeleton, so files without that attribute diverge from
// okapi's output on round-trip. The injector buffers bytes only while
// it is inside the opening `<file` tag — once the tag is emitted (or
// confirmed absent at EOF), bytes pass through directly.
type fileTagInjector struct {
	out        io.Writer
	targetLang string
	done       bool     // true once the first <file ...> tag has been processed
	inTag      bool     // currently buffering bytes inside <file ...
	buf        []byte   // pending bytes once inTag
	tail       [10]byte // sliding window of recent bytes (looking for "<file")
	tailLen    int
}

func newFileTagInjector(w io.Writer, targetLang string) *fileTagInjector {
	return &fileTagInjector{out: w, targetLang: targetLang}
}

// fileTagPrefix is the byte signature that triggers buffering.
var fileTagPrefix = []byte("<file")

// Write scans p for the first `<file ` opening tag and buffers from
// there until the closing `>`. When the tag closes, it injects
// target-language if missing, flushes the (possibly modified) tag,
// and disables further inspection.
func (f *fileTagInjector) Write(p []byte) (int, error) {
	if f.done && !f.inTag {
		return f.out.Write(p)
	}
	written := 0
	for i := range p {
		b := p[i]
		if f.inTag {
			f.buf = append(f.buf, b)
			if b == '>' {
				patched := injectTargetLanguage(f.buf, f.targetLang)
				if _, err := f.out.Write(patched); err != nil {
					return written, err
				}
				f.buf = nil
				f.inTag = false
				f.done = true
				written = i + 1
			}
			continue
		}
		if !f.done {
			// Slide the tail window and check for the prefix match.
			if f.tailLen < len(f.tail) {
				f.tail[f.tailLen] = b
				f.tailLen++
			} else {
				copy(f.tail[:], f.tail[1:])
				f.tail[len(f.tail)-1] = b
			}
			if f.tailLen >= len(fileTagPrefix) {
				start := f.tailLen - len(fileTagPrefix)
				next := byte(0)
				if start+len(fileTagPrefix) < f.tailLen {
					next = f.tail[start+len(fileTagPrefix)]
				}
				_ = next
				// Match `<file` followed by whitespace, '>', '/', or ':'.
				if bytes.Equal(f.tail[start:start+len(fileTagPrefix)], fileTagPrefix) {
					sep := byte(0)
					if i+1 < len(p) {
						sep = p[i+1]
					}
					if sep == ' ' || sep == '\t' || sep == '\r' || sep == '\n' || sep == '>' {
						// Flush bytes up to and including current; switch to buffering.
						if _, err := f.out.Write(p[written : i+1]); err != nil {
							return written, err
						}
						written = i + 1
						f.inTag = true
						continue
					}
				}
			}
		}
	}
	if !f.inTag && written < len(p) {
		if _, err := f.out.Write(p[written:]); err != nil {
			return written, err
		}
	}
	return len(p), nil
}

// Flush completes any in-progress buffering. Call this at end of
// stream so a `<file` opening that ends in mid-write doesn't get lost.
func (f *fileTagInjector) Flush() error {
	if f.inTag && len(f.buf) > 0 {
		_, err := f.out.Write(f.buf)
		f.buf = nil
		f.inTag = false
		return err
	}
	return nil
}

// injectTargetLanguage modifies a `<file ...>` start tag so it carries
// `target-language="..."`. If the tag already declares one, returns
// the input unchanged. If targetLang is empty, returns input unchanged.
func injectTargetLanguage(tag []byte, targetLang string) []byte {
	if targetLang == "" {
		return tag
	}
	if bytes.Contains(tag, []byte("target-language=")) {
		return tag
	}
	// Insert before the closing '>' (or '/>' for self-closing).
	closeIdx := len(tag) - 1
	if closeIdx >= 1 && tag[closeIdx] == '>' && tag[closeIdx-1] == '/' {
		closeIdx--
	}
	insert := fmt.Appendf(nil, ` target-language="%s"`, xmlEscapeAttr(targetLang))
	out := make([]byte, 0, len(tag)+len(insert))
	out = append(out, tag[:closeIdx]...)
	out = append(out, insert...)
	out = append(out, tag[closeIdx:]...)
	return out
}

// sourceText renders the block's <source> content. Source is the
// original — tools mutate the target, not the source — so the writer
// emits the source body native IR verbatim, preserving every byte of
// the original <source> element (inline-code attributes, attribute
// order, inter-element whitespace).
//
// Falls back to per-segment concatenation when no body annotation
// exists (synthetic blocks created by tools, for example).
func (w *Writer) sourceText(block *model.Block) string {
	compat := w.okapiCompat()
	opts := renderOpts{
		EncodableAs:     w.encoderForOkapiCompat(),
		StripCREntities: compat.StripCDataCREntities,
	}
	if a, ok := block.Anno("xliff:source-body"); ok {
		if sa, ok := a.(*SourceBodyNativeAnnotation); ok && sa.Content != nil {
			return renderNativeWithRunsOpts(sa.Content, nil, opts)
		}
	}
	return concatSegments(sourceSegViews(block))
}

// renderXliffRuns serializes a Run sequence into xliff 1.2 inline
// markup. TextRun bytes are XML-escaped; PcOpen/PcClose/Ph runs are
// re-wrapped in <bpt>/<ept>/<ph> elements so the round-trip preserves
// the source's inline placeholders.
func renderXliffRuns(runs []model.Run) string {
	var b strings.Builder
	for _, r := range runs {
		switch {
		case r.Text != nil:
			b.WriteString(xmlEscapeText(r.Text.Text))
		case r.Ph != nil:
			b.WriteString(`<ph id="`)
			b.WriteString(xmlEscapeAttr(r.Ph.ID))
			b.WriteString(`"`)
			if r.Ph.Equiv != "" {
				b.WriteString(` equiv-text="`)
				b.WriteString(xmlEscapeAttr(r.Ph.Equiv))
				b.WriteString(`"`)
			}
			if r.Ph.Data != "" {
				b.WriteString(`>`)
				b.WriteString(xmlEscapeText(r.Ph.Data))
				b.WriteString(`</ph>`)
			} else {
				b.WriteString(`/>`)
			}
		case r.PcOpen != nil:
			b.WriteString(`<bpt id="`)
			b.WriteString(xmlEscapeAttr(r.PcOpen.ID))
			b.WriteString(`"`)
			if r.PcOpen.Equiv != "" {
				b.WriteString(` equiv-text="`)
				b.WriteString(xmlEscapeAttr(r.PcOpen.Equiv))
				b.WriteString(`"`)
			}
			b.WriteString(`>`)
			b.WriteString(xmlEscapeText(r.PcOpen.Data))
			b.WriteString(`</bpt>`)
		case r.PcClose != nil:
			b.WriteString(`<ept id="`)
			b.WriteString(xmlEscapeAttr(r.PcClose.ID))
			b.WriteString(`">`)
			b.WriteString(xmlEscapeText(r.PcClose.Data))
			b.WriteString(`</ept>`)
		}
	}
	return b.String()
}

// fullTargetElement renders the complete <target ...>...</target>
// element, using stored source attrs (state, state-qualifier, xml:lang,
// custom-namespace) verbatim. okapi's xliff writer always emits the
// full element on round-trip; this matches that behaviour and
// survives empty/self-closing source targets (where inner-content
// injection would land outside the tag). _ = injectLang signals we
// intentionally don't synthesise an xml:lang the source didn't have —
// okapi preserves source's attribute set verbatim too.
func (w *Writer) fullTargetElement(block *model.Block, targetLang, injectLang model.LocaleID) string {
	_ = injectLang
	inner := w.targetText(block, targetLang)
	var b strings.Builder
	b.WriteString("<target")
	if a, ok := block.Anno("xliff:target-attrs"); ok {
		if ta, ok := a.(*TargetAttrsAnnotation); ok {
			for _, attr := range ta.Attrs {
				b.WriteString(` `)
				if attr.Space != "" {
					b.WriteString(attr.Space)
					b.WriteString(`:`)
				}
				b.WriteString(attr.Local)
				b.WriteString(`="`)
				b.WriteString(xmlEscapeAttr(attr.Value))
				b.WriteString(`"`)
			}
		}
	}
	b.WriteString(`>`)
	b.WriteString(inner)
	b.WriteString(`</target>`)
	return b.String()
}

// targetText returns the text to write for the block's <target> slot.
// Prefers the writer's locale, falling back to any existing target
// (non-matching languages round-trip verbatim) and finally to the
// source body (matches okapi for translate="no" or untranslated
// entries).
//
// When a block-level target body annotation exists, the writer walks
// it to reconstruct mrk segmentation, between-mrk whitespace, and
// inline-code attributes from the source file. Segments map to mrks
// by position; each mrk's text content comes from the corresponding
// target segment's runs.
//
// Falls back to per-segment rendering when no body annotation exists
// (synthetic targets created by tools).
func (w *Writer) targetText(block *model.Block, targetLang model.LocaleID) string {
	tgtSegs, _ := w.pickTargetSegments(block, targetLang)
	if len(tgtSegs) == 0 {
		return ""
	}
	compat := w.okapiCompat()
	opts := renderOpts{
		EncodableAs:     w.encoderForOkapiCompat(),
		StripCREntities: compat.StripCDataCREntities,
	}
	// UnwrapSingleSegMrk is now applied as a writer post-process pass
	// (see unwrapSingleSegMrkWhenSourceDiffers) so it can compare
	// `<source>` vs `<seg-source>` content and only unwrap when they
	// differ — matching XLIFFFilter.java:2278. The IR-level renderer
	// always emits the segmented form here.
	// Pick the IR that will drive structural emission. Normally this is
	// the target body IR (preserves the existing target's inline-code
	// shape). But when the target was originally empty/whitespace the
	// reader stored a near-trivial IR (just text), and pseudo-translate
	// has since populated tgtSegs from the SOURCE — so the runs now
	// carry the source's inline-code structure (PcOpen/PcClose/Ph) that
	// the trivial target IR can't accommodate. In that case fall back to
	// source body IR so the bpt/ept/ph wrappers actually get emitted.
	// MQ-12-Test01 has many such trans-units (`<target> </target>`
	// placeholders around inline-coded source content).
	if a, ok := block.Anno("xliff:target-body"); ok {
		if ta, ok := a.(*TargetBodyNativeAnnotation); ok && ta.Content != nil {
			if !irLacksInlinesNeededByRuns(ta.Content, tgtSegs) {
				return renderBodyWithSegmentsOpts(ta.Content, tgtSegs, opts, false)
			}
		}
	}
	if a, ok := block.Anno("xliff:source-body"); ok {
		if sa, ok := a.(*SourceBodyNativeAnnotation); ok && sa.Content != nil {
			return renderBodyWithSegmentsOpts(sa.Content, tgtSegs, opts, false)
		}
	}
	if blockIsSegmented(block) {
		return wrapSegmentsAsMrk(tgtSegs, sourceSegViews(block))
	}
	return concatSegments(tgtSegs)
}

// pickTargetSegments selects the segment views that should drive the
// <target> output, plus a "base" slice that supplies the structural
// native IR when the chosen segments lack one (typical for pseudo-
// translated targets that were synthesised from the source). Returns
// (nil, nil) when neither an existing target nor a source-derived
// fallback is usable.
func (w *Writer) pickTargetSegments(block *model.Block, targetLang model.LocaleID) ([]segView, []segView) {
	src := sourceSegViews(block)
	if block.HasTarget(targetLang) {
		return targetSegViews(block, targetLang), src
	}
	if loc := anyTargetLocale(block); loc != "" {
		return targetSegViews(block, loc), src
	}
	if len(src) > 0 {
		return src, src
	}
	return nil, nil
}

// concatSegments joins all segments' rendered content with no
// wrappers between them. Used for <source> (always unsegmented) and
// for <target> when the block isn't segmented.
func concatSegments(segs []segView) string {
	var b strings.Builder
	for _, s := range segs {
		b.WriteString(renderSegment(s))
	}
	return b.String()
}

// wrapSegmentsAsMrk emits each segment wrapped in
// <mrk mid="…" mtype="seg">…</mrk>. Used for <target> when the source
// carried <seg-source> — okapi's writer mirrors that segmentation
// onto the target. base supplies the structural native IR when the
// segment slice itself lacks one (pseudo-translated targets are
// text-only; we re-use the source segment's inline structure so
// inline-code attributes survive).
func wrapSegmentsAsMrk(segs, base []segView) string {
	var b strings.Builder
	for i, s := range segs {
		mid := midForSegView(s, i)
		b.WriteString(`<mrk mid="`)
		b.WriteString(xmlEscapeAttr(mid))
		b.WriteString(`" mtype="seg">`)
		b.WriteString(renderSegmentWithBase(s, baseAt(base, i)))
		b.WriteString(`</mrk>`)
	}
	return b.String()
}

// renderSegment renders one segment using its native IR when present,
// falling back to renderXliffRuns or plain text.
func renderSegment(seg segView) string {
	return renderSegmentWithBase(seg, segView{})
}

// renderSegmentWithBase renders a segment, falling back to base's
// native IR if seg has none. This lets a pseudo-translated target
// segment (text-only) inherit the source segment's inline structure.
func renderSegmentWithBase(seg, base segView) string {
	if seg.Native != nil {
		return renderNativeWithRuns(seg.Native, seg.Runs)
	}
	if base.Native != nil {
		return renderNativeWithRuns(base.Native, seg.Runs)
	}
	if len(seg.Runs) > 0 {
		return renderXliffRuns(seg.Runs)
	}
	return seg.Text()
}

// baseAt returns the i-th segment view from base, or the zero segView
// if out of range.
func baseAt(base []segView, i int) segView {
	if i < 0 || i >= len(base) {
		return segView{}
	}
	return base[i]
}

func (w *Writer) flush() (retErr error) {
	if w.Output == nil {
		return nil
	}

	targetLang := w.targetLang
	if !w.Locale.IsEmpty() {
		targetLang = w.Locale
	}

	// ew tracks the first write error; once set, all subsequent
	// Fprint/Fprintf calls are no-ops so a failed terminal write
	// (disk full / broken pipe) surfaces instead of being swallowed.
	ew := &errWriter{w: w.Output}

	fmt.Fprint(ew, xml.Header)
	fmt.Fprintf(ew, `<xliff version="1.2" xmlns="urn:oasis:names:tc:xliff:document:1.2">`)
	fmt.Fprintf(ew, "\n")

	// Write file envelope
	fmt.Fprintf(ew, `  <file original="%s" source-language="%s"`,
		xmlEscapeAttr(w.fileName), xmlEscapeAttr(string(w.sourceLang)))
	if !targetLang.IsEmpty() {
		fmt.Fprintf(ew, ` target-language="%s"`, xmlEscapeAttr(string(targetLang)))
	}
	fmt.Fprintf(ew, ` datatype="plaintext">`)
	fmt.Fprintf(ew, "\n    <body>\n")

	// Write trans-units from collected blocks
	for _, part := range w.parts {
		if part.Type != model.PartBlock {
			continue
		}
		block, ok := part.Resource.(*model.Block)
		if !ok {
			continue
		}

		fmt.Fprintf(ew, `      <trans-unit id="%s"`, xmlEscapeAttr(block.ID))
		if !block.Translatable {
			fmt.Fprintf(ew, ` translate="no"`)
		}
		if block.PreserveWhitespace {
			fmt.Fprintf(ew, ` xml:space="preserve"`)
		}
		if v, ok := block.Properties["approved"]; ok && v == "yes" {
			fmt.Fprintf(ew, ` approved="yes"`)
		}
		fmt.Fprintf(ew, ">\n")

		// Source
		sourceText := fragmentToXLIFF(block.Source)
		fmt.Fprintf(ew, "        <source>%s</source>\n", sourceText)

		// Target
		if block.HasTarget(targetLang) {
			targetText := fragmentToXLIFF(block.TargetRuns(targetLang))
			fmt.Fprintf(ew, "        <target>%s</target>\n", targetText)
		}

		// Notes
		for _, note := range block.Notes() {
			fmt.Fprintf(ew, "        <note")
			if note.From != "" {
				fmt.Fprintf(ew, ` from="%s"`, xmlEscapeAttr(note.From))
			}
			if note.Priority > 0 {
				fmt.Fprintf(ew, ` priority="%d"`, note.Priority)
			}
			if note.Annotates != "" {
				fmt.Fprintf(ew, ` annotates="%s"`, xmlEscapeAttr(note.Annotates))
			}
			fmt.Fprintf(ew, ">%s</note>\n", xmlEscapeText(note.Text))
		}

		// Alt-trans
		for _, alt := range block.AltTranslations() {
			fmt.Fprintf(ew, "        <alt-trans")
			if alt.CombinedScore > 0 {
				fmt.Fprintf(ew, ` match-quality="%.0f"`, alt.CombinedScore)
			}
			if alt.Origin != "" {
				fmt.Fprintf(ew, ` origin="%s"`, xmlEscapeAttr(alt.Origin))
			}
			fmt.Fprintf(ew, ">\n")
			if len(alt.Source) > 0 {
				fmt.Fprintf(ew, "          <source>%s</source>\n", xmlEscapeText(model.FlattenRuns(alt.Source)))
			}
			if len(alt.Target) > 0 {
				fmt.Fprintf(ew, `          <target xml:lang="%s">%s</target>`+"\n",
					xmlEscapeAttr(string(targetLang)), xmlEscapeText(model.FlattenRuns(alt.Target)))
			}
			fmt.Fprintf(ew, "        </alt-trans>\n")
		}

		fmt.Fprintf(ew, "      </trans-unit>\n")
	}

	fmt.Fprintf(ew, "    </body>\n  </file>\n</xliff>")
	if ew.err != nil {
		retErr = fmt.Errorf("xliff writer: flush output: %w", ew.err)
	}
	return retErr
}

// errWriter wraps an io.Writer and records the first error encountered.
// Once an error is sticky, subsequent writes become no-ops so callers
// can drive a sequence of fmt.Fprintf calls and check ew.err once at
// the end without each call swallowing its own write error.
type errWriter struct {
	w   io.Writer
	err error
}

func (e *errWriter) Write(p []byte) (int, error) {
	if e.err != nil {
		return 0, e.err
	}
	n, err := e.w.Write(p)
	if err != nil {
		e.err = err
	}
	return n, err
}

// fragmentToXLIFF converts a Run sequence to XLIFF inline content. This
// is the legacy non-skeleton flush path; it emits only plain text
// (TextRun content), which is sufficient for the synthetic blocks that
// reach it.
func fragmentToXLIFF(runs []model.Run) string {
	if len(runs) == 0 {
		return ""
	}
	return xmlEscapeText(model.RunsText(runs))
}
