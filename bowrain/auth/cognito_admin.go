package auth

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/cognitoidentityprovider"
	ciptypes "github.com/aws/aws-sdk-go-v2/service/cognitoidentityprovider/types"
)

// CognitoAdminConfig holds the parameters for administrative writes to an AWS
// Cognito user pool. The role the server runs under (the ECS task role in
// hosted prod) must be allowed cognito-idp:AdminUpdateUserAttributes on the
// pool — there are no static secrets, unlike the Keycloak service-account
// client.
type CognitoAdminConfig struct {
	// UserPoolID is the Cognito user pool ID, e.g. "eu-north-1_XXXXXXXXX".
	UserPoolID string
	// Region is the AWS region the pool lives in. Empty falls back to the
	// ambient AWS config's region (AWS_REGION / the task's region).
	Region string
}

// Validate returns nil if the config is usable.
func (c CognitoAdminConfig) Validate() error {
	if strings.TrimSpace(c.UserPoolID) == "" {
		return errors.New("cognito user pool ID is required")
	}
	return nil
}

// CognitoConfigFromIssuer derives the admin config (user pool ID + region) from
// a Cognito OIDC issuer URL of the form
// "https://cognito-idp.<region>.amazonaws.com/<userPoolId>" — the value the
// server is already configured with as OIDCIssuerURL under Cognito. It errors
// if the URL is not a recognizable Cognito issuer, so a misconfiguration
// disables the admin write-through loudly rather than pointing it at the wrong
// pool.
func CognitoConfigFromIssuer(issuerURL string) (CognitoAdminConfig, error) {
	u, err := url.Parse(strings.TrimSpace(issuerURL))
	if err != nil {
		return CognitoAdminConfig{}, fmt.Errorf("parse cognito issuer: %w", err)
	}
	host := u.Hostname()
	if !strings.HasPrefix(host, "cognito-idp.") || !strings.HasSuffix(host, ".amazonaws.com") {
		return CognitoAdminConfig{}, fmt.Errorf("not a cognito issuer host: %q", host)
	}
	region := strings.TrimSuffix(strings.TrimPrefix(host, "cognito-idp."), ".amazonaws.com")
	poolID := strings.Trim(u.Path, "/")
	if region == "" || poolID == "" || strings.ContainsRune(poolID, '/') {
		return CognitoAdminConfig{}, fmt.Errorf("cannot derive pool/region from cognito issuer %q", issuerURL)
	}
	return CognitoAdminConfig{UserPoolID: poolID, Region: region}, nil
}

// cognitoAdminAPI is the slice of the Cognito Identity Provider client the
// adapter uses. Narrowing it to an interface keeps the adapter unit-testable
// without reaching AWS.
type cognitoAdminAPI interface {
	AdminUpdateUserAttributes(
		ctx context.Context,
		in *cognitoidentityprovider.AdminUpdateUserAttributesInput,
		optFns ...func(*cognitoidentityprovider.Options),
	) (*cognitoidentityprovider.AdminUpdateUserAttributesOutput, error)
}

// CognitoAdminClient implements IdentityAdmin against an AWS Cognito user pool.
// It authenticates via the ambient AWS credential chain (the ECS task role in
// hosted prod), so it carries no long-lived secrets. Safe for concurrent use —
// the underlying SDK client is.
type CognitoAdminClient struct {
	cfg CognitoAdminConfig
	api cognitoAdminAPI
}

// NewCognitoAdminClient builds a client using the ambient AWS config (the
// default credential chain). It does not verify connectivity; the first call
// surfaces errors.
func NewCognitoAdminClient(ctx context.Context, cfg CognitoAdminConfig) (*CognitoAdminClient, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	var opts []func(*awsconfig.LoadOptions) error
	if r := strings.TrimSpace(cfg.Region); r != "" {
		opts = append(opts, awsconfig.WithRegion(r))
	}
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("load aws config for cognito admin: %w", err)
	}
	return &CognitoAdminClient{
		cfg: cfg,
		api: cognitoidentityprovider.NewFromConfig(awsCfg),
	}, nil
}

// UpdateUserEmail sets a new email and marks it verified on the Cognito user
// identified by oidcSub. Cognito's admin APIs accept the immutable `sub` value
// as the Username, which is exactly the OIDC `sub` claim Bowrain stored. Bowrain
// only calls this after it has itself verified ownership of newEmail.
func (c *CognitoAdminClient) UpdateUserEmail(ctx context.Context, oidcSub, newEmail string) error {
	if oidcSub == "" {
		return errors.New("oidc subject is required")
	}
	if newEmail == "" {
		return errors.New("new email is required")
	}
	_, err := c.api.AdminUpdateUserAttributes(ctx, &cognitoidentityprovider.AdminUpdateUserAttributesInput{
		UserPoolId: aws.String(c.cfg.UserPoolID),
		Username:   aws.String(oidcSub),
		UserAttributes: []ciptypes.AttributeType{
			{Name: aws.String("email"), Value: aws.String(newEmail)},
			// email_verified is a boolean attribute encoded as the string
			// "true"/"false" in Cognito's attribute API.
			{Name: aws.String("email_verified"), Value: aws.String("true")},
		},
	})
	if err != nil {
		return fmt.Errorf("cognito admin update user attributes: %w", err)
	}
	return nil
}

// Compile-time assertion that the Cognito admin client satisfies the port —
// the mirror of the Keycloak assertion in identity_admin.go.
var _ IdentityAdmin = (*CognitoAdminClient)(nil)
