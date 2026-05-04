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
				if seg := block.FirstSegment(); seg != nil && len(seg.Runs) > 0 {
					text = w.runsToXML(seg.Runs)
				}
			case "translation":
				if block.HasTarget(targetLocale) {
					targetSegs := block.Targets[targetLocale]
					if len(targetSegs) > 0 && len(targetSegs[0].Runs) > 0 {
						text = w.runsToXML(targetSegs[0].Runs)
					}
				} else if len(block.Targets) > 0 {
					// File declared a target language other than ours
					// (e.g. <TS language="af">). Preserve the existing
					// translation so non-matching round-trips match
					// okapi's "leave bilingual content alone" semantics.
					for _, segs := range block.Targets {
						if len(segs) > 0 && len(segs[0].Runs) > 0 {
							text = w.runsToXML(segs[0].Runs)
							break
						}
					}
				}
			case "numerus_translation":
				// One <numerusform>…</numerusform> per segment in the
				// target locale; mirrors okapi's TextModificationStep
				// path which pseudo-translates every plural form, not
				// just the first.
				segs := block.Targets[targetLocale]
				if len(segs) == 0 {
					// File declared a non-matching target locale; pick
					// any present target so existing forms pass through.
					for _, s := range block.Targets {
						if len(s) > 0 {
							segs = s
							break
						}
					}
				}
				var b strings.Builder
				for _, seg := range segs {
					if seg == nil {
						continue
					}
					b.WriteString("<numerusform>")
					b.WriteString(w.runsToXML(seg.Runs))
					b.WriteString("</numerusform>")
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

	// No type attribute — insert ` type="unfinished"` right after
	// `<translation`.
	newTag := append([]byte("<translation"), ` type="unfinished"`...)
	newTag = append(newTag, body...)
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
	if seg := block.FirstSegment(); seg != nil && len(seg.Runs) > 0 {
		sourceText = w.runsToXML(seg.Runs)
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

		// Each plural form is its own segment under
		// block.Targets[targetLocale]; the reader builds one segment per
		// <numerusform> so the pseudo / TextModificationStep pipeline
		// reaches every form. Fall back to the legacy
		// `numerusform:<i>` properties for blocks built outside the
		// reader (tests, programmatic construction).
		segs := block.Targets[targetLocale]
		if len(segs) > 0 {
			for _, seg := range segs {
				if seg == nil {
					continue
				}
				form := w.runsToXML(seg.Runs)
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
			targetSegs := block.Targets[targetLocale]
			if len(targetSegs) > 0 && len(targetSegs[0].Runs) > 0 {
				targetXML := w.runsToXML(targetSegs[0].Runs)
				transOpen += fmt.Sprintf(">%s</translation>\n", targetXML)
			} else {
				transOpen += "></translation>\n"
			}
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
	writeTSRunsXML(&buf, runs)
	return buf.String()
}

func writeTSRunsXML(buf *strings.Builder, runs []model.Run) {
	for _, r := range runs {
		switch {
		case r.Text != nil:
			for _, rr := range r.Text.Text {
				buf.WriteString(xmlEscapeRune(rr))
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
				writeTSRunsXML(buf, form)
			}
		case r.Select != nil:
			if form, ok := r.Select.Cases["other"]; ok {
				writeTSRunsXML(buf, form)
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

// xmlEscapeRune escapes a single rune for XML.
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
