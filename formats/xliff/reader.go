package xliff

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"strings"

	"github.com/gokapi/gokapi/core/format"
	"github.com/gokapi/gokapi/core/model"
)

// XLIFF 1.2 XML structures

type xliffDoc struct {
	XMLName xml.Name    `xml:"xliff"`
	Version string      `xml:"version,attr"`
	Files   []xliffFile `xml:"file"`
}

type xliffFile struct {
	Original   string    `xml:"original,attr"`
	SourceLang string    `xml:"source-language,attr"`
	TargetLang string    `xml:"target-language,attr"`
	Datatype   string    `xml:"datatype,attr"`
	Body       xliffBody `xml:"body"`
}

type xliffBody struct {
	TransUnits []transUnit  `xml:"trans-unit"`
	Groups     []xliffGroup `xml:"group"`
}

type xliffGroup struct {
	ID         string      `xml:"id,attr"`
	TransUnits []transUnit `xml:"trans-unit"`
}

type transUnit struct {
	ID     string       `xml:"id,attr"`
	Source xliffContent `xml:"source"`
	Target xliffContent `xml:"target"`
	Note   string       `xml:"note"`
}

type xliffContent struct {
	InnerXML string `xml:",innerxml"`
}

// Reader implements DataFormatReader for XLIFF 1.2 files.
type Reader struct {
	format.BaseFormatReader
	cfg *Config
}

// NewReader creates a new XLIFF 1.2 reader.
func NewReader() *Reader {
	cfg := &Config{}
	return &Reader{
		BaseFormatReader: format.BaseFormatReader{
			FormatName:        "xliff",
			FormatDisplayName: "XLIFF 1.2",
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
		MIMETypes:  []string{"application/xliff+xml", "application/x-xliff+xml"},
		Extensions: []string{".xlf", ".xliff"},
		Sniff: func(data []byte) bool {
			s := string(data)
			return strings.Contains(s, "<xliff") && strings.Contains(s, "urn:oasis:names:tc:xliff:document:1")
		},
	}
}

// Open opens a RawDocument for reading.
func (r *Reader) Open(ctx context.Context, doc *model.RawDocument) error {
	if doc == nil || doc.Reader == nil {
		return fmt.Errorf("xliff: nil document or reader")
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
		ch <- model.PartResult{Error: fmt.Errorf("xliff: reading: %w", err)}
		return
	}

	var doc xliffDoc
	if err := xml.Unmarshal(content, &doc); err != nil {
		ch <- model.PartResult{Error: fmt.Errorf("xliff: parsing: %w", err)}
		return
	}

	for _, file := range doc.Files {
		sourceLang := model.LocaleID(file.SourceLang)
		targetLang := model.LocaleID(file.TargetLang)

		layer := &model.Layer{
			ID:             fmt.Sprintf("file-%s", file.Original),
			Name:           file.Original,
			Format:         "xliff",
			Locale:         sourceLang,
			IsMultilingual: true,
			Properties: map[string]string{
				"datatype":        file.Datatype,
				"target-language": string(targetLang),
			},
		}
		if !r.emit(ctx, ch, &model.Part{Type: model.PartLayerStart, Resource: layer}) {
			return
		}

		// Process groups
		for _, group := range file.Body.Groups {
			gs := &model.GroupStart{ID: group.ID, Name: group.ID}
			if !r.emit(ctx, ch, &model.Part{Type: model.PartGroupStart, Resource: gs}) {
				return
			}
			for _, tu := range group.TransUnits {
				r.emitTransUnit(ctx, ch, tu, sourceLang, targetLang)
			}
			ge := &model.GroupEnd{ID: group.ID}
			if !r.emit(ctx, ch, &model.Part{Type: model.PartGroupEnd, Resource: ge}) {
				return
			}
		}

		// Process top-level trans-units
		for _, tu := range file.Body.TransUnits {
			r.emitTransUnit(ctx, ch, tu, sourceLang, targetLang)
		}

		r.emit(ctx, ch, &model.Part{Type: model.PartLayerEnd, Resource: layer})
	}
}

func (r *Reader) emitTransUnit(ctx context.Context, ch chan<- model.PartResult, tu transUnit, sourceLang, targetLang model.LocaleID) {
	sourceText := strings.TrimSpace(tu.Source.InnerXML)
	targetText := strings.TrimSpace(tu.Target.InnerXML)

	block := &model.Block{
		ID:           tu.ID,
		Name:         tu.ID,
		Translatable: true,
		Source:       []*model.Segment{{ID: "s1", Content: model.NewFragment(sourceText)}},
		Targets:      make(map[model.LocaleID][]*model.Segment),
		Properties:   make(map[string]string),
		Annotations:  make(map[string]model.Annotation),
	}

	if targetText != "" && !targetLang.IsEmpty() {
		block.Targets[targetLang] = []*model.Segment{{ID: "s1", Content: model.NewFragment(targetText)}}
	}

	if tu.Note != "" {
		block.Properties["note"] = tu.Note
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
