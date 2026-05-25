//go:build onnx && satmodel

// This end-to-end smoke test runs the real ONNX-backed engine against a real
// SaT model. It is gated behind BOTH build tags:
//
//	go test -tags "onnx satmodel" ./internal/sat/
//
// It requires the onnxruntime shared library (KAPI_SAT_ORT_LIB or on the
// loader path), the tokenizer native library linked in, and network access to
// download the model + tokenizer on first run (cached afterward). CI without
// the native deps never compiles this file, so the default test run stays
// green.
package sat

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEngineSegmentEndToEnd(t *testing.T) {
	eng, err := NewEngine(func(format string, args ...any) { t.Logf(format, args...) })
	require.NoError(t, err)
	defer func() { _ = eng.Close() }()

	const text = "Hello world. How are you?"
	bounds, err := eng.Segment(text, "sat-3l-sm", "en", 0)
	require.NoError(t, err)

	// Expect at least one interior boundary, and the first one should fall
	// after the first sentence ("Hello world. ") at the start of "How".
	require.NotEmpty(t, bounds, "expected at least one sentence boundary")
	runes := []rune(text)
	first := bounds[0]
	require.Greater(t, first, 0)
	require.Less(t, first, len(runes))
	// The boundary should land on or near the 'H' of "How" (rune index 13).
	assert.InDelta(t, 13, first, 2, "first boundary should be near the start of the second sentence")
	assert.True(t, eng.Loaded("sat-3l-sm"))
}
