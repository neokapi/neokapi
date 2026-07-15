package auth

import "context"

// IdentityAdmin is the provider-neutral seam for administrative writes to the
// upstream identity provider. Bowrain owns its own user records and mints its
// own session, so it only reaches into the IdP to keep provider-side state
// (currently the email) in sync with a Bowrain-initiated, already-verified
// change. Keeping this a tiny interface lets the hosted platform run a managed
// IdP (e.g. AWS Cognito) while self-hosted / dev deployments keep Keycloak,
// with the rest of the server unaware of which is in play.
type IdentityAdmin interface {
	// UpdateUserEmail sets a new email for the IdP user identified by subject
	// (the OIDC `sub`) and marks it verified — Bowrain only calls this after it
	// has itself verified ownership of newEmail via the email-change flow.
	UpdateUserEmail(ctx context.Context, subject, newEmail string) error
}

// Compile-time assertion that the Keycloak admin client satisfies the port.
// The Cognito adapter (hosted prod) will assert the same in its own file.
var _ IdentityAdmin = (*KeycloakAdminClient)(nil)
