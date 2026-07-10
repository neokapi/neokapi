// Package boot validates critical process configuration before the bowrain
// binaries start serving. The contract is fail-fast: a missing or typo'd
// env var must abort startup with a clear message rather than boot a
// degraded process (a server without auth routes, a worker without stores).
package boot

import (
	"errors"
	"fmt"
	"os"
	"strings"
)

// InsecureDevEnv is the environment variable that opts bowrain-server out of
// strict startup validation for local development (equivalent to the
// --allow-insecure-dev flag).
const InsecureDevEnv = "BOWRAIN_ALLOW_INSECURE_DEV"

// AllowInsecureDevFromEnv reports whether the dev escape hatch is enabled via
// the BOWRAIN_ALLOW_INSECURE_DEV environment variable.
func AllowInsecureDevFromEnv() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(InsecureDevEnv))) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}

// ValidatePostgresURL checks that url is a non-empty postgres:// or
// postgresql:// connection string. name identifies the setting in the error
// message (e.g. "BOWRAIN_DATABASE_URL").
func ValidatePostgresURL(name, url string) error {
	if url == "" {
		return fmt.Errorf("%s is required (postgres:// connection string)", name)
	}
	if !strings.HasPrefix(url, "postgres://") && !strings.HasPrefix(url, "postgresql://") {
		return fmt.Errorf("%s must start with postgres:// or postgresql://", name)
	}
	return nil
}

// MissingServerConfig returns one error per missing critical bowrain-server
// setting. An empty JWT secret silently disables the ENTIRE authenticated
// route tree; an empty database URL boots the server with nil stores. Both
// are fatal misconfigurations in production, so callers must abort unless
// the insecure-dev escape hatch is explicitly set.
//
// A non-empty but malformed database URL is not reported here — that is
// always fatal (see ValidatePostgresURL) and never excusable as dev mode.
func MissingServerConfig(jwtSecret, databaseURL string) []error {
	var missing []error
	if jwtSecret == "" {
		missing = append(missing, errors.New(
			"BOWRAIN_JWT_SECRET (or -jwt-secret) is required: without it the server boots with NO authenticated API routes"))
	}
	if databaseURL == "" {
		missing = append(missing, errors.New(
			"BOWRAIN_DATABASE_URL (or -database-url) is required: without it the server boots with no persistent stores"))
	}
	return missing
}
