package vtt

import (
	"bufio"
	"context"
	"fmt"
	"strings"

	"github.com/gokapi/gokapi/core/format"
	"github.com/gokapi/gokapi/core/model"
)

// Reader implements DataFormatReader for WebVTT subtitle files.
type Reader struct {
	format.BaseFormatReader
	cfg *Config
}

// NewReader creates a new VTT reader.
func NewReader() *Reader {
	cfg := &Config{}
	return &Reader{
		BaseFormatReader: format.BaseFormatReader{
			FormatName:        "vtt",
			FormatDisplayName: "WebVTT",
			FormatMimeType:    "text/vtt",
			FormatExtensions:  []string{".vtt"},
			Cfg:               cfg,
		},
		cfg: cfg,
	}
}

// Signature returns detection metadata for this format.
func (r *Reader) Signature() format.FormatSignature {
	return format.FormatSignature{
		MIMETypes:  []string{"text/vtt"},
		Extensions: []string{".vtt"},
		MagicBytes: [][]byte{[]byte("WEBVTT")},
	}
}

// Open opens a RawDocument for reading.
func (r *Reader) Open(ctx context.Context, doc *model.RawDocument) error {
	if doc == nil || doc.Reader == nil {
		return fmt.Errorf("vtt: nil document or reader")
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

// vttCue represents a single VTT cue (subtitle entry).
type vttCue struct {
	identifier string
	timecode   string
	text       string
}

func (r *Reader) readContent(ctx context.Context, ch chan<- model.PartResult) {
	locale := r.Doc.SourceLocale
	if locale.IsEmpty() {
		locale = model.LocaleEnglish
	}

	layer := &model.Layer{
		ID:       "doc1",
		Name:     r.Doc.URI,
		Format:   "vtt",
		Locale:   locale,
		Encoding: r.Doc.Encoding,
		MimeType: "text/vtt",
	}
	if !r.emit(ctx, ch, &model.Part{Type: model.PartLayerStart, Resource: layer}) {
		return
	}

	cues, header := r.parseCues()

	dataCounter := 0

	// Emit WEBVTT header as Data
	dataCounter++
	headerData := &model.Data{
		ID:   fmt.Sprintf("d%d", dataCounter),
		Name: "vtt-header",
		Properties: map[string]string{
			"content": header,
		},
	}
	if !r.emit(ctx, ch, &model.Part{Type: model.PartData, Resource: headerData}) {
		return
	}

	blockCounter := 0

	for i, cue := range cues {
		// Emit cue identifier as Data if present
		if cue.identifier != "" {
			dataCounter++
			idData := &model.Data{
				ID:   fmt.Sprintf("d%d", dataCounter),
				Name: fmt.Sprintf("cue-id.%d", i+1),
				Properties: map[string]string{
					"identifier": cue.identifier,
				},
			}
			if !r.emit(ctx, ch, &model.Part{Type: model.PartData, Resource: idData}) {
				return
			}
		}

		// Emit cue text as Block
		blockCounter++
		block := model.NewBlock(fmt.Sprintf("tu%d", blockCounter), cue.text)
		block.Name = fmt.Sprintf("subtitle.%d", i+1)
		block.Properties["timecode"] = cue.timecode
		if cue.identifier != "" {
			block.Properties["cue-id"] = cue.identifier
		}
		block.Properties["index"] = fmt.Sprintf("%d", i+1)
		if !r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block}) {
			return
		}
	}

	r.emit(ctx, ch, &model.Part{Type: model.PartLayerEnd, Resource: layer})
}

func (r *Reader) parseCues() ([]*vttCue, string) {
	scanner := bufio.NewScanner(r.Doc.Reader)
	var cues []*vttCue
	header := ""

	// Read the WEBVTT header line
	if scanner.Scan() {
		header = strings.TrimRight(scanner.Text(), "\r")
	}

	// Skip blank lines after header
	for scanner.Scan() {
		line := strings.TrimRight(scanner.Text(), "\r")
		if strings.TrimSpace(line) != "" {
			// This is the start of the first cue
			cue := r.parseCue(scanner, line)
			if cue != nil {
				cues = append(cues, cue)
			}
			break
		}
	}

	// Parse remaining cues
	for scanner.Scan() {
		line := strings.TrimRight(scanner.Text(), "\r")
		if strings.TrimSpace(line) == "" {
			continue
		}
		cue := r.parseCue(scanner, line)
		if cue != nil {
			cues = append(cues, cue)
		}
	}

	return cues, header
}

// parseCue parses a single VTT cue starting from the given first non-empty line.
func (r *Reader) parseCue(scanner *bufio.Scanner, firstLine string) *vttCue {
	cue := &vttCue{}

	// Determine if the first line is a timecode or a cue identifier
	if isTimecode(firstLine) {
		cue.timecode = firstLine
	} else {
		// It's a cue identifier
		cue.identifier = firstLine
		// Next line should be the timecode
		if scanner.Scan() {
			cue.timecode = strings.TrimRight(scanner.Text(), "\r")
		}
	}

	// Read text lines until blank line or EOF
	var textLines []string
	for scanner.Scan() {
		line := strings.TrimRight(scanner.Text(), "\r")
		if strings.TrimSpace(line) == "" {
			break
		}
		textLines = append(textLines, line)
	}

	cue.text = strings.Join(textLines, "\n")
	return cue
}

// isTimecode returns true if the line looks like a VTT timecode line.
func isTimecode(line string) bool {
	return strings.Contains(line, "-->")
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
