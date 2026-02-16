// Package server provides the REST and gRPC API server for Bowrain.
// It is imported by both cmd/bowrain-server (multi-user deployment)
// and cmd/kapi serve (local single-project mode).
package server

// ServerConfig holds configuration for the REST API server.
type ServerConfig struct {
	// Port is the HTTP port to listen on.
	Port int

	// Host is the address to bind to (e.g., "0.0.0.0", "127.0.0.1").
	Host string

	// DataDir is the directory for temporary files during processing.
	DataDir string

	// StorePath is the path to the SQLite content store database.
	// If empty, project/block/connector APIs are disabled.
	StorePath string

	// Auth
	JWTSecret        string
	OIDCIssuerURL    string
	OIDCClientID     string
	OIDCClientSecret string
	OIDCPublicURL    string // browser-facing OIDC URL; defaults to OIDCIssuerURL

	// Email
	SMTPHost string // SMTP server host:port
	SMTPFrom string // sender email address

	// WebUIDir is the path to built web UI static files.
	// If set, the server serves static files for the web UI.
	WebUIDir string

	// LocalMode indicates the server is running in local single-project mode
	// (kapi serve) with no authentication required.
	LocalMode bool
}

// DefaultServerConfig returns a ServerConfig with sensible defaults.
func DefaultServerConfig() ServerConfig {
	return ServerConfig{
		Port: 8080,
		Host: "0.0.0.0",
	}
}

// LocalServerConfig returns a ServerConfig for local single-project mode.
func LocalServerConfig() ServerConfig {
	return ServerConfig{
		Port:      3000,
		Host:      "127.0.0.1",
		LocalMode: true,
	}
}
