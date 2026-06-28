// Package video reads video files by demuxing them (core/av, via ffmpeg) into
// the two streams a localization flow extracts from (AD-030): the audio track →
// transcribed to timing-anchored Blocks by the kapi-asr engine, and sampled,
// deduplicated frames → OCR'd to spatially+temporally anchored Blocks by the
// kapi-vision engine. Each is emitted under its own child Layer. Engines that
// aren't installed are skipped, so the reader degrades gracefully.
package video

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/neokapi/neokapi/core/asr"
	"github.com/neokapi/neokapi/core/av"
	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/vision"
)

// Reader implements DataFormatReader for video files.
type Reader struct {
	format.BaseFormatReader
}

// NewReader creates a new video reader.
func NewReader() *Reader {
	return &Reader{
		BaseFormatReader: format.BaseFormatReader{
			FormatName:        "video",
			FormatDisplayName: "Video",
			FormatMimeType:    "video/mp4",
			FormatExtensions:  []string{".mp4", ".mov", ".m4v", ".mkv", ".webm", ".avi"},
		},
	}
}

// Signature returns detection metadata.
func (r *Reader) Signature() format.FormatSignature {
	return format.FormatSignature{
		MIMETypes:  []string{"video/mp4", "video/quicktime", "video/x-matroska", "video/webm"},
		Extensions: []string{".mp4", ".mov", ".m4v", ".mkv", ".webm", ".avi"},
		Binary:     true, // video containers are binary
	}
}

// Open prepares the document.
func (r *Reader) Open(_ context.Context, doc *model.RawDocument) error {
	if doc == nil || doc.Reader == nil {
		return errors.New("video: nil document or reader")
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
	videoPath, cleanup, err := r.materialize()
	if err != nil {
		ch <- model.PartResult{Error: err}
		return
	}
	defer cleanup()

	if !av.FFmpegAvailable() {
		// No ffmpeg/av engine: the video cannot be demuxed, but it is still a
		// localizable Media asset (replace-asset mode). Emit it as opaque Media
		// rather than erroring, mirroring the audio reader's no-engine path.
		uri := r.Doc.URI
		if uri == "" {
			uri = videoPath
		}
		media := &model.Media{
			ID:       "m1",
			URI:      uri,
			MimeType: r.Doc.MimeType,
			Filename: filepath.Base(uri),
		}
		r.emit(ctx, ch, &model.Part{Type: model.PartMedia, Resource: media})
		return
	}

	work, err := os.MkdirTemp("", "kapi-video-*")
	if err != nil {
		ch <- model.PartResult{Error: fmt.Errorf("video: work dir: %w", err)}
		return
	}
	defer func() { _ = os.RemoveAll(work) }()

	res, err := av.Demux(ctx, videoPath, work, av.Options{})
	if err != nil {
		ch <- model.PartResult{Error: fmt.Errorf("video: demux: %w", err)}
		return
	}

	doc := &model.Layer{ID: "doc1", Name: r.Doc.URI, Format: "video", Locale: locale, MimeType: "video/mp4"}
	if !r.emit(ctx, ch, &model.Part{Type: model.PartLayerStart, Resource: doc}) {
		return
	}

	counter := 0
	if res.HasAudio && res.AudioPath != "" && asr.Available("") {
		if !r.transcribe(ctx, ch, res.AudioPath, locale, &counter) {
			return
		}
	}
	if len(res.Frames) > 0 && vision.Available("") {
		if !r.ocrFrames(ctx, ch, res.Frames, locale, &counter) {
			return
		}
	}

	r.emit(ctx, ch, &model.Part{Type: model.PartLayerEnd, Resource: doc})
}

// transcribe emits the audio track as a child Layer of timing-anchored Blocks.
func (r *Reader) transcribe(ctx context.Context, ch chan<- model.PartResult, audioPath string, locale model.LocaleID, counter *int) bool {
	eng, err := asr.Open("")
	if err != nil {
		ch <- model.PartResult{Error: fmt.Errorf("video: asr: %w", err)}
		return false
	}
	defer func() { _ = eng.Close() }()
	out, err := eng.Transcribe(ctx, audioPath, asr.Options{Lang: locale.String()})
	if err != nil {
		ch <- model.PartResult{Error: fmt.Errorf("video: transcribe: %w", err)}
		return false
	}
	if out == nil {
		return true
	}
	layer := &model.Layer{ID: "audio", Name: "audio", Format: "audio", Locale: locale, ParentID: "doc1"}
	if !r.emit(ctx, ch, &model.Part{Type: model.PartLayerStart, Resource: layer}) {
		return false
	}
	for _, b := range asr.BlocksFromASR(out, counter) {
		if !r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: b}) {
			return false
		}
	}
	return r.emit(ctx, ch, &model.Part{Type: model.PartLayerEnd, Resource: layer})
}

// ocrFrames emits on-screen text as a child Layer of Blocks carrying both the
// spatial (geometry) anchor from OCR and the temporal (timing) anchor of the
// frame they were read from — the composite anchor of AD-002/AD-030.
func (r *Reader) ocrFrames(ctx context.Context, ch chan<- model.PartResult, frames []av.Frame, locale model.LocaleID, counter *int) bool {
	eng, err := vision.Open("")
	if err != nil {
		ch <- model.PartResult{Error: fmt.Errorf("video: vision: %w", err)}
		return false
	}
	defer func() { _ = eng.Close() }()

	layer := &model.Layer{ID: "frames", Name: "frames", Format: "image", Locale: locale, ParentID: "doc1"}
	if !r.emit(ctx, ch, &model.Part{Type: model.PartLayerStart, Resource: layer}) {
		return false
	}
	for _, f := range frames {
		ocr, err := eng.OCR(ctx, f.Path, vision.OCROptions{Lang: locale.String()})
		if err != nil || ocr == nil {
			continue // a single frame failing must not abort the stream
		}
		for _, b := range vision.BlocksFromOCR(ocr, 1, counter) {
			// Stamp the frame's time onto the (geometry-anchored) OCR block.
			b.SetTiming(&model.TimingAnnotation{StartMS: f.TimeMS, EndMS: f.TimeMS})
			if !r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: b}) {
				return false
			}
		}
	}
	return r.emit(ctx, ch, &model.Part{Type: model.PartLayerEnd, Resource: layer})
}

func (r *Reader) materialize() (path string, cleanup func(), err error) {
	noop := func() {}
	if r.Doc.URI != "" {
		if info, statErr := os.Stat(r.Doc.URI); statErr == nil && !info.IsDir() {
			return r.Doc.URI, noop, nil
		}
	}
	ext := ".mp4"
	if r.Doc.URI != "" {
		if e := filepath.Ext(r.Doc.URI); e != "" {
			ext = e
		}
	}
	tmp, err := os.CreateTemp("", "kapi-video-src-*"+ext)
	if err != nil {
		return "", noop, fmt.Errorf("video: temp file: %w", err)
	}
	if _, err := io.Copy(tmp, r.Doc.Reader); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
		return "", noop, fmt.Errorf("video: spool: %w", err)
	}
	_ = tmp.Close()
	return tmp.Name(), func() { _ = os.Remove(tmp.Name()) }, nil
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
