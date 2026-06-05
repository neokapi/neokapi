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
	Mode           string              `json:"mode"` // "standalone" or "server"
	Version        string              `json:"version"`
	Commit         string              `json:"commit"`
	BuildDate      string              `json:"build_date"`
	Formats        []FormatInfo        `json:"formats"`
	Tools          []registry.ToolInfo `json:"tools"`
	Locales        []locale.LocaleInfo `json:"locales"`
	ConnectorTypes []ConnectorTypeInfo `json:"connector_types,omitempty"`
}

// ConnectorTypeInfo describes an available connector type.
type ConnectorTypeInfo struct {
	Name     string `json:"name"`
	Category string `json:"category"`
}

// FormatInfo describes a registered format.
type FormatInfo struct {
	Name      string `json:"name"`
	HasReader bool   `json:"has_reader"`
	HasWriter bool   `json:"has_writer"`
}

// ErrorResponse is a standard error response.
type ErrorResponse struct {
	Error   string `json:"error"`
	Details string `json:"details,omitempty"`
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

// HandleReady returns a detailed readiness check of all server components.
func (s *Server) HandleReady(c echo.Context) error {
	components := make(map[string]ComponentStatus)

	// Database check.
	components["database"] = s.checkDatabase()

	// Queue check.
	components["queue"] = s.checkQueue()

	// AI provider check.
	components["ai"] = s.checkAI()

	// Session store check.
	components["session_store"] = s.checkSessionStore()

	// Email check.
	components["email"] = s.checkEmail()

	// Compute overall status.
	status := "ready"
	dbStatus := components["database"].Status
	aiStatus := components["ai"].Status

	if dbStatus == "down" || aiStatus == "down" {
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

func (s *Server) checkDatabase() ComponentStatus {
	if s.wsStores.pgDB == nil {
		return ComponentStatus{Status: "unconfigured"}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
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
	} else if s.Config.ServiceBusConnection != "" {
		queueType = "servicebus"
	}
	if !s.JobQueue.Healthy() {
		return ComponentStatus{Status: "down", Type: queueType}
	}
	return ComponentStatus{Status: "up", Type: queueType}
}

func (s *Server) checkAI() ComponentStatus {
	if s.CredentialStore == nil {
		return ComponentStatus{Status: "unconfigured"}
	}
	configs := s.CredentialStore.List()
	if len(configs) == 0 {
		return ComponentStatus{Status: "down", Providers: []AIProviderStatus{}}
	}
	providers := make([]AIProviderStatus, 0, len(configs))
	for _, cfg := range configs {
		_, err := s.CredentialStore.GetAPIKey(cfg.ID)
		providers = append(providers, AIProviderStatus{
			Name:       cfg.ProviderType,
			Model:      cfg.Model,
			Configured: err == nil,
		})
	}
	return ComponentStatus{Status: "up", Providers: providers}
}

func (s *Server) checkSessionStore() ComponentStatus {
	if s.SessionStore == nil {
		return ComponentStatus{Status: "unconfigured"}
	}
	if p, ok := s.SessionStore.(pingable); ok {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
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

	return c.JSON(http.StatusOK, InfoResponse{
		Mode:           mode,
		Version:        version.Version,
		Commit:         version.Commit,
		BuildDate:      version.BuildDate,
		Formats:        formats,
		Tools:          tools,
		Locales:        locales,
		ConnectorTypes: connectorTypes,
	})
}
