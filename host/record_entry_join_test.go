package host

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/neokapi/neokapi/core/partjoin"
)

// TestRecordEntryIDKeepsNULSourcesApart pins that two sources whose
// structural and flat texts split differently around a NUL get two entry ids,
// as recordSourceKey joins them, and so do a source and point that split
// differently. Before, both were joined with a bare NUL and shared one
// content-memory entry.
func TestRecordEntryIDKeepsNULSourcesApart(t *testing.T) {
	assert.Equal(t, "a\x00b"+"\x00"+"c", "a"+"\x00"+"b\x00c", "the plain join is alike")
	assert.NotEqual(t,
		recordEntryID(partjoin.NUL("a\x00b", "c"), ""),
		recordEntryID(partjoin.NUL("a", "b\x00c"), ""))
	assert.NotEqual(t,
		recordEntryID(partjoin.NUL("a", "b"), "c\x00d"),
		recordEntryID(partjoin.NUL("a", "b\x00c"), "d"))
}
