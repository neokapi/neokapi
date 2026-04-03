package server

import (
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
)

const tokenExchangeExpiry = 1 * time.Hour

// TokenExchangeResponse is the OAuth 2.0-style response for the token exchange endpoint.
type TokenExchangeResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"` // seconds
}

// HandleTokenExchange exchanges a validated API token (bwt_*) for a short-lived JWT.
// The caller must already be authenticated via AuthMiddleware (which validates the
// API token and sets user_id, email, name on the context).
func (s *Server) HandleTokenExchange(c echo.Context) error {
	if s.Services == nil || s.Services.Auth == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "auth not configured"})
	}

	userID, _ := c.Get("user_id").(string)
	email, _ := c.Get("email").(string)
	name, _ := c.Get("name").(string)

	if userID == "" {
		return c.JSON(http.StatusUnauthorized, ErrorResponse{Error: "not authenticated"})
	}

	user := &platauth.User{
		ID:    userID,
		Email: email,
		Name:  name,
	}

	token, err := s.Services.Auth.GenerateToken(user, tokenExchangeExpiry)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "generate token: " + err.Error()})
	}

	return c.JSON(http.StatusOK, TokenExchangeResponse{
		AccessToken: token,
		TokenType:   "Bearer",
		ExpiresIn:   int(tokenExchangeExpiry.Seconds()),
	})
}
