package mcp

import (
	"context"
	"testing"

	"github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/core/model"
	coreprofile "github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/terms"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// scopeContentStore is a store.ContentStore that only answers the reads the
// voice-scope resolver makes (project/stream/collection), plus ListProjects for
// the project-name lookup. Everything else panics via the embedded nil
// interface, which is never reached by the scoring path.
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

// wordRulesServer returns a server whose single profile "hexP" holds no words
// and whose workspace terms store forbids "utilize" (use "use"), bans
// "leverage" with no replacement, and names "Globex" as a competitor.
// contentStore is nil so an explicit profile_id short-circuits scope
// resolution.
func wordRulesServer(t *testing.T) *MCPServer {
	t.Helper()
	tb := newTestTermsStore(t)
	for _, c := range []terms.Concept{
		{ID: "use", Terms: []terms.Term{
			{Text: "use", Locale: "en", Status: model.TermPreferred},
			{Text: "utilize", Locale: "en", Status: model.TermForbidden},
		}},
		{ID: "leverage", Terms: []terms.Term{{Text: "leverage", Locale: "en", Status: model.TermForbidden}}},
		{ID: "globex", Terms: []terms.Term{{Text: "Globex", Locale: "en", Status: model.TermForbidden, CompetitorTerm: true}}},
	} {
		require.NoError(t, tb.AddConcept(t.Context(), c))
	}
	return &MCPServer{
		voiceStore: &memVoiceStore{profiles: []*coreprofile.VoiceProfile{{
			ID:    "hexP",
			Name:  "WordRules",
			Scope: "ws1",
			Personas: map[string]coreprofile.PersonaOverride{
				"jordan": {Tone: &coreprofile.ToneProfile{Formality: "casual"}},
			},
		}}},
		tbResolver: singleTermsResolver{tb: tb},
	}
}

func TestScoreVoiceCompliance_FlagsTheWorkspaceTerms(t *testing.T) {
	ms := wordRulesServer(t)

	for _, in := range []scoreVoiceComplianceInput{
		{ProfileID: "hexP", Text: "utilize synergy today", Locale: "en-US"},
		{ProfileID: "hexP", Text: "utilize synergy today"},
		{ProfileID: "hexP", Text: "utilize synergy today", Persona: "jordan"},
	} {
		_, out, err := ms.handleScoreVoiceCompliance(t.Context(), nil, in)
		require.NoError(t, err)
		assert.True(t, findingForTerm(out.Score.Findings, "utilize"), "the workspace's forbidden term is flagged (%+v)", in)
		assert.False(t, findingForTerm(out.Score.Findings, "synergy"), "a word no rule names is not")
	}
}

func TestScoreVoiceCompliance_NoTermsStoreChecksTheVoiceAlone(t *testing.T) {
	ms := wordRulesServer(t)
	ms.tbResolver = nil

	_, out, err := ms.handleScoreVoiceCompliance(t.Context(), nil, scoreVoiceComplianceInput{
		ProfileID: "hexP",
		Text:      "utilize synergy today",
	})
	require.NoError(t, err)
	assert.Empty(t, out.Score.Findings)
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

// TestRewriteInVoice_ReportsSkipped proves the rewrite substitutes what the
// workspace terms store names a replacement for and lists what it matched and
// left in place, so an agent can tell an unchanged text with nothing to fix
// from one that still carries violations.
func TestRewriteInVoice_ReportsSkipped(t *testing.T) {
	ms := wordRulesServer(t)

	_, out, err := ms.handleRewriteInVoice(t.Context(), nil, rewriteInVoiceInput{
		ProfileID: "hexP",
		Text:      "Leverage Globex to utilize your content.",
		Locale:    "en",
	})
	require.NoError(t, err)

	assert.Equal(t, "Leverage Globex to use your content.", out.Rewritten)
	assert.Equal(t, []string{`Replaced forbidden term "utilize" with "use"`}, out.Changes)
	require.Len(t, out.Skipped, 2)
	skipped := map[string]coreprofile.RewriteSkip{}
	for _, sk := range out.Skipped {
		skipped[sk.Term] = sk
	}
	assert.Equal(t, "forbidden", skipped["leverage"].List)
	assert.True(t, skipped["leverage"].Fails)
	assert.Equal(t, coreprofile.RewriteSkipNoReplacement, skipped["leverage"].Reason)
	assert.Equal(t, "competitor", skipped["Globex"].List)
	assert.True(t, skipped["Globex"].Fails)
	assert.NotEmpty(t, out.Guide)
}

// TestRewriteInVoice_CleanText: a text with no hits reports nothing skipped.
func TestRewriteInVoice_CleanText(t *testing.T) {
	ms := wordRulesServer(t)

	_, out, err := ms.handleRewriteInVoice(t.Context(), nil, rewriteInVoiceInput{
		ProfileID: "hexP",
		Text:      "Use the workspace.",
	})
	require.NoError(t, err)
	assert.Equal(t, "Use the workspace.", out.Rewritten)
	assert.Empty(t, out.Changes)
	assert.Empty(t, out.Skipped)
}
