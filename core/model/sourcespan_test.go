package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSourceSpanIsAdvisory(t *testing.T) {
	block := NewBlock("tu1", "Same wording")
	before := ComputeIdentity(block)
	_, ok := block.SourceSpan()
	require.False(t, ok)
	span := SourceSpan{Part: "word/document.xml", Start: 50, End: 120}
	block.SetSourceSpan(span)
	got, ok := block.SourceSpan()
	require.True(t, ok)
	assert.Equal(t, span, got)
	assert.Equal(t, before, ComputeIdentity(block), "moving source bytes must not change block identity")
	assert.Equal(t, &span, RefForBlock(block).Source)
	block.SetSourceSpan(SourceSpan{Start: -1, End: 2})
	_, ok = block.SourceSpan()
	assert.False(t, ok)
}
