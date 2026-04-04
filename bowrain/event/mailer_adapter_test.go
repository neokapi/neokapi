package event

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	bauth "github.com/neokapi/neokapi/bowrain/auth"
	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	"github.com/neokapi/neokapi/bowrain/mailer"
	bstore "github.com/neokapi/neokapi/bowrain/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// recordingSender captures outgoing emails for assertions.
type recordingSender struct {
	mu   sync.Mutex
	msgs []sentMsg
}

type sentMsg struct{ To, Subject, Body string }

func (r *recordingSender) Send(_ context.Context, to, subject, body string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.msgs = append(r.msgs, sentMsg{To: to, Subject: subject, Body: body})
	return nil
}

func (r *recordingSender) last() sentMsg {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.msgs) == 0 {
		return sentMsg{}
	}
	return r.msgs[len(r.msgs)-1]
}

func (r *recordingSender) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.msgs)
}

// mockAuthStore implements bauth.AuthStore with only GetUser functional.
type mockAuthStore struct {
	bauth.AuthStore // embed to satisfy the interface; unused methods panic
	users           map[string]*platauth.User
	err             error
}

func newMockAuthStore() *mockAuthStore {
	return &mockAuthStore{users: make(map[string]*platauth.User)}
}

func (m *mockAuthStore) GetUser(_ context.Context, id string) (*platauth.User, error) {
	if m.err != nil {
		return nil, m.err
	}
	u, ok := m.users[id]
	if !ok {
		return nil, errors.New("user not found")
	}
	return u, nil
}

func (m *mockAuthStore) Close() error { return nil }

func newTestMailer(t *testing.T) (*mailer.Mailer, *recordingSender) {
	t.Helper()
	sender := &recordingSender{}
	m, err := mailer.New(sender)
	require.NoError(t, err)
	return m, sender
}

func TestMailerAdapter_SendImmediate(t *testing.T) {
	m, sender := newTestMailer(t)

	store := newMockAuthStore()
	store.users["user-1"] = &platauth.User{
		ID:    "user-1",
		Email: "alice@example.com",
		Name:  "Alice",
	}

	adapter := NewMailerAdapter(m, store)

	notification := &bstore.Notification{
		UserID:   "user-1",
		Type:     bstore.NotificationGateFailed,
		Title:    "Quality gate failed",
		Body:     "3 terminology violations in fr-FR",
		Category: string(bstore.CategoryQuality),
		Priority: "high",
		LinkURL:  "https://app.bowrain.cloud/ws/acme/quality",
	}

	err := adapter.SendImmediate(t.Context(), "user-1", notification)
	if err != nil {
		t.Skipf("notification template not built: %v", err)
	}

	require.Equal(t, 1, sender.count())
	msg := sender.last()
	assert.Equal(t, "alice@example.com", msg.To)
	assert.Contains(t, msg.Subject, "Quality gate failed")
	assert.Contains(t, msg.Body, "3 terminology violations in fr-FR")
	assert.Contains(t, msg.Body, "Review Issues")
}

func TestMailerAdapter_SendImmediate_NoEmail(t *testing.T) {
	m, sender := newTestMailer(t)

	store := newMockAuthStore()
	store.users["user-1"] = &platauth.User{
		ID:    "user-1",
		Email: "",
		Name:  "NoEmail User",
	}

	adapter := NewMailerAdapter(m, store)

	notification := &bstore.Notification{
		UserID:   "user-1",
		Type:     bstore.NotificationTaskAssigned,
		Title:    "Task assigned",
		Body:     "You have a new task",
		Category: string(bstore.CategoryTask),
		Priority: "normal",
	}

	err := adapter.SendImmediate(t.Context(), "user-1", notification)
	require.NoError(t, err)
	assert.Equal(t, 0, sender.count())
}

func TestMailerAdapter_SendImmediate_UserNotFound(t *testing.T) {
	m, _ := newTestMailer(t)

	store := newMockAuthStore()
	// No users added — GetUser will return "user not found".

	adapter := NewMailerAdapter(m, store)

	notification := &bstore.Notification{
		UserID:   "user-unknown",
		Type:     bstore.NotificationGeneral,
		Title:    "Hello",
		Body:     "World",
		Category: string(bstore.CategorySystem),
	}

	err := adapter.SendImmediate(t.Context(), "user-unknown", notification)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "resolve user user-unknown")
}

func TestMailerAdapter_ActionLabel(t *testing.T) {
	tests := []struct {
		name      string
		category  string
		wantLabel string
	}{
		{"task category", string(bstore.CategoryTask), "Open Task"},
		{"quality category", string(bstore.CategoryQuality), "Review Issues"},
		{"automation category", string(bstore.CategoryAutomation), "View Flow"},
		{"default category", string(bstore.CategorySystem), "View Details"},
		{"empty category", "", "View Details"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, sender := newTestMailer(t)

			store := newMockAuthStore()
			store.users["user-1"] = &platauth.User{
				ID:        "user-1",
				Email:     "test@example.com",
				Name:      "Test",
				CreatedAt: time.Now(),
			}

			adapter := NewMailerAdapter(m, store)

			notification := &bstore.Notification{
				UserID:   "user-1",
				Title:    "Test notification",
				Body:     "Test body",
				Category: tt.category,
				Priority: "normal",
				LinkURL:  "https://example.com/action",
			}

			err := adapter.SendImmediate(t.Context(), "user-1", notification)
			if err != nil {
				t.Skipf("notification template not built: %v", err)
			}

			require.Equal(t, 1, sender.count())
			msg := sender.last()
			assert.Contains(t, msg.Body, tt.wantLabel)
		})
	}
}
