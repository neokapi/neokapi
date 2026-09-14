package server

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/bowrain/knowledge"
)

// TestContextScanApprove_DoNotTranslateTermIsProposed: an approved scan term
// marked do-not-translate is a governed creation, so the approval proposes it in
// a submitted change-set and writes nothing for it. The ordinary term beside it
// is created as before.
func TestContextScanApprove_DoNotTranslateTermIsProposed(t *testing.T) {
	srv, scanID := setupScanApproval(t)
	fake := newFakeKnowledgeStore()
	srv.KnowledgeStore = fake

	rec, err := approveScan(t, srv, scanID, `{
		"profile": {"name": "Acme Voice"},
		"locale": "en-US",
		"terms": [
			{"term": "Acme Cloud", "domain": "product", "do_not_translate": true},
			{"term": "cart", "domain": "commerce"}
		]
	}`)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	tb, err := srv.wsStores.getTerms(approveWSSlug)
	require.NoError(t, err)
	all, err := tb.Concepts(t.Context())
	require.NoError(t, err)
	var written []string
	for _, c := range all {
		require.NotEmpty(t, c.Terms)
		written = append(written, c.Terms[0].Text)
	}
	assert.Equal(t, []string{"cart"}, written, "only the ordinary term is written directly")

	sets, err := fake.ListChangeSets(t.Context(), approveWSID, knowledge.ChangeSetInReview)
	require.NoError(t, err)
	require.Len(t, sets, 1, "the do-not-translate term is proposed for review")
	ops, err := fake.ListOps(t.Context(), approveWSID, sets[0].ID)
	require.NoError(t, err)
	require.Len(t, ops, 1)
	assert.Equal(t, knowledge.OpConceptCreate, ops[0].Op)
	var p knowledge.ConceptCreatePayload
	require.NoError(t, json.Unmarshal(ops[0].Payload, &p))
	assert.True(t, p.Concept.DoNotTranslate)
	require.NotEmpty(t, p.Concept.Terms)
	assert.Equal(t, "Acme Cloud", p.Concept.Terms[0].Text)
	governed, err := knowledge.IsGovernedOp(*ops[0])
	require.NoError(t, err)
	assert.True(t, governed)
}
