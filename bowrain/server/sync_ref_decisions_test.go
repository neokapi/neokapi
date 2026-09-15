package server

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/venue"
)

// basis writes what a server draft records for a unit: the target it wrote and
// the source it wrote it for, and no decision.
func (h *refHarness) basis(t *testing.T, unit string) {
	t.Helper()
	_, err := h.store.UpsertUnitDecisions(t.Context(), h.project.ID, refTestStream,
		[]venue.UnitDecision{{
			ItemName: "docs/intro.md", Unit: unit, Variant: "fr",
			TargetHash: "target-" + unit, ContentHash: "source-" + unit,
			Updated: "2026-09-15T00:00:00Z",
		}})
	require.NoError(t, err)
}

const oneBasis = `[{"item":"docs/intro.md","unit":"u3","variant":"fr","targetHash":"target-u3","contentHash":"source-u3","updated":"2026-09-15T00:00:00Z"}]`

// A server run drafts units between a client's pull and its push, and each
// draft records its basis in the ledger. None of those records is a decision
// the client missed, so the client's decision still lands. Counting them made
// every push after a server run a conflict that a pull could not clear, because
// the next run moved the ledger again.
func TestSyncPushCommit_ServerDraftingAfterTheReadIsNotAConflict(t *testing.T) {
	h := newRefHarness(t)
	h.decide(t, "u1", "approved")
	client := h.currentRef(t)

	h.basis(t, "u7")
	h.basis(t, "u8")
	require.Equal(t, client.Decisions, h.currentRef(t).Decisions,
		"drafting records leave the decisions component where the client read it")

	rec := h.commit(t, "", oneDecision, client)
	assert.NotEqual(t, http.StatusConflict, rec.Code, rec.Body.String())
}

// A push that carries only what was produced decides nothing, so it asserts
// nothing about the decisions component, even when another reviewer's decision
// has landed since the client read it.
func TestSyncPushCommit_APushCarryingNoDecisionAssertsNothing(t *testing.T) {
	h := newRefHarness(t)
	h.decide(t, "u1", "approved")
	client := h.currentRef(t)
	h.decide(t, "u9", "approved")
	require.NotEqual(t, client.Decisions, h.currentRef(t).Decisions)

	rec := h.commit(t, "", oneBasis, client)
	assert.NotEqual(t, http.StatusConflict, rec.Code, rec.Body.String())
}
