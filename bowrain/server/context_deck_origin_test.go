package server

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/core/contextop"
	"github.com/neokapi/neokapi/core/model"
	coreprofile "github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/terms"
)

func TestLatestOrigin(t *testing.T) {
	t0 := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	terms1 := &contextDeckOrigin{ID: "op-terms-1", Kind: originKindImport, Source: contextop.ImportTerms, At: t0}
	memory := &contextDeckOrigin{ID: "op-memory", Kind: originKindImport, Source: contextop.ImportMemory, At: t0.Add(time.Hour)}
	terms2 := &contextDeckOrigin{ID: "op-terms-2", Kind: originKindImport, Source: contextop.ImportTerms, At: t0.Add(2 * time.Hour)}
	voiceEdit := &contextDeckOrigin{ID: "op-voice", Kind: originKindVoiceEdit, Profile: "neokapi-documentation", At: t0.Add(3 * time.Hour)}
	voiceImport := &contextDeckOrigin{ID: "op-voice-import", Kind: originKindImport, Source: contextop.ImportVoice, Path: ".kapi/profiles/neokapi-documentation/voice.yaml", At: t0}
	origins := []*contextDeckOrigin{terms1, voiceImport, memory, terms2, voiceEdit}

	conceptTests := []struct {
		name string
		at   time.Time
		want *contextDeckOrigin
	}{
		{"the latest terms import before the concept reached the store", t0.Add(90 * time.Minute), terms1},
		{"a concept stamped after both imports", t0.Add(5 * time.Hour), terms2},
		{"a concept older than every import takes the latest", t0.Add(-time.Hour), terms2},
		{"an unknown time takes the latest", time.Time{}, terms2},
	}
	for _, tt := range conceptTests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Same(t, tt.want, conceptOrigin(origins, tt.at))
		})
	}

	rule := func(id, name string, at time.Time) contextDeckCard {
		return contextDeckCard{
			Item: contextop.ItemRef{Kind: contextop.ItemVoiceRule, ID: "r", Profile: id}, At: at,
			Pattern: &contextDeckPattern{Profile: name},
		}
	}
	voiceTests := []struct {
		name string
		card contextDeckCard
		want *contextDeckOrigin
	}{
		{"a checkout names the profile by its slug", rule("TWMEjPVx", "neokapi documentation", t0.Add(4*time.Hour)), voiceEdit},
		{"the import before the edit", rule("TWMEjPVx", "neokapi documentation", t0.Add(time.Hour)), voiceImport},
		{"another profile has no origin", rule("k79J3Td2", "bowrain platform", t0.Add(4*time.Hour)), nil},
		{"a card that is no voice rule", contextDeckCard{}, nil},
	}
	for _, tt := range voiceTests {
		t.Run(tt.name, func(t *testing.T) {
			got := voiceOrigin(origins, tt.card)
			if tt.want == nil {
				assert.Nil(t, got)
				return
			}
			assert.Same(t, tt.want, got)
		})
	}
}

func TestVoiceWriteProfile(t *testing.T) {
	tests := []struct {
		name string
		rec  contextop.Record
		want string
	}{
		{"from the edit", contextop.Record{Kind: contextop.KindEdit, Voice: &contextop.VoiceEdit{Profile: "docs"}}, "docs"},
		{"from the note", contextop.Record{Kind: contextop.KindEdit, Subject: contextop.Subject{Kind: contextop.SubjectNote, Text: "voice profile neokapi-documentation edited whole"}}, "neokapi-documentation"},
		{"from a note with changes", contextop.Record{Kind: contextop.KindEdit, Subject: contextop.Subject{Kind: contextop.SubjectNote, Text: "voice profile docs: tone.formality"}}, "docs"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, voiceWriteProfile(tt.rec))
		})
	}
}

// The deck leaves bulk operations out and deals what they brought in, one
// card per item, each naming the operation. A recipe that binds a voice per
// collection deals the rules of every such voice. Names stored as member ids
// read as the member's name.
func TestContextDeck_BulkOperationsDealTheirItems(t *testing.T) {
	s, wsID, owner := newRecheckHarness(t)
	ctx := context.Background()
	proj := &platstore.Project{
		Name: "Dogfood", WorkspaceID: wsID, DefaultSourceLanguage: "en",
		TargetLanguages: []model.LocaleID{"nb"}, Properties: map[string]string{},
	}
	require.NoError(t, s.ContentStore.CreateProject(ctx, proj))

	// Two voices, each bound to a collection and neither to the project.
	docs := &coreprofile.VoiceProfile{
		Name: "neokapi documentation", Scope: wsID, CreatedBy: owner,
		Style: coreprofile.StyleRules{ProhibitedPatterns: []coreprofile.Pattern{
			{Regex: `\bleverage\b`, Description: "Say use."},
			{Regex: `\bseamless(ly)?\b`, Description: "Say what it does."},
		}},
	}
	platform := &coreprofile.VoiceProfile{
		Name: "bowrain platform", Scope: wsID, CreatedBy: owner,
		Style: coreprofile.StyleRules{ProhibitedPatterns: []coreprofile.Pattern{{Regex: `\bsolution\b`}}},
	}
	require.NoError(t, s.VoiceStore.CreateProfile(ctx, docs))
	require.NoError(t, s.VoiceStore.CreateProfile(ctx, platform))
	for name, profile := range map[string]*coreprofile.VoiceProfile{"docs": docs, "app": platform} {
		require.NoError(t, s.ContentStore.CreateCollection(ctx, &platstore.Collection{
			ProjectID: proj.ID, Name: name, Kind: platstore.CollectionUploaded, ItemLabel: "file",
			Stream: "main", Owner: "recipe",
			ConnectorConfig: map[string]string{coreprofile.PropertyProfileID: profile.ID},
		}))
	}

	tb, err := s.wsStores.getTerms(ctx, "ws") // the slug the routes are read under
	require.NoError(t, err)
	require.NoError(t, tb.AddConcept(ctx, terms.Concept{
		ID: "c-kapi", Definition: "The command line.", DoNotTranslate: true,
		Terms: []terms.Term{{Text: "kapi", Locale: "en", Status: model.TermPreferred}},
	}))

	cos, ok := platstore.ContextObjects(s.ContentStore)
	require.True(t, ok)
	_, err = cos.BindContextKey(ctx, proj.ID, "prj_bulkoriginsaaaaaaaaa")
	require.NoError(t, err)
	ledger, err := s.ContextLedger(ctx, proj.ID, LedgerMember{UserID: owner, Permissions: decideAll})
	require.NoError(t, err)
	note := func(text string) contextop.Subject { return contextop.Subject{Kind: contextop.SubjectNote, Text: text} }
	termsImport, err := ledger.Append(ctx, contextop.Record{
		Project: "prj_bulkoriginsaaaaaaaaa", Kind: contextop.KindImport,
		Subject: note("terms bundle .kapi/terms.json, 61 concepts"), Evidence: []contextop.Evidence{{Path: ".kapi/terms.json"}},
		Note: "recorded as a person in a claude-code shell",
	})
	require.NoError(t, err)
	_, err = ledger.Append(ctx, contextop.Record{
		Project: "prj_bulkoriginsaaaaaaaaa", Kind: contextop.KindImport,
		Subject: note("decision record .kapi/state, 13205 decisions"),
		Import:  &contextop.ImportSource{Source: contextop.ImportDecisions, Path: ".kapi/state", Count: 13205},
	})
	require.NoError(t, err)
	voiceEdit, err := ledger.Append(ctx, contextop.Record{
		Project: "prj_bulkoriginsaaaaaaaaa", Kind: contextop.KindEdit,
		Subject: note("voice profile neokapi-documentation edited whole"),
		Voice:   &contextop.VoiceEdit{Profile: "neokapi-documentation", Changes: []contextop.VoiceChange{{Field: "style.prohibited_patterns"}}},
	})
	require.NoError(t, err)
	_, err = ledger.Append(ctx, contextop.Record{
		Project: "prj_bulkoriginsaaaaaaaaa", Kind: contextop.KindObserve, Subject: note("Use sentence case in headings."),
	})
	require.NoError(t, err)

	caller := &decideCaller{userID: owner, perms: decideAll}
	e := echo.New()
	g := e.Group("/api/v1/:ws", func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			c.Set("user_id", caller.userID)
			c.Set("workspace_id", wsID)
			c.Set("project_permissions", caller.perms)
			return next(c)
		}
	})
	s.registerContextDeckRoutes(g)

	deck := readContextDeck(t, e, proj.ID, "")
	assert.Equal(t, map[string]int{"voice_rule": 3, "concept": 1, "term_rule": 0, "note": 1}, deck.Counts.ByKind,
		"no import or voice write is a card; every collection's voice deals its rules")
	for _, c := range deck.Cards {
		assert.NotContains(t, c.Text, "bundle")
		assert.NotContains(t, c.Text, "edited whole")
	}

	concept := deckCardWhere(t, deck, func(c contextDeckCard) bool { return c.Kind == deckKindConcept })
	require.NotNil(t, concept.Origin)
	assert.Equal(t, termsImport.ID, concept.Origin.ID)
	assert.Equal(t, originKindImport, concept.Origin.Kind)
	assert.Equal(t, contextop.ImportTerms, concept.Origin.Source)
	assert.Equal(t, ".kapi/terms.json", concept.Origin.Path)
	assert.Equal(t, 61, concept.Origin.Count)
	assert.Equal(t, "Owner", concept.Origin.Author.Name)
	assert.Equal(t, "claude-code", concept.Origin.Author.Host)
	assert.Empty(t, concept.Origin.Note, "the host is not repeated as a note")

	leverage := deckCardWhere(t, deck, func(c contextDeckCard) bool {
		return c.Pattern != nil && c.Pattern.Regex == `\bleverage\b`
	})
	require.NotNil(t, leverage.Origin)
	assert.Equal(t, voiceEdit.ID, leverage.Origin.ID)
	assert.Equal(t, originKindVoiceEdit, leverage.Origin.Kind)
	solution := deckCardWhere(t, deck, func(c contextDeckCard) bool {
		return c.Pattern != nil && c.Pattern.Regex == `\bsolution\b`
	})
	assert.Nil(t, solution.Origin, "the edit wrote another profile")

	var origin contextDeckOriginResponse
	require.Equal(t, http.StatusOK, decideRoute(t, e, http.MethodGet, proj.ID, "/context/deck/origins/"+voiceEdit.ID+"?limit=1", nil, &origin))
	require.NotNil(t, origin.Origin)
	assert.Equal(t, 2, origin.Total, "both of the edited profile's rules")
	assert.Equal(t, 2, origin.Pending)
	assert.Len(t, origin.Cards, 1)
	assert.Equal(t, "1", origin.NextCursor)
	assert.Nil(t, origin.Cards[0].Origin, "the listing names the origin once")

	var decisions contextDeckOriginResponse
	ids := map[string]string{}
	for _, r := range bulkRecords(deckSource{records: mustRecords(t, s, proj.ID, owner), key: "prj_bulkoriginsaaaaaaaaa"}) {
		ids[r.Subject.Text] = r.ID
	}
	require.Equal(t, http.StatusOK, decideRoute(t, e, http.MethodGet, proj.ID, "/context/deck/origins/"+ids["decision record .kapi/state, 13205 decisions"], nil, &decisions))
	assert.Equal(t, 13205, decisions.Origin.Count)
	assert.Zero(t, decisions.Total, "the server deals no decision as a context card")

	var missing ErrorResponse
	assert.Equal(t, http.StatusNotFound, decideRoute(t, e, http.MethodGet, proj.ID, "/context/deck/origins/nope", nil, &missing))
}

func mustRecords(t *testing.T, s *Server, projectID, userID string) []contextop.Record {
	t.Helper()
	ledger, err := s.ContextLedger(t.Context(), projectID, LedgerMember{UserID: userID})
	require.NoError(t, err)
	records, err := ledger.Records(t.Context(), contextop.Filter{})
	require.NoError(t, err)
	return records
}

// An operation a person recorded under a member's user id reads as the
// member's name, and a name that is no member's id stays as it is.
func TestRecordAuthorResolvesMemberIDs(t *testing.T) {
	s, _, owner := newRecheckHarness(t)
	names := map[string]string{}
	person := func(name string) contextop.Record {
		return contextop.Record{Actor: contextop.Actor{Kind: contextop.ActorPerson, Name: name}}
	}
	a := s.recordAuthor(t.Context(), person(owner), names)
	assert.Equal(t, "Owner", a.Name)
	assert.Equal(t, owner, a.userID)

	b := s.recordAuthor(t.Context(), person("asgeir"), names)
	assert.Equal(t, "asgeir", b.Name)
	assert.Empty(t, b.userID)

	agent := s.recordAuthor(t.Context(), contextop.Record{Actor: contextop.Actor{Kind: contextop.ActorAgent, Name: owner}}, names)
	assert.Equal(t, owner, agent.Name, "an agent's name is never looked up as a member")

	unknown := s.memberAuthor(t.Context(), "y2tWvDNp", names)
	assert.Empty(t, unknown.Name, "an id that names no member shows no name")
}
