package server

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	"github.com/neokapi/neokapi/bowrain/knowledge"
	"github.com/neokapi/neokapi/core/model"
	coreprofile "github.com/neokapi/neokapi/core/profile"
	coretools "github.com/neokapi/neokapi/core/tools"
	"github.com/neokapi/neokapi/terms"
)

// TestDoNotTranslateChangeMergesOnlyAfterReview proposes setting the flag on a
// concept in a change-set. An ordinary change-set merges from draft, and this
// one is governed, so it does not. Submitted and approved, it merges, sets the
// flag, records a revision of the concept, and the concept's rule fails a target
// that translated the term.
func TestDoNotTranslateChangeMergesOnlyAfterReview(t *testing.T) {
	h := newKGHarness(t)
	ctx := context.Background()
	const owner = "owner-1"
	h.srv.AuthStore = newFakeGovAuthStore().add(kgTestWS, owner, platauth.RoleOwner)

	tb := h.tb(t)
	require.NoError(t, tb.AddConcept(ctx, terms.Concept{
		ID:    "c-kapi",
		Terms: []terms.Term{{Text: "kapi", Locale: "en", Status: model.TermApproved}},
	}))
	require.NoError(t, h.fake.CreateChangeSet(ctx, &knowledge.ChangeSet{
		ID: "cs-dnt", WorkspaceID: kgTestWS, Name: "Keep kapi as written",
		Status: knowledge.ChangeSetDraft, CreatedBy: owner,
	}))
	require.NoError(t, h.fake.AppendOp(ctx, &knowledge.ChangeSetOp{
		WorkspaceID: kgTestWS, ChangesetID: "cs-dnt", Op: knowledge.OpConceptUpdate,
		Payload: json.RawMessage(`{"concept_id": "c-kapi", "do_not_translate": true}`), CreatedBy: owner,
	}))
	engine := knowledge.NewEngine(nil, tb, h.fake)
	flag := func() bool {
		t.Helper()
		c, ok, err := tb.GetConcept(ctx, "c-kapi")
		require.NoError(t, err)
		require.True(t, ok)
		return c.DoNotTranslate
	}

	cs, err := h.fake.GetChangeSet(ctx, kgTestWS, "cs-dnt")
	require.NoError(t, err)
	_, err = engine.MergeChangeSet(ctx, kgTestWS, h.fake, *cs)
	require.Error(t, err, "a governed change-set does not merge from draft, where an ordinary one would")
	assert.False(t, flag(), "nothing is applied before review")

	require.NoError(t, h.fake.SetChangeSetStatus(ctx, kgTestWS, "cs-dnt", knowledge.ChangeSetInReview))

	c, rec := h.req(http.MethodPost, "/", `{"comment": "a product name"}`, govPerms, "id", "cs-dnt")
	withActor(c, owner)
	require.NoError(t, h.srv.HandleApproveChangeSet(c))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	cs, err = h.fake.GetChangeSet(ctx, kgTestWS, "cs-dnt")
	require.NoError(t, err)
	res, err := engine.MergeChangeSet(ctx, kgTestWS, h.fake, *cs)
	require.NoError(t, err)
	assert.Equal(t, 1, res.RevisionsCreated, "the merge records a revision of the concept")
	assert.True(t, flag(), "the approved change sets the flag")

	merged, _, err := tb.GetConcept(ctx, "c-kapi")
	require.NoError(t, err)
	rule, ok := terms.RuleForConcept(merged, "en", "fr")
	require.True(t, ok)
	errs, _ := coretools.TermCheckViolations(&coretools.TermCheckConfig{
		TermRules: []coreprofile.TermRule{rule}, SourceLocale: "en", TargetLocale: "fr",
	}, "Open kapi", "Ouvrir capi")
	assert.NotEmpty(t, errs, "the merged flag is enforced")
}
