package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/labstack/echo/v4"
	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	"github.com/neokapi/neokapi/bowrain/core/store"
	bstore "github.com/neokapi/neokapi/bowrain/store"
	"github.com/neokapi/neokapi/core/locale"
	"github.com/neokapi/neokapi/core/model"
)

// ---------------------------------------------------------------------------
// Front Page (no workspace)
// ---------------------------------------------------------------------------

// HandlePulseFrontPage returns public workspaces with summary stats for the
// Pulse landing page.
// GET /api/v1/pulse
func (s *Server) HandlePulseFrontPage(c echo.Context) error {
	if s.AuthStore == nil {
		return c.JSON(http.StatusOK, map[string]any{
			"workspaces": []any{},
			"stats":      store.PulseGlobalStats{},
		})
	}

	cacheKey := pulseCacheKey("_front", "front", "")
	if cached, ok := s.pulseCache.Get(cacheKey); ok {
		return c.JSON(http.StatusOK, cached)
	}

	ctx := c.Request().Context()
	workspaces, err := s.AuthStore.ListPublicWorkspaces(ctx)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "failed to list workspaces"})
	}

	type workspaceSummary struct {
		Name        string  `json:"name"`
		Slug        string  `json:"slug"`
		Description string  `json:"description"`
		LogoURL     string  `json:"logo_url,omitempty"`
		Projects    int     `json:"projects"`
		Languages   int     `json:"languages"`
		Percentage  float64 `json:"percentage"`
	}

	summaries := make([]workspaceSummary, 0, len(workspaces))
	var totalProjects, totalLanguages int

	for _, ws := range workspaces {
		projects, err := s.pulseVisibleProjects(ctx, ws.ID)
		if err != nil {
			continue
		}

		langSet := make(map[string]bool)
		totalWords, translatedWords := 0, 0
		for _, p := range projects {
			summary := s.buildProjectSummary(ctx, p)
			totalWords += summary.TotalWords
			translatedWords += summary.TranslatedWords
			for _, loc := range summary.Locales {
				langSet[loc.Locale] = true
			}
		}

		pct := 0.0
		if totalWords > 0 {
			pct = float64(translatedWords) * 100 / float64(totalWords)
		}

		summaries = append(summaries, workspaceSummary{
			Name:        ws.Name,
			Slug:        ws.Slug,
			Description: ws.Description,
			LogoURL:     ws.LogoURL,
			Projects:    len(projects),
			Languages:   len(langSet),
			Percentage:  pct,
		})

		totalProjects += len(projects)
		totalLanguages += len(langSet)
	}

	resp := map[string]any{
		"workspaces": summaries,
		"stats": store.PulseGlobalStats{
			TotalProjects:  totalProjects,
			TotalLanguages: totalLanguages,
		},
	}

	s.pulseCache.Set(cacheKey, "overview", resp)
	setCDNCacheHeaders(c, 120)
	return c.JSON(http.StatusOK, resp)
}

// ---------------------------------------------------------------------------
// Workspace Overview
// ---------------------------------------------------------------------------

// HandlePulseOverview returns the workspace overview for the Pulse dashboard.
// GET /api/v1/pulse/:workspace
func (s *Server) HandlePulseOverview(c echo.Context) error {
	ws := pulseWorkspace(c)
	if ws == nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: "not found"})
	}

	wsID := ws.ID
	cacheKey := pulseCacheKey(wsID, "overview", "")
	if cached, ok := s.pulseCache.Get(cacheKey); ok {
		return c.JSON(http.StatusOK, cached)
	}

	ctx := c.Request().Context()
	projects, err := s.pulseVisibleProjects(ctx, wsID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "failed to list projects"})
	}

	overview := s.buildPulseOverview(ctx, ws, projects)

	s.pulseCache.Set(cacheKey, "overview", overview)
	setCDNCacheHeaders(c, 60)
	return c.JSON(http.StatusOK, overview)
}

// ---------------------------------------------------------------------------
// Project List
// ---------------------------------------------------------------------------

// HandlePulseProjects returns the project list for the workspace.
// GET /api/v1/pulse/:workspace/projects
func (s *Server) HandlePulseProjects(c echo.Context) error {
	ws := pulseWorkspace(c)
	if ws == nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: "not found"})
	}

	ctx := c.Request().Context()
	projects, err := s.pulseVisibleProjects(ctx, ws.ID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "failed to list projects"})
	}

	summaries := make([]store.PulseProjectSummary, 0, len(projects))
	for _, p := range projects {
		summaries = append(summaries, s.buildProjectSummary(ctx, p))
	}

	setCDNCacheHeaders(c, 60)
	return c.JSON(http.StatusOK, map[string]any{"projects": summaries})
}

// ---------------------------------------------------------------------------
// Project Detail
// ---------------------------------------------------------------------------

// HandlePulseProjectDetail returns detailed stats for a single project.
// GET /api/v1/pulse/:workspace/projects/:pid
func (s *Server) HandlePulseProjectDetail(c echo.Context) error {
	ws := pulseWorkspace(c)
	if ws == nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: "not found"})
	}

	p := pulseProject(c)
	if p == nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: "not found"})
	}
	ctx := c.Request().Context()

	summary := s.buildProjectSummary(ctx, p)
	detail := store.PulseProjectDetail{
		Project: summary,
		Locales: summary.Locales,
		Items:   s.buildItemStats(ctx, p),
	}

	setCDNCacheHeaders(c, 60)
	return c.JSON(http.StatusOK, detail)
}

// ---------------------------------------------------------------------------
// Locale Detail
// ---------------------------------------------------------------------------

// HandlePulseLocaleDetail returns detailed stats for a locale within a project.
// GET /api/v1/pulse/:workspace/projects/:pid/lang/:locale
func (s *Server) HandlePulseLocaleDetail(c echo.Context) error {
	ws := pulseWorkspace(c)
	if ws == nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: "not found"})
	}

	p := pulseProject(c)
	if p == nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: "not found"})
	}
	locale := c.Param("locale")
	ctx := c.Request().Context()

	stats, err := editorGetDashboardStats(ctx, s.ContentStore, p, "")
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "failed to compute stats"})
	}

	var localeStats store.LocaleTranslationStats
	found := false
	for _, ls := range stats.LocaleStats {
		if ls.Locale == locale {
			localeStats = ls
			found = true
			break
		}
	}
	if !found {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: "locale not found"})
	}

	detail := store.PulseLocaleDetail{
		Locale: locale,
		Stats:  localeStats,
		Items:  s.buildItemStats(ctx, p),
	}

	setCDNCacheHeaders(c, 60)
	return c.JSON(http.StatusOK, detail)
}

// ---------------------------------------------------------------------------
// Activity Feed
// ---------------------------------------------------------------------------

// HandlePulseActivity returns recent activity for the workspace.
// GET /api/v1/pulse/:workspace/activity
func (s *Server) HandlePulseActivity(c echo.Context) error {
	ws := pulseWorkspace(c)
	if ws == nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: "not found"})
	}

	activities := make([]store.PulseActivity, 0)

	if s.ActivityStore != nil {
		ctx := c.Request().Context()
		limit := 20
		if l := c.QueryParam("limit"); l != "" {
			if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 100 {
				limit = parsed
			}
		}

		q := bstore.ActivityQuery{
			WorkspaceID: ws.ID,
			ProjectID:   c.QueryParam("project"),
			ActorID:     c.QueryParam("contributor"),
			Type:        c.QueryParam("type"),
			Cursor:      c.QueryParam("cursor"),
			Limit:       limit,
		}

		result, err := s.ActivityStore.List(ctx, q)
		if err == nil && result.Activities != nil {
			for _, a := range result.Activities {
				activities = append(activities, store.PulseActivity{
					ID:        a.ID,
					Type:      string(a.Type),
					Actor:     a.ActorName,
					Project:   a.ProjectID,
					Locale:    a.Data["locale"],
					Summary:   a.Summary,
					Timestamp: a.CreatedAt,
				})
			}
		}
	}

	setCDNCacheHeaders(c, 30)
	return c.JSON(http.StatusOK, map[string]any{
		"activities": activities,
	})
}

// ---------------------------------------------------------------------------
// Activity Heatmap
// ---------------------------------------------------------------------------

// HandlePulseActivityHeatmap returns daily activity counts for the contribution heatmap.
// GET /api/v1/pulse/:workspace/activity/heatmap
func (s *Server) HandlePulseActivityHeatmap(c echo.Context) error {
	ws := pulseWorkspace(c)
	if ws == nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: "not found"})
	}

	days := make([]store.PulseHeatmapDay, 0)

	if s.ActivityStore != nil {
		since := time.Now().UTC().AddDate(-1, 0, 0)
		counts, err := s.ActivityStore.DailyCounts(c.Request().Context(), ws.ID, since)
		if err == nil {
			for _, dc := range counts {
				days = append(days, store.PulseHeatmapDay{Date: dc.Date, Count: dc.Count})
			}
		}
	}

	cacheKey := pulseCacheKey(ws.ID, "heatmap", "")
	s.pulseCache.Set(cacheKey, "heatmap", map[string]any{"days": days})
	setCDNCacheHeaders(c, 120)
	return c.JSON(http.StatusOK, map[string]any{"days": days})
}

// ---------------------------------------------------------------------------
// Leaderboard
// ---------------------------------------------------------------------------

// HandlePulseLeaderboard returns the contributor and language leaderboard.
// GET /api/v1/pulse/:workspace/leaderboard
func (s *Server) HandlePulseLeaderboard(c echo.Context) error {
	ws := pulseWorkspace(c)
	if ws == nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: "not found"})
	}

	wsID := ws.ID
	cacheKey := pulseCacheKey(wsID, "leaderboard", c.QueryString())
	if cached, ok := s.pulseCache.Get(cacheKey); ok {
		return c.JSON(http.StatusOK, cached)
	}

	ctx := c.Request().Context()
	projects, err := s.pulseVisibleProjects(ctx, wsID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "failed to list projects"})
	}

	langMap := make(map[string]*store.PulseLanguageRank)
	for _, p := range projects {
		stats, err := editorGetDashboardStats(ctx, s.ContentStore, p, "")
		if err != nil {
			continue
		}
		for _, ls := range stats.LocaleStats {
			if rank, ok := langMap[ls.Locale]; ok {
				rank.TranslatedWords += ls.TranslatedWords
				rank.TotalWords += ls.TotalWords
			} else {
				langMap[ls.Locale] = &store.PulseLanguageRank{
					Locale:          ls.Locale,
					DisplayName:     locale.DisplayName(model.LocaleID(ls.Locale)),
					TranslatedWords: ls.TranslatedWords,
					TotalWords:      ls.TotalWords,
				}
			}
		}
	}

	languages := make([]store.PulseLanguageRank, 0, len(langMap))
	for _, rank := range langMap {
		if rank.TotalWords > 0 {
			rank.Percentage = float64(rank.TranslatedWords) * 100 / float64(rank.TotalWords)
		}
		languages = append(languages, *rank)
	}

	leaderboard := store.PulseLeaderboard{
		Contributors: []store.PulseContributor{},
		Languages:    languages,
	}

	s.pulseCache.Set(cacheKey, "leaderboard", leaderboard)
	setCDNCacheHeaders(c, 120)
	return c.JSON(http.StatusOK, leaderboard)
}

// ---------------------------------------------------------------------------
// Terminology Explorer
// ---------------------------------------------------------------------------

// HandlePulseTerms returns terminology for the workspace.
// GET /api/v1/pulse/:workspace/terms
func (s *Server) HandlePulseTerms(c echo.Context) error {
	ws := pulseWorkspace(c)
	if ws == nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: "not found"})
	}

	terms := make([]store.PulseTermEntry, 0)

	setCDNCacheHeaders(c, 300)
	return c.JSON(http.StatusOK, map[string]any{
		"terms":        terms,
		"term_sources": ws.PulseTermSources,
	})
}

// HandlePulseTermDetail returns a single terminology concept.
// GET /api/v1/pulse/:workspace/terms/:cid
func (s *Server) HandlePulseTermDetail(c echo.Context) error {
	ws := pulseWorkspace(c)
	if ws == nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: "not found"})
	}

	_ = c.Param("cid")

	setCDNCacheHeaders(c, 300)
	return c.JSON(http.StatusOK, store.PulseTermEntry{})
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// pulseWorkspace extracts the workspace from the echo context (set by PulseAccessMiddleware).
func pulseWorkspace(c echo.Context) *platauth.Workspace {
	ws, _ := c.Get("pulse_workspace").(*platauth.Workspace)
	return ws
}

// pulseProject extracts the project from the echo context (set by PulseProjectAccessMiddleware).
func pulseProject(c echo.Context) *store.Project {
	p, _ := c.Get("pulse_project").(*store.Project)
	return p
}

// pulseVisibleProjects returns projects in the workspace that are public or unlisted.
func (s *Server) pulseVisibleProjects(ctx context.Context, workspaceID string) ([]*store.Project, error) {
	if s.ContentStore == nil {
		return nil, errors.New("content store not available")
	}

	allProjects, err := s.ContentStore.ListProjects(ctx)
	if err != nil {
		return nil, err
	}

	visible := make([]*store.Project, 0)
	for _, p := range allProjects {
		if p.WorkspaceID != workspaceID {
			continue
		}
		if p.Archived {
			continue
		}
		if p.DashboardVisibility == "public" || p.DashboardVisibility == "unlisted" {
			visible = append(visible, p)
		}
	}
	return visible, nil
}

// buildPulseOverview constructs the full overview response.
func (s *Server) buildPulseOverview(ctx context.Context, ws *platauth.Workspace, projects []*store.Project) store.PulseOverview {
	projectSummaries := make([]store.PulseProjectSummary, 0, len(projects))
	langMap := make(map[string]*store.PulseLanguageRank)
	totalWords := 0
	translatedWords := 0
	langSet := make(map[string]bool)

	for _, p := range projects {
		summary := s.buildProjectSummary(ctx, p)
		projectSummaries = append(projectSummaries, summary)
		totalWords += summary.TotalWords
		translatedWords += summary.TranslatedWords

		for _, loc := range summary.Locales {
			langSet[loc.Locale] = true
			if rank, ok := langMap[loc.Locale]; ok {
				rank.TranslatedWords += loc.TranslatedWords
				rank.TotalWords += loc.TotalWords
			} else {
				langMap[loc.Locale] = &store.PulseLanguageRank{
					Locale:          loc.Locale,
					DisplayName:     locale.DisplayName(model.LocaleID(loc.Locale)),
					TranslatedWords: loc.TranslatedWords,
					TotalWords:      loc.TotalWords,
				}
			}
		}
	}

	topLanguages := make([]store.PulseLanguageRank, 0, len(langMap))
	for _, rank := range langMap {
		if rank.TotalWords > 0 {
			rank.Percentage = float64(rank.TranslatedWords) * 100 / float64(rank.TotalWords)
		}
		topLanguages = append(topLanguages, *rank)
	}

	overallPct := 0.0
	if totalWords > 0 {
		overallPct = float64(translatedWords) * 100 / float64(totalWords)
	}

	return store.PulseOverview{
		Workspace: store.PulseWorkspaceInfo{
			Name:        ws.Name,
			Slug:        ws.Slug,
			Description: ws.Description,
			LogoURL:     ws.LogoURL,
		},
		Projects:       projectSummaries,
		TopLanguages:   topLanguages,
		TopContribs:    []store.PulseContributor{},
		RisingStars:    []store.PulseRisingStar{},
		RecentActivity: []store.PulseActivity{},
		Stats: store.PulseGlobalStats{
			TotalProjects:     len(projects),
			TotalLanguages:    len(langSet),
			TotalContributors: 0,
			TotalWords:        totalWords,
			TranslatedWords:   translatedWords,
			OverallPercent:    overallPct,
		},
	}
}

// buildProjectSummary builds a PulseProjectSummary from a store.Project.
func (s *Server) buildProjectSummary(ctx context.Context, p *store.Project) store.PulseProjectSummary {
	targets := make([]string, len(p.TargetLanguages))
	targetNames := make(map[string]string, len(p.TargetLanguages))
	for i, l := range p.TargetLanguages {
		code := string(l)
		targets[i] = code
		targetNames[code] = locale.DisplayName(l)
	}

	summary := store.PulseProjectSummary{
		ID:                        p.ID,
		Name:                      p.Name,
		SourceLanguage:            string(p.DefaultSourceLanguage),
		SourceLanguageDisplayName: locale.DisplayName(p.DefaultSourceLanguage),
		TargetLanguages:           targets,
		TargetLanguageNames:       targetNames,
	}

	stats, err := editorGetDashboardStats(ctx, s.ContentStore, p, "")
	if err != nil {
		return summary
	}

	summary.TotalWords = stats.TotalSourceWords * len(p.TargetLanguages)
	summary.Locales = stats.LocaleStats
	for _, ls := range stats.LocaleStats {
		summary.TranslatedWords += ls.TranslatedWords
	}
	if summary.TotalWords > 0 {
		summary.Percentage = float64(summary.TranslatedWords) * 100 / float64(summary.TotalWords)
	}

	return summary
}

// buildItemStats builds item-level translation stats for a project.
func (s *Server) buildItemStats(ctx context.Context, p *store.Project) []store.ItemTranslationStats {
	stats, err := editorGetDashboardStats(ctx, s.ContentStore, p, "")
	if err != nil {
		return []store.ItemTranslationStats{}
	}
	return stats.ItemStats
}

// setCDNCacheHeaders sets Cache-Control and CDN headers for public endpoints.
func setCDNCacheHeaders(c echo.Context, maxAge int) {
	c.Response().Header().Set("Cache-Control", fmt.Sprintf("public, max-age=%d", maxAge))
}
