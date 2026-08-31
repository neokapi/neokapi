package format

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These tests guard the seam that seven call sites used to open by hand and then
// discard the error from (#1468 item 1). A skeleton is what carries the structure
// a reader could not model; when a same-format pair has one and the store cannot
// be created, the writer re-serializes from the content model and the original
// formatting is lost — so the failure must surface, never degrade.
//
// The failure is induced by a real filesystem condition: TMPDIR pointed at a
// path that does not exist, which is what os.CreateTemp consults. No production
// hook, no test-only branch.

// brokenTempDir points TMPDIR at a non-existent directory for the duration of
// the test, so os.CreateTemp("") fails the way a read-only or missing TMPDIR
// makes it fail in the field. It returns the path so assertions can name it.
// t.TempDir() must be called before the env is swapped (it uses TMPDIR itself).
func brokenTempDir(t *testing.T) string {
	t.Helper()
	missing := filepath.Join(t.TempDir(), "no-such-tmpdir")
	t.Setenv("TMPDIR", missing)
	// Belt and braces for the platforms os.TempDir consults differently.
	t.Setenv("TMP", missing)
	t.Setenv("TEMP", missing)
	require.Equal(t, missing, os.TempDir(), "sanity: os.TempDir must follow the swapped TMPDIR")
	_, statErr := os.Stat(missing)
	require.True(t, os.IsNotExist(statErr), "sanity: the temp dir must genuinely not exist")
	return missing
}

type streamingStubReader struct {
	stubReader
}

func (s *streamingStubReader) StreamingReader() {}

type streamingStubWriter struct {
	stubWriter
}

func (s *streamingStubWriter) StreamingWriter() {}

// plainReader is a reader that does NOT implement SkeletonStoreEmitter, so the
// pair genuinely has no skeleton path — the documented
// reconstruct-from-the-content-model case.
type plainReader struct {
	BaseFormatReader
}

func (r *plainReader) Signature() FormatSignature                     { return FormatSignature{} }
func (r *plainReader) Open(context.Context, *model.RawDocument) error { return nil }
func (r *plainReader) Read(context.Context) <-chan model.PartResult   { return nil }
func (r *plainReader) Close() error                                   { return nil }

// plainWriter is a writer that does NOT implement SkeletonStoreConsumer.
type plainWriter struct {
	BaseFormatWriter
}

func (w *plainWriter) Write(context.Context, <-chan *model.Part) error { return nil }

// TestNewSkeletonStore_ReportsAnUnusableTempDir is the root of item 1: the
// constructor used to answer an unusable TMPDIR with an in-memory store and a nil
// error on EVERY platform, which made its own error unreachable — and so made the
// seven callers that discarded it impossible to catch. On a platform that has a
// temp filesystem, a temp file that cannot be created is an operational fault.
func TestNewSkeletonStore_ReportsAnUnusableTempDir(t *testing.T) {
	missing := brokenTempDir(t)

	store, err := NewSkeletonStore()
	require.Error(t, err, "an unusable TMPDIR must be reported, not absorbed into an in-memory store")
	assert.Nil(t, store)
	assert.Contains(t, err.Error(), missing, "the error must name the directory it could not write to")
	assert.Contains(t, err.Error(), "TMPDIR", "the error must state the remedy")
}

// TestNewSkeletonStore_HealthyTempDirStillWorks is the control for the test
// above: the constructor is not simply strict, it succeeds whenever a temp
// filesystem is usable.
func TestNewSkeletonStore_HealthyTempDirStillWorks(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())
	store, err := NewSkeletonStore()
	require.NoError(t, err)
	require.NotNil(t, store)
	t.Cleanup(func() { _ = store.Close() })

	store.WriteText([]byte("<p>"))
	b, err := store.Bytes()
	require.NoError(t, err)
	assert.NotEmpty(t, b, "a healthy store still records entries")
}

// TestNewWiredSkeleton_StoreFailureIsAnError is the regression for the swallow.
// A same-format emitter→consumer pair HAS a skeleton path, so a store it cannot
// create is a fault: the error must name what would be lost, not be discarded in
// favour of an unwired writer.
func TestNewWiredSkeleton_StoreFailureIsAnError(t *testing.T) {
	brokenTempDir(t)

	r := &stubReader{FormatName: "openxml"}
	w := &stubWriter{FormatName: "openxml"}

	store, err := NewWiredSkeleton(r, w)
	require.Error(t, err, "a skeleton the pair needs and cannot get must fail, not silently reconstruct")
	assert.Nil(t, store)
	assert.Contains(t, err.Error(), "openxml", "the error must name the round-trip that lost its skeleton")
	assert.Contains(t, err.Error(), "formatting", "the error must say what is lost")
	assert.Nil(t, r.wired, "no half-wired reader may be left pointing at a store that does not exist")
	assert.Nil(t, w.wired)
}

// TestNewWiredSkeleton_NoSkeletonPathIsNotAnError is the discriminating control
// that keeps "fail on any TMPDIR problem" from being an acceptable substitute.
// Absent and present-but-unobtainable are different facts: a pair with no
// skeleton path at all reconstructs from the content model by design, and must
// keep working with the very same broken TMPDIR that fails the case above.
func TestNewWiredSkeleton_NoSkeletonPathIsNotAnError(t *testing.T) {
	brokenTempDir(t)

	cases := []struct {
		name   string
		reader DataFormatReader
		writer DataFormatWriter
	}{
		{
			name:   "reader emits no skeleton",
			reader: &plainReader{FormatName: "json"},
			writer: &stubWriter{FormatName: "json"},
		},
		{
			name:   "writer consumes no skeleton",
			reader: &stubReader{FormatName: "json"},
			writer: &plainWriter{FormatName: "json"},
		},
		{
			name:   "cross-format conversion",
			reader: &stubReader{FormatName: "openxml"},
			writer: &stubWriter{FormatName: "markdown"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store, err := NewWiredSkeleton(tc.reader, tc.writer)
			require.NoError(t, err, "a pair with no skeleton path must not be failed by an unusable TMPDIR")
			assert.Nil(t, store, "and it gets no store: the writer reconstructs from the content model")
		})
	}
}

// TestNewWiredStreamingSkeleton_NeedsNoTempFile is the second discriminating
// control: the channel-backed store touches no temp file, so a caller that
// drives the streaming protocol must be unaffected by a broken TMPDIR. Failing
// it would be strictness for its own sake.
func TestNewWiredStreamingSkeleton_NeedsNoTempFile(t *testing.T) {
	brokenTempDir(t)

	r := &streamingStubReader{stubReader{FormatName: "json"}}
	w := &streamingStubWriter{stubWriter{FormatName: "json"}}

	store := NewWiredStreamingSkeleton(r, w)
	require.NotNil(t, store)
	t.Cleanup(func() { _ = store.Close() })

	assert.True(t, store.IsStreaming(), "the streaming caller gets the channel-backed store")
	assert.Same(t, store, r.wired, "reader wired")
	assert.Same(t, store, w.wired, "writer wired")
	assert.Equal(t, "json", store.OriginFormat(), "the store is origin-tagged so a foreign writer cannot consume it")

	assert.Nil(t, NewWiredStreamingSkeleton(&plainReader{FormatName: "json"}, w),
		"a pair with no skeleton path still gets no store")
}

// TestNewWiredSkeleton_NeverReturnsAStreamingStore pins the contract that keeps
// a buffering caller from deadlocking: NewWiredSkeleton is temp-file-backed even
// when the pair happens to be streaming-capable. A streaming store blocks the
// writer in Next() until the reader's feeder calls CloseWrite, which only
// core/flow's concurrent protocol does — handing one to `kapi ksed -i` (which
// buffers every part, then writes) hangs the command forever.
func TestNewWiredSkeleton_NeverReturnsAStreamingStore(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())

	r := &streamingStubReader{stubReader{FormatName: "json"}}
	w := &streamingStubWriter{stubWriter{FormatName: "json"}}

	store, err := NewWiredSkeleton(r, w)
	require.NoError(t, err)
	require.NotNil(t, store)
	t.Cleanup(func() { _ = store.Close() })

	assert.False(t, store.IsStreaming(),
		"a buffering caller must never be handed the channel-backed store")
}

// TestNewWiredSkeleton_HealthyPairIsWiredAndTagged is the positive control for
// the buffered path: a same-format pair on a working temp filesystem is wired at
// both ends and origin-tagged, exactly as the hand-rolled call sites did.
func TestNewWiredSkeleton_HealthyPairIsWiredAndTagged(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())

	r := &stubReader{FormatName: "html"}
	w := &stubWriter{FormatName: "html"}

	store, err := NewWiredSkeleton(r, w)
	require.NoError(t, err)
	require.NotNil(t, store)
	t.Cleanup(func() { _ = store.Close() })

	assert.False(t, store.IsStreaming(), "a non-streaming pair gets the file-backed store")
	assert.Same(t, store, r.wired)
	assert.Same(t, store, w.wired)
	assert.Equal(t, "html", store.OriginFormat())
}

// TestSkeletonPairEligible pins the "absent" side of the distinction on its own,
// so the predicate cannot drift into treating a foreign writer as eligible.
func TestSkeletonPairEligible(t *testing.T) {
	emit := &stubReader{FormatName: "xml"}
	consume := &stubWriter{FormatName: "xml"}

	assert.True(t, SkeletonPairEligible(emit, consume))
	assert.False(t, SkeletonPairEligible(&plainReader{FormatName: "xml"}, consume))
	assert.False(t, SkeletonPairEligible(emit, &plainWriter{FormatName: "xml"}))
	assert.False(t, SkeletonPairEligible(emit, &stubWriter{FormatName: "markdown"}))
	assert.False(t, SkeletonPairEligible(nil, consume))
	assert.False(t, SkeletonPairEligible(emit, nil))
}
