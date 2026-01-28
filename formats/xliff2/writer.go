package xliff2

import (
	"context"
	"encoding/xml"
	"fmt"

	"github.com/asgeirf/gokapi/core/format"
	"github.com/asgeirf/gokapi/core/model"
)

// Writer implements DataFormatWriter for XLIFF 2.0 files.
type Writer struct {
	format.BaseFormatWriter
	blocks     []*model.Block
	sourceLang model.LocaleID
	targetLang model.LocaleID
	fileID     string
}

// NewWriter creates a new XLIFF 2.0 writer.
func NewWriter() *Writer {
	return &Writer{
		BaseFormatWriter: format.BaseFormatWriter{
			FormatName: "xliff2",
		},
	}
}

// Write consumes Parts from a channel and writes XLIFF 2.0 output.
func (w *Writer) Write(ctx context.Context, parts <-chan *model.Part) error {
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
					w.fileID = layer.Name
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
	type xmlSegment struct {
		XMLName xml.Name   `xml:"segment"`
		ID      string     `xml:"id,attr,omitempty"`
		Source  xmlSource  `xml:"source"`
		Target  *xmlTarget `xml:"target,omitempty"`
	}
	type xmlUnit struct {
		XMLName  xml.Name     `xml:"unit"`
		ID       string       `xml:"id,attr"`
		Name     string       `xml:"name,attr,omitempty"`
		Segments []xmlSegment `xml:"segment"`
	}
	type xmlFile struct {
		XMLName xml.Name  `xml:"file"`
		ID      string    `xml:"id,attr"`
		Units   []xmlUnit `xml:"unit"`
	}
	type xmlXliff struct {
		XMLName xml.Name  `xml:"xliff"`
		Version string    `xml:"version,attr"`
		Xmlns   string    `xml:"xmlns,attr"`
		SrcLang string    `xml:"srcLang,attr"`
		TrgLang string    `xml:"trgLang,attr,omitempty"`
		Files   []xmlFile `xml:"file"`
	}

	var units []xmlUnit
	for _, block := range w.blocks {
		var segments []xmlSegment
		for _, seg := range block.Source {
			s := xmlSegment{
				ID:     seg.ID,
				Source: xmlSource{Content: seg.Content.Text()},
			}
			// Check for target in the block's Targets map
			if trgSegs, ok := block.Targets[targetLang]; ok {
				for _, ts := range trgSegs {
					if ts.ID == seg.ID {
						s.Target = &xmlTarget{Content: ts.Content.Text()}
						break
					}
				}
			}
			segments = append(segments, s)
		}
		units = append(units, xmlUnit{
			ID:       block.ID,
			Name:     block.Name,
			Segments: segments,
		})
	}

	doc := xmlXliff{
		Version: "2.0",
		Xmlns:   "urn:oasis:names:tc:xliff:document:2.0",
		SrcLang: string(w.sourceLang),
		TrgLang: string(targetLang),
		Files: []xmlFile{
			{
				ID:    w.fileID,
				Units: units,
			},
		},
	}

	fmt.Fprint(w.Output, xml.Header)
	encoder := xml.NewEncoder(w.Output)
	encoder.Indent("", "  ")
	if err := encoder.Encode(doc); err != nil {
		return fmt.Errorf("xliff2 writer: encoding: %w", err)
	}
	return encoder.Flush()
}
