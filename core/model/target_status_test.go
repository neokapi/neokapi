package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTargetStatus_RankAndLadder(t *testing.T) {
	assert.Equal(t, -1, TargetStatusNew.Rank(), "New sits below the ladder")
	assert.Equal(t, 0, TargetStatusDraft.Rank())
	assert.Less(t, TargetStatusDraft.Rank(), TargetStatusTranslated.Rank())
	assert.Less(t, TargetStatusTranslated.Rank(), TargetStatusEstablished.Rank())
	assert.Equal(t, -1, TargetStatus("nonsense").Rank())
	assert.Len(t, TargetStatusLadder(), 3)
}

func TestStampTargetProvenance(t *testing.T) {
	b := NewBlock("tu1", "Hello")
	// No-op when no target exists yet.
	b.StampTargetProvenance(LocaleFrench, TargetStatusDraft, Origin{Kind: OriginAI})
	_, held := b.TargetEdition(LocaleFrench)
	assert.False(t, held)

	// Stamps status + origin on an existing target without touching its runs.
	b.SetTargetText(LocaleFrench, "Bonjour")
	b.StampTargetProvenance(LocaleFrench, TargetStatusDraft, Origin{Kind: OriginAI, Engine: "anthropic"})

	tgt, held := b.TargetEdition(LocaleFrench)
	if assert.True(t, held) {
		assert.Equal(t, "Bonjour", b.TargetText(LocaleFrench), "runs untouched")
		assert.Equal(t, Status(TargetStatusDraft), tgt.Status)
		assert.Equal(t, OriginAI, tgt.Origin.Kind)
		assert.Equal(t, "anthropic", tgt.Origin.Engine)
	}
}
