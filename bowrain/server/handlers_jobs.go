package server

import (
	"net/http"

	"github.com/gokapi/gokapi/bowrain/jobs"
	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
)

// HandleCreateTranslationJob creates a new async translation job and enqueues it.
// POST /api/v1/workspaces/:ws/jobs/translate
func (s *Server) HandleCreateTranslationJob(c echo.Context) error {
	if s.JobStore == nil || s.JobQueue == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "job system not configured"})
	}

	ws := c.Param("ws")

	var req struct {
		ProjectID        string `json:"project_id"`
		ItemName         string `json:"item_name"`
		TargetLocale     string `json:"target_locale"`
		ProviderConfigID string `json:"provider_config_id"`
		Model            string `json:"model,omitempty"`
		BatchSize        int    `json:"batch_size,omitempty"`
		Concurrency      int    `json:"concurrency,omitempty"`
	}
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	}

	if req.ProjectID == "" || req.ItemName == "" || req.TargetLocale == "" {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "project_id, item_name, and target_locale are required"})
	}

	// Default to platform provider if none specified.
	providerConfigID := req.ProviderConfigID
	if providerConfigID == "" {
		providerConfigID = "platform"
	}

	job := &jobs.TranslationJob{
		ID:               uuid.NewString(),
		WorkspaceSlug:    ws,
		ProjectID:        req.ProjectID,
		ItemName:         req.ItemName,
		TargetLocale:     req.TargetLocale,
		ProviderConfigID: providerConfigID,
		Model:            req.Model,
		BatchSize:        req.BatchSize,
		Concurrency:      req.Concurrency,
		Status:           jobs.StatusQueued,
	}

	ctx := c.Request().Context()
	if err := s.JobStore.CreateJob(ctx, job); err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}

	if err := s.JobQueue.Enqueue(ctx, job.ID); err != nil {
		// Roll back the job record.
		_ = s.JobStore.DeleteJob(ctx, job.ID)
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "enqueue failed: " + err.Error()})
	}

	return c.JSON(http.StatusAccepted, map[string]any{
		"job_id": job.ID,
		"status": string(job.Status),
	})
}

// HandleCreateProjectTranslationJob creates a translation job scoped to a project.
// POST /api/v1/projects/:id/sync/translate
// Uses ClaimOrAuth middleware (no workspace required).
func (s *Server) HandleCreateProjectTranslationJob(c echo.Context) error {
	if s.JobStore == nil || s.JobQueue == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "job system not configured"})
	}

	projectID := c.Param("id")

	var req struct {
		ItemName         string `json:"item_name"`
		TargetLocale     string `json:"target_locale"`
		ProviderConfigID string `json:"provider_config_id"`
		Model            string `json:"model,omitempty"`
	}
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	}

	if req.ItemName == "" || req.TargetLocale == "" {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "item_name and target_locale are required"})
	}

	providerConfigID := req.ProviderConfigID
	if providerConfigID == "" {
		providerConfigID = "platform"
	}

	job := &jobs.TranslationJob{
		ID:               uuid.NewString(),
		WorkspaceSlug:    "_anon",
		ProjectID:        projectID,
		ItemName:         req.ItemName,
		TargetLocale:     req.TargetLocale,
		ProviderConfigID: providerConfigID,
		Model:            req.Model,
		Status:           jobs.StatusQueued,
	}

	ctx := c.Request().Context()
	if err := s.JobStore.CreateJob(ctx, job); err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}

	if err := s.JobQueue.Enqueue(ctx, job.ID); err != nil {
		_ = s.JobStore.DeleteJob(ctx, job.ID)
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "enqueue failed: " + err.Error()})
	}

	return c.JSON(http.StatusAccepted, map[string]any{
		"job_id": job.ID,
		"status": string(job.Status),
	})
}

// HandleGetJob returns the current status and progress of a job.
// GET /api/v1/workspaces/:ws/jobs/:id
func (s *Server) HandleGetJob(c echo.Context) error {
	if s.JobStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "job system not configured"})
	}

	id := c.Param("id")
	job, err := s.JobStore.GetJob(c.Request().Context(), id)
	if err != nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: "job not found"})
	}

	return c.JSON(http.StatusOK, job)
}

// HandleListJobs lists recent jobs for a workspace.
// GET /api/v1/workspaces/:ws/jobs
func (s *Server) HandleListJobs(c echo.Context) error {
	if s.JobStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "job system not configured"})
	}

	ws := c.Param("ws")
	jobList, err := s.JobStore.ListJobs(c.Request().Context(), ws, 50)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}

	return c.JSON(http.StatusOK, jobList)
}

// HandleGetAIUsage returns the AI usage summary for a workspace.
// GET /api/v1/workspaces/:ws/ai/usage
func (s *Server) HandleGetAIUsage(c echo.Context) error {
	if s.QuotaStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "quota tracking not configured"})
	}

	ws := c.Param("ws")
	summary, err := s.QuotaStore.GetUsageSummary(c.Request().Context(), ws)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}

	return c.JSON(http.StatusOK, summary)
}

// HandleDeleteJob cancels a job by setting its status to failed.
// DELETE /api/v1/workspaces/:ws/jobs/:id
func (s *Server) HandleDeleteJob(c echo.Context) error {
	if s.JobStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "job system not configured"})
	}

	id := c.Param("id")
	ctx := c.Request().Context()

	// Verify job exists.
	if _, err := s.JobStore.GetJob(ctx, id); err != nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: "job not found"})
	}

	// Atomically cancel only if still cancellable.
	// UpdateJobStatus is a no-op if the job already completed.
	if err := s.JobStore.UpdateJobStatus(ctx, id, jobs.StatusFailed, "cancelled by user"); err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}

	return c.NoContent(http.StatusNoContent)
}
