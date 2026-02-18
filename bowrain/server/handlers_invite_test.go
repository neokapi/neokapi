package server

import (
	"context"
	"sync"
	"testing"

	"github.com/gokapi/gokapi/bowrain/auth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockEmailSender records sent emails for test assertions.
type mockEmailSender struct {
	mu    sync.Mutex
	sent  []sentEmail
}

type sentEmail struct {
	To      string
	Subject string
	Body    string
}

func (m *mockEmailSender) Send(_ context.Context, to, subject, body string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sent = append(m.sent, sentEmail{To: to, Subject: subject, Body: body})
	return nil
}

func (m *mockEmailSender) getSent() []sentEmail {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]sentEmail{}, m.sent...)
}

func TestSendInviteEmail(t *testing.T) {
	mock := &mockEmailSender{}
	s := &Server{EmailSender: mock}

	inv := &auth.Invite{
		Code:  "abc123def456",
		Email: "translator@example.com",
		Role:  auth.RoleMember,
	}

	s.sendInviteEmail(context.Background(), inv, "https://app.bowrain.dev")

	require.Len(t, mock.getSent(), 1)
	email := mock.getSent()[0]
	assert.Equal(t, "translator@example.com", email.To)
	assert.Contains(t, email.Subject, "invited")
	assert.Contains(t, email.Body, "https://app.bowrain.dev/join/abc123def456")
	assert.Contains(t, email.Body, "Accept Invitation")
	assert.Contains(t, email.Body, string(auth.RoleMember))
}
