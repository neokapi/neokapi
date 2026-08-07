package flow

import (
	"context"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeReader serves a fixed Part list and counts its own closes, so a test can
// assert the runner released it exactly once.
type fakeReader struct {
	format.BaseFormatReader
	parts  []*model.Part
	closes atomic.Int32
	skel   *format.SkeletonStore
}

func newFakeReader(parts ...*model.Part) *fakeReader {
	r := &fakeReader{parts: parts}
	r.FormatName = "fake"
	r.FormatDisplayName = "Fake"
	return r
}

func (r *fakeReader) Signature() format.FormatSignature {
	return format.FormatSignature{Extensions: []string{".fake"}}
}

func (r *fakeReader) Open(_ context.Context, doc *model.RawDocument) error {
	r.Doc = doc
	return nil
}

func (r *fakeReader) Read(ctx context.Context) <-chan model.PartResult {
	ch := make(chan model.PartResult)
	go func() {
		defer close(ch)
		for _, p := range r.parts {
			select {
			case ch <- model.PartResult{Part: p}:
			case <-ctx.Done():
				return
			}
		}
	}()
	return ch
}

func (r *fakeReader) Close() error {
	r.closes.Add(1)
	return nil
}

func (r *fakeReader) SetSkeletonStore(s *format.SkeletonStore) { r.skel = s }

// streamingFakeReader declares the streaming capability, so the runner feeds it
// concurrently with the writer.
type streamingFakeReader struct{ *fakeReader }

func (streamingFakeReader) StreamingReader() {}

// fakeWriter counts the parts it consumed and records what the runner handed it.
type fakeWriter struct {
	format.BaseFormatWriter
	written atomic.Int32
	skel    *format.SkeletonStore
	content []byte
}

func newFakeWriter() *fakeWriter {
	w := &fakeWriter{}
	w.FormatName = "fake"
	return w
}

func (w *fakeWriter) Write(_ context.Context, parts <-chan *model.Part) error {
	for range parts {
		w.written.Add(1)
	}
	return nil
}

// skeletonFakeWriter consumes a skeleton; streamingSkeletonFakeWriter also
// declares it can consume one interleaved with the Part stream.
type skeletonFakeWriter struct{ *fakeWriter }

func (w skeletonFakeWriter) SetSkeletonStore(s *format.SkeletonStore) { w.fakeWriter.skel = s }

type streamingSkeletonFakeWriter struct{ skeletonFakeWriter }

func (streamingSkeletonFakeWriter) StreamingWriter() {}

// contentSkeletonFakeWriter needs the original bytes (as OpenXML and AsciiDoc
// do), which forces the source to be materialized up front and so forces the
// buffered feed — while still declaring every streaming capability.
type contentSkeletonFakeWriter struct{ streamingSkeletonFakeWriter }

func (w contentSkeletonFakeWriter) SetOriginalContent(b []byte) { w.fakeWriter.content = b }

// fakeTools is a one-step pass-through chain, the minimum a flow needs.
func fakeTools() []tool.Tool { return []tool.Tool{newPassThroughTool("pass")} }

func fakeBlockPart(id string) *model.Part {
	return &model.Part{Type: model.PartBlock, Resource: model.NewBlock(id, "Hello")}
}

func writeFakeInput(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "input.fake")
	require.NoError(t, os.WriteFile(path, []byte("source"), 0o600))
	return path
}

// TestFileRunner_RejectedDestinationClosesStreamingReader covers the reader
// opened before the pipeline is built: every destination check that rejects the
// run happens after that open, so the runner — not the feeder that never ran —
// has to release it.
func TestFileRunner_RejectedDestinationClosesStreamingReader(t *testing.T) {
	tests := []struct {
		name       string
		outputPath func(t *testing.T) string
	}{
		{
			name: "destination is a directory",
			outputPath: func(t *testing.T) string {
				t.Helper()
				return t.TempDir()
			},
		},
		{
			name: "output directory is not writable",
			outputPath: func(t *testing.T) string {
				t.Helper()
				dir := filepath.Join(t.TempDir(), "readonly")
				require.NoError(t, os.Mkdir(dir, 0o500))
				t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })
				return filepath.Join(dir, "out.fake")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := newFakeReader(fakeBlockPart("b1"))
			runner := NewFileRunner(FileRunnerConfig{SourceLocale: "en"})

			err := runner.RunFileWithReaderWriter(t.Context(), "test", nil,
				writeFakeInput(t), tt.outputPath(t), "fr",
				streamingFakeReader{reader}, newFakeWriter())

			require.Error(t, err)
			assert.Equal(t, int32(1), reader.closes.Load(), "the open reader must be closed when the run is rejected")
		})
	}
}

// TestFileRunner_StreamingReaderClosedOnceOnSuccess pins the other half of the
// contract: the abort path must not double-close a reader the feeder already
// released.
func TestFileRunner_StreamingReaderClosedOnceOnSuccess(t *testing.T) {
	reader := newFakeReader(fakeBlockPart("b1"))
	writer := newFakeWriter()
	runner := NewFileRunner(FileRunnerConfig{SourceLocale: "en"})

	outputPath := filepath.Join(t.TempDir(), "out.fake")
	require.NoError(t, runner.RunFileWithReaderWriter(t.Context(), "test", fakeTools(),
		writeFakeInput(t), outputPath, "fr", streamingFakeReader{reader}, writer))

	assert.Equal(t, int32(1), reader.closes.Load())
	assert.Equal(t, int32(1), writer.written.Load())
}

// TestWireSkeleton_StreamingBackingFollowsTheFeed pins the pairing rule: a
// channel-backed store blocks the writer inside Next() until the reader's feeder
// calls CloseWrite, so it may only be created for a run that actually feeds
// concurrently.
func TestWireSkeleton_StreamingBackingFollowsTheFeed(t *testing.T) {
	base := newFakeWriter()
	tests := []struct {
		name          string
		writer        format.DataFormatWriter
		streaming     bool
		wantStore     bool
		wantStreaming bool
	}{
		{name: "streaming feed, streaming writer", writer: streamingSkeletonFakeWriter{skeletonFakeWriter{base}}, streaming: true, wantStore: true, wantStreaming: true},
		{name: "buffered feed, streaming writer", writer: streamingSkeletonFakeWriter{skeletonFakeWriter{base}}, streaming: false, wantStore: true, wantStreaming: false},
		{name: "streaming feed, buffered writer", writer: skeletonFakeWriter{base}, streaming: true, wantStore: true, wantStreaming: false},
		{name: "writer consumes no skeleton", writer: base, streaming: true, wantStore: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store, err := wireSkeleton(newFakeReader(), tt.writer, tt.streaming)
			require.NoError(t, err)
			if !tt.wantStore {
				assert.Nil(t, store)
				return
			}
			require.NotNil(t, store)
			defer store.Close()
			assert.Equal(t, tt.wantStreaming, store.IsStreaming())
		})
	}
}

// TestFileRunner_BufferedFeedNeverGetsStreamingSkeleton drives the combination
// the pairing rule exists for: a reader and writer that both declare streaming,
// but a writer that also needs the original bytes — which forces the buffered
// feed. The skeleton store must follow the feed, not the capabilities.
func TestFileRunner_BufferedFeedNeverGetsStreamingSkeleton(t *testing.T) {
	reader := newFakeReader(fakeBlockPart("b1"))
	base := newFakeWriter()
	writer := contentSkeletonFakeWriter{streamingSkeletonFakeWriter{skeletonFakeWriter{base}}}
	runner := NewFileRunner(FileRunnerConfig{SourceLocale: "en"})

	outputPath := filepath.Join(t.TempDir(), "out.fake")
	require.NoError(t, runner.RunFileWithReaderWriter(t.Context(), "test", fakeTools(),
		writeFakeInput(t), outputPath, "fr", streamingFakeReader{reader}, writer))

	require.NotNil(t, base.skel, "a same-format skeleton pair must be wired")
	assert.False(t, base.skel.IsStreaming(), "a buffered feed must not be paired with a channel-backed skeleton store")
	assert.Equal(t, []byte("source"), base.content)
}

// TestFileRunner_StreamingFeedGetsStreamingSkeleton is the positive companion:
// when nothing forces the source to be materialized, both ends stream and the
// skeleton store is the channel-backed one.
func TestFileRunner_StreamingFeedGetsStreamingSkeleton(t *testing.T) {
	reader := newFakeReader(fakeBlockPart("b1"))
	base := newFakeWriter()
	writer := streamingSkeletonFakeWriter{skeletonFakeWriter{base}}
	runner := NewFileRunner(FileRunnerConfig{SourceLocale: "en"})

	outputPath := filepath.Join(t.TempDir(), "out.fake")
	require.NoError(t, runner.RunFileWithReaderWriter(t.Context(), "test", fakeTools(),
		writeFakeInput(t), outputPath, "fr", streamingFakeReader{reader}, writer))

	require.NotNil(t, base.skel)
	assert.True(t, base.skel.IsStreaming())
}
