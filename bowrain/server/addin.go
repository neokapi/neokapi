package server

import (
	"context"
	"log/slog"
	"os"
	"strings"

	"github.com/labstack/echo/v4"

	"github.com/neokapi/neokapi/bowrain/addin"
	aitools "github.com/neokapi/neokapi/core/ai/tools"
	aiprovider "github.com/neokapi/neokapi/providers/ai"
)

// registerAddinRoutes mounts the workspace add-in surface under /api/v1/addin.
// Both halves are fail-closed: an endpoint is only exposed once it can be
// authenticated.
//
//   - The REST API (/addin/check, /addin/terms, /addin/translate) is what the
//     Microsoft 365 Office task pane (and any browser client) calls. It is only
//     mounted when a JWT secret is configured, behind the standard bearer-token
//     auth middleware — never exposed unauthenticated.
//   - The Google Workspace add-on Card-JSON endpoints (/addin/google/*) are
//     called by Google's add-on runtime (not the user's browser), so they
//     cannot use bearer auth. They are only mounted when
//     BOWRAIN_GOOGLE_ADDON_AUDIENCE is configured, behind middleware that
//     verifies the inbound Google system ID token (signature against Google's
//     JWKS + issuer + audience + optional service-account email). Without that
//     config the endpoints stay disabled rather than open.
//
// Both share one [addin.Service]; translation uses the configured platform AI
// provider (BOWRAIN_PLATFORM_PROVIDER), falling back to the keyless demo
// provider so the surface works without extra configuration.
func (s *Server) registerAddinRoutes(v1 *echo.Group) {
	svc := addin.New()
	svc.PublicURL = s.addinPublicURL()
	svc.NewProvider = addinPlatformProvider

	// Authenticated REST API for the Office task pane / browser clients.
	if s.Config.JWTSecret != "" {
		svc.RegisterRoutes(v1.Group("/addin", AuthMiddleware(s.Config.JWTSecret, s.AuthStore)))
	} else {
		slog.Warn("addin: JWT secret not configured — Office add-in REST API disabled")
	}

	// Google add-on runtime callbacks, gated on Google system-ID-token
	// verification. Disabled (not open) when no audience is configured.
	if audiences := splitEnvList("BOWRAIN_GOOGLE_ADDON_AUDIENCE"); len(audiences) > 0 {
		saEmails := splitEnvList("BOWRAIN_GOOGLE_ADDON_SA_EMAIL")
		svc.RegisterGoogleRoutes(v1.Group("/addin", verifyGoogleAddonRequest(audiences, saEmails)))
	} else {
		slog.Warn("addin: BOWRAIN_GOOGLE_ADDON_AUDIENCE not set — Google Workspace add-on endpoints disabled")
	}
}

// splitEnvList reads a comma-separated environment variable into a trimmed,
// non-empty slice.
func splitEnvList(key string) []string {
	raw := strings.Split(os.Getenv(key), ",")
	out := make([]string, 0, len(raw))
	for _, v := range raw {
		if v = strings.TrimSpace(v); v != "" {
			out = append(out, v)
		}
	}
	return out
}

// addinPublicURL is the externally reachable base URL the Google add-on uses to
// build its button-callback URLs. BOWRAIN_ADDIN_PUBLIC_URL overrides; otherwise
// the browser-facing OIDC public URL is a sensible default.
func (s *Server) addinPublicURL() string {
	if u := strings.TrimRight(os.Getenv("BOWRAIN_ADDIN_PUBLIC_URL"), "/"); u != "" {
		return u
	}
	return strings.TrimRight(s.Config.OIDCPublicURL, "/")
}

// addinPlatformProvider builds the LLM provider used for add-in translation
// from the platform environment, mirroring the worker's selection. With no
// provider configured it returns the deterministic, keyless demo provider.
func addinPlatformProvider(context.Context) (aiprovider.LLMProvider, error) {
	name := os.Getenv("BOWRAIN_PLATFORM_PROVIDER")
	if name == "" {
		name = string(aiprovider.Demo)
	}
	return aitools.ProviderFromConfig(name, aiprovider.Config{
		APIKey: os.Getenv("BOWRAIN_PLATFORM_API_KEY"),
		Model:  os.Getenv("BOWRAIN_PLATFORM_MODEL"),
	})
}
