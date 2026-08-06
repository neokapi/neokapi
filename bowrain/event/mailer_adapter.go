package event

import (
	"context"
	"errors"
	"fmt"
	"strings"

	bauth "github.com/neokapi/neokapi/bowrain/auth"
	"github.com/neokapi/neokapi/bowrain/mailer"
	bstore "github.com/neokapi/neokapi/bowrain/store"
)

// categoryActionLabels maps notification categories to email CTA button labels.
var categoryActionLabels = map[string]string{
	string(bstore.CategoryTask):       "Open Task",
	string(bstore.CategoryQuality):    "Review Issues",
	string(bstore.CategoryAutomation): "View Flow",
}

// MailerAdapter implements DigestEmailer by sending immediate emails
// via the Mailer for high-priority notifications, and TaskAssignmentEmailer for
// the assignments urgent enough to leave the queue.
type MailerAdapter struct {
	mailer    *mailer.Mailer
	authStore bauth.AuthStore
	appOrigin string
}

// NewMailerAdapter creates a MailerAdapter that resolves user emails via AuthStore
// and sends immediate notification emails via the Mailer.
//
// appOrigin is this application's browser-facing origin (Config.AppPublicURL).
// It is what turns an in-app path into a link a mail can follow; empty means the
// deployment has not been told its own address, and messages whose only action
// is a link are skipped rather than sent broken.
func NewMailerAdapter(m *mailer.Mailer, authStore bauth.AuthStore, appOrigin string) *MailerAdapter {
	return &MailerAdapter{
		mailer:    m,
		authStore: authStore,
		appOrigin: strings.TrimSuffix(appOrigin, "/"),
	}
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

	return a.mailer.SendNotification(ctx, u.Email, u.Locale, mailer.NotificationData{
		Title:       notification.Title,
		Body:        notification.Body,
		Category:    notification.Category,
		Priority:    notification.Priority,
		ActionURL:   notification.LinkURL,
		ActionLabel: actionLabel,
	})
}

// errNoAppOrigin is what a link-only message reports when this deployment has
// not been told its own browser-facing address.
var errNoAppOrigin = errors.New("no app origin configured; set BOWRAIN_APP_PUBLIC_URL")

// SendTaskAssigned sends the dedicated assignment email for one task. The
// dispatcher has already decided the task is urgent enough and that the
// recipient still wants task mail; this resolves the people and the link.
func (a *MailerAdapter) SendTaskAssigned(ctx context.Context, userID string, task *bstore.Task, workspaceSlug string) error {
	if a.appOrigin == "" {
		return errNoAppOrigin
	}
	u, err := a.authStore.GetUser(ctx, userID)
	if err != nil {
		return fmt.Errorf("resolve user %s: %w", userID, err)
	}
	if u == nil || u.Email == "" {
		return nil
	}

	workspaceName := workspaceSlug
	if ws, err := a.authStore.GetWorkspace(ctx, task.WorkspaceID); err == nil && ws != nil && ws.Name != "" {
		workspaceName = ws.Name
	}

	// An unresolvable assigner reads as somebody rather than as nothing, and a
	// system-opened task says so plainly instead of naming an id.
	assigner := "Someone"
	switch task.CreatedBy {
	case "", "system":
		assigner = "Bowrain"
	default:
		if au, err := a.authStore.GetUser(ctx, task.CreatedBy); err == nil && au != nil && au.Name != "" {
			assigner = au.Name
		}
	}

	return a.mailer.SendTaskAssigned(ctx, u.Email, u.Locale, mailer.TaskAssignedData{
		WorkspaceName:   workspaceName,
		TaskTitle:       task.Title,
		TaskDescription: task.Description,
		Priority:        string(task.Priority),
		AssignerName:    assigner,
		TaskURL:         a.appOrigin + "/" + workspaceSlug + "/tasks",
	})
}
