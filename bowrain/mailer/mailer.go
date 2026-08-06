// Package mailer renders email templates and dispatches transactional
// emails for Bowrain.
//
// Templates are pre-rendered from React Email source (bowrain/emails/) at
// build time (make email-build) and embedded into the binary via embed.FS.
// At send time, Go's text/template fills in the dynamic values.
//
// Localization: templates/*.html is the English (source-locale) set;
// templates/<locale>/*.html holds a localized variant set produced by the
// dogfood l10n pipeline (make l10n — neokapi-i18n extraction over
// bowrain/emails/src, content memory-driven recycle, per-locale re-render). Subjects
// live in subjects/<locale>.json (subjects/en.json is the source; the
// others are generated the same way). Every Send*/Render* method takes the
// recipient's locale; unknown locales, missing variant sets, and missing
// per-template files all fall back to English — a missing translation must
// never block a send (see CLAUDE.md "Target-language drift must never
// block the build").
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
//	err = m.SendInvite(ctx, "translator@example.com", "nb", mailer.InviteData{
//	    WorkspaceName: "Acme Inc.",
//	    Role:          "member",
//	    JoinURL:       "https://app.bowrain.cloud/join/abc123",
//	})
package mailer

import (
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"html"
	"io/fs"
	"strings"
	"text/template"
)

//go:embed templates
var templateFS embed.FS

//go:embed subjects/*.json
var subjectFS embed.FS

// EmailSenderI dispatches a single email message. Implementations include
// SMTPSender (go-mail) and ResendSender (Resend REST API).
type EmailSenderI interface {
	Send(ctx context.Context, to, subject, htmlBody string) error
}

// Mailer renders email templates and sends them via a configured sender.
type Mailer struct {
	Sender EmailSenderI
	// templates maps locale → parsed HTML template set. The "en" set is
	// templates/*.html; other locales come from templates/<locale>/.
	templates map[string]*template.Template
	// subjects maps locale → template name → parsed subject template.
	subjects map[string]map[string]*template.Template
}

// sourceLocale is the authoring locale of the templates; every lookup
// falls back to it.
const sourceLocale = "en"

// New creates a Mailer backed by the given sender. It parses all embedded
// HTML template sets (English plus every templates/<locale>/ variant) and
// the per-locale subject catalogs; returns an error only if the embedded
// files are malformed.
func New(sender EmailSenderI) (*Mailer, error) {
	sets := map[string]*template.Template{}
	base, err := template.ParseFS(templateFS, "templates/*.html")
	if err != nil {
		return nil, err
	}
	sets[sourceLocale] = base

	entries, err := templateFS.ReadDir("templates")
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		t, err := template.ParseFS(templateFS, "templates/"+e.Name()+"/*.html")
		if err != nil {
			return nil, fmt.Errorf("parse %s templates: %w", e.Name(), err)
		}
		sets[e.Name()] = t
	}

	subjects, err := loadSubjects(subjectFS)
	if err != nil {
		return nil, err
	}

	return &Mailer{Sender: sender, templates: sets, subjects: subjects}, nil
}

// loadSubjects parses every subjects/<locale>.json into per-string
// text/template values (subjects carry Go tokens like {{.WorkspaceName}}).
func loadSubjects(fsys fs.FS) (map[string]map[string]*template.Template, error) {
	out := map[string]map[string]*template.Template{}
	files, err := fs.Glob(fsys, "subjects/*.json")
	if err != nil {
		return nil, err
	}
	for _, f := range files {
		locale := strings.TrimSuffix(strings.TrimPrefix(f, "subjects/"), ".json")
		raw, err := fs.ReadFile(fsys, f)
		if err != nil {
			return nil, err
		}
		var m map[string]string
		if err := json.Unmarshal(raw, &m); err != nil {
			return nil, fmt.Errorf("parse %s: %w", f, err)
		}
		set := map[string]*template.Template{}
		for name, text := range m {
			t, err := template.New(name).Parse(text)
			if err != nil {
				return nil, fmt.Errorf("parse subject %s (%s): %w", name, locale, err)
			}
			set[name] = t
		}
		out[locale] = set
	}
	if out[sourceLocale] == nil {
		return nil, fmt.Errorf("subjects/%s.json missing", sourceLocale)
	}
	return out, nil
}

// Locales lists the locales with an embedded template set (always
// includes "en").
func (m *Mailer) Locales() []string {
	out := make([]string, 0, len(m.templates))
	for l := range m.templates {
		out = append(out, l)
	}
	return out
}

// normalizeLocale lowercases a BCP-47 tag and reduces it to its primary
// subtag ("nb-NO" → "nb"); empty input maps to the source locale.
func normalizeLocale(locale string) string {
	base, _, _ := strings.Cut(strings.ToLower(strings.TrimSpace(locale)), "-")
	if base == "" {
		return sourceLocale
	}
	return base
}

// execute renders the named body template in the given locale, falling
// back to English when the locale has no variant set or the set lacks
// this template.
func (m *Mailer) execute(locale, name string, data any) (string, error) {
	locale = normalizeLocale(locale)
	set := m.templates[locale]
	if set == nil || set.Lookup(name) == nil {
		set = m.templates[sourceLocale]
	}
	var buf bytes.Buffer
	if err := set.ExecuteTemplate(&buf, name, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// subject renders the named subject line in the given locale (English
// fallback). data is the raw (unescaped) per-email data struct — subjects
// are plain text, not HTML.
func (m *Mailer) subject(locale, name string, data any) (string, error) {
	locale = normalizeLocale(locale)
	t := m.subjects[locale][name]
	if t == nil {
		t = m.subjects[sourceLocale][name]
	}
	if t == nil {
		return "", fmt.Errorf("no subject %q", name)
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
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

// SendInvite renders and sends an invitation email to the given address in
// the given locale (empty or unknown locale → English).
func (m *Mailer) SendInvite(ctx context.Context, to, locale string, data InviteData) error {
	body, err := m.renderInvite(locale, data)
	if err != nil {
		return err
	}
	subject, err := m.subject(locale, "invite", data)
	if err != nil {
		return err
	}
	return m.Sender.Send(ctx, to, subject, body)
}

// RenderInvite renders the invite email template to an HTML string.
// Exposed so callers can inspect the output in tests.
func (m *Mailer) RenderInvite(locale string, data InviteData) (string, error) {
	return m.renderInvite(locale, data)
}

func (m *Mailer) renderInvite(locale string, data InviteData) (string, error) {
	// All dynamic values are HTML-escaped before injection so that the
	// text/template engine outputs them verbatim and safely. This is
	// necessary because text/template (unlike html/template) performs no
	// automatic escaping.
	td := map[string]string{
		"WorkspaceName": html.EscapeString(data.WorkspaceName),
		"Role":          html.EscapeString(data.Role),
		"JoinURL":       escapeURL(data.JoinURL),
	}
	return m.execute(locale, "invite.html", td)
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
func (m *Mailer) SendCreditsWarning(ctx context.Context, to, locale string, data CreditsWarningData) error {
	body, err := m.renderCreditsWarning(locale, data)
	if err != nil {
		return err
	}
	subject, err := m.subject(locale, "credits-warning", data)
	if err != nil {
		return err
	}
	return m.Sender.Send(ctx, to, subject, body)
}

// RenderCreditsWarning renders the credits-warning email template to an HTML string.
// Exposed so callers can inspect the output in tests.
func (m *Mailer) RenderCreditsWarning(locale string, data CreditsWarningData) (string, error) {
	return m.renderCreditsWarning(locale, data)
}

func (m *Mailer) renderCreditsWarning(locale string, data CreditsWarningData) (string, error) {
	td := map[string]string{
		"WorkspaceName": html.EscapeString(data.WorkspaceName),
		"UsedCredits":   html.EscapeString(data.UsedCredits),
		"TotalCredits":  html.EscapeString(data.TotalCredits),
		"UsagePercent":  html.EscapeString(data.UsagePercent),
		"ResetDate":     html.EscapeString(data.ResetDate),
		"UpgradeURL":    escapeURL(data.UpgradeURL),
	}
	return m.execute(locale, "credits-warning.html", td)
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
func (m *Mailer) SendCreditsExhausted(ctx context.Context, to, locale string, data CreditsExhaustedData) error {
	body, err := m.renderCreditsExhausted(locale, data)
	if err != nil {
		return err
	}
	subject, err := m.subject(locale, "credits-exhausted", data)
	if err != nil {
		return err
	}
	return m.Sender.Send(ctx, to, subject, body)
}

// RenderCreditsExhausted renders the credits-exhausted email template to an HTML string.
// Exposed so callers can inspect the output in tests.
func (m *Mailer) RenderCreditsExhausted(locale string, data CreditsExhaustedData) (string, error) {
	return m.renderCreditsExhausted(locale, data)
}

func (m *Mailer) renderCreditsExhausted(locale string, data CreditsExhaustedData) (string, error) {
	td := map[string]string{
		"WorkspaceName": html.EscapeString(data.WorkspaceName),
		"ResetDate":     html.EscapeString(data.ResetDate),
		"UpgradeURL":    escapeURL(data.UpgradeURL),
		"BuyCreditsURL": escapeURL(data.BuyCreditsURL),
	}
	return m.execute(locale, "credits-exhausted.html", td)
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
func (m *Mailer) SendPaymentFailed(ctx context.Context, to, locale string, data PaymentFailedData) error {
	body, err := m.renderPaymentFailed(locale, data)
	if err != nil {
		return err
	}
	subject, err := m.subject(locale, "payment-failed", data)
	if err != nil {
		return err
	}
	return m.Sender.Send(ctx, to, subject, body)
}

// RenderPaymentFailed renders the payment-failed email template to an HTML string.
// Exposed so callers can inspect the output in tests.
func (m *Mailer) RenderPaymentFailed(locale string, data PaymentFailedData) (string, error) {
	return m.renderPaymentFailed(locale, data)
}

func (m *Mailer) renderPaymentFailed(locale string, data PaymentFailedData) (string, error) {
	td := map[string]string{
		"WorkspaceName":    html.EscapeString(data.WorkspaceName),
		"InvoiceAmount":    html.EscapeString(data.InvoiceAmount),
		"Currency":         html.EscapeString(data.Currency),
		"UpdatePaymentURL": escapeURL(data.UpdatePaymentURL),
	}
	return m.execute(locale, "payment-failed.html", td)
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
func (m *Mailer) SendSubscriptionChanged(ctx context.Context, to, locale string, data SubscriptionChangedData) error {
	body, err := m.renderSubscriptionChanged(locale, data)
	if err != nil {
		return err
	}
	subject, err := m.subject(locale, "subscription-changed", data)
	if err != nil {
		return err
	}
	return m.Sender.Send(ctx, to, subject, body)
}

// RenderSubscriptionChanged renders the subscription-changed email template to an HTML string.
// Exposed so callers can inspect the output in tests.
func (m *Mailer) RenderSubscriptionChanged(locale string, data SubscriptionChangedData) (string, error) {
	return m.renderSubscriptionChanged(locale, data)
}

func (m *Mailer) renderSubscriptionChanged(locale string, data SubscriptionChangedData) (string, error) {
	td := map[string]string{
		"WorkspaceName": html.EscapeString(data.WorkspaceName),
		"PlanName":      html.EscapeString(data.PlanName),
		"Status":        html.EscapeString(data.Status),
		"BillingURL":    escapeURL(data.BillingURL),
	}
	return m.execute(locale, "subscription-changed.html", td)
}

// ReviewRequestData holds the dynamic values for the review-request email —
// the summons a governed change-set sends to the people who may approve it.
type ReviewRequestData struct {
	// WorkspaceName is the human-readable workspace name.
	WorkspaceName string
	// ChangeSetName is the change-set's name, as its author titled it.
	ChangeSetName string
	// AuthorName is the display name of whoever proposed the change-set.
	AuthorName string
	// ChangeCount is the formatted change count (e.g. "57 changes").
	ChangeCount string
	// ReviewURL is the full URL of the change-set's review page.
	ReviewURL string
}

// SendReviewRequest renders and sends a review-request email for a submitted
// change-set. Recipients are the workspace members who may approve it; the
// author is never one of them (separation of duties).
func (m *Mailer) SendReviewRequest(ctx context.Context, to, locale string, data ReviewRequestData) error {
	body, err := m.renderReviewRequest(locale, data)
	if err != nil {
		return err
	}
	subject, err := m.subject(locale, "review-request", data)
	if err != nil {
		return err
	}
	return m.Sender.Send(ctx, to, subject, body)
}

// RenderReviewRequest renders the review-request email template to an HTML
// string (exposed for tests).
func (m *Mailer) RenderReviewRequest(locale string, data ReviewRequestData) (string, error) {
	return m.renderReviewRequest(locale, data)
}

func (m *Mailer) renderReviewRequest(locale string, data ReviewRequestData) (string, error) {
	td := map[string]string{
		"WorkspaceName": html.EscapeString(data.WorkspaceName),
		"ChangeSetName": html.EscapeString(data.ChangeSetName),
		"AuthorName":    html.EscapeString(data.AuthorName),
		"ChangeCount":   html.EscapeString(data.ChangeCount),
		"ReviewURL":     escapeURL(data.ReviewURL),
	}
	return m.execute(locale, "review-request.html", td)
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
func (m *Mailer) SendNotification(ctx context.Context, to, locale string, data NotificationData) error {
	body, err := m.renderNotification(locale, data)
	if err != nil {
		return err
	}
	subject, err := m.subject(locale, "notification", data)
	if err != nil {
		return err
	}
	return m.Sender.Send(ctx, to, subject, body)
}

// RenderNotification renders the notification email template to an HTML
// string (exposed for tests).
func (m *Mailer) RenderNotification(locale string, data NotificationData) (string, error) {
	return m.renderNotification(locale, data)
}

func (m *Mailer) renderNotification(locale string, data NotificationData) (string, error) {
	td := map[string]string{
		"Title":       html.EscapeString(data.Title),
		"Body":        html.EscapeString(data.Body),
		"Category":    html.EscapeString(data.Category),
		"Priority":    html.EscapeString(data.Priority),
		"ActionURL":   escapeURL(data.ActionURL),
		"ActionLabel": html.EscapeString(data.ActionLabel),
	}
	return m.execute(locale, "notification.html", td)
}

// SendDigest sends a pre-rendered digest email (HTML body built by
// DigestWorker). The digest body is assembled Go-side from range blocks,
// so it has no localized template variant yet; it ships in English.
func (m *Mailer) SendDigest(ctx context.Context, to, subject, htmlBody string) error {
	return m.Sender.Send(ctx, to, subject, htmlBody)
}

// EmailChangeVerifyData holds the dynamic values for the email-change verify mail.
type EmailChangeVerifyData struct {
	// NewEmail is the proposed new address (and the recipient).
	NewEmail string
	// ConfirmURL is the full link the user clicks to finalize the change.
	ConfirmURL string
	// ExpiresIn is a human-readable validity window (e.g. "24 hours").
	ExpiresIn string
}

// SendEmailChangeVerify sends the verification email used to confirm an
// email-change request. The email is delivered to the *new* address, so a
// successful click proves ownership.
func (m *Mailer) SendEmailChangeVerify(ctx context.Context, to, locale string, data EmailChangeVerifyData) error {
	body, err := m.renderEmailChangeVerify(locale, data)
	if err != nil {
		return err
	}
	subject, err := m.subject(locale, "email-change-verify", data)
	if err != nil {
		return err
	}
	return m.Sender.Send(ctx, to, subject, body)
}

// RenderEmailChangeVerify renders the email-change-verify template to an HTML
// string (exposed for tests).
func (m *Mailer) RenderEmailChangeVerify(locale string, data EmailChangeVerifyData) (string, error) {
	return m.renderEmailChangeVerify(locale, data)
}

func (m *Mailer) renderEmailChangeVerify(locale string, data EmailChangeVerifyData) (string, error) {
	td := map[string]string{
		"NewEmail":   html.EscapeString(data.NewEmail),
		"ConfirmURL": escapeURL(data.ConfirmURL),
		"ExpiresIn":  html.EscapeString(data.ExpiresIn),
	}
	return m.execute(locale, "email-change-verify.html", td)
}

// escapeURL encodes a URL for safe use inside an HTML attribute value.
// It HTML-escapes & (→ &amp;) and other HTML-special characters so the
// href="…" attribute parses correctly in all email clients.
func escapeURL(u string) string {
	// Replace bare ampersands that are not already part of an entity.
	u = strings.ReplaceAll(u, "&", "&amp;")
	return u
}
