// Structured document tags (content controls): the block-level w:sdt wrapper
// and its inline form, whose sdtPr/sdtEndPr markup is skeleton while the
// sdtContent inside stays translatable.

package openxml

import (
	"encoding/xml"
	"strings"

	"github.com/neokapi/neokapi/core/model"
)

// parseSDT parses a structured document tag, extracting its content.
func (p *wmlParser) parseSDT(d *xml.Decoder, partPath string, emitBlock func(*model.Block), emitData func()) error {
	// The caller's case branch consumed the `<w:sdt>` start element. Write
	// it to the skeleton so the writer re-emits the SDT envelope on
	// round-trip; bridge preserves <w:sdt><w:sdtContent>...</w:sdtContent>
	// </w:sdt> wrappers around block-level paragraphs (e.g. watermark
	// header2.xml contains a single paragraph wrapped in sdt). Per
	// ECMA-376-1 §17.5.2.31 (CT_SdtBlock) the sdt is a structural envelope
	// for content controls; dropping it on round-trip changes the document
	// structure and breaks the byte-equivalence guarantee.
	//
	// <w:sdtPr> (the SDT properties block — id, alias, dataBinding, …) and
	// <w:sdtEndPr> (post-content rPr) are captured raw because their
	// children carry attributes the streaming skeleton emit would not
	// preserve byte-for-byte. <w:sdtContent> is the content envelope; the
	// inner <w:p> paragraphs route through parseParagraph normally and
	// emit their block refs into the skeleton between the wrapper markers.
	// Nested <w:sdt> inside <w:sdtContent> recurses (Practice2.docx
	// footer2.xml).
	depth := 1
	inContent := false

	p.skelText("<w:sdt>")

	// Buffer for sdtPr / sdtEndPr captured payloads — they appear before
	// the content and must be emitted between `<w:sdt>` and
	// `<w:sdtContent>`.
	var preContent strings.Builder

	for depth > 0 {
		tok, err := d.Token()
		if err != nil {
			return err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			depth++
			switch t.Name.Local {
			case "sdtContent":
				p.skelText(preContent.String())
				preContent.Reset()
				inContent = true
				p.skelText("<w:sdtContent>")
			case "sdtPr", "sdtEndPr":
				raw, err := captureRawElement(d, t)
				if err != nil {
					return err
				}
				preContent.WriteString(raw)
				depth--
			case "sdt":
				if err := p.parseSDT(d, partPath, emitBlock, emitData); err != nil {
					return err
				}
				depth--
			case "p":
				if inContent {
					if err := p.parseParagraph(d, partPath, emitBlock); err != nil {
						return err
					}
					depth--
				}
			}
		case xml.EndElement:
			depth--
			if t.Name.Local == "sdtContent" {
				inContent = false
				p.skelText("</w:sdtContent>")
			}
		}
	}
	if preContent.Len() > 0 {
		p.skelText(preContent.String())
	}
	p.skelText("</w:sdt>")
	return nil
}

// sdtEndPrIsEmpty reports whether a captured `<w:sdtEndPr ...>` element
// has no child elements (either self-closing `<w:sdtEndPr/>` or empty
// body `<w:sdtEndPr></w:sdtEndPr>`). Per ECMA-376-1 \u00A717.5.2.38
// (CT_SdtEndPr) the element carries `<w:rPr>` children defining the
// post-content run properties; in practice most authoring tools emit
// an empty sdtEndPr that upstream Okapi drops on round-trip (the
// RunContainer SDT path filters the empty form out via the
// SDT_END_PROPERTIES skippable set).
//
// Used by parseInlineSDT to suppress the empty form when wrapping the
// SDT envelope into the paired-code OPEN sentinel \u2014 keeping the
// non-empty form for fidelity in the rare cases it carries children.
// 1085.docx is the canonical empty-form fixture.
func sdtEndPrIsEmpty(raw string) bool {
	// Self-closing form: `<w:sdtEndPr/>` or `<w:sdtEndPr ... />`.
	if strings.HasSuffix(strings.TrimRight(raw, " \t\r\n"), "/>") {
		return true
	}
	// Empty-body form: `<w:sdtEndPr ...></w:sdtEndPr>` with only whitespace
	// (or nothing) between the open and close tags.
	_, after, ok := strings.Cut(raw, ">")
	if !ok {
		return false
	}
	body := after
	closeIdx := strings.LastIndex(body, "</")
	if closeIdx < 0 {
		return false
	}
	return strings.TrimSpace(body[:closeIdx]) == ""
}

// parseInlineSDT drains an inline `<w:sdt>` wrapper, processing its
// child runs as if they were direct paragraph children and emitting
// paired-code sentinels (\uE10E open / \uE10F close, shared with the
// strict-OOXML revision-insertion path) around them so the writer can
// re-emit the SDT envelope verbatim.
//
// The OPEN sentinel carries the captured raw `<w:sdt ...>` start tag
// followed by every child verbatim up to the matching `</w:sdtContent>`
// open boundary — i.e. the captured `<w:sdtPr>...</w:sdtPr>`, the
// optional `<w:sdtEndPr/>`, and the `<w:sdtContent>` start tag itself.
// The CLOSE sentinel emits the literal `</w:sdtContent></w:sdt>` close
// pair. Inner runs of `<w:sdtContent>` are parsed inline and live in
// the textRun slice between the OPEN and CLOSE sentinels.
//
// When `<w:sdtContent>` is self-closing (no inner runs at all — the
// 1085.docx fixture: `<w:sdt><w:sdtPr><w:tag/><w:id/></w:sdtPr>
// <w:sdtEndPr/><w:sdtContent/></w:sdt>`), the OPEN sentinel emits
// `<w:sdt><w:sdtPr>...</w:sdtPr><w:sdtEndPr/><w:sdtContent>` and the
// CLOSE emits `</w:sdtContent></w:sdt>`; the empty
// `<w:sdtContent></w:sdtContent>` is canonical-equivalent to the
// self-closing form (XML canonicalisation collapses an empty element
// to its self-closing variant).
//
// Mirrors upstream Okapi RunContainer (RunContainer.java:97-176),
// which preserves <w:sdt>, <w:sdtPr>, <w:sdtEndPr>, and <w:sdtContent>
// as outer/inner markup around the extracted inner content
// (RunContainer.RUN_CONTAINER_TYPES + the sdt-specific properties handler).
// Per ECMA-376 Part 1 / ISO/IEC 29500-1 §17.5.2 (Structured Document
// Tags), `<w:sdtPr>` and `<w:sdtEndPr>` carry SDT metadata (id, tag,
// alias, …) that must round-trip; `<w:sdtContent>` wraps the placeholder
// content.
//
// rawStart is the raw XML form of the `<w:sdt ...>` open tag (including
// any attributes) produced by the caller via startElementToRaw.
func (p *wmlParser) parseInlineSDT(d *xml.Decoder, runs *[]textRun, rawStart string) error {
	// Capture sdtPr (always present per CT_SdtRun) and the optional
	// sdtEndPr verbatim, then accumulate them onto rawStart so the
	// OPEN sentinel emits the full `<w:sdt><w:sdtPr>...</w:sdtPr>
	// <w:sdtEndPr/><w:sdtContent>` prefix.
	var wrapperOpen strings.Builder
	wrapperOpen.WriteString(rawStart)
	inSdtContent := false
	for !inSdtContent {
		tok, err := d.Token()
		if err != nil {
			return err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "sdtPr":
				raw, err := captureRawElement(d, t)
				if err != nil {
					return err
				}
				wrapperOpen.WriteString(raw)
			case "sdtEndPr":
				// Empty <w:sdtEndPr/> is dropped by upstream Okapi
				// (RunContainer.SDT_END_PROPERTIES filter — only the
				// non-trivial members survive). When sdtEndPr carries
				// child elements (rare), preserve verbatim.
				raw, err := captureRawElement(d, t)
				if err != nil {
					return err
				}
				// Self-closing or empty body: drop. Otherwise keep.
				if !sdtEndPrIsEmpty(raw) {
					wrapperOpen.WriteString(raw)
				}
			case "sdtContent":
				wrapperOpen.WriteString(startElementToRaw(t))
				inSdtContent = true
			default:
				// Unknown SDT child outside sdtContent — skip the
				// subtree to keep round-trip safe; future fixtures
				// can add handling here.
				if err := skipElement(d); err != nil {
					return err
				}
			}
		case xml.EndElement:
			// Premature `</w:sdt>` — the SDT had no sdtContent at all.
			// Emit a single-shot pair: the OPEN sentinel carries the
			// rawStart + captured sdtPr/sdtEndPr; the CLOSE sentinel
			// emits a synthesised empty `</w:sdt>` pair (no
			// sdtContent boundary because the source had none).
			if t.Name.Local == "sdt" {
				*runs = append(*runs, textRun{text: "\uE10E:sdt-no-content:" + wrapperOpen.String(), props: runProps{}})
				*runs = append(*runs, textRun{text: "\uE10F:sdt-no-content:</w:sdt>", props: runProps{}})
				return nil
			}
		}
	}
	// Emit the OPEN sentinel covering everything through the
	// `<w:sdtContent>` start tag.
	*runs = append(*runs, textRun{text: "\uE10E:sdt:" + wrapperOpen.String(), props: runProps{}})
	// Drain `<w:sdtContent>` children, processing inner runs inline.
	var cfs complexFieldState
	for {
		tok, err := d.Token()
		if err != nil {
			return err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "r":
				rawRStart := startElementToRaw(t)
				r, err := p.parseRunWithFieldState(d, &cfs, rawRStart)
				if err != nil {
					return err
				}
				*runs = append(*runs, r...)
			case "sdt":
				// Nested inline SDT inside <w:sdtContent> — recurse
				// so the inner OPEN/CLOSE sentinels (and any text
				// runs they bracket) sit between our own sentinels in
				// the textRun stream. 834.docx footnotes.xml is the
				// canonical fixture: an outer SDT whose <w:sdtContent>
				// carries an inner <w:sdt> followed by a trailing
				// <w:r>; without recursion the nested SDT subtree
				// (and the trailing same-rPr run) are dropped on
				// output. Mirrors upstream Okapi RunContainer.java
				// :97-176 where SDT is part of RUN_CONTAINER_TYPES
				// and the parent RunContainer re-enters the same
				// parser for nested run-containers.
				rawNested := startElementToRaw(t)
				if err := p.parseInlineSDT(d, runs, rawNested); err != nil {
					return err
				}
			case "proofErr", "permStart", "permEnd", "bookmarkStart", "bookmarkEnd":
				// Skippable revision/bookmark markers — drop and
				// continue. Mirrors parseParagraph's treatment of the
				// same elements (run-container content is otherwise
				// transparent per RunContainer.java:97-176).
				if err := skipElement(d); err != nil {
					return err
				}
			default:
				// Unknown child inside sdtContent — skip subtree.
				if err := skipElement(d); err != nil {
					return err
				}
			}
		case xml.EndElement:
			if t.Name.Local == "sdtContent" {
				// Now drain to the matching `</w:sdt>` (no children
				// expected after sdtContent per CT_SdtRun, but be
				// defensive).
				for {
					tok2, err := d.Token()
					if err != nil {
						return err
					}
					if et, ok := tok2.(xml.EndElement); ok && et.Name.Local == "sdt" {
						*runs = append(*runs, textRun{text: "\uE10F:sdt:</w:sdtContent></w:sdt>", props: runProps{}})
						return nil
					}
				}
			}
		}
	}
}
