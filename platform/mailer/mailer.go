// Package mailer renders email templates and dispatches transactional
// emails for Bowrain.
//
// Templates are pre-rendered from React Email source (bowrain/emails/) at
// build time (make email-build) and embedded into the binary via embed.FS.
// At send time, Go's text/template fills in the dynamic values.
//
// Usage:
//
//	sender := mailer.NewSMTPSender(mailer.SMTPConfig{
//	    Host:     "smtp.example.com",
//	    Port:     587,
//	    Username: "user@example.com",
//	    Password: "secret",
//	    UseTLS:   true,
//	})
//	m, err := mailer.New(sender)
//	if err != nil { ... }
//	err = m.SendInvite(ctx, "translator@example.com", mailer.InviteData{
//	    WorkspaceName: "Acme Inc.",
//	    Role:          "member",
//	    JoinURL:       "https://app.bowrain.cloud/join/abc123",
//	})
package mailer

import (
	"bytes"
	"context"
	"embed"
	"html"
	"strings"
	"text/template"
)

//go:embed templates/*.html
var templateFS embed.FS

// EmailSenderI dispatches a single email message. Implementations include
// SMTPSender (go-mail) and ResendSender (Resend REST API).
type EmailSenderI interface {
	Send(ctx context.Context, to, subject, htmlBody string) error
}

// Mailer renders email templates and sends them via a configured sender.
type Mailer struct {
	Sender    EmailSenderI
	templates *template.Template
}

// New creates a Mailer backed by the given sender. It parses all embedded
// HTML templates; returns an error only if the embedded files are malformed.
func New(sender EmailSenderI) (*Mailer, error) {
	t, err := template.ParseFS(templateFS, "templates/*.html")
	if err != nil {
		return nil, err
	}
	return &Mailer{Sender: sender, templates: t}, nil
}

// InviteData holds the dynamic values for the invite email.
type InviteData struct {
	// WorkspaceName is the human-readable workspace name.
	WorkspaceName string
	// Role is the role assigned to the invitee (e.g. "member", "admin").
	Role string
	// JoinURL is the full accept-invitation URL.
	JoinURL string
}

// SendInvite renders and sends an invitation email to the given address.
func (m *Mailer) SendInvite(ctx context.Context, to string, data InviteData) error {
	body, err := m.renderInvite(data)
	if err != nil {
		return err
	}
	subject := "You've been invited to join " + data.WorkspaceName + " on Bowrain"
	return m.Sender.Send(ctx, to, subject, body)
}

// RenderInvite renders the invite email template to an HTML string.
// Exposed so callers can inspect the output in tests.
func (m *Mailer) RenderInvite(data InviteData) (string, error) {
	return m.renderInvite(data)
}

func (m *Mailer) renderInvite(data InviteData) (string, error) {
	// All dynamic values are HTML-escaped before injection so that the
	// text/template engine outputs them verbatim and safely. This is
	// necessary because text/template (unlike html/template) performs no
	// automatic escaping.
	td := map[string]string{
		"WorkspaceName": html.EscapeString(data.WorkspaceName),
		"Role":          html.EscapeString(data.Role),
		"JoinURL":       escapeURL(data.JoinURL),
	}
	return m.execute("invite.html", td)
}

func (m *Mailer) execute(name string, data any) (string, error) {
	var buf bytes.Buffer
	if err := m.templates.ExecuteTemplate(&buf, name, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// escapeURL encodes a URL for safe use inside an HTML attribute value.
// It HTML-escapes & (→ &amp;) and other HTML-special characters so the
// href="…" attribute parses correctly in all email clients.
func escapeURL(u string) string {
	// Replace bare ampersands that are not already part of an entity.
	u = strings.ReplaceAll(u, "&", "&amp;")
	return u
}
