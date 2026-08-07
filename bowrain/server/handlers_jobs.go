package server

import (
	"context"
	"fmt"
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/neokapi/neokapi/bowrain/jobs"
	"github.com/neokapi/neokapi/core/id"
)

// isPlatformProviderConfig reports whether a job's provider_config_id selects the
// platform-held key (metered in credits) rather than a workspace bring-your-own
// key. Mirrors jobs.TranslationJob.IsPlatformProvider for the enqueue path.
func isPlatformProviderConfig(providerConfigID string) bool {
	return providerConfigID == "" || providerConfigID == "platform"
}

// insufficientPlatformCredits reports whether a translation job must be refused
// at enqueue because it would run on the platform-held key with no spendable
// credits (Epic 004). Deduction is post-hoc, so without this guard a zero-credit
// workspace could start a large platform job and drive the ledger deeply
// negative. It never blocks:
//   - BYO jobs (a real provider_config_id) — they burn no credits;
//   - self-hosted / unbilled deployments (nil billing store, empty workspace);
//   - workspaces with no allocation yet or an unreadable balance (CheckCredits
//     error) — the pre-check degrades to allowing, like QuotaGuard.
func (s *Server) insufficientPlatformCredits(ctx context.Context, workspaceID, providerConfigID string) bool {
	if s.BillingStore == nil || workspaceID == "" {
		return false
	}
	if !isPlatformProviderConfig(providerConfigID) {
		return false // BYO — never metered in credits
	}
	remaining, err := s.BillingStore.CheckCredits(ctx, workspaceID)
	if err != nil {
		return false // no allocation / unreadable → allow (degrade gracefully)
	}
	return remaining <= 0
}

// errInsufficientCredits is the enqueue-refusal payload for a zero-credit
// platform-key job. It points the caller at the ways forward (the monthly
// reset applies only to paid plans; Free has no recurring allowance).
var errInsufficientCredits = ErrorResponse{
	Error: "insufficient credits: purchase a credit pack, upgrade your plan, wait for your plan's monthly reset, or configure your own AI provider key",
}

// HandleCreateTranslationJob creates a new async translation job and enqueues it.
// POST /api/v1/:ws/jobs/translate
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

	// The billing workspace ID (set by WorkspaceAccessMiddleware) drives credit
	// deduction in the worker; carry it on the job so platform runs are metered.
	wsID, _ := c.Get("workspace_id").(string)

	ctx := c.Request().Context()

	// Credit pre-check (Epic 004): refuse a platform-key job on a zero-credit
	// workspace up front, before it runs and drives the ledger negative.
	if s.insufficientPlatformCredits(ctx, wsID, providerConfigID) {
		return c.JSON(http.StatusPaymentRequired, errInsufficientCredits)
	}

	requester, _ := c.Get("user_id").(string)
	job := &jobs.TranslationJob{
		ID:               id.New(),
		WorkspaceSlug:    ws,
		WorkspaceID:      wsID,
		ProjectID:        req.ProjectID,
		ItemName:         req.ItemName,
		Stream:           refParam(c),
		TargetLocale:     req.TargetLocale,
		ProviderConfigID: providerConfigID,
		Model:            req.Model,
		BatchSize:        req.BatchSize,
		Concurrency:      req.Concurrency,
		Status:           jobs.StatusQueued,
		// Whoever asked is who hears about it if it fails.
		CreatedBy: requester,
	}

	if err := s.JobStore.CreateJob(ctx, job); err != nil {
		return serverErr(c, err)
	}

	if err := s.JobQueue.Enqueue(ctx, job.ID); err != nil {
		// Roll back the job record.
		_ = s.JobStore.DeleteJob(ctx, job.ID)
		return serverErr(c, fmt.Errorf("enqueue failed: %w", err))
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

	ctx := c.Request().Context()

	// Resolve the owning workspace from the path-scoped project. The slug
	// routes the job's content memory (recycle reads, AI-draft seeding): as a
	// literal "_anon" it pointed every claimed project's job at the shared
	// anonymous memory — leverage lost, and wording leaked across tenants. A
	// genuinely unclaimed project still resolves to no workspace and keeps the
	// anonymous slug.
	workspaceSlug := "_anon"
	if s.ContentStore != nil && s.AuthStore != nil {
		if proj, err := s.ContentStore.GetProject(ctx, projectID); err == nil && proj != nil && proj.WorkspaceID != "" {
			if ws, err := s.AuthStore.GetWorkspace(ctx, proj.WorkspaceID); err == nil && ws != nil && ws.Slug != "" {
				workspaceSlug = ws.Slug
			}
		}
	}

	requester, _ := c.Get("user_id").(string)
	job := &jobs.TranslationJob{
		ID:               id.New(),
		WorkspaceSlug:    workspaceSlug,
		ProjectID:        projectID,
		ItemName:         req.ItemName,
		Stream:           refParam(c),
		TargetLocale:     req.TargetLocale,
		ProviderConfigID: providerConfigID,
		Model:            req.Model,
		Status:           jobs.StatusQueued,
		CreatedBy:        requester,
	}
	if err := s.JobStore.CreateJob(ctx, job); err != nil {
		return serverErr(c, err)
	}

	if err := s.JobQueue.Enqueue(ctx, job.ID); err != nil {
		_ = s.JobStore.DeleteJob(ctx, job.ID)
		return serverErr(c, fmt.Errorf("enqueue failed: %w", err))
	}

	return c.JSON(http.StatusAccepted, map[string]any{
		"job_id": job.ID,
		"status": string(job.Status),
	})
}

// HandleGetJob returns the current status and progress of a job.
// GET /api/v1/:ws/jobs/:id
func (s *Server) HandleGetJob(c echo.Context) error {
	if s.JobStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "job system not configured"})
	}

	id := c.Param("id")
	job, err := s.JobStore.GetJob(c.Request().Context(), id)
	if err != nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: "job not found"})
	}
	// Cross-tenant guard: GetJob resolves by GLOBAL id (unlike ListJobs, which
	// filters WHERE workspace_slug). Without this a caller could read another
	// tenant's job (project id, file paths, provider config, tokens, errors) via
	// /api/v1/<their-ws>/jobs/<victim-job-id>. 404 (anti-enumeration) on mismatch.
	if !jobInRequestWorkspace(c, job) {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: "job not found"})
	}

	return c.JSON(http.StatusOK, job)
}

// jobInRequestWorkspace reports whether the job belongs to the workspace the
// request is scoped to. TranslationJob.WorkspaceSlug stores the workspace SLUG
// (as :ws in the path), so the comparison is against c.Param("ws"). Fails closed
// when the path carries no workspace slug.
func jobInRequestWorkspace(c echo.Context, job *jobs.TranslationJob) bool {
	ws := c.Param("ws")
	return ws != "" && job != nil && job.WorkspaceSlug == ws
}

// HandleListJobs lists recent jobs for a workspace.
// GET /api/v1/:ws/jobs
func (s *Server) HandleListJobs(c echo.Context) error {
	if s.JobStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "job system not configured"})
	}

	ws := c.Param("ws")
	jobList, err := s.JobStore.ListJobs(c.Request().Context(), ws, 50)
	if err != nil {
		return serverErr(c, err)
	}

	return c.JSON(http.StatusOK, jobList)
}

// HandleGetAIUsage returns the AI usage summary for a workspace.
// GET /api/v1/:ws/ai-usage
func (s *Server) HandleGetAIUsage(c echo.Context) error {
	if s.QuotaStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "quota tracking not configured"})
	}

	ws := c.Param("ws")
	summary, err := s.QuotaStore.GetUsageSummary(c.Request().Context(), ws)
	if err != nil {
		return serverErr(c, err)
	}

	return c.JSON(http.StatusOK, summary)
}

// HandleDeleteJob cancels a job by setting its status to failed.
// DELETE /api/v1/:ws/jobs/:id
func (s *Server) HandleDeleteJob(c echo.Context) error {
	if s.JobStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "job system not configured"})
	}

	id := c.Param("id")
	ctx := c.Request().Context()

	// Verify job exists AND belongs to this workspace. GetJob resolves by global
	// id, so without the workspace check a caller could cancel another tenant's
	// in-flight job via /api/v1/<their-ws>/jobs/<victim-job-id>.
	job, err := s.JobStore.GetJob(ctx, id)
	if err != nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: "job not found"})
	}
	if !jobInRequestWorkspace(c, job) {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: "job not found"})
	}

	// Atomically cancel only if still cancellable.
	// UpdateJobStatus is a no-op if the job already completed.
	if err := s.JobStore.UpdateJobStatus(ctx, id, jobs.StatusFailed, "cancelled by user"); err != nil {
		return serverErr(c, err)
	}

	return c.NoContent(http.StatusNoContent)
}
