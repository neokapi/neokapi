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
	// TokenAudience scopes a session JWT to the Bowrain API. Validation requires
	// this audience, rejecting a token minted for a different consumer.
	TokenAudience = "bowrain-api"
)

// ErrEmptySecret is returned when a JWT operation is attempted with an empty
// HMAC secret. Signing or validating with "" is a misconfiguration that would
// otherwise produce trivially forgeable tokens, so we fail closed.
var ErrEmptySecret = errors.New("jwt: empty signing secret")

// GenerateToken creates a signed JWT for the given user.
func GenerateToken(user *User, secret string, expiry time.Duration) (string, error) {
	if secret == "" {
		return "", ErrEmptySecret
	}
	now := time.Now()
	claims := Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    TokenIssuer,
			Subject:   user.ID,
			Audience:  jwt.ClaimStrings{TokenAudience},
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(expiry)),
		},
		Email: user.Email,
		Name:  user.Name,
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(secret))
	if err != nil {
		return "", fmt.Errorf("sign token: %w", err)
	}
	return signed, nil
}

// ValidateToken verifies a JWT string and returns the embedded claims. It pins
// the signing method to HS256, requires the Bowrain issuer and audience, and
// requires an expiry — so a token that is valid for some other purpose (or has
// no expiry) is rejected as a session token.
func ValidateToken(tokenString, secret string) (*Claims, error) {
	if secret == "" {
		return nil, ErrEmptySecret
	}
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(t *jwt.Token) (any, error) {
		return []byte(secret), nil
	},
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithIssuer(TokenIssuer),
		jwt.WithAudience(TokenAudience),
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

// GenerateRefreshToken returns a cryptographically random opaque token string.
func GenerateRefreshToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate refresh token: %w", err)
	}
	return base64.URLEncoding.EncodeToString(b), nil
}
