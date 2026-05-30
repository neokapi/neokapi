package server

import (
	"net/http"
	"sync"
	"testing"
	"time"

	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	platev "github.com/neokapi/neokapi/bowrain/core/event"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// collectEvents subscribes to the server's event bus and returns a snapshot
// function plus an unsubscribe. Events are gathered concurrently (the in-memory
// bus dispatches on a goroutine).
func collectEvents(t *testing.T, s *Server) (snapshot func() []platev.Event, stop func()) {
	t.Helper()
	var mu sync.Mutex
	var events []platev.Event
	sub := s.EventBus.SubscribeAll(func(ev platev.Event) {
		mu.Lock()
		events = append(events, ev)
		mu.Unlock()
	})
	snapshot = func() []platev.Event {
		mu.Lock()
		defer mu.Unlock()
		out := make([]platev.Event, len(events))
		copy(out, events)
		return out
	}
	stop = func() { s.EventBus.Unsubscribe(sub) }
	return snapshot, stop
}

func findEvent(events []platev.Event, t platev.EventType) (platev.Event, bool) {
	for _, ev := range events {
		if ev.Type == t {
			return ev, true
		}
	}
	return platev.Event{}, false
}

// TestPhase1_GovernanceEventEmitted verifies that a governance mutation
// (creating a role template) emits an attributed audit event on the bus.
func TestPhase1_GovernanceEventEmitted(t *testing.T) {
	s, ownerToken := newTestServer(t)
	snapshot, stop := collectEvents(t, s)
	defer stop()

	body := `{"name":"qa-role","display_name":"QA","permissions":["view_content","review"]}`
	code := do(t, s, http.MethodPost, "/api/v1/test/roles", ownerToken, body)
	require.Equal(t, http.StatusCreated, code)

	require.Eventually(t, func() bool {
		_, ok := findEvent(snapshot(), platev.EventRoleTemplateCreated)
		return ok
	}, 2*time.Second, 10*time.Millisecond, "role.template.created should be emitted")

	ev, _ := findEvent(snapshot(), platev.EventRoleTemplateCreated)
	assert.Equal(t, "test-user", ev.Actor)
	assert.Equal(t, "role_template", ev.ResourceType)
	assert.NotEmpty(t, ev.ResourceID)
	assert.Equal(t, "qa-role", ev.Data["name"])
}

// TestPhase1_AuthzDenialEmitted verifies that an authorization denial for an
// authenticated caller is recorded as an authz.denied event.
func TestPhase1_AuthzDenialEmitted(t *testing.T) {
	s, _ := newTestServer(t)
	memberToken := addWorkspaceMember(t, s, "member-evt", "member-evt@example.com", platauth.RoleMember)
	snapshot, stop := collectEvents(t, s)
	defer stop()

	// Member is denied a TM mutation (requires PermManageTM).
	code := do(t, s, http.MethodPost, "/api/v1/test/translation-memory", memberToken,
		`{"source":"x","target":"y","source_locale":"en","target_locale":"fr"}`)
	require.Equal(t, http.StatusForbidden, code)

	require.Eventually(t, func() bool {
		ev, ok := findEvent(snapshot(), platev.EventAuthzDenied)
		return ok && ev.Effect == "deny"
	}, 2*time.Second, 10*time.Millisecond, "authz.denied should be emitted for the denied member")

	ev, _ := findEvent(snapshot(), platev.EventAuthzDenied)
	assert.Equal(t, "member-evt", ev.Actor)
	assert.Equal(t, "deny", ev.Effect)
	assert.Contains(t, ev.Data["required_permission"], "manage_tm")
}
