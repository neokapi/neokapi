package server

import (
	"context"
	"log/slog"

	"github.com/labstack/echo/v4"

	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	"github.com/neokapi/neokapi/bowrain/mailer"
)

// The greeting: the one message an account gets for coming into existence.
//
// It hangs off onboarding rather than off sign-in. A signup is only an identity
// — no workspace, nothing to open, nothing the mail could point at — and the
// login path cannot tell a first sign-in from a second except by a ten-second
// window on created_at, which a slow OIDC round trip or a retried callback
// widens into a duplicate. Onboarding is the moment the account becomes a
// workspace: the email, the display name and the locale are on the user row,
// and the destination exists to be linked.
//
// Once per account is a property of the write, not of this code: the marker is
// users.onboarded_at, and CompleteOnboarding reports the caller that won the
// conditional flip from NULL. An account that already had a personal workspace
// — an invited member who onboards later, an account predating the marker, a
// double-submitted form — never wins it, and so is never greeted twice.

// greetNewAccount mails the welcome email when this onboarding call is the one
// that provisioned the account's first workspace.
//
// Best-effort, and off the request: the workspace exists by the time this runs,
// and a mail server that is slow or absent must not turn a completed onboarding
// into a failed one.
func (s *Server) greetNewAccount(c echo.Context, userID string, w *platauth.Workspace, firstWorkspace bool) {
	if !firstWorkspace || userID == "" || w == nil {
		return
	}
	if s.Mailer == nil || s.AuthStore == nil {
		return
	}
	go s.sendWelcomeEmail(context.WithoutCancel(c.Request().Context()), userID, w.Name, requestBaseURL(c)+"/"+w.Slug)
}

// sendWelcomeEmail renders and sends the greeting in the new member's own
// locale. Every failure is logged and swallowed: nothing downstream of a
// completed onboarding depends on the mail arriving.
func (s *Server) sendWelcomeEmail(ctx context.Context, userID, workspaceName, workspaceURL string) {
	u, err := s.AuthStore.GetUser(ctx, userID)
	if err != nil || u == nil || u.Email == "" {
		slog.WarnContext(ctx, "welcome email: no address for the new account; it goes ungreeted",
			"user_id", userID, "error", err)
		return
	}
	data := mailer.WelcomeData{
		WorkspaceName: workspaceName,
		WorkspaceURL:  workspaceURL,
	}
	if err := s.Mailer.SendWelcome(ctx, u.Email, u.Locale, data); err != nil {
		slog.WarnContext(ctx, "welcome email failed", "user_id", userID, "error", err)
	}
}
