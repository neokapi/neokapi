package auth

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cognitoidentityprovider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCognitoConfigFromIssuer(t *testing.T) {
	tests := []struct {
		name       string
		issuer     string
		wantPool   string
		wantRegion string
		wantErr    bool
	}{
		{
			name:       "valid cognito issuer",
			issuer:     "https://cognito-idp.eu-north-1.amazonaws.com/eu-north-1_Q1N15Ny0P",
			wantPool:   "eu-north-1_Q1N15Ny0P",
			wantRegion: "eu-north-1",
		},
		{
			name:       "trailing slash tolerated",
			issuer:     "https://cognito-idp.us-east-1.amazonaws.com/us-east-1_AbCd1234/",
			wantPool:   "us-east-1_AbCd1234",
			wantRegion: "us-east-1",
		},
		{
			name:    "keycloak issuer rejected",
			issuer:  "https://auth.bowrain.cloud/realms/bowrain",
			wantErr: true,
		},
		{
			name:    "cognito host without pool rejected",
			issuer:  "https://cognito-idp.eu-north-1.amazonaws.com/",
			wantErr: true,
		},
		{
			name:    "empty issuer rejected",
			issuer:  "",
			wantErr: true,
		},
		{
			name:    "lookalike host rejected",
			issuer:  "https://cognito-idp.eu-north-1.amazonaws.com.evil.example/pool",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := CognitoConfigFromIssuer(tt.issuer)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantPool, got.UserPoolID)
			assert.Equal(t, tt.wantRegion, got.Region)
			assert.NoError(t, got.Validate())
		})
	}
}

// fakeCognitoAPI records the last AdminUpdateUserAttributes call and returns a
// configurable error.
type fakeCognitoAPI struct {
	gotInput *cognitoidentityprovider.AdminUpdateUserAttributesInput
	err      error
	calls    int
}

func (f *fakeCognitoAPI) AdminUpdateUserAttributes(
	_ context.Context,
	in *cognitoidentityprovider.AdminUpdateUserAttributesInput,
	_ ...func(*cognitoidentityprovider.Options),
) (*cognitoidentityprovider.AdminUpdateUserAttributesOutput, error) {
	f.calls++
	f.gotInput = in
	if f.err != nil {
		return nil, f.err
	}
	return &cognitoidentityprovider.AdminUpdateUserAttributesOutput{}, nil
}

func TestCognitoAdminClient_UpdateUserEmail(t *testing.T) {
	t.Run("sends pool, sub username, and verified email attributes", func(t *testing.T) {
		fake := &fakeCognitoAPI{}
		c := &CognitoAdminClient{cfg: CognitoAdminConfig{UserPoolID: "eu-north-1_Q1N15Ny0P"}, api: fake}

		err := c.UpdateUserEmail(context.Background(), "sub-123", "new@example.com")
		require.NoError(t, err)
		require.Equal(t, 1, fake.calls)
		require.NotNil(t, fake.gotInput)
		assert.Equal(t, "eu-north-1_Q1N15Ny0P", aws.ToString(fake.gotInput.UserPoolId))
		assert.Equal(t, "sub-123", aws.ToString(fake.gotInput.Username))

		attrs := map[string]string{}
		for _, a := range fake.gotInput.UserAttributes {
			attrs[aws.ToString(a.Name)] = aws.ToString(a.Value)
		}
		assert.Equal(t, "new@example.com", attrs["email"])
		assert.Equal(t, "true", attrs["email_verified"])
	})

	t.Run("rejects empty subject", func(t *testing.T) {
		fake := &fakeCognitoAPI{}
		c := &CognitoAdminClient{cfg: CognitoAdminConfig{UserPoolID: "pool"}, api: fake}
		err := c.UpdateUserEmail(context.Background(), "", "new@example.com")
		require.Error(t, err)
		assert.Zero(t, fake.calls)
	})

	t.Run("rejects empty email", func(t *testing.T) {
		fake := &fakeCognitoAPI{}
		c := &CognitoAdminClient{cfg: CognitoAdminConfig{UserPoolID: "pool"}, api: fake}
		err := c.UpdateUserEmail(context.Background(), "sub-123", "")
		require.Error(t, err)
		assert.Zero(t, fake.calls)
	})

	t.Run("propagates API error", func(t *testing.T) {
		fake := &fakeCognitoAPI{err: errors.New("access denied")}
		c := &CognitoAdminClient{cfg: CognitoAdminConfig{UserPoolID: "pool"}, api: fake}
		err := c.UpdateUserEmail(context.Background(), "sub-123", "new@example.com")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "access denied")
	})
}
