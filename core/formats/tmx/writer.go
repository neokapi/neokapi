package tmx

import (
	"context"
	"encoding/xml"
	"fmt"

	"github.com/gokapi/gokapi/core/format"
	"github.com/gokapi/gokapi/core/model"
)

// Writer implements DataFormatWriter for TMX files.
type Writer struct {
	format.BaseFormatWriter
	headerProps map[string]string
	blocks      []*model.Block
}

// NewWriter creates a new TMX writer.
func NewWriter() *Writer {
	return &Writer{
		BaseFormatWriter: format.BaseFormatWriter{
			FormatName: "tmx",
		},
		headerProps: make(map[string]string),
	}
}

// Write consumes Parts from a channel and writes TMX XML.
func (w *Writer) Write(ctx context.Context, parts <-chan *model.Part) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case part, ok := <-parts:
			if !ok {
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
	Seg  string `xml:"seg"`
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
			Seg:  block.SourceText(),
		})

		// Add target TUVs
		for locale, segs := range block.Targets {
			if len(segs) == 0 {
				continue
			}
			text := block.TargetText(locale)
			tu.TUVs = append(tu.TUVs, xmlTUV{
				Lang: string(locale),
				Seg:  text,
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

