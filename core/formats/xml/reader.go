package xml

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"path/filepath"
	"slices"
	"strings"

	"github.com/gokapi/gokapi/core/format"
	"github.com/gokapi/gokapi/core/model"
)

// Reader implements DataFormatReader for XML files.
type Reader struct {
	format.BaseFormatReader
	cfg      *Config
	resolver format.SubfilterResolver
	layerSeq int
}

// Ensure Reader implements SubfilterAware.
var _ format.SubfilterAware = (*Reader)(nil)

// NewReader creates a new XML reader.
func NewReader() *Reader {
	cfg := &Config{}
	return &Reader{
		BaseFormatReader: format.BaseFormatReader{
			FormatName:        "xml",
			FormatDisplayName: "XML",
			FormatMimeType:    "text/xml",
			FormatExtensions:  []string{".xml"},
			Cfg:               cfg,
		},
		cfg: cfg,
	}
}

// SetSubfilterResolver sets the resolver for creating sub-format readers.
func (r *Reader) SetSubfilterResolver(resolver format.SubfilterResolver) {
	r.resolver = resolver
}

// SetConfig applies a new configuration.
func (r *Reader) SetConfig(cfg format.DataFormatConfig) error {
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("invalid config: %w", err)
	}
	r.Cfg = cfg
	if c, ok := cfg.(*Config); ok {
		r.cfg = c
	}
	return nil
}

// Signature returns detection metadata for this format.
func (r *Reader) Signature() format.FormatSignature {
	return format.FormatSignature{
		MIMETypes:  []string{"text/xml", "application/xml"},
		Extensions: []string{".xml"},
		MagicBytes: [][]byte{[]byte("<?xml")},
	}
}

// Open opens a RawDocument for reading.
func (r *Reader) Open(ctx context.Context, doc *model.RawDocument) error {
	if doc == nil || doc.Reader == nil {
		return fmt.Errorf("xml: nil document or reader")
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
	locale := r.Doc.SourceLocale
	if locale.IsEmpty() {
		locale = model.LocaleEnglish
	}

	layer := &model.Layer{
		ID:       "doc1",
		Name:     r.Doc.URI,
		Format:   "xml",
		Locale:   locale,
		Encoding: r.Doc.Encoding,
		MimeType: "text/xml",
	}
	if !r.emit(ctx, ch, &model.Part{Type: model.PartLayerStart, Resource: layer}) {
		return
	}

	content, err := io.ReadAll(r.Doc.Reader)
	if err != nil {
		ch <- model.PartResult{Error: fmt.Errorf("xml: reading: %w", err)}
		return
	}

	decoder := xml.NewDecoder(strings.NewReader(string(content)))
	blockCounter := 0
	dataCounter := 0
	var elementStack []string

	for {
		tok, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			ch <- model.PartResult{Error: fmt.Errorf("xml: parsing: %w", err)}
			return
		}

		switch t := tok.(type) {
		case xml.StartElement:
			elementStack = append(elementStack, t.Name.Local)
			// Emit translatable attributes as blocks
			for _, attr := range t.Attr {
				if r.isTranslatableAttr(attr.Name.Local) {
					blockCounter++
					block := model.NewBlock(fmt.Sprintf("tu%d", blockCounter), attr.Value)
					block.Name = strings.Join(elementStack, ".") + "@" + attr.Name.Local
					block.Type = "attribute"
					if !r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block}) {
						return
					}
				}
			}

		case xml.EndElement:
			if len(elementStack) > 0 {
				elementStack = elementStack[:len(elementStack)-1]
			}

		case xml.CharData:
			text := strings.TrimSpace(string(t))
			if text == "" {
				continue
			}
			elemPath := strings.Join(elementStack, ".")
			if r.isTranslatableElement(elementStack) {
				// Check for subfilter match
				if mapping := r.matchSubfilter(elemPath); mapping != nil && r.resolver != nil {
					r.emitSubfiltered(ctx, ch, text, elemPath, layer.ID, mapping, &blockCounter, &dataCounter)
					continue
				}
				blockCounter++
				block := model.NewBlock(fmt.Sprintf("tu%d", blockCounter), text)
				block.Name = elemPath
				if !r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block}) {
					return
				}
			} else {
				dataCounter++
				data := &model.Data{
					ID:   fmt.Sprintf("d%d", dataCounter),
					Name: elemPath,
				}
				if !r.emit(ctx, ch, &model.Part{Type: model.PartData, Resource: data}) {
					return
				}
			}

		case xml.ProcInst:
			dataCounter++
			data := &model.Data{
				ID:   fmt.Sprintf("d%d", dataCounter),
				Name: "processing-instruction",
			}
			r.emit(ctx, ch, &model.Part{Type: model.PartData, Resource: data})

		case xml.Comment:
			dataCounter++
			data := &model.Data{
				ID:   fmt.Sprintf("d%d", dataCounter),
				Name: "comment",
			}
			r.emit(ctx, ch, &model.Part{Type: model.PartData, Resource: data})
		}
	}

	r.emit(ctx, ch, &model.Part{Type: model.PartLayerEnd, Resource: layer})
}

func (r *Reader) isTranslatableElement(stack []string) bool {
	if len(r.cfg.TranslatableElements) == 0 {
		return true // all elements translatable by default
	}
	if len(stack) == 0 {
		return false
	}
	current := stack[len(stack)-1]
	return slices.Contains(r.cfg.TranslatableElements, current)
}

func (r *Reader) isTranslatableAttr(name string) bool {
	return slices.Contains(r.cfg.TranslatableAttributes, name)
}

func (r *Reader) emit(ctx context.Context, ch chan<- model.PartResult, part *model.Part) bool {
	select {
	case ch <- model.PartResult{Part: part}:
		return true
	case <-ctx.Done():
		return false
	}
}

// matchSubfilter checks if the given element path matches any configured subfilter mapping.
func (r *Reader) matchSubfilter(path string) *format.SubfilterMapping {
	for i := range r.cfg.Subfilters {
		sf := &r.cfg.Subfilters[i]
		if matchGlob(sf.Pattern, path) {
			return sf
		}
	}
	return nil
}

// emitSubfiltered emits a child layer with content parsed by the subfilter format reader.
func (r *Reader) emitSubfiltered(ctx context.Context, ch chan<- model.PartResult, content, path, parentLayerID string, mapping *format.SubfilterMapping, blockCounter, dataCounter *int) {
	subReader, err := r.resolver.ResolveReader(mapping.Format)
	if err != nil {
		// Fall back to plain block
		*blockCounter++
		block := model.NewBlock(fmt.Sprintf("tu%d", *blockCounter), content)
		block.Name = path
		r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block})
		return
	}

	r.layerSeq++
	childLayerID := fmt.Sprintf("sf%d", r.layerSeq)

	locale := r.Doc.SourceLocale
	if locale.IsEmpty() {
		locale = model.LocaleEnglish
	}

	childLayer := &model.Layer{
		ID:       childLayerID,
		Name:     path,
		Format:   mapping.Format,
		Locale:   locale,
		ParentID: parentLayerID,
		Properties: map[string]string{
			"subfilter.source":      "xml",
			"subfilter.elementPath": path,
		},
	}

	if !r.emit(ctx, ch, &model.Part{Type: model.PartLayerStart, Resource: childLayer}) {
		return
	}

	subDoc := &model.RawDocument{
		URI:          path,
		SourceLocale: locale,
		Encoding:     "UTF-8",
		Reader:       io.NopCloser(bytes.NewReader([]byte(content))),
	}
	if err := subReader.Open(ctx, subDoc); err != nil {
		ch <- model.PartResult{Error: fmt.Errorf("xml: subfilter open for %s: %w", path, err)}
		r.emit(ctx, ch, &model.Part{Type: model.PartLayerEnd, Resource: childLayer})
		return
	}

	for pr := range subReader.Read(ctx) {
		if pr.Error != nil {
			ch <- model.PartResult{Error: fmt.Errorf("xml: subfilter read for %s: %w", path, pr.Error)}
			break
		}
		// Skip the sub-reader's document-level layer events
		if pr.Part.Type == model.PartLayerStart || pr.Part.Type == model.PartLayerEnd {
			if layer, ok := pr.Part.Resource.(*model.Layer); ok && layer.IsRoot() {
				continue
			}
		}
		r.emit(ctx, ch, pr.Part)
	}
	subReader.Close()

	r.emit(ctx, ch, &model.Part{Type: model.PartLayerEnd, Resource: childLayer})
}

// matchGlob matches a path against a glob pattern using dot-separated segments.
func matchGlob(pattern, path string) bool {
	patternNorm := strings.ReplaceAll(pattern, ".", "/")
	pathNorm := strings.ReplaceAll(path, ".", "/")
	matched, _ := filepath.Match(patternNorm, pathNorm)
	return matched
}

// Close releases resources.
func (r *Reader) Close() error {
	if r.Doc != nil && r.Doc.Reader != nil {
		return r.Doc.Reader.Close()
	}
	return nil
}
