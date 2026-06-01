package mcp

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	corebrand "github.com/neokapi/neokapi/core/brand"
	"github.com/neokapi/neokapi/core/model"
)

// memBrandStore is a minimal in-memory BrandStore for testing.
type memBrandStore struct {
	profiles  []*corebrand.VoiceProfile
	decisions map[string]*corebrand.RuleDecision
	suggested []*corebrand.SuggestedRule
}

func (m *memBrandStore) CreateProfile(_ context.Context, p *corebrand.VoiceProfile) error {
	m.profiles = append(m.profiles, p)
	return nil
}
func (m *memBrandStore) GetProfile(_ context.Context, id string) (*corebrand.VoiceProfile, error) {
	for _, p := range m.profiles {
		if p.ID == id {
			return p, nil
		}
	}
	return nil, assert.AnError
}
func (m *memBrandStore) UpdateProfile(_ context.Context, p *corebrand.VoiceProfile) error {
	return nil
}
func (m *memBrandStore) DeleteProfile(_ context.Context, id string) error { return nil }
func (m *memBrandStore) ListProfiles(_ context.Context, wsID string) ([]*corebrand.VoiceProfile, error) {
	var result []*corebrand.VoiceProfile
	for _, p := range m.profiles {
		if p.WorkspaceID == wsID {
			result = append(result, p)
		}
	}
	return result, nil
}
func (m *memBrandStore) StoreScore(_ context.Context, _ *corebrand.StoredScore) error { return nil }
func (m *memBrandStore) GetScores(_ context.Context, _ string, _ model.LocaleID) ([]*corebrand.StoredScore, error) {
	return nil, nil
}
func (m *memBrandStore) GetScoreTrends(_ context.Context, _ string, _ int) ([]*corebrand.ScoreTrend, error) {
	return nil, nil
}
func (m *memBrandStore) StoreCorrection(_ context.Context, _ *corebrand.Correction) error {
	return nil
}
func (m *memBrandStore) GetSuggestedRules(_ context.Context, _ string, _ int) ([]*corebrand.SuggestedRule, error) {
	return m.suggested, nil
}
func (m *memBrandStore) RecordRuleDecision(_ context.Context, d *corebrand.RuleDecision) error {
	if m.decisions == nil {
		m.decisions = map[string]*corebrand.RuleDecision{}
	}
	m.decisions[d.ProfileID+"|"+strings.ToLower(d.Term)] = d
	return nil
}
func (m *memBrandStore) GetRuleDecision(_ context.Context, profileID, term string) (*corebrand.RuleDecision, error) {
	return m.decisions[profileID+"|"+strings.ToLower(term)], nil
}
func (m *memBrandStore) ListRuleDecisions(_ context.Context, profileID string) ([]*corebrand.RuleDecision, error) {
	var out []*corebrand.RuleDecision
	for _, d := range m.decisions {
		if d.ProfileID == profileID {
			out = append(out, d)
		}
	}
	return out, nil
}
func (m *memBrandStore) ListProfileVersions(_ context.Context, _ string) ([]*corebrand.ProfileVersion, error) {
	return nil, nil
}
func (m *memBrandStore) GetProfileVersion(_ context.Context, _ string, _ int) (*corebrand.ProfileVersion, error) {
	return nil, nil
}
func (m *memBrandStore) GetProfileAtTag(_ context.Context, _, _ string) (*corebrand.VoiceProfile, error) {
	return nil, nil
}
func (m *memBrandStore) CreateProfileTag(_ context.Context, _ *corebrand.ProfileTag) error {
	return nil
}
func (m *memBrandStore) ListProfileTags(_ context.Context, _ string) ([]*corebrand.ProfileTag, error) {
	return nil, nil
}
func (m *memBrandStore) DeleteProfileTag(_ context.Context, _, _ string) error { return nil }
func (m *memBrandStore) GetScoresByStream(_ context.Context, _, _ string) ([]*corebrand.StoredScore, error) {
	return nil, nil
}
func (m *memBrandStore) Close() error { return nil }

func testProfile() *corebrand.VoiceProfile {
	return &corebrand.VoiceProfile{
		ID:          "test-profile-1",
		Name:        "Test Brand",
		Description: "A test brand voice profile",
		WorkspaceID: "ws-1",
		Tone: corebrand.ToneProfile{
			Personality: []string{"friendly", "professional"},
			Formality:   "neutral",
			Emotion:     "warm",
			Humor:       "light",
		},
		Style: corebrand.StyleRules{
			ActiveVoice:    true,
			SentenceLength: "medium",
			PersonPOV:      "second",
			Contractions:   "sometimes",
		},
		Vocabulary: corebrand.VocabularyRules{
			ForbiddenTerms: []corebrand.TermRule{
				{Term: "synergy", Replacement: "collaboration", Severity: "minor"},
				{Term: "leverage", Replacement: "use", Severity: "minor"},
			},
			CompetitorTerms: []corebrand.TermRule{
				{Term: "Acrolinx", Severity: "critical"},
			},
			PreferredTerms: []corebrand.TermRule{
				{Term: "platform", Note: "Use instead of 'tool'"},
			},
		},
		Examples: []corebrand.VoiceExample{
			{
				Before:      "We leverage synergies to drive outcomes.",
				After:       "We use collaboration to achieve results.",
				Explanation: "Replace corporate jargon with clear language",
				Category:    "vocabulary",
			},
		},
		Version: 1,
	}
}

func setupTestMCPServer(t *testing.T) (*httptest.Server, *memBrandStore) {
	t.Helper()

	store := &memBrandStore{}
	_ = store.CreateProfile(t.Context(), testProfile())

	ms, err := NewMCPServer(store, Config{}) // no auth for testing
	require.NoError(t, err)

	ts := httptest.NewServer(ms.Handler())
	t.Cleanup(ts.Close)
	return ts, store
}

func TestMCPServerInitialize(t *testing.T) {
	ts, _ := setupTestMCPServer(t)

	client := mcp.NewClient(
		&mcp.Implementation{Name: "test-client", Version: "1.0.0"},
		nil,
	)
	transport := &mcp.StreamableClientTransport{
		Endpoint:   ts.URL,
		HTTPClient: ts.Client(),
	}

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	session, err := client.Connect(ctx, transport, nil)
	require.NoError(t, err)
	defer session.Close()
}

func TestMCPServerListTools(t *testing.T) {
	ts, _ := setupTestMCPServer(t)

	client := mcp.NewClient(
		&mcp.Implementation{Name: "test-client", Version: "1.0.0"},
		nil,
	)
	transport := &mcp.StreamableClientTransport{
		Endpoint:   ts.URL,
		HTTPClient: ts.Client(),
	}

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	session, err := client.Connect(ctx, transport, nil)
	require.NoError(t, err)
	defer session.Close()

	result, err := session.ListTools(ctx, nil)
	require.NoError(t, err)
	require.NotNil(t, result)

	var toolNames []string
	for _, tool := range result.Tools {
		toolNames = append(toolNames, tool.Name)
	}
	assert.Contains(t, toolNames, "check_vocabulary")
	assert.Contains(t, toolNames, "list_profiles")
	assert.Contains(t, toolNames, "get_voice_guide")
	assert.Contains(t, toolNames, "score_brand_compliance")
	assert.Contains(t, toolNames, "suggest_corrections")
	assert.Contains(t, toolNames, "rewrite_in_voice")
}

func TestMCPServerListPrompts(t *testing.T) {
	ts, _ := setupTestMCPServer(t)

	client := mcp.NewClient(
		&mcp.Implementation{Name: "test-client", Version: "1.0.0"},
		nil,
	)
	transport := &mcp.StreamableClientTransport{
		Endpoint:   ts.URL,
		HTTPClient: ts.Client(),
	}

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	session, err := client.Connect(ctx, transport, nil)
	require.NoError(t, err)
	defer session.Close()

	result, err := session.ListPrompts(ctx, nil)
	require.NoError(t, err)
	require.NotNil(t, result)

	var promptNames []string
	for _, prompt := range result.Prompts {
		promptNames = append(promptNames, prompt.Name)
	}
	assert.Contains(t, promptNames, "write_in_voice")
	assert.Contains(t, promptNames, "rewrite_in_voice")
	assert.Contains(t, promptNames, "check_draft")
}

func TestMCPServerListResources(t *testing.T) {
	ts, _ := setupTestMCPServer(t)

	client := mcp.NewClient(
		&mcp.Implementation{Name: "test-client", Version: "1.0.0"},
		nil,
	)
	transport := &mcp.StreamableClientTransport{
		Endpoint:   ts.URL,
		HTTPClient: ts.Client(),
	}

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	session, err := client.Connect(ctx, transport, nil)
	require.NoError(t, err)
	defer session.Close()

	result, err := session.ListResourceTemplates(ctx, nil)
	require.NoError(t, err)
	require.NotNil(t, result)

	var templateNames []string
	for _, tmpl := range result.ResourceTemplates {
		templateNames = append(templateNames, tmpl.Name)
	}
	assert.Contains(t, templateNames, "brand_profile")
	assert.Contains(t, templateNames, "brand_vocabulary")
	assert.Contains(t, templateNames, "brand_examples")
	assert.Contains(t, templateNames, "brand_terminology")
}
