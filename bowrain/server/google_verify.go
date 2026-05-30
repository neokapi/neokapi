package server

import (
	"context"
	"crypto"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	jose "github.com/go-jose/go-jose/v4"
	"github.com/golang-jwt/jwt/v5"
	"github.com/labstack/echo/v4"
)

// googleCertsURL is Google's JWKS endpoint for OpenID/identity tokens.
const googleCertsURL = "https://www.googleapis.com/oauth2/v3/certs"

// googleIssuers are the accepted `iss` values for a Google-signed ID token.
var googleIssuers = map[string]bool{
	"accounts.google.com":         true,
	"https://accounts.google.com": true,
}

// googleTokenVerifier validates the system ID token Google's Workspace add-on
// runtime sends as the Bearer token on every call to the add-on's HTTP
// endpoints. It checks the RS256 signature against Google's JWKS, the issuer,
// the audience (the add-on's configured identity), expiry, and — when
// configured — the calling service-account email. This proves the request came
// from Google's add-on runtime for *this* add-on, not an arbitrary caller.
//
// Reference: https://developers.google.com/workspace/add-ons/guides/alternate-runtimes#verify_authenticity_of_requests
type googleTokenVerifier struct {
	audiences []string // accepted `aud` values (deployment URL and/or project number)
	saEmails  []string // optional accepted service-account emails (empty = skip)
	certsURL  string
	client    *http.Client

	mu        sync.RWMutex
	keys      map[string]crypto.PublicKey
	fetchedAt time.Time
	ttl       time.Duration
}

func newGoogleTokenVerifier(audiences, saEmails []string) *googleTokenVerifier {
	return &googleTokenVerifier{
		audiences: audiences,
		saEmails:  saEmails,
		certsURL:  googleCertsURL,
		client:    &http.Client{Timeout: 10 * time.Second},
		ttl:       time.Hour,
	}
}

// keyForKid returns the RSA public key for the given key id, refreshing the
// cached JWKS on a miss or once the TTL has elapsed.
func (v *googleTokenVerifier) keyForKid(ctx context.Context, kid string) (crypto.PublicKey, error) {
	v.mu.RLock()
	key, ok := v.keys[kid]
	fresh := time.Since(v.fetchedAt) < v.ttl
	v.mu.RUnlock()
	if ok && fresh {
		return key, nil
	}
	if err := v.refresh(ctx); err != nil {
		return nil, err
	}
	v.mu.RLock()
	defer v.mu.RUnlock()
	if key, ok := v.keys[kid]; ok {
		return key, nil
	}
	return nil, fmt.Errorf("no signing key for kid %q", kid)
}

func (v *googleTokenVerifier) refresh(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, v.certsURL, nil)
	if err != nil {
		return err
	}
	resp, err := v.client.Do(req)
	if err != nil {
		return fmt.Errorf("fetch google certs: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("fetch google certs: HTTP %d", resp.StatusCode)
	}
	var jwks jose.JSONWebKeySet
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&jwks); err != nil {
		return fmt.Errorf("decode google certs: %w", err)
	}
	keys := make(map[string]crypto.PublicKey, len(jwks.Keys))
	for _, k := range jwks.Keys {
		keys[k.KeyID] = k.Key
	}
	if len(keys) == 0 {
		return errors.New("google certs: empty key set")
	}
	v.mu.Lock()
	v.keys = keys
	v.fetchedAt = time.Now()
	v.mu.Unlock()
	return nil
}

// verify validates the token and returns nil only if every check passes.
func (v *googleTokenVerifier) verify(ctx context.Context, tokenStr string) error {
	claims := jwt.MapClaims{}
	_, err := jwt.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (any, error) {
		kid, _ := t.Header["kid"].(string)
		if kid == "" {
			return nil, errors.New("missing kid")
		}
		return v.keyForKid(ctx, kid)
	}, jwt.WithValidMethods([]string{"RS256"}), jwt.WithExpirationRequired())
	if err != nil {
		return err
	}

	iss, _ := claims["iss"].(string)
	if !googleIssuers[iss] {
		return fmt.Errorf("unexpected issuer %q", iss)
	}
	if !audienceMatches(claims, v.audiences) {
		return errors.New("audience mismatch")
	}
	if len(v.saEmails) > 0 {
		email, _ := claims["email"].(string)
		if !contains(v.saEmails, email) {
			return errors.New("unexpected service-account email")
		}
	}
	return nil
}

// audienceMatches reports whether the token's `aud` (string or []string)
// intersects the accepted set.
func audienceMatches(claims jwt.MapClaims, accepted []string) bool {
	switch aud := claims["aud"].(type) {
	case string:
		return contains(accepted, aud)
	case []any:
		for _, a := range aud {
			if s, ok := a.(string); ok && contains(accepted, s) {
				return true
			}
		}
	}
	return false
}

func contains(set []string, v string) bool {
	for _, s := range set {
		if s == v {
			return true
		}
	}
	return false
}

// verifyGoogleAddonRequest is Echo middleware that rejects any request to the
// Google Workspace add-on endpoints whose Bearer system ID token does not
// validate against Google's keys and the configured add-on identity.
func verifyGoogleAddonRequest(audiences, saEmails []string) echo.MiddlewareFunc {
	v := newGoogleTokenVerifier(audiences, saEmails)
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			header := c.Request().Header.Get("Authorization")
			token := strings.TrimPrefix(header, "Bearer ")
			if token == "" || token == header {
				return c.JSON(http.StatusUnauthorized, ErrorResponse{Error: "missing bearer token"})
			}
			if err := v.verify(c.Request().Context(), token); err != nil {
				slog.Warn("google addon: rejected request", "error", err, "path", c.Path())
				return c.JSON(http.StatusUnauthorized, ErrorResponse{Error: "unauthorized"})
			}
			return next(c)
		}
	}
}
