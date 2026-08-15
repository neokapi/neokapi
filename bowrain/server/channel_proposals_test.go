package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	coresync "github.com/neokapi/neokapi/bowrain/core/sync"
)

// seedChannelProject creates a project holding one collection at one channel.
func seedChannelProject(t *testing.T, srv *Server, name, workspaceID, collection, channel string) *platstore.Project {
	t.Helper()
	ctx := t.Context()
	proj := &platstore.Project{
		Name: name, WorkspaceID: workspaceID, DefaultStream: "main",
		DefaultSourceLanguage: "en", Properties: map[string]string{},
	}
	require.NoError(t, srv.ContentStore.CreateProject(ctx, proj))
	require.NoError(t, srv.ContentStore.CreateCollection(ctx, &platstore.Collection{
		ID: proj.ID + "-" + collection, ProjectID: proj.ID, Name: collection,
		Kind: platstore.CollectionUploaded, Stream: "main",
		Context: map[string]string{"product": "acme", "channel": channel},
		Owner:   coresync.ContextOwnerRecipe,
	}))
	return proj
}

// One project spells the channel `website`, another already spells it `web`.
// The workspace records the observation and rewrites neither.
func TestChannelAliasProposalsRaisedOnPush(t *testing.T) {
	srv, token := newTestServer(t)
	ctx := t.Context()

	held := seedChannelProject(t, srv, "app", "test-ws", "site", "web")
	arriving := seedChannelProject(t, srv, "docs", "test-ws", "docs", "website")
	// A project in another workspace is not this workspace's vocabulary.
	seedChannelProject(t, srv, "elsewhere", "other-ws", "site", "webs")

	srv.raiseChannelAliasProposals(ctx, arriving, "main")

	aliases, ok := srv.ContentStore.(platstore.ChannelAliasStore)
	require.True(t, ok)
	proposals, err := aliases.ListChannelAliasProposals(ctx, "test-ws", "")
	require.NoError(t, err)
	require.Len(t, proposals, 1)
	assert.Equal(t, "website", proposals[0].ProposedChannel)
	assert.Equal(t, "web", proposals[0].ExistingChannel)
	assert.Equal(t, "acme", proposals[0].Profile)
	assert.Equal(t, coresync.EvidencePrefix, proposals[0].Evidence)
	assert.Equal(t, arriving.ID, proposals[0].ProjectID)
	assert.Equal(t, "docs", proposals[0].Collection)

	// Neither project's own slugs moved: the workspace proposes, never resolves.
	for _, p := range []*platstore.Project{held, arriving} {
		cols, cerr := srv.ContentStore.ListCollections(ctx, p.ID, "main")
		require.NoError(t, cerr)
		require.Len(t, cols, 1)
		assert.Contains(t, []string{"web", "website"}, cols[0].Context["channel"])
	}

	// And the listing reports it.
	e := srv.GetEcho()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/test/context/channel-proposals", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	var body channelProposalsResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Len(t, body.Proposals, 1)
	assert.Equal(t, "website", body.Proposals[0].ProposedChannel)
}

// Judging settles the row and leaves both recipes alone: a dismissal survives
// the next push's re-sighting, which is the whole point of recording it.
func TestJudgeChannelAliasProposalEndpoint(t *testing.T) {
	srv, token := newTestServer(t)
	ctx := t.Context()
	e := srv.GetEcho()

	seedChannelProject(t, srv, "app", "test-ws", "site", "web")
	arriving := seedChannelProject(t, srv, "docs", "test-ws", "docs", "website")
	srv.raiseChannelAliasProposals(ctx, arriving, "main")

	judge := func(status string) *httptest.ResponseRecorder {
		body := `{"profile":"acme","proposed_channel":"website","existing_channel":"web","status":"` +
			status + `"}`
		req := httptest.NewRequest(http.MethodPost,
			"/api/v1/test/context/channel-proposals/judge", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		return rec
	}

	rec := judge(platstore.ChannelAliasDismissed)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var judged platstore.ChannelAliasProposal
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &judged))
	assert.Equal(t, platstore.ChannelAliasDismissed, judged.Status)
	assert.Equal(t, "test-user", judged.JudgedBy)
	assert.NotEmpty(t, judged.JudgedAt)

	// The same fragmentation is observed again on the next push.
	srv.raiseChannelAliasProposals(ctx, arriving, "main")

	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/test/context/channel-proposals?status=proposed", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	var open channelProposalsResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &open))
	assert.Empty(t, open.Proposals, "a re-sighting must not resurrect a dismissal")

	// Neither project's slug moved: the workspace judges equivalence, never
	// resolution.
	cols, err := srv.ContentStore.ListCollections(ctx, arriving.ID, "main")
	require.NoError(t, err)
	require.Len(t, cols, 1)
	assert.Equal(t, "website", cols[0].Context["channel"])
}

// A verdict outside accepted|dismissed, and a pair nobody proposed, are both
// refused rather than written.
func TestJudgeChannelAliasProposalRefusals(t *testing.T) {
	srv, token := newTestServer(t)
	ctx := t.Context()
	e := srv.GetEcho()

	seedChannelProject(t, srv, "app", "test-ws", "site", "web")
	arriving := seedChannelProject(t, srv, "docs", "test-ws", "docs", "website")
	srv.raiseChannelAliasProposals(ctx, arriving, "main")

	tests := []struct {
		name string
		body string
		want int
	}{
		{
			"a verdict that is not a judgement",
			`{"profile":"acme","proposed_channel":"website","existing_channel":"web","status":"maybe"}`,
			http.StatusBadRequest,
		},
		{
			"a proposal without its key",
			`{"profile":"acme","status":"accepted"}`,
			http.StatusBadRequest,
		},
		{
			"a pair the workspace never observed",
			`{"profile":"acme","proposed_channel":"invented","existing_channel":"web","status":"accepted"}`,
			http.StatusNotFound,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost,
				"/api/v1/test/context/channel-proposals/judge", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer "+token)
			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, req)
			assert.Equal(t, tt.want, rec.Code, rec.Body.String())
		})
	}

	aliases, ok := srv.ContentStore.(platstore.ChannelAliasStore)
	require.True(t, ok)
	proposals, err := aliases.ListChannelAliasProposals(ctx, "test-ws", "")
	require.NoError(t, err)
	require.Len(t, proposals, 1)
	assert.Equal(t, platstore.ChannelAliasProposed, proposals[0].Status)
}

// A project created through the API carries no stream of its own. Reading its
// collections on the empty stream returns none, which reads exactly like a
// workspace holding no channels — so the pair goes unproposed and nothing says
// why. The other side of the comparison resolves the default stream.
func TestChannelAliasProposalsSeeProjectsWithNoDeclaredStream(t *testing.T) {
	srv, _ := newTestServer(t)
	ctx := t.Context()

	held := seedChannelProject(t, srv, "app", "test-ws", "site", "web")
	held.DefaultStream = ""
	require.NoError(t, srv.ContentStore.UpdateProject(ctx, held))

	arriving := seedChannelProject(t, srv, "docs", "test-ws", "docs", "website")
	srv.raiseChannelAliasProposals(ctx, arriving, "main")

	aliases, ok := srv.ContentStore.(platstore.ChannelAliasStore)
	require.True(t, ok)
	proposals, err := aliases.ListChannelAliasProposals(ctx, "test-ws", "")
	require.NoError(t, err)
	require.Len(t, proposals, 1)
	assert.Equal(t, "web", proposals[0].ExistingChannel)
}

// A project pushing vocabulary nobody else shares proposes nothing.
func TestChannelAliasProposalsStaySilentWithoutFragmentation(t *testing.T) {
	srv, _ := newTestServer(t)
	ctx := t.Context()

	seedChannelProject(t, srv, "app", "test-ws", "site", "web")
	arriving := seedChannelProject(t, srv, "news", "test-ws", "letters", "newsletter")

	srv.raiseChannelAliasProposals(ctx, arriving, "main")

	aliases, ok := srv.ContentStore.(platstore.ChannelAliasStore)
	require.True(t, ok)
	proposals, err := aliases.ListChannelAliasProposals(ctx, "test-ws", "")
	require.NoError(t, err)
	assert.Empty(t, proposals)
}
