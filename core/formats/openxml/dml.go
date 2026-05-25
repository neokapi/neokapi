package openxml

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
)

// dmlNamespace is the DrawingML namespace.
const dmlNamespace = "http://schemas.openxmlformats.org/drawingml/2006/main"

// dmlParser parses DrawingML XML parts (PPTX slides, notes, masters).
type dmlParser struct {
	cfg           *Config
	blockCounter  *int
	skeletonStore *format.SkeletonStore
	skelBuf       bytes.Buffer
	rels          map[string]relationship

	// stripEmptyParaProps mirrors okapi's BlockProperties.Default.
	// getEvents (line 169-171 of okapi/filters/openxml/src/main/java/
	// net/sf/okapi/filters/openxml/BlockProperties.java) which omits
	// the entire pPr element when isEmpty() returns true (no
	// attributes, no non-empty children). Set true when parsing
	// chart/diagram parts where okapi unconditionally strips
	// scaffold-only <a:pPr><a:defRPr/></a:pPr> blocks (see
	// gold/Transimple_chart.docx). Left false for PPTX slides where
	// the existing behaviour preserved pPr verbatim.
	stripEmptyParaProps bool
}

// parsePart streams through a DrawingML XML part, emitting Blocks.
func (p *dmlParser) parsePart(data []byte, partPath string, emitBlock func(*model.Block)) error {
	d := xml.NewDecoder(bytes.NewReader(data))

	for {
		tok, err := d.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("dml: parsing %s: %w", partPath, err)
		}

		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "txBody":
				// DrawingML text body — contains paragraphs
				if err := p.parseTextBody(d, partPath, emitBlock); err != nil {
					return err
				}
			default:
				p.skelWriteStartElement(t)
			}

		case xml.EndElement:
			p.skelWriteEndElement(t)

		case xml.CharData:
			p.skelText(xmlEscape(string(t)))

		case xml.ProcInst:
			p.skelText("<?" + t.Target + " " + string(t.Inst) + "?>")

		case xml.Comment:
			p.skelText("<!--" + string(t) + "-->")
		}
	}
	return nil
}

// parseTextBody parses an <a:txBody> element.
func (p *dmlParser) parseTextBody(d *xml.Decoder, partPath string, emitBlock func(*model.Block)) error {
	p.skelWriteString("<a:txBody>")

	for {
		tok, err := d.Token()
		if err != nil {
			return err
		}

		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "p":
				if err := p.parseParagraph(d, partPath, emitBlock); err != nil {
					return err
				}
			case "bodyPr", "lstStyle":
				raw, err := captureRawElement(d, t)
				if err != nil {
					return err
				}
				p.skelText(raw)
			default:
				p.skelWriteStartElement(t)
			}

		case xml.EndElement:
			if t.Name.Local == "txBody" {
				p.skelWriteString("</a:txBody>")
				return nil
			}
			p.skelWriteEndElement(t)
		}
	}
}

// parseParagraph parses an <a:p> element and emits a Block.
func (p *dmlParser) parseParagraph(d *xml.Decoder, partPath string, emitBlock func(*model.Block)) error {
	var runs []textRun
	var paraProps string

	for {
		tok, err := d.Token()
		if err != nil {
			return err
		}

		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "pPr", "endParaRPr":
				raw, err := captureRawElement(d, t)
				if err != nil {
					return err
				}
				if t.Name.Local == "pPr" {
					if p.stripEmptyParaProps && isStructurallyEmptyDMLBlockProperties(raw) {
						paraProps = ""
					} else {
						paraProps = raw
					}
				}

			case "r":
				run, err := p.parseRun(d)
				if err != nil {
					return err
				}
				runs = append(runs, run...)

			case "br":
				runs = append(runs, textRun{text: "\n", props: runProps{}})
				if err := skipElement(d); err != nil {
					return err
				}

			default:
				if err := skipElement(d); err != nil {
					return err
				}
			}

		case xml.EndElement:
			if t.Name.Local == "p" {
				merged := mergeRuns(runs)

				if isEmptyRuns(merged) {
					p.skelWriteString("<a:p>")
					if paraProps != "" {
						p.skelText(paraProps)
					}
					p.skelWriteString("</a:p>")
					return nil
				}

				*p.blockCounter++
				blockID := fmt.Sprintf("tu%d", *p.blockCounter)

				p.skelWriteString("<a:p>")
				if paraProps != "" {
					p.skelText(paraProps)
				}
				p.skelRef(blockID)
				p.skelWriteString("</a:p>")

				block := p.buildBlock(blockID, merged, partPath)
				emitBlock(block)
				return nil
			}
		}
	}
}

// parseRun parses an <a:r> element.
func (p *dmlParser) parseRun(d *xml.Decoder) ([]textRun, error) {
	var props runProps
	var runs []textRun

	for {
		tok, err := d.Token()
		if err != nil {
			return nil, err
		}

		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "rPr":
				props = parseDMLRunProps(t)
				if err := skipElement(d); err != nil {
					return nil, err
				}
			case "t":
				text, err := readCharData(d)
				if err != nil {
					return nil, err
				}
				runs = append(runs, textRun{text: text, props: props})
			default:
				if err := skipElement(d); err != nil {
					return nil, err
				}
			}

		case xml.EndElement:
			if t.Name.Local == "r" {
				return runs, nil
			}
		}
	}
}

// parseDMLRunProps extracts run properties from DrawingML <a:rPr> attributes.
func parseDMLRunProps(el xml.StartElement) runProps {
	var props runProps
	for _, a := range el.Attr {
		switch a.Name.Local {
		case "b":
			props.bold = a.Value == "1" || a.Value == "true"
		case "i":
			props.italic = a.Value == "1" || a.Value == "true"
		case "u":
			if a.Value != "" && a.Value != "none" {
				props.underline = a.Value
			}
		case "strike":
			if a.Value != "" && a.Value != "noStrike" {
				props.strike = true
			}
		case "baseline":
			if strings.HasPrefix(a.Value, "-") {
				props.vertAlign = "subscript"
			} else if a.Value != "" && a.Value != "0" {
				props.vertAlign = "superscript"
			}
		}
	}
	return props
}

// buildBlock creates a model.Block from text runs.
func (p *dmlParser) buildBlock(id string, runs []textRun, partPath string) *model.Block {
	b := &runBuilder{}
	spanCounter := 0
	var activeProps *runProps

	for _, run := range runs {
		if run.text == "\n" {
			spanCounter++
			b.AddPh(fmt.Sprintf("c%d", spanCounter),
				TypeBreak, SubTypeBreak,
				"<a:br/>", "\n", "",
				false, false, false)
			continue
		}

		if activeProps == nil || !activeProps.equal(run.props) {
			if activeProps != nil && !activeProps.isEmpty() {
				activeProps.appendClosingRuns(b, &spanCounter)
			}
			if !run.props.isEmpty() {
				run.props.appendOpeningRuns(b, &spanCounter)
			}
			propsCopy := run.props
			activeProps = &propsCopy
		}

		b.AddText(run.text)
	}

	if activeProps != nil && !activeProps.isEmpty() {
		activeProps.appendClosingRuns(b, &spanCounter)
	}

	return &model.Block{
		ID:           id,
		Type:         "paragraph",
		Translatable: true,
		Source:       b.Runs(),
		Targets:      make(map[model.VariantKey]*model.Target),
		Properties:   map[string]string{"partPath": partPath},
		Annotations:  make(map[string]model.Annotation),
	}
}

// Skeleton helpers

func (p *dmlParser) skelText(s string) {
	if p.skeletonStore != nil {
		p.skelBuf.WriteString(s)
	}
}

func (p *dmlParser) skelRef(id string) {
	if p.skeletonStore != nil {
		if p.skelBuf.Len() > 0 {
			_ = p.skeletonStore.WriteText(p.skelBuf.Bytes())
			p.skelBuf.Reset()
		}
		_ = p.skeletonStore.WriteRef(id)
	}
}

func (p *dmlParser) skelFlush() {
	if p.skeletonStore != nil && p.skelBuf.Len() > 0 {
		_ = p.skeletonStore.WriteText(p.skelBuf.Bytes())
		p.skelBuf.Reset()
	}
}

func (p *dmlParser) skelWriteStartElement(t xml.StartElement) {
	if p.skeletonStore == nil {
		return
	}
	registerNamespaces(t.Attr)
	var buf strings.Builder
	buf.WriteString("<")
	writeElementName(&buf, t.Name)
	for _, a := range t.Attr {
		buf.WriteString(" ")
		writeAttrName(&buf, a.Name)
		buf.WriteString(`="`)
		buf.WriteString(xmlEscapeAttr(a.Value))
		buf.WriteString(`"`)
	}
	buf.WriteString(">")
	p.skelBuf.WriteString(buf.String())
}

func (p *dmlParser) skelWriteEndElement(t xml.EndElement) {
	if p.skeletonStore == nil {
		return
	}
	var buf strings.Builder
	buf.WriteString("</")
	writeElementName(&buf, t.Name)
	buf.WriteString(">")
	p.skelBuf.WriteString(buf.String())
}

func (p *dmlParser) skelWriteString(s string) {
	if p.skeletonStore != nil {
		p.skelBuf.WriteString(s)
	}
}
