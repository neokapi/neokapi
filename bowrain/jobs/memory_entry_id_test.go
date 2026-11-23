package jobs

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
)

func textRuns(s string) []model.Run { return []model.Run{{Text: &model.TextRun{Text: s}}} }

// TestMemoryEntryIDKeepsItsBytes pins that texts holding neither NUL nor
// U+0001 give the id they always have.
func TestMemoryEntryIDKeepsItsBytes(t *testing.T) {
	sum := sha256.Sum256([]byte("en\x00nb\x00Save\x00Lagre"))
	assert.Equal(t, "tmseed-"+hex.EncodeToString(sum[:])[:32], memoryEntryID("en", "nb", textRuns("Save"), textRuns("Lagre")))
}

// TestMemoryEntryIDKeepsNULTextsApart pins that two pairs whose texts split
// differently around a NUL, as a client field can hold one, are two entries.
// Before, they shared an id and the second write merged into the first.
func TestMemoryEntryIDKeepsNULTextsApart(t *testing.T) {
	assert.Equal(t, "a\x00b"+"\x00"+"c", "a"+"\x00"+"b\x00c", "the plain join is alike")
	assert.NotEqual(t,
		memoryEntryID("en", "nb", textRuns("a\x00b"), textRuns("c")),
		memoryEntryID("en", "nb", textRuns("a"), textRuns("b\x00c")))
}
