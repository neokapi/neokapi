package jobs

import (
	"context"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	aiprovider "github.com/neokapi/neokapi/providers/ai"
)

const cognitiveServicesScope = "https://cognitiveservices.azure.com/.default"

// PlatformProviderConfig holds configuration for the platform-provided Azure OpenAI.
type PlatformProviderConfig struct {
	Endpoint string // Azure OpenAI endpoint URL
	ClientID string // User-assigned managed identity client ID (optional)
}

// NewPlatformProvider creates an Azure OpenAI provider that authenticates
// using a managed identity. The deployment name selects which model to use
// (e.g. "gpt-4o", "gpt-4o-mini").
func NewPlatformProvider(cfg PlatformProviderConfig, deployment string) (aiprovider.LLMProvider, error) {
	var cred *azidentity.ManagedIdentityCredential
	var err error

	if cfg.ClientID != "" {
		cred, err = azidentity.NewManagedIdentityCredential(&azidentity.ManagedIdentityCredentialOptions{
			ID: azidentity.ClientID(cfg.ClientID),
		})
	} else {
		cred, err = azidentity.NewManagedIdentityCredential(nil)
	}
	if err != nil {
		return nil, fmt.Errorf("create managed identity credential: %w", err)
	}

	tp := func(ctx context.Context) (string, error) {
		token, err := cred.GetToken(ctx, policy.TokenRequestOptions{
			Scopes: []string{cognitiveServicesScope},
		})
		if err != nil {
			return "", fmt.Errorf("get cognitive services token: %w", err)
		}
		return token.Token, nil
	}

	if deployment == "" {
		deployment = "gpt-4o"
	}

	return aiprovider.NewAzureOpenAITokenProvider(cfg.Endpoint, deployment, tp), nil
}
