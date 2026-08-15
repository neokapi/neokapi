package server

import (
	"net/http"
	"net/url"
	"testing"

	bowsync "github.com/neokapi/neokapi/bowrain/sync"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/bowrain/apierror"
	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	"github.com/neokapi/neokapi/bowrain/knowledge"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/ref"
	"github.com/neokapi/neokapi/terms"
)

// The terms component's compare-and-swap, on the write that carries a
// terminology proposal into review.
//
// A change-set is a DIFF: the client built its ops by comparing local
// terminology against what it last pulled. Submitting it is the moment a
// reviewer will act on it, so a workspace whose terminology moved since makes
// the proposal a description of a state that no longer exists.

// seedConcept writes one concept into the harness's workspace terminology.
func seedConcept(t *testing.T, tb terms.Store, id, text, definition string) {
	t.Helper()
	require.NoError(t, tb.AddConcept(t.Context(), terms.Concept{
		ID: id, Domain: "product", Definition: definition,
		Terms: []terms.Term{{Text: text, Locale: model.LocaleEnglish, Status: model.TermPreferred}},
	}))
}

// termsRef reads the terms component the workspace currently stands at.
func termsRef(t *testing.T, h *kgHarness) string {
	t.Helper()
	tb, err := h.srv.wsStores.getTerms(kgTestWS)
	require.NoError(t, err)
	component, err := bowsync.TermsComponentOf(t.Context(), tb)
	require.NoError(t, err)
	return component
}

// draftWithOneOp seeds a submittable draft change-set carrying a governed op.
func draftWithOneOp(t *testing.T, h *kgHarness, id string) {
	t.Helper()
	ctx := t.Context()
	require.NoError(t, h.fake.CreateChangeSet(ctx, &knowledge.ChangeSet{
		ID: id, WorkspaceID: kgTestWS, Name: "kapi push", Status: knowledge.ChangeSetDraft,
		CreatedBy: "ana", Origin: "kapi-push/concepts",
	}))
	require.NoError(t, h.fake.AppendOp(ctx, &knowledge.ChangeSetOp{
		ChangesetID: id, WorkspaceID: kgTestWS, Op: knowledge.OpTermStatus,
		Payload: []byte(`{"concept_id":"c1","locale":"en","text":"convergence","status":"forbidden"}`),
	}))
}

// submit posts the submit route with an optional terms assertion, returning the
// status and the body a client would render.
func submit(t *testing.T, h *kgHarness, changesetID, expectedTerms string) (int, string) {
	t.Helper()
	target := "/"
	if expectedTerms != "" {
		target += "?expected_terms_ref=" + url.QueryEscape(expectedTerms)
	}
	c, rec := h.req(http.MethodPost, target, "", platauth.PermAll, "id", changesetID)
	withActor(c, "ana")
	require.NoError(t, h.srv.HandleSubmitChangeSet(c))
	return rec.Code, rec.Body.String()
}

// TestSubmitChangeSet_StaleTermsComponentIsRefused is the two-client case: both
// pull the same terminology, one edit lands in the workspace, and the other's
// proposal — a diff against what it pulled — is refused rather than queued for a
// reviewer who would be approving an intention nobody now holds.
func TestSubmitChangeSet_StaleTermsComponentIsRefused(t *testing.T) {
	h := newKGHarness(t)
	tb := h.tb(t)
	seedConcept(t, tb, "c1", "convergence", "The loop.")

	observed := termsRef(t, h)
	require.NotEmpty(t, observed)

	// Another client's edit lands in the workspace.
	seedConcept(t, tb, "c2", "voice", "A profile.")
	require.NotEqual(t, observed, termsRef(t, h))

	draftWithOneOp(t, h, "cs-stale")
	code, body := submit(t, h, "cs-stale", observed)
	require.Equal(t, http.StatusConflict, code)
	assert.Contains(t, body, apierror.CodeGovernanceMoved)
	assert.Contains(t, body, string(ref.ComponentTerms))
	assert.Contains(t, body, "Pull first")
}

// TestSubmitChangeSet_CurrentTermsComponentPasses: the same proposal against
// the terminology actually in force goes through.
func TestSubmitChangeSet_CurrentTermsComponentPasses(t *testing.T) {
	h := newKGHarness(t)
	seedConcept(t, h.tb(t), "c1", "convergence", "The loop.")

	draftWithOneOp(t, h, "cs-current")
	code, _ := submit(t, h, "cs-current", termsRef(t, h))
	assert.Equal(t, http.StatusOK, code)
}

// TestSubmitChangeSet_AnUnassertingClientIsAccepted is the backward-tolerance
// contract: a client that predates the ref sends no assertion and submits
// exactly as it always did.
func TestSubmitChangeSet_AnUnassertingClientIsAccepted(t *testing.T) {
	h := newKGHarness(t)
	seedConcept(t, h.tb(t), "c1", "convergence", "The loop.")
	seedConcept(t, h.tb(t), "c2", "voice", "A profile.")

	draftWithOneOp(t, h, "cs-silent")
	code, _ := submit(t, h, "cs-silent", "")
	assert.Equal(t, http.StatusOK, code)
}
