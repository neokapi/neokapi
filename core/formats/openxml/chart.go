package openxml

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/neokapi/neokapi/core/model"
)

// isChartPartPath reports whether the ZIP entry path is a DrawingML
// chart part. Chart parts live under {word,ppt,xl}/charts/ and have
// the content type application/vnd.openxmlformats-officedocument.
// drawingml.chart+xml. The path-prefix probe is sufficient because
// non-chart XML files in the same directory don't exist by spec.
func isChartPartPath(name string) bool {
	if !strings.HasSuffix(name, ".xml") {
		return false
	}
	switch {
	case strings.HasPrefix(name, "word/charts/"),
		strings.HasPrefix(name, "ppt/charts/"),
		strings.HasPrefix(name, "xl/charts/"):
		// Exclude /_rels/ subdirectories.
		return !strings.Contains(name, "/_rels/")
	}
	return false
}

// isDiagramDataPartPath reports whether the ZIP entry path is a
// SmartArt diagram-data part (word/diagrams/data*.xml or its PPTX/XLSX
// equivalents). Other diagram parts (layout*.xml, colors*.xml,
// quickStyle*.xml, drawing*.xml) are NOT translatable by this filter
// per okapi WordDocument.java line 202 — only DIAGRAM_DATA_TYPE.
func isDiagramDataPartPath(name string) bool {
	if !strings.HasSuffix(name, ".xml") {
		return false
	}
	if strings.Contains(name, "/_rels/") {
		return false
	}
	// data1.xml, data2.xml, etc. live alongside layout/colors/etc. in
	// {word,ppt,xl}/diagrams/. Match the basename prefix to exclude
	// non-data siblings.
	for _, prefix := range []string{"word/diagrams/", "ppt/diagrams/", "xl/diagrams/"} {
		if rel, ok := strings.CutPrefix(name, prefix); ok {
			return strings.HasPrefix(rel, "data")
		}
	}
	return false
}

// isStructurallyEmptyDMLBlockProperties reports whether a captured
// DrawingML block-properties container (e.g. <a:pPr>...</a:pPr>) carries
// no attributes on the outer element AND only structurally-empty child
// run-property containers (<a:defRPr/>, <a:defRPr></a:defRPr>). Mirrors
// okapi's BlockProperties.Default.isEmpty (line 203-205 of okapi/
// filters/openxml/src/main/java/net/sf/okapi/filters/openxml/
// BlockProperties.java) in combination with RunProperties.Default.
// getEvents (line 580 of RunProperties.java), which together drop both
// the inner empty defRPr and the outer pPr from the round-trip output.
//
// The simple_chart fixture's chart-title paragraph
// <a:p><a:pPr><a:defRPr/></a:pPr><a:r>...</a:r></a:p> demonstrates the
// pattern: gold/Transimple_chart.docx emits just <a:p><a:r>...</a:r></a:p>.
//
// We're matching against captureRawElement's output which always
// expands self-closing tags to open/close form, so the pattern
// recognised is <a:pPr><a:defRPr></a:defRPr></a:pPr> exactly. A
// trailing-whitespace-tolerant version covers the encoding/xml output
// variant.
func isStructurallyEmptyDMLBlockProperties(raw string) bool {
	// Outer element open with no attributes — matches encoding/xml's
	// <a:pPr> (no attribute byte before the closing >).
	const open = "<a:pPr>"
	const close = "</a:pPr>"
	if !strings.HasPrefix(raw, open) || !strings.HasSuffix(raw, close) {
		return false
	}
	inner := raw[len(open) : len(raw)-len(close)]
	inner = strings.TrimSpace(inner)
	// Empty inner means <a:pPr></a:pPr> — drop.
	if inner == "" {
		return true
	}
	// One or more empty defRPr siblings count as structurally empty.
	// captureRawElement emits both <a:defRPr></a:defRPr> form (when the
	// source self-closed) and the literal whitespace-padded variant.
	for inner != "" {
		switch {
		case strings.HasPrefix(inner, "<a:defRPr></a:defRPr>"):
			inner = strings.TrimSpace(inner[len("<a:defRPr></a:defRPr>"):])
		default:
			return false
		}
	}
	return true
}

// Chart and diagram parts in DOCX/PPTX/XLSX use the DrawingML
// chart/diagram schema (ECMA-376-1 §21.2 charts, §21.4 diagrams). Their
// translatable text lives inside DrawingML <a:p> paragraphs which are
// NOT wrapped in <txBody> — chart titles use
//
//	<c:title><c:tx><c:rich><a:p>...</a:p></c:rich></c:tx></c:title>
//
// per ECMA-376-1 §21.2.2.210 (Tx) / §21.2.2.156 (Rich), and SmartArt
// diagram nodes use
//
//	<dgm:pt><dgm:t><a:p>...</a:p></dgm:t></dgm:pt>
//
// per ECMA-376-1 §21.4.5.10 (Pt) / §21.4.5.16 (T). The cached numeric/
// category strings inside <c:strCache><c:pt><c:v>...</c:v> are NOT
// translated by Okapi (they are derived from a separate spreadsheet
// data source). See gold/Transimple_chart.docx where only "Widget
// Sales" (the <c:rich><a:p> title) is translated; cached "1st Qtr"
// etc. are passed through.
//
// Upstream parity: Okapi routes both content types through
// StyledTextPart with an empty ChartFragments wrapper:
//
//	WordDocument.java line 202-203 (Drawing.DIAGRAM_DATA_TYPE,
//	  Drawing.CHART_TYPE → isStyledTextPart)
//	WordDocument.java line 244-252 (StyledTextPart constructor)
//	StyledTextPart.java line 49-100 (paragraph processing)
//
// StyledTextPart processes the entire stream as DrawingML, walking
// <a:p>/<a:r>/<a:t> exactly the same way it walks WordprocessingML
// inside word/document.xml. Empty paragraphs (those containing only
// <a:endParaRPr lang="..."/>) are kept as <a:p/> with the lang-
// stripping skippable elements applied.
//
// Implementation note: the dmlParser already handles <a:p> paragraphs
// correctly (parseParagraph reuses captureRawElement to drop
// endParaRPr inside paragraphs, mergeRuns/isEmptyRuns to identify
// text-bearing vs empty paragraphs). The chart/diagram dispatch just
// needs a freestanding-<a:p>-triggered top-level walk instead of the
// txBody-triggered one used for PPTX slides.

// parseChartOrDiagramPart processes a chart (word/charts/chart*.xml)
// or diagram (word/diagrams/data*.xml) part. It walks the entire XML
// stream emitting most events to skeleton, but recognises freestanding
// <a:p> paragraphs (DrawingML-main namespace, local-name match) and
// runs them through the normal paragraph extractor. Other elements,
// including chart-namespace (<c:...>) and diagram-namespace (<dgm:...>)
// containers, plus chart-specific <c:strCache>/<c:numCache>/<c:txPr>
// blocks, pass through unchanged.
func (p *dmlParser) parseChartOrDiagramPart(data []byte, partPath string, emitBlock func(*model.Block)) error {
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
			// <a:p> at any depth is a translatable paragraph (chart
			// rich-text title, chart axis label, diagram node text,
			// diagram font defaults inside <c:txPr>). Namespace match
			// on the prefix-resolved URI ensures we don't misfire on
			// homonyms (e.g. <c:p> would be in the chart namespace).
			if t.Name.Local == "p" && t.Name.Space == dmlNamespace {
				if err := p.parseParagraph(d, partPath, emitBlock); err != nil {
					return err
				}
				continue
			}
			p.skelWriteStartElement(t)

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
