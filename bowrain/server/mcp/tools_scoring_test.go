package mcp

import (
	"context"
	"testing"

	"github.com/neokapi/neokapi/bowrain/core/store"
	corebrand "github.com/neokapi/neokapi/core/brand"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// scopeContentStore is a store.ContentStore that only answers the reads the
// brand-scope resolver makes (project/stream/collection), plus ListProjects for
// resolveProjectID. Everything else panics via the embedded nil interface,
// which is never reached by the scoring path.
type scopeContentStore struct {
	store.ContentStore
	project    *store.Project
	stream     *store.Stream
	collection *store.Collection
}

func (f *scopeContentStore) GetProject(_ context.Context, _ string) (*store.Project, error) {
	return f.project, nil
}
func (f *scopeContentStore) GetStream(_ context.Context, _, _ string) (*store.Stream, error) {
	return f.stream, nil
}
func (f *scopeContentStore) GetCollection(_ context.Context, _, _ string) (*store.Collection, error) {
	return f.collection, nil
}
func (f *scopeContentStore) ListProjects(_ context.Context) ([]*store.Project, error) {
	return []*store.Project{f.project}, nil
}

// wsDefaultFunc adapts a func to brandscope.WorkspaceDefault.
type wsDefaultFunc func(ctx context.Context, workspaceID string) (string, error)

func (f wsDefaultFunc) WorkspaceBrandProfileID(ctx context.Context, workspaceID string) (string, error) {
	return f(ctx, workspaceID)
}

func scoringTestServer(cs store.ContentStore, wsDefaultID string) *MCPServer {
	return &MCPServer{
		brandStore: &memBrandStore{profiles: []*corebrand.VoiceProfile{
			{ID: "hex1", Name: "Explicit", WorkspaceID: "ws1"},
			{ID: "hex2", Name: "ProjectBound", WorkspaceID: "ws1"},
			{ID: "hex3", Name: "WorkspaceDefault", WorkspaceID: "ws1"},
		}},
		contentStore: cs,
		wsDefault: wsDefaultFunc(func(_ context.Context, _ string) (string, error) {
			return wsDefaultID, nil
		}),
	}
}

func TestScoreBrandCompliance_ExplicitProfileWins(t *testing.T) {
	cs := &scopeContentStore{project: &store.Project{
		WorkspaceID: "ws1",
		Properties:  map[string]string{corebrand.PropertyProfileID: "hex2"},
	}}
	ms := scoringTestServer(cs, "hex3")

	_, out, err := ms.handleScoreBrandCompliance(t.Context(), nil, scoreBrandComplianceInput{
		ProfileID:       "hex1",
		Text:            "hello world",
		brandScopeInput: brandScopeInput{ProjectID: "hex-proj"},
	})
	require.NoError(t, err)
	assert.Equal(t, "hex1", out.Score.ProfileID, "explicit profile_id wins over the project binding")
}

func TestScoreBrandCompliance_ProjectBindingBeatsWorkspaceDefault(t *testing.T) {
	cs := &scopeContentStore{project: &store.Project{
		WorkspaceID: "ws1",
		Properties:  map[string]string{corebrand.PropertyProfileID: "hex2"},
	}}
	ms := scoringTestServer(cs, "hex3")

	_, out, err := ms.handleScoreBrandCompliance(t.Context(), nil, scoreBrandComplianceInput{
		Text:            "hello world",
		brandScopeInput: brandScopeInput{ProjectID: "hex-proj"},
	})
	require.NoError(t, err)
	assert.Equal(t, "hex2", out.Score.ProfileID, "the project binding wins when no explicit profile is given")
}

func TestScoreBrandCompliance_FallsThroughToWorkspaceDefault(t *testing.T) {
	cs := &scopeContentStore{project: &store.Project{WorkspaceID: "ws1"}} // no binding
	ms := scoringTestServer(cs, "hex3")

	_, out, err := ms.handleScoreBrandCompliance(t.Context(), nil, scoreBrandComplianceInput{
		Text:            "hello world",
		brandScopeInput: brandScopeInput{ProjectID: "hex-proj"},
	})
	require.NoError(t, err)
	assert.Equal(t, "hex3", out.Score.ProfileID, "with no scope binding, the workspace default is used")
}

func TestScoreBrandCompliance_NoProfileAnywhere(t *testing.T) {
	cs := &scopeContentStore{project: &store.Project{WorkspaceID: "ws1"}}
	ms := scoringTestServer(cs, "") // no workspace default either

	_, _, err := ms.handleScoreBrandCompliance(t.Context(), nil, scoreBrandComplianceInput{
		Text:            "hello world",
		brandScopeInput: brandScopeInput{ProjectID: "hex-proj"},
	})
	require.Error(t, err, "no profile bound at any level is an error, matching the prior empty-profile behavior")
	assert.Contains(t, err.Error(), "no brand voice profile")
}

func TestScoreBrandCompliance_StreamBindingBeatsProject(t *testing.T) {
	cs := &scopeContentStore{
		project: &store.Project{
			WorkspaceID: "ws1",
			Properties:  map[string]string{corebrand.PropertyProfileID: "hex2"},
		},
		stream: &store.Stream{Properties: map[string]string{corebrand.PropertyProfileID: "hex3"}},
	}
	ms := scoringTestServer(cs, "")

	_, out, err := ms.handleScoreBrandCompliance(t.Context(), nil, scoreBrandComplianceInput{
		Text:            "hello world",
		brandScopeInput: brandScopeInput{ProjectID: "hex-proj", Stream: "v2"},
	})
	require.NoError(t, err)
	assert.Equal(t, "hex3", out.Score.ProfileID, "a stream binding wins over the project binding")
}
