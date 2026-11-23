// Package audio reads audio files as localizable content: when an ASR engine is
// available (the kapi-asr plugin), it transcribes the speech into timing-anchored
// Blocks (AD-030). The audio is emitted as a Media asset either way. Like the
// image reader with vision, transcription is path-based: the engine opens a
// private copy itself, so the audio bytes never travel through the part stream.
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
		FormatName:        "audio",
		FormatDisplayName: "Audio",
		FormatMimeType:    "audio/basic",
		FormatExtensions:  []string{".wav", ".mp3", ".m4a", ".aac", ".flac", ".ogg", ".opus"},
	}
}

// Signature returns detection metadata.
func (r *Reader) Signature() format.FormatSignature {
	return format.FormatSignature{
		MIMETypes:  []string{"audio/wav", "audio/mpeg", "audio/mp4", "audio/flac", "audio/ogg"},
		Extensions: []string{".wav", ".mp3", ".m4a", ".aac", ".flac", ".ogg", ".opus"},
		MagicBytes: [][]byte{[]byte("RIFF"), []byte("ID3"), []byte("OggS"), []byte("fLaC")},
		Binary:     true,
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
	return format.StreamParts(ctx, func(ctx context.Context, ch chan<- model.PartResult) error {
		r.readContent(ctx, ch)
		return nil
	})
}

func (r *Reader) readContent(ctx context.Context, ch chan<- model.PartResult) {
	locale := r.Doc.SourceLocale
	if locale.IsEmpty() {
		locale = model.LocaleEnglish
	}
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

	// The audio itself, as the writer copies it. It is emitted whether or not
	// an ASR engine transcribes it: a document read where an engine is
	// installed is written back as one read where it is not.
	uri := r.Doc.URI
	if uri == "" {
		uri, _ = format.SourceFile(r.Doc)
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
	if asr.Available("") {
		path, cleanup, err := r.materialize()
		if err != nil {
			ch <- model.PartResult{Error: err}
			return
		}
		ok := r.transcribe(ctx, ch, path, locale)
		cleanup()
		if !ok {
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

// materialize returns the path of a private copy of the audio for the ASR
// engine, which reads a path. The engine gets a temp file this process
// created and wrote from the descriptor the caller opened (doc.ReaderAt, else
// doc.Reader), never the source's own name. That name lies in the project, and
// a link put in its place after the caller opened it would lead the engine to
// any file. The copy streams, never holding the whole track in memory.
func (r *Reader) materialize() (path string, cleanup func(), err error) {
	noop := func() {}
	src := io.Reader(r.Doc.Reader)
	if sec, ok := format.ReopenSource(r.Doc); ok {
		src = sec
	}
	ext := ".audio"
	if r.Doc.URI != "" {
		if e := filepath.Ext(r.Doc.URI); e != "" {
			ext = e
		}
	}
	//pathguard: creates a new file in the OS temp dir
	tmp, err := os.CreateTemp("", "kapi-audio-*"+ext)
	if err != nil {
		return "", noop, fmt.Errorf("audio: temp file: %w", err)
	}
	if _, err := io.Copy(tmp, src); err != nil {
		_ = tmp.Close()
		//pathguard: removes the temp file the reader created with os.CreateTemp
		_ = os.Remove(tmp.Name())
		return "", noop, fmt.Errorf("audio: spool: %w", err)
	}
	// Checked on the success path too. io.Copy returning nil does not mean the
	// bytes reached the disk: a write it buffered can still fail at Close (a
	// full filesystem, a quota, a failing network mount). Discarding that error
	// hands ffmpeg a silently TRUNCATED file, and the fault resurfaces as a
	// decode error that names the wrong cause, far from where it happened.
	if err := tmp.Close(); err != nil {
		//pathguard: removes the temp file the reader created with os.CreateTemp
		_ = os.Remove(tmp.Name())
		return "", noop, fmt.Errorf("audio: spool: close: %w", err)
	}
	//pathguard: removes the temp file the reader created with os.CreateTemp
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
