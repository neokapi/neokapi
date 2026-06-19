package server

import (
	"fmt"
	"io"
	"maps"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/labstack/echo/v4"
	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	"github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/core/id"
	"github.com/neokapi/neokapi/core/model"
	aiprovider "github.com/neokapi/neokapi/providers/ai"
	"github.com/neokapi/neokapi/sievepen"
)

// HandleCreateEditorProject creates a new translation project in the ContentStore.
func (s *Server) HandleCreateEditorProject(c echo.Context) error {
	if s.ContentStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "editor not configured"})
	}

	wsID, _ := c.Get("workspace_id").(string)
	var req struct {
		Name                  string   `json:"name"`
		DefaultSourceLanguage string   `json:"default_source_language"`
		TargetLanguages       []string `json:"target_languages"`
	}
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	}

	info, err := editorCreateProject(c.Request().Context(), s.ContentStore, wsID, req.Name, req.DefaultSourceLanguage, req.TargetLanguages)
	if err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	}

	return c.JSON(http.StatusCreated, info)
}

// HandleGetEditorProject returns an editor project.
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

	info, err := editorBuildProjectInfo(ctx, s.ContentStore, proj, streamParamWithProject(c, proj))
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}

	return c.JSON(http.StatusOK, info)
}

// HandleListEditorProjects lists all editor projects for a workspace.
func (s *Server) HandleListEditorProjects(c echo.Context) error {
	if s.ContentStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "editor not configured"})
	}

	wsID, _ := c.Get("workspace_id").(string)
	ctx := c.Request().Context()

	projects, err := s.ContentStore.ListProjects(ctx)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}

	// Filter by workspace (using UUID from middleware context).
	var result []*ProjectInfoResponse
	for _, p := range projects {
		if p.WorkspaceID == wsID {
			info := projectToInfoResponse(p)
			// Populate items so the dashboard can show file counts.
			stream := streamParamWithProject(c, p)
			items, err := s.ContentStore.ListItems(ctx, p.ID, stream)
			if err == nil {
				for _, item := range items {
					info.Items = append(info.Items, ProjectItemResponse{
						ID:     item.ID,
						Name:   item.Name,
						Format: item.Format,
						Type:   item.ItemType,
						Size:   0,
					})
				}
			}
			result = append(result, info)
		}
	}
	if result == nil {
		result = []*ProjectInfoResponse{}
	}
	return c.JSON(http.StatusOK, result)
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
		if proj.Properties == nil {
			proj.Properties = make(map[string]string)
		}
		maps.Copy(proj.Properties, req.Properties)
	}

	if err := s.ContentStore.UpdateProject(ctx, proj); err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}

	info, err := editorBuildProjectInfo(ctx, s.ContentStore, proj, streamParamWithProject(c, proj))
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}

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
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
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
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
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
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
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
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
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

	form, err := c.MultipartForm()
	if err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "multipart form required"})
	}

	files := make(map[string][]byte)
	for _, fh := range form.File["files"] {
		f, err := fh.Open()
		if err != nil {
			return c.JSON(http.StatusBadRequest, ErrorResponse{Error: fmt.Sprintf("open %q: %s", fh.Filename, err)})
		}
		data, err := io.ReadAll(f)
		f.Close()
		if err != nil {
			return c.JSON(http.StatusBadRequest, ErrorResponse{Error: fmt.Sprintf("read %q: %s", fh.Filename, err)})
		}
		files[fh.Filename] = data
	}

	if len(files) == 0 {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "no files uploaded"})
	}

	info, err := editorAddFiles(c.Request().Context(), s.ContentStore, s.FormatRegistry, pid, streamParam(c), files)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}

	wsID, _ := c.Get("workspace_id").(string)
	s.invalidateDashboardCache(wsID, pid)
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

	info, err := editorRemoveFile(c.Request().Context(), s.ContentStore, pid, streamParam(c), fname)
	if err != nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: err.Error()})
	}

	wsID, _ := c.Get("workspace_id").(string)
	s.invalidateDashboardCache(wsID, pid)
	return c.JSON(http.StatusOK, info)
}

// HandleGetFileBlocks returns all blocks for a file in a project.
func (s *Server) HandleGetFileBlocks(c echo.Context) error {
	if s.ContentStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "editor not configured"})
	}

	pid := projectParam(c)
	fname := fileParam(c)
	ctx := c.Request().Context()

	// Get project to know target locales.
	proj, err := s.ContentStore.GetProject(ctx, pid)
	if err != nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: err.Error()})
	}

	targetLocales := make([]string, len(proj.TargetLanguages))
	for i, l := range proj.TargetLanguages {
		targetLocales[i] = string(l)
	}

	blocks, err := editorGetBlocks(ctx, s.ContentStore, pid, streamParam(c), fname, targetLocales)
	if err != nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: err.Error()})
	}

	return c.JSON(http.StatusOK, blocks)
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
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
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

	wsID, _ := c.Get("workspace_id").(string)
	stats, err := editorAITranslate(c.Request().Context(), s.ContentStore, pid, streamParam(c), fname, req, s.CredentialStore, s.BillingHooks, wsID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}

	s.invalidateDashboardCache(wsID, pid)
	return c.JSON(http.StatusOK, stats)
}

// HandleTMTranslate leverages translation memory to translate blocks.
func (s *Server) HandleTMTranslate(c echo.Context) error {
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

	stats, err := editorTMTranslate(c.Request().Context(), s.ContentStore, s.wsStores, ws, pid, streamParam(c), fname, req.TargetLocale)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}

	wsID, _ := c.Get("workspace_id").(string)
	s.invalidateDashboardCache(wsID, pid)
	return c.JSON(http.StatusOK, stats)
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

// HandleExportTranslatedFile exports a translated file.
func (s *Server) HandleExportTranslatedFile(c echo.Context) error {
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

	outputPath, err := editorExportTranslatedFile(c.Request().Context(), s.ContentStore, s.FormatRegistry, pid, streamParam(c), fname, req.TargetLocale, s.Config.DataDir)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}
	defer os.Remove(outputPath)

	return c.File(outputPath)
}

// HandleLookupTMForBlock looks up TM matches for a specific block.
func (s *Server) HandleLookupTMForBlock(c echo.Context) error {
	if s.ContentStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "editor not configured"})
	}

	ws := c.Param("ws")
	pid := projectParam(c)
	bid := c.Param("bid")
	targetLocale := c.QueryParam("target_locale")

	matches, err := editorLookupTMForBlock(c.Request().Context(), s.ContentStore, s.wsStores, ws, pid, streamParam(c), bid, targetLocale)
	if err != nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: err.Error()})
	}

	if matches == nil {
		matches = []TMMatchInfoResponse{}
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

// HandleGetTMEntries searches TM entries.
func (s *Server) HandleGetTMEntries(c echo.Context) error {
	if s.wsStores == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "editor not configured"})
	}

	ws := c.Param("ws")
	query := c.QueryParam("q")
	sourceLocale := c.QueryParam("source_locale")
	targetLocale := c.QueryParam("target_locale")
	projectID := c.QueryParam("project_id")
	offset, _ := strconv.Atoi(c.QueryParam("offset"))
	limit, _ := strconv.Atoi(c.QueryParam("limit"))
	if limit <= 0 {
		limit = 50
	}

	tm, err := s.wsStores.getTM(ws)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}

	stream := c.QueryParam("stream")
	ctx := c.Request().Context()
	var entries []sievepen.TMEntry
	var total int
	if stream != "" && stream != "main" && s.ContentStore != nil {
		pid := c.QueryParam("project_id")
		chain := buildStreamChain(ctx, s.ContentStore, pid, stream)
		entries, total, err = tm.SearchEntriesForStream(ctx, sievepen.SearchParams{
			Query:         query,
			AnyLocale:     sourceLocale,
			RequireLocale: targetLocale,
			Stream:        stream,
			StreamChain:   chain[1:],
			Offset:        offset,
			Limit:         limit,
		})
	} else {
		entries, total, err = tm.SearchEntries(ctx, sievepen.SearchParams{
			Query:         query,
			AnyLocale:     sourceLocale,
			RequireLocale: targetLocale,
			Offset:        offset,
			Limit:         limit,
		})
	}
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}

	// Post-filter by project_id if specified.
	if projectID != "" {
		filtered := make([]sievepen.TMEntry, 0, len(entries))
		for _, e := range entries {
			if e.ProjectID == projectID {
				filtered = append(filtered, e)
			}
		}
		entries = filtered
		total = len(filtered)
	}

	infos := make([]TMEntryInfoResponse, len(entries))
	for i, e := range entries {
		infos[i] = editorEntryToInfo(e, sourceLocale, targetLocale)
	}

	return c.JSON(http.StatusOK, TMSearchResponse{Entries: infos, TotalCount: total})
}

// HandleGetTMCount returns the TM entry count.
func (s *Server) HandleGetTMCount(c echo.Context) error {
	if s.wsStores == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "editor not configured"})
	}

	ws := c.Param("ws")

	tm, err := s.wsStores.getTM(ws)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}

	count, err := tm.Count(c.Request().Context())
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}
	return c.JSON(http.StatusOK, map[string]int{"count": count})
}

// HandleAddTMEntry adds a new TM entry.
func (s *Server) HandleAddTMEntry(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermManageTM); err != nil {
		return err
	}
	if s.wsStores == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "editor not configured"})
	}

	ws := c.Param("ws")

	var req TMAddRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	}

	tm, err := s.wsStores.getTM(ws)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}

	srcLoc := model.LocaleID(req.SourceLocale)
	tgtLoc := model.LocaleID(req.TargetLocale)
	entry := sievepen.TMEntry{
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
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}

	return c.JSON(http.StatusCreated, editorEntryToInfo(entry, req.SourceLocale, req.TargetLocale))
}

// HandleUpdateTMEntry updates an existing TM entry.
func (s *Server) HandleUpdateTMEntry(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermManageTM); err != nil {
		return err
	}
	if s.wsStores == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "editor not configured"})
	}

	ws := c.Param("ws")
	eid := c.Param("eid")

	var req TMUpdateRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	}

	tm, err := s.wsStores.getTM(ws)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}

	existing, ok, err := tm.GetEntry(c.Request().Context(), eid)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}
	if !ok {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: fmt.Sprintf("TM entry %q not found", eid)})
	}

	// Delete old and add updated.
	if err := tm.Delete(c.Request().Context(), eid); err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
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
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}

	return c.NoContent(http.StatusNoContent)
}

// HandleDeleteTMEntry deletes a TM entry.
func (s *Server) HandleDeleteTMEntry(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermManageTM); err != nil {
		return err
	}
	if s.wsStores == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "editor not configured"})
	}

	ws := c.Param("ws")
	eid := c.Param("eid")

	tm, err := s.wsStores.getTM(ws)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}

	if err := tm.Delete(c.Request().Context(), eid); err != nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: err.Error()})
	}

	return c.NoContent(http.StatusNoContent)
}

// HandleListProviderConfigs lists all saved AI provider configurations.
func (s *Server) HandleListProviderConfigs(c echo.Context) error {
	if s.CredentialStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "credentials not configured"})
	}

	configs := s.CredentialStore.List()
	out := make([]ProviderConfigResponse, len(configs))
	for i, cfg := range configs {
		out[i] = toProviderConfigResponse(cfg)
	}

	return c.JSON(http.StatusOK, out)
}

// HandleSaveProviderConfig creates or updates a provider configuration.
func (s *Server) HandleSaveProviderConfig(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermManageConnectors); err != nil {
		return err
	}
	if s.CredentialStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "credentials not configured"})
	}

	var req SaveProviderConfigRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	}

	saved, err := s.CredentialStore.Upsert(req.toCredentials())
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: fmt.Sprintf("save provider config: %s", err)})
	}

	if req.APIKey != "" {
		if err := s.CredentialStore.SetAPIKey(saved.ID, req.APIKey); err != nil {
			return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: fmt.Sprintf("save API key: %s", err)})
		}
	}

	result := toProviderConfigResponse(saved)
	return c.JSON(http.StatusCreated, result)
}

// HandleDeleteProviderConfig removes a provider configuration.
func (s *Server) HandleDeleteProviderConfig(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermManageConnectors); err != nil {
		return err
	}
	if s.CredentialStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "credentials not configured"})
	}

	id := c.Param("id")
	if err := s.CredentialStore.Remove(id); err != nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: err.Error()})
	}
	_ = s.CredentialStore.DeleteAPIKey(id) // best-effort

	return c.NoContent(http.StatusNoContent)
}

// HandleTestProviderConfig tests a provider configuration.
func (s *Server) HandleTestProviderConfig(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermManageConnectors); err != nil {
		return err
	}
	if s.CredentialStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "credentials not configured"})
	}

	var req SaveProviderConfigRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	}

	cfg := req.toCredentials()
	prov := editorCreateProvider(cfg.ProviderType, req.APIKey, cfg.Model)
	defer prov.Close()

	if _, err := prov.Chat(c.Request().Context(), []aiprovider.Message{
		aiprovider.TextMessage("user", "Hello, respond with OK."),
	}); err != nil {
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

	limit, _ := strconv.Atoi(c.QueryParam("limit"))
	if limit <= 0 {
		limit = 20
	}

	entries, err := s.ContentStore.GetBlockHistory(c.Request().Context(), pid, streamParam(c), bid, locale, limit)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}

	return c.JSON(http.StatusOK, entries)
}

// HandleGetTranslationDashboard returns aggregated translation stats for a project.
func (s *Server) HandleGetTranslationDashboard(c echo.Context) error {
	if s.ContentStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "editor not configured"})
	}

	pid := projectParam(c)
	stream := streamParam(c)
	wsID, _ := c.Get("workspace_id").(string)
	ctx := c.Request().Context()

	// Check cache first
	cacheKey := dashboardCacheKey(wsID, pid, stream)
	if cached, ok := s.dashboardCache.Load(cacheKey); ok {
		entry := cached.(*dashboardCacheEntry)
		if time.Now().Before(entry.expiresAt) {
			return c.JSON(http.StatusOK, entry.stats)
		}
		s.dashboardCache.Delete(cacheKey)
	}

	proj, err := s.ContentStore.GetProject(ctx, pid)
	if err != nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: err.Error()})
	}

	stats, err := editorGetDashboardStats(ctx, s.ContentStore, proj, stream)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}

	// Cache the result
	s.dashboardCache.Store(cacheKey, &dashboardCacheEntry{
		stats:     stats,
		expiresAt: time.Now().Add(dashboardCacheTTL),
	})

	return c.JSON(http.StatusOK, stats)
}
