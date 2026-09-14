package server

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	bstore "github.com/neokapi/neokapi/bowrain/store"
	"github.com/neokapi/neokapi/core/model"
)

// TestApproveTermCandidate_DoNotTranslateSetsTheFlag: a reviewer approving a term
// candidate marked do-not-translate is the governing decision for it, so the
// concept the approval creates carries the flag in the same write. A candidate
// with no such mark creates a concept without it.
func TestApproveTermCandidate_DoNotTranslateSetsTheFlag(t *testing.T) {
	h := newKGHarness(t)
	ctx := context.Background()

	for i, candidate := range []model.TermCandidateAnnotation{
		{Text: "kubectl", Locale: "en", Category: model.TermCategory("product"), Translatability: model.TranslatabilityDNT},
		{Text: "dashboard", Locale: "en", Category: model.TermCategory("ui")},
	} {
		data, err := json.Marshal(candidate)
		require.NoError(t, err)
		h.srv.approveTermCandidate(ctx, &bstore.ReviewItem{
			ID: "item-" + candidate.Text, ProjectID: "proj-x", Type: bstore.ReviewItemTermCandidate,
			Status: bstore.ReviewItemApproved, Data: data, Locale: "en",
		}, kgTestWS)
		require.Positive(t, i+1)
	}

	all, err := h.tb(t).Concepts(ctx)
	require.NoError(t, err)
	flags := map[string]bool{}
	for _, c := range all {
		require.NotEmpty(t, c.Terms)
		flags[c.Terms[0].Text] = c.DoNotTranslate
	}
	assert.Equal(t, map[string]bool{"kubectl": true, "dashboard": false}, flags)
}
