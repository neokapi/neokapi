package server

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"

	"github.com/gokapi/gokapi/ai/provider"
	"github.com/gokapi/gokapi/core/locale"
	"github.com/labstack/echo/v4"
)

// HandleCreateEditorProject creates a new in-memory translation project.
func (s *Server) HandleCreateEditorProject(c echo.Context) error {
	if s.EditorStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "editor not configured"})
	}

	ws := c.Param("ws")
	var req struct {
		Name          string   `json:"name"`
		SourceLocale  string   `json:"source_locale"`
		TargetLocales []string `json:"target_locales"`
	}
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	}

	info, err := s.EditorStore.createProject(ws, s.FormatRegistry, req.Name, req.SourceLocale, req.TargetLocales)
	if err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	}

	return c.JSON(http.StatusCreated, info)
}

// HandleGetEditorProject returns an editor project.
func (s *Server) HandleGetEditorProject(c echo.Context) error {
	if s.EditorStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "editor not configured"})
	}

	ws := c.Param("ws")
	pid := c.Param("pid")
	p, err := s.EditorStore.get(ws, pid)
	if err != nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: err.Error()})
	}

	return c.JSON(http.StatusOK, p.info)
}

// HandleListEditorProjects lists all editor projects for a workspace.
func (s *Server) HandleListEditorProjects(c echo.Context) error {
	if s.EditorStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "editor not configured"})
	}

	ws := c.Param("ws")
	projects := s.EditorStore.list(ws)
	return c.JSON(http.StatusOK, projects)
}

// HandleDeleteEditorProject closes and removes an editor project.
func (s *Server) HandleDeleteEditorProject(c echo.Context) error {
	if s.EditorStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "editor not configured"})
	}

	ws := c.Param("ws")
	pid := c.Param("pid")
	s.EditorStore.remove(ws, pid)
	return c.NoContent(http.StatusNoContent)
}

// HandleUploadFiles uploads files to a project via multipart form.
func (s *Server) HandleUploadFiles(c echo.Context) error {
	if s.EditorStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "editor not configured"})
	}

	ws := c.Param("ws")
	pid := c.Param("pid")

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

	info, err := s.EditorStore.addFiles(ws, pid, s.FormatRegistry, files)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}

	return c.JSON(http.StatusOK, info)
}

// HandleRemoveFile removes a file from a project.
func (s *Server) HandleRemoveFile(c echo.Context) error {
	if s.EditorStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "editor not configured"})
	}

	ws := c.Param("ws")
	pid := c.Param("pid")
	fname := c.Param("fname")

	info, err := s.EditorStore.removeFile(ws, pid, fname)
	if err != nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: err.Error()})
	}

	return c.JSON(http.StatusOK, info)
}

// HandleGetFileBlocks returns all blocks for a file in a project.
func (s *Server) HandleGetFileBlocks(c echo.Context) error {
	if s.EditorStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "editor not configured"})
	}

	ws := c.Param("ws")
	pid := c.Param("pid")
	fname := c.Param("fname")

	blocks, err := s.EditorStore.getBlocks(ws, pid, fname)
	if err != nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: err.Error()})
	}

	return c.JSON(http.StatusOK, blocks)
}

// HandleUpdateBlockTarget updates the target text for a block.
func (s *Server) HandleUpdateBlockTarget(c echo.Context) error {
	if s.EditorStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "editor not configured"})
	}

	ws := c.Param("ws")
	pid := c.Param("pid")
	bid := c.Param("bid")

	var req UpdateBlockTargetRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	}

	if err := s.EditorStore.updateBlockTarget(ws, pid, bid, req); err != nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: err.Error()})
	}

	return c.NoContent(http.StatusNoContent)
}

// HandleUpdateBlockTargetCoded updates a block target with coded text and spans.
func (s *Server) HandleUpdateBlockTargetCoded(c echo.Context) error {
	if s.EditorStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "editor not configured"})
	}

	ws := c.Param("ws")
	pid := c.Param("pid")
	bid := c.Param("bid")

	var req UpdateBlockTargetCodedRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	}

	if err := s.EditorStore.updateBlockTargetCoded(ws, pid, bid, req); err != nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: err.Error()})
	}

	return c.NoContent(http.StatusNoContent)
}

// HandlePseudoTranslate pseudo-translates all blocks in a file.
func (s *Server) HandlePseudoTranslate(c echo.Context) error {
	if s.EditorStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "editor not configured"})
	}

	ws := c.Param("ws")
	pid := c.Param("pid")
	fname := c.Param("fname")

	var req struct {
		TargetLocale string `json:"target_locale"`
	}
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	}

	stats, err := s.EditorStore.pseudoTranslate(ws, pid, fname, req.TargetLocale)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}

	return c.JSON(http.StatusOK, stats)
}

// HandleAITranslate translates all blocks using an AI provider.
func (s *Server) HandleAITranslate(c echo.Context) error {
	if s.EditorStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "editor not configured"})
	}

	ws := c.Param("ws")
	pid := c.Param("pid")
	fname := c.Param("fname")

	var req TranslateRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	}

	stats, err := s.EditorStore.aiTranslate(ws, pid, fname, req, s.CredentialStore)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}

	return c.JSON(http.StatusOK, stats)
}

// HandleTMTranslate leverages translation memory to translate blocks.
func (s *Server) HandleTMTranslate(c echo.Context) error {
	if s.EditorStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "editor not configured"})
	}

	ws := c.Param("ws")
	pid := c.Param("pid")
	fname := c.Param("fname")

	var req struct {
		TargetLocale string `json:"target_locale"`
	}
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	}

	stats, err := s.EditorStore.tmTranslate(ws, pid, fname, req.TargetLocale)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}

	return c.JSON(http.StatusOK, stats)
}

// HandleGetWordCount returns word and character counts for a file.
func (s *Server) HandleGetWordCount(c echo.Context) error {
	if s.EditorStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "editor not configured"})
	}

	ws := c.Param("ws")
	pid := c.Param("pid")
	fname := c.Param("fname")

	result, err := s.EditorStore.getWordCount(ws, pid, fname)
	if err != nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: err.Error()})
	}

	return c.JSON(http.StatusOK, result)
}

// HandleExportTranslatedFile exports a translated file.
func (s *Server) HandleExportTranslatedFile(c echo.Context) error {
	if s.EditorStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "editor not configured"})
	}

	ws := c.Param("ws")
	pid := c.Param("pid")
	fname := c.Param("fname")

	var req struct {
		TargetLocale string `json:"target_locale"`
	}
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	}

	outputPath, err := s.EditorStore.exportTranslatedFile(ws, pid, fname, req.TargetLocale, s.FormatRegistry, s.Config.DataDir)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}
	defer os.Remove(outputPath)

	return c.File(outputPath)
}

// HandleLookupTMForBlock looks up TM matches for a specific block.
func (s *Server) HandleLookupTMForBlock(c echo.Context) error {
	if s.EditorStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "editor not configured"})
	}

	ws := c.Param("ws")
	pid := c.Param("pid")
	bid := c.Param("bid")
	itemName := c.QueryParam("item")
	targetLocale := c.QueryParam("target_locale")

	matches, err := s.EditorStore.lookupTMForBlock(ws, pid, itemName, bid, targetLocale)
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
	if s.EditorStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "editor not configured"})
	}

	ws := c.Param("ws")
	pid := c.Param("pid")
	bid := c.Param("bid")
	itemName := c.QueryParam("item")
	targetLocale := c.QueryParam("target_locale")

	matches, err := s.EditorStore.lookupTermsForBlock(ws, pid, itemName, bid, targetLocale)
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
	if s.EditorStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "editor not configured"})
	}

	ws := c.Param("ws")
	query := c.QueryParam("q")
	sourceLocale := c.QueryParam("source_locale")
	targetLocale := c.QueryParam("target_locale")
	offset, _ := strconv.Atoi(c.QueryParam("offset"))
	limit, _ := strconv.Atoi(c.QueryParam("limit"))
	if limit <= 0 {
		limit = 50
	}

	result, err := s.EditorStore.getTMEntries(ws, query, sourceLocale, targetLocale, offset, limit)
	if err != nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: err.Error()})
	}

	return c.JSON(http.StatusOK, result)
}

// HandleGetTMCount returns the TM entry count.
func (s *Server) HandleGetTMCount(c echo.Context) error {
	if s.EditorStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "editor not configured"})
	}

	ws := c.Param("ws")

	count, err := s.EditorStore.getTMCount(ws)
	if err != nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: err.Error()})
	}

	return c.JSON(http.StatusOK, map[string]int{"count": count})
}

// HandleAddTMEntry adds a new TM entry.
func (s *Server) HandleAddTMEntry(c echo.Context) error {
	if s.EditorStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "editor not configured"})
	}

	ws := c.Param("ws")

	var req TMAddRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	}

	entry, err := s.EditorStore.addTMEntry(ws, req)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}

	return c.JSON(http.StatusCreated, entry)
}

// HandleUpdateTMEntry updates an existing TM entry.
func (s *Server) HandleUpdateTMEntry(c echo.Context) error {
	if s.EditorStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "editor not configured"})
	}

	ws := c.Param("ws")
	eid := c.Param("eid")

	var req TMUpdateRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	}

	if err := s.EditorStore.updateTMEntry(ws, eid, req); err != nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: err.Error()})
	}

	return c.NoContent(http.StatusNoContent)
}

// HandleDeleteTMEntry deletes a TM entry.
func (s *Server) HandleDeleteTMEntry(c echo.Context) error {
	if s.EditorStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "editor not configured"})
	}

	ws := c.Param("ws")
	eid := c.Param("eid")

	if err := s.EditorStore.deleteTMEntry(ws, eid); err != nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: err.Error()})
	}

	return c.NoContent(http.StatusNoContent)
}

// HandleGetTerms searches terminology concepts.
func (s *Server) HandleGetTerms(c echo.Context) error {
	if s.EditorStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "editor not configured"})
	}

	ws := c.Param("ws")
	query := c.QueryParam("q")
	sourceLocale := c.QueryParam("source_locale")
	targetLocale := c.QueryParam("target_locale")
	offset, _ := strconv.Atoi(c.QueryParam("offset"))
	limit, _ := strconv.Atoi(c.QueryParam("limit"))
	if limit <= 0 {
		limit = 50
	}

	result, err := s.EditorStore.getTerms(ws, query, sourceLocale, targetLocale, offset, limit)
	if err != nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: err.Error()})
	}

	return c.JSON(http.StatusOK, result)
}

// HandleGetTermCount returns the concept count.
func (s *Server) HandleGetTermCount(c echo.Context) error {
	if s.EditorStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "editor not configured"})
	}

	ws := c.Param("ws")
	count := s.EditorStore.getTermCount(ws)

	return c.JSON(http.StatusOK, map[string]int{"count": count})
}

// HandleAddConcept adds a new terminology concept.
func (s *Server) HandleAddConcept(c echo.Context) error {
	if s.EditorStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "editor not configured"})
	}

	ws := c.Param("ws")

	var req AddConceptRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	}

	concept, err := s.EditorStore.addConcept(ws, req)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}

	return c.JSON(http.StatusCreated, concept)
}

// HandleUpdateConcept updates a terminology concept.
func (s *Server) HandleUpdateConcept(c echo.Context) error {
	if s.EditorStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "editor not configured"})
	}

	ws := c.Param("ws")
	cid := c.Param("cid")

	var req UpdateConceptRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	}

	if err := s.EditorStore.updateConcept(ws, cid, req); err != nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: err.Error()})
	}

	return c.NoContent(http.StatusNoContent)
}

// HandleDeleteConcept deletes a terminology concept.
func (s *Server) HandleDeleteConcept(c echo.Context) error {
	if s.EditorStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "editor not configured"})
	}

	ws := c.Param("ws")
	cid := c.Param("cid")

	if err := s.EditorStore.deleteConcept(ws, cid); err != nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: err.Error()})
	}

	return c.NoContent(http.StatusNoContent)
}

// HandleImportTermsCSV imports terms from CSV.
func (s *Server) HandleImportTermsCSV(c echo.Context) error {
	if s.EditorStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "editor not configured"})
	}

	ws := c.Param("ws")

	var req ImportCSVRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	}

	count, err := s.EditorStore.importTermsCSV(ws, req)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}

	return c.JSON(http.StatusOK, map[string]int{"imported": count})
}

// HandleImportTermsJSON imports terms from JSON.
func (s *Server) HandleImportTermsJSON(c echo.Context) error {
	if s.EditorStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "editor not configured"})
	}

	ws := c.Param("ws")

	var req ImportJSONRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	}

	count, err := s.EditorStore.importTermsJSON(ws, req.JSONContent)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}

	return c.JSON(http.StatusOK, map[string]int{"imported": count})
}

// HandleExportTermsJSON exports terms as JSON.
func (s *Server) HandleExportTermsJSON(c echo.Context) error {
	if s.EditorStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "editor not configured"})
	}

	ws := c.Param("ws")
	name := c.QueryParam("name")

	data, err := s.EditorStore.exportTermsJSON(ws, name)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}

	return c.JSONBlob(http.StatusOK, []byte(data))
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
	if s.CredentialStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "credentials not configured"})
	}

	var req SaveProviderConfigRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	}

	saved := s.CredentialStore.Upsert(req.toCredentials())

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

	if _, err := prov.Chat(c.Request().Context(), []provider.Message{
		{Role: "user", Content: "Hello, respond with OK."},
	}); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: fmt.Sprintf("connection test failed: %s", err)})
	}

	return c.NoContent(http.StatusNoContent)
}

// HandleGetKnownLocales returns a curated list of BCP-47 locales.
func (s *Server) HandleGetKnownLocales(c echo.Context) error {
	locales := locale.WellKnownLocales()
	return c.JSON(http.StatusOK, locales)
}
