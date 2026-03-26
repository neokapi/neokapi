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

// CreditsWarningData holds the dynamic values for the credits-warning email.
type CreditsWarningData struct {
	// WorkspaceName is the human-readable workspace name.
	WorkspaceName string
	// UsedCredits is the number of credits consumed (formatted string).
	UsedCredits string
	// TotalCredits is the total credit allowance (formatted string).
	TotalCredits string
	// UsagePercent is the usage percentage (e.g. "80").
	UsagePercent string
	// ResetDate is the human-readable date when credits reset.
	ResetDate string
	// UpgradeURL is the full URL to the plan upgrade page.
	UpgradeURL string
}

// SendCreditsWarning renders and sends a credits-warning email to the given address.
func (m *Mailer) SendCreditsWarning(ctx context.Context, to string, data CreditsWarningData) error {
	body, err := m.renderCreditsWarning(data)
	if err != nil {
		return err
	}
	subject := "Your AI credits are running low — " + data.WorkspaceName
	return m.Sender.Send(ctx, to, subject, body)
}

// RenderCreditsWarning renders the credits-warning email template to an HTML string.
// Exposed so callers can inspect the output in tests.
func (m *Mailer) RenderCreditsWarning(data CreditsWarningData) (string, error) {
	return m.renderCreditsWarning(data)
}

func (m *Mailer) renderCreditsWarning(data CreditsWarningData) (string, error) {
	td := map[string]string{
		"WorkspaceName": html.EscapeString(data.WorkspaceName),
		"UsedCredits":   html.EscapeString(data.UsedCredits),
		"TotalCredits":  html.EscapeString(data.TotalCredits),
		"UsagePercent":  html.EscapeString(data.UsagePercent),
		"ResetDate":     html.EscapeString(data.ResetDate),
		"UpgradeURL":    escapeURL(data.UpgradeURL),
	}
	return m.execute("credits-warning.html", td)
}

// CreditsExhaustedData holds the dynamic values for the credits-exhausted email.
type CreditsExhaustedData struct {
	// WorkspaceName is the human-readable workspace name.
	WorkspaceName string
	// ResetDate is the human-readable date when credits reset.
	ResetDate string
	// UpgradeURL is the full URL to the plan upgrade page.
	UpgradeURL string
	// BuyCreditsURL is the full URL to purchase additional credits.
	BuyCreditsURL string
}

// SendCreditsExhausted renders and sends a credits-exhausted email to the given address.
func (m *Mailer) SendCreditsExhausted(ctx context.Context, to string, data CreditsExhaustedData) error {
	body, err := m.renderCreditsExhausted(data)
	if err != nil {
		return err
	}
	subject := "Your AI credits are exhausted — " + data.WorkspaceName
	return m.Sender.Send(ctx, to, subject, body)
}

// RenderCreditsExhausted renders the credits-exhausted email template to an HTML string.
// Exposed so callers can inspect the output in tests.
func (m *Mailer) RenderCreditsExhausted(data CreditsExhaustedData) (string, error) {
	return m.renderCreditsExhausted(data)
}

func (m *Mailer) renderCreditsExhausted(data CreditsExhaustedData) (string, error) {
	td := map[string]string{
		"WorkspaceName": html.EscapeString(data.WorkspaceName),
		"ResetDate":     html.EscapeString(data.ResetDate),
		"UpgradeURL":    escapeURL(data.UpgradeURL),
		"BuyCreditsURL": escapeURL(data.BuyCreditsURL),
	}
	return m.execute("credits-exhausted.html", td)
}

// PaymentFailedData holds the dynamic values for the payment-failed email.
type PaymentFailedData struct {
	// WorkspaceName is the human-readable workspace name.
	WorkspaceName string
	// InvoiceAmount is the formatted invoice amount (e.g. "49.00").
	InvoiceAmount string
	// Currency is the three-letter currency code (e.g. "USD").
	Currency string
	// UpdatePaymentURL is the full URL to update the payment method.
	UpdatePaymentURL string
}

// SendPaymentFailed renders and sends a payment-failed email to the given address.
func (m *Mailer) SendPaymentFailed(ctx context.Context, to string, data PaymentFailedData) error {
	body, err := m.renderPaymentFailed(data)
	if err != nil {
		return err
	}
	subject := "Payment failed for your subscription — " + data.WorkspaceName
	return m.Sender.Send(ctx, to, subject, body)
}

// RenderPaymentFailed renders the payment-failed email template to an HTML string.
// Exposed so callers can inspect the output in tests.
func (m *Mailer) RenderPaymentFailed(data PaymentFailedData) (string, error) {
	return m.renderPaymentFailed(data)
}

func (m *Mailer) renderPaymentFailed(data PaymentFailedData) (string, error) {
	td := map[string]string{
		"WorkspaceName":    html.EscapeString(data.WorkspaceName),
		"InvoiceAmount":    html.EscapeString(data.InvoiceAmount),
		"Currency":         html.EscapeString(data.Currency),
		"UpdatePaymentURL": escapeURL(data.UpdatePaymentURL),
	}
	return m.execute("payment-failed.html", td)
}

// SubscriptionChangedData holds the dynamic values for the subscription-changed email.
type SubscriptionChangedData struct {
	// WorkspaceName is the human-readable workspace name.
	WorkspaceName string
	// PlanName is the new plan name (e.g. "Pro", "Team").
	PlanName string
	// Status is the subscription status (e.g. "Active", "Trialing", "Canceled").
	Status string
	// BillingURL is the full URL to the billing management page.
	BillingURL string
}

// SendSubscriptionChanged renders and sends a subscription-changed email to the given address.
func (m *Mailer) SendSubscriptionChanged(ctx context.Context, to string, data SubscriptionChangedData) error {
	body, err := m.renderSubscriptionChanged(data)
	if err != nil {
		return err
	}
	subject := "Your subscription has been updated — " + data.WorkspaceName
	return m.Sender.Send(ctx, to, subject, body)
}

// RenderSubscriptionChanged renders the subscription-changed email template to an HTML string.
// Exposed so callers can inspect the output in tests.
func (m *Mailer) RenderSubscriptionChanged(data SubscriptionChangedData) (string, error) {
	return m.renderSubscriptionChanged(data)
}

func (m *Mailer) renderSubscriptionChanged(data SubscriptionChangedData) (string, error) {
	td := map[string]string{
		"WorkspaceName": html.EscapeString(data.WorkspaceName),
		"PlanName":      html.EscapeString(data.PlanName),
		"Status":        html.EscapeString(data.Status),
		"BillingURL":    escapeURL(data.BillingURL),
	}
	return m.execute("subscription-changed.html", td)
}

// NotificationData holds the dynamic values for an immediate notification email.
type NotificationData struct {
	// Title is the notification headline.
	Title string
	// Body is the notification detail text.
	Body string
	// Category is the notification category label (e.g. "Quality", "Task").
	Category string
	// Priority is "high" or "normal".
	Priority string
	// ActionURL is the URL for the CTA button.
	ActionURL string
	// ActionLabel is the text for the CTA button.
	ActionLabel string
}

// SendNotification renders and sends an immediate notification email.
func (m *Mailer) SendNotification(ctx context.Context, to string, data NotificationData) error {
	body, err := m.renderNotification(data)
	if err != nil {
		return err
	}
	subject := data.Title + " — Bowrain"
	return m.Sender.Send(ctx, to, subject, body)
}

func (m *Mailer) renderNotification(data NotificationData) (string, error) {
	td := map[string]string{
		"Title":       html.EscapeString(data.Title),
		"Body":        html.EscapeString(data.Body),
		"Category":    html.EscapeString(data.Category),
		"Priority":    html.EscapeString(data.Priority),
		"ActionURL":   escapeURL(data.ActionURL),
		"ActionLabel": html.EscapeString(data.ActionLabel),
	}
	return m.execute("notification.html", td)
}

// SendDigest sends a pre-rendered digest email (HTML body built by DigestWorker).
func (m *Mailer) SendDigest(ctx context.Context, to, subject, htmlBody string) error {
	return m.Sender.Send(ctx, to, subject, htmlBody)
}

// escapeURL encodes a URL for safe use inside an HTML attribute value.
// It HTML-escapes & (→ &amp;) and other HTML-special characters so the
// href="…" attribute parses correctly in all email clients.
func escapeURL(u string) string {
	// Replace bare ampersands that are not already part of an entity.
	u = strings.ReplaceAll(u, "&", "&amp;")
	return u
}
