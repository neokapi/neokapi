package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/cognitoidentityprovider"
	cipdocument "github.com/aws/aws-sdk-go-v2/service/cognitoidentityprovider/document"
)

// cognitoCredentialAPI is the slice of the Cognito Identity Provider client the
// credential manager uses. Narrowing it to an interface keeps the adapter
// unit-testable with a fake that never reaches AWS. All four operations are
// ACCESS-TOKEN authorized (they act as the signed-in user), so — unlike the
// admin client's AdminUpdateUserAttributes — they need no IAM permission and no
// task-role policy.
type cognitoCredentialAPI interface {
	ListWebAuthnCredentials(ctx context.Context, in *cognitoidentityprovider.ListWebAuthnCredentialsInput, optFns ...func(*cognitoidentityprovider.Options)) (*cognitoidentityprovider.ListWebAuthnCredentialsOutput, error)
	StartWebAuthnRegistration(ctx context.Context, in *cognitoidentityprovider.StartWebAuthnRegistrationInput, optFns ...func(*cognitoidentityprovider.Options)) (*cognitoidentityprovider.StartWebAuthnRegistrationOutput, error)
	CompleteWebAuthnRegistration(ctx context.Context, in *cognitoidentityprovider.CompleteWebAuthnRegistrationInput, optFns ...func(*cognitoidentityprovider.Options)) (*cognitoidentityprovider.CompleteWebAuthnRegistrationOutput, error)
	DeleteWebAuthnCredential(ctx context.Context, in *cognitoidentityprovider.DeleteWebAuthnCredentialInput, optFns ...func(*cognitoidentityprovider.Options)) (*cognitoidentityprovider.DeleteWebAuthnCredentialOutput, error)
}

// CognitoCredentialManager implements CredentialManager against an AWS Cognito
// user pool. It relays the WebAuthn ceremony to the browser: the server holds
// the user's access token (minted on demand from the retained refresh token)
// and never exposes it; the browser only runs navigator.credentials.* on the
// relayed challenge. Safe for concurrent use — the underlying SDK client is.
type CognitoCredentialManager struct {
	api cognitoCredentialAPI
}

// NewCognitoCredentialManager builds a credential manager using the ambient AWS
// config (default credential chain). The region is taken from cfg (derived from
// the OIDC issuer via CognitoConfigFromIssuer, mirroring the admin client) and
// falls back to the ambient config's region when empty. The pool ID is not
// needed — these operations are scoped by the caller's access token, not the
// pool.
func NewCognitoCredentialManager(ctx context.Context, cfg CognitoAdminConfig) (*CognitoCredentialManager, error) {
	var opts []func(*awsconfig.LoadOptions) error
	if r := strings.TrimSpace(cfg.Region); r != "" {
		opts = append(opts, awsconfig.WithRegion(r))
	}
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("load aws config for cognito credentials: %w", err)
	}
	return &CognitoCredentialManager{api: cognitoidentityprovider.NewFromConfig(awsCfg)}, nil
}

// InApp reports that Cognito supports the server-relayed in-app ceremony.
func (m *CognitoCredentialManager) InApp() bool { return true }

// AccountURL is empty for Cognito — management is in-app, not via a console.
func (m *CognitoCredentialManager) AccountURL(string) string { return "" }

func (m *CognitoCredentialManager) ListPasskeys(ctx context.Context, accessToken string) ([]Passkey, error) {
	if accessToken == "" {
		return nil, errors.New("access token is required")
	}
	out, err := m.api.ListWebAuthnCredentials(ctx, &cognitoidentityprovider.ListWebAuthnCredentialsInput{
		AccessToken: aws.String(accessToken),
	})
	if err != nil {
		return nil, fmt.Errorf("cognito list webauthn credentials: %w", err)
	}
	passkeys := make([]Passkey, 0, len(out.Credentials))
	for _, cred := range out.Credentials {
		pk := Passkey{
			ID:         aws.ToString(cred.CredentialId),
			Name:       aws.ToString(cred.FriendlyCredentialName),
			RPID:       aws.ToString(cred.RelyingPartyId),
			Transports: cred.AuthenticatorTransports,
		}
		if cred.CreatedAt != nil {
			pk.CreatedAt = *cred.CreatedAt
		}
		passkeys = append(passkeys, pk)
	}
	return passkeys, nil
}

func (m *CognitoCredentialManager) RegisterStart(ctx context.Context, accessToken string) (json.RawMessage, error) {
	if accessToken == "" {
		return nil, errors.New("access token is required")
	}
	out, err := m.api.StartWebAuthnRegistration(ctx, &cognitoidentityprovider.StartWebAuthnRegistrationInput{
		AccessToken: aws.String(accessToken),
	})
	if err != nil {
		return nil, fmt.Errorf("cognito start webauthn registration: %w", err)
	}
	if out.CredentialCreationOptions == nil {
		return nil, errors.New("cognito returned empty credential creation options")
	}
	// Cognito is a REST-JSON protocol, so the smithy document marshals straight
	// to JSON bytes — the opaque PublicKeyCredentialCreationOptions the browser
	// hands to navigator.credentials.create(). The server never interprets it.
	raw, err := out.CredentialCreationOptions.MarshalSmithyDocument()
	if err != nil {
		return nil, fmt.Errorf("marshal credential creation options: %w", err)
	}
	return json.RawMessage(raw), nil
}

func (m *CognitoCredentialManager) RegisterFinish(ctx context.Context, accessToken string, attestation json.RawMessage) error {
	if accessToken == "" {
		return errors.New("access token is required")
	}
	if len(attestation) == 0 {
		return errors.New("attestation is required")
	}
	// Round-trip the browser's attestation JSON through a generic value so the
	// lazy document marshals it as a JSON object (not as an opaque byte string).
	var v any
	if err := json.Unmarshal(attestation, &v); err != nil {
		return fmt.Errorf("parse attestation: %w", err)
	}
	_, err := m.api.CompleteWebAuthnRegistration(ctx, &cognitoidentityprovider.CompleteWebAuthnRegistrationInput{
		AccessToken: aws.String(accessToken),
		Credential:  cipdocument.NewLazyDocument(v),
	})
	if err != nil {
		return fmt.Errorf("cognito complete webauthn registration: %w", err)
	}
	return nil
}

func (m *CognitoCredentialManager) DeletePasskey(ctx context.Context, accessToken, credentialID string) error {
	if accessToken == "" {
		return errors.New("access token is required")
	}
	if credentialID == "" {
		return errors.New("credential id is required")
	}
	_, err := m.api.DeleteWebAuthnCredential(ctx, &cognitoidentityprovider.DeleteWebAuthnCredentialInput{
		AccessToken:  aws.String(accessToken),
		CredentialId: aws.String(credentialID),
	})
	if err != nil {
		return fmt.Errorf("cognito delete webauthn credential: %w", err)
	}
	return nil
}

// Compile-time assertion that the Cognito credential manager satisfies the port.
var _ CredentialManager = (*CognitoCredentialManager)(nil)
