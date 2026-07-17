package mailer_test

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/neokapi/neokapi/bowrain/mailer"
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

	err = m.SendInvite(t.Context(), "user@example.com", "en", data)
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

	html, err := m.RenderInvite("en", data)
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

	html, err := m.RenderInvite("en", mailer.InviteData{
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

// ---------------------------------------------------------------------------
// Credits Warning
// ---------------------------------------------------------------------------

func TestSendCreditsWarning(t *testing.T) {
	sender := &recordingSender{}
	m, err := mailer.New(sender)
	require.NoError(t, err)

	data := mailer.CreditsWarningData{
		WorkspaceName: "Acme Corp",
		UsedCredits:   "8,000",
		TotalCredits:  "10,000",
		UsagePercent:  "80",
		ResetDate:     "April 1, 2026",
		UpgradeURL:    "https://app.bowrain.cloud/billing/upgrade?ws=abc123",
	}

	err = m.SendCreditsWarning(t.Context(), "admin@example.com", "en", data)
	require.NoError(t, err)

	msg := sender.last()
	assert.Equal(t, "admin@example.com", msg.To)
	assert.Contains(t, msg.Subject, "Acme Corp")
	assert.Contains(t, msg.Subject, "credits are running low")
	assert.Contains(t, msg.Body, "Acme Corp")
	assert.Contains(t, msg.Body, "8,000")
	assert.Contains(t, msg.Body, "10,000")
	assert.Contains(t, msg.Body, "80")
	assert.Contains(t, msg.Body, "April 1, 2026")
	assert.Contains(t, msg.Body, "Upgrade Plan")
}

func TestRenderCreditsWarningHTMLEscaping(t *testing.T) {
	sender := &recordingSender{}
	m, err := mailer.New(sender)
	require.NoError(t, err)

	data := mailer.CreditsWarningData{
		WorkspaceName: `Acme & "Co" <Ltd>`,
		UsedCredits:   "800",
		TotalCredits:  "1,000",
		UsagePercent:  "80",
		ResetDate:     "April 1, 2026",
		UpgradeURL:    "https://example.com/upgrade?a=1&b=2",
	}

	html, err := m.RenderCreditsWarning("en", data)
	require.NoError(t, err)

	assert.Contains(t, html, "Acme &amp; &#34;Co&#34; &lt;Ltd&gt;")
	assert.True(t,
		strings.Contains(html, "a=1&amp;b=2") || strings.Contains(html, "a=1&b=2"),
		"URL should appear in rendered HTML",
	)
}

func TestRenderCreditsWarningContainsExpectedElements(t *testing.T) {
	sender := &recordingSender{}
	m, err := mailer.New(sender)
	require.NoError(t, err)

	html, err := m.RenderCreditsWarning("en", mailer.CreditsWarningData{
		WorkspaceName: "TestWS",
		UsedCredits:   "750",
		TotalCredits:  "1,000",
		UsagePercent:  "75",
		ResetDate:     "May 1, 2026",
		UpgradeURL:    "https://example.com/upgrade",
	})
	require.NoError(t, err)

	assert.Contains(t, html, "Bowrain", "should contain brand name")
	assert.Contains(t, html, "Your AI credits are running low", "should contain header text")
	assert.Contains(t, html, "Upgrade Plan", "should contain CTA button text")
	assert.Contains(t, html, "TestWS", "should contain workspace name")
	assert.Contains(t, html, "750", "should contain used credits")
	assert.Contains(t, html, "1,000", "should contain total credits")
	assert.Contains(t, html, "75", "should contain usage percent")
	assert.Contains(t, html, "May 1, 2026", "should contain reset date")
	assert.Contains(t, html, "https://example.com/upgrade", "should contain upgrade URL")
}

// ---------------------------------------------------------------------------
// Credits Exhausted
// ---------------------------------------------------------------------------

func TestSendCreditsExhausted(t *testing.T) {
	sender := &recordingSender{}
	m, err := mailer.New(sender)
	require.NoError(t, err)

	data := mailer.CreditsExhaustedData{
		WorkspaceName: "Acme Corp",
		ResetDate:     "April 1, 2026",
		UpgradeURL:    "https://app.bowrain.cloud/billing/upgrade?ws=abc123",
		BuyCreditsURL: "https://app.bowrain.cloud/billing/credits?ws=abc123",
	}

	err = m.SendCreditsExhausted(t.Context(), "admin@example.com", "en", data)
	require.NoError(t, err)

	msg := sender.last()
	assert.Equal(t, "admin@example.com", msg.To)
	assert.Contains(t, msg.Subject, "Acme Corp")
	assert.Contains(t, msg.Subject, "credits are exhausted")
	assert.Contains(t, msg.Body, "Acme Corp")
	assert.Contains(t, msg.Body, "April 1, 2026")
	assert.Contains(t, msg.Body, "Upgrade Plan")
	assert.Contains(t, msg.Body, "Buy Additional Credits")
}

func TestRenderCreditsExhaustedHTMLEscaping(t *testing.T) {
	sender := &recordingSender{}
	m, err := mailer.New(sender)
	require.NoError(t, err)

	data := mailer.CreditsExhaustedData{
		WorkspaceName: `Acme & "Co" <Ltd>`,
		ResetDate:     "April 1, 2026",
		UpgradeURL:    "https://example.com/upgrade?a=1&b=2",
		BuyCreditsURL: "https://example.com/credits?x=1&y=2",
	}

	html, err := m.RenderCreditsExhausted("en", data)
	require.NoError(t, err)

	assert.Contains(t, html, "Acme &amp; &#34;Co&#34; &lt;Ltd&gt;")
	assert.True(t,
		strings.Contains(html, "a=1&amp;b=2") || strings.Contains(html, "a=1&b=2"),
		"upgrade URL should appear in rendered HTML",
	)
	assert.True(t,
		strings.Contains(html, "x=1&amp;y=2") || strings.Contains(html, "x=1&y=2"),
		"buy credits URL should appear in rendered HTML",
	)
}

func TestRenderCreditsExhaustedContainsExpectedElements(t *testing.T) {
	sender := &recordingSender{}
	m, err := mailer.New(sender)
	require.NoError(t, err)

	html, err := m.RenderCreditsExhausted("en", mailer.CreditsExhaustedData{
		WorkspaceName: "TestWS",
		ResetDate:     "May 1, 2026",
		UpgradeURL:    "https://example.com/upgrade",
		BuyCreditsURL: "https://example.com/credits",
	})
	require.NoError(t, err)

	assert.Contains(t, html, "Bowrain", "should contain brand name")
	assert.Contains(t, html, "Your AI credits are exhausted", "should contain header text")
	assert.Contains(t, html, "Upgrade Plan", "should contain upgrade CTA button text")
	assert.Contains(t, html, "Buy Additional Credits", "should contain buy credits CTA button text")
	assert.Contains(t, html, "TestWS", "should contain workspace name")
	assert.Contains(t, html, "May 1, 2026", "should contain reset date")
	assert.Contains(t, html, "https://example.com/upgrade", "should contain upgrade URL")
	assert.Contains(t, html, "https://example.com/credits", "should contain buy credits URL")
}

// ---------------------------------------------------------------------------
// Payment Failed
// ---------------------------------------------------------------------------

func TestSendPaymentFailed(t *testing.T) {
	sender := &recordingSender{}
	m, err := mailer.New(sender)
	require.NoError(t, err)

	data := mailer.PaymentFailedData{
		WorkspaceName:    "Acme Corp",
		InvoiceAmount:    "49.00",
		Currency:         "USD",
		UpdatePaymentURL: "https://app.bowrain.cloud/billing/payment?ws=abc123",
	}

	err = m.SendPaymentFailed(t.Context(), "admin@example.com", "en", data)
	require.NoError(t, err)

	msg := sender.last()
	assert.Equal(t, "admin@example.com", msg.To)
	assert.Contains(t, msg.Subject, "Acme Corp")
	assert.Contains(t, msg.Subject, "Payment failed")
	assert.Contains(t, msg.Body, "Acme Corp")
	assert.Contains(t, msg.Body, "49.00")
	assert.Contains(t, msg.Body, "USD")
	assert.Contains(t, msg.Body, "Update Payment Method")
}

func TestRenderPaymentFailedHTMLEscaping(t *testing.T) {
	sender := &recordingSender{}
	m, err := mailer.New(sender)
	require.NoError(t, err)

	data := mailer.PaymentFailedData{
		WorkspaceName:    `Acme & "Co" <Ltd>`,
		InvoiceAmount:    "49.00",
		Currency:         "USD",
		UpdatePaymentURL: "https://example.com/payment?a=1&b=2",
	}

	html, err := m.RenderPaymentFailed("en", data)
	require.NoError(t, err)

	assert.Contains(t, html, "Acme &amp; &#34;Co&#34; &lt;Ltd&gt;")
	assert.True(t,
		strings.Contains(html, "a=1&amp;b=2") || strings.Contains(html, "a=1&b=2"),
		"URL should appear in rendered HTML",
	)
}

func TestRenderPaymentFailedContainsExpectedElements(t *testing.T) {
	sender := &recordingSender{}
	m, err := mailer.New(sender)
	require.NoError(t, err)

	html, err := m.RenderPaymentFailed("en", mailer.PaymentFailedData{
		WorkspaceName:    "TestWS",
		InvoiceAmount:    "99.00",
		Currency:         "EUR",
		UpdatePaymentURL: "https://example.com/payment",
	})
	require.NoError(t, err)

	assert.Contains(t, html, "Bowrain", "should contain brand name")
	assert.Contains(t, html, "Payment failed for your subscription", "should contain header text")
	assert.Contains(t, html, "Update Payment Method", "should contain CTA button text")
	assert.Contains(t, html, "TestWS", "should contain workspace name")
	assert.Contains(t, html, "99.00", "should contain invoice amount")
	assert.Contains(t, html, "EUR", "should contain currency")
	assert.Contains(t, html, "https://example.com/payment", "should contain update payment URL")
}

// ---------------------------------------------------------------------------
// Subscription Changed
// ---------------------------------------------------------------------------

func TestSendSubscriptionChanged(t *testing.T) {
	sender := &recordingSender{}
	m, err := mailer.New(sender)
	require.NoError(t, err)

	data := mailer.SubscriptionChangedData{
		WorkspaceName: "Acme Corp",
		PlanName:      "Pro",
		Status:        "Active",
		BillingURL:    "https://app.bowrain.cloud/billing?ws=abc123",
	}

	err = m.SendSubscriptionChanged(t.Context(), "admin@example.com", "en", data)
	require.NoError(t, err)

	msg := sender.last()
	assert.Equal(t, "admin@example.com", msg.To)
	assert.Contains(t, msg.Subject, "Acme Corp")
	assert.Contains(t, msg.Subject, "subscription has been updated")
	assert.Contains(t, msg.Body, "Acme Corp")
	assert.Contains(t, msg.Body, "Pro")
	assert.Contains(t, msg.Body, "Active")
	assert.Contains(t, msg.Body, "View Billing")
}

func TestRenderSubscriptionChangedHTMLEscaping(t *testing.T) {
	sender := &recordingSender{}
	m, err := mailer.New(sender)
	require.NoError(t, err)

	data := mailer.SubscriptionChangedData{
		WorkspaceName: `Acme & "Co" <Ltd>`,
		PlanName:      "Pro",
		Status:        "Active",
		BillingURL:    "https://example.com/billing?a=1&b=2",
	}

	html, err := m.RenderSubscriptionChanged("en", data)
	require.NoError(t, err)

	assert.Contains(t, html, "Acme &amp; &#34;Co&#34; &lt;Ltd&gt;")
	assert.True(t,
		strings.Contains(html, "a=1&amp;b=2") || strings.Contains(html, "a=1&b=2"),
		"URL should appear in rendered HTML",
	)
}

func TestRenderSubscriptionChangedContainsExpectedElements(t *testing.T) {
	sender := &recordingSender{}
	m, err := mailer.New(sender)
	require.NoError(t, err)

	html, err := m.RenderSubscriptionChanged("en", mailer.SubscriptionChangedData{
		WorkspaceName: "TestWS",
		PlanName:      "Team",
		Status:        "Trialing",
		BillingURL:    "https://example.com/billing",
	})
	require.NoError(t, err)

	assert.Contains(t, html, "Bowrain", "should contain brand name")
	assert.Contains(t, html, "Your subscription has been updated", "should contain header text")
	assert.Contains(t, html, "View Billing", "should contain CTA button text")
	assert.Contains(t, html, "TestWS", "should contain workspace name")
	assert.Contains(t, html, "Team", "should contain plan name")
	assert.Contains(t, html, "Trialing", "should contain status")
	assert.Contains(t, html, "https://example.com/billing", "should contain billing URL")
}

// ---------------------------------------------------------------------------
// Digest (raw HTML passthrough)
// ---------------------------------------------------------------------------

func TestSendDigest(t *testing.T) {
	sender := &recordingSender{}
	m, err := mailer.New(sender)
	require.NoError(t, err)

	htmlBody := "<html><body><h1>Daily Digest</h1><p>3 new updates</p></body></html>"
	err = m.SendDigest(t.Context(), "user@example.com", "Your daily digest — 3 updates", htmlBody)
	require.NoError(t, err)

	msg := sender.last()
	assert.Equal(t, "user@example.com", msg.To)
	assert.Equal(t, "Your daily digest — 3 updates", msg.Subject)
	assert.Equal(t, htmlBody, msg.Body)
}

// ---------------------------------------------------------------------------
// Notification (immediate)
// ---------------------------------------------------------------------------

func TestSendNotification(t *testing.T) {
	sender := &recordingSender{}
	m, err := mailer.New(sender)
	require.NoError(t, err)

	data := mailer.NotificationData{
		Title:       "Quality gate failed",
		Body:        "3 terminology violations found in fr-FR",
		Category:    "Quality",
		Priority:    "high",
		ActionURL:   "https://app.bowrain.cloud/ws/acme/quality",
		ActionLabel: "Review Issues",
	}

	// SendNotification uses the notification.html template. If the template
	// hasn't been built (vp run build), we expect a template error. Skip in that case.
	err = m.SendNotification(t.Context(), "user@example.com", "en", data)
	if err != nil {
		t.Skipf("notification template not built: %v", err)
	}

	msg := sender.last()
	assert.Equal(t, "user@example.com", msg.To)
	assert.Contains(t, msg.Subject, "Quality gate failed")
	assert.Contains(t, msg.Body, "3 terminology violations found in fr-FR")
}

// ---------------------------------------------------------------------------
// Per-locale render coverage (dogfood l10n)
// ---------------------------------------------------------------------------

// TestRenderAllTemplatesAllLocales renders every template in every embedded
// locale (en + the localized variant sets produced by `make l10n-emails`)
// and asserts the render succeeds with no unexecuted template tokens left
// in the body or subject. It deliberately does not assert any particular
// translation text: a source change whose seed hasn't caught up regenerates
// the variant with English fallback, which must never fail the build.
func TestRenderAllTemplatesAllLocales(t *testing.T) {
	sender := &recordingSender{}
	m, err := mailer.New(sender)
	require.NoError(t, err)

	// The dogfood pipeline ships pseudo (qps) and Norwegian Bokmål (nb)
	// variant sets alongside English.
	assert.ElementsMatch(t, []string{"en", "qps", "nb"}, m.Locales())

	sends := map[string]func(locale string) error{
		"invite": func(l string) error {
			return m.SendInvite(t.Context(), "to@example.com", l, mailer.InviteData{
				WorkspaceName: "Acme", Role: "member", JoinURL: "https://example.com/join/x",
			})
		},
		"credits-warning": func(l string) error {
			return m.SendCreditsWarning(t.Context(), "to@example.com", l, mailer.CreditsWarningData{
				WorkspaceName: "Acme", UsedCredits: "800", TotalCredits: "1,000",
				UsagePercent: "80", ResetDate: "May 1, 2026", UpgradeURL: "https://example.com/up",
			})
		},
		"credits-exhausted": func(l string) error {
			return m.SendCreditsExhausted(t.Context(), "to@example.com", l, mailer.CreditsExhaustedData{
				WorkspaceName: "Acme", ResetDate: "May 1, 2026",
				UpgradeURL: "https://example.com/up", BuyCreditsURL: "https://example.com/buy",
			})
		},
		"payment-failed": func(l string) error {
			return m.SendPaymentFailed(t.Context(), "to@example.com", l, mailer.PaymentFailedData{
				WorkspaceName: "Acme", InvoiceAmount: "49.00", Currency: "USD",
				UpdatePaymentURL: "https://example.com/pay",
			})
		},
		"subscription-changed": func(l string) error {
			return m.SendSubscriptionChanged(t.Context(), "to@example.com", l, mailer.SubscriptionChangedData{
				WorkspaceName: "Acme", PlanName: "Pro", Status: "Active",
				BillingURL: "https://example.com/billing",
			})
		},
		"notification": func(l string) error {
			return m.SendNotification(t.Context(), "to@example.com", l, mailer.NotificationData{
				Title: "Check failed", Body: "Details", Category: "Quality",
				Priority: "high", ActionURL: "https://example.com/q", ActionLabel: "Review",
			})
		},
		"email-change-verify": func(l string) error {
			return m.SendEmailChangeVerify(t.Context(), "to@example.com", l, mailer.EmailChangeVerifyData{
				NewEmail: "new@example.com", ConfirmURL: "https://example.com/confirm",
				ExpiresIn: "24 hours",
			})
		},
	}

	// "" exercises the empty-locale (unknown-user) fallback; "nb-NO"
	// exercises BCP-47 normalization to the nb variant set.
	for _, locale := range []string{"en", "qps", "nb", "", "nb-NO", "de"} {
		for name, send := range sends {
			t.Run(name+"/"+locale, func(t *testing.T) {
				require.NoError(t, send(locale))
				msg := sender.last()
				assert.NotEmpty(t, msg.Subject)
				assert.NotEmpty(t, msg.Body)
				// Every Go template token must have been executed.
				assert.NotContains(t, msg.Subject, "{{")
				assert.NotContains(t, msg.Body, "{{.")
				// No KLF inline-markup tokens may leak from a stale catalog.
				assert.NotContains(t, msg.Body, "{=m")
			})
		}
	}
}
