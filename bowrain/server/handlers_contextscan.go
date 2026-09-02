package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/neokapi/neokapi/bowrain/contextscan"
	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	"github.com/neokapi/neokapi/bowrain/jobs"
	"github.com/neokapi/neokapi/bowrain/knowledge"
	"github.com/neokapi/neokapi/core/id"
	"github.com/neokapi/neokapi/core/model"
	coreprofile "github.com/neokapi/neokapi/core/profile"
	corestorage "github.com/neokapi/neokapi/core/storage"
	"github.com/neokapi/neokapi/terms"
)

// Upload caps for the brand-scan uploads endpoint (epic 016): per-file cap is
// contextscan.MaxFileBytes (10 MiB); a batch carries at most 10 files and
// 40 MiB total.
const (
	maxContextScanUploadFiles      = 10
	maxContextScanUploadTotalBytes = 40 << 20
)

// contextScanUploadEntry is one stored upload in the uploads response.
type contextScanUploadEntry struct {
	Key      string `json:"key"`
	Filename string `json:"filename"`
	Size     int64  `json:"size"`
}

// contextScanSkippedEntry names a file that was not stored, with the reason
// (disallowed type, oversize, or no in-process extractor for it).
type contextScanSkippedEntry struct {
	Name   string `json:"name"`
	Reason string `json:"reason"`
}

// HandleContextScanUploads stores brand source files for a later scan.
// POST /api/v1/:ws/context-scans/uploads (multipart field "files")
//
// Files are validated up front against the contextscan extraction allowlist and
// size caps; each accepted file is wrapped in a ContextScanUploadEnvelope (blob
// keys carry no metadata, and the worker needs the filename to pick a format
// reader) and written to blob storage. Rejected files come back in "skipped"
// with a per-file reason, mirroring the epic-006 upload contract.
//
// Envelopes are not deleted here or by the scan itself (Regenerate reuses the
// keys); the worker's ContextScanUploadSweeper removes them once their job has
// been terminal for the retention window.
func (s *Server) HandleContextScanUploads(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermManageVoice); err != nil {
		return err
	}
	if s.BlobStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "blob storage not configured"})
	}

	form, err := c.MultipartForm()
	if err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "multipart form required: " + err.Error()})
	}
	files := form.File["files"]
	if len(files) == 0 {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "no files provided (multipart field \"files\")"})
	}
	if len(files) > maxContextScanUploadFiles {
		return c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: "too many files: a context scan accepts at most 10 files per upload",
		})
	}
	var totalBytes int64
	for _, fh := range files {
		totalBytes += fh.Size
	}
	if totalBytes > maxContextScanUploadTotalBytes {
		return c.JSON(http.StatusRequestEntityTooLarge, ErrorResponse{
			Error: "upload too large: a context scan accepts at most 40 MiB per upload",
		})
	}

	ctx := c.Request().Context()
	uploads := make([]contextScanUploadEntry, 0, len(files))
	var skipped []contextScanSkippedEntry
	for _, fh := range files {
		contentType := fh.Header.Get("Content-Type")
		if fh.Size > contextscan.MaxFileBytes {
			skipped = append(skipped, contextScanSkippedEntry{
				Name: fh.Filename, Reason: "file too large (max 10 MiB)",
			})
			continue
		}
		// Format check up front — types the registry has no reader for are
		// skipped with the extractor's own reason, before any bytes land.
		if err := contextscan.CheckFileSupported(s.FormatRegistry, fh.Filename, contentType); err != nil {
			skipped = append(skipped, contextScanSkippedEntry{Name: fh.Filename, Reason: err.Error()})
			continue
		}

		src, err := fh.Open()
		if err != nil {
			skipped = append(skipped, contextScanSkippedEntry{Name: fh.Filename, Reason: "unreadable file"})
			continue
		}
		data, err := io.ReadAll(io.LimitReader(src, contextscan.MaxFileBytes+1))
		_ = src.Close()
		if err != nil {
			skipped = append(skipped, contextScanSkippedEntry{Name: fh.Filename, Reason: "unreadable file"})
			continue
		}
		if int64(len(data)) > contextscan.MaxFileBytes {
			skipped = append(skipped, contextScanSkippedEntry{
				Name: fh.Filename, Reason: "file too large (max 10 MiB)",
			})
			continue
		}

		envelope, err := json.Marshal(jobs.ContextScanUploadEnvelope{
			Filename:    fh.Filename,
			ContentType: contentType,
			Data:        data,
		})
		if err != nil {
			skipped = append(skipped, contextScanSkippedEntry{Name: fh.Filename, Reason: "failed to encode upload"})
			continue
		}
		ref, err := s.BlobStore.Upload(ctx, envelope, corestorage.UploadOptions{
			ContentType: "application/json",
			Filename:    fh.Filename,
		})
		if err != nil {
			skipped = append(skipped, contextScanSkippedEntry{Name: fh.Filename, Reason: "failed to store upload"})
			continue
		}
		uploads = append(uploads, contextScanUploadEntry{
			Key:      ref.Key,
			Filename: fh.Filename,
			Size:     int64(len(data)),
		})
	}

	resp := map[string]any{"uploads": uploads}
	if len(skipped) > 0 {
		resp["skipped"] = skipped
	}
	return c.JSON(http.StatusOK, resp)
}

// HandleCreateContextScan enqueues an async AI context scan (epic 016).
// POST /api/v1/:ws/context-scans
//
// Route middleware applies the per-IP AI throttle and billing.QuotaGuard; the
// handler additionally requires PermManageVoice and refuses a scan up front
// when the workspace has no spendable platform credits (402) — scans always
// run on the platform key, and deduction is post-hoc.
func (s *Server) HandleCreateContextScan(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermManageVoice); err != nil {
		return err
	}
	if s.ContextScanStore == nil || s.ContextScanQueue == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "context scan system not configured"})
	}

	var req jobs.ContextScanRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	}
	if !req.HasSource() {
		return c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: "at least one source is required: paste_text, urls, upload_keys, or repo_url",
		})
	}
	if len(req.URLs) > jobs.MaxContextScanURLs {
		return c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: "too many urls: a context scan accepts at most 5 links",
		})
	}

	ctx := c.Request().Context()
	wsID, _ := c.Get("workspace_id").(string)

	// Credit pre-check (Epic 004 pattern): brand scans are always platform-key
	// work, so a zero-credit workspace is refused before the job runs.
	if s.insufficientPlatformCredits(ctx, wsID, "platform") {
		return c.JSON(http.StatusPaymentRequired, errInsufficientCredits)
	}

	requestJSON, err := json.Marshal(req)
	if err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	}

	job := &jobs.ContextScanJob{
		ID:            id.New(),
		WorkspaceID:   wsID,
		WorkspaceSlug: c.Param("ws"),
		Status:        jobs.ContextScanStatusQueued,
		Request:       requestJSON,
	}
	if err := s.ContextScanStore.CreateContextScanJob(ctx, job); err != nil {
		return serverErr(c, err)
	}
	if err := s.ContextScanQueue.Enqueue(ctx, job.ID); err != nil {
		// Roll back the job record so no orphaned queued row lingers.
		_ = s.ContextScanStore.DeleteContextScanJob(ctx, job.ID)
		return serverErr(c, fmt.Errorf("enqueue failed: %w", err))
	}

	return c.JSON(http.StatusAccepted, map[string]any{
		"job_id": job.ID,
		"status": string(job.Status),
	})
}

// contextScanInRequestWorkspace reports whether the brand-scan job belongs to
// the workspace the request is scoped to. GetContextScanJob resolves by GLOBAL
// id, so without this a caller could read another tenant's scan (sources,
// draft profile, token spend) via /api/v1/<their-ws>/context-scans/<victim-id>.
// 404 (anti-enumeration) on mismatch, mirroring jobInRequestWorkspace.
func contextScanInRequestWorkspace(c echo.Context, job *jobs.ContextScanJob) bool {
	ws := c.Param("ws")
	return ws != "" && job != nil && job.WorkspaceSlug == ws
}

// HandleGetContextScan returns the status, progress, and (when completed) the
// draft result of a context scan.
// GET /api/v1/:ws/context-scans/:id
func (s *Server) HandleGetContextScan(c echo.Context) error {
	if s.ContextScanStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "context scan system not configured"})
	}

	job, err := s.ContextScanStore.GetContextScanJob(c.Request().Context(), c.Param("id"))
	if err != nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: "context scan not found"})
	}
	if !contextScanInRequestWorkspace(c, job) {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: "context scan not found"})
	}

	resp := map[string]any{
		"id":          job.ID,
		"status":      string(job.Status),
		"progress":    job.Progress,
		"message":     job.Message,
		"tokens_used": job.TokensUsed,
	}
	if job.Error != "" {
		resp["error"] = job.Error
	}
	if job.Status == jobs.ContextScanStatusCompleted && len(job.Result) > 0 {
		resp["draft"] = job.Result
	}
	return c.JSON(http.StatusOK, resp)
}

// ContextScanApprovedTerm is one candidate term a reviewer kept. Locale falls
// back to the request's Locale, then to "en".
type ContextScanApprovedTerm struct {
	Term       string `json:"term"`
	Definition string `json:"definition,omitempty"`
	Domain     string `json:"domain,omitempty"`
	Locale     string `json:"locale,omitempty"`
}

// ContextScanApproveRequest is the reviewed outcome of a context scan: the edited
// draft profile and the candidate terms that survived review.
type ContextScanApproveRequest struct {
	// At is the point the approved artefacts would govern, as an axis map.
	// EMPTY is the project's default point — whatever defaults.coordinates
	// resolves to — which is the onboarding case and always accepted.
	At      map[string]string         `json:"at,omitempty"`
	Profile VoiceProfileRequest       `json:"profile"`
	Terms   []ContextScanApprovedTerm `json:"terms,omitempty"`
	// Locale is the locale approved terms are created in when a term does not
	// name its own. Defaults to "en".
	Locale string `json:"locale,omitempty"`
}

// ContextScanApproveResponse reports what the approval applied: the stored
// profile and which action produced it, plus the concepts created and the ones
// that were already there.
type ContextScanApproveResponse struct {
	Profile          *coreprofile.VoiceProfile `json:"profile"`
	ProfileAction    string                    `json:"profile_action"`
	ConceptsCreated  int                       `json:"concepts_created"`
	ConceptsExisting int                       `json:"concepts_existing"`
	ConceptIDs       []string                  `json:"concept_ids"`
}

// HandleApproveContextScan applies a reviewed context scan in one request: the
// profile and every approved term.
// POST /api/v1/:ws/context-scans/:id/approve
//
// The endpoint is idempotent by content, so a retry after a partial failure
// converges instead of duplicating. The profile goes through the same
// upsert-by-name the push uses; a term is created only when the workspace has
// no concept carrying it in that locale already. Everything is validated
// before anything is written, so a malformed term never leaves a profile
// stored without its vocabulary.
//
// Candidates enter curation as "proposed": creating a term already forbidden
// or preferred is a governed transition that must travel through a change-set,
// and a scan never bypasses that.
func (s *Server) HandleApproveContextScan(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermManageVoice); err != nil {
		return err
	}
	if s.ContextScanStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "context scan system not configured"})
	}
	if s.VoiceStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "voice not configured"})
	}
	if s.wsStores == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "editor not configured"})
	}

	ctx := c.Request().Context()
	job, err := s.ContextScanStore.GetContextScanJob(ctx, c.Param("id"))
	if err != nil || !contextScanInRequestWorkspace(c, job) {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: "context scan not found"})
	}
	if job.Status != jobs.ContextScanStatusCompleted {
		return c.JSON(http.StatusConflict, ErrorResponse{
			Error: fmt.Sprintf("only a completed context scan can be approved (status %q)", job.Status),
		})
	}

	var req ContextScanApproveRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	}
	if strings.TrimSpace(req.Profile.Name) == "" {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "profile.name is required"})
	}
	if unknown := s.undeclaredAxes(ctx, c, req.At); len(unknown) > 0 {
		return c.JSON(http.StatusConflict, ErrorResponse{
			Error: fmt.Sprintf(
				"cannot bind at %s: this workspace declares no content on %s. Declare the axis in a recipe and push before binding anything at it.",
				formatPoint(req.At), strings.Join(unknown, ", ")),
		})
	}
	defaultLocale := model.LocaleID(strings.TrimSpace(req.Locale))
	if defaultLocale == "" {
		defaultLocale = "en"
	}
	approved := make([]terms.Term, 0, len(req.Terms))
	for i, t := range req.Terms {
		text := strings.TrimSpace(t.Term)
		if text == "" {
			return c.JSON(http.StatusBadRequest, ErrorResponse{
				Error: fmt.Sprintf("terms[%d].term is required", i),
			})
		}
		locale := model.LocaleID(strings.TrimSpace(t.Locale))
		if locale == "" {
			locale = defaultLocale
		}
		approved = append(approved, terms.Term{Text: text, Locale: locale, Status: model.TermProposed})
	}

	tb, err := s.wsStores.getTerms(c.Param("ws"))
	if err != nil {
		return serverErrStatus(c, http.StatusServiceUnavailable, err)
	}

	wsID, _ := c.Get("workspace_id").(string)
	actor, _ := c.Get("user_id").(string)
	upsert, err := s.upsertVoiceProfile(ctx, wsID, actor, req.Profile, "superseded by a brand-scan approval")
	if err != nil {
		return serverErr(c, err)
	}

	resp := ContextScanApproveResponse{
		Profile:       upsert.Profile,
		ProfileAction: upsert.Action,
		ConceptIDs:    []string{},
	}
	for i, term := range approved {
		existingID, err := findConceptByTerm(ctx, tb, term)
		if err != nil {
			return s.partialApproval(c, resp, err)
		}
		if existingID != "" {
			resp.ConceptsExisting++
			resp.ConceptIDs = append(resp.ConceptIDs, existingID)
			continue
		}
		now := time.Now().UTC()
		concept := terms.Concept{
			ID:         id.New(),
			Domain:     req.Terms[i].Domain,
			Definition: req.Terms[i].Definition,
			Terms:      []terms.Term{term},
			CreatedAt:  now,
			UpdatedAt:  now,
		}
		if err := tb.AddConcept(ctx, concept); err != nil {
			return s.partialApproval(c, resp, err)
		}
		resp.ConceptsCreated++
		resp.ConceptIDs = append(resp.ConceptIDs, concept.ID)
		s.publishKnowledgeEvents(c, []knowledge.MergeEvent{
			conceptEvent(knowledge.EventConceptCreated, wsID, concept.ID, actor),
		})
	}
	return c.JSON(http.StatusOK, resp)
}

// partialApproval reports an approval that stopped part-way, naming what did
// land. The cause stays in the log against the request's reference; the body
// carries the caller's own state, which is what makes the retry safe.
func (s *Server) partialApproval(c echo.Context, resp ContextScanApproveResponse, cause error) error {
	slog.ErrorContext(c.Request().Context(), "brand-scan approval stopped part-way",
		"scan_id", c.Param("id"), "concepts_created", resp.ConceptsCreated,
		"reference", requestID(c), "error", cause)
	return c.JSON(http.StatusInternalServerError, map[string]any{
		"error":   "the approval was partially applied; retry it. The profile and the terms already stored are left as they are",
		"applied": resp,
	})
}

// findConceptByTerm returns the id of a concept already carrying term in its
// locale, or the empty string. Identity is the locale plus the lowered text,
// matching the change-set ops' term identity.
func findConceptByTerm(ctx context.Context, tb terms.Store, term terms.Term) (string, error) {
	concepts, _, err := tb.Search(ctx, term.Text, term.Locale, "", 0, conceptTermProbeLimit)
	if err != nil {
		return "", err
	}
	want := strings.ToLower(term.Text)
	for _, cp := range concepts {
		for _, existing := range cp.Terms {
			if existing.Locale == term.Locale && strings.ToLower(existing.Text) == want {
				return cp.ID, nil
			}
		}
	}
	return "", nil
}

// conceptTermProbeLimit bounds the page findConceptByTerm scans for an exact
// term. The search ranks by similarity, so an exact match that is not in the
// first page is not a match anyone would find either.
const conceptTermProbeLimit = 50

// VoiceDraftCheckRequest is the request body for the stateless draft tester:
// an (unsaved, possibly user-edited) draft profile plus sample text.
type VoiceDraftCheckRequest struct {
	Profile *coreprofile.VoiceProfile `json:"profile"`
	Text    string                    `json:"text"`
}

// HandleCheckVoiceDraft scores sample text against an inline draft voice
// profile — the zero-AI-cost live tester for the scan review surface.
// POST /api/v1/:ws/context-scans/check-draft
//
// It mirrors HandleCheckVoice but takes the profile in the request body
// instead of loading a stored one, so a draft can be tested (and refined)
// before it is ever persisted. Deterministic matcher only — no provider
// call, hence no QuotaGuard.
func (s *Server) HandleCheckVoiceDraft(c echo.Context) error {
	var req VoiceDraftCheckRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	}
	if req.Text == "" {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "text is required"})
	}
	if req.Profile == nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "profile is required"})
	}

	// Same deterministic gate as the stored-profile check: whole-word,
	// Unicode-aware vocabulary matching plus prohibited style patterns, anchored
	// to a single text run.
	runs := []model.Run{{Text: &model.TextRun{Text: req.Text}}}
	findings := coreprofile.Findings(req.Profile, req.Text, runs)
	if findings == nil {
		findings = []coreprofile.VoiceFinding{}
	}
	score := coreprofile.CalculateScore(findings)

	return c.JSON(http.StatusOK, VoiceCheckResponse{
		Score:    score,
		Findings: findings,
	})
}

// undeclaredAxes reports which axes of a proposed point this workspace has no
// declared content on, in sorted order. Empty means the point is bindable.
//
// The recipe is the only thing that mints a coordinate, and the server's copy
// of what a recipe declares is the collection rows a push reconciles — the same
// index the context-profiles board is built from. So "is this axis real?" is
// answered from pushed content rather than from a list the server keeps
// separately, which is what stops the server believing in a coordinate the
// recipe has never heard of.
//
// An empty point is the project's default point and is always bindable: it is
// where a scan that found no structure proposes, and requiring a declaration
// for it would make onboarding depend on a push that has not happened yet.
func (s *Server) undeclaredAxes(ctx context.Context, c echo.Context, at map[string]string) []string {
	if len(at) == 0 {
		return nil
	}

	declared := map[string]bool{}
	wsID, _ := c.Get("workspace_id").(string)
	if s.Services != nil && s.Services.Project != nil && s.ContentStore != nil {
		if projects, err := s.Services.Project.ListProjects(ctx); err == nil {
			points := newProfilePoints()
			for _, pr := range projects {
				if pr == nil || pr.WorkspaceID != wsID || pr.Archived {
					continue
				}
				s.collectProfilePoints(ctx, points, pr)
			}
			for _, axis := range points.axes() {
				declared[axis] = true
			}
		}
	}

	var unknown []string
	for axis, value := range at {
		if strings.TrimSpace(axis) == "" || strings.TrimSpace(value) == "" {
			continue
		}
		if !declared[axis] {
			unknown = append(unknown, axis)
		}
	}
	sort.Strings(unknown)
	return unknown
}

// formatPoint renders a point the way a recipe writes it, for an error a
// reader can act on: axis=value pairs in a stable order.
func formatPoint(at map[string]string) string {
	if len(at) == 0 {
		return "the default point"
	}
	axes := make([]string, 0, len(at))
	for axis := range at {
		axes = append(axes, axis)
	}
	sort.Strings(axes)
	parts := make([]string, 0, len(axes))
	for _, axis := range axes {
		parts = append(parts, axis+"="+at[axis])
	}
	return strings.Join(parts, ",")
}
