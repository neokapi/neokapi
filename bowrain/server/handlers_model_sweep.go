package server

import (
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	"github.com/neokapi/neokapi/bowrain/jobs"
)

// Measured steerability endpoints: Bowrain periodically measures how much each
// candidate model actually obeys THIS project's brand context (voice +
// term rules + DNT) and recommends the winner. Sweeps are platform QC — they run
// on the platform provider and never deduct customer credits.
//
// Trigger surface: (a) the explicit refresh POST below (PermManageVoice,
// gated on model_sweeps.enabled); (c) the platform's weekly cron can dispatch
// per-project sweeps through the same enqueue path later. A brand.profile.
// updated event exists, but resolving which projects (and locales) a profile
// change affects means scanning every project's binding ladder, so an
// event-debounced auto-resweep is deliberately NOT wired — refresh manually or
// let the cron re-measure.

// modelSweepRow is one measurement in the API response.
type modelSweepRow struct {
	Model         string    `json:"model"`
	Adherence     float64   `json:"adherence"`
	AdherenceBare float64   `json:"adherence_bare"`
	Lift          float64   `json:"lift"`
	FixtureCount  int       `json:"fixture_count"`
	TokensUsed    int       `json:"tokens_used"`
	MeasuredAt    time.Time `json:"measured_at"`
	Recommended   bool      `json:"recommended"`
}

// modelSweepLocale groups a locale's measurements with its recommendation.
type modelSweepLocale struct {
	Locale           string          `json:"locale"`
	RecommendedModel string          `json:"recommended_model,omitempty"`
	Models           []modelSweepRow `json:"models"`
}

// modelRecommendationsResponse is the GET payload: the instance gate (so the
// settings panel can disable Refresh with a reason) plus the latest
// measurement per (locale, model) and the per-locale recommendation.
type modelRecommendationsResponse struct {
	Enabled bool               `json:"enabled"`
	Locales []modelSweepLocale `json:"locales"`
}

// HandleGetModelRecommendations returns the project's measured steerability
// results: per target locale, the latest measurement per candidate model and
// the recommended model (highest lift above the absolute-adherence floor with
// enough fixtures).
//
// GET /api/v1/:ws/:id/model-recommendations
func (s *Server) HandleGetModelRecommendations(c echo.Context) error {
	if s.SweepStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "model sweeps not configured"})
	}
	projectID := c.Param("id")

	rows, err := s.SweepStore.ListModelSweepResults(c.Request().Context(), projectID)
	if err != nil {
		return serverErr(c, err)
	}

	// Group by locale (rows arrive locale-sorted, lift-descending) and mark
	// each locale's recommendation with the shared policy.
	byLocale := map[string][]*jobs.ModelSweepResult{}
	var order []string
	for _, r := range rows {
		if _, ok := byLocale[r.Locale]; !ok {
			order = append(order, r.Locale)
		}
		byLocale[r.Locale] = append(byLocale[r.Locale], r)
	}

	resp := modelRecommendationsResponse{
		Enabled: s.PlatformConfig.ModelSweepsEnabled(),
		Locales: make([]modelSweepLocale, 0, len(order)),
	}
	for _, locale := range order {
		group := byLocale[locale]
		best := jobs.RecommendModel(group)
		loc := modelSweepLocale{Locale: locale}
		if best != nil {
			loc.RecommendedModel = best.Model
		}
		for _, r := range group {
			loc.Models = append(loc.Models, modelSweepRow{
				Model:         r.Model,
				Adherence:     r.Adherence,
				AdherenceBare: r.AdherenceBare,
				Lift:          r.Lift,
				FixtureCount:  r.FixtureCount,
				TokensUsed:    r.TokensUsed,
				MeasuredAt:    r.MeasuredAt,
				Recommended:   best != nil && r.Model == best.Model,
			})
		}
		resp.Locales = append(resp.Locales, loc)
	}
	return c.JSON(http.StatusOK, resp)
}

// HandleRefreshModelRecommendations enqueues one model sweep job per target
// locale for the project. Requires PermManageVoice and the instance-wide
// model_sweeps.enabled flag (default OFF). Sweeps ride the translation job
// queue as sentinel jobs (__model_sweep__) and are platform QC: no credit
// pre-check and no credit deduction.
//
// POST /api/v1/:ws/:id/model-recommendations/refresh
func (s *Server) HandleRefreshModelRecommendations(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermManageVoice); err != nil {
		return err
	}
	if !s.PlatformConfig.ModelSweepsEnabled() {
		return c.JSON(http.StatusForbidden, ErrorResponse{
			Error: "model sweeps are disabled on this instance (model_sweeps.enabled)"})
	}
	if s.JobStore == nil || s.JobQueue == nil || s.SweepStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "job system not configured"})
	}
	if s.ContentStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "content store not configured"})
	}

	ctx := c.Request().Context()
	projectID := c.Param("id")
	proj, err := s.ContentStore.GetProject(ctx, projectID)
	if err != nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: "project not found"})
	}
	if len(proj.TargetLanguages) == 0 {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "project has no target languages to measure"})
	}

	ws := c.Param("ws")
	wsID, _ := c.Get("workspace_id").(string)

	locales := make([]string, 0, len(proj.TargetLanguages))
	for _, locale := range proj.TargetLanguages {
		job := jobs.NewModelSweepJob(wsID, ws, projectID, string(locale))
		if err := s.JobStore.CreateJob(ctx, job); err != nil {
			return serverErr(c, err)
		}
		if err := s.JobQueue.Enqueue(ctx, job.ID); err != nil {
			return serverErr(c, err)
		}
		locales = append(locales, string(locale))
	}

	return c.JSON(http.StatusAccepted, map[string]any{
		"enqueued": len(locales),
		"locales":  locales,
	})
}
