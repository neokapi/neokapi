package srt

import (
	"bufio"
	"context"
	"fmt"
	"strings"

	"github.com/gokapi/gokapi/format"
	"github.com/gokapi/gokapi/model"
)

// Reader implements DataFormatReader for SRT subtitle files.
type Reader struct {
	format.BaseFormatReader
	cfg *Config
}

// NewReader creates a new SRT reader.
func NewReader() *Reader {
	cfg := &Config{}
	return &Reader{
		BaseFormatReader: format.BaseFormatReader{
			FormatName:        "srt",
			FormatDisplayName: "SRT Subtitles",
			FormatMimeType:    "application/x-subrip",
			FormatExtensions:  []string{".srt"},
			Cfg:               cfg,
		},
		cfg: cfg,
	}
}

// Signature returns detection metadata for this format.
func (r *Reader) Signature() format.FormatSignature {
	return format.FormatSignature{
		MIMETypes:  []string{"application/x-subrip", "text/srt"},
		Extensions: []string{".srt"},
	}
}

// Open opens a RawDocument for reading.
func (r *Reader) Open(ctx context.Context, doc *model.RawDocument) error {
	if doc == nil || doc.Reader == nil {
		return fmt.Errorf("srt: nil document or reader")
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

// srtEntry represents a single subtitle entry.
type srtEntry struct {
	sequence string
	timecode string
	text     string
}

func (r *Reader) readContent(ctx context.Context, ch chan<- model.PartResult) {
	locale := r.Doc.SourceLocale
	if locale.IsEmpty() {
		locale = model.LocaleEnglish
	}

	layer := &model.Layer{
		ID:       "doc1",
		Name:     r.Doc.URI,
		Format:   "srt",
		Locale:   locale,
		Encoding: r.Doc.Encoding,
		MimeType: "application/x-subrip",
	}
	if !r.emit(ctx, ch, &model.Part{Type: model.PartLayerStart, Resource: layer}) {
		return
	}

	entries := r.parseEntries()

	blockCounter := 0
	dataCounter := 0

	for _, entry := range entries {
		// Emit sequence number as Data
		dataCounter++
		seqData := &model.Data{
			ID:   fmt.Sprintf("d%d", dataCounter),
			Name: fmt.Sprintf("sequence.%s", entry.sequence),
			Properties: map[string]string{
				"sequence": entry.sequence,
			},
		}
		if !r.emit(ctx, ch, &model.Part{Type: model.PartData, Resource: seqData}) {
			return
		}

		// Emit subtitle text as Block
		blockCounter++
		block := model.NewBlock(fmt.Sprintf("tu%d", blockCounter), entry.text)
		block.Name = fmt.Sprintf("subtitle.%s", entry.sequence)
		block.Properties["timecode"] = entry.timecode
		block.Properties["sequence"] = entry.sequence
		if !r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block}) {
			return
		}
	}

	r.emit(ctx, ch, &model.Part{Type: model.PartLayerEnd, Resource: layer})
}

func (r *Reader) parseEntries() []*srtEntry {
	scanner := bufio.NewScanner(r.Doc.Reader)
	var entries []*srtEntry

	type state int
	const (
		stateSequence state = iota
		stateTimecode
		stateText
	)

	current := &srtEntry{}
	st := stateSequence
	var textLines []string

	for scanner.Scan() {
		line := scanner.Text()
		line = strings.TrimRight(line, "\r")

		switch st {
		case stateSequence:
			trimmed := strings.TrimSpace(line)
			if trimmed == "" {
				continue
			}
			current.sequence = trimmed
			st = stateTimecode

		case stateTimecode:
			current.timecode = strings.TrimSpace(line)
			st = stateText
			textLines = nil

		case stateText:
			trimmed := strings.TrimSpace(line)
			if trimmed == "" {
				// End of entry
				current.text = strings.Join(textLines, "\n")
				entries = append(entries, current)
				current = &srtEntry{}
				st = stateSequence
			} else {
				textLines = append(textLines, line)
			}
		}
	}

	// Don't forget the last entry if file doesn't end with blank line
	if st == stateText && len(textLines) > 0 {
		current.text = strings.Join(textLines, "\n")
		entries = append(entries, current)
	}

	return entries
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
