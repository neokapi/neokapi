package tmx

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"strings"

	"github.com/asgeirf/gokapi/core/format"
	"github.com/asgeirf/gokapi/core/model"
)

// Reader implements DataFormatReader for TMX (Translation Memory eXchange) files.
type Reader struct {
	format.BaseFormatReader
	cfg *Config
}

// NewReader creates a new TMX reader.
func NewReader() *Reader {
	cfg := &Config{}
	return &Reader{
		BaseFormatReader: format.BaseFormatReader{
			FormatName:        "tmx",
			FormatDisplayName: "TMX",
			FormatMimeType:    "application/x-tmx+xml",
			FormatExtensions:  []string{".tmx"},
			Cfg:               cfg,
		},
		cfg: cfg,
	}
}

// Signature returns detection metadata for this format.
func (r *Reader) Signature() format.FormatSignature {
	return format.FormatSignature{
		MIMETypes:  []string{"application/x-tmx+xml"},
		Extensions: []string{".tmx"},
	}
}

// Open opens a RawDocument for reading.
func (r *Reader) Open(ctx context.Context, doc *model.RawDocument) error {
	if doc == nil || doc.Reader == nil {
		return fmt.Errorf("tmx: nil document or reader")
	}
	r.Doc = doc
	return nil
}

// Read returns a channel of PartResults.
func (r *Reader) Read(ctx context.Context) <-chan model.PartResult {
	ch := make(chan model.PartResult, 64)
	go func() {
		defer close(ch)
		r.readContent(ctx, ch)
	}()
	return ch
}

// TMX XML structures
type tmxDocument struct {
	XMLName xml.Name  `xml:"tmx"`
	Version string    `xml:"version,attr"`
	Header  tmxHeader `xml:"header"`
	Body    tmxBody   `xml:"body"`
}

type tmxHeader struct {
	CreationTool        string `xml:"creationtool,attr"`
	CreationToolVersion string `xml:"creationtoolversion,attr"`
	SegType             string `xml:"segtype,attr"`
	OriginalFormat      string `xml:"o-tmf,attr"`
	AdminLang           string `xml:"adminlang,attr"`
	SrcLang             string `xml:"srclang,attr"`
	DataType            string `xml:"datatype,attr"`
}

type tmxBody struct {
	TUs []tmxTU `xml:"tu"`
}

type tmxTU struct {
	TUid  string    `xml:"tuid,attr"`
	Props []tmxProp `xml:"prop"`
	TUVs  []tmxTUV  `xml:"tuv"`
	Notes []tmxNote `xml:"note"`
}

type tmxTUV struct {
	Lang string `xml:"lang,attr"`
	Seg  string `xml:"seg"`
}

type tmxProp struct {
	Type  string `xml:"type,attr"`
	Value string `xml:",chardata"`
}

type tmxNote struct {
	Value string `xml:",chardata"`
}

func (r *Reader) readContent(ctx context.Context, ch chan<- model.PartResult) {
	locale := r.Doc.SourceLocale
	if locale.IsEmpty() {
		locale = model.LocaleEnglish
	}

	layer := &model.Layer{
		ID:             "doc1",
		Name:           r.Doc.URI,
		Format:         "tmx",
		Locale:         locale,
		Encoding:       r.Doc.Encoding,
		MimeType:       "application/x-tmx+xml",
		IsMultilingual: true,
	}
	if !r.emit(ctx, ch, &model.Part{Type: model.PartLayerStart, Resource: layer}) {
		return
	}

	content, err := io.ReadAll(r.Doc.Reader)
	if err != nil {
		ch <- model.PartResult{Error: fmt.Errorf("tmx: reading: %w", err)}
		return
	}

	var doc tmxDocument
	if err := xml.Unmarshal(content, &doc); err != nil {
		ch <- model.PartResult{Error: fmt.Errorf("tmx: parsing: %w", err)}
		return
	}

	// Emit header metadata as Data
	dataCounter := 0
	dataCounter++
	headerData := &model.Data{
		ID:   fmt.Sprintf("d%d", dataCounter),
		Name: "tmx-header",
		Properties: map[string]string{
			"version":      doc.Version,
			"srclang":      doc.Header.SrcLang,
			"adminlang":    doc.Header.AdminLang,
			"datatype":     doc.Header.DataType,
			"segtype":      doc.Header.SegType,
			"o-tmf":        doc.Header.OriginalFormat,
			"creationtool": doc.Header.CreationTool,
		},
	}
	if !r.emit(ctx, ch, &model.Part{Type: model.PartData, Resource: headerData}) {
		return
	}

	// Determine source language from header
	srcLang := strings.ToLower(doc.Header.SrcLang)
	if srcLang == "" {
		srcLang = string(locale)
	}

	blockCounter := 0

	for _, tu := range doc.Body.TUs {
		blockCounter++

		// Find source TUV
		var sourceText string
		for _, tuv := range tu.TUVs {
			tuvLang := strings.ToLower(tuv.Lang)
			if tuvLang == srcLang || strings.HasPrefix(tuvLang, srcLang+"-") || strings.HasPrefix(srcLang, tuvLang+"-") {
				sourceText = tuv.Seg
				break
			}
		}

		// If no source found by language match, use the first TUV
		if sourceText == "" && len(tu.TUVs) > 0 {
			sourceText = tu.TUVs[0].Seg
		}

		tuID := tu.TUid
		if tuID == "" {
			tuID = fmt.Sprintf("tu%d", blockCounter)
		}

		block := model.NewBlock(tuID, sourceText)
		block.Name = tuID

		// Store properties
		for _, prop := range tu.Props {
			block.Properties[prop.Type] = prop.Value
		}

		// Store notes
		if len(tu.Notes) > 0 {
			var notes []string
			for _, note := range tu.Notes {
				notes = append(notes, note.Value)
			}
			block.Properties["notes"] = strings.Join(notes, "\n")
		}

		// Add target translations from other TUVs
		for _, tuv := range tu.TUVs {
			tuvLang := strings.ToLower(tuv.Lang)
			if tuvLang == srcLang || strings.HasPrefix(tuvLang, srcLang+"-") || strings.HasPrefix(srcLang, tuvLang+"-") {
				continue
			}
			block.SetTargetText(model.LocaleID(tuv.Lang), tuv.Seg)
		}

		if !r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block}) {
			return
		}
	}

	r.emit(ctx, ch, &model.Part{Type: model.PartLayerEnd, Resource: layer})
}

func (r *Reader) emit(ctx context.Context, ch chan<- model.PartResult, part *model.Part) bool {
	select {
	case ch <- model.PartResult{Part: part}:
		return true
	case <-ctx.Done():
		return false
	}
}

// Close releases resources.
func (r *Reader) Close() error {
	if r.Doc != nil && r.Doc.Reader != nil {
		return r.Doc.Reader.Close()
	}
	return nil
}
