package server

import (
	"context"
	"fmt"
	"log"
	"net/http"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/platform/store"
	"github.com/labstack/echo/v4"
)

// AnonymousProjectRequest is the request body for creating an anonymous project.
type AnonymousProjectRequest struct {
	Name          string   `json:"name"`
	SourceLocale  string   `json:"source_locale"`
	TargetLocales []string `json:"target_locales"` // optional; empty = dynamic
	Email         string   `json:"email"`          // optional; if set, server sends claim email
}

// AnonymousProjectResponse is the response body for anonymous project creation.
type AnonymousProjectResponse struct {
	ProjectID  string `json:"project_id"`
	ClaimToken string `json:"claim_token"`
	ExpiresAt  string `json:"expires_at"`
}

// HandleCreateAnonymousProject creates an anonymous project with a claim token.
// No authentication required. target_locales is optional (empty = dynamic locales).
// If email is provided, the server sends a claim email to that address.
func (s *Server) HandleCreateAnonymousProject(c echo.Context) error {
	if s.Services == nil || s.Services.Auth == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "auth not configured"})
	}

	var req AnonymousProjectRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	}

	if req.Name == "" || req.SourceLocale == "" {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "name and source_locale are required"})
	}

	ctx := c.Request().Context()
	projectID, claimToken, err := s.Services.Auth.CreateAnonymousProject(ctx, req.Name, req.SourceLocale, req.TargetLocales)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}

	// Also create the project in the content store so blocks can reference it.
	if s.Services.Project != nil {
		targetLocales := make([]model.LocaleID, len(req.TargetLocales))
		for i, l := range req.TargetLocales {
			targetLocales[i] = model.LocaleID(l)
		}
		p := &store.Project{
			ID:            projectID,
			Name:          req.Name,
			SourceLocale:  model.LocaleID(req.SourceLocale),
			TargetLocales: targetLocales,
		}
		if err := s.Services.Project.CreateProject(ctx, p); err != nil {
			return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "create content project: " + err.Error()})
		}
	}

	// Send claim email if email is provided and SMTP is configured.
	if req.Email != "" && s.EmailSender != nil {
		baseURL := requestBaseURL(c)
		go s.sendClaimEmail(context.Background(), req.Email, projectID, claimToken, baseURL)
	}

	return c.JSON(http.StatusCreated, AnonymousProjectResponse{
		ProjectID:  projectID,
		ClaimToken: claimToken,
	})
}

// ClaimRequest is the request body for claiming an anonymous project.
type ClaimRequest struct {
	ClaimToken string `json:"claim_token"`
}

// ClaimResponse is the response body for a successful claim.
type ClaimResponse struct {
	ProjectID     string `json:"project_id"`
	WorkspaceSlug string `json:"workspace_slug"`
}

// HandleClaimProject claims an anonymous project into the user's personal workspace.
// Requires JWT authentication.
func (s *Server) HandleClaimProject(c echo.Context) error {
	if s.Services == nil || s.Services.Auth == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "auth not configured"})
	}

	var req ClaimRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	}

	if req.ClaimToken == "" {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "claim_token is required"})
	}

	userID, _ := c.Get("user_id").(string)
	if userID == "" {
		return c.JSON(http.StatusUnauthorized, ErrorResponse{Error: "not authenticated"})
	}

	ctx := c.Request().Context()
	projectID, wsSlug, err := s.Services.Auth.ClaimProject(ctx, userID, req.ClaimToken)
	if err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	}

	// Associate the project with the workspace in the content store.
	if s.Services.Project != nil && s.AuthStore != nil {
		ws, err := s.AuthStore.GetWorkspaceBySlug(ctx, wsSlug)
		if err == nil {
			p, err := s.Services.Project.GetProject(ctx, projectID)
			if err == nil {
				p.WorkspaceID = ws.ID
				_ = s.Services.Project.UpdateProject(ctx, p)
			}
		}
	}

	return c.JSON(http.StatusOK, ClaimResponse{
		ProjectID:     projectID,
		WorkspaceSlug: wsSlug,
	})
}

// sendClaimEmail sends an HTML email with the claim token so the user can claim their project.
func (s *Server) sendClaimEmail(ctx context.Context, email, projectID, claimToken, baseURL string) {
	claimURL := fmt.Sprintf("%s/claim?token=%s", baseURL, claimToken)

	subject := "Your Bowrain Project Claim Token"
	body := fmt.Sprintf(`<!DOCTYPE html>
<html>
<body style="font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; max-width: 480px; margin: 0 auto; padding: 24px;">
<h2 style="margin-bottom: 8px;">Your Project is Ready</h2>
<p>Your project <strong>%s</strong> has been created on Bowrain.</p>
<p>To claim this project and link it to your account, click the button below:</p>
<p style="margin: 24px 0;">
  <a href="%s" style="display: inline-block; padding: 12px 24px; background-color: #2563eb; color: #ffffff; text-decoration: none; border-radius: 6px; font-weight: 500;">
    Claim Project
  </a>
</p>
<p style="color: #6b7280; font-size: 13px;">
  Or copy this link: <a href="%s">%s</a>
</p>
</body>
</html>`, projectID, claimURL, claimURL, claimURL)

	if err := s.EmailSender.Send(ctx, email, subject, body); err != nil {
		log.Printf("failed to send claim email to %s: %v", email, err)
	}
}
