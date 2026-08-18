package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func blockWith(text string, props map[string]string) *Block {
	return &Block{
		Name:       "greeting",
		Type:       "unit",
		Source:     []Run{{Text: &TextRun{Text: text}}},
		Properties: props,
	}
}

func recordHashOf(b *Block) string { return ComputeIdentity(b).RecordHash() }

// The reason this hash exists: a reader that starts recording something new
// about a block leaves its text alone, so a transfer decision made on the
// content hash reports it unchanged forever. The record hash moves, so the next
// ordinary push carries the block and content already stored acquires the field.
func TestRecordHash_MovesWhenAReaderRecordsSomethingNew(t *testing.T) {
	before := blockWith("Hello", map[string]string{"component": "Header"})
	after := blockWith("Hello", map[string]string{"component": "Header", "hash": "k1_abc"})

	require.Equal(t, ComputeIdentity(before).ContentHash, ComputeIdentity(after).ContentHash,
		"the text did not change, which is exactly why the content hash cannot decide this")
	assert.NotEqual(t, recordHashOf(before), recordHashOf(after))
}

// A derived locator is carried, not identifying. Otherwise inserting a line at
// the top of a file re-uploads everything below it, which is the cost that made
// hashing content alone look attractive in the first place.
func TestRecordHash_IgnoresAdvisoryProperties(t *testing.T) {
	before := blockWith("Hello", map[string]string{"component": "Header", AdvisoryPropertyPrefix + "line": "4"})
	after := blockWith("Hello", map[string]string{"component": "Header", AdvisoryPropertyPrefix + "line": "91"})

	assert.Equal(t, recordHashOf(before), recordHashOf(after))
}

// The other two things a block is stored with. A block renamed or retyped is
// not the block the far side holds, and a push that reports it unchanged leaves
// the server serving a stale name.
func TestRecordHash_CoversNameAndType(t *testing.T) {
	base := blockWith("Hello", nil)

	renamed := blockWith("Hello", nil)
	renamed.Name = "farewell"
	assert.NotEqual(t, recordHashOf(base), recordHashOf(renamed))

	retyped := blockWith("Hello", nil)
	retyped.Type = "heading"
	assert.NotEqual(t, recordHashOf(base), recordHashOf(retyped))
}

// Text still decides, of course.
func TestRecordHash_MovesWithTheText(t *testing.T) {
	assert.NotEqual(t,
		recordHashOf(blockWith("Hello", nil)),
		recordHashOf(blockWith("Goodbye", nil)))
}

// An absent property map and an empty one are the same block. The store returns
// nil for a block with no properties while a reader may hand over an empty map,
// so a hash that told them apart would re-upload every propertyless block on
// every push, forever.
func TestRecordHash_NilAndEmptyPropertiesAgree(t *testing.T) {
	assert.Equal(t,
		recordHashOf(blockWith("Hello", nil)),
		recordHashOf(blockWith("Hello", map[string]string{})))
}

// Both halves are stored as columns on the server, which folds them without
// rehydrating a block. The two paths must agree or every push re-uploads.
func TestComputeRecordHash_FoldsTheStoredHalves(t *testing.T) {
	b := blockWith("Hello", map[string]string{"component": "Header"})
	id := ComputeIdentity(b)

	assert.Equal(t, id.RecordHash(), ComputeRecordHash(id.ContentHash, id.ContextHash))
}

// Pinned like the content hash it folds. Changing the fold re-uploads every
// project's whole corpus once, which is survivable but must be deliberate.
func TestRecordHash_GoldenPin(t *testing.T) {
	assert.Equal(t,
		"43e033ef89fd4eef279f3508564e2feb0e2f12613ab70998bdb763f5578fe154",
		ComputeRecordHash(
			"64ec88ca00b268e5ba1a35678a1b5316d212f4f366b2477232534a8aeca37f3c",
			"e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"))
}

func TestRecordHash_NilIdentityIsEmpty(t *testing.T) {
	var id *BlockIdentity
	assert.Empty(t, id.RecordHash())
}
