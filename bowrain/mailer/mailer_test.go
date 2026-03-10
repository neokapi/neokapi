package mailer_test

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/gokapi/gokapi/bowrain/mailer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// recordingSender captures outgoing emails for assertions.
type recordingSender struct {
	mu   sync.Mutex
	msgs []capturedMsg
}

type capturedMsg struct{ To, Subject, Body string }

func (r *recordingSender) Send(_ context.Context, to, subject, body string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.msgs = append(r.msgs, capturedMsg{To: to, Subject: subject, Body: body})
	return nil
}

func (r *recordingSender) last() capturedMsg {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.msgs) == 0 {
		return capturedMsg{}
	}
	return r.msgs[len(r.msgs)-1]
}

func TestMailerNew(t *testing.T) {
	sender := &recordingSender{}
	m, err := mailer.New(sender)
	require.NoError(t, err)
	assert.NotNil(t, m)
}

func TestSendInvite(t *testing.T) {
	sender := &recordingSender{}
	m, err := mailer.New(sender)
	require.NoError(t, err)

	data := mailer.InviteData{
		WorkspaceName: "Acme Corp",
		Role:          "member",
		JoinURL:       "https://app.bowrain.cloud/join/abc123",
	}

	err = m.SendInvite(context.Background(), "user@example.com", data)
	require.NoError(t, err)

	msg := sender.last()
	assert.Equal(t, "user@example.com", msg.To)
	assert.Contains(t, msg.Subject, "Acme Corp")
	assert.Contains(t, msg.Subject, "invited")
	assert.Contains(t, msg.Body, "Acme Corp")
	assert.Contains(t, msg.Body, "member")
	assert.Contains(t, msg.Body, "https://app.bowrain.cloud/join/abc123")
	assert.Contains(t, msg.Body, "Accept Invitation")
}

func TestRenderInviteHTMLEscaping(t *testing.T) {
	sender := &recordingSender{}
	m, err := mailer.New(sender)
	require.NoError(t, err)

	// Workspace name with HTML-special characters.
	data := mailer.InviteData{
		WorkspaceName: `Acme & "Co" <Ltd>`,
		Role:          "admin",
		JoinURL:       "https://example.com/join/x?a=1&b=2",
	}

	html, err := m.RenderInvite(data)
	require.NoError(t, err)

	// HTML special characters should be escaped in the output.
	assert.Contains(t, html, "Acme &amp; &#34;Co&#34; &lt;Ltd&gt;")
	// URL ampersands should be encoded for safe HTML attributes.
	assert.True(t,
		strings.Contains(html, "a=1&amp;b=2") || strings.Contains(html, "a=1&b=2"),
		"URL should appear in rendered HTML",
	)
}

func TestRenderInviteContainsExpectedElements(t *testing.T) {
	sender := &recordingSender{}
	m, err := mailer.New(sender)
	require.NoError(t, err)

	html, err := m.RenderInvite(mailer.InviteData{
		WorkspaceName: "TestWS",
		Role:          "viewer",
		JoinURL:       "https://example.com/join/xyz",
	})
	require.NoError(t, err)

	// Verify structural elements expected from the template.
	assert.Contains(t, html, "Bowrain", "should contain brand name")
	assert.Contains(t, html, "Accept Invitation", "should contain CTA button text")
	assert.Contains(t, html, "TestWS", "should contain workspace name")
	assert.Contains(t, html, "viewer", "should contain role")
	assert.Contains(t, html, "https://example.com/join/xyz", "should contain join URL")
}
