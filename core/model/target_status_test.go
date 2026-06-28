package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTargetStatus_RankAndLadder(t *testing.T) {
	assert.Equal(t, -1, TargetStatusNew.Rank(), "New sits below the ladder")
	assert.Equal(t, 0, TargetStatusDraft.Rank())
	assert.Less(t, TargetStatusDraft.Rank(), TargetStatusTranslated.Rank())
	assert.Less(t, TargetStatusTranslated.Rank(), TargetStatusReviewed.Rank())
	assert.Less(t, TargetStatusReviewed.Rank(), TargetStatusSignedOff.Rank())
	assert.Equal(t, -1, TargetStatus("nonsense").Rank())
	assert.Len(t, TargetStatusLadder(), 4)
}

func TestStampTargetProvenance(t *testing.T) {
	b := NewBlock("tu1", "Hello")
	// No-op when no target exists yet.
	b.StampTargetProvenance(LocaleFrench, TargetStatusDraft, Origin{Kind: OriginAI})
	assert.Nil(t, b.Target(LocaleFrench))

	// Stamps status + origin on an existing target without touching its runs.
	b.SetTargetText(LocaleFrench, "Bonjour")
	b.StampTargetProvenance(LocaleFrench, TargetStatusDraft, Origin{Kind: OriginAI, Engine: "anthropic"})

	tgt := b.Target(LocaleFrench)
	if assert.NotNil(t, tgt) {
		assert.Equal(t, "Bonjour", b.TargetText(LocaleFrench), "runs untouched")
		assert.Equal(t, TargetStatusDraft, tgt.Status)
		assert.Equal(t, OriginAI, tgt.Origin.Kind)
		assert.Equal(t, "anthropic", tgt.Origin.Engine)
	}
}
