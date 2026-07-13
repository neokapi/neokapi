package arb_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/formats/arb"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestStreamingPair_ByteExactARB runs the reader+writer against a concurrent
// streaming skeleton store (the file-runner's isStreamingPair path): the writer
// pulls each block on demand. Asserts no deadlock + byte-exact.
func TestStreamingPair_ByteExactARB(t *testing.T) {
	in := "{\n  \"@@locale\": \"en\",\n  \"a\": \"one\",\n  \"b\": \"two\",\n  \"@b\": { \"description\": \"second\" },\n  \"c\": \"three\"\n}\n"
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "app.arb")
	require.NoError(t, os.WriteFile(path, []byte(in), 0o644))
	f, err := os.Open(path)
	require.NoError(t, err)

	reader := arb.NewReader()
	writer := arb.NewWriter()
	store := format.NewStreamingSkeletonStore()
	reader.SetSkeletonStore(store)
	writer.SetSkeletonStore(store)
	require.True(t, store.IsStreaming())
	require.NoError(t, reader.Open(ctx, &model.RawDocument{URI: path, SourceLocale: model.LocaleEnglish, Reader: f}))
	var buf bytes.Buffer
	require.NoError(t, writer.SetOutputWriter(&buf))
	partsCh := make(chan *model.Part, 64)
	go func() {
		defer close(partsCh)
		defer store.CloseWrite()
		defer reader.Close()
		for res := range reader.Read(ctx) {
			assert.NoError(t, res.Error)
			if res.Part != nil {
				partsCh <- res.Part
			}
		}
	}()
	require.NoError(t, writer.Write(ctx, partsCh))
	writer.Close()
	assert.Equal(t, in, buf.String(), "streaming pair must round-trip byte-exact")
}
