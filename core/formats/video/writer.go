package video

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
)

// Writer emits a video document: it writes the (first) Media part's bytes to
// the output. This is the whole-video-localization sink — a localized video (a
// per-locale replacement the user or a connector supplies) is written out as-is,
// mirroring the image and audio writers. The Media's inline Data is preferred;
// if it carries only a URI reference, the source file is copied. The demuxed
// frame/audio Layers and their timing-anchored Blocks carry no replacement
// video, so they pass through without affecting the emitted bytes.
type Writer struct {
	format.BaseFormatWriter
}

// NewWriter constructs a video writer.
func NewWriter() *Writer {
	return &Writer{BaseFormatWriter: format.BaseFormatWriter{FormatName: "video"}}
}

// Write consumes the part stream and writes the first Media part's video bytes.
func (w *Writer) Write(ctx context.Context, parts <-chan *model.Part) error {
	var media *model.Media
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case p, ok := <-parts:
			if !ok {
				if media == nil {
					return errors.New("video: no media part to write")
				}
				return w.writeMedia(media)
			}
			if p == nil {
				continue
			}
			if m, ok := p.Resource.(*model.Media); ok && media == nil {
				media = m // first Media (the video file) wins
			}
		}
	}
}

func (w *Writer) writeMedia(m *model.Media) error {
	if w.Output == nil {
		return errors.New("video: no output configured")
	}
	data := m.Data
	if len(data) == 0 && m.URI != "" {
		b, err := os.ReadFile(m.URI)
		if err != nil {
			return fmt.Errorf("video: read source %s: %w", m.URI, err)
		}
		data = b
	}
	if len(data) == 0 {
		return errors.New("video: media part has no bytes")
	}
	_, err := w.Output.Write(data)
	return err
}
