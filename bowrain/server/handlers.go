package server

import (
	"cmp"
	"context"
	"net/http"
	"slices"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/neokapi/neokapi/core/locale"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/core/version"
	aiprovider "github.com/neokapi/neokapi/providers/ai"
)

// HealthResponse is the response for the health check endpoint.
type HealthResponse struct {
	Status    string `json:"status"`
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	BuildDate string `json:"build_date"`
}

// InfoResponse returns server info including mode, build metadata, and reference data.
type InfoResponse struct {
	Mode      string `json:"mode"` // "standalone" or "server"
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	BuildDate string `json:"build_date"`
	// Features reports deployment-level capabilities the web app gates UI on
	// (distinct from the per-workspace plan entitlements in the workspace
	// response): "brand_scan" is true only when the brand-scan job system
	// (PostgreSQL store + queue) is configured, so SQLite/standalone servers
	// hide the hosted-scan entry points instead of surfacing a 503.
	Features       map[string]bool     `json:"features"`
	Formats        []FormatInfo        `json:"formats"`
	Tools          []registry.ToolInfo `json:"tools"`
	Locales        []locale.LocaleInfo `json:"locales"`
	ConnectorTypes []ConnectorTypeInfo `json:"connector_types,omitempty"`
	// ProviderTypes lists the AI providers a workspace admin can configure with
	// credentials, sourced from the framework provider registry so the settings
	// UI never hardcodes (and drifts from) the Go provider constants.
	ProviderTypes []ProviderTypeInfo `json:"provider_types,omitempty"`
}

// ConnectorTypeInfo describes an available connector type.
type ConnectorTypeInfo struct {
	Name     string `json:"name"`
	Category string `json:"category"`
}

// ProviderTypeInfo describes a configurable AI provider.
type ProviderTypeInfo struct {
	Name     string `json:"name"`      // registry id, e.g. "anthropic"
	Label    string `json:"label"`     // human-readable, e.g. "Anthropic"
	NeedsKey bool   `json:"needs_key"` // false for local/keyless providers (Ollama)
}

// FormatInfo describes a registered format.
type FormatInfo struct {
	Name      string `json:"name"`
	HasReader bool   `json:"has_reader"`
	HasWriter bool   `json:"has_writer"`
}

// ErrorResponse is a standard error response.
//
// Reference is the per-request correlation ID (the same value echoed in the
// X-Request-ID response header and stamped on every server log line for this
// request). Clients surface it to the user as an error "reference" so a single
// ID drills all the way down to logs and the Sentry issue.
//
// Message is the additive human-readable sentence for the error; Error keeps
// its exact historical value (stable code or legacy string) since clients and
// tests parse it. The full envelope contract lives in bowrain/apierror.
type ErrorResponse struct {
	Error     string `json:"error"`
	Message   string `json:"message,omitempty"`
	Details   string `json:"details,omitempty"`
	Code      string `json:"code,omitempty"`
	Reference string `json:"reference,omitempty"`
}

// HandleHealth returns a simple health check.
func (s *Server) HandleHealth(c echo.Context) error {
	return c.JSON(http.StatusOK, HealthResponse{
		Status:    "ok",
		Version:   version.Version,
		Commit:    version.Commit,
		BuildDate: version.BuildDate,
	})
}

// ReadinessResponse is the response for the readiness endpoint.
type ReadinessResponse struct {
	Status     string                     `json:"status"` // "ready", "degraded", "unhealthy"
	Version    string                     `json:"version"`
	Components map[string]ComponentStatus `json:"components"`
}

// ComponentStatus describes the health of a single component.
type ComponentStatus struct {
	Status    string             `json:"status"` // "up", "down", "unconfigured"
	Type      string             `json:"type,omitempty"`
	LatencyMs *int64             `json:"latency_ms,omitempty"`
	Providers []AIProviderStatus `json:"providers,omitempty"`
	Error     string             `json:"error,omitempty"`
}

// AIProviderStatus describes a configured AI provider.
type AIProviderStatus struct {
	Name       string `json:"name"`
	Model      string `json:"model,omitempty"`
	Configured bool   `json:"configured"`
}

// pingable is an optional interface for session stores that support health checks.
type pingable interface {
	Ping(ctx context.Context) error
}

// readiness runs every component check and computes the overall status
// (ready | degraded | unhealthy). Shared by the public /api/v1/ready probe and
// the admin health dashboard so both report identically.
func (s *Server) readiness(ctx context.Context) (map[string]ComponentStatus, string) {
	components := map[string]ComponentStatus{
		"database":      s.checkDatabase(ctx),
		"queue":         s.checkQueue(),
		"ai":            s.checkAI(),
		"session_store": s.checkSessionStore(ctx),
		"email":         s.checkEmail(),
	}

	status := "ready"
	if components["database"].Status == "down" || components["ai"].Status == "down" {
		status = "unhealthy"
	} else {
		for name, comp := range components {
			if name == "database" || name == "ai" {
				continue
			}
			if comp.Status == "down" {
				status = "degraded"
				break
			}
		}
	}
	return components, status
}

// HandleReady returns a detailed readiness check of all server components.
func (s *Server) HandleReady(c echo.Context) error {
	components, status := s.readiness(c.Request().Context())

	httpStatus := http.StatusOK
	if status == "unhealthy" {
		httpStatus = http.StatusServiceUnavailable
	}

	return c.JSON(httpStatus, ReadinessResponse{
		Status:     status,
		Version:    version.Version,
		Components: components,
	})
}

func (s *Server) checkDatabase(ctx context.Context) ComponentStatus {
	if s.wsStores == nil || s.wsStores.pgDB == nil {
		return ComponentStatus{Status: "unconfigured"}
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	start := time.Now()
	if err := s.wsStores.pgDB.PingContext(ctx); err != nil {
		return ComponentStatus{Status: "down", Type: "postgres", Error: err.Error()}
	}
	latency := time.Since(start).Milliseconds()
	return ComponentStatus{Status: "up", Type: "postgres", LatencyMs: &latency}
}

func (s *Server) checkQueue() ComponentStatus {
	if s.JobQueue == nil {
		return ComponentStatus{Status: "unconfigured"}
	}
	queueType := "channel"
	switch s.JobQueue.(type) {
	case interface{ Healthy() bool }:
		// Use type name for known types.
	}
	if s.Config.NATSURL != "" {
		queueType = "nats"
	}
	if !s.JobQueue.Healthy() {
		return ComponentStatus{Status: "down", Type: queueType}
	}
	return ComponentStatus{Status: "up", Type: queueType}
}

func (s *Server) checkAI() ComponentStatus {
	var providers []AIProviderStatus

	// A server-configured platform provider (the hosted cloud's Bedrock task role)
	// makes AI available even when the local file-based CredentialStore is empty
	// and per-workspace keys live in Postgres. Without this, hosted deployments —
	// which never populate the file store — reported ai:down and failed readiness.
	if s.Config.PlatformProvider != "" {
		providers = append(providers, AIProviderStatus{
			Name:       s.Config.PlatformProvider,
			Model:      s.Config.PlatformModel,
			Configured: true,
		})
	}

	if s.CredentialStore != nil {
		for _, cfg := range s.CredentialStore.List() {
			_, err := s.CredentialStore.GetAPIKey(cfg.ID)
			providers = append(providers, AIProviderStatus{
				Name:       cfg.ProviderType,
				Model:      cfg.Model,
				Configured: err == nil,
			})
		}
	}

	if len(providers) == 0 {
		if s.CredentialStore == nil {
			return ComponentStatus{Status: "unconfigured"}
		}
		return ComponentStatus{Status: "down", Providers: []AIProviderStatus{}}
	}
	return ComponentStatus{Status: "up", Providers: providers}
}

func (s *Server) checkSessionStore(ctx context.Context) ComponentStatus {
	if s.SessionStore == nil {
		return ComponentStatus{Status: "unconfigured"}
	}
	if p, ok := s.SessionStore.(pingable); ok {
		ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		if err := p.Ping(ctx); err != nil {
			return ComponentStatus{Status: "down", Type: "redis", Error: err.Error()}
		}
		return ComponentStatus{Status: "up", Type: "redis"}
	}
	return ComponentStatus{Status: "up", Type: "memory"}
}

func (s *Server) checkEmail() ComponentStatus {
	if s.EmailSender == nil {
		return ComponentStatus{Status: "unconfigured"}
	}
	return ComponentStatus{Status: "up"}
}

// HandleInfo returns server info: mode, build metadata, and all reference data
// (formats, tools, locales, connector types).
func (s *Server) HandleInfo(c echo.Context) error {
	mode := "server"
	if s.Config.JWTSecret == "" {
		mode = "standalone"
	}

	// Formats.
	nameSet := make(map[registry.FormatID]struct{})
	for _, name := range s.FormatRegistry.ReaderNames() {
		nameSet[name] = struct{}{}
	}
	for _, name := range s.FormatRegistry.WriterNames() {
		nameSet[name] = struct{}{}
	}
	var formats []FormatInfo
	for name := range nameSet {
		formats = append(formats, FormatInfo{
			Name:      string(name),
			HasReader: s.FormatRegistry.HasReader(name),
			HasWriter: s.FormatRegistry.HasWriter(name),
		})
	}
	slices.SortFunc(formats, func(a, b FormatInfo) int {
		return cmp.Compare(a.Name, b.Name)
	})

	// Tools. Return the registry's rich ToolInfo (category, description,
	// is_source_transform, …) so flow-editor palettes can classify tools and
	// gate the source-transform stage.
	toolNames := s.ToolRegistry.Names()
	slices.Sort(toolNames)
	tools := make([]registry.ToolInfo, 0, len(toolNames))
	for _, name := range toolNames {
		if info := s.ToolRegistry.ToolInfo(name); info != nil {
			tools = append(tools, *info)
		} else {
			tools = append(tools, registry.ToolInfo{Name: name})
		}
	}

	// Locales.
	locales := locale.WellKnownLocales()

	// Connector types.
	var connectorTypes []ConnectorTypeInfo
	if s.Services != nil {
		for _, ct := range s.Services.Connector.ListConnectorTypes() {
			connectorTypes = append(connectorTypes, ConnectorTypeInfo{
				Name:     ct.Name,
				Category: string(ct.Category),
			})
		}
	}

	// AI provider types. Sourced from the framework provider registry so the
	// settings UI never re-declares the Go provider constants. We omit the two
	// providers that aren't configurable as a hosted workspace credential:
	// "demo" (illustrative playground backend) and "claude-code" (a keyless
	// local-CLI subscription that can't run server-side).
	var providerTypes []ProviderTypeInfo
	for _, p := range aiprovider.Providers() {
		if p.Name == aiprovider.Demo || p.Name == aiprovider.ClaudeCode {
			continue
		}
		providerTypes = append(providerTypes, ProviderTypeInfo{
			Name:     string(p.Name),
			Label:    p.Label,
			NeedsKey: p.NeedsKey(),
		})
	}
	slices.SortFunc(providerTypes, func(a, b ProviderTypeInfo) int {
		return cmp.Compare(a.Label, b.Label)
	})

	return c.JSON(http.StatusOK, InfoResponse{
		Mode:      mode,
		Version:   version.Version,
		Commit:    version.Commit,
		BuildDate: version.BuildDate,
		Features: map[string]bool{
			"brand_scan": s.BrandScanStore != nil && s.BrandScanQueue != nil,
		},
		Formats:        formats,
		Tools:          tools,
		Locales:        locales,
		ConnectorTypes: connectorTypes,
		ProviderTypes:  providerTypes,
	})
}
