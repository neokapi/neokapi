package server

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/neokapi/neokapi/bowrain/analytics"
	"github.com/neokapi/neokapi/bowrain/billing"
	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	"github.com/neokapi/neokapi/bowrain/core/store"
	bstore "github.com/neokapi/neokapi/bowrain/store"
	"github.com/neokapi/neokapi/core/id"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/memory"
	aiprovider "github.com/neokapi/neokapi/providers/ai"
)

// HandleGetEditorProject returns an editor project. The default response is the
// full detail view, with every item embedded.
//
// `?view=summary` returns the same shape with an empty items[] — the project's
// metadata, collections, streams and aggregates. Embedding the items costs a
// block read per item, so a surface that only needs the project's name,
// locales, streams or collections asks for the summary; the item list itself is
// served, paged and sorted, by the dashboard endpoint. The default is the full
// view because that is the shape existing clients (the Wails desktop, the Go
// editor client) already read.
func (s *Server) HandleGetEditorProject(c echo.Context) error {
	if s.ContentStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "editor not configured"})
	}

	pid := projectParam(c)
	ctx := c.Request().Context()

	proj, err := s.ContentStore.GetProject(ctx, pid)
	if err != nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: err.Error()})
	}

	build := editorBuildProjectInfo
	if c.QueryParam("view") == "summary" {
		build = editorBuildProjectSummaryInfo
	}
	info, err := build(ctx, s.ContentStore, proj, streamParamWithProject(c, proj))
	if err != nil {
		return serverErr(c, err)
	}

	wsID, _ := c.Get("workspace_id").(string)
	s.annotateProjectOrigin(ctx, wsID, info)
	return c.JSON(http.StatusOK, info)
}

// HandleUpdateEditorProject updates a project's name and locales.
func (s *Server) HandleUpdateEditorProject(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermManageProject); err != nil {
		return err
	}

	if s.ContentStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "editor not configured"})
	}

	pid := projectParam(c)
	ctx := c.Request().Context()

	proj, err := s.ContentStore.GetProject(ctx, pid)
	if err != nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: err.Error()})
	}

	var req struct {
		Name                string            `json:"name"`
		TargetLanguages     []string          `json:"target_languages"`
		DefaultStream       *string           `json:"default_stream,omitempty"`
		DashboardVisibility string            `json:"dashboard_visibility,omitempty"`
		Properties          map[string]string `json:"properties,omitempty"`
	}
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	}

	if req.Name != "" {
		proj.Name = req.Name
	}
	if req.TargetLanguages != nil {
		locales := make([]model.LocaleID, len(req.TargetLanguages))
		for i, l := range req.TargetLanguages {
			locales[i] = model.LocaleID(l)
		}
		proj.TargetLanguages = locales
	}
	if req.DefaultStream != nil {
		proj.DefaultStream = *req.DefaultStream
	}
	if req.DashboardVisibility != "" {
		if req.DashboardVisibility != string(platauth.DashboardPrivate) && s.AuthStore != nil {
			if ws, wsErr := s.AuthStore.GetWorkspace(ctx, proj.WorkspaceID); wsErr == nil && ws.Type == platauth.WorkspaceTypePersonal {
				return c.JSON(http.StatusForbidden, ErrorResponse{Error: "personal workspaces cannot expose projects publicly"})
			}
		}
		proj.DashboardVisibility = req.DashboardVisibility
	}
	if req.Properties != nil {
		// Turning on the public ship feed is the same disclosure decision as a
		// public dashboard, and properties are written raw, so the personal-
		// workspace refusal has to be made here rather than only on the field
		// that reads more like a setting.
		if req.Properties[ShipFeedProperty] == "true" && s.AuthStore != nil {
			if ws, wsErr := s.AuthStore.GetWorkspace(ctx, proj.WorkspaceID); wsErr == nil && ws.Type == platauth.WorkspaceTypePersonal {
				return c.JSON(http.StatusForbidden, ErrorResponse{Error: "personal workspaces cannot expose projects publicly"})
			}
		}
		if proj.Properties == nil {
			proj.Properties = make(map[string]string)
		}
		maps.Copy(proj.Properties, req.Properties)
	}

	if err := s.ContentStore.UpdateProject(ctx, proj); err != nil {
		return serverErr(c, err)
	}

	info, err := editorBuildProjectInfo(ctx, s.ContentStore, proj, streamParamWithProject(c, proj))
	if err != nil {
		return serverErr(c, err)
	}

	wsID, _ := c.Get("workspace_id").(string)
	s.annotateProjectOrigin(ctx, wsID, info)
	return c.JSON(http.StatusOK, info)
}

// HandleUpdateStreamName renames a stream's description.
func (s *Server) HandleUpdateStreamName(c echo.Context) error {
	if s.ContentStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "store not configured"})
	}

	projectID := c.Param("id")
	streamName := c.Param("stream")
	ctx := c.Request().Context()

	stream, err := s.ContentStore.GetStream(ctx, projectID, streamName)
	if err != nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: err.Error()})
	}

	var req struct {
		Description string `json:"description"`
		Visibility  string `json:"visibility"`
	}
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	}

	if req.Description != "" {
		stream.Description = req.Description
	}
	if req.Visibility != "" {
		stream.Visibility = store.StreamVisibility(req.Visibility)
	}

	if err := s.ContentStore.UpdateStream(ctx, stream); err != nil {
		return serverErr(c, err)
	}

	return c.JSON(http.StatusOK, stream)
}

// HandleDeleteEditorProject archives a project (soft delete).
func (s *Server) HandleDeleteEditorProject(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermManageProject); err != nil {
		return err
	}

	if s.ContentStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "editor not configured"})
	}

	pid := projectParam(c)
	if err := s.ContentStore.ArchiveProject(c.Request().Context(), pid); err != nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: err.Error()})
	}

	// Archiving is the delete a person performs; permanent deletion empties the
	// recycle bin afterwards. The funnel counts the first, so a project is
	// counted deleted once whether or not its bytes are later dropped.
	wsID, _ := c.Get("workspace_id").(string)
	userID, _ := c.Get("user_id").(string)
	s.trackEvent(userID, analytics.EventProjectDeleted, analytics.Props(wsID, pid))

	return c.NoContent(http.StatusNoContent)
}

// HandleRestoreProject restores an archived project.
func (s *Server) HandleRestoreProject(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermManageProject); err != nil {
		return err
	}

	if s.ContentStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "editor not configured"})
	}

	pid := projectParam(c)
	if err := s.ContentStore.RestoreProject(c.Request().Context(), pid); err != nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: err.Error()})
	}

	return c.NoContent(http.StatusNoContent)
}

// HandleRestoreStream restores an archived stream.
func (s *Server) HandleRestoreStream(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermManageStreams); err != nil {
		return err
	}

	if s.ContentStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "store not configured"})
	}

	projectID := c.Param("id")
	streamName := c.Param("stream")
	ctx := c.Request().Context()

	stream, err := s.ContentStore.GetStream(ctx, projectID, streamName)
	if err != nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: err.Error()})
	}

	stream.Archived = false
	if err := s.ContentStore.UpdateStream(ctx, stream); err != nil {
		return serverErr(c, err)
	}

	return c.NoContent(http.StatusNoContent)
}

// HandleListArchivedProjects lists archived projects in a workspace (the "recycle bin").
func (s *Server) HandleListArchivedProjects(c echo.Context) error {
	if s.ContentStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "store not configured"})
	}

	wsID, _ := c.Get("workspace_id").(string)
	projects, err := s.ContentStore.ListArchivedProjects(c.Request().Context(), wsID)
	if err != nil {
		return serverErr(c, err)
	}

	return c.JSON(http.StatusOK, projects)
}

// HandlePermanentlyDeleteProject permanently deletes an archived project.
func (s *Server) HandlePermanentlyDeleteProject(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermManageProject); err != nil {
		return err
	}

	if s.ContentStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "editor not configured"})
	}

	pid := projectParam(c)
	ctx := c.Request().Context()

	// Only allow permanent deletion of archived projects.
	proj, err := s.ContentStore.GetProject(ctx, pid)
	if err != nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: err.Error()})
	}
	if !proj.Archived {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "project must be archived before permanent deletion"})
	}

	if err := s.ContentStore.DeleteProject(ctx, pid); err != nil {
		return serverErr(c, err)
	}

	return c.NoContent(http.StatusNoContent)
}

// HandleUploadFiles uploads files to a project via multipart form.
func (s *Server) HandleUploadFiles(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermManageFiles); err != nil {
		return err
	}

	if s.ContentStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "editor not configured"})
	}

	pid := projectParam(c)
	ctx := c.Request().Context()

	// Refuse the upload when the project's source is connector-sourced (kapi
	// push / GitHub App / git): the repository owns the source there, and an
	// upload would be overwritten on the next sync. A source connector makes the
	// whole project read-only; otherwise a project-level upload lands in the
	// default collection, so gate on that collection's origin.
	defColl, _ := s.ContentStore.GetDefaultCollection(ctx, pid)
	if refused, rerr := s.guardSourceMutation(c, pid, defColl); refused {
		return rerr
	}

	form, err := c.MultipartForm()
	if err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "multipart form required"})
	}

	files, answered, err := readMultipartUploads(c, form)
	if answered {
		return err
	}

	info, err := editorAddFiles(ctx, s.ContentStore, s.FormatRegistry, pid, streamParam(c), files)
	if err != nil {
		return serverErr(c, err)
	}

	wsID, _ := c.Get("workspace_id").(string)
	s.invalidateDashboardCache(wsID, pid)
	s.annotateProjectOrigin(ctx, wsID, info)
	return c.JSON(http.StatusOK, info)
}

// HandleRemoveFile removes a file from a project.
func (s *Server) HandleRemoveFile(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermManageFiles); err != nil {
		return err
	}

	if s.ContentStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "editor not configured"})
	}

	pid := projectParam(c)
	fname := fileParam(c)
	ctx := c.Request().Context()
	stream := streamParam(c)

	// Refuse the delete when the item's source is connector-sourced: removing a
	// synced file here only reappears on the next sync. Gate on the item's own
	// collection (covers a connector-backed collection) plus the project-level
	// source-connector signal.
	itemColl := s.collectionForItem(ctx, pid, stream, fname)
	if refused, rerr := s.guardSourceMutation(c, pid, itemColl); refused {
		return rerr
	}

	info, err := editorRemoveFile(ctx, s.ContentStore, pid, stream, fname)
	if err != nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: err.Error()})
	}

	wsID, _ := c.Get("workspace_id").(string)
	s.invalidateDashboardCache(wsID, pid)
	s.annotateProjectOrigin(ctx, wsID, info)
	return c.JSON(http.StatusOK, info)
}

// blockQueryFromRequest reads the block filters off the query string:
// item, locale, status (a per-locale bucket), q (case-insensitive substring
// over source and the locale's target), translatable, limit and offset.
// An unknown status is a 400 rather than a silently empty page.
func blockQueryFromRequest(c echo.Context, pid string) (store.BlockQuery, error) {
	q := store.BlockQuery{
		ProjectID:    pid,
		Stream:       streamParam(c),
		ItemName:     fileParam(c),
		TargetLocale: c.QueryParam("locale"),
		Text:         strings.TrimSpace(c.QueryParam("q")),
	}
	if raw := c.QueryParam("status"); raw != "" && raw != "all" {
		if !slices.Contains(store.BlockStatusBuckets(), raw) {
			return q, fmt.Errorf("status must be one of %s", strings.Join(store.BlockStatusBuckets(), ", "))
		}
		if q.TargetLocale == "" {
			return q, errors.New("status requires a locale")
		}
		q.Status = raw
	}
	if raw := c.QueryParam("translatable"); raw != "" {
		v, err := strconv.ParseBool(raw)
		if err != nil {
			return q, errors.New("translatable must be true or false")
		}
		q.Translatable = &v
	}
	return q, nil
}

// HandleGetFileBlocks returns a page of a project's blocks, narrowed by the
// query string.
func (s *Server) HandleGetFileBlocks(c echo.Context) error {
	if s.ContentStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "editor not configured"})
	}

	pid := projectParam(c)
	ctx := c.Request().Context()

	targetLocales, err := s.projectTargetLocales(ctx, pid)
	if err != nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: err.Error()})
	}

	query, err := blockQueryFromRequest(c, pid)
	if err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	}
	// Clamp the page: without a file wildcard on this route ItemName is often
	// empty, which reduces the query to WHERE project_id = ? — a full-project
	// hydrate-and-serialize (17k blocks at dogfood scale). Bound it like the
	// sync block endpoint does.
	query.Limit, query.Offset = pageParams(c, store.DefaultBlockLimit, store.DefaultBlockLimit)

	blocks, err := editorQueryBlocks(ctx, s.ContentStore, query, targetLocales)
	if err != nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: err.Error()})
	}

	return c.JSON(http.StatusOK, blocks)
}

// projectTargetLocales names a project's target languages as plain strings —
// what the block payload keys its `targets` map by.
func (s *Server) projectTargetLocales(ctx context.Context, pid string) ([]string, error) {
	proj, err := s.ContentStore.GetProject(ctx, pid)
	if err != nil {
		return nil, err
	}
	locales := make([]string, len(proj.TargetLanguages))
	for i, l := range proj.TargetLanguages {
		locales[i] = string(l)
	}
	return locales, nil
}

// HandleGetBlock returns one block in the same shape the list route returns its
// elements: GET /:ws/:id/blocks/:ref/:bid.
//
// Every other single-block operation already addresses a block this way
// (/history, /notes, /term-matches, /html, and the PUTs), so reading the block
// itself was the one gap. It exists because a client that has just written a
// target had no way to ask what the server now holds: it either re-fetched the
// whole item's blocks or rebuilt the block from its own request plus a local
// copy of the server's demotion rule. The second is what `ReviewSession`
// did — correct only for as long as the two copies of the rule agree.
func (s *Server) HandleGetBlock(c echo.Context) error {
	if s.ContentStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "editor not configured"})
	}

	pid := projectParam(c)
	ctx := c.Request().Context()

	targetLocales, err := s.projectTargetLocales(ctx, pid)
	if err != nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: err.Error()})
	}

	sb, err := s.ContentStore.GetBlock(ctx, pid, streamParam(c), c.Param("bid"))
	if err != nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: err.Error()})
	}
	return c.JSON(http.StatusOK, storedBlockToInfoResponse(sb, targetLocales))
}

// BlockStatusCountsResponse is the per-locale status histogram, keyed as the
// editor names its buckets.
type BlockStatusCountsResponse struct {
	NotStarted int `json:"not-started"`
	Draft      int `json:"draft"`
	Translated int `json:"translated"`
	Reviewed   int `json:"reviewed"`
}

// BlockCountsResponse summarizes a block query. Status partitions
// Translatable for the requested locale; with no locale it is all zeros.
type BlockCountsResponse struct {
	Total        int                       `json:"total"`
	Translatable int                       `json:"translatable"`
	Locale       string                    `json:"locale,omitempty"`
	Status       BlockStatusCountsResponse `json:"status"`
}

// HandleGetBlockCounts answers a file's progress with one aggregate query. It
// takes the same filters as the blocks route (item, locale, q, translatable)
// minus status, which is the histogram it reports.
//
// GET /:ws/:id/blocks/:ref/counts?item=file.json&locale=fr
func (s *Server) HandleGetBlockCounts(c echo.Context) error {
	if s.ContentStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "editor not configured"})
	}

	query, err := blockQueryFromRequest(c, projectParam(c))
	if err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	}
	query.Status = ""

	counts, err := s.ContentStore.CountBlocks(c.Request().Context(), query)
	if err != nil {
		return serverErr(c, err)
	}
	return c.JSON(http.StatusOK, BlockCountsResponse{
		Total:        counts.Total,
		Translatable: counts.Translatable,
		Locale:       query.TargetLocale,
		Status: BlockStatusCountsResponse{
			NotStarted: counts.NotStarted,
			Draft:      counts.Draft,
			Translated: counts.Translated,
			Reviewed:   counts.Reviewed,
		},
	})
}

// ItemInfoResponse is one item's metadata plus its block tallies — what a
// surface needs to name and size a file without the project's whole item
// list. Word count is not part of it: /:ws/:id/word-count/:ref answers that.
type ItemInfoResponse struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Format       string `json:"format"`
	Type         string `json:"type"`
	CollectionID string `json:"collection_id,omitempty"`
	BlockCount   int    `json:"block_count"`
	Translatable int    `json:"translatable"`
}

// HandleGetItem returns one item's metadata. The item name is a query
// parameter because names carry slashes, and the list route already owns
// /items/:ref.
//
// GET /:ws/:id/items/:ref/one?item=path/to/file.json
func (s *Server) HandleGetItem(c echo.Context) error {
	if s.ContentStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "editor not configured"})
	}

	pid := projectParam(c)
	fname := fileParam(c)
	if fname == "" {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "item is required"})
	}
	ctx := c.Request().Context()
	stream := streamParam(c)

	item, err := s.ContentStore.GetItem(ctx, pid, stream, fname)
	if err != nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: err.Error()})
	}
	counts, err := s.ContentStore.CountBlocks(ctx, store.BlockQuery{
		ProjectID: pid, Stream: stream, ItemName: item.Name,
	})
	if err != nil {
		return serverErr(c, err)
	}

	return c.JSON(http.StatusOK, ItemInfoResponse{
		ID:           item.ID,
		Name:         item.Name,
		Format:       item.Format,
		Type:         item.ItemType,
		CollectionID: item.CollectionID,
		BlockCount:   counts.Total,
		Translatable: counts.Translatable,
	})
}

// HandleUpdateBlockTarget updates the target text for a block.
func (s *Server) HandleUpdateBlockTarget(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermTranslate); err != nil {
		return err
	}

	if s.ContentStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "editor not configured"})
	}

	pid := projectParam(c)
	bid := c.Param("bid")

	var req UpdateBlockTargetRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	}
	if err := s.requireLanguagePermission(c, platauth.PermTranslate, req.TargetLocale); err != nil {
		return err
	}
	// ABAC: editing in-review/published content is gated by status + ownership.
	if err := s.requireEditableStatus(c, pid, bid, req.TargetLocale); err != nil {
		return err
	}

	if err := editorUpdateBlockTarget(c.Request().Context(), s.ContentStore, pid, streamParam(c), bid, req); err != nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: err.Error()})
	}

	wsID, _ := c.Get("workspace_id").(string)
	s.invalidateDashboardCache(wsID, pid)

	userID, _ := c.Get("user_id").(string)
	s.trackEvent(userID, "translation_saved", map[string]any{
		"project_id": pid,
		"block_id":   bid,
		"locale":     req.TargetLocale,
	})

	return c.NoContent(http.StatusNoContent)
}

// HandleUpdateBlockTargetRuns updates a block target from a Run sequence.
func (s *Server) HandleUpdateBlockTargetRuns(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermTranslate); err != nil {
		return err
	}

	if s.ContentStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "editor not configured"})
	}

	pid := projectParam(c)
	bid := c.Param("bid")

	var req UpdateBlockTargetRunsRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	}
	if err := s.requireLanguagePermission(c, platauth.PermTranslate, req.TargetLocale); err != nil {
		return err
	}
	if err := s.requireEditableStatus(c, pid, bid, req.TargetLocale); err != nil {
		return err
	}

	if err := editorUpdateBlockTargetRuns(c.Request().Context(), s.ContentStore, pid, streamParam(c), bid, req); err != nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: err.Error()})
	}

	wsID, _ := c.Get("workspace_id").(string)
	s.invalidateDashboardCache(wsID, pid)
	return c.NoContent(http.StatusNoContent)
}

// HandlePseudoTranslate pseudo-translates all blocks in a file.
func (s *Server) HandlePseudoTranslate(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermTranslate); err != nil {
		return err
	}

	if s.ContentStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "editor not configured"})
	}

	pid := projectParam(c)
	fname := fileParam(c)

	var req struct {
		TargetLocale string `json:"target_locale"`
	}
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	}
	if err := s.requireLanguagePermission(c, platauth.PermTranslate, req.TargetLocale); err != nil {
		return err
	}

	stats, err := editorPseudoTranslate(c.Request().Context(), s.ContentStore, pid, streamParam(c), fname, req.TargetLocale)
	if err != nil {
		return serverErr(c, err)
	}

	wsID, _ := c.Get("workspace_id").(string)
	s.invalidateDashboardCache(wsID, pid)
	return c.JSON(http.StatusOK, stats)
}

// HandleAITranslate translates all blocks using an AI provider.
func (s *Server) HandleAITranslate(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermTranslate); err != nil {
		return err
	}

	if s.ContentStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "editor not configured"})
	}

	pid := projectParam(c)
	fname := fileParam(c)

	var req TranslateRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	}
	if err := s.requireLanguagePermission(c, platauth.PermTranslate, req.TargetLocale); err != nil {
		return err
	}

	// Weekly-credit gate (Epic 004), BYO-aware. This synchronous route is not
	// wrapped by QuotaGuard because the middleware cannot see the request body:
	// a workspace out of platform credits but carrying its own key (saved
	// provider_config_id or inline api_key) must still be allowed, mirroring the
	// async enqueue pre-check. BYO burns no credits, so it is never gated.
	byo := (req.ProviderConfigID != "" && req.ProviderConfigID != "platform") || req.APIKey != ""
	if err := billing.GuardSyncCredits(c, s.BillingStore, byo, s.billingGuardEvent()); err != nil {
		return err
	}

	// Bind the project's standing brand context from the server's own stores —
	// the same instances the voice and terminology surfaces use. Everything in
	// it is optional: missing stores mean a bare translation, never an error.
	voiceCtx := s.editorVoiceContext()

	wsID, _ := c.Get("workspace_id").(string)
	stats, err := editorAITranslate(c.Request().Context(), s.ContentStore, s.ProviderStore, s.QuotaStore,
		pid, streamParam(c), fname, req, s.BillingHooks, wsID, c.Param("ws"),
		s.platformProviderConfigForWorkspace(c.Request().Context(), wsID), voiceCtx)
	if err != nil {
		// The provider is circuit-broken: nothing was sent, no credits were
		// spent. Say so with a typed 503 the editor renders as "queued, will
		// retry" rather than a red failure the user cannot act on.
		if resp, ok := unavailableErr(c, err); ok {
			return resp
		}
		return serverErr(c, err)
	}

	s.invalidateDashboardCache(wsID, pid)
	return c.JSON(http.StatusOK, stats)
}

// HandleMemoryTranslate leverages content memory to translate blocks.
func (s *Server) HandleMemoryTranslate(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermTranslate); err != nil {
		return err
	}

	if s.ContentStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "editor not configured"})
	}

	ws := c.Param("ws")
	pid := projectParam(c)
	fname := fileParam(c)

	var req struct {
		TargetLocale string `json:"target_locale"`
	}
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	}
	if err := s.requireLanguagePermission(c, platauth.PermTranslate, req.TargetLocale); err != nil {
		return err
	}

	stats, err := editorMemoryTranslate(c.Request().Context(), s.ContentStore, s.wsStores, ws, pid, streamParam(c), fname, req.TargetLocale)
	if err != nil {
		return serverErr(c, err)
	}

	wsID, _ := c.Get("workspace_id").(string)
	s.invalidateDashboardCache(wsID, pid)
	return c.JSON(http.StatusOK, stats)
}

// HandleTermEnforce runs terminology enforcement over an item's blocks and
// returns the violations. It is a read-only check owned by the server via the
// framework term-enforce tool.
func (s *Server) HandleTermEnforce(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermViewContent); err != nil {
		return err
	}

	if s.ContentStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "editor not configured"})
	}

	ws := c.Param("ws")
	pid := projectParam(c)
	fname := fileParam(c)

	var req struct {
		TargetLocale string `json:"target_locale"`
	}
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	}
	if req.TargetLocale == "" {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "target_locale is required"})
	}

	results, err := editorTermEnforce(c.Request().Context(), s.ContentStore, s.wsStores, ws, pid, streamParam(c), fname, req.TargetLocale)
	if err != nil {
		return serverErr(c, err)
	}
	if results == nil {
		results = []TermEnforceResultResponse{}
	}
	return c.JSON(http.StatusOK, results)
}

// HandleGetWordCount returns word and character counts for a file.
func (s *Server) HandleGetWordCount(c echo.Context) error {
	if s.ContentStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "editor not configured"})
	}

	pid := projectParam(c)
	fname := fileParam(c)
	ctx := c.Request().Context()

	proj, err := s.ContentStore.GetProject(ctx, pid)
	if err != nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: err.Error()})
	}

	targetLocales := make([]string, len(proj.TargetLanguages))
	for i, l := range proj.TargetLanguages {
		targetLocales[i] = string(l)
	}

	result, err := editorGetWordCount(ctx, s.ContentStore, pid, streamParam(c), fname, targetLocales)
	if err != nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: err.Error()})
	}

	return c.JSON(http.StatusOK, result)
}

// HandleExportTranslatedFile answers the editor's export request.
//
// Server-side merged-file export is not available and will not become
// available: source bytes stopped being stored in the Item model in #136, so
// there is nothing to merge into. `kapi pull` is the export path.
//
// That refusal is permanent and expected, so it must not travel as a 500. It
// used to: the stub error went through serverErr, which rendered "the server
// hit an unexpected error", buried the sentence naming the alternative in the
// server log, and paged the on-call for a working endpoint. 501 states the
// condition plainly and puts the sentence in the body where the caller reads
// it.
func (s *Server) HandleExportTranslatedFile(c echo.Context) error {
	if s.ContentStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "editor not configured"})
	}
	return c.JSON(http.StatusNotImplemented, ErrorResponse{
		Error: "server-side file export is not available: use 'kapi pull' to export translated files",
	})
}

// HandleLookupMemoryForBlock looks up content-memory matches for a specific block.
func (s *Server) HandleLookupMemoryForBlock(c echo.Context) error {
	if s.ContentStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "editor not configured"})
	}

	ws := c.Param("ws")
	pid := projectParam(c)
	bid := c.Param("bid")
	targetLocale := c.QueryParam("target_locale")

	matches, err := editorLookupMemoryForBlock(c.Request().Context(), s.ContentStore, s.wsStores, ws, pid, streamParam(c), bid, targetLocale)
	if err != nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: err.Error()})
	}

	if matches == nil {
		matches = []MemoryMatchInfoResponse{}
	}
	return c.JSON(http.StatusOK, matches)
}

// HandleLookupTermsForBlock looks up term matches for a specific block.
func (s *Server) HandleLookupTermsForBlock(c echo.Context) error {
	if s.ContentStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "editor not configured"})
	}

	ws := c.Param("ws")
	pid := projectParam(c)
	bid := c.Param("bid")
	targetLocale := c.QueryParam("target_locale")

	matches, err := editorLookupTermsForBlock(c.Request().Context(), s.ContentStore, s.wsStores, ws, pid, streamParam(c), bid, targetLocale)
	if err != nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: err.Error()})
	}

	if matches == nil {
		matches = []BlockTermMatchResponse{}
	}
	return c.JSON(http.StatusOK, matches)
}

// HandleGetMemoryEntries searches content-memory entries.
func (s *Server) HandleGetMemoryEntries(c echo.Context) error {
	if s.wsStores == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "editor not configured"})
	}

	ws := c.Param("ws")
	query := c.QueryParam("q")
	sourceLocale := c.QueryParam("source_locale")
	targetLocale := c.QueryParam("target_locale")
	projectID := c.QueryParam("project_id")
	limit, offset := pageParams(c, 50, maxListPageSize)

	tm, err := s.wsStores.getMemory(ws)
	if err != nil {
		return serverErr(c, err)
	}

	stream := c.QueryParam("stream")
	ctx := c.Request().Context()
	var entries []memory.Entry
	var total int
	if stream != "" && stream != "main" && s.ContentStore != nil {
		pid := c.QueryParam("project_id")
		chain := buildStreamChain(ctx, s.ContentStore, pid, stream)
		entries, total, err = tm.SearchEntriesForStream(ctx, memory.SearchParams{
			Query:         query,
			AnyLocale:     sourceLocale,
			RequireLocale: targetLocale,
			Stream:        stream,
			StreamChain:   chain[1:],
			Offset:        offset,
			Limit:         limit,
		})
	} else {
		entries, total, err = tm.SearchEntries(ctx, memory.SearchParams{
			Query:         query,
			AnyLocale:     sourceLocale,
			RequireLocale: targetLocale,
			Offset:        offset,
			Limit:         limit,
		})
	}
	if err != nil {
		return serverErr(c, err)
	}

	// Post-filter by project_id if specified.
	if projectID != "" {
		filtered := make([]memory.Entry, 0, len(entries))
		for _, e := range entries {
			if e.ProjectID == projectID {
				filtered = append(filtered, e)
			}
		}
		entries = filtered
		total = len(filtered)
	}

	infos := make([]MemoryEntryInfoResponse, len(entries))
	for i, e := range entries {
		infos[i] = editorEntryToInfo(e, sourceLocale, targetLocale)
	}

	return c.JSON(http.StatusOK, MemorySearchResponse{Entries: infos, TotalCount: total})
}

// HandleGetMemoryCount returns the content-memory entry count.
func (s *Server) HandleGetMemoryCount(c echo.Context) error {
	if s.wsStores == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "editor not configured"})
	}

	ws := c.Param("ws")

	tm, err := s.wsStores.getMemory(ws)
	if err != nil {
		return serverErr(c, err)
	}

	count, err := tm.Count(c.Request().Context())
	if err != nil {
		return serverErr(c, err)
	}
	return c.JSON(http.StatusOK, map[string]int{"count": count})
}

// HandleAddMemoryEntry adds a new content-memory entry.
func (s *Server) HandleAddMemoryEntry(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermManageMemory); err != nil {
		return err
	}
	if s.wsStores == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "editor not configured"})
	}

	ws := c.Param("ws")

	var req MemoryAddRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	}

	tm, err := s.wsStores.getMemory(ws)
	if err != nil {
		return serverErr(c, err)
	}

	srcLoc := model.LocaleID(req.SourceLocale)
	tgtLoc := model.LocaleID(req.TargetLocale)
	entry := memory.Entry{
		ID: id.New(),
		Variants: map[model.LocaleID][]model.Run{
			srcLoc: {{Text: &model.TextRun{Text: req.Source}}},
			tgtLoc: {{Text: &model.TextRun{Text: req.Target}}},
		},
		HintSrcLang: srcLoc,
		ProjectID:   req.ProjectID,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	stream := streamParam(c)
	if stream != "" && stream != "main" {
		err = tm.AddWithStream(c.Request().Context(), entry, stream)
	} else {
		err = tm.Add(c.Request().Context(), entry)
	}
	if err != nil {
		return serverErr(c, err)
	}

	return c.JSON(http.StatusCreated, editorEntryToInfo(entry, req.SourceLocale, req.TargetLocale))
}

// HandleUpdateMemoryEntry updates an existing content-memory entry.
func (s *Server) HandleUpdateMemoryEntry(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermManageMemory); err != nil {
		return err
	}
	if s.wsStores == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "editor not configured"})
	}

	ws := c.Param("ws")
	eid := c.Param("eid")

	var req MemoryUpdateRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	}

	tm, err := s.wsStores.getMemory(ws)
	if err != nil {
		return serverErr(c, err)
	}

	existing, ok, err := tm.GetEntry(c.Request().Context(), eid)
	if err != nil {
		return serverErr(c, err)
	}
	if !ok {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: fmt.Sprintf("content-memory entry %q not found", eid)})
	}

	// Delete old and add updated.
	if err := tm.Delete(c.Request().Context(), eid); err != nil {
		return serverErr(c, err)
	}

	srcLoc := model.LocaleID(req.SourceLocale)
	tgtLoc := model.LocaleID(req.TargetLocale)
	if existing.Variants == nil {
		existing.Variants = make(map[model.LocaleID][]model.Run)
	}
	existing.Variants[srcLoc] = []model.Run{{Text: &model.TextRun{Text: req.Source}}}
	existing.Variants[tgtLoc] = []model.Run{{Text: &model.TextRun{Text: req.Target}}}
	if existing.HintSrcLang == "" {
		existing.HintSrcLang = srcLoc
	}
	existing.UpdatedAt = time.Now()

	if err := tm.Add(c.Request().Context(), existing); err != nil {
		return serverErr(c, err)
	}

	return c.NoContent(http.StatusNoContent)
}

// HandleDeleteMemoryEntry deletes a content-memory entry.
func (s *Server) HandleDeleteMemoryEntry(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermManageMemory); err != nil {
		return err
	}
	if s.wsStores == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "editor not configured"})
	}

	ws := c.Param("ws")
	eid := c.Param("eid")

	tm, err := s.wsStores.getMemory(ws)
	if err != nil {
		return serverErr(c, err)
	}

	if err := tm.Delete(c.Request().Context(), eid); err != nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: err.Error()})
	}

	return c.NoContent(http.StatusNoContent)
}

// providerStoreToResponse projects a stored provider config onto the API view
// (never the API key). Epic 004: the provider store is workspace-scoped Postgres.
func providerStoreToResponse(c bstore.ProviderConfig) ProviderConfigResponse {
	return ProviderConfigResponse{
		ID:           c.ID,
		Name:         c.Name,
		ProviderType: c.Type,
		Model:        c.Model,
		BaseURL:      c.BaseURL,
	}
}

// HandleListProviderConfigs lists the calling workspace's saved AI provider
// configurations (Epic 004). Scoped to the :ws workspace; a read is gated by
// PermManageConnectors (the same capability that governs writes) so provider
// settings never leak across tenants. API keys are never returned.
func (s *Server) HandleListProviderConfigs(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermManageConnectors); err != nil {
		return err
	}
	if s.ProviderStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "providers not configured"})
	}
	wsID, _ := c.Get("workspace_id").(string)
	if wsID == "" {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "workspace not resolved"})
	}

	configs, err := s.ProviderStore.List(c.Request().Context(), wsID)
	if err != nil {
		return serverErr(c, err)
	}
	out := make([]ProviderConfigResponse, len(configs))
	for i, cfg := range configs {
		out[i] = providerStoreToResponse(cfg)
	}
	return c.JSON(http.StatusOK, out)
}

// HandleSaveProviderConfig creates or updates a provider configuration for the
// calling workspace (Epic 004). The API key is sealed at rest in Postgres; an
// empty api_key on update leaves the stored key unchanged. This no longer writes
// to the OS keychain, so it works in a headless production container.
func (s *Server) HandleSaveProviderConfig(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermManageConnectors); err != nil {
		return err
	}
	if s.ProviderStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "providers not configured"})
	}
	wsID, _ := c.Get("workspace_id").(string)
	if wsID == "" {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "workspace not resolved"})
	}

	var req SaveProviderConfigRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	}
	if req.Name == "" {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "name is required"})
	}

	saved, err := s.ProviderStore.Upsert(c.Request().Context(), &bstore.ProviderConfig{
		ID:            req.ID,
		WorkspaceID:   wsID,
		WorkspaceSlug: c.Param("ws"),
		Name:          req.Name,
		Type:          req.ProviderType,
		Model:         req.Model,
		BaseURL:       req.BaseURL,
		APIKey:        req.APIKey, // sealed on write; empty preserves the stored key
	})
	if err != nil {
		return serverErr(c, fmt.Errorf("save provider config: %w", err))
	}
	return c.JSON(http.StatusCreated, providerStoreToResponse(saved))
}

// HandleDeleteProviderConfig removes a provider configuration owned by the
// calling workspace (Epic 004). A config that belongs to another workspace is
// indistinguishable from a missing one (404).
func (s *Server) HandleDeleteProviderConfig(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermManageConnectors); err != nil {
		return err
	}
	if s.ProviderStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "providers not configured"})
	}
	wsID, _ := c.Get("workspace_id").(string)
	if wsID == "" {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "workspace not resolved"})
	}

	if err := s.ProviderStore.Delete(c.Request().Context(), wsID, c.Param("id")); err != nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: err.Error()})
	}
	return c.NoContent(http.StatusNoContent)
}

// HandleTestProviderConfig tests a provider configuration by making a live
// request. It uses the request-supplied raw API key directly (an unstored BYO
// key) and never persists anything, so it needs no provider store.
func (s *Server) HandleTestProviderConfig(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermManageConnectors); err != nil {
		return err
	}

	var req SaveProviderConfigRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	}

	prov := editorCreateProvider(req.ProviderType, req.APIKey, req.Model)
	defer prov.Close()

	if _, err := prov.Chat(c.Request().Context(), []aiprovider.Message{
		aiprovider.TextMessage("user", "Hello, respond with OK."),
	}); err != nil {
		// A circuit-broken provider is not a bad credential: the test never ran.
		// Reporting it as a 400 would tell the user their key is wrong.
		if resp, ok := unavailableErr(c, err); ok {
			return resp
		}
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: fmt.Sprintf("connection test failed: %s", err)})
	}

	return c.NoContent(http.StatusNoContent)
}

// HandleGetBlockHistory returns the history of changes for a block.
func (s *Server) HandleGetBlockHistory(c echo.Context) error {
	if s.ContentStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "editor not configured"})
	}

	pid := projectParam(c)
	bid := c.Param("bid")
	locale := c.QueryParam("locale")
	if locale == "" {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "locale query parameter is required"})
	}

	limit, _ := pageParams(c, 20, maxListPageSize)

	entries, err := s.ContentStore.GetBlockHistory(c.Request().Context(), pid, streamParam(c), bid, locale, limit)
	if err != nil {
		return serverErr(c, err)
	}

	return c.JSON(http.StatusOK, entries)
}

// HandleGetTranslationDashboard returns aggregated translation stats for a project.
//
// Locale totals and collection rollups are always complete; the per-file
// item_stats list is paginated when a `limit` query parameter is given
// (`offset`, `sort` = name|words|completion, `dir` = asc|desc), with
// `item_total` carrying the full count. Without a limit the full list is
// returned unchanged (legacy shape). The full computation is cached per
// project/stream, so paging/sorting requests slice the cached result.
//
// `collection=<id>` narrows item_stats to one collection's items and makes
// `item_total` that collection's count, so a consumer drilling into a
// collection pages the list it is actually showing instead of filtering a page
// of the project. `ungrouped=1` asks for the collection-less items — the same
// bucket the rollups mark — because their collection id is the empty string and
// so no `collection` value can name them. The two are mutually exclusive; a
// request carrying both is a 400 rather than a guess at which one was meant.
//
// Each collection rollup carries the coordinates its collection row persists —
// `channel` and `coordinates` — so a consumer can group collections by the
// point in context space their content occupies. Items whose collection id is
// empty aggregate under one bucket with an empty `collection_id` and
// `ungrouped: true`; the flag, not an invented id, marks it, because an
// invented id would resolve to no collection anywhere else in the API. Naming
// that bucket ("Ungrouped", "Default") is the consumer's call. Rollups arrive
// ordered by collection name with the ungrouped bucket last.
func (s *Server) HandleGetTranslationDashboard(c echo.Context) error {
	if s.ContentStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "editor not configured"})
	}

	pid := projectParam(c)
	stream := streamParam(c)
	wsID, _ := c.Get("workspace_id").(string)
	ctx := c.Request().Context()

	sortField := c.QueryParam("sort")
	switch sortField {
	case "", "name", "words", "completion":
	default:
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "sort must be name, words, or completion"})
	}
	ungrouped := boolParam(c, "ungrouped")
	window := dashboardItemWindow{
		collectionID: c.QueryParam("collection"),
		ungrouped:    ungrouped,
		sortField:    sortField,
		dir:          c.QueryParam("dir"),
	}
	window.limit, _ = strconv.Atoi(c.QueryParam("limit"))
	window.offset, _ = strconv.Atoi(c.QueryParam("offset"))
	if window.collectionID != "" && window.ungrouped {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "collection and ungrouped are mutually exclusive"})
	}

	// Check cache first
	cacheKey := dashboardCacheKey(wsID, pid, stream)
	if cached, ok := s.dashboardCache.Load(cacheKey); ok {
		entry := cached.(*dashboardCacheEntry)
		if time.Now().Before(entry.expiresAt) {
			return c.JSON(http.StatusOK, pageDashboardStats(entry.stats, window))
		}
		s.dashboardCache.Delete(cacheKey)
	}

	proj, err := s.ContentStore.GetProject(ctx, pid)
	if err != nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: err.Error()})
	}

	stats, err := editorGetDashboardStats(ctx, s.ContentStore, proj, stream)
	if err != nil {
		return serverErr(c, err)
	}

	// Derive per-locale + per-collection ship states and compliance rates
	// (bounded check pass + deterministic term compliance + persisted voice scores)
	// so the cached result carries them for every paged slice. The term gate
	// resolves the workspace terms snapshot + per-locale voice profile once
	// (never per block) — a nil gate (no terms, no voice store) is a no-op.
	gate := s.resolveTermGate(ctx, proj, stream, wsID)
	if err := applyShipStates(ctx, s.ContentStore, s.VoiceStore, proj.ID, stream, gate, stats); err != nil {
		return serverErr(c, err)
	}

	// Cache the full result; each request slices its own page from it.
	s.dashboardCache.Store(cacheKey, &dashboardCacheEntry{
		stats:     stats,
		expiresAt: time.Now().Add(dashboardCacheTTL),
	})

	return c.JSON(http.StatusOK, pageDashboardStats(stats, window))
}
