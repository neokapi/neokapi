package bridgetest

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/gokapi/gokapi/core/model"
	"github.com/gokapi/gokapi/core/plugin/bridge"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// RoundTripResult holds the output of a roundtrip test.
type RoundTripResult struct {
	// Parts extracted during the read phase.
	Parts []*model.Part
	// Output is the reconstructed document bytes from the write phase.
	Output []byte
}

// RoundTrip performs a full read → write cycle through the bridge:
//  1. Read the input content using the specified filter to extract parts
//  2. Write the parts back through the same filter to reconstruct the document
//
// Returns the extracted parts and the reconstructed output bytes.
func RoundTrip(t *testing.T, pool *bridge.BridgePool, cfg bridge.BridgeConfig, filterClass string, content []byte, uri, mimeType string, filterParams map[string]any) RoundTripResult {
	t.Helper()

	// --- Read phase ---
	reader := bridge.NewBridgeFormatReader(pool, cfg, filterClass)
	if filterParams != nil {
		reader.SetFilterParams(filterParams)
	}

	doc := &model.RawDocument{
		URI:          uri,
		SourceLocale: "en",
		Encoding:     "UTF-8",
		MimeType:     mimeType,
		Reader:       io.NopCloser(bytes.NewReader(content)),
	}

	ctx := context.Background()
	require.NoError(t, reader.Open(ctx, doc))

	var parts []*model.Part
	for pr := range reader.Read(ctx) {
		require.NoError(t, pr.Error, "roundtrip read phase")
		parts = append(parts, pr.Part)
	}
	require.NoError(t, reader.Close())

	// --- Write phase ---
	var output bytes.Buffer
	writer := bridge.NewBridgeFormatWriter(pool, cfg, filterClass)
	if filterParams != nil {
		writer.SetFilterParams(filterParams)
	}
	writer.SetOriginalContent(content)
	writer.SetEncoding("UTF-8")
	writer.SetLocale("fr")
	require.NoError(t, writer.SetOutputWriter(&output))

	partsCh := make(chan *model.Part, len(parts))
	for _, p := range parts {
		partsCh <- p
	}
	close(partsCh)

	require.NoError(t, writer.Write(ctx, partsCh), "roundtrip write phase")

	return RoundTripResult{
		Parts:  parts,
		Output: output.Bytes(),
	}
}

// AssertRoundTrip performs a roundtrip and asserts the output matches the input
// byte-for-byte. This is the strongest form of roundtrip validation.
func AssertRoundTrip(t *testing.T, pool *bridge.BridgePool, cfg bridge.BridgeConfig, filterClass string, content []byte, uri, mimeType string, filterParams map[string]any) RoundTripResult {
	t.Helper()

	result := RoundTrip(t, pool, cfg, filterClass, content, uri, mimeType, filterParams)
	assert.Equal(t, string(content), string(result.Output),
		"roundtrip output should match original input")
	return result
}

// RoundTripTestFiles runs roundtrip tests over all files matching a glob pattern
// within the testdata directory. Each file becomes a subtest named after the
// filename. Files that don't roundtrip cleanly fail the subtest.
func RoundTripTestFiles(t *testing.T, pool *bridge.BridgePool, cfg bridge.BridgeConfig, filterClass, globPattern, mimeType string, filterParams map[string]any) {
	t.Helper()

	files, err := filepath.Glob(globPattern)
	require.NoError(t, err, "globbing test files")

	if len(files) == 0 {
		t.Skipf("no test files matching %s", globPattern)
	}

	for _, f := range files {
		name := filepath.Base(f)
		t.Run(name, func(t *testing.T) {
			content, err := os.ReadFile(f)
			require.NoError(t, err)
			AssertRoundTrip(t, pool, cfg, filterClass, content, name, mimeType, filterParams)
		})
	}
}
