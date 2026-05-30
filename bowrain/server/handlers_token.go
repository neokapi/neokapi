package server

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	platev "github.com/neokapi/neokapi/bowrain/core/event"
)

// CreateTokenRequest is the request body for creating an API token.
type CreateTokenRequest struct {
	Name       string   `json:"name"`
	ExpireDays int      `json:"expire_days,omitempty"` // 0 = no expiration
	Scopes     []string `json:"scopes,omitempty"`      // e.g. ["*"], ["read"], ["translate:fr,de"]
}

// CreateTokenResponse is returned after creating a token. The plaintext
// token is shown only once.
type CreateTokenResponse struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	TokenPrefix string     `json:"token_prefix"`
	Token       string     `json:"token"` // plaintext, shown once
	Scopes      string     `json:"scopes"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
}

// HandleCreateToken creates a new API token for the workspace.
func (s *Server) HandleCreateToken(c echo.Context) error {
	if s.Services == nil || s.Services.Auth == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "auth not configured"})
	}

	var req CreateTokenRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	}
	if req.Name == "" {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "name is required"})
	}

	userID, _ := c.Get("user_id").(string)
	workspaceID, _ := c.Get("workspace_id").(string)

	// Build scopes JSON. Default to ["*"] if no scopes provided.
	scopesJSON := `["*"]`
	if len(req.Scopes) > 0 {
		b, err := json.Marshal(req.Scopes)
		if err != nil {
			return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "invalid scopes"})
		}
		scopesJSON = string(b)
		if err := platauth.ValidateScopes(scopesJSON); err != nil {
			return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "invalid scopes: " + err.Error()})
		}
	}

	var expiresAt *time.Time
	if req.ExpireDays > 0 {
		t := time.Now().Add(time.Duration(req.ExpireDays) * 24 * time.Hour)
		expiresAt = &t
	}

	ctx := c.Request().Context()
	token, plaintext, err := s.Services.Auth.CreateAPIToken(ctx, userID, workspaceID, req.Name, scopesJSON, expiresAt)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}
	s.emitAudit(c, auditEvent{
		Type:         platev.EventTokenCreated,
		WorkspaceID:  workspaceID,
		ResourceType: "api_token",
		ResourceID:   token.ID,
		Data:         map[string]string{"name": token.Name, "scopes": token.Scopes},
	})

	return c.JSON(http.StatusCreated, CreateTokenResponse{
		ID:          token.ID,
		Name:        token.Name,
		TokenPrefix: token.TokenPrefix,
		Token:       plaintext,
		Scopes:      token.Scopes,
		ExpiresAt:   token.ExpiresAt,
		CreatedAt:   token.CreatedAt,
	})
}

// HandleListTokens lists all API tokens for the workspace.
func (s *Server) HandleListTokens(c echo.Context) error {
	if s.AuthStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "auth not configured"})
	}

	workspaceID, _ := c.Get("workspace_id").(string)
	ctx := c.Request().Context()

	tokens, err := s.AuthStore.ListAPITokens(ctx, workspaceID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}

	return c.JSON(http.StatusOK, tokens)
}

// HandleDeleteToken revokes an API token.
func (s *Server) HandleDeleteToken(c echo.Context) error {
	if s.AuthStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "auth not configured"})
	}

	tokenID := c.Param("id")
	ctx := c.Request().Context()

	if err := s.AuthStore.DeleteAPIToken(ctx, tokenID); err != nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: err.Error()})
	}
	s.emitAudit(c, auditEvent{
		Type:         platev.EventTokenRevoked,
		ResourceType: "api_token",
		ResourceID:   tokenID,
	})

	return c.NoContent(http.StatusNoContent)
}
