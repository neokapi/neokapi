package server

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	platev "github.com/neokapi/neokapi/bowrain/core/event"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These tests cover the sign-off rung on the platform review endpoint: an
// approving call carrying status:"signed-off" promotes the target one rung
// above reviewed, under the same permission and separation-of-duties rules as
// an approval. They run against the real PostgreSQL content store, so every
// assertion covers the handler → StoreBlocks → overlay round-trip.

// signOffBody is the request body for signing one locale off.
func signOffBody(locale string) string {
	return fmt.Sprintf(`{"target_locale":%q,"reviewed":true,"status":"signed-off","item_name":"greetings.txt"}`, locale)
}

// TestHandleReviewBlockSignOffRungs walks every (starting rung, request) pair
// the endpoint answers for, so the sign-off path and the approval path are
// pinned against each other rather than one at a time.
func TestHandleReviewBlockSignOffRungs(t *testing.T) {
	cases := []struct {
		name string
		// from is the rung the fr target starts at, and target is its text: an
		// empty text means the locale has no reviewable translation at all.
		from   model.TargetStatus
		target string
		body   string
		code   int
		want   model.TargetStatus
	}{
		{
			name: "sign off from translated",
			from: model.TargetStatusTranslated, target: "Bonjour",
			body: signOffBody("fr"), code: http.StatusOK, want: model.TargetStatusSignedOff,
		},
		{
			name: "sign off from reviewed",
			from: model.TargetStatusReviewed, target: "Bonjour",
			body: signOffBody("fr"), code: http.StatusOK, want: model.TargetStatusSignedOff,
		},
		{
			name: "re-signing a signed-off target keeps the rung",
			from: model.TargetStatusSignedOff, target: "Bonjour",
			body: signOffBody("fr"), code: http.StatusOK, want: model.TargetStatusSignedOff,
		},
		{
			name: "approving a signed-off target keeps the rung",
			from: model.TargetStatusSignedOff, target: "Bonjour",
			body: `{"target_locale":"fr","reviewed":true,"item_name":"greetings.txt"}`,
			code: http.StatusOK, want: model.TargetStatusSignedOff,
		},
		{
			name: "an approval with no status still lands on reviewed",
			from: model.TargetStatusTranslated, target: "Bonjour",
			body: `{"target_locale":"fr","reviewed":true,"item_name":"greetings.txt"}`,
			code: http.StatusOK, want: model.TargetStatusReviewed,
		},
		{
			name: "a demotion rung on an approval is refused",
			from: model.TargetStatusTranslated, target: "Bonjour",
			body: `{"target_locale":"fr","reviewed":true,"status":"draft","item_name":"greetings.txt"}`,
			code: http.StatusBadRequest, want: model.TargetStatusTranslated,
		},
		{
			name: "signing off a locale with no translation is unprocessable",
			from: model.TargetStatusTranslated, target: "",
			body: signOffBody("fr"), code: http.StatusUnprocessableEntity, want: "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv, cs := newReviewTestServer(t)

			b := &model.Block{ID: "b1", Translatable: true}
			b.SetSourceText("Hello")
			if tc.target != "" {
				b.SetTargetText("fr", tc.target)
				b.Target("fr").Status = tc.from
			}
			b.SetTargetText("de", "Hallo")
			pid, ids := seedReviewProject(t, cs, []*model.Block{b})
			bid := ids["Hello"]

			rec, err := callReviewBlockBodyAs(t, srv, pid, bid, tc.body, platauth.PermAll)
			require.NoError(t, err)
			require.Equal(t, tc.code, rec.Code, rec.Body.String())

			got := getStoredBlock(t, cs, pid, bid)
			if tc.want == "" {
				assert.Nil(t, got.Target("fr"), "a refused decision must not create a target")
			} else {
				require.NotNil(t, got.Target("fr"))
				assert.Equal(t, tc.want, got.Target("fr").Status)
				assert.Equal(t, tc.target, got.TargetText("fr"),
					"a review decision must not touch the translation text")
			}
			assert.Equal(t, model.TargetStatusNew, got.Target("de").Status,
				"deciding fr must leave de alone")
		})
	}
}

// TestHandleReviewBlockSignOffResponse: the response reports the rung the
// target now holds, so a client that drew the optimistic rung can reconcile.
func TestHandleReviewBlockSignOffResponse(t *testing.T) {
	srv, cs := newReviewTestServer(t)

	b := &model.Block{ID: "b1", Translatable: true}
	b.SetSourceText("Hello")
	b.SetTargetText("fr", "Bonjour")
	b.Target("fr").Status = model.TargetStatusTranslated
	pid, ids := seedReviewProject(t, cs, []*model.Block{b})

	rec, err := callReviewBlockBodyAs(t, srv, pid, ids["Hello"], signOffBody("fr"), platauth.PermAll)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Contains(t, rec.Body.String(), `"status":"signed-off"`)
	assert.Contains(t, rec.Body.String(), `"reviewed":true`)
}

// TestReviewSignOffNeedsReviewPermission: signing off is the review permission
// for the language, the same gate as approving. A translator is refused and the
// rung is untouched.
func TestReviewSignOffNeedsReviewPermission(t *testing.T) {
	s, wsID, _ := newRecheckHarness(t)
	projID, ids := seedGovernedProject(t, s, wsID, []*model.Block{
		pendingFrBlock("b1", "Hello", "Bonjour"),
	})
	bid := ids["Hello"]

	rec := callReviewBlockGoverned(t, s, wsID, projID, bid, signOffBody("fr"), testTranslator, "u-translator")
	require.Equal(t, http.StatusForbidden, rec.Code, rec.Body.String())
	assert.Equal(t, model.TargetStatusDraft, targetStatus(t, s, projID, bid, "fr"),
		"a refused sign-off must leave the rung alone")

	rec = callReviewBlockGoverned(t, s, wsID, projID, bid, signOffBody("fr"), testReviewer, "u-reviewer")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, model.TargetStatusSignedOff, targetStatus(t, s, projID, bid, "fr"))
}

// TestReviewSignOffSoDBlocksOwnWork: separation of duties is one rule for every
// promotion, so a reviewer signing off the wording they wrote is refused under
// a blocking policy, and somebody else may sign the same target off. Signing
// off a target ALREADY at reviewed is a fresh decision and is vetted too.
func TestReviewSignOffSoDBlocksOwnWork(t *testing.T) {
	s, wsID, _ := newRecheckHarness(t)
	require.NoError(t, s.AuthStore.SetSoDMode(context.Background(), wsID, platauth.SoDBlock))

	projID, ids := seedGovernedProject(t, s, wsID, []*model.Block{
		pendingFrBlock("b1", "Hello", "Bonjour"),
	})
	bid := ids["Hello"]
	attributeTarget(t, s, projID, bid, "fr", "u-reviewer", "Bonjour à tous")

	rec := callReviewBlockGoverned(t, s, wsID, projID, bid, signOffBody("fr"), testReviewer, "u-reviewer")
	require.Equal(t, http.StatusForbidden, rec.Code, rec.Body.String())
	assert.Contains(t, rec.Body.String(), sodRefusal)
	assert.Equal(t, model.TargetStatusDraft, targetStatus(t, s, projID, bid, "fr"),
		"a blocked self-sign-off must leave the rung alone")

	// Somebody else approves it, taking it to reviewed.
	rec = callReviewBlockGoverned(t, s, wsID, projID, bid, approveBody("fr"), testReviewer, "u-other")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Equal(t, model.TargetStatusReviewed, targetStatus(t, s, projID, bid, "fr"))

	// The author still may not sign it off: the rung above reviewed is another
	// promotion, and the policy judges the author of the wording either way.
	rec = callReviewBlockGoverned(t, s, wsID, projID, bid, signOffBody("fr"), testReviewer, "u-reviewer")
	require.Equal(t, http.StatusForbidden, rec.Code, rec.Body.String())
	assert.Equal(t, model.TargetStatusReviewed, targetStatus(t, s, projID, bid, "fr"))

	rec = callReviewBlockGoverned(t, s, wsID, projID, bid, signOffBody("fr"), testReviewer, "u-other")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, model.TargetStatusSignedOff, targetStatus(t, s, projID, bid, "fr"))
}

// TestReviewSignOffIsAudited: the audit record names the decision sign-off
// rather than approval, and carries the rungs the target moved between. The
// decision ledger writes the same verdict for the content pipeline.
func TestReviewSignOffIsAudited(t *testing.T) {
	s, wsID, _ := newRecheckHarness(t)
	projID, ids := seedGovernedProject(t, s, wsID, []*model.Block{
		pendingFrBlock("b1", "Hello", "Bonjour"),
	})
	bid := ids["Hello"]

	snapshot, stop := collectEvents(t, s)
	defer stop()

	rec := callReviewBlockGoverned(t, s, wsID, projID, bid, signOffBody("fr"), testReviewer, "u-reviewer")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	require.Eventually(t, func() bool {
		ev, ok := findEvent(snapshot(), platev.EventReviewDecided)
		return ok && ev.Data["decision"] == "signed-off"
	}, 2*time.Second, 20*time.Millisecond, "the audit row must say sign-off, not approval")

	ev, _ := findEvent(snapshot(), platev.EventReviewDecided)
	assert.Equal(t, "u-reviewer", ev.Actor)
	assert.Equal(t, projID, ev.ProjectID)
	assert.Equal(t, "block", ev.ResourceType)
	assert.Equal(t, bid, ev.ResourceID)
	assert.Equal(t, "fr", ev.Data["locale"])
	assert.Equal(t, "main", ev.Data["stream"])
	assert.Equal(t, string(model.TargetStatusDraft), ev.Before["status"])
	assert.Equal(t, string(model.TargetStatusSignedOff), ev.After["status"])
}

// TestReviewSignOffLedgerRow: the ledger row a sign-off writes carries the
// signed-off review state, so the pull that carries decisions home says what
// was decided rather than reporting an approval.
func TestReviewSignOffLedgerRow(t *testing.T) {
	srv, cs := newReviewTestServer(t)

	b := &model.Block{ID: "b1", Translatable: true}
	b.SetSourceText("Hello")
	b.SetTargetText("fr", "Bonjour")
	b.Target("fr").Status = model.TargetStatusTranslated
	pid, ids := seedReviewProject(t, cs, []*model.Block{b})
	bid := ids["Hello"]

	rec, err := callReviewBlockBodyAs(t, srv, pid, bid, signOffBody("fr"), platauth.PermAll)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	decisions, err := cs.ListUnitDecisions(t.Context(), pid, "main")
	require.NoError(t, err)
	require.Len(t, decisions, 1)
	assert.Equal(t, "signed-off", decisions[0].ReviewState)
	assert.Equal(t, string(model.TargetStatusSignedOff), decisions[0].Status)
	assert.Equal(t, "fr", decisions[0].Variant)
}
