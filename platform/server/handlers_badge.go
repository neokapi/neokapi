package server

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
	"github.com/neokapi/neokapi/core/locale"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/platform/store"
)

// BadgeResponse is a shields.io endpoint-compatible JSON response.
// See https://shields.io/endpoint
type BadgeResponse struct {
	SchemaVersion int    `json:"schemaVersion"`
	Label         string `json:"label"`
	Message       string `json:"message"`
	Color         string `json:"color"`
	NamedLogo     string `json:"namedLogo,omitempty"`
	CacheSeconds  int    `json:"cacheSeconds,omitempty"`
}

// HandleProjectBadge returns a shields.io-compatible badge for a project.
// GET /api/v1/badges/projects/:id
// Public (no auth required), CDN-cacheable.
func (s *Server) HandleProjectBadge(c echo.Context) error {
	if s.Services == nil {
		return c.JSON(http.StatusServiceUnavailable, BadgeResponse{
			SchemaVersion: 1,
			Label:         "bowrain",
			Message:       "unavailable",
			Color:         "lightgrey",
		})
	}

	projectID := c.Param("id")
	ctx := c.Request().Context()

	p, err := s.Services.Project.GetProject(ctx, projectID)
	if err != nil {
		return c.JSON(http.StatusOK, BadgeResponse{
			SchemaVersion: 1,
			Label:         "bowrain",
			Message:       "not found",
			Color:         "lightgrey",
			CacheSeconds:  300,
		})
	}

	badgeType := c.QueryParam("type")
	switch badgeType {
	case "locales":
		return c.JSON(http.StatusOK, localeBadge(p))
	case "blocks":
		return c.JSON(http.StatusOK, s.blockBadge(c, p))
	case "progress":
		return c.JSON(http.StatusOK, s.progressBadge(c, p))
	default:
		return c.JSON(http.StatusOK, s.progressBadge(c, p))
	}
}

func localeBadge(p *store.Project) BadgeResponse {
	locales := make([]string, len(p.TargetLanguages))
	for i, l := range p.TargetLanguages {
		locales[i] = locale.DisplayName(model.LocaleID(l))
	}
	srcName := locale.DisplayName(p.DefaultSourceLanguage)
	msg := fmt.Sprintf("%s → %s", srcName, strings.Join(locales, ", "))
	return BadgeResponse{
		SchemaVersion: 1,
		Label:         "locales",
		Message:       msg,
		Color:         "blue",
		CacheSeconds:  3600,
	}
}

func (s *Server) blockBadge(c echo.Context, p *store.Project) BadgeResponse {
	blocks, err := s.Services.Project.GetBlocks(c.Request().Context(), store.BlockQuery{
		ProjectID: p.ID,
	})
	if err != nil {
		return BadgeResponse{
			SchemaVersion: 1,
			Label:         "blocks",
			Message:       "error",
			Color:         "lightgrey",
			CacheSeconds:  60,
		}
	}
	return BadgeResponse{
		SchemaVersion: 1,
		Label:         "blocks",
		Message:       fmt.Sprintf("%d", len(blocks)),
		Color:         "blue",
		CacheSeconds:  3600,
	}
}

func (s *Server) progressBadge(c echo.Context, p *store.Project) BadgeResponse {
	blocks, err := s.Services.Project.GetBlocks(c.Request().Context(), store.BlockQuery{
		ProjectID: p.ID,
	})
	if err != nil || len(blocks) == 0 {
		return BadgeResponse{
			SchemaVersion: 1,
			Label:         "bowrain",
			Message:       "no content",
			Color:         "lightgrey",
			CacheSeconds:  300,
		}
	}

	total := len(blocks)

	// Compute per-locale translation progress.
	localeParts := []string{}
	allComplete := true
	for _, loc := range p.TargetLanguages {
		translated := 0
		for _, b := range blocks {
			if b.Block.TargetText(loc) != "" {
				translated++
			}
		}
		pct := 0
		if total > 0 {
			pct = translated * 100 / total
		}
		if pct < 100 {
			allComplete = false
		}
		localeParts = append(localeParts, fmt.Sprintf("%s %d%%", locale.DisplayName(model.LocaleID(loc)), pct))
	}

	color := "yellow"
	if allComplete {
		color = "brightgreen"
	}

	msg := strings.Join(localeParts, " · ")
	if msg == "" {
		msg = fmt.Sprintf("%d blocks", total)
		color = "blue"
	}

	return BadgeResponse{
		SchemaVersion: 1,
		Label:         "bowrain",
		Message:       msg,
		Color:         color,
		CacheSeconds:  600,
	}
}
