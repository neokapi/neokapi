package auth

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Claims are the JWT claims carried in neokapi access tokens.
type Claims struct {
	jwt.RegisteredClaims
	Email string `json:"email"`
	Name  string `json:"name"`
}

const (
	// TokenIssuer identifies Bowrain as the minter of its own session JWTs. It is
	// validated on every parse, so a token signed with the session secret for
	// some other purpose can never be replayed as a session token.
	TokenIssuer = "bowrain"
	// TokenAudience scopes a regular user session JWT to the Bowrain API.
	// Validation requires this audience, rejecting a token minted for a
	// different consumer.
	TokenAudience = "bowrain-api"
	// TokenAudienceAdmin scopes an admin control-plane session JWT. It is a
	// DISTINCT audience from TokenAudience so an admin session token can never be
	// accepted on a user route and a user session token can never be accepted on
	// an admin route — the audience check enforces the admin/user split.
	TokenAudienceAdmin = "bowrain-admin"
)

// ErrEmptySecret is returned when a JWT operation is attempted with an empty
// HMAC secret. Signing or validating with "" is a misconfiguration that would
// otherwise produce trivially forgeable tokens, so we fail closed.
var ErrEmptySecret = errors.New("jwt: empty signing secret")

// generateToken mints a signed HS256 JWT with the Bowrain issuer and the given
// audience.
func generateToken(subject, email, name, secret, audience string, expiry time.Duration) (string, error) {
	if secret == "" {
		return "", ErrEmptySecret
	}
	now := time.Now()
	claims := Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    TokenIssuer,
			Subject:   subject,
			Audience:  jwt.ClaimStrings{audience},
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(expiry)),
		},
		Email: email,
		Name:  name,
	}
	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
	if err != nil {
		return "", fmt.Errorf("sign token: %w", err)
	}
	return signed, nil
}

// GenerateToken creates a signed user session JWT (audience bowrain-api).
func GenerateToken(user *User, secret string, expiry time.Duration) (string, error) {
	return generateToken(user.ID, user.Email, user.Name, secret, TokenAudience, expiry)
}

// GenerateAdminToken creates a signed admin control-plane session JWT (audience
// bowrain-admin). The identity comes from the admin-realm id_token (subject,
// email, name), not the user store — admin identity is deliberately separate.
func GenerateAdminToken(subject, email, name, secret string, expiry time.Duration) (string, error) {
	return generateToken(subject, email, name, secret, TokenAudienceAdmin, expiry)
}

// validateToken parses and validates an HS256 JWT, requiring the Bowrain issuer,
// the given audience, and an expiry. A token for a different audience (or one
// with no expiry, or signed with a non-HS256 method) is rejected.
func validateToken(tokenString, secret, audience string) (*Claims, error) {
	if secret == "" {
		return nil, ErrEmptySecret
	}
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(t *jwt.Token) (any, error) {
		return []byte(secret), nil
	},
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithIssuer(TokenIssuer),
		jwt.WithAudience(audience),
		jwt.WithExpirationRequired(),
	)
	if err != nil {
		return nil, fmt.Errorf("parse token: %w", err)
	}
	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, errors.New("invalid token claims")
	}
	return claims, nil
}

// ValidateToken verifies a user session JWT (audience bowrain-api).
func ValidateToken(tokenString, secret string) (*Claims, error) {
	return validateToken(tokenString, secret, TokenAudience)
}

// ValidateAdminToken verifies an admin control-plane session JWT (audience
// bowrain-admin). A user session token fails this check and an admin session
// token fails ValidateToken, keeping the two identities strictly separated.
func ValidateAdminToken(tokenString, secret string) (*Claims, error) {
	return validateToken(tokenString, secret, TokenAudienceAdmin)
}

// GenerateRefreshToken returns a cryptographically random opaque token string.
func GenerateRefreshToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate refresh token: %w", err)
	}
	return base64.URLEncoding.EncodeToString(b), nil
}
