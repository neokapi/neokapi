package audio

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
)

// Writer emits an audio document: it writes the (first) Media part's bytes to
// the output. This is the whole-audio-localization sink — a localized audio
// track (a per-locale replacement the user or a connector supplies) is written
// out as-is, mirroring the image writer. The Media's inline Data is preferred;
// if it carries only a URI reference, the source file is copied. Transcription
// Blocks (when an ASR engine ran) carry no replacement audio, so they pass
// through without affecting the emitted bytes.
type Writer struct {
	format.BaseFormatWriter
}

// NewWriter constructs an audio writer.
func NewWriter() *Writer {
	return &Writer{BaseFormatWriter: format.BaseFormatWriter{FormatName: "audio"}}
}

// Write consumes the part stream and writes the first Media part's audio bytes.
func (w *Writer) Write(ctx context.Context, parts <-chan *model.Part) error {
	var media *model.Media
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case p, ok := <-parts:
			if !ok {
				if media == nil {
					return errors.New("audio: no media part to write")
				}
				return w.writeMedia(media)
			}
			if p == nil {
				continue
			}
			if m, ok := p.Resource.(*model.Media); ok && media == nil {
				media = m // first Media (the audio track) wins
			}
		}
	}
}

func (w *Writer) writeMedia(m *model.Media) error {
	if w.Output == nil {
		return errors.New("audio: no output configured")
	}
	data := m.Data
	if len(data) == 0 && m.URI != "" {
		b, err := os.ReadFile(m.URI)
		if err != nil {
			return fmt.Errorf("audio: read source %s: %w", m.URI, err)
		}
		data = b
	}
	if len(data) == 0 {
		return errors.New("audio: media part has no bytes")
	}
	_, err := w.Output.Write(data)
	return err
}
