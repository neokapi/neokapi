package androidxml_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/formats/androidxml"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func streamingPairRoundtripAndroid(t *testing.T, input string) string {
	t.Helper()
	ctx := context.Background()
	reader := androidxml.NewReader()
	writer := androidxml.NewWriter()
	store := format.NewStreamingSkeletonStore()
	reader.SetSkeletonStore(store)
	writer.SetSkeletonStore(store)
	require.True(t, store.IsStreaming())
	require.NoError(t, reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish)))
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
	return buf.String()
}

func TestStreamingPair_ByteExactAndroid(t *testing.T) {
	cases := map[string]string{
		"plain": "<?xml version=\"1.0\" encoding=\"utf-8\"?>\n<resources>\n    <string name=\"hello\">Hello %1$s</string>\n</resources>\n",
		"array": "<?xml version=\"1.0\" encoding=\"utf-8\"?>\n<resources>\n    <string-array name=\"days\">\n        <item>Monday</item>\n        <item>Tuesday</item>\n    </string-array>\n</resources>\n",
		"mixed": "<?xml version=\"1.0\" encoding=\"utf-8\"?>\n<resources>\n    <!-- note -->\n    <string name=\"a\">A</string>\n    <string name=\"ref\">@string/a</string>\n    <string name=\"b\">B</string>\n</resources>\n",
	}
	for name, in := range cases {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, in, streamingPairRoundtripAndroid(t, in), "streaming pair must round-trip byte-exact")
		})
	}
}
