package server

import (
	"encoding/json"
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/neokapi/neokapi/bowrain/billing"
	platev "github.com/neokapi/neokapi/bowrain/core/event"
	"github.com/neokapi/neokapi/bowrain/platformconfig"
)

// HandleGetPublicConfig returns the unauthenticated, public-safe subset of the
// instance-wide platform configuration: whether signups are open, any
// maintenance banner, and — when the customer model-choice feature is enabled —
// the enabled models and default. The web app calls this on bootstrap to render
// the signup gate, maintenance banner, and (optionally) a model picker.
//
// GET /api/v1/config
func (s *Server) HandleGetPublicConfig(c echo.Context) error {
	return c.JSON(http.StatusOK, s.PlatformConfig.PublicSnapshot())
}

// HandleAdminGetPlatformConfig returns the full admin-facing platform
// configuration (resolved settings + the model catalog for the current provider,
// so the UI can render onboard/offboard without a second call).
//
// GET /api/admin/platform
func (s *Server) HandleAdminGetPlatformConfig(c echo.Context) error {
	return c.JSON(http.StatusOK, platformConfigResponse{
		Snapshot:   s.PlatformConfig.Snapshot(),
		Persistent: s.PlatformConfig.Persistent(),
		Defaults:   s.PlatformConfig.DefaultsValue(),
	})
}

// platformConfigResponse wraps the snapshot with whether the instance can persist
// changes (a DB-less instance serves env defaults read-only) and the bootstrap
// defaults, so the admin UI can render "(default)" hints and disable editing when
// there is no store.
type platformConfigResponse struct {
	Snapshot   platformconfig.Snapshot `json:"snapshot"`
	Persistent bool                    `json:"persistent"`
	Defaults   platformconfig.Defaults `json:"defaults"`
}

// platformConfigUpdate is the PATCH-style request body for updating platform
// config. Every field is a pointer / omittable: only provided fields are written,
// so the UI can save one section without clobbering the rest. Setting a field
// writes its platform_config key; unset fields are left as-is.
type platformConfigUpdate struct {
	AI *struct {
		Provider       *string   `json:"provider,omitempty"`
		DefaultModel   *string   `json:"default_model,omitempty"`
		BaseURL        *string   `json:"base_url,omitempty"`
		EnabledModels  *[]string `json:"enabled_models,omitempty"`
		CustomerChoice *bool     `json:"customer_choice,omitempty"`
	} `json:"ai,omitempty"`
	Signups *struct {
		Open *bool `json:"open,omitempty"`
	} `json:"signups,omitempty"`
	Maintenance *struct {
		Enabled *bool   `json:"enabled,omitempty"`
		Message *string `json:"message,omitempty"`
	} `json:"maintenance,omitempty"`
	WorkspaceDefaults *struct {
		Plan      *string `json:"plan,omitempty"`
		TrialDays *int    `json:"trial_days,omitempty"`
	} `json:"workspace_defaults,omitempty"`
	// Features fully replaces the instance-wide feature flag map when provided
	// (an admin edits the whole set at once). Nil leaves it unchanged.
	Features *map[string]bool `json:"features,omitempty"`
	// ModelSweeps gates measured steerability (model recommendation sweeps).
	ModelSweeps *struct {
		Enabled *bool `json:"enabled,omitempty"`
	} `json:"model_sweeps,omitempty"`
}

// HandleAdminUpdatePlatformConfig applies a partial platform-config update,
// validates it, persists it, then publishes platform_config.changed so every
// server and worker reloads its cached settings without a redeploy.
//
// PUT /api/admin/platform
func (s *Server) HandleAdminUpdatePlatformConfig(c echo.Context) error {
	if !s.PlatformConfig.Persistent() {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{
			Error: "platform config is not persistent on this instance (no database configured)"})
	}
	adminEmail, _ := c.Get("admin_email").(string)

	var req platformConfigUpdate
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "invalid request body"})
	}

	updates := map[string]json.RawMessage{}
	set := func(key string, v any) error {
		b, err := json.Marshal(v)
		if err != nil {
			return err
		}
		updates[key] = b
		return nil
	}

	// Resolve the effective post-update AI values so cross-field validation
	// (default ∈ enabled) reflects both existing and incoming settings.
	effProvider := s.PlatformConfig.AIProvider()
	effDefaultModel := s.PlatformConfig.AIDefaultModel()
	effEnabled := s.PlatformConfig.AIEnabledModels()

	if req.AI != nil {
		if req.AI.Provider != nil {
			if *req.AI.Provider == "" {
				return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "ai.provider cannot be empty"})
			}
			effProvider = *req.AI.Provider
			if err := set(platformconfig.KeyAIProvider, *req.AI.Provider); err != nil {
				return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
			}
		}
		if req.AI.DefaultModel != nil {
			if *req.AI.DefaultModel == "" {
				return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "ai.default_model cannot be empty"})
			}
			effDefaultModel = *req.AI.DefaultModel
			if err := set(platformconfig.KeyAIDefaultModel, *req.AI.DefaultModel); err != nil {
				return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
			}
		}
		if req.AI.BaseURL != nil {
			if err := set(platformconfig.KeyAIBaseURL, *req.AI.BaseURL); err != nil {
				return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
			}
		}
		if req.AI.EnabledModels != nil {
			if len(*req.AI.EnabledModels) == 0 {
				return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "ai.enabled_models cannot be empty; disable customer choice instead"})
			}
			effEnabled = *req.AI.EnabledModels
			if err := set(platformconfig.KeyAIEnabledModels, *req.AI.EnabledModels); err != nil {
				return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
			}
		}
		if req.AI.CustomerChoice != nil {
			if err := set(platformconfig.KeyAICustomerChoice, *req.AI.CustomerChoice); err != nil {
				return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
			}
		}
	}

	// The default model must be one of the enabled models, so the platform's
	// default is always actually onboarded.
	if !containsString(effEnabled, effDefaultModel) {
		return c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: "ai.default_model must be one of ai.enabled_models"})
	}
	_ = effProvider // reserved for future provider-specific validation

	if req.Signups != nil && req.Signups.Open != nil {
		if err := set(platformconfig.KeySignupsOpen, *req.Signups.Open); err != nil {
			return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		}
	}

	if req.Maintenance != nil {
		if req.Maintenance.Enabled != nil {
			if err := set(platformconfig.KeyMaintenanceOn, *req.Maintenance.Enabled); err != nil {
				return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
			}
		}
		if req.Maintenance.Message != nil {
			if err := set(platformconfig.KeyMaintenanceMsg, *req.Maintenance.Message); err != nil {
				return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
			}
		}
	}

	if req.WorkspaceDefaults != nil {
		if req.WorkspaceDefaults.Plan != nil {
			if !billing.ValidPlans[billing.Plan(*req.WorkspaceDefaults.Plan)] {
				return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "invalid workspace_defaults.plan: " + *req.WorkspaceDefaults.Plan})
			}
			if err := set(platformconfig.KeyDefaultPlan, *req.WorkspaceDefaults.Plan); err != nil {
				return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
			}
		}
		if req.WorkspaceDefaults.TrialDays != nil {
			if *req.WorkspaceDefaults.TrialDays <= 0 || *req.WorkspaceDefaults.TrialDays > 365 {
				return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "workspace_defaults.trial_days must be between 1 and 365"})
			}
			if err := set(platformconfig.KeyTrialDays, *req.WorkspaceDefaults.TrialDays); err != nil {
				return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
			}
		}
	}

	if req.Features != nil {
		if err := set(platformconfig.KeyFeatures, *req.Features); err != nil {
			return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		}
	}

	if req.ModelSweeps != nil && req.ModelSweeps.Enabled != nil {
		if err := set(platformconfig.KeyModelSweeps, *req.ModelSweeps.Enabled); err != nil {
			return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		}
	}

	if len(updates) == 0 {
		// Nothing to change — return the current snapshot rather than error.
		return c.JSON(http.StatusOK, platformConfigResponse{
			Snapshot:   s.PlatformConfig.Snapshot(),
			Persistent: true,
			Defaults:   s.PlatformConfig.DefaultsValue(),
		})
	}

	ctx := c.Request().Context()
	if err := s.PlatformConfig.Set(ctx, updates, adminEmail); err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}

	// Fan-out: tell every server and worker to reload its cached settings.
	if s.EventBus != nil {
		s.EventBus.Publish(platev.Event{
			Type:   platev.EventPlatformConfigChanged,
			Source: "server",
			Actor:  adminEmail,
			Data:   map[string]string{"keys": joinKeys(updates)},
		})
	}

	return c.JSON(http.StatusOK, platformConfigResponse{
		Snapshot:   s.PlatformConfig.Snapshot(),
		Persistent: true,
		Defaults:   s.PlatformConfig.DefaultsValue(),
	})
}

func joinKeys(m map[string]json.RawMessage) string {
	first := true
	var b []byte
	for k := range m {
		if !first {
			b = append(b, ',')
		}
		b = append(b, k...)
		first = false
	}
	return string(b)
}
