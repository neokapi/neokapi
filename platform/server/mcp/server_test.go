package mcp

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	corebrand "github.com/neokapi/neokapi/core/brand"
	platauth "github.com/neokapi/neokapi/platform/auth"
)

// bearerTokenTransport is an http.RoundTripper that injects a Bearer token header.
type bearerTokenTransport struct {
	token string
	base  http.RoundTripper
}

func (t *bearerTokenTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	req.Header.Set("Authorization", "Bearer "+t.token)
	return t.base.RoundTrip(req)
}

// memBrandStore is a minimal in-memory BrandStore for testing.
type memBrandStore struct {
	profiles []*corebrand.VoiceProfile
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
func (m *memBrandStore) GetScores(_ context.Context, _, _ string) ([]*corebrand.StoredScore, error) {
	return nil, nil
}
func (m *memBrandStore) GetScoreTrends(_ context.Context, _ string, _ int) ([]*corebrand.ScoreTrend, error) {
	return nil, nil
}
func (m *memBrandStore) StoreCorrection(_ context.Context, _ *corebrand.Correction) error {
	return nil
}
func (m *memBrandStore) GetSuggestedRules(_ context.Context, _ string, _ int) ([]*corebrand.SuggestedRule, error) {
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
	_ = store.CreateProfile(context.Background(), testProfile())

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

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
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

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
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

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
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

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
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

// --- API token (bwt_*) authentication tests ---

// stubAuthStore implements only the methods needed by validateAPIToken.
// All other AuthStore methods panic to catch unintended calls.
type stubAuthStore struct {
	tokens map[string]*platauth.APIToken // keyed by token hash
	users  map[string]*platauth.User     // keyed by user ID
}

func (s *stubAuthStore) GetAPITokenByHash(_ context.Context, tokenHash string) (*platauth.APIToken, error) {
	tok, ok := s.tokens[tokenHash]
	if !ok {
		return nil, errors.New("not found")
	}
	return tok, nil
}
func (s *stubAuthStore) GetUser(_ context.Context, id string) (*platauth.User, error) {
	u, ok := s.users[id]
	if !ok {
		return nil, errors.New("not found")
	}
	return u, nil
}
func (s *stubAuthStore) UpdateAPITokenLastUsed(_ context.Context, _ string) error { return nil }

// Unused AuthStore methods — satisfy the interface.
func (s *stubAuthStore) CreateUser(context.Context, *platauth.User) error                { panic("unused") }
func (s *stubAuthStore) GetUserByEmail(context.Context, string) (*platauth.User, error)  { panic("unused") }
func (s *stubAuthStore) GetUserByOIDCSub(context.Context, string) (*platauth.User, error) {
	panic("unused")
}
func (s *stubAuthStore) UpdateUser(context.Context, *platauth.User) error { panic("unused") }
func (s *stubAuthStore) CreateWorkspace(context.Context, *platauth.Workspace) error {
	panic("unused")
}
func (s *stubAuthStore) GetWorkspace(context.Context, string) (*platauth.Workspace, error) {
	panic("unused")
}
func (s *stubAuthStore) GetWorkspaceBySlug(context.Context, string) (*platauth.Workspace, error) {
	panic("unused")
}
func (s *stubAuthStore) ListWorkspaces(context.Context, string) ([]*platauth.Workspace, error) {
	panic("unused")
}
func (s *stubAuthStore) UpdateWorkspace(context.Context, *platauth.Workspace) error { panic("unused") }
func (s *stubAuthStore) DeleteWorkspace(context.Context, string) error               { panic("unused") }
func (s *stubAuthStore) AddMember(context.Context, string, string, platauth.Role) error {
	panic("unused")
}
func (s *stubAuthStore) RemoveMember(context.Context, string, string) error { panic("unused") }
func (s *stubAuthStore) UpdateRole(context.Context, string, string, platauth.Role) error {
	panic("unused")
}
func (s *stubAuthStore) ListMembers(context.Context, string) ([]*platauth.Membership, error) {
	panic("unused")
}
func (s *stubAuthStore) GetMembership(context.Context, string, string) (*platauth.Membership, error) {
	panic("unused")
}
func (s *stubAuthStore) CreateUnclaimedProject(context.Context, string, string, string, string, string, time.Time) error {
	panic("unused")
}
func (s *stubAuthStore) GetUnclaimedByToken(context.Context, string) (*platauth.UnclaimedProject, error) {
	panic("unused")
}
func (s *stubAuthStore) DeleteUnclaimed(context.Context, string) error { panic("unused") }
func (s *stubAuthStore) PurgeExpiredUnclaimed(context.Context) (int, error) {
	panic("unused")
}
func (s *stubAuthStore) CreateInvite(context.Context, *platauth.Invite) error   { panic("unused") }
func (s *stubAuthStore) GetInviteByCode(context.Context, string) (*platauth.Invite, error) {
	panic("unused")
}
func (s *stubAuthStore) ListInvites(context.Context, string) ([]*platauth.Invite, error) {
	panic("unused")
}
func (s *stubAuthStore) IncrementInviteUseCount(context.Context, string) error { panic("unused") }
func (s *stubAuthStore) DeleteInvite(context.Context, string) error            { panic("unused") }
func (s *stubAuthStore) CreateAPIToken(context.Context, *platauth.APIToken, string) error {
	panic("unused")
}
func (s *stubAuthStore) ListAPITokens(context.Context, string) ([]*platauth.APIToken, error) {
	panic("unused")
}
func (s *stubAuthStore) DeleteAPIToken(context.Context, string) error { panic("unused") }
func (s *stubAuthStore) StoreRefreshToken(context.Context, string, string, time.Time) (string, error) {
	panic("unused")
}
func (s *stubAuthStore) ValidateRefreshTokenByHash(context.Context, string) (string, error) {
	panic("unused")
}
func (s *stubAuthStore) RevokeRefreshToken(context.Context, string) error      { panic("unused") }
func (s *stubAuthStore) RevokeUserRefreshTokens(context.Context, string) error { panic("unused") }
func (s *stubAuthStore) Close() error                                          { return nil }

func hashToken(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}

func TestTokenVerifierAcceptsAPIToken(t *testing.T) {
	rawToken := "bwt_test1234567890abcdef"
	tokenHash := hashToken(rawToken)

	as := &stubAuthStore{
		tokens: map[string]*platauth.APIToken{
			tokenHash: {
				ID:     "tok-1",
				UserID: "user-1",
				Name:   "test-token",
			},
		},
		users: map[string]*platauth.User{
			"user-1": {
				ID:    "user-1",
				Email: "agent@example.com",
				Name:  "Agent Bot",
			},
		},
	}

	verifier := tokenVerifier("some-jwt-secret", as)

	info, err := verifier(context.Background(), rawToken, nil)
	require.NoError(t, err)
	assert.Equal(t, "user-1", info.UserID)
	assert.Equal(t, "agent@example.com", info.Extra["email"])
	assert.Equal(t, "Agent Bot", info.Extra["name"])
	assert.Equal(t, "tok-1", info.Extra["api_token_id"])
}

func TestTokenVerifierRejectsExpiredAPIToken(t *testing.T) {
	rawToken := "bwt_expired_token_000001"
	tokenHash := hashToken(rawToken)
	expired := time.Now().Add(-1 * time.Hour)

	as := &stubAuthStore{
		tokens: map[string]*platauth.APIToken{
			tokenHash: {
				ID:        "tok-2",
				UserID:    "user-1",
				ExpiresAt: &expired,
			},
		},
		users: map[string]*platauth.User{
			"user-1": {ID: "user-1", Email: "a@b.com", Name: "A"},
		},
	}

	verifier := tokenVerifier("some-jwt-secret", as)

	_, err := verifier(context.Background(), rawToken, nil)
	require.Error(t, err)
	assert.True(t, errors.Is(err, auth.ErrInvalidToken))
	assert.Contains(t, err.Error(), "expired")
}

func TestTokenVerifierRejectsUnknownAPIToken(t *testing.T) {
	as := &stubAuthStore{
		tokens: map[string]*platauth.APIToken{},
		users:  map[string]*platauth.User{},
	}

	verifier := tokenVerifier("some-jwt-secret", as)

	_, err := verifier(context.Background(), "bwt_nonexistent_token_0", nil)
	require.Error(t, err)
	assert.True(t, errors.Is(err, auth.ErrInvalidToken))
}

func TestTokenVerifierFallsBackToJWTForNonBWTToken(t *testing.T) {
	as := &stubAuthStore{
		tokens: map[string]*platauth.APIToken{},
		users:  map[string]*platauth.User{},
	}

	verifier := tokenVerifier("some-jwt-secret", as)

	// A random non-bwt_ string should be treated as a JWT and fail JWT validation.
	_, err := verifier(context.Background(), "not-a-valid-jwt-token", nil)
	require.Error(t, err)
	assert.True(t, errors.Is(err, auth.ErrInvalidToken))
}

func TestMCPServerWithAPITokenAuth(t *testing.T) {
	rawToken := "bwt_mcp_integration_test"
	tokenHash := hashToken(rawToken)

	as := &stubAuthStore{
		tokens: map[string]*platauth.APIToken{
			tokenHash: {
				ID:     "tok-mcp",
				UserID: "user-mcp",
				Name:   "mcp-test-token",
			},
		},
		users: map[string]*platauth.User{
			"user-mcp": {
				ID:    "user-mcp",
				Email: "mcp@example.com",
				Name:  "MCP Agent",
			},
		},
	}

	bs := &memBrandStore{}
	_ = bs.CreateProfile(context.Background(), testProfile())

	ms, err := NewMCPServer(bs, Config{
		JWTSecret: "test-secret",
		AuthStore: as,
	})
	require.NoError(t, err)

	ts := httptest.NewServer(ms.Handler())
	t.Cleanup(ts.Close)

	// Connect with a bwt_ token via the MCP client using a custom HTTP client
	// that injects the Authorization header.
	httpClient := ts.Client()
	httpClient.Transport = &bearerTokenTransport{
		token: rawToken,
		base:  httpClient.Transport,
	}
	client := mcp.NewClient(
		&mcp.Implementation{Name: "test-agent", Version: "1.0.0"},
		nil,
	)
	transport := &mcp.StreamableClientTransport{
		Endpoint:   ts.URL,
		HTTPClient: httpClient,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	session, err := client.Connect(ctx, transport, nil)
	require.NoError(t, err, "MCP client should connect with a bwt_ API token")
	defer session.Close()

	// Verify the session works by listing tools.
	result, err := session.ListTools(ctx, nil)
	require.NoError(t, err)
	assert.NotEmpty(t, result.Tools)
}
