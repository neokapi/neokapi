package filehome_test

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/change/filehome"
	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/formats/json"
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
	if testing.Short() {
		t.Skip("memory test skipped in -short")
	}
	require.True(t, format.IsStreamingReader(json.NewReader()), "the JSON reader streams")
	require.True(t, format.IsStreamingWriter(json.NewWriter()), "the JSON writer streams")

	const n = 100_000
	// ceiling is the most heap the edit may take above what was live before
	// it. A buffered round trip of this catalog takes several times the file.
	const ceiling = 24 << 20

	dir := t.TempDir()
	size := writeCatalog(t, filepath.Join(dir, "catalog.json"), n)
	reg := newRegistry()
	svc := change.NewService(filehome.Formats{Registry: reg},
		change.OneHome(filehome.New(filehome.DirLayout{Root: dir, Formats: reg, SourceLocale: "en"}, filehome.Options{LockDir: t.TempDir()})))

	ctx := context.Background()
	page, err := svc.Read(ctx, change.ReadRequest{Doc: "catalog.json", Blocks: []string{"message.050000"}})
	require.NoError(t, err)
	require.Len(t, page.Blocks, 1, "the read finds the block by key")
	b := page.Blocks[0]
	text := "This message was edited"

	defer debug.SetGCPercent(debug.SetGCPercent(20))
	runtime.GC()
	var base runtime.MemStats
	runtime.ReadMemStats(&base)
	var peak atomic.Uint64
	stop, done := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(done)
		var m runtime.MemStats
		for {
			select {
			case <-stop:
				return
			default:
			}
			runtime.ReadMemStats(&m)
			if m.HeapAlloc > peak.Load() {
				peak.Store(m.HeapAlloc)
			}
			time.Sleep(300 * time.Microsecond)
		}
	}()
	res, err := svc.Apply(ctx, change.Set{Ops: []change.Op{{
		Kind: change.KindSetContent, At: b.Ref, IfMatch: b.Rev, Body: &change.SetContent{Text: &text},
	}}}, person)
	close(stop)
	<-done
	require.NoError(t, err)
	require.Equal(t, change.SetApplied, res.Status, "%+v", res.Ops)

	var delta uint64
	if p := peak.Load(); p > base.HeapAlloc {
		delta = p - base.HeapAlloc
	}
	t.Logf("catalog %d KiB, %d messages; peak heap above the baseline %d KiB", size/1024, n, delta/1024)
	assert.Less(t, delta, uint64(ceiling), "the edit took %d KiB of heap; the ceiling is %d KiB", delta/1024, ceiling/1024)

	got, err := os.ReadFile(filepath.Join(dir, "catalog.json"))
	require.NoError(t, err)
	assert.Contains(t, string(got), `"message.050000": "This message was edited",`)
	assert.Equal(t, n+2, strings.Count(string(got), "\n"), "every other message is kept")
}
