package server

import (
	"context"
	"math"
	"net/http"
	"sort"
	"strconv"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/neokapi/neokapi/bowrain/core/brandscope"
	"github.com/neokapi/neokapi/bowrain/core/store"
	coreprofile "github.com/neokapi/neokapi/core/profile"
)

// Workspace brand-compliance rollup — the all-surfaces board that answers "how
// on-brand is every project right now?" in one matrix. It is a pure aggregation
// over already-stored per-project scores and trends (the same numbers the
// per-project ComplianceOverview shows): nothing is re-scored here.

const (
	defaultRollupLimit = 50
	maxRollupLimit     = 200
)

// BrandRollupEntry is one project's row in the workspace brand-compliance
// rollup. Overall and LastScoredAt are nil when the project has never been
// scored; the effective ProfileID/Name come from the brand-voice resolution
// ladder (#1268), so a project inherits the workspace default when it carries no
// binding of its own.
type BrandRollupEntry struct {
	ProjectID    string                       `json:"project_id"`
	ProjectName  string                       `json:"project_name"`
	ProfileID    string                       `json:"profile_id,omitempty"`
	ProfileName  string                       `json:"profile_name,omitempty"`
	Overall      *int                         `json:"overall"`
	Dimensions   []coreprofile.DimensionScore `json:"dimensions,omitempty"`
	Trend        string                       `json:"trend"` // "up" | "down" | "flat" | "" (no history)
	Drift        *coreprofile.DriftResult     `json:"drift,omitempty"`
	ScoredBlocks int                          `json:"scored_blocks"`
	LastScoredAt *time.Time                   `json:"last_scored_at"`
}

// BrandRollupResponse is the workspace-level brand-compliance rollup: one entry
// per project in the requested page, plus the pagination envelope so a client
// can page through a workspace with many projects.
type BrandRollupResponse struct {
	Projects []BrandRollupEntry `json:"projects"`
	Total    int                `json:"total"`
	Limit    int                `json:"limit"`
	Offset   int                `json:"offset"`
}

// HandleGetBrandVoiceRollup returns the workspace-wide brand-compliance rollup:
// for every project in the workspace, its effective bound profile, latest
// overall score, per-dimension scores, score-trend direction, drift flag, and
// last-scored timestamp — aggregated from stored scores, never recomputed. The
// projects are ordered by name and paginated (?limit,?offset). Drift-window
// settings come from the shared drift query params (?recent_days,?min_score,
// ?drop_points,?days). Gated only by workspace membership, matching the existing
// per-project brand reads.
func (s *Server) HandleGetBrandVoiceRollup(c echo.Context) error {
	if s.BrandStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "brand voice not configured"})
	}
	if s.Services == nil || s.Services.Project == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "store not configured"})
	}

	wsID, _ := c.Get("workspace_id").(string)
	ctx := c.Request().Context()
	limit, offset := rollupPageParams(c)
	cfg, days := driftConfigFromQuery(c)

	allProjects, err := s.Services.Project.ListProjects(ctx)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}

	// Keep only the workspace's own live projects, ordered by name so the page is
	// stable across requests (ID breaks ties for duplicate names).
	wsProjects := make([]*store.Project, 0, len(allProjects))
	for _, p := range allProjects {
		if p == nil || p.WorkspaceID != wsID || p.Archived {
			continue
		}
		wsProjects = append(wsProjects, p)
	}
	sort.SliceStable(wsProjects, func(i, j int) bool {
		if wsProjects[i].Name != wsProjects[j].Name {
			return wsProjects[i].Name < wsProjects[j].Name
		}
		return wsProjects[i].ID < wsProjects[j].ID
	})
	total := len(wsProjects)
	page := paginateProjects(wsProjects, offset, limit)

	// Resolution-ladder inputs: the workspace default rung and the org-scope
	// reads. Both are optional — a missing rung is simply skipped.
	var wd brandscope.WorkspaceDefault
	if s.AuthStore != nil {
		wd = &mcpWorkspaceDefaultAdapter{auth: s.AuthStore}
	}
	var cs brandscope.ScopeStore
	if s.ContentStore != nil {
		cs = s.ContentStore
	}

	entries := make([]BrandRollupEntry, 0, len(page))
	for _, p := range page {
		entries = append(entries, s.brandRollupEntry(ctx, p, wsID, cs, wd, cfg, days))
	}

	return c.JSON(http.StatusOK, BrandRollupResponse{
		Projects: entries,
		Total:    total,
		Limit:    limit,
		Offset:   offset,
	})
}

// brandRollupEntry builds one project's rollup row from stored data: the
// effective profile (resolution ladder), the score/dimension aggregate (stored
// per-block scores), and the trend/drift (daily score trend). Every store read
// is best-effort — a failure on one project degrades that row, never the board.
func (s *Server) brandRollupEntry(
	ctx context.Context,
	p *store.Project,
	wsID string,
	cs brandscope.ScopeStore,
	wd brandscope.WorkspaceDefault,
	cfg coreprofile.DriftConfig,
	days int,
) BrandRollupEntry {
	entry := BrandRollupEntry{ProjectID: p.ID, ProjectName: p.Name}

	// Effective bound profile via the resolution ladder (collection → stream →
	// project → workspace default). Best-effort: an unresolved profile just
	// leaves the column blank.
	if profile, err := brandscope.Resolve(ctx, cs, wd, s.BrandStore, brandscope.Scope{
		ProjectID:   p.ID,
		WorkspaceID: wsID,
	}); err == nil && profile != nil {
		entry.ProfileID = profile.ID
		entry.ProfileName = profile.Name
	}

	// Overall score + per-dimension breakdown from the stored per-block checks —
	// the same rounded-mean aggregation the per-project ComplianceOverview shows.
	if scores, err := s.BrandStore.GetScores(ctx, p.ID, ""); err == nil && len(scores) > 0 {
		overall := rollupAverageScore(scores)
		entry.Overall = &overall
		entry.Dimensions = rollupAggregateDimensions(scores)
		entry.ScoredBlocks = len(scores)
		entry.LastScoredAt = latestCheckedAt(scores)
	}

	// Trend direction + drift from the daily score trend.
	if trends, err := s.BrandStore.GetScoreTrends(ctx, p.ID, days); err == nil && len(trends) > 0 {
		drift := coreprofile.AnalyzeDrift(trends, cfg)
		entry.Drift = &drift
		entry.Trend = rollupTrend(drift)
	}
	return entry
}

// rollupPageParams reads limit/offset from the query, clamping limit to a sane
// range so a single call can't fan out unboundedly across a large workspace.
func rollupPageParams(c echo.Context) (limit, offset int) {
	limit, offset = defaultRollupLimit, 0
	if v := c.QueryParam("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}
	if limit > maxRollupLimit {
		limit = maxRollupLimit
	}
	if v := c.QueryParam("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			offset = n
		}
	}
	return limit, offset
}

// paginateProjects returns the [offset, offset+limit) window of projects,
// clamped to the slice bounds.
func paginateProjects(projects []*store.Project, offset, limit int) []*store.Project {
	if offset >= len(projects) {
		return nil
	}
	end := min(offset+limit, len(projects))
	return projects[offset:end]
}

// rollupAverageScore is the rounded mean of the stored block scores — the
// project's headline compliance number, matching the web dashboard's
// averageScore().
func rollupAverageScore(scores []*coreprofile.StoredScore) int {
	if len(scores) == 0 {
		return 0
	}
	sum := 0
	for _, sc := range scores {
		sum += sc.Score
	}
	return int(math.Round(float64(sum) / float64(len(scores))))
}

// rollupDimensionOrder is the canonical dimension order so a rollup row reads
// the same as every other brand breakdown.
var rollupDimensionOrder = []coreprofile.Dimension{
	coreprofile.DimensionTone,
	coreprofile.DimensionStyle,
	coreprofile.DimensionVocabulary,
	coreprofile.DimensionClarity,
	coreprofile.DimensionBrand,
}

// rollupAggregateDimensions averages each dimension's score and penalty across
// the stored scores, sums issue counts, and returns them in canonical order —
// mirroring the web dashboard's aggregateDimensions().
func rollupAggregateDimensions(scores []*coreprofile.StoredScore) []coreprofile.DimensionScore {
	type acc struct {
		score, penalty, issues, n int
	}
	byDim := make(map[coreprofile.Dimension]*acc)
	for _, sc := range scores {
		for _, dim := range sc.Dimensions {
			a := byDim[dim.Dimension]
			if a == nil {
				a = &acc{}
				byDim[dim.Dimension] = a
			}
			a.score += dim.Score
			a.penalty += dim.Penalty
			a.issues += dim.Issues
			a.n++
		}
	}
	if len(byDim) == 0 {
		return nil
	}
	out := make([]coreprofile.DimensionScore, 0, len(byDim))
	for dim, a := range byDim {
		out = append(out, coreprofile.DimensionScore{
			Dimension: dim,
			Score:     int(math.Round(float64(a.score) / float64(a.n))),
			Penalty:   int(math.Round(float64(a.penalty) / float64(a.n))),
			Issues:    a.issues,
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		return dimRank(out[i].Dimension) < dimRank(out[j].Dimension)
	})
	return out
}

func dimRank(d coreprofile.Dimension) int {
	for i, known := range rollupDimensionOrder {
		if known == d {
			return i
		}
	}
	return len(rollupDimensionOrder)
}

// latestCheckedAt returns the most recent CheckedAt across the stored scores.
func latestCheckedAt(scores []*coreprofile.StoredScore) *time.Time {
	var latest time.Time
	for _, sc := range scores {
		if sc.CheckedAt.After(latest) {
			latest = sc.CheckedAt
		}
	}
	if latest.IsZero() {
		return nil
	}
	return &latest
}

// rollupTrend maps a drift analysis to a coarse trend direction for the matrix:
// the recent window rising a point or more above the baseline reads as "up",
// falling a point or more as "down", otherwise "flat". No recent activity is
// "flat" (nothing moved), and no history at all is "" (unknown), handled by the
// caller.
func rollupTrend(d coreprofile.DriftResult) string {
	if d.RecentCount == 0 {
		return "flat"
	}
	switch diff := d.RecentAvg - d.BaselineAvg; {
	case diff >= 1.0:
		return "up"
	case diff <= -1.0:
		return "down"
	default:
		return "flat"
	}
}
