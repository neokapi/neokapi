package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/neokapi/neokapi/bowrain/brandscan"
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
// brandscan.MaxFileBytes (10 MiB); a batch carries at most 10 files and
// 40 MiB total.
const (
	maxBrandScanUploadFiles      = 10
	maxBrandScanUploadTotalBytes = 40 << 20
)

// brandScanUploadEntry is one stored upload in the uploads response.
type brandScanUploadEntry struct {
	Key      string `json:"key"`
	Filename string `json:"filename"`
	Size     int64  `json:"size"`
}

// brandScanSkippedEntry names a file that was not stored, with the reason
// (disallowed type, oversize, or a deferred pdf/pptx extractor).
type brandScanSkippedEntry struct {
	Name   string `json:"name"`
	Reason string `json:"reason"`
}

// HandleBrandScanUploads stores brand source files for a later scan.
// POST /api/v1/:ws/brand-scans/uploads (multipart field "files")
//
// Files are validated up front against the brandscan extraction allowlist and
// size caps; each accepted file is wrapped in a BrandScanUploadEnvelope (blob
// keys carry no metadata, and the worker needs the filename to pick a format
// reader) and written to blob storage. Rejected files come back in "skipped"
// with a per-file reason, mirroring the epic-006 upload contract.
//
// Envelopes are not deleted here or by the scan itself (Regenerate reuses the
// keys); the worker's BrandScanUploadSweeper removes them once their job has
// been terminal for the retention window.
func (s *Server) HandleBrandScanUploads(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermManageBrand); err != nil {
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
	if len(files) > maxBrandScanUploadFiles {
		return c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: "too many files: a brand scan accepts at most 10 files per upload",
		})
	}
	var totalBytes int64
	for _, fh := range files {
		totalBytes += fh.Size
	}
	if totalBytes > maxBrandScanUploadTotalBytes {
		return c.JSON(http.StatusRequestEntityTooLarge, ErrorResponse{
			Error: "upload too large: a brand scan accepts at most 40 MiB per upload",
		})
	}

	ctx := c.Request().Context()
	uploads := make([]brandScanUploadEntry, 0, len(files))
	var skipped []brandScanSkippedEntry
	for _, fh := range files {
		contentType := fh.Header.Get("Content-Type")
		if fh.Size > brandscan.MaxFileBytes {
			skipped = append(skipped, brandScanSkippedEntry{
				Name: fh.Filename, Reason: "file too large (max 10 MiB)",
			})
			continue
		}
		// Format check up front — types the registry has no reader for are
		// skipped with the extractor's own reason, before any bytes land.
		if err := brandscan.CheckFileSupported(s.FormatRegistry, fh.Filename, contentType); err != nil {
			skipped = append(skipped, brandScanSkippedEntry{Name: fh.Filename, Reason: err.Error()})
			continue
		}

		src, err := fh.Open()
		if err != nil {
			skipped = append(skipped, brandScanSkippedEntry{Name: fh.Filename, Reason: "unreadable file"})
			continue
		}
		data, err := io.ReadAll(io.LimitReader(src, brandscan.MaxFileBytes+1))
		_ = src.Close()
		if err != nil {
			skipped = append(skipped, brandScanSkippedEntry{Name: fh.Filename, Reason: "unreadable file"})
			continue
		}
		if int64(len(data)) > brandscan.MaxFileBytes {
			skipped = append(skipped, brandScanSkippedEntry{
				Name: fh.Filename, Reason: "file too large (max 10 MiB)",
			})
			continue
		}

		envelope, err := json.Marshal(jobs.BrandScanUploadEnvelope{
			Filename:    fh.Filename,
			ContentType: contentType,
			Data:        data,
		})
		if err != nil {
			skipped = append(skipped, brandScanSkippedEntry{Name: fh.Filename, Reason: "failed to encode upload"})
			continue
		}
		ref, err := s.BlobStore.Upload(ctx, envelope, corestorage.UploadOptions{
			ContentType: "application/json",
			Filename:    fh.Filename,
		})
		if err != nil {
			skipped = append(skipped, brandScanSkippedEntry{Name: fh.Filename, Reason: "failed to store upload"})
			continue
		}
		uploads = append(uploads, brandScanUploadEntry{
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

// HandleCreateBrandScan enqueues an async AI brand scan (epic 016).
// POST /api/v1/:ws/brand-scans
//
// Route middleware applies the per-IP AI throttle and billing.QuotaGuard; the
// handler additionally requires PermManageBrand and refuses a scan up front
// when the workspace has no spendable platform credits (402) — scans always
// run on the platform key, and deduction is post-hoc.
func (s *Server) HandleCreateBrandScan(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermManageBrand); err != nil {
		return err
	}
	if s.BrandScanStore == nil || s.BrandScanQueue == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "brand scan system not configured"})
	}

	var req jobs.BrandScanRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	}
	if !req.HasSource() {
		return c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: "at least one source is required: paste_text, urls, upload_keys, or repo_url",
		})
	}
	if len(req.URLs) > jobs.MaxBrandScanURLs {
		return c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: "too many urls: a brand scan accepts at most 5 links",
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

	job := &jobs.BrandScanJob{
		ID:            id.New(),
		WorkspaceID:   wsID,
		WorkspaceSlug: c.Param("ws"),
		Status:        jobs.BrandScanStatusQueued,
		Request:       requestJSON,
	}
	if err := s.BrandScanStore.CreateBrandScanJob(ctx, job); err != nil {
		return serverErr(c, err)
	}
	if err := s.BrandScanQueue.Enqueue(ctx, job.ID); err != nil {
		// Roll back the job record so no orphaned queued row lingers.
		_ = s.BrandScanStore.DeleteBrandScanJob(ctx, job.ID)
		return serverErr(c, fmt.Errorf("enqueue failed: %w", err))
	}

	return c.JSON(http.StatusAccepted, map[string]any{
		"job_id": job.ID,
		"status": string(job.Status),
	})
}

// brandScanInRequestWorkspace reports whether the brand-scan job belongs to
// the workspace the request is scoped to. GetBrandScanJob resolves by GLOBAL
// id, so without this a caller could read another tenant's scan (sources,
// draft profile, token spend) via /api/v1/<their-ws>/brand-scans/<victim-id>.
// 404 (anti-enumeration) on mismatch, mirroring jobInRequestWorkspace.
func brandScanInRequestWorkspace(c echo.Context, job *jobs.BrandScanJob) bool {
	ws := c.Param("ws")
	return ws != "" && job != nil && job.WorkspaceSlug == ws
}

// HandleGetBrandScan returns the status, progress, and (when completed) the
// draft result of a brand scan.
// GET /api/v1/:ws/brand-scans/:id
func (s *Server) HandleGetBrandScan(c echo.Context) error {
	if s.BrandScanStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "brand scan system not configured"})
	}

	job, err := s.BrandScanStore.GetBrandScanJob(c.Request().Context(), c.Param("id"))
	if err != nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: "brand scan not found"})
	}
	if !brandScanInRequestWorkspace(c, job) {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: "brand scan not found"})
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
	if job.Status == jobs.BrandScanStatusCompleted && len(job.Result) > 0 {
		resp["draft"] = job.Result
	}
	return c.JSON(http.StatusOK, resp)
}

// BrandScanApprovedTerm is one candidate term a reviewer kept. Locale falls
// back to the request's Locale, then to "en".
type BrandScanApprovedTerm struct {
	Term       string `json:"term"`
	Definition string `json:"definition,omitempty"`
	Domain     string `json:"domain,omitempty"`
	Locale     string `json:"locale,omitempty"`
}

// BrandScanApproveRequest is the reviewed outcome of a brand scan: the edited
// draft profile and the candidate terms that survived review.
type BrandScanApproveRequest struct {
	Profile BrandProfileRequest     `json:"profile"`
	Terms   []BrandScanApprovedTerm `json:"terms,omitempty"`
	// Locale is the locale approved terms are created in when a term does not
	// name its own. Defaults to "en".
	Locale string `json:"locale,omitempty"`
}

// BrandScanApproveResponse reports what the approval applied: the stored
// profile and which action produced it, plus the concepts created and the ones
// that were already there.
type BrandScanApproveResponse struct {
	Profile          *coreprofile.VoiceProfile `json:"profile"`
	ProfileAction    string                    `json:"profile_action"`
	ConceptsCreated  int                       `json:"concepts_created"`
	ConceptsExisting int                       `json:"concepts_existing"`
	ConceptIDs       []string                  `json:"concept_ids"`
}

// HandleApproveBrandScan applies a reviewed brand scan in one request: the
// profile and every approved term.
// POST /api/v1/:ws/brand-scans/:id/approve
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
func (s *Server) HandleApproveBrandScan(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermManageBrand); err != nil {
		return err
	}
	if s.BrandScanStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "brand scan system not configured"})
	}
	if s.BrandStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "brand voice not configured"})
	}
	if s.wsStores == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "editor not configured"})
	}

	ctx := c.Request().Context()
	job, err := s.BrandScanStore.GetBrandScanJob(ctx, c.Param("id"))
	if err != nil || !brandScanInRequestWorkspace(c, job) {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: "brand scan not found"})
	}
	if job.Status != jobs.BrandScanStatusCompleted {
		return c.JSON(http.StatusConflict, ErrorResponse{
			Error: fmt.Sprintf("only a completed brand scan can be approved (status %q)", job.Status),
		})
	}

	var req BrandScanApproveRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	}
	if strings.TrimSpace(req.Profile.Name) == "" {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "profile.name is required"})
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

	tb, err := s.wsStores.getTB(c.Param("ws"))
	if err != nil {
		return serverErrStatus(c, http.StatusServiceUnavailable, err)
	}

	wsID, _ := c.Get("workspace_id").(string)
	actor, _ := c.Get("user_id").(string)
	upsert, err := s.upsertBrandProfile(ctx, wsID, actor, req.Profile, "superseded by a brand-scan approval")
	if err != nil {
		return serverErr(c, err)
	}

	resp := BrandScanApproveResponse{
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
func (s *Server) partialApproval(c echo.Context, resp BrandScanApproveResponse, cause error) error {
	slog.ErrorContext(c.Request().Context(), "brand-scan approval stopped part-way",
		"scan_id", c.Param("id"), "concepts_created", resp.ConceptsCreated,
		"reference", requestID(c), "error", cause)
	return c.JSON(http.StatusInternalServerError, map[string]any{
		"error":   "the approval was partially applied; retry it — the profile and the terms already stored are left as they are",
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

// BrandDraftCheckRequest is the request body for the stateless draft tester:
// an (unsaved, possibly user-edited) draft profile plus sample text.
type BrandDraftCheckRequest struct {
	Profile *coreprofile.VoiceProfile `json:"profile"`
	Text    string                    `json:"text"`
}

// HandleCheckBrandDraft scores sample text against an inline draft voice
// profile — the zero-AI-cost live tester for the scan review surface.
// POST /api/v1/:ws/brand-scans/check-draft
//
// It mirrors HandleCheckBrandVoice but takes the profile in the request body
// instead of loading a stored one, so a draft can be tested (and refined)
// before it is ever persisted. Deterministic matcher only — no provider
// call, hence no QuotaGuard.
func (s *Server) HandleCheckBrandDraft(c echo.Context) error {
	var req BrandDraftCheckRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	}
	if req.Text == "" {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "text is required"})
	}
	if req.Profile == nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "profile is required"})
	}

	// Same shared matcher + mapper as the stored-profile check: whole-word,
	// Unicode-aware vocabulary matching anchored to a single text run.
	runs := []model.Run{{Text: &model.TextRun{Text: req.Text}}}
	findings := coreprofile.HitsToFindings(coreprofile.MatchVocabulary(req.Profile, req.Text), req.Text, runs)
	if findings == nil {
		findings = []coreprofile.VoiceFinding{}
	}
	score := coreprofile.CalculateScore(findings)

	return c.JSON(http.StatusOK, BrandCheckResponse{
		Score:    score,
		Findings: findings,
	})
}
