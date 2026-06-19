// Package audio reads audio files as localizable content: when an ASR engine is
// available (the kapi-asr plugin), it transcribes the speech into timing-anchored
// Blocks (AD-030); otherwise the audio is emitted as a Media asset. Like the
// image reader + vision, transcription is PATH-based — the engine opens the file
// itself, so the audio bytes never travel through the part stream.
package audio

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/neokapi/neokapi/core/asr"
	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
)

// Reader implements DataFormatReader for audio files.
type Reader struct {
	format.BaseFormatReader
}

// NewReader creates a new audio reader.
func NewReader() *Reader {
	return &Reader{
		BaseFormatReader: format.BaseFormatReader{
			FormatName:        "audio",
			FormatDisplayName: "Audio",
			FormatMimeType:    "audio/basic",
			FormatExtensions:  []string{".wav", ".mp3", ".m4a", ".aac", ".flac", ".ogg", ".opus"},
		},
	}
}

// Signature returns detection metadata.
func (r *Reader) Signature() format.FormatSignature {
	return format.FormatSignature{
		MIMETypes:  []string{"audio/wav", "audio/mpeg", "audio/mp4", "audio/flac", "audio/ogg"},
		Extensions: []string{".wav", ".mp3", ".m4a", ".aac", ".flac", ".ogg", ".opus"},
		MagicBytes: [][]byte{[]byte("RIFF"), []byte("ID3"), []byte("OggS"), []byte("fLaC")},
	}
}

// Open prepares the document for reading.
func (r *Reader) Open(_ context.Context, doc *model.RawDocument) error {
	if doc == nil || doc.Reader == nil {
		return errors.New("audio: nil document or reader")
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
	path, cleanup, err := r.materialize()
	if err != nil {
		ch <- model.PartResult{Error: err}
		return
	}
	defer cleanup()

	layer := &model.Layer{
		ID:       "doc1",
		Name:     r.Doc.URI,
		Format:   "audio",
		Locale:   locale,
		MimeType: "audio/basic",
	}
	if !r.emit(ctx, ch, &model.Part{Type: model.PartLayerStart, Resource: layer}) {
		return
	}

	if asr.Available("") {
		if !r.transcribe(ctx, ch, path, locale) {
			return
		}
	} else {
		// No ASR engine: the audio is still a localizable Media asset.
		uri := r.Doc.URI
		if uri == "" {
			uri = path
		}
		media := &model.Media{
			ID:       "m1",
			URI:      uri,
			MimeType: r.Doc.MimeType,
			Filename: filepath.Base(uri),
		}
		if !r.emit(ctx, ch, &model.Part{Type: model.PartMedia, Resource: media}) {
			return
		}
	}

	r.emit(ctx, ch, &model.Part{Type: model.PartLayerEnd, Resource: layer})
}

// transcribe runs the registered ASR engine over the audio at path and emits one
// timing-anchored Block per recognized segment.
func (r *Reader) transcribe(ctx context.Context, ch chan<- model.PartResult, path string, locale model.LocaleID) bool {
	eng, err := asr.Open("")
	if err != nil {
		ch <- model.PartResult{Error: fmt.Errorf("audio: open asr engine: %w", err)}
		return false
	}
	defer func() { _ = eng.Close() }()

	res, err := eng.Transcribe(ctx, path, asr.Options{Lang: locale.String()})
	if err != nil {
		ch <- model.PartResult{Error: fmt.Errorf("audio: transcribe: %w", err)}
		return false
	}
	if res == nil {
		return true
	}
	counter := 0
	for _, b := range asr.BlocksFromASR(res, &counter) {
		if !r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: b}) {
			return false
		}
	}
	return true
}

// materialize returns a local filesystem path to the audio. If the source URI is
// already a local file it is used directly; otherwise the reader streams
// doc.Reader to a temp file (never holding the whole track in memory).
func (r *Reader) materialize() (path string, cleanup func(), err error) {
	noop := func() {}
	if r.Doc.URI != "" {
		if info, statErr := os.Stat(r.Doc.URI); statErr == nil && !info.IsDir() {
			return r.Doc.URI, noop, nil
		}
	}
	ext := ".audio"
	if r.Doc.URI != "" {
		if e := filepath.Ext(r.Doc.URI); e != "" {
			ext = e
		}
	}
	tmp, err := os.CreateTemp("", "kapi-audio-*"+ext)
	if err != nil {
		return "", noop, fmt.Errorf("audio: temp file: %w", err)
	}
	if _, err := io.Copy(tmp, r.Doc.Reader); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
		return "", noop, fmt.Errorf("audio: spool: %w", err)
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
