package event

import (
	"context"
	"fmt"

	bstore "github.com/neokapi/neokapi/bowrain/store"
	"github.com/neokapi/neokapi/bowrain/mailer"
	bauth "github.com/neokapi/neokapi/bowrain/auth"
)

// categoryActionLabels maps notification categories to email CTA button labels.
var categoryActionLabels = map[string]string{
	string(bstore.CategoryTask):       "Open Task",
	string(bstore.CategoryQuality):    "Review Issues",
	string(bstore.CategoryAutomation): "View Flow",
}

// MailerAdapter implements DigestEmailer by sending immediate emails
// via the Mailer for high-priority notifications.
type MailerAdapter struct {
	mailer    *mailer.Mailer
	authStore bauth.AuthStore
}

// NewMailerAdapter creates a MailerAdapter that resolves user emails via AuthStore
// and sends immediate notification emails via the Mailer.
func NewMailerAdapter(m *mailer.Mailer, authStore bauth.AuthStore) *MailerAdapter {
	return &MailerAdapter{mailer: m, authStore: authStore}
}

// SendImmediate sends an immediate email notification to the given user.
func (a *MailerAdapter) SendImmediate(ctx context.Context, userID string, notification *bstore.Notification) error {
	u, err := a.authStore.GetUser(ctx, userID)
	if err != nil {
		return fmt.Errorf("resolve user %s: %w", userID, err)
	}
	if u.Email == "" {
		return nil
	}

	actionLabel := "View Details"
	if label, ok := categoryActionLabels[notification.Category]; ok {
		actionLabel = label
	}

	return a.mailer.SendNotification(ctx, u.Email, mailer.NotificationData{
		Title:       notification.Title,
		Body:        notification.Body,
		Category:    notification.Category,
		Priority:    notification.Priority,
		ActionURL:   notification.LinkURL,
		ActionLabel: actionLabel,
	})
}
