// Package server provides the REST and gRPC API server for Bowrain.
package server

// ServerConfig holds configuration for the REST API server.
//
// Auth behavior is determined by JWTSecret: when set, the server enables
// authentication, OIDC login, and workspace management. When empty (e.g.
// in tests), routes are registered without auth middleware.
type ServerConfig struct {
	// Port is the HTTP port to listen on.
	Port int

	// Host is the address to bind to (e.g., "0.0.0.0", "127.0.0.1").
	Host string

	// DataDir is the directory for temporary files during processing.
	DataDir string

	// DatabaseURL is a PostgreSQL connection string. Supported schemes:
	//   - postgres://user:pass@host/db  → PostgreSQL via pgx
	//   - postgresql://user@host/db     → PostgreSQL via pgx
	// Required for production deployments.
	DatabaseURL string

	// DatabaseAuth selects the PostgreSQL authentication method.
	// "azure" uses Entra ID managed identity tokens (passwordless).
	// Empty or any other value uses password from the DatabaseURL.
	DatabaseAuth string

	// AzureClientID is the client ID of the user-assigned managed identity
	// for Azure Entra ID database authentication. Only used when
	// DatabaseAuth is "azure".
	AzureClientID string

	// Auth
	JWTSecret        string
	OIDCIssuerURL    string
	OIDCClientID     string
	OIDCClientSecret string
	OIDCPublicURL    string // browser-facing OIDC URL; defaults to OIDCIssuerURL

	// Email — SMTP sender.
	// Set SMTPHost + SMTPFrom for unauthenticated relay (local dev / Mailpit).
	// Set SMTPUsername + SMTPPassword for authenticated SMTP (production).
	// Set SMTPUseTLS=true for implicit TLS (port 465 / SMTPS).
	SMTPHost     string // SMTP server host:port  (e.g. "smtp.example.com:587")
	SMTPFrom     string // Sender address         (e.g. "noreply@bowrain.cloud")
	SMTPUsername string // SMTP auth username     (empty = no auth)
	SMTPPassword string // SMTP auth password
	SMTPUseTLS   bool   // Use implicit TLS (SMTPS); false = try STARTTLS

	// Email — Resend API sender (alternative to SMTP for SaaS deployments).
	// When ResendAPIKey is set, Resend is used instead of SMTP.
	// SMTPFrom is reused as the "from" address.
	ResendAPIKey string // Resend API key (sk_live_…)

	// WebUIDir is the path to built web UI static files (development only).
	// In production, the web UI is served by a separate container (bowrain-web).
	WebUIDir string

	// PulseUIDir is the path to built Pulse dashboard static files (development only).
	// When set, requests to the pulse subdomain are served from this directory.
	PulseUIDir string

	// Blob storage (AD-029)
	BlobBackend            string // "azure", "local" (default: "local")
	AzureStorageAccountURL string // Azure Blob Storage account URL
	AzureStorageContainer  string // Azure Blob Storage container name (default: "bowrain-assets")
	AzureStorageConnStr    string // Azure connection string (dev/Azurite fallback)
	BlobStorageLocalDir    string // Local blob storage root directory

	// External services
	ServiceBusConnection string // Azure Service Bus connection string for job queue
	NATSUrl              string // NATS server URL for job queue (e.g. nats://localhost:4222)
	RedisURL             string // Redis connection string for caching and session state
	RedisPassword        string // Redis password (overrides any password in RedisURL)

	// Agent (@bravo) — container runtime for ZeroClaw.
	// AgentRuntime selects the container backend: "docker" or "aca" (Azure Container Apps).
	// When empty, the agent falls back to local mock responses.
	AgentRuntime string

	// AgentImage is the ZeroClaw container image for @bravo.
	AgentImage string // default: "ghcr.io/neokapi/bravo-agent:latest"

	// AgentMaxConcurrent is the max concurrent agent containers per workspace.
	AgentMaxConcurrent int // default: 3

	// Docker runtime settings (when AgentRuntime == "docker").
	AgentDockerHost    string // default: "unix:///var/run/docker.sock"
	AgentDockerNetwork string // Docker network for agent containers (optional)

	// Azure Container Apps settings (when AgentRuntime == "aca").
	AgentACASubscription  string // Azure subscription ID
	AgentACAResourceGroup string // Azure resource group
	AgentACAEnvironmentID string // Container App Environment resource ID
	AgentACALocation      string // Azure region (e.g. "westus2")

	// Agent model configuration — injected into ZeroClaw containers.
	AgentModelProvider string // e.g. "azure-openai", "anthropic"
	AgentModelName     string // e.g. "gpt-4o", "claude-sonnet-4-20250514"
	AgentModelAPIBase  string // provider API base URL
	AgentModelAPIKey   string // provider API key

	// Billing (AD-030)
	StripeSecretKey     string
	StripeWebhookSecret string
	StripeProPriceID    string
	StripeTeamPriceID   string
	StripeCreditPriceID string
	PostHogAPIKey       string
	PostHogHost         string

	// Admin control plane (AD-030)
	AdminOIDCIssuerURL    string
	AdminOIDCClientID     string
	AdminOIDCClientSecret string
}

// DefaultServerConfig returns a ServerConfig with sensible defaults.
func DefaultServerConfig() ServerConfig {
	return ServerConfig{
		Port: 8080,
		Host: "0.0.0.0",
	}
}
