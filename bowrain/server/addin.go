package server

import (
	"context"
	"os"
	"strings"

	"github.com/labstack/echo/v4"

	"github.com/neokapi/neokapi/bowrain/addin"
	aitools "github.com/neokapi/neokapi/core/ai/tools"
	aiprovider "github.com/neokapi/neokapi/providers/ai"
)

// registerAddinRoutes mounts the workspace add-in surface under /api/v1/addin:
//
//   - The Google Workspace add-on Card-JSON endpoints (/addin/google/*) are
//     PUBLIC — Google's add-on runtime POSTs to them, not the user's browser.
//     In production they should verify the inbound Google system ID token; that
//     verification hook is documented on the deployment manifest.
//   - The REST API (/addin/check, /addin/terms, /addin/translate) is what the
//     Microsoft 365 Office task pane (and any browser client) calls, so it sits
//     behind the standard bearer-token auth middleware.
//
// Both share one [addin.Service]; translation uses the configured platform AI
// provider (BOWRAIN_PLATFORM_PROVIDER), falling back to the keyless demo
// provider so the surface works without extra configuration.
func (s *Server) registerAddinRoutes(v1 *echo.Group) {
	svc := addin.New()
	svc.PublicURL = s.addinPublicURL()
	svc.NewProvider = addinPlatformProvider

	// Public: Google add-on runtime callbacks.
	svc.RegisterGoogleRoutes(v1.Group("/addin"))

	// Authenticated: Office task pane / browser REST API.
	if s.Config.JWTSecret != "" {
		svc.RegisterRoutes(v1.Group("/addin", AuthMiddleware(s.Config.JWTSecret, s.AuthStore)))
	} else {
		svc.RegisterRoutes(v1.Group("/addin"))
	}
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
