package xliff2

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"strings"

	"github.com/asgeirf/gokapi/core/format"
	"github.com/asgeirf/gokapi/core/model"
)

// XLIFF 2.0 XML structures

type xliff2Doc struct {
	XMLName  xml.Name     `xml:"xliff"`
	Version  string       `xml:"version,attr"`
	SrcLang  string       `xml:"srcLang,attr"`
	TrgLang  string       `xml:"trgLang,attr"`
	Files    []xliff2File `xml:"file"`
}

type xliff2File struct {
	ID    string       `xml:"id,attr"`
	Units []xliff2Unit `xml:"unit"`
}

type xliff2Unit struct {
	ID       string           `xml:"id,attr"`
	Name     string           `xml:"name,attr"`
	Notes    []xliff2Note     `xml:"notes>note"`
	Segments []xliff2Segment  `xml:"segment"`
}

type xliff2Segment struct {
	ID     string        `xml:"id,attr"`
	Source xliff2Content `xml:"source"`
	Target xliff2Content `xml:"target"`
}

type xliff2Note struct {
	Content string `xml:",chardata"`
}

type xliff2Content struct {
	InnerXML string `xml:",innerxml"`
}

// Reader implements DataFormatReader for XLIFF 2.0 files.
type Reader struct {
	format.BaseFormatReader
	cfg *Config
}

// NewReader creates a new XLIFF 2.0 reader.
func NewReader() *Reader {
	cfg := &Config{}
	return &Reader{
		BaseFormatReader: format.BaseFormatReader{
			FormatName:        "xliff2",
			FormatDisplayName: "XLIFF 2.0",
			FormatMimeType:    "application/xliff+xml",
			FormatExtensions:  []string{".xlf", ".xliff"},
			Cfg:               cfg,
		},
		cfg: cfg,
	}
}

// Signature returns detection metadata for this format.
func (r *Reader) Signature() format.FormatSignature {
	return format.FormatSignature{
		MIMETypes:  []string{"application/xliff+xml"},
		Extensions: []string{".xlf", ".xliff"},
		Sniff: func(data []byte) bool {
			s := string(data)
			return strings.Contains(s, "<xliff") && strings.Contains(s, "version=\"2")
		},
	}
}

// Open opens a RawDocument for reading.
func (r *Reader) Open(ctx context.Context, doc *model.RawDocument) error {
	if doc == nil || doc.Reader == nil {
		return fmt.Errorf("xliff2: nil document or reader")
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

func (r *Reader) readContent(ctx context.Context, ch chan<- model.PartResult) {
	content, err := io.ReadAll(r.Doc.Reader)
	if err != nil {
		ch <- model.PartResult{Error: fmt.Errorf("xliff2: reading: %w", err)}
		return
	}

	var doc xliff2Doc
	if err := xml.Unmarshal(content, &doc); err != nil {
		ch <- model.PartResult{Error: fmt.Errorf("xliff2: parsing: %w", err)}
		return
	}

	srcLang := model.LocaleID(doc.SrcLang)
	trgLang := model.LocaleID(doc.TrgLang)

	for _, file := range doc.Files {
		layer := &model.Layer{
			ID:             fmt.Sprintf("file-%s", file.ID),
			Name:           file.ID,
			Format:         "xliff2",
			Locale:         srcLang,
			IsMultilingual: true,
			Properties: map[string]string{
				"target-language": string(trgLang),
			},
		}
		if !r.emit(ctx, ch, &model.Part{Type: model.PartLayerStart, Resource: layer}) {
			return
		}

		for _, unit := range file.Units {
			r.emitUnit(ctx, ch, unit, srcLang, trgLang)
		}

		r.emit(ctx, ch, &model.Part{Type: model.PartLayerEnd, Resource: layer})
	}
}

func (r *Reader) emitUnit(ctx context.Context, ch chan<- model.PartResult, unit xliff2Unit, srcLang, trgLang model.LocaleID) {
	// Build source and target segments from the unit's segments
	var sourceSegs []*model.Segment
	targets := make(map[model.LocaleID][]*model.Segment)

	for _, seg := range unit.Segments {
		segID := seg.ID
		if segID == "" {
			segID = fmt.Sprintf("s%d", len(sourceSegs)+1)
		}

		sourceText := strings.TrimSpace(seg.Source.InnerXML)
		sourceSegs = append(sourceSegs, &model.Segment{
			ID:      segID,
			Content: model.NewFragment(sourceText),
		})

		targetText := strings.TrimSpace(seg.Target.InnerXML)
		if targetText != "" && !trgLang.IsEmpty() {
			targets[trgLang] = append(targets[trgLang], &model.Segment{
				ID:      segID,
				Content: model.NewFragment(targetText),
			})
		}
	}

	block := &model.Block{
		ID:           unit.ID,
		Name:         unit.Name,
		Translatable: true,
		Source:       sourceSegs,
		Targets:      targets,
		Properties:   make(map[string]string),
		Annotations:  make(map[string]model.Annotation),
	}

	// Add notes as properties
	for i, note := range unit.Notes {
		block.Properties[fmt.Sprintf("note-%d", i)] = note.Content
	}

	r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block})
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
