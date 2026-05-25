package ts

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
)

// Writer implements DataFormatWriter for Qt TS files.
type Writer struct {
	format.BaseFormatWriter
	skeletonStore *format.SkeletonStore
	headerProps   map[string]string
	groups        []*contextGroup
	currentCtx    *contextGroup
	allBlocks     []*model.Block // all blocks in order, for skeleton lookup
}

// Ensure Writer implements SkeletonStoreConsumer.
var _ format.SkeletonStoreConsumer = (*Writer)(nil)

// contextGroup holds blocks for one <context> element.
type contextGroup struct {
	name   string
	blocks []*model.Block
}

// NewWriter creates a new Qt TS writer.
func NewWriter() *Writer {
	return &Writer{
		BaseFormatWriter: format.BaseFormatWriter{
			FormatName: "ts",
		},
		headerProps: make(map[string]string),
	}
}

// SetSkeletonStore sets the skeleton store for byte-exact output.
func (w *Writer) SetSkeletonStore(store *format.SkeletonStore) {
	w.skeletonStore = store
}

// Write consumes Parts from a channel and writes Qt TS XML.
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
			w.collectPart(part)
		}
	}
}

func (w *Writer) collectPart(part *model.Part) {
	switch part.Type {
	case model.PartData:
		if data, ok := part.Resource.(*model.Data); ok {
			if data.Name == "ts-header" {
				w.headerProps = data.Properties
			}
		}
	case model.PartGroupStart:
		if gs, ok := part.Resource.(*model.GroupStart); ok {
			w.currentCtx = &contextGroup{name: gs.Name}
			w.groups = append(w.groups, w.currentCtx)
		}
	case model.PartGroupEnd:
		w.currentCtx = nil
	case model.PartBlock:
		if block, ok := part.Resource.(*model.Block); ok {
			w.allBlocks = append(w.allBlocks, block)
			if w.currentCtx != nil {
				w.currentCtx.blocks = append(w.currentCtx.blocks, block)
			} else {
				// Block without a context group -- create an implicit one
				ctxName := block.Properties["context"]
				if ctxName == "" {
					ctxName = "default"
				}
				// Find or create context
				var grp *contextGroup
				for _, g := range w.groups {
					if g.name == ctxName {
						grp = g
						break
					}
				}
				if grp == nil {
					grp = &contextGroup{name: ctxName}
					w.groups = append(w.groups, grp)
				}
				grp.blocks = append(grp.blocks, block)
			}
		}
	}
}

// writeFromSkeleton reads skeleton entries and fills in block content.
func (w *Writer) writeFromSkeleton() error {
	if err := w.skeletonStore.Flush(); err != nil {
		return fmt.Errorf("ts writer: flush skeleton: %w", err)
	}

	// Determine target locale
	language := w.headerProps["language"]
	targetLocale := model.LocaleID(language)
	if w.Locale != "" {
		targetLocale = w.Locale
	}

	firstText := true
	// pendingText buffers the most recent SkeletonText chunk so we can
	// rewrite the trailing `<translation …>` opening tag right before
	// the matching translation ref reaches the output. Okapi's
	// TsFilter renders the `type` attribute via an APPROVED-property
	// placeholder that flips to "unfinished" whenever the translation
	// content has been modified by a downstream step (TextModificationStep,
	// pseudo, MT, …). The native pipeline always rewrites the target
	// content via the ref, so we always need to force the `type` flip.
	var pendingText []byte
	flushPending := func() error {
		if len(pendingText) > 0 {
			if _, err := w.Output.Write(pendingText); err != nil {
				return err
			}
			pendingText = pendingText[:0]
		}
		return nil
	}

	for {
		entry, err := w.skeletonStore.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("ts writer: read skeleton: %w", err)
		}
		switch entry.Type {
		case format.SkeletonText:
			if err := flushPending(); err != nil {
				return err
			}
			data := entry.Data
			if firstText {
				// The first skeleton text chunk contains the document
				// prologue (XML declaration + DOCTYPE + the `<TS …>`
				// element opening). Okapi's TsFilter rewrites this
				// prologue when it serializes back: it force-emits
				// `<?xml version="1.0" encoding="UTF-8"?>` (Woodstox
				// normalises the encoding token to its canonical
				// uppercase form, drops the `standalone` attribute) and
				// emits the DOCTYPE via Woodstox's
				// `dtd.getDocumentTypeDeclaration()` which always
				// renders the internal-subset brackets even when the
				// source had `<!DOCTYPE TS>` with no subset. Mirror
				// both rewrites so the parity round-trip matches the
				// reference engine instead of preserving the source's
				// original prologue verbatim.
				data = normalizeTSPrologue(data)
				firstText = false
			}
			pendingText = append(pendingText[:0], data...)
		case format.SkeletonRef:
			// Ref ID is "blockIdx:elemType" where blockIdx is 0-based
			refID := string(entry.Data)
			idxStr, refSuffix, ok := strings.Cut(refID, ":")
			if !ok {
				continue
			}
			blockIdx, err := strconv.Atoi(idxStr)
			if err != nil || blockIdx < 0 || blockIdx >= len(w.allBlocks) {
				continue
			}
			block := w.allBlocks[blockIdx]
			elemType := refSuffix

			// For translation refs, rewrite the trailing
			// `<translation …>` opening tag in the buffered skeleton
			// text so it carries `type="unfinished"` whenever the
			// new target text differs from the snapshot the reader
			// captured. Okapi's TsFilter renders the type attribute
			// via an APPROVED-property placeholder that flips to
			// "unfinished" the moment a downstream step modifies the
			// translation content. The native equivalent is comparing
			// the rendered text against `_orig_target_text` so an
			// untouched round-trip stays byte-exact while a modified
			// one (pseudo / MT / manual edit) gets the unfinished flag.
			// synthesized_translation already carries the canonical
			// `type="unfinished"` opener in its prefix text, so skip the
			// rewrite. For real translation/numerus refs the pendingText
			// holds the source's `<translation …>` tag and we may need to
			// flip its type when downstream content was modified.
			if (elemType == "translation" || elemType == "numerus_translation") &&
				shouldEmitUnfinished(block) {
				pendingText = forceTranslationUnfinished(pendingText)
			}
			if err := flushPending(); err != nil {
				return err
			}

			var text string
			switch elemType {
			case "source":
				if len(block.Source) > 0 {
					text = w.runsToXML(block.Source)
				}
			case "translation", "synthesized_translation":
				// `synthesized_translation` is the placeholder the reader
				// inserts when okapi's `addTargetSection` would synthesize
				// a `<translation type="unfinished" variants="no">…</translation>`
				// after the last validBefore element. The wrapping markup
				// is already in the surrounding skeleton text — here we
				// just emit the target body, defaulting to the
				// pseudo-translated source when no explicit target was
				// produced (matches okapi's text-modification fill).
				if block.HasTarget(targetLocale) {
					text = w.runsToXML(block.TargetRuns(targetLocale))
				} else if len(block.Targets) > 0 {
					// File declared a target language other than ours
					// (e.g. <TS language="af">). Preserve the existing
					// translation so non-matching round-trips match
					// okapi's "leave bilingual content alone" semantics.
					for _, loc := range block.TargetLocales() {
						if runs := block.TargetRuns(loc); len(runs) > 0 {
							text = w.runsToXML(runs)
							break
						}
					}
				}
				if text == "" && elemType == "synthesized_translation" {
					// No target produced for this block — emit the source
					// verbatim as the translation body. This mirrors
					// `tikal -m` on a fresh extract where
					// TextModificationStep is disabled: the empty target
					// gets filled with the source string before the writer
					// runs.
					if len(block.Source) > 0 {
						text = w.runsToXML(block.Source)
					}
				}
			case "numerus_translation":
				// One <numerusform>…</numerusform> per segment in the
				// target locale; mirrors okapi's TextModificationStep
				// path which pseudo-translates every plural form, not
				// just the first. Each form is preceded by a line break
				// and emitted with its original attribute string (e.g.
				// ` variants="no"`) so the round-trip preserves the
				// `<translation>\n<numerusform variants="no">…</numerusform>\n</translation>`
				// shape okapi's pipeline produces.
				formRuns := w.numerusFormRuns(block, targetLocale)
				var attrs []string
				if joined := block.Properties["_numerusform_attrs"]; joined != "" {
					attrs = strings.Split(joined, "\x1f")
				}
				// `_numerusform_nonempty` carries the originally-empty
				// per-form flags so we can preserve `<numerusform></numerusform>`
				// rather than replace it with a pseudo-translated source
				// copy. Without this an `applyPseudoToBlock` upstream
				// fills the block's target with pseudo(source) and we'd
				// emit that for every form — okapi's per-form TextUnit
				// flow has nothing to modify when both source and target
				// are empty.
				var nonemptyFlags []string
				if raw := block.Properties["_numerusform_nonempty"]; raw != "" {
					nonemptyFlags = strings.Split(raw, ",")
				}
				// `_numerusform_prefixes` are the source's verbatim
				// inter-form character data (typically newline +
				// indentation) so we re-emit `\n            <numerusform>`
				// pairs that match the original layout. Falls back to
				// the document's prevailing line break when prefixes
				// are absent (e.g. for blocks built outside the reader).
				var prefixes []string
				if raw := block.Properties["_numerusform_prefixes"]; raw != "" {
					prefixes = strings.Split(raw, "\x1f")
				}
				trailingWS := block.Properties["_numerusform_trailing_ws"]
				lineBreak := block.Properties["_line_break"]
				if lineBreak == "" {
					lineBreak = "\n"
				}
				// The number of forms to emit comes from the original
				// source's numerusform count (carried as the prefix
				// list length or the nonempty-flag length), NOT from
				// `len(segs)`. applyPseudoToBlock can collapse the
				// segment count when it falls back to source-as-base
				// (one source segment → one pseudo'd target segment),
				// which would otherwise drop the empty forms.
				formCount := len(formRuns)
				if n := len(prefixes); n > formCount {
					formCount = n
				}
				if n := len(nonemptyFlags); n > formCount {
					formCount = n
				}
				if n := len(attrs); n > formCount {
					formCount = n
				}
				var b strings.Builder
				for i := range formCount {
					if i < len(prefixes) {
						b.WriteString(prefixes[i])
					} else {
						b.WriteString(lineBreak)
					}
					b.WriteString("<numerusform")
					if i < len(attrs) {
						b.WriteString(attrs[i])
					}
					b.WriteByte('>')
					if i < len(nonemptyFlags) && nonemptyFlags[i] == "0" {
						// Original form was empty — preserve the empty
						// shape instead of substituting in a
						// pseudo-translated source clone.
					} else if i < len(formRuns) && len(formRuns[i]) > 0 {
						b.WriteString(w.runsToXMLEscapeApos(formRuns[i]))
					}
					b.WriteString("</numerusform>")
				}
				if formCount > 0 {
					if trailingWS != "" {
						b.WriteString(trailingWS)
					} else {
						b.WriteString(lineBreak)
					}
				}
				text = b.String()
			}

			if _, err := io.WriteString(w.Output, text); err != nil {
				return err
			}
		}
	}
	if err := flushPending(); err != nil {
		return err
	}
	return nil
}

// shouldEmitUnfinished reports whether the writer must rewrite the
// `<translation …>` opening tag to carry `type="unfinished"`. The
// rule mirrors okapi's TsFilter:
//
//   - Source already declared `type="unfinished"` → keep flag.
//   - Source had no `type` but the original target content was empty
//     (or whitespace-only) → flip to `type="unfinished"`. This is the
//     `targetIsEmpty()` branch in TsFilter.generate(): an empty
//     `<translation></translation>` becomes `<translation
//     type="unfinished">…</translation>` with the synthesized
//     content (typically the source text or a pseudo-translation
//     of it).
//   - Source had `type` but it was non-empty (approved translation
//     with content) → no flag, even if a downstream step modified
//     the content. Okapi preserves approved=yes through pseudo /
//     TextModificationStep — the only mutation is the empty-target
//     branch above.
//   - Source had `type="obsolete"` → caller doesn't reach this
//     function (obsolete blocks are non-translatable).
func shouldEmitUnfinished(block *model.Block) bool {
	if block.Properties["type"] == "unfinished" {
		return true
	}
	orig, ok := block.Properties["_orig_target_text"]
	if !ok {
		// No snapshot — covers blocks constructed programmatically.
		// Default to unfinished (safer than emitting an approved
		// translation with un-vetted content).
		return true
	}
	return strings.TrimSpace(orig) == ""
}

// forceTranslationUnfinished rewrites the LAST `<translation …>`
// opening tag in buf to carry `type="unfinished"`. If the tag already
// has a `type` attribute (any value other than "obsolete"), replace
// it with `unfinished`. If the tag has `type="obsolete"`, leave it
// alone — obsolete entries pass through verbatim. If the tag has no
// `type` attribute at all, insert `type="unfinished"` immediately
// after `<translation`.
//
// Returns the (possibly rewritten) buffer. If buf doesn't end in a
// `<translation …>` opening tag, returns buf unchanged.
func forceTranslationUnfinished(buf []byte) []byte {
	// Find the last `<translation` in buf followed eventually by `>`.
	// Limit the search to a reasonable suffix so we don't wander into
	// previous messages.
	open := bytes.LastIndex(buf, []byte("<translation"))
	if open < 0 {
		return buf
	}
	// Locate the closing `>` of the opening tag.
	closeIdx := bytes.IndexByte(buf[open:], '>')
	if closeIdx < 0 {
		return buf
	}
	closeIdx += open
	tag := buf[open : closeIdx+1] // includes < and >
	body := tag[len("<translation"):]
	body = body[:len(body)-1] // strip trailing `>`

	// Already has type="…"? Find existing type attribute.
	if i := bytes.Index(body, []byte(" type=")); i >= 0 {
		quoteStart := i + len(" type=")
		if quoteStart >= len(body) {
			return buf
		}
		quote := body[quoteStart]
		if quote != '"' && quote != '\'' {
			return buf
		}
		valEnd := bytes.IndexByte(body[quoteStart+1:], quote)
		if valEnd < 0 {
			return buf
		}
		valEnd += quoteStart + 1
		existing := string(body[quoteStart+1 : valEnd])
		if existing == "obsolete" {
			return buf
		}
		// Replace the existing value with `unfinished`.
		newBody := append([]byte{}, body[:quoteStart+1]...)
		newBody = append(newBody, "unfinished"...)
		newBody = append(newBody, body[valEnd:]...)
		newTag := append([]byte("<translation"), newBody...)
		newTag = append(newTag, '>')
		out := append([]byte{}, buf[:open]...)
		out = append(out, newTag...)
		return out
	}

	// No type attribute — append ` type="unfinished"` AFTER any
	// existing attributes (variants=…, etc.). Okapi's APPROVED-property
	// placeholder is wired into addTargetSection between the existing
	// attributes and the closing `>`, so on a `<translation
	// variants="no">` source the rewritten tag becomes
	// `<translation variants="no" type="unfinished">` rather than
	// `<translation type="unfinished" variants="no">`.
	newTag := append([]byte("<translation"), body...)
	newTag = append(newTag, ` type="unfinished"`...)
	newTag = append(newTag, '>')
	out := append([]byte{}, buf[:open]...)
	out = append(out, newTag...)
	return out
}

func (w *Writer) flush() error {
	if w.Output == nil {
		return nil
	}

	version := w.headerProps["version"]
	if version == "" {
		version = "2.1"
	}
	language := w.headerProps["language"]
	srcLanguage := w.headerProps["sourcelanguage"]

	// Determine target locale from writer setting or header
	targetLocale := model.LocaleID(language)
	if w.Locale != "" {
		targetLocale = w.Locale
	}

	// Write the XML prologue. When the reader captured the source
	// prologue + `<TS …>` tag (XML declaration with original encoding /
	// standalone, DOCTYPE with internal subset, leading comments),
	// re-emit those verbatim so round-trip preserves them. Otherwise
	// fall back to the hard-coded defaults.
	if prologue, ok := w.headerProps["xml-prologue"]; ok && prologue != "" {
		if _, err := io.WriteString(w.Output, prologue); err != nil {
			return err
		}
		if tsTag, ok := w.headerProps["ts-tag"]; ok && tsTag != "" {
			if _, err := io.WriteString(w.Output, tsTag); err != nil {
				return err
			}
			if _, err := io.WriteString(w.Output, "\n"); err != nil {
				return err
			}
		}
	} else {
		if _, err := io.WriteString(w.Output, xml.Header); err != nil {
			return err
		}
		if _, err := io.WriteString(w.Output, "<!DOCTYPE TS>\n"); err != nil {
			return err
		}
		var tsTag strings.Builder
		fmt.Fprintf(&tsTag, `<TS version="%s"`, xmlEscape(version))
		if language != "" {
			fmt.Fprintf(&tsTag, ` language="%s"`, xmlEscape(language))
		}
		if srcLanguage != "" {
			fmt.Fprintf(&tsTag, ` sourcelanguage="%s"`, xmlEscape(srcLanguage))
		}
		tsTag.WriteString(">\n")
		if _, err := io.WriteString(w.Output, tsTag.String()); err != nil {
			return err
		}
	}

	for _, grp := range w.groups {
		if _, err := io.WriteString(w.Output, "<context>\n"); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(w.Output, "    <name>%s</name>\n", xmlEscape(grp.name)); err != nil {
			return err
		}

		for _, block := range grp.blocks {
			if err := w.writeMessage(block, targetLocale); err != nil {
				return err
			}
		}

		if _, err := io.WriteString(w.Output, "</context>\n"); err != nil {
			return err
		}
	}

	if _, err := io.WriteString(w.Output, "</TS>\n"); err != nil {
		return err
	}

	return nil
}

func (w *Writer) writeMessage(block *model.Block, targetLocale model.LocaleID) error {
	// Build <message> opening tag
	var msgTag strings.Builder
	msgTag.WriteString("    <message")
	if block.ID != "" && !strings.HasPrefix(block.ID, "tu") {
		fmt.Fprintf(&msgTag, ` id="%s"`, xmlEscape(block.ID))
	}
	if block.Properties["numerus"] == "yes" {
		msgTag.WriteString(` numerus="yes"`)
	}
	msgTag.WriteString(">\n")
	if _, err := io.WriteString(w.Output, msgTag.String()); err != nil {
		return err
	}

	// Write source
	var sourceText string
	if len(block.Source) > 0 {
		sourceText = w.runsToXML(block.Source)
	}
	if _, err := fmt.Fprintf(w.Output, "        <source>%s</source>\n", sourceText); err != nil {
		return err
	}

	// Write comment
	if comment := block.Properties["comment"]; comment != "" {
		if _, err := fmt.Fprintf(w.Output, "        <comment>%s</comment>\n", xmlEscape(comment)); err != nil {
			return err
		}
	}

	// Write extracomment
	if extraComment := block.Properties["extracomment"]; extraComment != "" {
		if _, err := fmt.Fprintf(w.Output, "        <extracomment>%s</extracomment>\n", xmlEscape(extraComment)); err != nil {
			return err
		}
	}

	// Write translatorcomment
	if transComment := block.Properties["translatorcomment"]; transComment != "" {
		if _, err := fmt.Fprintf(w.Output, "        <translatorcomment>%s</translatorcomment>\n", xmlEscape(transComment)); err != nil {
			return err
		}
	}

	// Write translation
	transType := block.Properties["type"]

	if block.Properties["numerus"] == "yes" {
		// Write numerus forms
		transOpen := "        <translation"
		if transType != "" {
			transOpen += fmt.Sprintf(` type="%s"`, xmlEscape(transType))
		}
		transOpen += ">\n"
		if _, err := io.WriteString(w.Output, transOpen); err != nil {
			return err
		}

		// Each plural form is one span of the target-side segmentation
		// overlay over block.TargetRuns(targetLocale); the reader builds
		// one span per <numerusform> so the pseudo / TextModificationStep
		// pipeline reaches every form. Fall back to the legacy
		// `numerusform:<i>` properties for blocks built outside the
		// reader (tests, programmatic construction).
		formRuns := w.numerusFormRuns(block, targetLocale)
		if len(formRuns) > 0 {
			for _, runs := range formRuns {
				form := w.runsToXML(runs)
				if _, err := fmt.Fprintf(w.Output, "            <numerusform>%s</numerusform>\n", form); err != nil {
					return err
				}
			}
		} else {
			for i := 0; ; i++ {
				key := fmt.Sprintf("numerusform:%d", i)
				form, ok := block.Properties[key]
				if !ok {
					break
				}
				if _, err := fmt.Fprintf(w.Output, "            <numerusform>%s</numerusform>\n", xmlEscape(form)); err != nil {
					return err
				}
			}
		}

		if _, err := io.WriteString(w.Output, "        </translation>\n"); err != nil {
			return err
		}
	} else {
		// Write regular translation
		transOpen := "        <translation"
		if transType != "" {
			transOpen += fmt.Sprintf(` type="%s"`, xmlEscape(transType))
		}

		if block.HasTarget(targetLocale) {
			targetXML := w.runsToXML(block.TargetRuns(targetLocale))
			transOpen += fmt.Sprintf(">%s</translation>\n", targetXML)
		} else {
			transOpen += "></translation>\n"
		}
		if _, err := io.WriteString(w.Output, transOpen); err != nil {
			return err
		}
	}

	if _, err := io.WriteString(w.Output, "    </message>\n"); err != nil {
		return err
	}

	return nil
}

// runsToXML walks a Run sequence and returns an XML fragment —
// TextRun content XML-escaped, inline-code runs re-emitting the
// original Data (byte elements, <source>/<target> preserve this
// verbatim).
func (w *Writer) runsToXML(runs []model.Run) string {
	var buf strings.Builder
	writeTSRunsXML(&buf, runs, false)
	return buf.String()
}

// runsToXMLEscapeApos is runsToXML for `<numerusform>` content:
// `'` is encoded as `&apos;` rather than passed through raw, matching
// the okapi reference's per-form encoding.
func (w *Writer) runsToXMLEscapeApos(runs []model.Run) string {
	var buf strings.Builder
	writeTSRunsXML(&buf, runs, true)
	return buf.String()
}

// numerusFormRuns reconstructs the per-numerusform Run slices for a
// numerus block. The reader stores the plural forms as one flat target
// Run sequence plus a target-side SEGMENTATION OVERLAY whose spans
// (ordered by their `numerus-form` index) carve out each form. The
// writer extracts each span's runs via Range.ExtractRuns so it can emit
// one `<numerusform>` per form. When no segmentation overlay is present
// (e.g. a block built outside the reader, or whose forms were collapsed
// to a single target run by a downstream step), the whole target run
// sequence is returned as a single form so existing content still
// round-trips.
//
// When the requested locale has no target, falls back to any present
// target so a file declaring a non-matching `<TS language>` still
// passes its existing forms through unchanged.
func (w *Writer) numerusFormRuns(block *model.Block, locale model.LocaleID) [][]model.Run {
	runs := block.TargetRuns(locale)
	key := model.Variant(locale)
	if len(runs) == 0 {
		for _, loc := range block.TargetLocales() {
			if r := block.TargetRuns(loc); len(r) > 0 {
				runs = r
				key = model.Variant(loc)
				break
			}
		}
	}
	if len(runs) == 0 {
		return nil
	}
	ov := block.SegmentationFor(&key)
	if ov == nil || len(ov.Spans) == 0 {
		return [][]model.Run{runs}
	}
	forms := make([][]model.Run, len(ov.Spans))
	for i, span := range ov.Spans {
		forms[i] = span.Range.ExtractRuns(runs)
	}
	return forms
}

func writeTSRunsXML(buf *strings.Builder, runs []model.Run, escapeApos bool) {
	for _, r := range runs {
		switch {
		case r.Text != nil:
			for _, rr := range r.Text.Text {
				if escapeApos {
					buf.WriteString(xmlEscapeRuneEntity(rr))
				} else {
					buf.WriteString(xmlEscapeRune(rr))
				}
			}
		case r.Ph != nil:
			buf.WriteString(r.Ph.Data)
		case r.PcOpen != nil:
			buf.WriteString(r.PcOpen.Data)
		case r.PcClose != nil:
			buf.WriteString(r.PcClose.Data)
		case r.Sub != nil:
			buf.WriteString(r.Sub.Ref)
		case r.Plural != nil:
			if form, ok := r.Plural.Forms[model.PluralOther]; ok {
				writeTSRunsXML(buf, form, escapeApos)
			}
		case r.Select != nil:
			if form, ok := r.Select.Cases["other"]; ok {
				writeTSRunsXML(buf, form, escapeApos)
			}
		}
	}
}

// normalizeTSPrologue rewrites the document prologue at the start of
// the first skeleton-text chunk to match what okapi's TsFilter
// produces on output:
//
//   - The XML declaration is force-emitted as
//     `<?xml version="1.0" encoding="UTF-8"?>` regardless of what the
//     source declared. Woodstox normalises the encoding token to its
//     canonical (UTF-8) form and the filter discards the `standalone`
//     attribute.
//   - The DOCTYPE renders the internal-subset brackets — Woodstox's
//     `dtd.getDocumentTypeDeclaration()` always returns
//     `<!DOCTYPE TS []>` even when the source had `<!DOCTYPE TS>`
//     with no subset. Existing internal subsets pass through.
//   - Source line endings (CRLF / LF) and any leading BOM are
//     preserved.
//
// When the source had no XML declaration at all (Qt's lupdate emits
// some files that way), insert one. When the source had no DOCTYPE,
// don't add one — Woodstox only fires a DTD event when the source
// actually contained one.
func normalizeTSPrologue(data []byte) []byte {
	// Preserve a leading UTF-8 BOM if present.
	var bom []byte
	body := data
	if len(body) >= 3 && body[0] == 0xEF && body[1] == 0xBB && body[2] == 0xBF {
		bom = body[:3]
		body = body[3:]
	}

	// Skip leading whitespace before any markup so we recognise the
	// XML declaration even when the source had a stray newline first.
	leadStart := 0
	for leadStart < len(body) {
		c := body[leadStart]
		if c != ' ' && c != '\t' && c != '\n' && c != '\r' {
			break
		}
		leadStart++
	}
	leading := body[:leadStart]
	body = body[leadStart:]

	// Rewrite or insert the XML declaration.
	canonicalDecl := []byte(`<?xml version="1.0" encoding="UTF-8"?>`)
	if bytes.HasPrefix(body, []byte("<?xml")) {
		if end := bytes.Index(body, []byte("?>")); end >= 0 {
			tail := append([]byte{}, body[end+2:]...)
			body = append(append([]byte{}, canonicalDecl...), tail...)
		}
	} else {
		// Source had no XML declaration. Okapi prepends one with no
		// trailing line break (TsFilter.open: `skel.append("\"?>");`),
		// so whatever followed the missing declaration in the source
		// (DOCTYPE, root element, leading whitespace) appears
		// immediately after.
		tail := append([]byte{}, body...)
		body = append(append([]byte{}, canonicalDecl...), tail...)
	}

	// Force `[]` into a bracket-less DOCTYPE TS declaration. Skip if
	// the source already has an internal subset (the `[` appears
	// before the closing `>`).
	if idx := bytes.Index(body, []byte("<!DOCTYPE TS")); idx >= 0 {
		afterTag := idx + len("<!DOCTYPE TS")
		if closeIdx := bytes.IndexByte(body[afterTag:], '>'); closeIdx >= 0 {
			closeIdx += afterTag
			between := body[afterTag:closeIdx]
			if !bytes.ContainsRune(between, '[') {
				rebuilt := make([]byte, 0, len(body)+3)
				rebuilt = append(rebuilt, body[:idx]...)
				rebuilt = append(rebuilt, "<!DOCTYPE TS []"...)
				rebuilt = append(rebuilt, body[closeIdx:]...)
				body = rebuilt
			}
		}
	}

	out := make([]byte, 0, len(bom)+len(leading)+len(body))
	out = append(out, bom...)
	out = append(out, leading...)
	out = append(out, body...)
	return out
}

// xmlEscape escapes special XML characters.
func xmlEscape(s string) string {
	var buf strings.Builder
	for _, r := range s {
		buf.WriteString(xmlEscapeRune(r))
	}
	return buf.String()
}

// xmlEscapeRune escapes a single rune for XML inside Block content.
// Empirically matches okapi's writer output for `<source>` content:
// `&`, `<`, `>`, `"` escape; `'` is preserved raw. (The TSEncoder/
// XMLEncoder default `quoteMode=ALL` would normally escape `'` to
// `&apos;`, but the bridge's pseudo pipeline emits raw apostrophes
// in `<source>` text on the round-trip — see the per-form override
// below for `<numerusform>` which DOES escape.)
func xmlEscapeRune(r rune) string {
	switch r {
	case '&':
		return "&amp;"
	case '<':
		return "&lt;"
	case '>':
		return "&gt;"
	case '"':
		return "&quot;"
	default:
		return string(r)
	}
}

// xmlEscapeRuneEntity is the variant used for `<numerusform>` runs:
// it additionally encodes `'` → `&apos;`, matching the okapi
// reference's per-form encoding (vs source which keeps raw apos).
func xmlEscapeRuneEntity(r rune) string {
	if r == '\'' {
		return "&apos;"
	}
	return xmlEscapeRune(r)
}
