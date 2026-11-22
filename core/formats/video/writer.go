package video

import (
	"context"
	"errors"
	"fmt"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/rootpath"
)

// Writer emits a video document: it writes the (first) Media part's bytes to
// the output. This is the whole-video sink: a localized video (a
// per-locale replacement the user or a connector supplies) is written out as-is,
// mirroring the image and audio writers. The Media's inline Data is preferred;
// if it carries only a URI reference, the source file is copied. The demuxed
// frame/audio Layers and their timing-anchored Blocks carry no replacement
// video, so they pass through without affecting the emitted bytes.
type Writer struct {
	format.BaseFormatWriter

	// root is the project directory, when the run has one (SetProjectRoot).
	root string
}

// NewWriter constructs a video writer.
func NewWriter() *Writer {
	return &Writer{FormatName: "video", Binary: true}
}

// SetProjectRoot sets the project directory. A Media URI is then read only
// when it lies beneath it.
func (w *Writer) SetProjectRoot(root string) { w.root = root }

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
		// The URI is content: within a project it must name a file beneath it.
		b, err := rootpath.ReadFileWithin(w.root, m.URI)
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
