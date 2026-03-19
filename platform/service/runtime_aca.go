package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
)

// ACARuntime implements ContainerRuntime using Azure Container Apps dynamic sessions.
// Each agent is a revision of a Container App running ZeroClaw.
//
// Architecture:
//   - A Container App Environment is pre-provisioned (shared infra).
//   - Each agent conversation spawns a new Container App (or revision).
//   - The Container App runs the ZeroClaw image with injected env vars.
//   - Ingress is configured for the gateway port, producing an FQDN.
//   - On stop, the Container App is deleted.
type ACARuntime struct {
	client         *http.Client
	credential     azcore.TokenCredential
	subscriptionID string
	resourceGroup  string
	environmentID  string // full resource ID of the Container App Environment
	location       string // Azure region (e.g. "westus2")
}

// ACAConfig configures the Azure Container Apps runtime.
type ACAConfig struct {
	// Credential is an Azure token credential (e.g. DefaultAzureCredential).
	Credential azcore.TokenCredential

	// SubscriptionID is the Azure subscription.
	SubscriptionID string

	// ResourceGroup is the Azure resource group for agent Container Apps.
	ResourceGroup string

	// EnvironmentID is the full resource ID of the Container App Environment.
	// e.g. "/subscriptions/.../resourceGroups/.../providers/Microsoft.App/managedEnvironments/bravo-env"
	EnvironmentID string

	// Location is the Azure region (e.g. "westus2").
	Location string
}

// NewACARuntime creates an Azure Container Apps runtime.
func NewACARuntime(cfg ACAConfig) *ACARuntime {
	return &ACARuntime{
		client:         &http.Client{Timeout: 2 * time.Minute},
		credential:     cfg.Credential,
		subscriptionID: cfg.SubscriptionID,
		resourceGroup:  cfg.ResourceGroup,
		environmentID:  cfg.EnvironmentID,
		location:       cfg.Location,
	}
}

// Spawn creates a new Container App for the agent conversation.
func (r *ACARuntime) Spawn(ctx context.Context, cfg ContainerConfig) (*AgentContainer, error) {
	gatewayPort := cfg.GatewayPort
	if gatewayPort == 0 {
		gatewayPort = 42617
	}

	// Container App name: must be lowercase alphanumeric + hyphens, max 32 chars.
	appName := sanitizeAppName("bravo-" + cfg.ConversationID)

	envVars := []acaEnvVar{
		{Name: "BRAVO_MODEL_PROVIDER", Value: cfg.ModelProvider},
		{Name: "BRAVO_MODEL_NAME", Value: cfg.ModelName},
		{Name: "BRAVO_MODEL_API_BASE", Value: cfg.ModelAPIBase},
		{Name: "BRAVO_MODEL_API_KEY", SecretRef: "model-api-key"},
		{Name: "BRAVO_MCP_ENDPOINT", Value: cfg.MCPEndpoint},
		{Name: "BRAVO_AGENT_TOKEN", SecretRef: "agent-token"},
	}
	if cfg.SystemPrompt != "" {
		envVars = append(envVars, acaEnvVar{Name: "BRAVO_SYSTEM_PROMPT", Value: cfg.SystemPrompt})
	}
	for k, v := range cfg.Env {
		envVars = append(envVars, acaEnvVar{Name: k, Value: v})
	}

	secrets := []acaSecret{
		{Name: "model-api-key", Value: cfg.ModelAPIKey},
		{Name: "agent-token", Value: cfg.AgentToken},
	}
	var registries []acaRegistry
	if cfg.RegistryServer != "" && cfg.RegistryPassword != "" {
		secrets = append(secrets, acaSecret{Name: "registry-password", Value: cfg.RegistryPassword})
		registries = append(registries, acaRegistry{
			Server:            cfg.RegistryServer,
			Username:          cfg.RegistryUsername,
			PasswordSecretRef: "registry-password",
		})
	}

	app := acaContainerApp{
		Location: r.location,
		Properties: acaProperties{
			EnvironmentID: r.environmentID,
			Configuration: acaConfiguration{
				Ingress: &acaIngress{
					TargetPort: gatewayPort,
					External:   false,
					Transport:  "auto",
				},
				Secrets:    secrets,
				Registries: registries,
			},
			Template: acaTemplate{
				Containers: []acaContainer{
					{
						Name:  "bravo",
						Image: cfg.Image,
						Resources: acaResources{
							CPU:    0.25,
							Memory: "0.5Gi",
						},
						Env: envVars,
					},
				},
				Scale: acaScale{
					MinReplicas: 1,
					MaxReplicas: 1,
				},
			},
		},
		Tags: map[string]string{
			"neokapi.agent":        "bravo",
			"neokapi.conversation": cfg.ConversationID,
			"neokapi.workspace":    cfg.WorkspaceID,
			"neokapi.user":         cfg.UserID,
		},
	}

	body, err := json.Marshal(app)
	if err != nil {
		return nil, fmt.Errorf("marshal container app: %w", err)
	}

	url := fmt.Sprintf(
		"https://management.azure.com/subscriptions/%s/resourceGroups/%s/providers/Microsoft.App/containerApps/%s?api-version=2024-03-01",
		r.subscriptionID, r.resourceGroup, appName,
	)

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	if err := r.authorize(ctx, req); err != nil {
		return nil, fmt.Errorf("authorize: %w", err)
	}

	resp, err := r.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("aca create: %w", err)
	}
	defer resp.Body.Close()

	// 200 = updated, 201 = created. Both are OK.
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return nil, fmt.Errorf("aca create returned %d: %s", resp.StatusCode, string(respBody))
	}

	// Parse response to get the FQDN.
	var createResp acaCreateResponse
	if err := json.NewDecoder(resp.Body).Decode(&createResp); err != nil {
		return nil, fmt.Errorf("parse aca response: %w", err)
	}

	// The FQDN may take a moment to be available. Poll if needed.
	fqdn := createResp.Properties.Configuration.Ingress.FQDN
	if fqdn == "" {
		// Poll for FQDN availability (Container App provisioning is async).
		fqdn, err = r.pollFQDN(ctx, appName)
		if err != nil {
			_ = r.Stop(ctx, appName)
			return nil, fmt.Errorf("wait for FQDN: %w", err)
		}
	}

	gatewayURL := "https://" + fqdn

	return &AgentContainer{
		ID:             appName,
		GatewayURL:     gatewayURL,
		ConversationID: cfg.ConversationID,
		WorkspaceID:    cfg.WorkspaceID,
		UserID:         cfg.UserID,
		CreatedAt:      time.Now(),
	}, nil
}

// Stop deletes the Container App.
func (r *ACARuntime) Stop(ctx context.Context, containerID string) error {
	url := fmt.Sprintf(
		"https://management.azure.com/subscriptions/%s/resourceGroups/%s/providers/Microsoft.App/containerApps/%s?api-version=2024-03-01",
		r.subscriptionID, r.resourceGroup, containerID,
	)

	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, url, nil)
	if err != nil {
		return fmt.Errorf("create delete request: %w", err)
	}
	if err := r.authorize(ctx, req); err != nil {
		return fmt.Errorf("authorize: %w", err)
	}

	resp, err := r.client.Do(req)
	if err != nil {
		return fmt.Errorf("aca delete: %w", err)
	}
	resp.Body.Close()

	// 200, 202 (accepted), 204 (no content) are all success.
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	// 404 means already deleted — not an error.
	if resp.StatusCode == http.StatusNotFound {
		return nil
	}
	return fmt.Errorf("aca delete returned %d", resp.StatusCode)
}

// Health checks if the Container App is running and has a provisioned FQDN.
func (r *ACARuntime) Health(ctx context.Context, containerID string) (bool, error) {
	url := fmt.Sprintf(
		"https://management.azure.com/subscriptions/%s/resourceGroups/%s/providers/Microsoft.App/containerApps/%s?api-version=2024-03-01",
		r.subscriptionID, r.resourceGroup, containerID,
	)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return false, err
	}
	if err := r.authorize(ctx, req); err != nil {
		return false, err
	}

	resp, err := r.client.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return false, nil
	}
	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("aca get returned %d", resp.StatusCode)
	}

	var getResp acaCreateResponse
	if err := json.NewDecoder(resp.Body).Decode(&getResp); err != nil {
		return false, err
	}

	running := getResp.Properties.ProvisioningState == "Succeeded" &&
		getResp.Properties.Configuration.Ingress.FQDN != ""
	return running, nil
}

// authorize adds the Azure Bearer token to the request.
func (r *ACARuntime) authorize(ctx context.Context, req *http.Request) error {
	token, err := r.credential.GetToken(ctx, policy.TokenRequestOptions{
		Scopes: []string{"https://management.azure.com/.default"},
	})
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token.Token)
	return nil
}

// pollFQDN polls the Container App until the FQDN is available.
func (r *ACARuntime) pollFQDN(ctx context.Context, appName string) (string, error) {
	url := fmt.Sprintf(
		"https://management.azure.com/subscriptions/%s/resourceGroups/%s/providers/Microsoft.App/containerApps/%s?api-version=2024-03-01",
		r.subscriptionID, r.resourceGroup, appName,
	)

	deadline := time.Now().Add(90 * time.Second)
	for time.Now().Before(deadline) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return "", err
		}
		if err := r.authorize(ctx, req); err != nil {
			return "", err
		}

		resp, err := r.client.Do(req)
		if err != nil {
			return "", err
		}

		var getResp acaCreateResponse
		_ = json.NewDecoder(resp.Body).Decode(&getResp)
		resp.Body.Close()

		if fqdn := getResp.Properties.Configuration.Ingress.FQDN; fqdn != "" {
			return fqdn, nil
		}

		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}

	return "", fmt.Errorf("timeout waiting for Container App FQDN")
}

// sanitizeAppName ensures the name is valid for Azure Container Apps:
// lowercase, alphanumeric + hyphens, max 32 chars, no leading/trailing hyphens.
func sanitizeAppName(name string) string {
	var out []byte
	for _, c := range []byte(name) {
		switch {
		case c >= 'a' && c <= 'z', c >= '0' && c <= '9':
			out = append(out, c)
		case c >= 'A' && c <= 'Z':
			out = append(out, c+32) // lowercase
		case c == '-' || c == '_':
			out = append(out, '-')
		}
	}
	// Trim leading/trailing hyphens.
	s := string(out)
	for len(s) > 0 && s[0] == '-' {
		s = s[1:]
	}
	for len(s) > 0 && s[len(s)-1] == '-' {
		s = s[:len(s)-1]
	}
	if len(s) > 32 {
		s = s[:32]
	}
	return s
}

// ---------------------------------------------------------------------------
// Azure Container Apps API types (minimal subset)
// ---------------------------------------------------------------------------

type acaContainerApp struct {
	Location   string            `json:"location"`
	Properties acaProperties     `json:"properties"`
	Tags       map[string]string `json:"tags,omitempty"`
}

type acaProperties struct {
	EnvironmentID     string           `json:"managedEnvironmentId"`
	Configuration     acaConfiguration `json:"configuration"`
	Template          acaTemplate      `json:"template"`
	ProvisioningState string           `json:"provisioningState,omitempty"`
}

type acaConfiguration struct {
	Ingress    *acaIngress   `json:"ingress,omitempty"`
	Secrets    []acaSecret   `json:"secrets,omitempty"`
	Registries []acaRegistry `json:"registries,omitempty"`
}

type acaRegistry struct {
	Server            string `json:"server"`
	Username          string `json:"username"`
	PasswordSecretRef string `json:"passwordSecretRef"`
}

type acaIngress struct {
	TargetPort int    `json:"targetPort"`
	External   bool   `json:"external"`
	Transport  string `json:"transport"`
	FQDN       string `json:"fqdn,omitempty"`
}

type acaSecret struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type acaTemplate struct {
	Containers []acaContainer `json:"containers"`
	Scale      acaScale       `json:"scale"`
}

type acaContainer struct {
	Name      string       `json:"name"`
	Image     string       `json:"image"`
	Resources acaResources `json:"resources"`
	Env       []acaEnvVar  `json:"env,omitempty"`
}

type acaResources struct {
	CPU    float64 `json:"cpu"`
	Memory string  `json:"memory"`
}

type acaEnvVar struct {
	Name      string `json:"name"`
	Value     string `json:"value,omitempty"`
	SecretRef string `json:"secretRef,omitempty"`
}

type acaScale struct {
	MinReplicas int `json:"minReplicas"`
	MaxReplicas int `json:"maxReplicas"`
}

type acaCreateResponse struct {
	Properties struct {
		ProvisioningState string           `json:"provisioningState"`
		Configuration     acaConfiguration `json:"configuration"`
	} `json:"properties"`
}

// Ensure ACARuntime implements the interface at compile time.
var _ ContainerRuntime = (*ACARuntime)(nil)

// Ensure DockerRuntime implements the interface at compile time.
// (Declared here alongside the ACA check for visibility.)
var _ ContainerRuntime = (*DockerRuntime)(nil)

