package server

import (
	"encoding/json"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/neokapi/neokapi/bowrain/analytics"
	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	platev "github.com/neokapi/neokapi/bowrain/core/event"
	"github.com/neokapi/neokapi/bowrain/service"
)

// CreateTokenRequest is the request body for creating an API token.
type CreateTokenRequest struct {
	Name       string   `json:"name"`
	ExpireDays int      `json:"expire_days,omitempty"` // 0 = no expiration
	Scopes     []string `json:"scopes,omitempty"`      // e.g. ["*"], ["read"], ["translate:fr,de"]
	// ToolName names the tool this token is for, such as a CI workflow or a
	// kapi-action step. Setting it makes the token a tool token: the server
	// records work done under it as the tool's ("tool/<ToolName>"), issued by
	// the member who mints it, and that member can review the tool's work.
	// Omitting it leaves the token a personal one.
	ToolName string `json:"tool_name,omitempty"`
}

// CreateTokenResponse is returned after creating a token. The plaintext
// token is shown only once.
type CreateTokenResponse struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	ToolName    string     `json:"tool_name,omitempty"`
	IssuedBy    string     `json:"issued_by,omitempty"`
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
	// A token mints no token: a tool acts for its issuer and stops when its
	// own token is revoked.
	if a := requestActorOf(c); a.isTool() {
		return c.JSON(http.StatusForbidden, ErrorResponse{
			Error: "a tool token cannot mint tokens: sign in as a member to mint one", Code: toolTokenRefusedCode,
		})
	}
	if err := platauth.ValidateToolName(req.ToolName); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
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
	token, plaintext, err := s.Services.Auth.CreateAPIToken(ctx, service.APITokenSpec{
		UserID:      userID,
		WorkspaceID: workspaceID,
		Name:        req.Name,
		ToolName:    req.ToolName,
		ScopesJSON:  scopesJSON,
		ExpiresAt:   expiresAt,
	})
	if err != nil {
		return serverErr(c, err)
	}
	auditData := map[string]string{"name": token.Name, "scopes": token.Scopes}
	if token.IsTool() {
		// The identity, not the bare name: this is the string that appears
		// as the author of every change-set the tool proposes.
		auditData["author_identity"] = token.AuthorIdentity()
	}
	s.emitAudit(c, auditEvent{
		Type:         platev.EventTokenCreated,
		WorkspaceID:  workspaceID,
		ResourceType: "api_token",
		ResourceID:   token.ID,
		Data:         auditData,
	})

	// The machine channel, counted where it starts. `machine` separates a token
	// minted for a CI job or an agent from one a person mints for their own
	// laptop; `scope` says what that channel was trusted with. Neither the token
	// nor its name travels — a token name is something a person typed.
	props := analytics.Props(workspaceID, "")
	props["machine"] = token.IsTool()
	props["scope"] = scopeActions(token.Scopes)
	s.trackEvent(userID, analytics.EventAPITokenCreated, props)

	return c.JSON(http.StatusCreated, CreateTokenResponse{
		ID:          token.ID,
		Name:        token.Name,
		ToolName:    token.ToolName,
		IssuedBy:    token.IssuedBy,
		TokenPrefix: token.TokenPrefix,
		Token:       plaintext,
		Scopes:      token.Scopes,
		ExpiresAt:   token.ExpiresAt,
		CreatedAt:   token.CreatedAt,
	})
}

// scopeActions renders a token's scopes as the sorted, comma-joined set of
// actions they grant. The raw scope strings carry project ids and language
// constraints, which would splinter the analytics value into one bucket per
// token; the action is the part that says what kind of channel this is.
func scopeActions(scopesJSON string) string {
	var scopes []string
	if err := json.Unmarshal([]byte(scopesJSON), &scopes); err != nil {
		return ""
	}
	seen := make(map[string]struct{}, len(scopes))
	actions := make([]string, 0, len(scopes))
	for _, s := range scopes {
		resolved, err := platauth.ParseScope(s)
		if err != nil {
			continue
		}
		action := string(resolved.Action)
		if _, dup := seen[action]; dup {
			continue
		}
		seen[action] = struct{}{}
		actions = append(actions, action)
	}
	slices.Sort(actions)
	return strings.Join(actions, ",")
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
		return serverErr(c, err)
	}

	return c.JSON(http.StatusOK, tokens)
}

// HandleDeleteToken revokes an API token.
func (s *Server) HandleDeleteToken(c echo.Context) error {
	if s.AuthStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "auth not configured"})
	}

	tokenID := c.Param("id")
	workspaceID, _ := c.Get("workspace_id").(string)
	ctx := c.Request().Context()

	// Only a token of this workspace is revoked here. Revoking deletes the
	// token and nothing else: the issuer's sign-in and other tokens stay, and
	// the records the token made keep its id and tool name.
	tokens, err := s.AuthStore.ListAPITokens(ctx, workspaceID)
	if err != nil {
		return serverErr(c, err)
	}
	idx := slices.IndexFunc(tokens, func(t *platauth.APIToken) bool { return t.ID == tokenID })
	if idx < 0 {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: "api token " + tokenID + " not found"})
	}
	revoked := tokens[idx]
	if err := s.AuthStore.DeleteAPIToken(ctx, tokenID); err != nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: err.Error()})
	}
	data := map[string]string{"name": revoked.Name}
	if revoked.IsTool() {
		data["tool"] = revoked.ToolName
		data["issued_by"] = revoked.IssuedBy
	}
	s.emitAudit(c, auditEvent{
		Type:         platev.EventTokenRevoked,
		ResourceType: "api_token",
		ResourceID:   tokenID,
		Data:         data,
	})

	return c.NoContent(http.StatusNoContent)
}
