package xcstrings_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/formats/xcstrings"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestStreamingPair_ByteExactXC runs the reader+writer against a concurrent
// streaming skeleton store (the file-runner's isStreamingPair path): the writer
// pulls each block on demand. Asserts no deadlock + byte-exact.
func TestStreamingPair_ByteExactXC(t *testing.T) {
	in := "{\n  \"sourceLanguage\" : \"en\",\n  \"strings\" : {\n    \"A\" : {\n      \"comment\" : \"note\",\n      \"localizations\" : {\n        \"de\" : {\n          \"stringUnit\" : {\n            \"state\" : \"translated\",\n            \"value\" : \"Ah\"\n          }\n        }\n      }\n    },\n    \"B\" : {\n      \"localizations\" : {\n        \"de\" : {\n          \"stringUnit\" : {\n            \"state\" : \"translated\",\n            \"value\" : \"Beh\"\n          }\n        }\n      }\n    }\n  },\n  \"version\" : \"1.0\"\n}\n"
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "L.xcstrings")
	require.NoError(t, os.WriteFile(path, []byte(in), 0o644))
	f, err := os.Open(path)
	require.NoError(t, err)

	reader := xcstrings.NewReader()
	writer := xcstrings.NewWriter()
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
