package mcp

import (
	"context"
	"testing"

	"github.com/neokapi/neokapi/bowrain/core/store"
	coreprofile "github.com/neokapi/neokapi/core/profile"
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

// wsDefaultFunc adapts a func to voicescope.WorkspaceDefault.
type wsDefaultFunc func(ctx context.Context, workspaceID string) (string, error)

func (f wsDefaultFunc) WorkspaceVoiceProfileID(ctx context.Context, workspaceID string) (string, error) {
	return f(ctx, workspaceID)
}

func scoringTestServer(cs store.ContentStore, wsDefaultID string) *MCPServer {
	return &MCPServer{
		voiceStore: &memVoiceStore{profiles: []*coreprofile.VoiceProfile{
			{ID: "hex1", Name: "Explicit", Scope: "ws1"},
			{ID: "hex2", Name: "ProjectBound", Scope: "ws1"},
			{ID: "hex3", Name: "WorkspaceDefault", Scope: "ws1"},
		}},
		contentStore: cs,
		wsDefault: wsDefaultFunc(func(_ context.Context, _ string) (string, error) {
			return wsDefaultID, nil
		}),
	}
}

func TestScoreVoiceCompliance_ExplicitProfileWins(t *testing.T) {
	cs := &scopeContentStore{project: &store.Project{
		WorkspaceID: "ws1",
		Properties:  map[string]string{coreprofile.PropertyProfileID: "hex2"},
	}}
	ms := scoringTestServer(cs, "hex3")

	_, out, err := ms.handleScoreVoiceCompliance(t.Context(), nil, scoreVoiceComplianceInput{
		ProfileID: "hex1",
		Text:      "hello world",
		ProjectID: "hex-proj",
	})
	require.NoError(t, err)
	assert.Equal(t, "hex1", out.Score.ProfileID, "explicit profile_id wins over the project binding")
}

func TestScoreVoiceCompliance_ProjectBindingBeatsWorkspaceDefault(t *testing.T) {
	cs := &scopeContentStore{project: &store.Project{
		WorkspaceID: "ws1",
		Properties:  map[string]string{coreprofile.PropertyProfileID: "hex2"},
	}}
	ms := scoringTestServer(cs, "hex3")

	_, out, err := ms.handleScoreVoiceCompliance(t.Context(), nil, scoreVoiceComplianceInput{
		Text:      "hello world",
		ProjectID: "hex-proj",
	})
	require.NoError(t, err)
	assert.Equal(t, "hex2", out.Score.ProfileID, "the project binding wins when no explicit profile is given")
}

func TestScoreVoiceCompliance_FallsThroughToWorkspaceDefault(t *testing.T) {
	cs := &scopeContentStore{project: &store.Project{WorkspaceID: "ws1"}} // no binding
	ms := scoringTestServer(cs, "hex3")

	_, out, err := ms.handleScoreVoiceCompliance(t.Context(), nil, scoreVoiceComplianceInput{
		Text:      "hello world",
		ProjectID: "hex-proj",
	})
	require.NoError(t, err)
	assert.Equal(t, "hex3", out.Score.ProfileID, "with no scope binding, the workspace default is used")
}

func TestScoreVoiceCompliance_NoProfileAnywhere(t *testing.T) {
	cs := &scopeContentStore{project: &store.Project{WorkspaceID: "ws1"}}
	ms := scoringTestServer(cs, "") // no workspace default either

	_, _, err := ms.handleScoreVoiceCompliance(t.Context(), nil, scoreVoiceComplianceInput{
		Text:      "hello world",
		ProjectID: "hex-proj",
	})
	require.Error(t, err, "no profile bound at any level is an error, matching the prior empty-profile behavior")
	assert.Contains(t, err.Error(), "no voice profile")
}

// findingForTerm reports whether any finding was raised for the given term.
func findingForTerm(findings []coreprofile.VoiceFinding, term string) bool {
	for _, f := range findings {
		if f.OriginalText == term {
			return true
		}
	}
	return false
}

// personaScoringServer returns a server whose single profile "hexP" forbids
// "utilize" (brand guardrail) and defines a "jordan" persona that additionally
// avoids "synergy". contentStore is nil so an explicit profile_id short-circuits
// scope resolution.
func personaScoringServer() *MCPServer {
	return &MCPServer{
		voiceStore: &memVoiceStore{profiles: []*coreprofile.VoiceProfile{{
			ID:    "hexP",
			Name:  "WithPersona",
			Scope: "ws1",
			Vocabulary: coreprofile.VocabularyRules{
				ForbiddenTerms: []coreprofile.TermRule{{Term: "utilize", Replacement: "use"}},
			},
			Personas: map[string]coreprofile.PersonaOverride{
				"jordan": {Avoided: []coreprofile.TermRule{{Term: "synergy"}}},
			},
		}}},
	}
}

func TestScoreVoiceCompliance_PersonaRespected(t *testing.T) {
	ms := personaScoringServer()

	// With the persona, its avoided term is flagged on top of the brand's own.
	_, out, err := ms.handleScoreVoiceCompliance(t.Context(), nil, scoreVoiceComplianceInput{
		ProfileID: "hexP",
		Text:      "utilize synergy today",
		Persona:   "jordan",
	})
	require.NoError(t, err)
	assert.True(t, findingForTerm(out.Score.Findings, "utilize"), "brand forbidden term is always flagged")
	assert.True(t, findingForTerm(out.Score.Findings, "synergy"), "persona avoided term is flagged when the persona is applied")
}

func TestScoreVoiceCompliance_NoPersonaDoesNotApplyPersonaVocab(t *testing.T) {
	ms := personaScoringServer()

	_, out, err := ms.handleScoreVoiceCompliance(t.Context(), nil, scoreVoiceComplianceInput{
		ProfileID: "hexP",
		Text:      "utilize synergy today",
	})
	require.NoError(t, err)
	assert.True(t, findingForTerm(out.Score.Findings, "utilize"), "brand forbidden term is flagged")
	assert.False(t, findingForTerm(out.Score.Findings, "synergy"), "persona avoided term is not flagged without the persona")
}

func TestScoreVoiceCompliance_UnknownPersonaFallsBackToBaseProfile(t *testing.T) {
	ms := personaScoringServer()

	// An unknown persona is not an error: it leaves the base profile in force.
	_, out, err := ms.handleScoreVoiceCompliance(t.Context(), nil, scoreVoiceComplianceInput{
		ProfileID: "hexP",
		Text:      "utilize synergy today",
		Persona:   "nobody",
	})
	require.NoError(t, err)
	assert.True(t, findingForTerm(out.Score.Findings, "utilize"), "brand forbidden term is still flagged")
	assert.False(t, findingForTerm(out.Score.Findings, "synergy"), "an unknown persona adds nothing")
}

func TestScoreVoiceCompliance_StreamBindingBeatsProject(t *testing.T) {
	cs := &scopeContentStore{
		project: &store.Project{
			WorkspaceID: "ws1",
			Properties:  map[string]string{coreprofile.PropertyProfileID: "hex2"},
		},
		stream: &store.Stream{Properties: map[string]string{coreprofile.PropertyProfileID: "hex3"}},
	}
	ms := scoringTestServer(cs, "")

	_, out, err := ms.handleScoreVoiceCompliance(t.Context(), nil, scoreVoiceComplianceInput{
		Text:      "hello world",
		ProjectID: "hex-proj", Stream: "v2",
	})
	require.NoError(t, err)
	assert.Equal(t, "hex3", out.Score.ProfileID, "a stream binding wins over the project binding")
}
