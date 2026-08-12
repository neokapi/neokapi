package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
