package server

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/bowrain/knowledge"
	"github.com/neokapi/neokapi/bowrain/store/sqlitestore"
	"github.com/neokapi/neokapi/core/contextop"
	"github.com/neokapi/neokapi/core/model"
	coreprofile "github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/core/workspace"
	"github.com/neokapi/neokapi/terms"
)

// deckSQLiteServer serves the context object, Decide and context deck routes
// over a SQLite content store, an in-memory terms store holding one concept
// and an in-memory knowledge store, signed in as whoever caller names.
func deckSQLiteServer(t *testing.T, caller *decideCaller) (*echo.Echo, *sqlitestore.SQLiteStore, *fakeKnowledgeStore) {
	t.Helper()
	cs, err := sqlitestore.NewSQLiteStore(filepath.Join(t.TempDir(), "content.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = cs.Close() })
	ks := newFakeKnowledgeStore()
	// Another member wrote the concept, so the caller may keep it as a peer.
	ks.latest = map[string]*knowledge.ConceptRevision{"c-memory": {ConceptID: "c-memory", Rev: 1, Actor: "member-b"}}
	srv := &Server{ContentStore: cs, wsStores: newWorkspaceStores(), KnowledgeStore: ks}
	tb := &testTermStore{terms.NewInMemoryStore()}
	srv.wsStores.termsFactory = func() terms.Store { return tb }
	require.NoError(t, tb.AddConcept(t.Context(), terms.Concept{
		ID: "c-memory", Definition: "The store of source and target pairs.",
		Terms: []terms.Term{
			{Text: "innhaldsminne", Locale: "nb", Status: model.TermPreferred},
			{Text: "content memory", Locale: "en", Status: model.TermPreferred},
			{Text: "translation memory", Locale: "en", Status: model.TermForbidden},
		},
	}))
	e := echo.New()
	g := e.Group("/api/v1/:ws", func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			c.Set("user_id", caller.userID)
			c.Set("workspace_id", "ws")
			c.Set("project_permissions", caller.perms)
			c.Set("project_coordinates", caller.reach)
			return next(c)
		}
	})
	srv.registerContextObjectRoutes(g)
	srv.registerRulesDecideRoutes(g)
	srv.registerContextDeckRoutes(g)
	return e, cs, ks
}

func readContextDeck(t *testing.T, e *echo.Echo, pid, query string) contextDeckResponse {
	t.Helper()
	var deck contextDeckResponse
	require.Equal(t, http.StatusOK, decideRoute(t, e, http.MethodGet, pid, "/context/deck"+query, nil, &deck))
	return deck
}

func deckCardWhere(t *testing.T, deck contextDeckResponse, match func(contextDeckCard) bool) contextDeckCard {
	t.Helper()
	for _, c := range deck.Cards {
		if match(c) {
			return c
		}
	}
	require.FailNow(t, "no card matches")
	return contextDeckCard{}
}

func termCard(term, replacement string) func(contextDeckCard) bool {
	return func(c contextDeckCard) bool {
		return c.Term != nil && c.Term.Term == term && c.Term.Replacement == replacement
	}
}

func TestContextDeck_ListsPendingItems_SQLite(t *testing.T) {
	caller := &decideCaller{userID: "member-a", perms: decideAll}
	e, cs, _ := deckSQLiteServer(t, caller)
	pid := seedDecideParity(t, e, cs)

	deck := readContextDeck(t, e, pid, "")
	assert.True(t, deck.Bound)
	assert.Equal(t, "pending", deck.Mode)
	assert.Equal(t, 6, deck.Counts.Total)
	assert.Equal(t, map[string]int{"voice_rule": 0, "concept": 1, "term_rule": 4, "note": 1}, deck.Counts.ByKind)
	assert.Equal(t, 4, deck.Counts.Agent)
	assert.Equal(t, 2, deck.Counts.Person)
	assert.Equal(t, "concept", deck.Cards[0].Kind, "terms come before the ledger's rules")

	concept := deck.Cards[0]
	require.Len(t, concept.Terms, 3)
	assert.Equal(t, "en", concept.Terms[0].Locale, "the source language leads")
	assert.Equal(t, "en", concept.Language)

	signIn := deckCardWhere(t, deck, termCard("Sign In", "Sign in"))
	assert.Equal(t, "suggested", signIn.Status)
	assert.True(t, signIn.Author.AgentAuthored)
	assert.Equal(t, "s-parity", signIn.Author.Session)
	assert.Equal(t, contextop.BasisAgentAuthored, signIn.Basis)
	assert.Equal(t, []string{"keep", "reject", "later"}, signIn.Actions)
	assert.Equal(t, map[string]string{"brand": "acme", "surface": "docs"}, signIn.Where.Coordinates)

	contested := deckCardWhere(t, deck, termCard("utilise", "use"))
	assert.NotContains(t, contested.Actions, "keep", "a contested rule is settled in Decide first")

	note := deckCardWhere(t, deck, func(c contextDeckCard) bool { return c.Kind == "note" })
	assert.True(t, note.OwnWork, "the reader wrote the note by hand")
	assert.NotContains(t, note.Actions, "keep", "a person's own item waits for another person")
	assert.Contains(t, note.Actions, "reject")

	page := readContextDeck(t, e, pid, "?limit=2")
	assert.Len(t, page.Cards, 2)
	assert.Equal(t, "2", page.NextCursor)
	rest := readContextDeck(t, e, pid, "?limit=10&cursor=2")
	assert.Len(t, rest.Cards, 4)
	assert.Empty(t, rest.NextCursor)

	notes := readContextDeck(t, e, pid, "?kind=note")
	assert.Len(t, notes.Cards, 1)
	assert.Equal(t, 6, notes.Counts.Total, "the counts are the whole deck's")

	var refused ErrorResponse
	assert.Equal(t, http.StatusBadRequest, decideRoute(t, e, http.MethodGet, pid, "/context/deck?mode=replay", nil, &refused))
}

func TestContextDeck_Decide_SQLite(t *testing.T) {
	caller := &decideCaller{userID: "member-a", perms: decideAll}
	e, cs, ks := deckSQLiteServer(t, caller)
	pid := seedDecideParity(t, e, cs)
	deck := readContextDeck(t, e, pid, "")

	decide := func(card contextDeckCard, action, revision string, out any) int {
		t.Helper()
		return decideRoute(t, e, http.MethodPost, pid, "/context/deck/decide",
			contextDeckDecideRequest{Item: card.Item, Revision: revision, Action: action}, out)
	}

	tests := []struct {
		name     string
		card     func(contextDeckCard) bool
		action   string
		revision func(contextDeckCard) string
		status   int
		outcome  string
		code     string
	}{
		{
			name: "a stale revision is refused", card: termCard("gadget", "widget"), action: "keep",
			revision: func(contextDeckCard) string { return "not-the-revision" }, status: http.StatusConflict, code: deckCodeStale,
		},
		{
			name: "keeping a rule in force attests it", card: termCard("gadget", "widget"), action: "keep",
			status: http.StatusOK, outcome: deckOutcomeAttested,
		},
		{
			name: "it then waits no longer", card: termCard("gadget", "widget"), action: "keep",
			status: http.StatusConflict, code: deckCodeNotPending,
		},
		{
			name: "keeping a suggestion keeps and attests it", card: termCard("Sign In", "Sign in"), action: "keep",
			status: http.StatusOK, outcome: deckOutcomeKept,
		},
		{
			name: "a person's own note cannot be kept by them", card: func(c contextDeckCard) bool { return c.Kind == "note" }, action: "keep",
			status: http.StatusForbidden, code: deckCodeNotOffered,
		},
		{
			name: "rejecting the note drops it", card: func(c contextDeckCard) bool { return c.Kind == "note" }, action: "reject",
			status: http.StatusOK, outcome: deckOutcomeDropped,
		},
		{
			name: "keeping a concept attests its revision", card: func(c contextDeckCard) bool { return c.Kind == "concept" }, action: "keep",
			status: http.StatusOK, outcome: deckOutcomeAttested,
		},
		{
			name: "later records nothing", card: termCard("utilise", "use"), action: "later",
			status: http.StatusOK, outcome: deckActionLater,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			card := deckCardWhere(t, deck, tt.card)
			revision := card.Revision
			if tt.revision != nil {
				revision = tt.revision(card)
			}
			var out struct {
				contextDeckDecideResponse
				Code string `json:"code"`
			}
			assert.Equal(t, tt.status, decide(card, tt.action, revision, &out))
			assert.Equal(t, tt.outcome, out.Outcome)
			assert.Equal(t, tt.code, out.Code)
		})
	}

	after := readContextDeck(t, e, pid, "")
	assert.Equal(t, 2, after.Counts.Total, "the two contested sides still wait")
	assert.Equal(t, map[string]int{"voice_rule": 0, "concept": 0, "term_rule": 2, "note": 0}, after.Counts.ByKind)
	assert.Empty(t, ks.changesets, "keeping proposes no change set")

	// The ledger holds the attestations and the keep, as a person's.
	srv := &Server{ContentStore: cs}
	ledger, err := srv.ContextLedger(t.Context(), pid, LedgerMember{UserID: "member-a"})
	require.NoError(t, err)
	records, err := ledger.Records(t.Context(), contextop.Filter{Kinds: []contextop.Kind{contextop.KindAttest}})
	require.NoError(t, err)
	require.Len(t, records, 3)
	for _, r := range records {
		assert.Equal(t, contextop.ActorPerson, r.Actor.Kind)
		assert.Equal(t, contextop.VerdictKeep, r.Attest.Verdict)
	}
}

func TestContextDeck_RejectConceptProposesChangeSet_SQLite(t *testing.T) {
	caller := &decideCaller{userID: "member-a", perms: decideAll}
	e, cs, ks := deckSQLiteServer(t, caller)
	pid := seedDecideParity(t, e, cs)
	concept := deckCardWhere(t, readContextDeck(t, e, pid, ""), func(c contextDeckCard) bool { return c.Kind == "concept" })

	var out contextDeckDecideResponse
	require.Equal(t, http.StatusOK, decideRoute(t, e, http.MethodPost, pid, "/context/deck/decide",
		contextDeckDecideRequest{Item: concept.Item, Revision: concept.Revision, Action: "reject"}, &out))
	assert.Equal(t, deckOutcomeProposed, out.Outcome)
	require.NotEmpty(t, out.ChangesetID)

	cset := ks.changesets[fakeKey("ws", out.ChangesetID)]
	require.NotNil(t, cset)
	assert.Equal(t, knowledge.ChangeSetInReview, cset.Status)
	assert.Equal(t, "member-a", cset.CreatedBy)
	assert.Equal(t, "context-deck/retire/c-memory", cset.Origin)
	ops := ks.ops[fakeKey("ws", out.ChangesetID)]
	require.Len(t, ops, 2, "the two terms still in use are deprecated; the forbidden one is left")
	for _, op := range ops {
		assert.Equal(t, knowledge.OpTermStatus, op.Op)
	}

	// The concept is still pending: the change set waits for review.
	assert.Equal(t, 1, readContextDeck(t, e, pid, "?kind=concept").Counts.ByKind["concept"])
}

func TestContextDeck_PermissionsAndBinding_SQLite(t *testing.T) {
	caller := &decideCaller{userID: "member-b", perms: decideAll}
	e, cs, _ := deckSQLiteServer(t, caller)
	pid := seedDecideParity(t, e, cs)
	caller.perms = platauth.PermViewContent

	deck := readContextDeck(t, e, pid, "")
	for _, c := range deck.Cards {
		assert.Equal(t, []string{"later"}, c.Actions, "a reader who may not manage context only puts cards off")
	}
	signIn := deckCardWhere(t, deck, termCard("Sign In", "Sign in"))
	var refused ErrorResponse
	assert.Equal(t, http.StatusForbidden, decideRoute(t, e, http.MethodPost, pid, "/context/deck/decide",
		contextDeckDecideRequest{Item: signIn.Item, Revision: signIn.Revision, Action: "keep"}, &refused))

	// A project no push has bound deals its concepts and offers no keep.
	p := &platstore.Project{Name: "unbound", DefaultSourceLanguage: "en", WorkspaceID: "ws", Properties: map[string]string{}}
	require.NoError(t, cs.CreateProject(t.Context(), p))
	caller.perms = decideAll
	unbound := readContextDeck(t, e, p.ID, "")
	assert.False(t, unbound.Bound)
	require.Len(t, unbound.Cards, 1)
	assert.Equal(t, []string{"later"}, unbound.Cards[0].Actions)
	assert.Equal(t, http.StatusConflict, decideRoute(t, e, http.MethodPost, p.ID, "/context/deck/decide",
		contextDeckDecideRequest{Item: unbound.Cards[0].Item, Revision: unbound.Cards[0].Revision, Action: "keep"}, &refused))
}

func TestOpAuthor(t *testing.T) {
	tests := []struct {
		name  string
		rec   contextop.Record
		agent bool
		host  string
	}{
		{"an agent", contextop.Record{Actor: contextop.Actor{Kind: contextop.ActorAgent, Name: "claude", Session: "s1"}}, true, ""},
		{"a tool", contextop.Record{Actor: contextop.Actor{Kind: contextop.ActorTool, Name: "settle"}}, true, ""},
		{"a person", contextop.Record{Actor: contextop.Actor{Kind: contextop.ActorPerson, Name: "asgeir"}}, false, ""},
		{"a person in an agent host's shell", contextop.Record{
			Actor: contextop.Actor{Kind: contextop.ActorPerson, Name: "asgeir"},
			Note:  "recorded as a person in a claude-code shell",
		}, true, "claude-code"},
		{"a person with a note of their own after it", contextop.Record{
			Actor: contextop.Actor{Kind: contextop.ActorPerson},
			Note:  "fixed the casing; recorded as a person in a codex shell",
		}, true, "codex"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := opAuthor(tt.rec)
			assert.Equal(t, tt.agent, a.AgentAuthored)
			assert.Equal(t, tt.host, a.Host)
		})
	}
}

func TestContextDeckCardJudge(t *testing.T) {
	all := platauth.PermManageTerms | platauth.PermManageVoice
	tests := []struct {
		name    string
		author  contextDeckAuthor
		kind    string
		status  string
		rv      deckReviewer
		bound   bool
		basis   contextop.ReviewBasis
		own     bool
		actions []string
	}{
		{"an agent's rule", contextDeckAuthor{Kind: "agent", AgentAuthored: true}, deckKindTermRule, "established",
			deckReviewer{userID: "u1", name: "Ada", perms: all}, true, contextop.BasisAgentAuthored, false, []string{"keep", "reject", "later"}},
		{"another person's rule", contextDeckAuthor{Kind: "person", Name: "Bea"}, deckKindTermRule, "established",
			deckReviewer{userID: "u1", name: "Ada", perms: all}, true, contextop.BasisPeer, false, []string{"keep", "reject", "later"}},
		{"the reader's own rule", contextDeckAuthor{Kind: "person", Name: "Ada"}, deckKindTermRule, "established",
			deckReviewer{userID: "u1", name: "Ada", perms: all}, true, "", true, []string{"reject", "later"}},
		{"the sole owner's own concept", contextDeckAuthor{Kind: "person", userID: "u1"}, deckKindConcept, "",
			deckReviewer{userID: "u1", name: "Ada", perms: all, solo: true}, true, contextop.BasisSoloOwner, true, []string{"keep", "reject", "later"}},
		{"a voice rule cannot be retired here yet", contextDeckAuthor{Kind: "person", userID: "u2"}, deckKindVoiceRule, "",
			deckReviewer{userID: "u1", name: "Ada", perms: all}, true, contextop.BasisPeer, false, []string{"keep", "later"}},
		{"an item no record names an author for", contextDeckAuthor{Kind: "person"}, deckKindVoiceRule, "",
			deckReviewer{userID: "u1", name: "Ada", perms: all}, true, "", false, []string{"later"}},
		{"the sole member keeps an item with no author", contextDeckAuthor{Kind: "person"}, deckKindVoiceRule, "",
			deckReviewer{userID: "u1", name: "Ada", perms: all, solo: true}, true, contextop.BasisSoloOwner, false, []string{"keep", "later"}},
		{"no permission", contextDeckAuthor{Kind: "agent", AgentAuthored: true}, deckKindTermRule, "established",
			deckReviewer{userID: "u1", name: "Ada"}, true, contextop.BasisAgentAuthored, false, []string{"later"}},
		{"an unbound project", contextDeckAuthor{Kind: "agent", AgentAuthored: true}, deckKindTermRule, "established",
			deckReviewer{userID: "u1", name: "Ada", perms: all}, false, contextop.BasisAgentAuthored, false, []string{"later"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			card := contextDeckCard{Kind: tt.kind, Status: tt.status, Author: tt.author}
			card.judge(tt.rv, platauth.PermManageTerms, true, tt.bound)
			assert.Equal(t, tt.basis, card.Basis)
			assert.Equal(t, tt.own, card.OwnWork)
			assert.Equal(t, tt.actions, card.Actions)
		})
	}
}

func TestOriginNoteDropsTheHostMarker(t *testing.T) {
	assert.Equal(t, "edited with `kapi voice edit`", originNote("edited with `kapi voice edit` (recorded as a person in a claude-code shell)"))
	assert.Equal(t, "fixed the casing", originNote("fixed the casing; recorded as a person in a codex shell"))
}

func TestVoiceEditHistoryFindsTheWriteThatAddedARule(t *testing.T) {
	ruleA := coreprofile.Pattern{Regex: `\bleverage\b`, Description: "no leverage"}
	ruleB := coreprofile.Pattern{Regex: `\butilize\b`, Description: "no utilize"}
	profile := &coreprofile.VoiceProfile{ID: "p1", Name: "Docs"}
	profile.Style.ProhibitedPatterns = []coreprofile.Pattern{ruleA, ruleB}
	t0 := time.Date(2026, 11, 10, 6, 0, 0, 0, time.UTC)

	// The first write created the profile with rule A; the second, from an
	// agent host's shell, added rule B.
	first := contextop.Record{ID: "op1", At: t0, Voice: &contextop.VoiceEdit{Profile: "docs"}}
	second := contextop.Record{
		ID: "op2", At: t0.Add(time.Hour),
		Actor: contextop.Actor{Kind: contextop.ActorPerson, Name: "u1"},
		Note:  "recorded as a person in a claude-code shell",
		Voice: &contextop.VoiceEdit{Profile: "docs", Previous: "name: Docs\nstyle:\n  prohibited_patterns:\n    - regex: '\\bleverage\\b'\n      description: no leverage\n"},
	}
	unrelated := contextop.Record{ID: "op3", At: t0, Voice: &contextop.VoiceEdit{Profile: "other"}}
	steps := voiceEditSteps([]contextop.Record{second, unrelated, first}, profile)
	require.Len(t, steps, 2)

	_, revA := patternIdentity("prohibited", ruleA)
	_, revB := patternIdentity("prohibited", ruleB)
	require.NotNil(t, steps.introduced("prohibited", revA))
	assert.Equal(t, "op1", steps.introduced("prohibited", revA).rec.ID)
	require.NotNil(t, steps.introduced("prohibited", revB))
	assert.Equal(t, "op2", steps.introduced("prohibited", revB).rec.ID)
	assert.True(t, opAuthor(steps.introduced("prohibited", revB).rec).AgentAuthored)
	assert.Nil(t, steps.introduced("required", revB))
}

func TestPatternIdentity(t *testing.T) {
	base := coreprofile.Pattern{Regex: `\bleverage\b`, Description: "Say use."}
	id, rev := patternIdentity("prohibited", base)
	edited := base
	edited.Description = "Say use instead."
	id2, rev2 := patternIdentity("prohibited", edited)
	assert.Equal(t, id, id2, "rewording a rule keeps its id")
	assert.NotEqual(t, rev, rev2, "and makes a new revision of it")
	id3, _ := patternIdentity("required", base)
	assert.NotEqual(t, id, id3, "the same expression in the other list is another rule")
}

// The inbox lists a project's pending context for its owner before any
// language, against the real PostgreSQL stores.
func TestReviewInbox_ListsPendingContext(t *testing.T) {
	s, wsID, owner := newRecheckHarness(t)
	ctx := context.Background()
	proj := &platstore.Project{
		Name: "Context Proj", WorkspaceID: wsID, DefaultSourceLanguage: "en",
		TargetLanguages: []model.LocaleID{"fr"}, Properties: map[string]string{},
	}
	require.NoError(t, s.ContentStore.CreateProject(ctx, proj))

	// A voice profile bound to the project, with one prohibited pattern.
	profile := &coreprofile.VoiceProfile{
		Name: "Docs", Scope: wsID, CreatedBy: owner,
		Style: coreprofile.StyleRules{ProhibitedPatterns: []coreprofile.Pattern{{Regex: `\bleverage\b`, Description: "Say use."}}},
	}
	require.NoError(t, s.VoiceStore.CreateProfile(ctx, profile))
	proj.Properties[coreprofile.PropertyProfileID] = profile.ID
	require.NoError(t, s.ContentStore.UpdateProject(ctx, proj))

	cos, ok := platstore.ContextObjects(s.ContentStore)
	require.True(t, ok)
	_, err := cos.BindContextKey(ctx, proj.ID, "prj_decideparityaaaaaaaa")
	require.NoError(t, err)
	segment, err := os.ReadFile(filepath.Join(decideParityDir, "log.jsonl"))
	require.NoError(t, err)
	ops, err := workspace.DecodeSegment(segment, "prj_decideparityaaaaaaaa")
	require.NoError(t, err)
	require.NoError(t, cos.PutContextObjects(ctx, proj.ID, owner, []platstore.ContextObject{
		{Name: workspace.SegmentName("desktop", ops[0].ID), Data: segment},
	}))

	inbox := readInbox(t, s, owner)
	require.Len(t, inbox.Context, 1)
	entry := inbox.Context[0]
	assert.Equal(t, proj.ID, entry.Project)
	assert.Equal(t, "rc", entry.Workspace)
	// The owner may keep the agent's suggestion and its rule in force, the
	// note by member-a, and the voice rule they wrote as the sole owner. The
	// contested pair is settled in Decide, so it is not counted.
	assert.Equal(t, 1, entry.ByKind["voice_rule"])
	assert.Equal(t, 2, entry.ByKind["term_rule"])
	assert.Equal(t, 1, entry.ByKind["note"])
	assert.Equal(t, 4, entry.Waiting)
}
