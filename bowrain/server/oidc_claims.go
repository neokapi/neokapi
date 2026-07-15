package server

import (
	"errors"
	"strconv"

	"github.com/coreos/go-oidc/v3/oidc"
)

// flexBool unmarshals a JSON boolean that different IdPs encode differently:
// Keycloak emits a real JSON bool (true/false), while some AWS Cognito token
// variants emit the string "true"/"false". An absent or unparseable value is
// treated as false (fail-safe: unknown means unverified).
type flexBool bool

func (b *flexBool) UnmarshalJSON(data []byte) error {
	s := string(data)
	// Strip surrounding quotes for the string-encoded form.
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		s = s[1 : len(s)-1]
	}
	if s == "" || s == "null" {
		*b = false
		return nil
	}
	v, err := strconv.ParseBool(s)
	if err != nil {
		// Unknown encoding → unverified, rather than erroring the whole login.
		*b = false
		return nil
	}
	*b = flexBool(v)
	return nil
}

// oidcIdentityClaims are the identity claims Bowrain consumes from a verified
// ID token. email_verified is enforced before an account is created or linked,
// because login provisioning links accounts by email (see AuthService.
// GetOrCreateUser): an IdP asserting an UNVERIFIED email could otherwise link a
// fresh IdP identity into an existing Bowrain account (account takeover),
// which is a real risk once federated social login is enabled.
type oidcIdentityClaims struct {
	Email         string   `json:"email"`
	Name          string   `json:"name"`
	EmailVerified flexBool `json:"email_verified"`
}

// errEmailNotVerified is returned when a login presents an unverified email and
// verification is required.
var errEmailNotVerified = errors.New("email address is not verified")

// enforceVerified rejects an unverified email when requireVerified is set. It is
// pure so it can be unit-tested without fabricating an oidc.IDToken.
func enforceVerified(claims oidcIdentityClaims, requireVerified bool) error {
	if requireVerified && !bool(claims.EmailVerified) {
		return errEmailNotVerified
	}
	return nil
}

// identityFromToken extracts the identity claims from a verified ID token and,
// when requireVerified is set, rejects an unverified email. It is the single
// choke point every OIDC callback (web, desktop, device) routes through so the
// verified-email gate cannot be bypassed via one of the flows.
func identityFromToken(idToken *oidc.IDToken, requireVerified bool) (oidcIdentityClaims, error) {
	var claims oidcIdentityClaims
	if err := idToken.Claims(&claims); err != nil {
		return claims, err
	}
	if err := enforceVerified(claims, requireVerified); err != nil {
		return claims, err
	}
	return claims, nil
}
