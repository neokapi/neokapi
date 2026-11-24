package filehome_test

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/change/filehome"
	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/formats/json"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/registry"
)

// writeCatalog writes a JSON object of n messages to path and returns its
// size. The content never lives as one string the test keeps.
func writeCatalog(t *testing.T, path string, n int) int64 {
	t.Helper()
	f, err := os.Create(path)
	require.NoError(t, err)
	bw := bufio.NewWriter(f)
	fmt.Fprint(bw, "{\n")
	for i := range n {
		sep := ","
		if i == n-1 {
			sep = ""
		}
		fmt.Fprintf(bw, "  \"message.%06d\": \"This is message number %d of the catalog, with a few words\"%s\n", i, i, sep)
	}
	fmt.Fprint(bw, "}\n")
	require.NoError(t, bw.Flush())
	require.NoError(t, f.Close())
	info, err := os.Stat(path)
	require.NoError(t, err)
	return info.Size()
}

// TestFileHome_ALargeEditStaysInBoundedMemory edits one message of a
// 100,000-message JSON catalog through the service and asserts the heap the
// edit takes stays under a fixed ceiling, well below the size of the file: the
// reader, the editor and the writer stream, and the service keeps no block it
// did not change.
func TestFileHome_ALargeEditStaysInBoundedMemory(t *testing.T) {
	assertLargeEditBounded(t, "message.050000", "This message was edited", `"message.050000": "This message was edited",`, 24<<20)
}

// An edit that adds a slash and a non-ASCII character waits on how the whole
// document spells them, and the writer holds the output after it while it
// waits. The hold is bounded, so the edit of a large catalog still streams.
func TestFileHome_ALargeEditThatAddsASlashAndAnAccentStaysInBoundedMemory(t *testing.T) {
	// The catalog spells no slash, so the writer's default escapes the added
	// one, and no non-ASCII character, so the added é is UTF-8. The message is
	// early, so nearly the whole catalog follows the waiting value.
	assertLargeEditBounded(t, "message.000100", "This message was édited and/or kept", `"message.000100": "This message was édited and\/or kept",`, 12<<20)
}

// assertLargeEditBounded sets the content of the message at key in a
// 100,000-message catalog to text through the service. The heap the edit takes
// above what was live before it must stay under ceiling; a buffered round trip
// of this catalog takes several times the file. The file must then hold want
// and every other message.
func assertLargeEditBounded(t *testing.T, key, text, want string, ceiling uint64) {
	t.Helper()
	if testing.Short() {
		t.Skip("memory test skipped in -short")
	}
	require.True(t, format.IsStreamingReader(json.NewReader()), "the JSON reader streams")
	require.True(t, format.IsStreamingWriter(json.NewWriter()), "the JSON writer streams")

	const n = 100_000

	dir := t.TempDir()
	size := writeCatalog(t, filepath.Join(dir, "catalog.json"), n)
	reg := newRegistry()
	svc := change.NewService(filehome.Formats{Registry: reg},
		change.OneHome(filehome.New(filehome.DirLayout{Root: dir, Formats: reg, SourceLocale: "en"}, filehome.Options{})))

	ctx := context.Background()
	page, err := svc.Read(ctx, change.ReadRequest{Doc: "catalog.json", Blocks: []string{key}})
	require.NoError(t, err)
	require.Len(t, page.Blocks, 1, "the read finds the block by key")
	b := page.Blocks[0]

	defer debug.SetGCPercent(debug.SetGCPercent(20))
	streams := armGatedJSON(reg)
	var peak uint64
	passes := 0
	sampled := make(chan struct{})
	go func() {
		defer close(sampled)
		for st := range streams {
			passes++
			peak = max(peak, testutil.SamplePeakStops(st.gate, size, 64, st.store, st.done))
		}
	}()
	res, err := svc.Apply(ctx, change.Set{Ops: []change.Op{{
		Kind: change.KindSetContent, At: b.Ref, IfMatch: b.Rev, Body: &change.SetContent{Text: &text},
	}}}, person)
	close(streams)
	<-sampled
	require.Positive(t, passes, "the edit streams the catalog through the gated reader")
	require.NoError(t, err)
	require.Equal(t, change.SetApplied, res.Status, "%+v", res.Ops)

	delta := peak
	t.Logf("catalog %d KiB, %d messages; peak heap above the baseline %d KiB", size/1024, n, delta/1024)
	assert.Less(t, delta, ceiling, "the edit took %d KiB of heap; the ceiling is %d KiB", delta/1024, ceiling/1024)

	got, err := os.ReadFile(filepath.Join(dir, "catalog.json"))
	require.NoError(t, err)
	assert.Contains(t, string(got), want)
	assert.Equal(t, n+2, strings.Count(string(got), "\n"), "every other message is kept")
}

// gatedStream is one streaming pass of the JSON reader into the JSON writer:
// the gate on the reader's input, the skeleton store between the two, and a
// channel that closes when the pass closes its reader.
type gatedStream struct {
	gate  *testutil.Gate
	store *format.SkeletonStore
	done  chan struct{}
}

// armGatedJSON replaces reg's JSON reader with one whose streaming passes
// read through a gate, and returns the channel each such pass is sent on.
// The test samples each pass with SamplePeakStops: the reader stops at the
// gate, the writer catches up on the store, and only then is the heap
// measured. A pass that is not streaming reads its input directly.
func armGatedJSON(reg *registry.FormatRegistry) chan gatedStream {
	streams := make(chan gatedStream)
	reg.RegisterReader("json", func() format.DataFormatReader {
		return &gatedJSONReader{Reader: json.NewReader(), streams: streams}
	}, json.NewReader().Signature(), "JSON")
	return streams
}

type gatedJSONReader struct {
	*json.Reader
	streams chan<- gatedStream
	store   *format.SkeletonStore
	once    sync.Once
	done    chan struct{}
}

func (r *gatedJSONReader) SetSkeletonStore(s *format.SkeletonStore) {
	r.store = s
	r.Reader.SetSkeletonStore(s)
}

func (r *gatedJSONReader) Open(ctx context.Context, doc *model.RawDocument) error {
	if r.store != nil && r.store.IsStreaming() {
		gate := testutil.NewGate(doc.Reader)
		doc.Reader = io.NopCloser(gate)
		r.done = make(chan struct{})
		r.streams <- gatedStream{gate: gate, store: r.store, done: r.done}
	}
	return r.Reader.Open(ctx, doc)
}

func (r *gatedJSONReader) Close() error {
	err := r.Reader.Close()
	if r.done != nil {
		r.once.Do(func() { close(r.done) })
	}
	return err
}
