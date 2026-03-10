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

	// StorePath is the path to the SQLite content store database.
	// If empty and DatabaseURL is also empty, project/block/connector APIs are disabled.
	// Deprecated: prefer DatabaseURL for new deployments.
	StorePath string

	// DatabaseURL is a database connection string. Supported schemes:
	//   - postgres://user:pass@host/db  → PostgreSQL via pgx
	//   - postgresql://user@host/db     → PostgreSQL via pgx
	//   - sqlite:///path/to/file.db     → SQLite (same as StorePath)
	// When set, takes precedence over StorePath.
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

	// Email
	SMTPHost string // SMTP server host:port
	SMTPFrom string // sender email address

	// WebUIDir is the path to built web UI static files (development only).
	// In production, the web UI is served by a separate container (bowrain-web).
	WebUIDir string

	// External services
	ServiceBusConnection string // Azure Service Bus connection string for job queue
	NATSUrl              string // NATS server URL for job queue (e.g. nats://localhost:4222)
	RedisURL      string // Redis connection string for caching and session state
	RedisPassword string // Redis password (overrides any password in RedisURL)
}

// DefaultServerConfig returns a ServerConfig with sensible defaults.
func DefaultServerConfig() ServerConfig {
	return ServerConfig{
		Port: 8080,
		Host: "0.0.0.0",
	}
}
