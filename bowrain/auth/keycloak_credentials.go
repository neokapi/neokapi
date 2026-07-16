package auth

import (
	"context"
	"encoding/json"
	"strings"
)

// KeycloakCredentialManager implements CredentialManager for self-host / dev
// Keycloak. Keycloak cannot server-relay a WebAuthn *registration* ceremony
// (registration runs through a browser required-action in its own account
// console), so rather than build a half-parity this adapter reports InApp()
// == false and hands off to the Keycloak Account Console — which self-host
// users already have and which lists, adds, and deletes passkeys natively.
type KeycloakCredentialManager struct {
	// accountURL is the deep link into the account console's passkeys section,
	// e.g. https://auth.example.com/realms/bowrain/account/#/security/signing-in
	accountURL string
}

// NewKeycloakCredentialManager builds the adapter from the realm's browser-
// facing base URL (the OIDC issuer, e.g.
// "https://auth.example.com/realms/bowrain"). It appends the account-console
// passkeys deep link. Returns a manager with an empty AccountURL if issuer is
// blank (the handlers still degrade gracefully).
func NewKeycloakCredentialManager(issuerURL string) *KeycloakCredentialManager {
	base := strings.TrimRight(strings.TrimSpace(issuerURL), "/")
	url := ""
	if base != "" {
		// #/security/signing-in is the passkeys ("Signing in") section of the
		// Keycloak account console. Verify the fragment against the Keycloak
		// version if the console layout changes.
		url = base + "/account/#/security/signing-in"
	}
	return &KeycloakCredentialManager{accountURL: url}
}

// InApp is false: Keycloak manages credentials via its account console.
func (m *KeycloakCredentialManager) InApp() bool { return false }

// AccountURL returns the account-console passkeys deep link. The console is
// session-scoped, so sub is ignored.
func (m *KeycloakCredentialManager) AccountURL(string) string { return m.accountURL }

// The in-app operations are never reached when InApp() is false; they return
// ErrNotInApp defensively so a misrouted call fails loudly rather than silently.

func (m *KeycloakCredentialManager) ListPasskeys(context.Context, string) ([]Passkey, error) {
	return nil, ErrNotInApp
}

func (m *KeycloakCredentialManager) RegisterStart(context.Context, string) (json.RawMessage, error) {
	return nil, ErrNotInApp
}

func (m *KeycloakCredentialManager) RegisterFinish(context.Context, string, json.RawMessage) error {
	return ErrNotInApp
}

func (m *KeycloakCredentialManager) DeletePasskey(context.Context, string, string) error {
	return ErrNotInApp
}

// Compile-time assertion that the Keycloak credential manager satisfies the port.
var _ CredentialManager = (*KeycloakCredentialManager)(nil)
