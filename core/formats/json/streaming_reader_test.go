package json_test

import (
	"context"
	"fmt"
	"io"
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
// paths: the buffered cross-format path stashes json.original).
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
// stream as the buffered read (no skeleton store) for the same document. That
// is, the streaming walk is identical to the buffered walk.
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
// flat window, independent of document size.
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
		gate := testutil.NewGate(pr)
		require.NoError(t, reader.Open(context.Background(), &model.RawDocument{
			URI:          "big.json",
			SourceLocale: model.LocaleEnglish,
			Reader:       io.NopCloser(gate),
		}))

		// The gate stops the reader at 256 points through the document, the
		// same density for both sizes, so the two runs estimate the same
		// bounded working set. Each sample is taken with the reader waiting at
		// the gate and every part it produced received: nothing allocates
		// during the collection. A reader that held the document would keep it
		// live, so the bound below still trips.
		peak = testutil.SamplePeak(gate, int64(sz), 256, reader.Read(context.Background()), func(res model.PartResult) {
			require.NoError(t, res.Error)
		})
		reader.Close()
		return peak, sz
	}

	ps, smallSize := peakReading(10_000)
	pl, largeSize := peakReading(200_000) // 20x

	testutil.AssertBoundedMemory(t, "json streaming reader", ps, uint64(smallSize), pl, uint64(largeSize))
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
