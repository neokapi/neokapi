package server

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/host/reviewdeck"
)

func TestResolveInvolvement(t *testing.T) {
	overrides := []involvementOverride{
		{Coordinates: map[string]string{"product": "docs"}, Involvement: involvementContext},
		{Coordinates: map[string]string{"product": "app"}, Involvement: involvementInterfaces},
		{Coordinates: map[string]string{"channel": "ui"}, Involvement: involvementEvery},
		{Coordinates: map[string]string{"product": "app", "channel": "email"}, Involvement: involvementContext},
	}
	tests := []struct {
		name   string
		coords map[string]string
		want   string
		index  int
	}{
		{"no override applies", map[string]string{"product": "site"}, involvementInterfaces, -1},
		{"no coordinates", nil, involvementInterfaces, -1},
		{"one axis", map[string]string{"product": "docs", "channel": "reference"}, involvementContext, 0},
		{"more axes win", map[string]string{"product": "app", "channel": "email"}, involvementContext, 3},
		{"a tie goes to the deeper involvement", map[string]string{"product": "app", "channel": "ui"}, involvementEvery, 2},
		{"a partial match does not apply", map[string]string{"channel": "email"}, involvementInterfaces, -1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, i := resolveInvolvement(involvementInterfaces, overrides, tt.coords)
			assert.Equal(t, tt.want, got)
			assert.Equal(t, tt.index, i)
		})
	}
}

// involvementProject seeds a project with two collections, docs (product
// docs) and app (product app, channel ui), each holding one fast, one closer
// and one desk French translation, with nothing in the context open.
func involvementProject(t *testing.T, s *Server, wsID string) (string, map[string]string) {
	t.Helper()
	ctx := context.Background()
	proj := &platstore.Project{
		Name: "Involvement", WorkspaceID: wsID, DefaultSourceLanguage: "en",
		TargetLanguages: []model.LocaleID{"fr"},
		Properties:      map[string]string{"translate_after": "none"},
	}
	require.NoError(t, s.ContentStore.CreateProject(ctx, proj))
	long := strings.Repeat("A long paragraph of source text. ", 10)
	ids := map[string]string{}
	for _, c := range []struct {
		id, item string
		coords   map[string]string
	}{
		{"col-docs", "docs/guide.md", map[string]string{"product": "docs"}},
		{"col-app", "app/strings.json", map[string]string{"product": "app", "channel": "ui"}},
	} {
		require.NoError(t, s.ContentStore.CreateCollection(ctx, &platstore.Collection{
			ID: c.id, ProjectID: proj.ID, Name: c.id, Kind: platstore.CollectionKind("uploaded"),
			ItemLabel: "page", Context: c.coords,
		}))
		require.NoError(t, s.ContentStore.StoreItem(ctx, proj.ID, "main", &platstore.Item{
			Name: c.item, Format: "txt", ItemType: "file", CollectionID: c.id,
		}))
		require.NoError(t, s.ContentStore.StoreBlocksForItem(ctx, proj.ID, "main", c.item, []*model.Block{
			pendingFrBlock(c.id+"-fast", "Fast "+c.id, "Rapide "+c.id),
			pendingFrBlock(c.id+"-closer", "Closer "+c.id, "Proche "+c.id),
			pendingFrBlock(c.id+"-desk", long+c.id, "Un long paragraphe "+c.id),
		}))
		stored, err := s.ContentStore.GetBlocks(ctx, platstore.BlockQuery{ProjectID: proj.ID, Stream: "main", ItemName: c.item})
		require.NoError(t, err)
		for _, sb := range stored {
			ids[sb.Block.SourceText()] = sb.Block.ID
		}
		recordScore(t, s, proj.ID, ids["Fast "+c.id], 92)
	}
	return proj.ID, ids
}

// putOverrides sets the project's overrides as the owner.
func putOverrides(t *testing.T, s *Server, wsID, projID, owner, body string) reviewInvolvementResponse {
	t.Helper()
	rec := planWriteAs(t, s, s.HandlePutReviewInvolvementOverrides, wsID, projID, body, owner, platauth.PermAll)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	return decodeJSON[reviewInvolvementResponse](t, rec)
}

func TestReviewInvolvementOverrides_Validated(t *testing.T) {
	s, wsID, owner := newRecheckHarness(t)
	projID, _ := involvementProject(t, s, wsID)

	for name, body := range map[string]string{
		"an unknown axis":       `{"overrides":[{"coordinates":{"brand":"acme"},"involvement":"every"}]}`,
		"an unknown value":      `{"overrides":[{"coordinates":{"product":"site"},"involvement":"every"}]}`,
		"no axis":               `{"overrides":[{"coordinates":{},"involvement":"every"}]}`,
		"an unknown level":      `{"overrides":[{"coordinates":{"product":"app"},"involvement":"some"}]}`,
		"the same point twice":  `{"overrides":[{"coordinates":{"product":"app"},"involvement":"every"},{"coordinates":{"product":"app"},"involvement":"context"}]}`,
		"a body that is no map": `{"overrides":"every"}`,
	} {
		t.Run(name, func(t *testing.T) {
			rec := planWriteAs(t, s, s.HandlePutReviewInvolvementOverrides, wsID, projID, body, owner, platauth.PermAll)
			assert.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
		})
	}

	rec := planWriteAs(t, s, s.HandlePutReviewInvolvementOverrides, wsID, projID,
		`{"overrides":[{"coordinates":{"product":"app"},"involvement":"every"}]}`, "u-reviewer", testReviewer)
	assert.Equal(t, http.StatusForbidden, rec.Code, "a reviewer does not manage the project")

	got := putOverrides(t, s, wsID, projID, owner,
		`{"overrides":[{"coordinates":{"product":"app","channel":"ui"},"involvement":"every"},{"coordinates":{"product":"docs"},"involvement":"interfaces"}]}`)
	assert.Equal(t, involvementContext, got.Involvement)
	assert.Equal(t, map[string][]string{"channel": {"ui"}, "product": {"app", "docs"}}, got.Axes)
	require.Len(t, got.Overrides, 2)
	assert.Equal(t, map[string]string{"product": "docs"}, got.Overrides[0].Coordinates, "fewer axes first")
	assert.Equal(t, 1, got.Overrides[0].Collections)
	assert.Equal(t, involvementEvery, got.Overrides[1].Involvement)

	proj, err := s.ContentStore.GetProject(t.Context(), projID)
	require.NoError(t, err)
	assert.Equal(t, owner, proj.Properties[propInvolvementSetBy], "the settle pass is credited to the person who set the overrides")

	rec = deckCall(t, s, s.HandleGetReviewInvolvement, wsID, projID, "", "u-reviewer", testReviewer, nil, url.Values{})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Len(t, decodeJSON[reviewInvolvementResponse](t, rec).Overrides, 2)

	assert.Empty(t, putOverrides(t, s, wsID, projID, owner, `{"overrides":[]}`).Overrides, "an empty list removes every override")
}

func TestReviewPlan_HonoursAnEveryOverride(t *testing.T) {
	s, wsID, owner := newRecheckHarness(t)
	projID, _ := involvementProject(t, s, wsID)
	query := url.Values{"locale": {"fr"}}
	readDeck(t, s, wsID, projID, query) // classify the deck

	openContext(t, s)
	plan := readPlan(t, s, wsID, projID)
	require.Len(t, plan.Languages, 1)
	assert.Equal(t, heldContext, plan.Held)
	assert.Equal(t, 4, plan.Languages[0].Held, "the fast and closer translations of both collections")
	assert.Equal(t, 2, plan.Languages[0].Settleable)
	assert.Equal(t, 2, plan.Languages[0].Pages, "one page per collection under context")

	putOverrides(t, s, wsID, projID, owner, `{"overrides":[{"coordinates":{"product":"app"},"involvement":"every"}]}`)
	plan = readPlan(t, s, wsID, projID)
	lang := plan.Languages[0]
	assert.Equal(t, heldContext, plan.Held)
	assert.Equal(t, 2, lang.Held, "the app collection holds nothing back")
	assert.Equal(t, 4, lang.Waiting, "the app's three and the docs' exception")
	assert.Equal(t, 2, lang.Failing)
	assert.Equal(t, 1, lang.Settleable, "the app's fast translation is left to a person")
	require.Len(t, plan.Overrides, 1)
	assert.Equal(t, 1, plan.Overrides[0].Collections)
	for _, p := range plan.Coordinates.Points {
		want := map[string]string{"col-docs": involvementContext, "col-app": involvementEvery}[p.Collection]
		assert.Equal(t, want, p.Involvement, p.Collection)
	}
	assert.NotEqual(t, settleNone, plan.Policy.Settle, "the docs are still left to the checks")

	deck := readDeck(t, s, wsID, projID, query)
	require.NotNil(t, deck.Held)
	assert.Equal(t, 2, deck.Held.Count)
	assert.Equal(t, 4, deck.Counts.Waiting)
	require.Len(t, deck.Cards, 4)
	for _, c := range deck.Cards {
		if c.Group != reviewdeck.DeckDesk {
			assert.Equal(t, "col-app", c.CollectionID, "a held collection deals its exceptions only")
		}
	}
	fast := readDeck(t, s, wsID, projID, url.Values{"locale": {"fr"}, "group": {"fast"}})
	require.Len(t, fast.Cards, 1)
	assert.Equal(t, "col-app", fast.Cards[0].CollectionID)

	app := readDeck(t, s, wsID, projID, url.Values{"locale": {"fr"}, "collection": {"col-app"}})
	assert.Nil(t, app.Held, "the hold does not reach a collection under every")
	assert.Len(t, app.Cards, 3)

	rec := deckCall(t, s, s.HandleGetReviewDeckCollections, wsID, projID, "", "u-reviewer", testReviewer, nil, query)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	colls := decodeJSON[reviewDeckCollectionsResponse](t, rec)
	assert.Equal(t, 4, colls.All.Waiting)
	assert.Equal(t, 2, colls.All.Held)
	for _, c := range colls.Collections {
		switch c.CollectionID {
		case "col-app":
			assert.Equal(t, 3, c.Waiting)
			assert.Zero(t, c.Held)
		case "col-docs":
			assert.Equal(t, 1, c.Waiting)
			assert.Equal(t, 2, c.Held)
		}
	}

	inbox := readInbox(t, s, owner)
	require.Len(t, inbox.Languages, 1)
	assert.Equal(t, 4, inbox.Languages[0].Waiting)
	assert.Equal(t, 2, inbox.Languages[0].Held)
}

func TestReviewPlan_EveryEverywhereLeavesNothingToTheChecks(t *testing.T) {
	s, wsID, owner := newRecheckHarness(t)
	projID, _ := involvementProject(t, s, wsID)
	require.Equal(t, http.StatusNoContent,
		planWriteAs(t, s, s.HandlePutReviewInvolvement, wsID, projID, `{"involvement":"every"}`, owner, platauth.PermAll).Code)
	assert.Equal(t, settleNone, readPlan(t, s, wsID, projID).Policy.Settle)

	// An override under every's default puts the docs back under the checks.
	putOverrides(t, s, wsID, projID, owner, `{"overrides":[{"coordinates":{"product":"docs"},"involvement":"context"}]}`)
	readDeck(t, s, wsID, projID, url.Values{"locale": {"fr"}})
	plan := readPlan(t, s, wsID, projID)
	assert.Equal(t, settlePolicy, plan.Policy.Settle)
	assert.Equal(t, 1, plan.Languages[0].Settleable, "the docs' fast translation")
}

// pendingIn reports, per collection's blocks, whether the French
// translation still waits for review.
func pendingIn(t *testing.T, s *Server, projID string, ids map[string]string, sources ...string) map[string]bool {
	t.Helper()
	out := map[string]bool{}
	for _, src := range sources {
		stored, err := s.ContentStore.GetBlocks(t.Context(), platstore.BlockQuery{ProjectID: projID, Stream: "main", IDs: []string{ids[src]}})
		require.NoError(t, err)
		require.Len(t, stored, 1)
		out[src] = targetPendingReview(stored[0].Block, "fr")
	}
	return out
}

func TestSettleAfterRun_NeverSettlesUnderAnEveryOverride(t *testing.T) {
	s, wsID, owner := newRecheckHarness(t)
	s.subscribeSettleAfterRun()
	projID, ids := involvementProject(t, s, wsID)
	putOverrides(t, s, wsID, projID, owner, `{"overrides":[{"coordinates":{"product":"app","channel":"ui"},"involvement":"every"}]}`)
	require.Equal(t, settlePolicy, readPlanAs(t, s, wsID, projID).Policy.Settle)

	runCompleted(t, s, projID, 1)

	plan := readPlanAs(t, s, wsID, projID)
	require.NotNil(t, plan.Policy.Last)
	assert.True(t, plan.Policy.Last.Automatic)
	assert.Positive(t, plan.Policy.Last.Approved)
	got := pendingIn(t, s, projID, ids, "Fast col-docs", "Fast col-app", "Closer col-app")
	assert.False(t, got["Fast col-docs"], "the docs settle by policy")
	assert.True(t, got["Fast col-app"], "a block under every is never settled by policy")
	assert.True(t, got["Closer col-app"])
}

func TestSettleReviewPlan_ByHandLeavesAnEveryOverrideAlone(t *testing.T) {
	s, wsID, owner := newRecheckHarness(t)
	projID, ids := involvementProject(t, s, wsID)
	putOverrides(t, s, wsID, projID, owner, `{"overrides":[{"coordinates":{"product":"app"},"involvement":"every"}]}`)

	rec := planWrite(t, s, s.HandleSettleReviewPlan, wsID, projID, `{}`, platauth.PermAll)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	got := pendingIn(t, s, projID, ids, "Fast col-docs", "Fast col-app")
	assert.False(t, got["Fast col-docs"])
	assert.True(t, got["Fast col-app"])
}
