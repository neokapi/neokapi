package server

import (
	"net/http"

	"github.com/labstack/echo/v4"
)

// AnonymousProjectRequest is the request body for creating an anonymous project.
type AnonymousProjectRequest struct {
	Name          string   `json:"name"`
	SourceLocale  string   `json:"source_locale"`
	TargetLocales []string `json:"target_locales"`
}

// AnonymousProjectResponse is the response body for anonymous project creation.
type AnonymousProjectResponse struct {
	ProjectID  string `json:"project_id"`
	ClaimToken string `json:"claim_token"`
	ExpiresAt  string `json:"expires_at"`
}

// HandleCreateAnonymousProject creates an anonymous project with a claim token.
// No authentication required.
func (s *Server) HandleCreateAnonymousProject(c echo.Context) error {
	if s.Services == nil || s.Services.Auth == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "auth not configured"})
	}

	var req AnonymousProjectRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	}

	if req.Name == "" || req.SourceLocale == "" || len(req.TargetLocales) == 0 {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "name, source_locale, and target_locales are required"})
	}

	ctx := c.Request().Context()
	projectID, claimToken, err := s.Services.Auth.CreateAnonymousProject(ctx, req.Name, req.SourceLocale, req.TargetLocales)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}

	// Look up the unclaimed project to get the expiry.
	// The claim token is the plaintext; we return it to the caller.
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

	return c.JSON(http.StatusOK, ClaimResponse{
		ProjectID:     projectID,
		WorkspaceSlug: wsSlug,
	})
}
