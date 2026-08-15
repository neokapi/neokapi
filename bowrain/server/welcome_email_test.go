package server

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/bowrain/auth"
	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	"github.com/neokapi/neokapi/bowrain/mailer"
	"github.com/neokapi/neokapi/bowrain/service"
	"github.com/neokapi/neokapi/core/id"
)

// These tests pin the greeting: an account gets one welcome for coming into
// existence, and never a second one. The failure they exist to catch is not a
// missing mail but a repeated one — a returning member greeted as new is the
// kind of defect nobody reports and everybody notices.

// welcomeHarness wires the onboarding path with an in-memory auth store and a
// recording mail sender: no Postgres, no network, and the real AuthService in
// between so the once-per-account write is the one under test.
type welcomeHarness struct {
	srv   *Server
	store *onboardingAuthStore
	sends *recordingSender
}

func newWelcomeHarness(t *testing.T) *welcomeHarness {
	t.Helper()

	store := &onboardingAuthStore{
		users: map[string]*platauth.User{
			"u-new": {ID: "u-new", Email: "dana@example.test", Name: "Dana", Locale: "en"},
		},
	}
	srv := shutdownOnCleanup(t, NewServer(DefaultConfig()))
	srv.AuthStore = store
	srv.Services = &service.Services{Auth: service.NewAuthService(store, "test-secret")}

	sends := &recordingSender{}
	m, err := mailer.New(sends)
	require.NoError(t, err)
	srv.Mailer = m

	return &welcomeHarness{srv: srv, store: store, sends: sends}
}

// onboard posts the onboarding form as the given user.
func (h *welcomeHarness) onboard(t *testing.T, userID, slug string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/me/onboarding",
		strings.NewReader(`{"slug":"`+slug+`","display_name":"Dana"}`))
	req.Header.Set("Content-Type", echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := h.srv.GetEcho().NewContext(req, rec)
	c.Set("user_id", userID)
	require.NoError(t, h.srv.HandleCompleteOnboarding(c))
	return rec
}

// TestOnboardingGreetsANewAccountOnce covers the seam itself: the call that
// provisions the first workspace mails the welcome, and a repeat of the same
// call — a double-submitted form, a retried request — does not.
func TestOnboardingGreetsANewAccountOnce(t *testing.T) {
	h := newWelcomeHarness(t)

	rec := h.onboard(t, "u-new", "dana")
	require.Equal(t, http.StatusOK, rec.Code)

	require.Eventually(t, func() bool { return len(h.sends.messages()) == 1 }, 2*time.Second, 10*time.Millisecond,
		"the account that just got its first workspace is greeted")
	msg := h.sends.messages()[0]
	assert.Equal(t, "dana@example.test", msg.to)
	assert.Contains(t, msg.subject, "Welcome to Bowrain")
	assert.Contains(t, msg.body, "context graph", "the greeting says what Bowrain is")
	assert.Contains(t, msg.body, "/dana", "the greeting links the workspace it is about")

	// Onboarding again is idempotent server-side; the greeting must be too.
	rec = h.onboard(t, "u-new", "dana")
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Never(t, func() bool { return len(h.sends.messages()) > 1 }, 300*time.Millisecond, 10*time.Millisecond,
		"a second onboarding call greets nobody: the account already exists")
}

// TestOnboardingDoesNotGreetAnExistingAccount covers the account that predates
// the marker — onboarded_at nil, personal workspace already there. Onboarding
// stamps the marker for it, and stamping alone must not trigger a greeting.
func TestOnboardingDoesNotGreetAnExistingAccount(t *testing.T) {
	h := newWelcomeHarness(t)
	h.store.users["u-old"] = &platauth.User{ID: "u-old", Email: "old@example.test", Name: "Otto"}
	h.store.addWorkspace(&platauth.Workspace{
		ID: "ws-old", Name: "Otto", Slug: "otto", Type: platauth.WorkspaceTypePersonal,
	}, "u-old")

	rec := h.onboard(t, "u-old", "otto")
	require.Equal(t, http.StatusOK, rec.Code)

	assert.Never(t, func() bool { return len(h.sends.messages()) > 0 }, 300*time.Millisecond, 10*time.Millisecond,
		"an account that already had a workspace is not new, whatever the marker said")
	require.NotNil(t, h.store.users["u-old"].OnboardedAt, "the marker is still stamped for it")
}

// TestConcurrentOnboardingGreetsOnce is why the marker is a conditional write
// rather than a read followed by an update: two calls racing on the same
// account must produce one greeting, not two.
func TestConcurrentOnboardingGreetsOnce(t *testing.T) {
	h := newWelcomeHarness(t)

	// Resolved once: GetEcho lazily builds the router, and racing on that would
	// be the test racing with itself rather than the onboarding path racing.
	e := h.srv.GetEcho()

	start := make(chan struct{})
	var wg sync.WaitGroup
	// Distinct slugs, so the store's uniqueness constraint cannot be what keeps
	// the second caller out. Only the marker can.
	for i := range 4 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/me/onboarding",
				strings.NewReader(`{"slug":"dana-`+string(rune('a'+i))+`"}`))
			req.Header.Set("Content-Type", echo.MIMEApplicationJSON)
			c := e.NewContext(req, httptest.NewRecorder())
			c.Set("user_id", "u-new")
			<-start
			_ = h.srv.HandleCompleteOnboarding(c)
		}(i)
	}
	close(start)
	wg.Wait()

	assert.Never(t, func() bool { return len(h.sends.messages()) > 1 }, 500*time.Millisecond, 10*time.Millisecond,
		"the conditional onboarded_at write admits one winner, so one account gets one greeting")
}

// TestWelcomeEmailWithoutAMailerIsSilent pins the best-effort contract: a
// deployment with no mail configured still onboards.
func TestWelcomeEmailWithoutAMailerIsSilent(t *testing.T) {
	h := newWelcomeHarness(t)
	h.srv.Mailer = nil

	rec := h.onboard(t, "u-new", "dana")
	assert.Equal(t, http.StatusOK, rec.Code, "onboarding does not depend on the mail going out")
}

// ---------------------------------------------------------------------------
// Fakes
// ---------------------------------------------------------------------------

// onboardingAuthStore is the slice of auth.AuthStore the onboarding path reads
// and writes. Everything else is nil and would panic if reached, which is the
// point — it pins what the seam touches.
type onboardingAuthStore struct {
	auth.AuthStore
	mu         sync.Mutex
	users      map[string]*platauth.User
	workspaces []*platauth.Workspace
	// owners maps workspace ID → member IDs.
	owners map[string][]string
}

var errNoSuchRow = errors.New("not found")

func (s *onboardingAuthStore) addWorkspace(w *platauth.Workspace, ownerID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.workspaces = append(s.workspaces, w)
	if s.owners == nil {
		s.owners = map[string][]string{}
	}
	s.owners[w.ID] = append(s.owners[w.ID], ownerID)
}

func (s *onboardingAuthStore) GetUser(_ context.Context, userID string) (*platauth.User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	u, ok := s.users[userID]
	if !ok {
		return nil, errNoSuchRow
	}
	clone := *u
	return &clone, nil
}

func (s *onboardingAuthStore) UpdateUser(_ context.Context, u *platauth.User) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.users[u.ID]; !ok {
		return errNoSuchRow
	}
	clone := *u
	s.users[u.ID] = &clone
	return nil
}

// MarkUserOnboarded mirrors the conditional UPDATE: the first caller to find
// the column NULL wins it, and every later one is told it did not.
func (s *onboardingAuthStore) MarkUserOnboarded(_ context.Context, userID string, at time.Time) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	u, ok := s.users[userID]
	if !ok {
		return false, errNoSuchRow
	}
	if u.OnboardedAt != nil {
		return false, nil
	}
	stamped := at
	u.OnboardedAt = &stamped
	return true, nil
}

// Reads hand out copies, as a scan from a real row would. Sharing the stored
// pointer would let two callers mutate one workspace and turn a property of the
// fake into a finding about the code.
func (s *onboardingAuthStore) ListWorkspaces(_ context.Context, userID string) ([]*platauth.Workspace, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []*platauth.Workspace
	for _, w := range s.workspaces {
		if slices.Contains(s.owners[w.ID], userID) {
			clone := *w
			out = append(out, &clone)
		}
	}
	return out, nil
}

func (s *onboardingAuthStore) GetWorkspaceBySlug(_ context.Context, slug string) (*platauth.Workspace, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, w := range s.workspaces {
		if w.Slug == slug {
			clone := *w
			return &clone, nil
		}
	}
	return nil, errNoSuchRow
}

func (s *onboardingAuthStore) IsSlugReserved(context.Context, string) (string, time.Time, bool, error) {
	return "", time.Time{}, false, nil
}

func (s *onboardingAuthStore) CreateWorkspace(_ context.Context, w *platauth.Workspace) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if w.ID == "" {
		w.ID = id.New()
	}
	for _, existing := range s.workspaces {
		if existing.Slug == w.Slug {
			return errors.New("slug already taken")
		}
	}
	// Stored by value, as a row would be: the caller keeps mutating its own
	// struct after this returns.
	clone := *w
	s.workspaces = append(s.workspaces, &clone)
	return nil
}

func (s *onboardingAuthStore) AddMember(_ context.Context, workspaceID, userID string, _ platauth.Role) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.owners == nil {
		s.owners = map[string][]string{}
	}
	s.owners[workspaceID] = append(s.owners[workspaceID], userID)
	return nil
}

func (s *onboardingAuthStore) SeedDefaultRoleTemplates(context.Context, string) error { return nil }

// Close satisfies the store lifecycle the server's shutdown walks.
func (s *onboardingAuthStore) Close() error { return nil }
