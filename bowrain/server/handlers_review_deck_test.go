package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/bowrain/platformconfig"
	bstore "github.com/neokapi/neokapi/bowrain/store"
	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/host/reviewdeck"
	"github.com/neokapi/neokapi/memory"
	"github.com/neokapi/neokapi/terms"
)

// These tests run the review deck and the fix candidates against the real
// PostgreSQL content and auth stores, and send the change sets a card's
// revisions make through the real changes route.

// deckCall calls a review route as the router would: in workspace "rc", as
// user, holding perms, with the query given.
func deckCall(t *testing.T, s *Server, h echo.HandlerFunc, wsID, projID, bid, user string, perms platauth.Permission, langs []string, query url.Values) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(http.MethodGet, "/?"+query.Encode(), nil)
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(r, rec)
	names, values := []string{"ws", "id", "ref"}, []string{"rc", projID, "main"}
	if bid != "" {
		names, values = append(names, "bid"), append(values, bid)
	}
	c.SetParamNames(names...)
	c.SetParamValues(values...)
	c.Set("workspace_id", wsID)
	c.Set("user_id", user)
	c.Set("project_permissions", perms)
	if langs != nil {
		c.Set("project_languages", langs)
	}
	if err := h(c); err != nil {
		require.ErrorIs(t, err, errAccessDenied)
	}
	return rec
}

func readDeck(t *testing.T, s *Server, wsID, projID string, query url.Values) reviewDeckResponse {
	t.Helper()
	rec := deckCall(t, s, s.HandleGetReviewDeck, wsID, projID, "", "u-reviewer", testReviewer, nil, query)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	return decodeJSON[reviewDeckResponse](t, rec)
}

// placeholderBlock is a source with a placeholder and a French draft, with or
// without it.
func placeholderBlock(id, text string, keep bool) *model.Block {
	b := &model.Block{ID: id, Translatable: true}
	b.SetSourceRuns([]model.Run{model.TextR(text + " "), model.PhR(model.PlaceholderRun{ID: "1", Type: "var", Data: "{name}", Equiv: "name"})})
	target := []model.Run{model.TextR("Bonjour ")}
	if keep {
		target = append(target, model.PhR(model.PlaceholderRun{ID: "1", Type: "var", Data: "{name}", Equiv: "name"}))
	}
	b.SetTargetRuns("fr", target)
	b.SetEditionStatus(model.Variant("fr"), model.Status(model.TargetStatusDraft))
	return b
}

func recordScore(t *testing.T, s *Server, projID, bid string, score int) {
	t.Helper()
	cs := s.ContentStore.(*bstore.PostgresStore)
	rev := targetRev(t, cs, projID, bid, "fr")
	require.NoError(t, s.ContentStore.RecordPreReview(context.Background(), projID, "main", platstore.PreReview{
		BlockID: bid, Locale: "fr", Score: score, Reviewer: "agent/test", Revision: rev,
	}))
}

func TestReviewDeck_GroupsOrdersAndPages(t *testing.T) {
	s, wsID, _ := newRecheckHarness(t)
	long := strings.Repeat("A long paragraph of source text. ", 10)
	projID, ids := seedGovernedProject(t, s, wsID, []*model.Block{
		pendingFrBlock("b2", "Closer low", "Bonjour bas"),
		pendingFrBlock("b3", "Fast one", "Bonjour un"),
		pendingFrBlock("b4", "Closer unscored", "Bonjour sans"),
		pendingFrBlock("b5", long, "Un long paragraphe."),
		placeholderBlock("b6", "Hello", false),
		pendingFrBlock("b7", "Fast two", "Bonjour deux"),
	})
	phID := ""
	for text, id := range ids {
		if strings.HasPrefix(text, "Hello") {
			phID = id
		}
	}
	require.NotEmpty(t, phID)
	recordScore(t, s, projID, ids["Fast one"], 92)
	recordScore(t, s, projID, ids["Fast two"], 80)
	recordScore(t, s, projID, ids["Closer low"], 50)
	recordScore(t, s, projID, ids[long], 95)
	recordScore(t, s, projID, phID, 95)

	deck := readDeck(t, s, wsID, projID, url.Values{"locale": {"fr"}})
	assert.Equal(t, deckCounts{Fast: 2, Closer: 2, Desk: 2, Total: 6}, deck.Counts, "%+v", deck.Cards)
	assert.Equal(t, reviewdeck.DefaultDeckPolicy, deck.Policy)

	groupOf := map[string]reviewdeck.DeckGroup{}
	var order []reviewdeck.DeckGroup
	for _, card := range deck.Cards {
		groupOf[card.BlockID] = card.Group
		order = append(order, card.Group)
		assert.NotEmpty(t, card.Reason)
		assert.Equal(t, targetRev(t, s.ContentStore.(*bstore.PostgresStore), projID, card.BlockID, "fr"), card.TargetRevision)
		assert.Equal(t, sourceRev(t, s.ContentStore.(*bstore.PostgresStore), projID, card.BlockID), card.SourceRevision)
		assert.Equal(t, "greetings.txt", card.ItemName)
		assert.NotEmpty(t, card.TargetRuns)
	}
	assert.Equal(t, reviewdeck.DeckFast, groupOf[ids["Fast one"]])
	assert.Equal(t, reviewdeck.DeckFast, groupOf[ids["Fast two"]], "a score at the bar is fast")
	assert.Equal(t, reviewdeck.DeckCloser, groupOf[ids["Closer low"]])
	assert.Equal(t, reviewdeck.DeckCloser, groupOf[ids["Closer unscored"]])
	assert.Equal(t, reviewdeck.DeckDesk, groupOf[ids[long]], "a long source needs the desk")
	assert.Equal(t, reviewdeck.DeckDesk, groupOf[phID], "a lost placeholder is a failing finding")
	assert.IsNonDecreasing(t, groupRanks(order), "fast, then closer, then desk")

	for _, card := range deck.Cards {
		if card.BlockID == phID {
			assert.Positive(t, card.FailingFindingsCount)
			require.Len(t, card.SourceRuns, 2)
			assert.NotNil(t, card.SourceRuns[1].Ph, "a placeholder is a run of its own")
		}
	}

	// Two cards a page, walked by cursor, give the same deck.
	var walked []string
	query := url.Values{"locale": {"fr"}, "limit": {"2"}}
	for range 10 {
		page := readDeck(t, s, wsID, projID, query)
		for _, card := range page.Cards {
			walked = append(walked, card.BlockID)
		}
		if page.NextCursor == "" {
			break
		}
		query.Set("cursor", page.NextCursor)
	}
	var whole []string
	for _, card := range deck.Cards {
		whole = append(whole, card.BlockID)
	}
	assert.Equal(t, whole, walked)

	// One group alone.
	desk := readDeck(t, s, wsID, projID, url.Values{"locale": {"fr"}, "group": {"desk"}})
	assert.Len(t, desk.Cards, 2)
	assert.Equal(t, 6, desk.Counts.Total, "the counts cover the whole deck")
}

func groupRanks(groups []reviewdeck.DeckGroup) []int {
	out := make([]int, len(groups))
	for i, g := range groups {
		for j, want := range reviewdeck.DeckGroups {
			if g == want {
				out[i] = j
			}
		}
	}
	return out
}

func TestReviewDeck_NeedsTheReviewPermissionForTheLanguage(t *testing.T) {
	s, wsID, _ := newRecheckHarness(t)
	projID, _ := seedGovernedProject(t, s, wsID, []*model.Block{pendingFrBlock("b1", "Hello", "Bonjour")})
	q := url.Values{"locale": {"fr"}}

	rec := deckCall(t, s, s.HandleGetReviewDeck, wsID, projID, "", "u-translator", testTranslator, nil, q)
	assert.Equal(t, http.StatusForbidden, rec.Code, rec.Body.String())
	rec = deckCall(t, s, s.HandleGetReviewDeck, wsID, projID, "", "u-reviewer", testReviewer, []string{"de"}, q)
	assert.Equal(t, http.StatusForbidden, rec.Code, rec.Body.String())
	rec = deckCall(t, s, s.HandleGetReviewDeck, wsID, projID, "", "u-reviewer", testReviewer, []string{"fr"}, q)
	assert.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	rec = deckCall(t, s, s.HandleGetReviewDeck, wsID, projID, "", "u-reviewer", testReviewer, nil, url.Values{})
	assert.Equal(t, http.StatusBadRequest, rec.Code, "the locale is required")
}

func TestReviewDeck_MarksTheCallersOwnWork(t *testing.T) {
	s, wsID, _ := newRecheckHarness(t)
	projID, ids := seedGovernedProject(t, s, wsID, []*model.Block{
		pendingFrBlock("b1", "Hello", "Bonjour"),
		pendingFrBlock("b2", "Bye", "Au revoir"),
	})
	attributeTarget(t, s, projID, ids["Hello"], "fr", "u-reviewer", "Bonjour à tous")

	deck := readDeck(t, s, wsID, projID, url.Values{"locale": {"fr"}})
	own := map[string]bool{}
	for _, card := range deck.Cards {
		own[card.BlockID] = card.OwnWork
	}
	assert.True(t, own[ids["Hello"]])
	assert.False(t, own[ids["Bye"]])
}

// fixHarness is the recheck harness with a content memory per workspace and
// the demo provider as the platform's AI backend.
func fixHarness(t *testing.T) (*Server, string) {
	t.Helper()
	s, wsID, _ := newRecheckHarness(t)
	s.wsStores.memoryFactory = func() memory.Store { return &testMemoryStore{memory.NewInMemoryStore()} }
	s.PlatformConfig = platformconfig.NewInMemoryService(platformconfig.Defaults{AIProvider: "demo"}, nil)
	return s, wsID
}

func readFixes(t *testing.T, s *Server, wsID, projID, bid string, q url.Values) fixCandidatesResponse {
	t.Helper()
	rec := deckCall(t, s, s.HandleGetFixCandidates, wsID, projID, bid, "u-reviewer", testReviewer, nil, q)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	return decodeJSON[fixCandidatesResponse](t, rec)
}

func TestFixCandidates_TermMemoryAndAI(t *testing.T) {
	s, wsID := fixHarness(t)
	ctx := context.Background()
	b := placeholderBlock("b1", "Open the workspace", true)
	b.SetTargetRuns("fr", []model.Run{model.TextR("Ouvrez votre atelier "), model.PhR(model.PlaceholderRun{ID: "1", Type: "var", Data: "{name}", Equiv: "name"})})
	projID, ids := seedGovernedProject(t, s, wsID, []*model.Block{b})
	bid := ids["Open the workspace "]
	require.NotEmpty(t, bid, "%v", ids)

	tb, err := s.wsStores.getTerms(ctx, "rc")
	require.NoError(t, err)
	require.NoError(t, tb.AddConcept(ctx, terms.Concept{ID: "c-ws", Terms: []terms.Term{
		{Text: "workspace", Locale: "en", Status: model.TermPreferred},
		{Text: "espace de travail", Locale: "fr", Status: model.TermPreferred},
		{Text: "atelier", Locale: "fr", Status: model.TermForbidden},
	}}))
	tm, err := s.wsStores.getMemory(ctx, "rc")
	require.NoError(t, err)
	sb, err := s.ContentStore.GetBlock(ctx, projID, "main", bid)
	require.NoError(t, err)
	require.NoError(t, tm.Add(ctx, memory.Entry{ID: "m1", Variants: map[model.LocaleID][]model.Run{
		"en": sb.Block.SourceRuns(),
		"fr": {model.TextR("Ouvrez l'espace de travail "), model.PhR(model.PlaceholderRun{ID: "1", Type: "var", Data: "{name}", Equiv: "name"})},
	}}))
	require.NoError(t, tm.Add(ctx, memory.Entry{ID: "m2", Variants: map[model.LocaleID][]model.Run{
		"en": sb.Block.SourceRuns(),
		"fr": {model.TextR("Ouvrez l'espace")},
	}}))

	res := readFixes(t, s, wsID, projID, bid, url.Values{"locale": {"fr"}, "reason": {"wrong_term"}})
	assert.Equal(t, targetRev(t, s.ContentStore.(*bstore.PostgresStore), projID, bid, "fr"), res.TargetRevision)
	assert.Equal(t, "Ouvrez votre atelier {name}", res.CurrentText)
	kinds := map[reviewdeck.FixKind]reviewdeck.FixCandidate{}
	for _, c := range res.Candidates {
		kinds[c.Kind] = c
		assert.True(t, model.DiffRunCodes(sb.Block.SourceRuns(), c.Runs).Balanced(), "every candidate keeps the placeholder: %s", c.Text)
	}
	require.Contains(t, kinds, reviewdeck.FixTerm, "%+v", res)
	assert.Equal(t, "Ouvrez votre espace de travail {name}", kinds[reviewdeck.FixTerm].Text)
	assert.Contains(t, kinds, reviewdeck.FixMemory)
	assert.Equal(t, reviewdeck.FixTerm, res.Candidates[0].Kind, "a wrong term puts the term fix first")
	assert.NotContains(t, kinds, reviewdeck.FixAI, "no AI rewrite unless asked")
	require.NotEmpty(t, res.Dropped, "the memory match that lost its placeholder is dropped")
	assert.Equal(t, reviewdeck.FixMemory, res.Dropped[0].Kind)

	withAI := readFixes(t, s, wsID, projID, bid, url.Values{"locale": {"fr"}, "reason": {"tone"}, "ai": {"true"}})
	var ai *reviewdeck.FixCandidate
	for i := range withAI.Candidates {
		if withAI.Candidates[i].Kind == reviewdeck.FixAI {
			ai = &withAI.Candidates[i]
		}
	}
	require.NotNil(t, ai, "%+v", withAI)
	assert.Equal(t, []reviewdeck.FixReason{reviewdeck.FixReasonTone}, ai.Reasons)

	// The term fix, sent as the card's correction and approval in one change
	// set, lands and settles the block.
	fr := at("greetings.txt", bid, "fr")
	reviewer := changeCaller{user: "u-reviewer", perms: testReviewer, ws: wsID}
	fixText := kinds[reviewdeck.FixTerm].EditText
	assert.Equal(t, `Ouvrez votre espace de travail <x id="1/"/>`, fixText)
	rec, out := sendChanges(t, s, projID, reviewer, change.Set{RequireBasis: true, Ops: []change.Op{
		{Kind: change.KindSetContent, At: fr, IfMatch: res.TargetRevision, Basis: res.SourceRevision,
			Body: &change.SetContent{Text: &fixText}},
		{Kind: change.KindDecide, At: fr, IfMatch: res.TargetRevision, Basis: res.SourceRevision,
			Body: &change.Decide{Outcome: change.OutcomeEstablish, Reasons: []string{"wrong_term"}}},
	}})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Equal(t, change.SetApplied, out.Status, "%+v", out.Ops)
	stored := getStoredBlock(t, s.ContentStore.(*bstore.PostgresStore), projID, bid)
	assert.Equal(t, "Ouvrez votre espace de travail {name}", model.RenderRunsWithData(stored.TargetRuns("fr")))
	assert.Equal(t, model.TargetStatusEstablished, targetStatusOf(t, stored, "fr"))
}

func TestFixCandidates_NeedsTheReviewPermission(t *testing.T) {
	s, wsID := fixHarness(t)
	projID, ids := seedGovernedProject(t, s, wsID, []*model.Block{pendingFrBlock("b1", "Hello", "Bonjour")})
	rec := deckCall(t, s, s.HandleGetFixCandidates, wsID, projID, ids["Hello"], "u-translator", testTranslator, nil, url.Values{"locale": {"fr"}})
	assert.Equal(t, http.StatusForbidden, rec.Code)
	rec = deckCall(t, s, s.HandleGetFixCandidates, wsID, projID, ids["Hello"], "u-reviewer", testReviewer, nil, url.Values{"locale": {"fr"}, "reason": {"rude"}})
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

// The change contract the card's decisions go through: a decision bound to a
// source that moved is stale, and a correction that loses a placeholder is a
// guard refusal.
func TestReviewDeck_CardChangeSetsAreGuarded(t *testing.T) {
	s, wsID, _ := newRecheckHarness(t)
	projID, ids := seedGovernedProject(t, s, wsID, []*model.Block{placeholderBlock("b1", "Hello", true)})
	bid := ids["Hello "]
	require.NotEmpty(t, bid, "%v", ids)
	deck := readDeck(t, s, wsID, projID, url.Values{"locale": {"fr"}})
	require.Len(t, deck.Cards, 1)
	card := deck.Cards[0]
	fr := at("greetings.txt", bid, "fr")
	reviewer := changeCaller{user: "u-reviewer", perms: testReviewer, ws: wsID}

	// A placeholder dropped fails the placeholder check the edit introduces; a
	// code the translation and the source do not hold is the inline-code guard.
	for _, tc := range []struct {
		name    string
		text    string
		code    change.Code
		subcode change.Subcode
	}{
		{"a correction that drops the placeholder fails the placeholder check", "Salut", change.CodeGateFailed, ""},
		{"a correction with a code nobody holds is refused by the guard", `Salut <x id="9/"/>`, change.CodeGuard, change.SubcodeCodesChanged},
	} {
		t.Run(tc.name, func(t *testing.T) {
			text := tc.text
			rec, res := sendChanges(t, s, projID, reviewer, change.Set{RequireBasis: true, Ops: []change.Op{
				{Kind: change.KindSetContent, At: fr, IfMatch: card.TargetRevision, Basis: card.SourceRevision,
					Body: &change.SetContent{Text: &text}},
				{Kind: change.KindDecide, At: fr, IfMatch: card.TargetRevision, Basis: card.SourceRevision,
					Body: &change.Decide{Outcome: change.OutcomeEstablish}},
			}})
			require.Equal(t, http.StatusUnprocessableEntity, rec.Code, rec.Body.String())
			require.Equal(t, change.SetRefused, res.Status)
			require.NotNil(t, res.Ops[0].Error)
			assert.Equal(t, tc.code, res.Ops[0].Error.Code, res.Ops[0].Error.Message)
			assert.Equal(t, tc.subcode, res.Ops[0].Error.Subcode)
			stored := getStoredBlock(t, s.ContentStore.(*bstore.PostgresStore), projID, bid)
			assert.Equal(t, "Bonjour {name}", model.RenderRunsWithData(stored.TargetRuns("fr")), "nothing lands")
		})
	}

	t.Run("an approval after the source moved is stale on its basis", func(t *testing.T) {
		src := at("greetings.txt", bid, "")
		owner := changeCaller{user: "owner-rc", perms: platauth.PermAll, ws: wsID}
		rec, res := sendChanges(t, s, projID, owner, change.Set{Ops: []change.Op{
			{Kind: change.KindSetContent, At: src, IfMatch: card.SourceRevision,
				Body: &change.SetContent{Runs: []model.Run{model.TextR("Hello there "), model.PhR(model.PlaceholderRun{ID: "1"})}}},
		}})
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		require.Equal(t, change.SetApplied, res.Status, "%+v", res.Ops)

		rec, res = sendChanges(t, s, projID, reviewer, change.Set{RequireBasis: true, Ops: []change.Op{
			{Kind: change.KindDecide, At: fr, IfMatch: card.TargetRevision, Basis: card.SourceRevision,
				Body: &change.Decide{Outcome: change.OutcomeEstablish}},
		}})
		require.Equal(t, http.StatusConflict, rec.Code, rec.Body.String())
		require.NotNil(t, res.Ops[0].Error)
		assert.Equal(t, change.CodeStale, res.Ops[0].Error.Code)
		assert.Equal(t, "basis", res.Ops[0].Error.Field)
	})
}

// A rejected translation goes back for a new draft. It leaves the deck and its
// counts until the new draft lands, and then returns at the desk.
func TestReviewDeck_RejectedTranslationWaitsForANewDraft(t *testing.T) {
	s, wsID, _ := newRecheckHarness(t)
	projID, ids := seedGovernedProject(t, s, wsID, []*model.Block{
		pendingFrBlock("b1", "Hello", "Bonjour"),
		pendingFrBlock("b2", "Bye", "Au revoir"),
	})
	q := url.Values{"locale": {"fr"}}
	deck := readDeck(t, s, wsID, projID, q)
	require.Equal(t, 2, deck.Counts.Total, "%+v", deck.Cards)

	var card deckCard
	for _, c := range deck.Cards {
		if c.BlockID == ids["Hello"] {
			card = c
		}
	}
	require.NotEmpty(t, card.BlockID)
	reviewer := changeCaller{user: "u-reviewer", perms: testReviewer, ws: wsID}
	rec, res := sendChanges(t, s, projID, reviewer, change.Set{RequireBasis: true, Ops: []change.Op{
		{Kind: change.KindDecide, At: at(card.ItemName, card.BlockID, "fr"), IfMatch: card.TargetRevision,
			Basis: card.SourceRevision, Body: &change.Decide{Outcome: change.OutcomeReject}},
	}})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Equal(t, change.SetApplied, res.Status, "%+v", res.Ops)

	deck = readDeck(t, s, wsID, projID, q)
	assert.Equal(t, 1, deck.Counts.Total, "a rejected translation is not waiting for review")
	assert.Zero(t, deck.Counts.Unclassified)
	require.Len(t, deck.Cards, 1)
	assert.Equal(t, ids["Bye"], deck.Cards[0].BlockID)

	// The new draft brings it back, at the desk.
	attributeTarget(t, s, projID, ids["Hello"], "fr", "u-translator", "Salut")
	deck = readDeck(t, s, wsID, projID, q)
	assert.Equal(t, 2, deck.Counts.Total, "%+v", deck.Cards)
	groupOf := map[string]reviewdeck.DeckGroup{}
	for _, c := range deck.Cards {
		groupOf[c.BlockID] = c.Group
	}
	assert.Equal(t, reviewdeck.DeckDesk, groupOf[ids["Hello"]], "a new draft after a rejection needs the desk")
}
