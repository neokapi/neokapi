package html_test

import (
	"bytes"
	"errors"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/format"
	htmlfmt "github.com/neokapi/neokapi/core/formats/html"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
)

// TestSkeletonInsertedCharsetMeta covers the Content-Type meta the reader adds
// to a head that declares no charset. The source does not hold those bytes, so
// the skeleton marks them inserted: skeleton alignment skips them, and every
// path that replays an HTML skeleton still writes them, so the output keeps the
// meta that mirrors Okapi's HtmlFilter.
func TestSkeletonInsertedCharsetMeta(t *testing.T) {
	const input = "<html>\n<head><title>Hi</title></head>\n<body><p>Hello</p></body>\n</html>\n"
	want := "<html>\n<head>" + injectedContentTypeMeta + "<title>Hi</title></head>\n<body><p>Hello</p></body>\n</html>\n"

	read := func(t *testing.T, store *format.SkeletonStore, src string) []*model.Part {
		t.Helper()
		reader := htmlfmt.NewReader()
		reader.SetSkeletonStore(store)
		require.NoError(t, reader.Open(t.Context(), testutil.RawDocFromString(src, model.LocaleEnglish)))
		parts := testutil.CollectParts(t, reader.Read(t.Context()))
		require.NoError(t, reader.Close())
		return parts
	}
	write := func(t *testing.T, store *format.SkeletonStore, parts []*model.Part) string {
		t.Helper()
		writer := htmlfmt.NewWriter()
		writer.SetSkeletonStore(store)
		var buf bytes.Buffer
		require.NoError(t, writer.SetOutputWriter(&buf))
		require.NoError(t, writer.Write(t.Context(), testutil.PartsToChannel(parts)))
		require.NoError(t, writer.Close())
		return buf.String()
	}
	entriesOf := func(t *testing.T, store *format.SkeletonStore) []format.SkeletonEntry {
		t.Helper()
		require.NoError(t, store.Flush())
		var entries []format.SkeletonEntry
		for {
			e, err := store.Next()
			if errors.Is(err, io.EOF) {
				return entries
			}
			require.NoError(t, err)
			entries = append(entries, e)
		}
	}

	t.Run("the skeleton marks the meta inserted, and aligns with the source", func(t *testing.T) {
		store := format.NewMemorySkeletonStore()
		read(t, store, input)
		entries := entriesOf(t, store)
		var inserted []string
		for _, e := range entries {
			if e.Type == format.SkeletonInserted {
				inserted = append(inserted, string(e.Data))
				continue
			}
			assert.NotContains(t, string(e.Data), "Content-Type", "the meta belongs in an inserted entry alone")
		}
		assert.Equal(t, []string{injectedContentTypeMeta}, inserted)

		extents, err := format.AlignSkeleton([]byte(input), entries)
		require.NoError(t, err)
		var located []string
		for _, x := range extents {
			located = append(located, input[x.Start:x.End])
		}
		assert.Equal(t, []string{"Hi", "Hello"}, located)
	})

	t.Run("a head that declares its charset gets no inserted bytes", func(t *testing.T) {
		store := format.NewMemorySkeletonStore()
		read(t, store, `<html><head><meta charset="utf-8"><title>Hi</title></head><body><p>Hello</p></body></html>`)
		for _, e := range entriesOf(t, store) {
			assert.NotEqual(t, format.SkeletonInserted, e.Type)
		}
	})

	// Each replay path writes the same bytes the reader and writer produced
	// before the meta was marked inserted.
	t.Run("the writer with a file-backed store", func(t *testing.T) {
		store, err := format.NewSkeletonStore()
		require.NoError(t, err)
		defer store.Close()
		parts := read(t, store, input)
		assert.Equal(t, want, write(t, store, parts))
	})

	t.Run("the writer with a streaming store", func(t *testing.T) {
		store := format.NewStreamingSkeletonStore()
		defer store.Close()
		parts := read(t, store, input)
		store.CloseWrite()
		assert.Equal(t, want, write(t, store, parts))
	})

	t.Run("the writer replaying persisted skeleton bytes", func(t *testing.T) {
		store := format.NewMemorySkeletonStore()
		parts := read(t, store, input)
		data, err := store.Bytes()
		require.NoError(t, err)
		replay := format.NewSkeletonStoreFromBytes(data)
		defer replay.Close()
		assert.Equal(t, want, write(t, replay, parts))
	})

	t.Run("the shared skeleton walker", func(t *testing.T) {
		store := format.NewMemorySkeletonStore()
		parts := read(t, store, input)
		require.NoError(t, store.Flush())
		blocks := map[string]*model.Block{}
		for _, p := range parts {
			if b, ok := p.Resource.(*model.Block); ok {
				blocks[b.ID] = b
			}
		}
		render := func(b *model.Block) ([]byte, error) {
			if b == nil {
				return nil, nil
			}
			return []byte(model.RenderRunsWithData(b.Source)), nil
		}
		var out bytes.Buffer
		require.NoError(t, format.BufferedSkeletonWrite(store, blocks, &out, render, nil))
		assert.Equal(t, want, out.String())
	})
}
