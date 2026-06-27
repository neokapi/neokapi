package json_test

import (
	"context"
	"fmt"
	"io"
	"runtime"
	"runtime/debug"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	jsonfmt "github.com/neokapi/neokapi/core/formats/json"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// blockDataSig reduces a Part stream to the Block/Data signal (ignoring the
// layer, whose properties legitimately differ between the streaming and buffered
// paths — the buffered cross-format path stashes json.original).
func blockDataSig(parts []*model.Part) []string {
	var sig []string
	for _, p := range parts {
		switch p.Type {
		case model.PartBlock:
			b := p.Resource.(*model.Block)
			sig = append(sig, fmt.Sprintf("B name=%q src=%q tr=%v q=%q", b.Name, b.SourceText(), b.Translatable, b.Properties["json.quote"]))
		case model.PartData:
			d := p.Resource.(*model.Data)
			sig = append(sig, fmt.Sprintf("D name=%q", d.Name))
		}
	}
	return sig
}

// TestStreamingReaderMatchesBuffered asserts the streaming JSON read (skeleton
// store wired, validation off → streaming path) emits the same Block/Data part
// stream as the buffered read (no skeleton store) for the same document — i.e.
// the streaming walk is identical to the buffered walk.
func TestStreamingReaderMatchesBuffered(t *testing.T) {
	inputs := []string{
		`{"key":"Text1"}`,
		`{ "a" : "x" , "b" : { "c" : "y" }, "n": 5, "ok": true }`,
		"{\n  // comment\n  \"title\": \"Hello\",\n  /* block */ \"body\": \"World\"\n}\n",
		`{"items":["a","b"],"objs":[{"k":"v"},{"k":"w"}]}`,
		`{'single':'quoted',"dbl":"v"}`,
		`{"nested":{"a":{"b":{"c":"deep"}}}}`,
		`{"esc":"line\nbreak \"q\" \\ é 😀","u":"café"}`,
	}
	for _, in := range inputs {
		streaming := readWithSkeleton(t, in)
		buffered := readPlain(t, in)
		assert.Equal(t, blockDataSig(buffered), blockDataSig(streaming),
			"streaming vs buffered part stream differs for %q", in)
	}
}

func readWithSkeleton(t *testing.T, in string) []*model.Part {
	t.Helper()
	ctx := t.Context()
	reader := jsonfmt.NewReader()
	store, err := format.NewSkeletonStore()
	require.NoError(t, err)
	defer store.Close()
	reader.SetSkeletonStore(store)
	require.NoError(t, reader.Open(ctx, testutil.RawDocFromString(in, model.LocaleEnglish)))
	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()
	return parts
}

func readPlain(t *testing.T, in string) []*model.Part {
	t.Helper()
	ctx := t.Context()
	reader := jsonfmt.NewReader()
	require.NoError(t, reader.Open(ctx, testutil.RawDocFromString(in, model.LocaleEnglish)))
	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()
	return parts
}

// TestStreamingReaderBoundedMemory reads a large JSON document supplied as a pure
// stream (an io.Pipe generator, never a whole buffer) through the streaming
// reader path, draining and discarding parts, and asserts peak heap is a small,
// flat window — independent of document size.
func TestStreamingReaderBoundedMemory(t *testing.T) {
	if testing.Short() {
		t.Skip("memory test skipped in -short")
	}
	defer debug.SetGCPercent(debug.SetGCPercent(20))

	peakReading := func(n int) (peak uint64, size int) {
		pr, sz := genJSONObject(n)
		reader := jsonfmt.NewReader()
		store, err := format.NewSkeletonStore()
		require.NoError(t, err)
		defer store.Close()
		reader.SetSkeletonStore(store)
		require.NoError(t, reader.Open(context.Background(), &model.RawDocument{
			URI:          "big.json",
			SourceLocale: model.LocaleEnglish,
			Reader:       io.NopCloser(pr),
		}))

		runtime.GC()
		var base runtime.MemStats
		runtime.ReadMemStats(&base)

		var m runtime.MemStats
		count := 0
		for res := range reader.Read(context.Background()) {
			require.NoError(t, res.Error)
			count++
			if count%256 == 0 {
				runtime.ReadMemStats(&m)
				if m.HeapAlloc > base.HeapAlloc && m.HeapAlloc-base.HeapAlloc > peak {
					peak = m.HeapAlloc - base.HeapAlloc
				}
			}
		}
		reader.Close()
		return peak, sz
	}

	ps, smallSize := peakReading(10_000)
	pl, largeSize := peakReading(200_000) // 20x
	t.Logf("streaming reader peakΔ: small(%d KiB)=%d KiB, large(%d KiB)=%d KiB",
		smallSize/1024, ps/1024, largeSize/1024, pl/1024)

	if pl > uint64(largeSize)/4 {
		t.Errorf("streaming reader peak %d B not bounded well below doc size %d B", pl, largeSize)
	}
	if ps > 0 && pl > ps*3 {
		t.Errorf("streaming reader peak scaled with input: small=%d B large=%d B (20x doc -> %.1fx)", ps, pl, float64(pl)/float64(ps))
	}
}

// genJSONObject emits a JSON object of n translatable entries on demand via an
// io.Pipe, so the document never exists as a whole buffer.
func genJSONObject(n int) (io.Reader, int) {
	size := len("{\n") + len("\n}\n")
	for i := range n {
		if i > 0 {
			size += len(",\n")
		}
		size += len(fmt.Sprintf("  \"key_%d\": \"translatable value number %d with words\"", i, i))
	}
	pr, pw := io.Pipe()
	go func() {
		_, _ = io.WriteString(pw, "{\n")
		for i := range n {
			if i > 0 {
				_, _ = io.WriteString(pw, ",\n")
			}
			fmt.Fprintf(pw, "  \"key_%d\": \"translatable value number %d with words\"", i, i)
		}
		_, _ = io.WriteString(pw, "\n}\n")
		pw.Close()
	}()
	return pr, size
}
