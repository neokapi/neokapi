package knowledge

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSameOps(t *testing.T) {
	op := func(kind OpType, payload string) *ChangeSetOp {
		return &ChangeSetOp{Op: kind, Payload: json.RawMessage(payload)}
	}
	t.Run("key order and whitespace do not count", func(t *testing.T) {
		a := []*ChangeSetOp{op(OpConceptDelete, `{"concept_id":"x","note":"n"}`)}
		b := []*ChangeSetOp{op(OpConceptDelete, `{ "note": "n", "concept_id": "x" }`)}
		assert.True(t, SameOps(a, b))
	})
	t.Run("a differing payload counts", func(t *testing.T) {
		a := []*ChangeSetOp{op(OpConceptDelete, `{"concept_id":"x"}`)}
		b := []*ChangeSetOp{op(OpConceptDelete, `{"concept_id":"y"}`)}
		assert.False(t, SameOps(a, b))
	})
	t.Run("op order counts", func(t *testing.T) {
		a := []*ChangeSetOp{op(OpConceptDelete, `{"concept_id":"x"}`), op(OpConceptDelete, `{"concept_id":"y"}`)}
		b := []*ChangeSetOp{op(OpConceptDelete, `{"concept_id":"y"}`), op(OpConceptDelete, `{"concept_id":"x"}`)}
		assert.False(t, SameOps(a, b))
	})
	t.Run("length counts", func(t *testing.T) {
		a := []*ChangeSetOp{op(OpConceptDelete, `{"concept_id":"x"}`)}
		assert.False(t, SameOps(a, nil))
	})
}

func TestSupersededTransitions(t *testing.T) {
	require.NoError(t, ValidateStatusTransition(ChangeSetDraft, ChangeSetSuperseded))
	require.NoError(t, ValidateStatusTransition(ChangeSetInReview, ChangeSetSuperseded))
	// Decided change-sets are history, not candidates for replacement.
	assert.Error(t, ValidateStatusTransition(ChangeSetMerged, ChangeSetSuperseded))
	assert.Error(t, ValidateStatusTransition(ChangeSetAbandoned, ChangeSetSuperseded))
	// Superseded is terminal.
	assert.Error(t, ValidateStatusTransition(ChangeSetSuperseded, ChangeSetInReview))
}
