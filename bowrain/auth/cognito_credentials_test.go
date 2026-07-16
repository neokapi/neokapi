package auth

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cognitoidentityprovider"
	cipdocument "github.com/aws/aws-sdk-go-v2/service/cognitoidentityprovider/document"
	ciptypes "github.com/aws/aws-sdk-go-v2/service/cognitoidentityprovider/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeCognitoCredentialAPI records inputs and returns configurable outputs for
// each WebAuthn operation, so the adapter is testable without reaching AWS.
type fakeCognitoCredentialAPI struct {
	listIn   *cognitoidentityprovider.ListWebAuthnCredentialsInput
	listOut  *cognitoidentityprovider.ListWebAuthnCredentialsOutput
	listErr  error
	startIn  *cognitoidentityprovider.StartWebAuthnRegistrationInput
	startOut *cognitoidentityprovider.StartWebAuthnRegistrationOutput
	startErr error
	compIn   *cognitoidentityprovider.CompleteWebAuthnRegistrationInput
	compErr  error
	delIn    *cognitoidentityprovider.DeleteWebAuthnCredentialInput
	delErr   error
}

func (f *fakeCognitoCredentialAPI) ListWebAuthnCredentials(_ context.Context, in *cognitoidentityprovider.ListWebAuthnCredentialsInput, _ ...func(*cognitoidentityprovider.Options)) (*cognitoidentityprovider.ListWebAuthnCredentialsOutput, error) {
	f.listIn = in
	if f.listErr != nil {
		return nil, f.listErr
	}
	return f.listOut, nil
}

func (f *fakeCognitoCredentialAPI) StartWebAuthnRegistration(_ context.Context, in *cognitoidentityprovider.StartWebAuthnRegistrationInput, _ ...func(*cognitoidentityprovider.Options)) (*cognitoidentityprovider.StartWebAuthnRegistrationOutput, error) {
	f.startIn = in
	if f.startErr != nil {
		return nil, f.startErr
	}
	return f.startOut, nil
}

func (f *fakeCognitoCredentialAPI) CompleteWebAuthnRegistration(_ context.Context, in *cognitoidentityprovider.CompleteWebAuthnRegistrationInput, _ ...func(*cognitoidentityprovider.Options)) (*cognitoidentityprovider.CompleteWebAuthnRegistrationOutput, error) {
	f.compIn = in
	if f.compErr != nil {
		return nil, f.compErr
	}
	return &cognitoidentityprovider.CompleteWebAuthnRegistrationOutput{}, nil
}

func (f *fakeCognitoCredentialAPI) DeleteWebAuthnCredential(_ context.Context, in *cognitoidentityprovider.DeleteWebAuthnCredentialInput, _ ...func(*cognitoidentityprovider.Options)) (*cognitoidentityprovider.DeleteWebAuthnCredentialOutput, error) {
	f.delIn = in
	if f.delErr != nil {
		return nil, f.delErr
	}
	return &cognitoidentityprovider.DeleteWebAuthnCredentialOutput{}, nil
}

func TestCognitoCredentialManager_InApp(t *testing.T) {
	m := &CognitoCredentialManager{api: &fakeCognitoCredentialAPI{}}
	assert.True(t, m.InApp())
	assert.Empty(t, m.AccountURL("sub-1"))
}

func TestCognitoCredentialManager_ListPasskeys(t *testing.T) {
	created := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
	fake := &fakeCognitoCredentialAPI{
		listOut: &cognitoidentityprovider.ListWebAuthnCredentialsOutput{
			Credentials: []ciptypes.WebAuthnCredentialDescription{
				{
					CredentialId:            aws.String("cred-1"),
					FriendlyCredentialName:  aws.String("My Laptop"),
					RelyingPartyId:          aws.String("bowrain.cloud"),
					AuthenticatorTransports: []string{"internal", "hybrid"},
					CreatedAt:               &created,
				},
			},
		},
	}
	m := &CognitoCredentialManager{api: fake}

	got, err := m.ListPasskeys(context.Background(), "AT-abc")
	require.NoError(t, err)
	// Access token threaded through.
	require.NotNil(t, fake.listIn)
	assert.Equal(t, "AT-abc", aws.ToString(fake.listIn.AccessToken))
	// Mapped correctly.
	require.Len(t, got, 1)
	assert.Equal(t, "cred-1", got[0].ID)
	assert.Equal(t, "My Laptop", got[0].Name)
	assert.Equal(t, "bowrain.cloud", got[0].RPID)
	assert.Equal(t, []string{"internal", "hybrid"}, got[0].Transports)
	assert.True(t, created.Equal(got[0].CreatedAt))
}

func TestCognitoCredentialManager_ListPasskeys_RequiresToken(t *testing.T) {
	m := &CognitoCredentialManager{api: &fakeCognitoCredentialAPI{}}
	_, err := m.ListPasskeys(context.Background(), "")
	require.Error(t, err)
}

func TestCognitoCredentialManager_RegisterStart_RoundTripsOpaqueJSON(t *testing.T) {
	options := map[string]any{
		"challenge": "Y2hhbGxlbmdl",
		"rp":        map[string]any{"id": "bowrain.cloud", "name": "Bowrain"},
		"user":      map[string]any{"id": "dXNlcg", "name": "a@b.co"},
	}
	fake := &fakeCognitoCredentialAPI{
		startOut: &cognitoidentityprovider.StartWebAuthnRegistrationOutput{
			CredentialCreationOptions: cipdocument.NewLazyDocument(options),
		},
	}
	m := &CognitoCredentialManager{api: fake}

	raw, err := m.RegisterStart(context.Background(), "AT-xyz")
	require.NoError(t, err)
	assert.Equal(t, "AT-xyz", aws.ToString(fake.startIn.AccessToken))

	// The returned bytes are valid JSON carrying the same fields, opaquely.
	var back map[string]any
	require.NoError(t, json.Unmarshal(raw, &back))
	assert.Equal(t, "Y2hhbGxlbmdl", back["challenge"])
	rp, ok := back["rp"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "bowrain.cloud", rp["id"])
}

func TestCognitoCredentialManager_RegisterFinish_PassesAttestation(t *testing.T) {
	fake := &fakeCognitoCredentialAPI{}
	m := &CognitoCredentialManager{api: fake}

	attestation := json.RawMessage(`{"id":"cred-1","type":"public-key","response":{"clientDataJSON":"abc"}}`)
	err := m.RegisterFinish(context.Background(), "AT-1", attestation)
	require.NoError(t, err)
	require.NotNil(t, fake.compIn)
	assert.Equal(t, "AT-1", aws.ToString(fake.compIn.AccessToken))
	require.NotNil(t, fake.compIn.Credential)

	// The document round-trips back to the same structure (REST-JSON bytes).
	raw, err := fake.compIn.Credential.MarshalSmithyDocument()
	require.NoError(t, err)
	var back map[string]any
	require.NoError(t, json.Unmarshal(raw, &back))
	assert.Equal(t, "cred-1", back["id"])
	resp, ok := back["response"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "abc", resp["clientDataJSON"])
}

func TestCognitoCredentialManager_RegisterFinish_RejectsBadInput(t *testing.T) {
	m := &CognitoCredentialManager{api: &fakeCognitoCredentialAPI{}}
	require.Error(t, m.RegisterFinish(context.Background(), "", json.RawMessage(`{}`)))
	require.Error(t, m.RegisterFinish(context.Background(), "AT", nil))
	require.Error(t, m.RegisterFinish(context.Background(), "AT", json.RawMessage(`not json`)))
}

func TestCognitoCredentialManager_DeletePasskey(t *testing.T) {
	fake := &fakeCognitoCredentialAPI{}
	m := &CognitoCredentialManager{api: fake}

	err := m.DeletePasskey(context.Background(), "AT-9", "cred-42")
	require.NoError(t, err)
	require.NotNil(t, fake.delIn)
	assert.Equal(t, "AT-9", aws.ToString(fake.delIn.AccessToken))
	assert.Equal(t, "cred-42", aws.ToString(fake.delIn.CredentialId))
}

func TestCognitoCredentialManager_DeletePasskey_RequiresArgs(t *testing.T) {
	m := &CognitoCredentialManager{api: &fakeCognitoCredentialAPI{}}
	require.Error(t, m.DeletePasskey(context.Background(), "", "cred"))
	require.Error(t, m.DeletePasskey(context.Background(), "AT", ""))
}

func TestCognitoCredentialManager_PropagatesAPIErrors(t *testing.T) {
	sentinel := errors.New("boom")
	m := &CognitoCredentialManager{api: &fakeCognitoCredentialAPI{listErr: sentinel, startErr: sentinel, compErr: sentinel, delErr: sentinel}}

	_, err := m.ListPasskeys(context.Background(), "AT")
	require.ErrorContains(t, err, "boom")
	_, err = m.RegisterStart(context.Background(), "AT")
	require.ErrorContains(t, err, "boom")
	require.ErrorContains(t, m.RegisterFinish(context.Background(), "AT", json.RawMessage(`{}`)), "boom")
	require.ErrorContains(t, m.DeletePasskey(context.Background(), "AT", "cred"), "boom")
}

func TestKeycloakCredentialManager(t *testing.T) {
	m := NewKeycloakCredentialManager("https://auth.example.com/realms/bowrain")
	assert.False(t, m.InApp())
	assert.Equal(t, "https://auth.example.com/realms/bowrain/account/#/security/signing-in", m.AccountURL("sub-1"))

	// In-app ops are refused defensively.
	_, err := m.ListPasskeys(context.Background(), "AT")
	require.ErrorIs(t, err, ErrNotInApp)
	_, err = m.RegisterStart(context.Background(), "AT")
	require.ErrorIs(t, err, ErrNotInApp)
	require.ErrorIs(t, m.RegisterFinish(context.Background(), "AT", nil), ErrNotInApp)
	require.ErrorIs(t, m.DeletePasskey(context.Background(), "AT", "cred"), ErrNotInApp)

	// Blank issuer → empty account URL, no panic.
	assert.Empty(t, NewKeycloakCredentialManager("").AccountURL("x"))

	var _ CredentialManager = (*KeycloakCredentialManager)(nil)
}
