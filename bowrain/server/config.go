// Package server provides the REST and gRPC API server for Bowrain.
package server

// Config holds configuration for the REST API server.
//
// Auth behavior is determined by JWTSecret: when set, the server enables
// authentication, OIDC login, and workspace management. When empty (e.g.
// in tests), routes are registered without auth middleware.
type Config struct {
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

	// Keycloak Admin API — used to write through email changes initiated
	// from the Bowrain UI. Empty values disable Bowrain-managed email change
	// (the UI surfaces an error if the user attempts the flow).
	//
	// KeycloakAdminURL should point at an in-cluster URL (e.g.
	// "http://keycloak.identity.svc.cluster.local:8080") so admin traffic
	// stays inside the cluster and never traverses the public ingress.
	// The token endpoint and admin endpoints both resolve from this URL —
	// they are not the browser-facing OIDCPublicURL.
	KeycloakAdminURL          string
	KeycloakRealm             string // Realm name (default: "bowrain")
	KeycloakAdminClientID     string // Service-account client ID with realm-management:manage-users
	KeycloakAdminClientSecret string

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

	// Blob storage (Bowrain AD-007)
	BlobBackend            string // "azure", "local" (default: "local")
	AzureStorageAccountURL string // Azure Blob Storage account URL
	AzureStorageContainer  string // Azure Blob Storage container name (default: "bowrain-assets")
	AzureStorageConnStr    string // Azure connection string (dev/Azurite fallback)
	BlobStorageLocalDir    string // Local blob storage root directory

	// Sync protocol (Bowrain AD-009)
	MaxPushBytes int64 // Max total upload size per push (default: 256MB)

	// External services
	ServiceBusConnection string // Azure Service Bus connection string for job queue
	NATSURL              string // NATS server URL for job queue (e.g. nats://localhost:4222)
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

	// Billing (Bowrain AD-018)
	StripeSecretKey     string
	StripeWebhookSecret string
	StripeProPriceID    string
	StripeTeamPriceID   string
	StripeCreditPriceID string
	PostHogAPIKey       string
	PostHogHost         string

	// Admin control plane (Bowrain AD-018)
	AdminOIDCIssuerURL    string
	AdminOIDCClientID     string
	AdminOIDCClientSecret string

	// AllowInsecureAdminAuth opts the /api/admin/* control plane into the
	// plain user-JWT fallback when no admin OIDC verifier is configured.
	//
	// SECURITY: with this enabled and no AdminVerifier, ANY valid user JWT or
	// session is accepted by admin routes — there is no admin-role check. This
	// is intended only for local development. It is deliberately NOT the
	// default: in production the admin API must be gated by AdminGuard (an
	// admin-realm OIDC verifier), and absent that the routes are not mounted at
	// all so a missing/failed admin-OIDC config can never silently fall back to
	// accepting regular-user tokens (privilege escalation).
	AllowInsecureAdminAuth bool

	// Audit (Phase 2). AuditRetentionDays prunes audit_log rows older than the
	// given number of days (0 = keep forever). AuditSIEMWebhookURL forwards
	// every event as NDJSON to an external SIEM/log sink (empty = disabled).
	AuditRetentionDays  int
	AuditSIEMWebhookURL string
}

// ServerConfig is a deprecated alias for [Config].
//
// Deprecated: Use [Config] instead.
type ServerConfig = Config

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() Config {
	return Config{
		Port: 8080,
		Host: "0.0.0.0",
	}
}

// DefaultServerConfig is a deprecated alias for [DefaultConfig].
//
// Deprecated: Use [DefaultConfig] instead.
func DefaultServerConfig() Config {
	return DefaultConfig()
}
