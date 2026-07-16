package auth

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

// ErrNotInApp is returned by a CredentialManager whose InApp() is false when a
// caller nonetheless invokes an in-app operation (list/register/delete). Those
// providers (Keycloak) manage credentials through their own account console, to
// which callers should redirect via AccountURL instead.
var ErrNotInApp = errors.New("credential manager does not support in-app management")

// Passkey is a provider-neutral view of a registered WebAuthn credential.
type Passkey struct {
	ID         string    // provider credential id (opaque; used for delete)
	Name       string    // friendly name, may be empty
	CreatedAt  time.Time // when the credential was registered
	RPID       string    // relying-party id it was registered under
	Transports []string  // usb, internal, hybrid, …
}

// CredentialManager is the provider-neutral seam for a user's self-service
// credential management (passkeys today; password/MFA later). Unlike
// IdentityAdmin — which performs admin writes with the server's own role or
// service-account secret — these are USER-scoped operations: they act as the
// signed-in user and need that user's upstream access token, obtained on demand
// from the retained refresh token (see UpstreamTokenStore). The token is
// threaded in by the handler, never stored on the adapter.
//
// The port abstracts the decision (InApp), not a forced-identical
// implementation: hosted Cognito relays the WebAuthn ceremony inline, while
// self-host Keycloak hands off to its mature account console.
type CredentialManager interface {
	// InApp reports whether passkey management runs inline (server-relayed
	// ceremony). Cognito returns true; Keycloak returns false and exposes
	// AccountURL instead.
	InApp() bool

	// AccountURL returns the IdP's self-service console URL for the user
	// identified by sub when InApp() is false (Keycloak). Empty when InApp() is
	// true. sub is accepted for providers that scope the console per-user; the
	// Keycloak console is session-scoped and ignores it.
	AccountURL(sub string) string

	// ListPasskeys returns the user's registered WebAuthn credentials.
	ListPasskeys(ctx context.Context, accessToken string) ([]Passkey, error)

	// RegisterStart returns the raw PublicKeyCredentialCreationOptions JSON to
	// hand to navigator.credentials.create(). Opaque passthrough — the server
	// does not interpret it.
	RegisterStart(ctx context.Context, accessToken string) (json.RawMessage, error)

	// RegisterFinish submits the browser's attestation (raw PublicKeyCredential
	// JSON) back to the provider to complete registration.
	RegisterFinish(ctx context.Context, accessToken string, attestation json.RawMessage) error

	// DeletePasskey removes a registered credential by its provider id.
	DeletePasskey(ctx context.Context, accessToken, credentialID string) error
}
