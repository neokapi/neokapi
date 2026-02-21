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

	// gRPC TLS
	GRPCTLSCertFile string // path to TLS certificate PEM file for gRPC server
	GRPCTLSKeyFile  string // path to TLS private key PEM file for gRPC server

	// WebUIDir is the path to built web UI static files.
	// If set, the server serves static files for the web UI.
	WebUIDir string
}

// DefaultServerConfig returns a ServerConfig with sensible defaults.
func DefaultServerConfig() ServerConfig {
	return ServerConfig{
		Port: 8080,
		Host: "0.0.0.0",
	}
}
