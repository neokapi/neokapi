package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	platev "github.com/neokapi/neokapi/bowrain/core/event"
	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/bowrain/service"
	bstore "github.com/neokapi/neokapi/bowrain/store"
	"github.com/neokapi/neokapi/core/contextop"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/workspace"
)

// serve sends one request to the server's routes with a bearer token.
func serve(t *testing.T, srv *Server, method, target, bearer string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var rd *bytes.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		require.NoError(t, err)
		rd = bytes.NewReader(raw)
	} else {
		rd = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, target, rd)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+bearer)
	rec := httptest.NewRecorder()
	srv.GetEcho().ServeHTTP(rec, req)
	return rec
}

// mintTool mints a tool token for test-user in workspace test-ws.
func mintTool(t *testing.T, srv *Server, tool string) (id, plaintext string) {
	t.Helper()
	tok, plaintext, err := srv.Services.Auth.CreateAPIToken(t.Context(), service.APITokenSpec{
		UserID: "test-user", WorkspaceID: "test-ws", Name: "Dogfood Sync", ToolName: tool,
	})
	require.NoError(t, err)
	assert.Equal(t, "test-user", tok.IssuedBy)
	return tok.ID, plaintext
}

// A push under a tool token is recorded as the tool's, issued by the member,
// in the ledger, the feed and the audit event, whatever actor the client
// wrote; the member keeps the tool's work in the context deck, and the tool
// token is refused every review action.
func TestToolToken_PushIsTheToolsAndTheIssuerReviewsIt(t *testing.T) {
	srv, owner, stopRecorder := feedServer(t)
	ctx := t.Context()
	tokenID, toolToken := mintTool(t, srv, "dogfood-sync")

	recorded := make(chan platev.Event, 4)
	srv.EventBus.Subscribe(platev.EventContextRecorded, func(ev platev.Event) { recorded <- ev })

	// The desktop names a person, "asgeir", as the actor of what it observed.
	d := newDesktop(t, "site")
	observed := d.observe(t, "use", "utilise")
	r := &routeRemote{h: srv.GetEcho(), base: "/api/v1/test/p-feed/context/objects", bearer: toolToken}
	require.NoError(t, r.Put(ctx, d.push(t)...))

	t.Run("the stored segment names the tool, its token and the issuer", func(t *testing.T) {
		cos, ok := srv.contextObjects()
		require.True(t, ok)
		names, err := cos.ListContextObjects(ctx, "p-feed", workspace.RemoteLogDir)
		require.NoError(t, err)
		require.NotEmpty(t, names)
		obj, err := cos.GetContextObject(ctx, "p-feed", names[0])
		require.NoError(t, err)
		assert.Equal(t, platstore.Pusher{UserID: "test-user", Tool: "dogfood-sync", TokenID: tokenID}, obj.PushedBy)
	})

	t.Run("the ledger reads the operation as the tool's, not the client's claim", func(t *testing.T) {
		ledger, err := srv.ContextLedger(ctx, "p-feed", testMember("test-user"))
		require.NoError(t, err)
		records, err := ledger.Records(ctx, contextop.Filter{})
		require.NoError(t, err)
		require.Len(t, records, 1)
		assert.Equal(t, observed.ID, records[0].ID)
		assert.Equal(t, contextop.Actor{Kind: contextop.ActorTool, Name: "dogfood-sync", OnBehalfOf: "Test"}, records[0].Actor)
	})

	t.Run("the audit event names the tool and its token", func(t *testing.T) {
		select {
		case ev := <-recorded:
			assert.Equal(t, "test-user", ev.Actor, "the event's actor is the issuer")
			assert.Equal(t, "tool", ev.Data["actor_kind"])
			assert.Equal(t, "dogfood-sync", ev.Data["actor_name"])
			assert.Equal(t, tokenID, ev.Data["token_id"])
		case <-time.After(5 * time.Second):
			t.Fatal("no context.recorded event")
		}
	})

	t.Run("the feed shows the tool on behalf of the issuer", func(t *testing.T) {
		stopRecorder()
		code, feed := getFeed(t, srv, owner, "p-feed", nil)
		require.Equal(t, http.StatusOK, code)
		entries := entriesByKind(feed)[string(bstore.FeedKindContextChange)]
		require.NotEmpty(t, entries)
		e := entries[0]
		assert.Equal(t, "tool", e.Actor.Kind)
		assert.Equal(t, "dogfood-sync", e.Actor.Name)
		require.NotNil(t, e.OnBehalfOf)
		assert.Equal(t, "test-user", e.OnBehalfOf.ID)
		assert.Equal(t, "Test", e.OnBehalfOf.Name)
	})

	var card contextDeckCard
	t.Run("the issuer is offered a keep on the tool's work as agent-authored", func(t *testing.T) {
		rec := serve(t, srv, http.MethodGet, "/api/v1/test/p-feed/context/deck", owner, nil)
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		var deck contextDeckResponse
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &deck))
		require.Len(t, deck.Cards, 1)
		card = deck.Cards[0]
		assert.Equal(t, "tool", card.Author.Kind)
		assert.Equal(t, "dogfood-sync", card.Author.Name)
		assert.Equal(t, "Test", card.Author.OnBehalfOf)
		assert.True(t, card.Author.AgentAuthored)
		assert.False(t, card.OwnWork)
		assert.Equal(t, contextop.BasisAgentAuthored, card.Basis)
		assert.Contains(t, card.Actions, deckActionKeep)
	})

	decide := contextDeckDecideRequest{Item: card.Item, Revision: card.Revision, Action: deckActionKeep}

	refusals := []struct {
		name   string
		method string
		target string
		body   any
	}{
		{"keep a context card", http.MethodPost, "/api/v1/test/p-feed/context/deck/decide", decide},
		{"approve a change set", http.MethodPost, "/api/v1/test/changesets/cs-1/approve", map[string]string{}},
		{"reject a change set", http.MethodPost, "/api/v1/test/changesets/cs-1/reject", map[string]string{}},
		{"decide a review item", http.MethodPost, "/api/v1/test/p-feed/review-queue/main/item-1/decide", map[string]string{"decision": "approve"}},
		{"approve passing drafts", http.MethodPost, "/api/v1/test/p-feed/review/approve-passing", map[string]string{}},
	}
	for _, tt := range refusals {
		t.Run("the tool token may not "+tt.name, func(t *testing.T) {
			rec := serve(t, srv, tt.method, tt.target, toolToken, tt.body)
			require.Equal(t, http.StatusForbidden, rec.Code, rec.Body.String())
			var refusal ErrorResponse
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &refusal))
			assert.Equal(t, toolTokenRefusedCode, refusal.Code)
		})
	}

	t.Run("the tool token mints no token", func(t *testing.T) {
		rec := serve(t, srv, http.MethodPost, "/api/v1/test/tokens", toolToken, map[string]string{"name": "More"})
		assert.NotEqual(t, http.StatusCreated, rec.Code)
	})

	t.Run("the issuer keeps the tool's work with their own sign-in", func(t *testing.T) {
		rec := serve(t, srv, http.MethodPost, "/api/v1/test/p-feed/context/deck/decide", owner, decide)
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		var resp contextDeckDecideResponse
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		assert.Equal(t, deckOutcomeKept, resp.Outcome)
	})
}

// Revoking a tool token stops the tool and leaves the issuer's sign-in alone.
func TestToolToken_RevocationStopsOnlyTheTool(t *testing.T) {
	srv, owner := newTestServer(t)
	tokenID, toolToken := mintTool(t, srv, "dogfood-sync")

	const target = "/api/v1/test/projects"
	require.Equal(t, http.StatusOK, serve(t, srv, http.MethodGet, target, toolToken, nil).Code, "the tool token works before revocation")

	require.NoError(t, srv.AuthStore.DeleteAPIToken(t.Context(), tokenID))

	assert.Equal(t, http.StatusUnauthorized, serve(t, srv, http.MethodGet, target, toolToken, nil).Code, "the revoked token is refused")
	assert.Equal(t, http.StatusOK, serve(t, srv, http.MethodGet, target, owner, nil).Code, "the issuer still signs in")
}

// A tool token is minted with a tool name over the HTTP route and listed with
// the tool and the member who issued it; the route revokes only a token of
// its own workspace.
func TestToolToken_MintListAndRevoke(t *testing.T) {
	srv, jwt, wsSlug := newTokenTestServer(t)

	t.Run("a name that is not a slug is refused", func(t *testing.T) {
		rec := serve(t, srv, http.MethodPost, "/api/v1/"+wsSlug+"/tokens", jwt,
			map[string]string{"name": "Dogfood Sync", "tool_name": "Dogfood Sync"})
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	rec := serve(t, srv, http.MethodPost, "/api/v1/"+wsSlug+"/tokens", jwt,
		map[string]string{"name": "Dogfood Sync", "tool_name": "dogfood-sync"})
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	var created CreateTokenResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &created))
	assert.Equal(t, "dogfood-sync", created.ToolName)
	assert.NotEmpty(t, created.IssuedBy)
	assert.True(t, strings.HasPrefix(created.Token, "bwt_"))

	rec = serve(t, srv, http.MethodGet, "/api/v1/"+wsSlug+"/tokens", jwt, nil)
	require.Equal(t, http.StatusOK, rec.Code)
	var listed []map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &listed))
	require.Len(t, listed, 1)
	assert.Equal(t, "dogfood-sync", listed[0]["tool_name"])
	assert.Equal(t, created.IssuedBy, listed[0]["issued_by"])
	assert.Equal(t, "Token User", listed[0]["issuer_name"])

	assert.Equal(t, http.StatusNotFound,
		serve(t, srv, http.MethodDelete, "/api/v1/"+wsSlug+"/tokens/not-a-token", jwt, nil).Code)
	assert.Equal(t, http.StatusNoContent,
		serve(t, srv, http.MethodDelete, "/api/v1/"+wsSlug+"/tokens/"+created.ID, jwt, nil).Code)
	assert.Equal(t, http.StatusUnauthorized,
		serve(t, srv, http.MethodGet, "/api/v1/"+wsSlug+"/tokens", created.Token, nil).Code,
		"the revoked tool token stops working")
	assert.Equal(t, http.StatusOK,
		serve(t, srv, http.MethodGet, "/api/v1/"+wsSlug+"/tokens", jwt, nil).Code,
		"the issuer's sign-in is untouched")
}

// A tool's push that carries a keep a person recorded is refused, naming the
// operation, and stores nothing. Once the person pushes it with their own
// sign-in, the keep reads as theirs and the tool's push of the same segments
// changes nothing.
func TestToolToken_PushCarryingAPersonsKeepIsRefused(t *testing.T) {
	srv, owner := newTestServer(t)
	ctx := t.Context()
	p := &platstore.Project{
		Name: "ctx", DefaultSourceLanguage: "en", TargetLanguages: []model.LocaleID{"fr"},
		WorkspaceID: "test-ws", Properties: map[string]string{},
	}
	require.NoError(t, srv.ContentStore.CreateProject(ctx, p))
	_, toolToken := mintTool(t, srv, "dogfood-sync")

	d := newDesktop(t, "site")
	observed := d.observe(t, "use", "utilise")
	d.keep(t, observed.ID)
	segments := d.push(t)
	base := "/api/v1/test/" + p.ID + "/context/objects"

	tool := &routeRemote{h: srv.GetEcho(), base: base, bearer: toolToken}
	rec := tool.do(ctx, http.MethodPost, base, pushBody(t, segments))
	require.Equal(t, http.StatusUnprocessableEntity, rec.Code, rec.Body.String())
	var refusal ErrorResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &refusal))
	assert.Equal(t, toolPushVerdictCode, refusal.Code)
	assert.NotContains(t, refusal.Details, workspace.ShortOpID(observed.ID), "the observation is not a verdict")
	assert.NotEmpty(t, refusal.Details, "the refusal names the keep")

	cos, ok := srv.contextObjects()
	require.True(t, ok)
	names, err := cos.ListContextObjects(ctx, p.ID, workspace.RemoteLogDir)
	require.NoError(t, err)
	assert.Empty(t, names, "a refused push stores no segment")

	person := &routeRemote{h: srv.GetEcho(), base: base, bearer: owner}
	require.NoError(t, person.Put(ctx, segments...))
	require.NoError(t, tool.Put(ctx, segments...), "segments the project holds change nothing")

	ledger, err := srv.ContextLedger(ctx, p.ID, testMember("test-user"))
	require.NoError(t, err)
	records, err := ledger.Records(ctx, contextop.Filter{})
	require.NoError(t, err)
	for _, r := range records {
		assert.Equal(t, contextop.ActorPerson, r.Actor.Kind, "operation %s reads as the person's", r.ID)
	}
}
