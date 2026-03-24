//go:build integration

package compat

import (
	"bytes"
	"context"
	"io"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/require"
)

// nativeRoundTrip reads input through a native format reader then writes it
// back through the corresponding writer using a skeleton store for byte-exact
// reconstruction.
func nativeRoundTrip(t *testing.T, newReader func() format.DataFormatReader, newWriter func() format.DataFormatWriter, input []byte, uri string) []byte {
	t.Helper()
	ctx := context.Background()

	reader := newReader()
	writer := newWriter()

	// Wire skeleton store for byte-exact roundtrip.
	store, err := format.NewSkeletonStore()
	require.NoError(t, err, "creating skeleton store")
	defer store.Close()

	if emitter, ok := reader.(format.SkeletonStoreEmitter); ok {
		emitter.SetSkeletonStore(store)
	}
	if consumer, ok := writer.(format.SkeletonStoreConsumer); ok {
		consumer.SetSkeletonStore(store)
	}
	if setter, ok := writer.(format.OriginalContentSetter); ok {
		setter.SetOriginalContent(input)
	}

	// Read.
	doc := &model.RawDocument{
		URI:          uri,
		SourceLocale: "en",
		Encoding:     "UTF-8",
		Reader:       io.NopCloser(bytes.NewReader(input)),
	}
	require.NoError(t, reader.Open(ctx, doc))

	var parts []*model.Part
	for pr := range reader.Read(ctx) {
		require.NoError(t, pr.Error, "reading part")
		parts = append(parts, pr.Part)
	}
	require.NoError(t, reader.Close())

	// Write.
	var buf bytes.Buffer
	require.NoError(t, writer.SetOutputWriter(&buf))

	ch := make(chan *model.Part, len(parts))
	for _, p := range parts {
		ch <- p
	}
	close(ch)

	require.NoError(t, writer.Write(ctx, ch))
	require.NoError(t, writer.Close())

	return buf.Bytes()
}
