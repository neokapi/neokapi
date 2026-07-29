package safeio_test

import (
	"bytes"
	"strings"
	"testing"
	"unsafe"

	"github.com/neokapi/neokapi/core/safeio"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNoCopyStringMatchesTheCopyingConversion pins the only thing a caller
// should have to think about: the value is exactly what string(b) would have
// produced. Arbitrary bytes included — this is a view, not a decoder, so
// invalid UTF-8 must come through untouched rather than being sanitised.
func TestNoCopyStringMatchesTheCopyingConversion(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		in   []byte
	}{
		{name: "nil", in: nil},
		{name: "empty", in: []byte{}},
		{name: "ascii", in: []byte("hello")},
		{name: "json document", in: []byte(`{"greeting":"Hello","farewell":"Bye"}`)},
		{name: "multi-byte runes", in: []byte("héllo 日本語 🎉")},
		{name: "nul and control bytes", in: []byte{0x00, 0x01, 0x1f, 0x7f}},
		{name: "invalid utf-8", in: []byte{0xff, 0xfe, 'a', 0x80}},
		{name: "embedded newlines", in: []byte("line1\nline2\r\n")},
		{name: "large", in: bytes.Repeat([]byte("abcdefghij"), 4096)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, string(tt.in), safeio.NoCopyString(tt.in))
			assert.Len(t, safeio.NoCopyString(tt.in), len(tt.in))
		})
	}
}

// TestNoCopyStringSharesTheBackingArray is the reason the function exists. If
// someone "fixes" it to string(b) — a reasonable-looking reaction to the word
// unsafe — the readers silently start copying every document they parse. The
// pointer identity check says so immediately.
func TestNoCopyStringSharesTheBackingArray(t *testing.T) {
	t.Parallel()

	b := []byte("the original document bytes")
	s := safeio.NoCopyString(b)

	assert.Equal(t, uintptr(unsafe.Pointer(unsafe.SliceData(b))), uintptr(unsafe.Pointer(unsafe.StringData(s))),
		"the string must view the slice's backing array, not a copy of it")
}

// TestNoCopyStringDoesNotAllocate is the same guard stated in the units that
// matter. A whole-document copy on every file read is what this avoids, and an
// allocation budget of zero is the only assertion that keeps holding as the
// implementation changes.
func TestNoCopyStringDoesNotAllocate(t *testing.T) {
	// Not parallel: testing.AllocsPerRun requires a stable GOMAXPROCS.
	doc := bytes.Repeat([]byte("some document content\n"), 8192)
	var sink string

	allocs := testing.AllocsPerRun(100, func() {
		sink = safeio.NoCopyString(doc)
	})

	require.NotEmpty(t, sink)
	assert.Zero(t, allocs, "converting a document to a string view must not allocate")

	// For contrast, and to keep the claim honest: the copying conversion does.
	copyAllocs := testing.AllocsPerRun(100, func() {
		sink = strings.Clone(string(doc))
	})
	assert.Positive(t, copyAllocs, "string(b) allocates — that is the cost being avoided")
}

// TestNoCopyStringNilIsSafe pins the edge the contract promises: a nil slice
// has no backing array to point at, and must still yield "" rather than
// dereferencing nil.
func TestNoCopyStringNilIsSafe(t *testing.T) {
	t.Parallel()

	var b []byte
	require.NotPanics(t, func() {
		assert.Empty(t, safeio.NoCopyString(b))
	})
	require.NotPanics(t, func() {
		assert.Empty(t, safeio.NoCopyString([]byte{}))
	})
}
