package tmx

import (
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

// Writer implements DataFormatWriter for TMX files.
type Writer struct {
	format.BaseFormatWriter
	skeletonStore *format.SkeletonStore
	headerProps   map[string]string
	blocks        []*model.Block
}

// Ensure Writer implements SkeletonStoreConsumer.
var _ format.SkeletonStoreConsumer = (*Writer)(nil)

// NewWriter creates a new TMX writer.
func NewWriter() *Writer {
	return &Writer{
		BaseFormatWriter: format.BaseFormatWriter{
			FormatName: "tmx",
		},
		headerProps: make(map[string]string),
	}
}

// SetSkeletonStore sets the skeleton store for byte-exact output.
func (w *Writer) SetSkeletonStore(store *format.SkeletonStore) {
	w.skeletonStore = store
}

// Write consumes Parts from a channel and writes TMX XML.
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
	case model.PartBlock:
		if block, ok := part.Resource.(*model.Block); ok {
			w.blocks = append(w.blocks, block)
		}
	case model.PartData:
		if data, ok := part.Resource.(*model.Data); ok {
			if data.Name == "tmx-header" {
				w.headerProps = data.Properties
			}
		}
	}
}

// writeFromSkeleton reads skeleton entries and fills in block content.
func (w *Writer) writeFromSkeleton() error {
	if err := w.skeletonStore.Flush(); err != nil {
		return fmt.Errorf("tmx writer: flush skeleton: %w", err)
	}

	// Build a lookup: srcLang from header
	srcLang := strings.ToLower(w.headerProps["srclang"])
	if srcLang == "" {
		srcLang = "en"
	}

	for {
		entry, err := w.skeletonStore.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("tmx writer: read skeleton: %w", err)
		}
		switch entry.Type {
		case format.SkeletonText:
			if _, err := w.Output.Write(entry.Data); err != nil {
				return err
			}
		case format.SkeletonRef:
			// Ref ID is "tuIdx:lang" where tuIdx is 0-based
			refID := string(entry.Data)
			idxStr, refSuffix, ok := strings.Cut(refID, ":")
			if !ok {
				continue
			}
			tuIdx, err := strconv.Atoi(idxStr)
			if err != nil || tuIdx < 0 || tuIdx >= len(w.blocks) {
				continue
			}
			block := w.blocks[tuIdx]
			lang := refSuffix

			// Determine the segment runs for this TUV. renderTMXSeg
			// preserves inline codes (<ph>, <bpt>, <ept>, <it>, <hi>)
			// that block.SourceText / TargetText would silently drop.
			var segs []*model.Segment
			langLower := strings.ToLower(lang)
			if langMatches(langLower, srcLang) {
				segs = block.Source
			} else {
				localeID := model.LocaleID(lang)
				if block.HasTarget(localeID) {
					segs = block.Targets[localeID]
				} else {
					segs = block.Source
				}
			}

			if _, err := io.WriteString(w.Output, renderTMXSeg(segs)); err != nil {
				return err
			}
		}
	}
	return nil
}

// renderTMXSeg walks a segment slice and produces the inner XML for a
// <seg> element. TextRun content is XML-escaped; Ph / PcOpen / PcClose
// runs are emitted as <ph>, <bpt>, <ept>, <it>, or <hi> based on the
// SubType set by the reader. Inline element Data is also XML-escaped.
//
// <hi> is special-cased — the reader emits PcOpen{hi}+Text+PcClose{hi}
// so the inner text remains translatable, but on output we collapse
// back to a single `<hi ...>text</hi>` element instead of emitting a
// self-closed open and a separate close.
//
// Attribute order follows okapi's writer to maximise byte-level
// agreement: i (paired-code id), pos (it only), type, x.
func renderTMXSeg(segs []*model.Segment) string {
	var b strings.Builder
	xCounter := 0
	for _, seg := range segs {
		if seg == nil {
			continue
		}
		for _, run := range seg.Runs {
			switch {
			case run.Text != nil:
				b.WriteString(xmlEscapeString(run.Text.Text))
			case run.Ph != nil:
				xCounter++
				writeTMXInline(&b, "ph", run.Ph.SubType, run.Ph.ID, run.Ph.Type, "", xCounter, run.Ph.Data)
			case run.PcOpen != nil:
				if run.PcOpen.SubType == "tmx-hi" {
					xCounter++
					writeTMXHiOpen(&b, run.PcOpen.Type, xCounter)
					continue
				}
				xCounter++
				writeTMXInline(&b, "bpt", run.PcOpen.SubType, run.PcOpen.ID, run.PcOpen.Type, "begin", xCounter, run.PcOpen.Data)
			case run.PcClose != nil:
				if run.PcClose.SubType == "tmx-hi" {
					b.WriteString("</hi>")
					continue
				}
				xCounter++
				writeTMXInline(&b, "ept", run.PcClose.SubType, run.PcClose.ID, run.PcClose.Type, "end", xCounter, run.PcClose.Data)
			}
		}
	}
	return b.String()
}

// writeTMXHiOpen emits an opening `<hi ...>` tag without auto-closing,
// since `<hi>` wraps translatable inner text and the matching close tag
// arrives later as a PcClose run.
func writeTMXHiOpen(b *strings.Builder, spanType string, x int) {
	b.WriteString("<hi")
	if spanType != "" {
		b.WriteString(` type="`)
		b.WriteString(xmlEscapeAttr(spanType))
		b.WriteByte('"')
	}
	b.WriteString(` x="`)
	b.WriteString(strconv.Itoa(x))
	b.WriteString(`">`)
}

// writeTMXInline emits one self-closing TMX inline element (<ph>,
// <bpt>, <ept>, <it>): opening tag with attributes, escaped Data,
// closing tag. Use writeTMXHiOpen / </hi> for paired <hi> elements.
func writeTMXInline(b *strings.Builder, defaultElem, subType, id, spanType, defaultPos string, x int, data string) {
	elem := defaultElem
	pos := ""
	switch subType {
	case "tmx-bpt":
		elem = "bpt"
	case "tmx-ept":
		elem = "ept"
	case "tmx-ph":
		elem = "ph"
	case "tmx-it":
		elem = "it"
	case "tmx-it-begin":
		elem = "it"
		pos = "begin"
	case "tmx-it-end":
		elem = "it"
		pos = "end"
	}

	b.WriteByte('<')
	b.WriteString(elem)
	if pos != "" {
		b.WriteString(` pos="`)
		b.WriteString(xmlEscapeAttr(pos))
		b.WriteByte('"')
	} else if defaultPos != "" && elem == "it" {
		b.WriteString(` pos="`)
		b.WriteString(xmlEscapeAttr(defaultPos))
		b.WriteByte('"')
	}
	if elem == "bpt" || elem == "ept" {
		if id != "" {
			b.WriteString(` i="`)
			b.WriteString(xmlEscapeAttr(id))
			b.WriteByte('"')
		}
	}
	if spanType != "" {
		b.WriteString(` type="`)
		b.WriteString(xmlEscapeAttr(spanType))
		b.WriteByte('"')
	}
	b.WriteString(` x="`)
	b.WriteString(strconv.Itoa(x))
	b.WriteByte('"')
	b.WriteByte('>')
	b.WriteString(xmlEscapeString(data))
	b.WriteString("</")
	b.WriteString(elem)
	b.WriteByte('>')
}

// xmlEscapeAttr escapes for attribute-value context (adds quote
// escaping on top of xmlEscapeString).
func xmlEscapeAttr(s string) string {
	s = xmlEscapeString(s)
	return strings.ReplaceAll(s, `"`, "&quot;")
}

// xmlEscapeString escapes special XML characters in text content.
func xmlEscapeString(s string) string {
	var buf strings.Builder
	for _, r := range s {
		switch r {
		case '&':
			buf.WriteString("&amp;")
		case '<':
			buf.WriteString("&lt;")
		case '>':
			buf.WriteString("&gt;")
		default:
			buf.WriteRune(r)
		}
	}
	return buf.String()
}

// xmlTMX and related types for output.
type xmlTMX struct {
	XMLName xml.Name  `xml:"tmx"`
	Version string    `xml:"version,attr"`
	Header  xmlHeader `xml:"header"`
	Body    xmlBody   `xml:"body"`
}

type xmlHeader struct {
	CreationTool        string `xml:"creationtool,attr,omitempty"`
	CreationToolVersion string `xml:"creationtoolversion,attr,omitempty"`
	SegType             string `xml:"segtype,attr,omitempty"`
	OriginalFormat      string `xml:"o-tmf,attr,omitempty"`
	AdminLang           string `xml:"adminlang,attr,omitempty"`
	SrcLang             string `xml:"srclang,attr,omitempty"`
	DataType            string `xml:"datatype,attr,omitempty"`
}

type xmlBody struct {
	TUs []xmlTU `xml:"tu"`
}

type xmlTU struct {
	TUid string   `xml:"tuid,attr,omitempty"`
	TUVs []xmlTUV `xml:"tuv"`
}

type xmlTUV struct {
	Lang string `xml:"xml:lang,attr"`
	Seg  xmlSeg `xml:"seg"`
}

// xmlSeg renders the <seg> body via innerxml so we can splice inline
// codes (<ph>, <bpt>, <ept>, <it>, <hi>) generated by renderTMXSeg
// without xml.Encoder escaping them. The TextRun portions are
// XML-escaped inside renderTMXSeg before being concatenated.
type xmlSeg struct {
	Inner string `xml:",innerxml"`
}

func (w *Writer) flush() error {
	if w.Output == nil {
		return nil
	}

	version := w.headerProps["version"]
	if version == "" {
		version = "1.4"
	}

	srcLang := w.headerProps["srclang"]
	if srcLang == "" {
		srcLang = "en"
	}

	doc := xmlTMX{
		Version: version,
		Header: xmlHeader{
			CreationTool:        w.headerProps["creationtool"],
			CreationToolVersion: w.headerProps["creationtoolversion"],
			SegType:             w.headerProps["segtype"],
			OriginalFormat:      w.headerProps["o-tmf"],
			AdminLang:           w.headerProps["adminlang"],
			SrcLang:             srcLang,
			DataType:            w.headerProps["datatype"],
		},
	}

	for _, block := range w.blocks {
		tu := xmlTU{
			TUid: block.ID,
		}

		// Add source TUV
		tu.TUVs = append(tu.TUVs, xmlTUV{
			Lang: srcLang,
			Seg:  xmlSeg{Inner: renderTMXSeg(block.Source)},
		})

		// Add target TUVs
		for locale, segs := range block.Targets {
			if len(segs) == 0 {
				continue
			}
			tu.TUVs = append(tu.TUVs, xmlTUV{
				Lang: string(locale),
				Seg:  xmlSeg{Inner: renderTMXSeg(segs)},
			})
		}

		doc.Body.TUs = append(doc.Body.TUs, tu)
	}

	if _, err := fmt.Fprint(w.Output, xml.Header); err != nil {
		return err
	}

	encoder := xml.NewEncoder(w.Output)
	encoder.Indent("", "  ")
	if err := encoder.Encode(doc); err != nil {
		return fmt.Errorf("tmx writer: encoding: %w", err)
	}

	return nil
}
