package xliff

import (
	"context"
	"encoding/xml"
	"fmt"

	"github.com/gokapi/gokapi/core/format"
	"github.com/gokapi/gokapi/core/model"
)

// Writer implements DataFormatWriter for XLIFF 1.2 files.
type Writer struct {
	format.BaseFormatWriter
	blocks     []*model.Block
	sourceLang model.LocaleID
	targetLang model.LocaleID
	fileName   string
}

// NewWriter creates a new XLIFF 1.2 writer.
func NewWriter() *Writer {
	return &Writer{
		BaseFormatWriter: format.BaseFormatWriter{
			FormatName: "xliff",
		},
	}
}

// Write consumes Parts from a channel and writes XLIFF 1.2 output.
func (w *Writer) Write(ctx context.Context, parts <-chan *model.Part) error {
	// Collect all parts first
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case part, ok := <-parts:
			if !ok {
				return w.flush()
			}
			switch part.Type {
			case model.PartBlock:
				if block, ok := part.Resource.(*model.Block); ok {
					w.blocks = append(w.blocks, block)
				}
			case model.PartLayerStart:
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

func (w *Writer) flush() error {
	if w.Output == nil {
		return nil
	}

	targetLang := w.targetLang
	if !w.Locale.IsEmpty() {
		targetLang = w.Locale
	}

	type xmlTarget struct {
		XMLName xml.Name `xml:"target"`
		Content string   `xml:",chardata"`
	}
	type xmlSource struct {
		XMLName xml.Name `xml:"source"`
		Content string   `xml:",chardata"`
	}
	type xmlTransUnit struct {
		XMLName xml.Name   `xml:"trans-unit"`
		ID      string     `xml:"id,attr"`
		Source  xmlSource  `xml:"source"`
		Target  *xmlTarget `xml:"target,omitempty"`
	}
	type xmlBody struct {
		XMLName    xml.Name       `xml:"body"`
		TransUnits []xmlTransUnit `xml:"trans-unit"`
	}
	type xmlFile struct {
		XMLName    xml.Name `xml:"file"`
		Original   string   `xml:"original,attr"`
		SourceLang string   `xml:"source-language,attr"`
		TargetLang string   `xml:"target-language,attr,omitempty"`
		Datatype   string   `xml:"datatype,attr"`
		Body       xmlBody  `xml:"body"`
	}
	type xmlXliff struct {
		XMLName xml.Name  `xml:"xliff"`
		Version string    `xml:"version,attr"`
		Xmlns   string    `xml:"xmlns,attr"`
		Files   []xmlFile `xml:"file"`
	}

	var transUnits []xmlTransUnit
	for _, block := range w.blocks {
		tu := xmlTransUnit{
			ID:     block.ID,
			Source: xmlSource{Content: block.SourceText()},
		}
		if block.HasTarget(targetLang) {
			tu.Target = &xmlTarget{Content: block.TargetText(targetLang)}
		}
		transUnits = append(transUnits, tu)
	}

	doc := xmlXliff{
		Version: "1.2",
		Xmlns:   "urn:oasis:names:tc:xliff:document:1.2",
		Files: []xmlFile{
			{
				Original:   w.fileName,
				SourceLang: string(w.sourceLang),
				TargetLang: string(targetLang),
				Datatype:   "plaintext",
				Body:       xmlBody{TransUnits: transUnits},
			},
		},
	}

	fmt.Fprint(w.Output, xml.Header)
	encoder := xml.NewEncoder(w.Output)
	encoder.Indent("", "  ")
	if err := encoder.Encode(doc); err != nil {
		return fmt.Errorf("xliff writer: encoding: %w", err)
	}
	return encoder.Flush()
}
