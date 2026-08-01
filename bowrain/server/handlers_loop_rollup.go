package server

import (
	"net/http"
	"sort"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/neokapi/neokapi/bowrain/core/store"
	bstore "github.com/neokapi/neokapi/bowrain/store"
)

// Workspace loop rollup — the workspace home's cheap aggregate over the loop's
// two per-project blind spots (#1363 follow-up): the most recent convergence
// run anywhere in the workspace, and a ship-state rollup of project-locales.
// One request replaces what would otherwise be a per-project fan-out (up to
// max-projects run-list + dashboard requests) from the dashboard route.
//
// Cost model, by design O(small):
//   - latest run: one indexed query over convergence_runs
//     (idx_convergence_runs_project) scoped to the workspace's project IDs —
//     no per-project queries, no event reads.
//   - ship rollup: a fold over the in-memory dashboard cache
//     (s.dashboardCache) — the per-locale ship states #1342's applyShipStates
//     already derived for the per-project dashboard. Nothing is recomputed:
//     projects without a usable cached rollup are simply not counted, and the
//     payload says so (basis "cached", counted vs total projects). This keeps
//     the workspace home free of any per-block scan.

// loopShipBasisCached labels a ship rollup folded from cached per-project
// dashboard rollups (the only basis currently produced). Consumers must treat
// the counts as covering counted_projects of total_projects, not the whole
// workspace.
const loopShipBasisCached = "cached"

// loopRollupCacheGrace bounds how long past its dashboard TTL a cached rollup
// may still count toward the ship rollup. Cache entries are deleted on every
// content change this instance observes, so a surviving entry is current for
// this instance; the grace only bounds cross-instance staleness (a write
// served by another replica) to a window that is honest for a coarse
// workspace-home card without forcing a recompute.
const loopRollupCacheGrace = 10 * time.Minute

// loopRollupRunView is the wire shape of the workspace's most recent
// convergence run within the loop rollup.
type loopRollupRunView struct {
	ID          string `json:"id"`
	ProjectID   string `json:"project_id"`
	ProjectName string `json:"project_name,omitempty"`
	// Stream is the project's default stream — the run itself is
	// project-scoped, but every deep link into the project is stream-scoped.
	Stream string `json:"stream,omitempty"`
	State  string `json:"state"`
	// StallReason labels why a parked/failed run did not converge
	// (needs_credits | source_not_ready | …). Empty on converged/running.
	StallReason string `json:"stall_reason,omitempty"`
	Trigger     string `json:"trigger,omitempty"`
	CreatedAt   string `json:"created_at,omitempty"`
	FinishedAt  string `json:"finished_at,omitempty"`
	// UpdatedAt is the run's last observable progress: last_activity when the
	// run heart-beats, else finished_at, else created_at.
	UpdatedAt string `json:"updated_at,omitempty"`
}

// loopRollupShipProject is one counted project's contribution to the ship
// rollup — kept so the client can deep-link a representative project (the
// list is bounded by the projects with a cached rollup, itself bounded by the
// workspace's project count).
type loopRollupShipProject struct {
	ProjectID   string `json:"project_id"`
	ProjectName string `json:"project_name,omitempty"`
	Stream      string `json:"stream,omitempty"`
	Governed    int    `json:"governed"`
	AIShippable int    `json:"ai_shippable"`
	Pending     int    `json:"pending"`
}

// loopRollupShipView is the workspace ship-state rollup: counts of
// project-locales per derived ship state, over the projects whose dashboard
// rollup is cached (basis "cached"). counted_projects < total_projects means
// the numbers cover only part of the workspace — the client must present them
// as such.
type loopRollupShipView struct {
	Basis           string                  `json:"basis"`
	Governed        int                     `json:"governed"`
	AIShippable     int                     `json:"ai_shippable"`
	Pending         int                     `json:"pending"`
	CountedProjects int                     `json:"counted_projects"`
	TotalProjects   int                     `json:"total_projects"`
	Projects        []loopRollupShipProject `json:"projects,omitempty"`
}

// loopRollupResponse is the GET /:ws/loop-rollup payload. Both fields are
// optional: an absent latest_run means the workspace has never run, an absent
// ship means no project currently has a cached dashboard rollup to fold —
// the client hides the corresponding card rather than showing a guess.
type loopRollupResponse struct {
	LatestRun *loopRollupRunView  `json:"latest_run,omitempty"`
	Ship      *loopRollupShipView `json:"ship,omitempty"`
}

// projectDefaultStream is the stream every workspace-home deep link targets
// for a project.
func projectDefaultStream(p *store.Project) string {
	if p != nil && p.DefaultStream != "" {
		return p.DefaultStream
	}
	return "main"
}

func toLoopRollupRunView(run *bstore.ConvergenceRun, proj *store.Project) *loopRollupRunView {
	v := &loopRollupRunView{
		ID:          run.ID,
		ProjectID:   run.ProjectID,
		Stream:      projectDefaultStream(proj),
		State:       run.State,
		StallReason: run.StallReason,
		Trigger:     run.Trigger,
	}
	if proj != nil {
		v.ProjectName = proj.Name
	}
	if !run.CreatedAt.IsZero() {
		v.CreatedAt = run.CreatedAt.UTC().Format(time.RFC3339)
		v.UpdatedAt = v.CreatedAt
	}
	if run.FinishedAt != nil && !run.FinishedAt.IsZero() {
		v.FinishedAt = run.FinishedAt.UTC().Format(time.RFC3339)
		v.UpdatedAt = v.FinishedAt
	}
	if run.LastActivity != nil && !run.LastActivity.IsZero() {
		v.UpdatedAt = run.LastActivity.UTC().Format(time.RFC3339)
	}
	return v
}

// HandleWorkspaceLoopRollup answers GET /:ws/loop-rollup — see the package
// comment above for the shape and the cost model. Auth + membership are
// enforced by the workspace route-group middleware, same as the projects list
// this rollup complements.
func (s *Server) HandleWorkspaceLoopRollup(c echo.Context) error {
	if s.Services == nil {
		return apiErr(c, http.StatusServiceUnavailable, "store not configured")
	}
	workspaceID, _ := c.Get("workspace_id").(string)
	ctx := c.Request().Context()

	allProjects, err := s.Services.Project.ListProjects(ctx)
	if err != nil {
		return serverErr(c, err)
	}
	projects := make([]*store.Project, 0, len(allProjects))
	for _, p := range allProjects {
		if p.WorkspaceID == workspaceID {
			projects = append(projects, p)
		}
	}

	resp := loopRollupResponse{}

	if len(projects) > 0 && s.ConvergenceRunStore != nil {
		ids := make([]string, len(projects))
		byID := make(map[string]*store.Project, len(projects))
		for i, p := range projects {
			ids[i] = p.ID
			byID[p.ID] = p
		}
		// Fail-soft: a run-store hiccup degrades the card, never the rollup.
		if run, err := s.ConvergenceRunStore.LatestRunForProjects(ctx, ids); err == nil && run != nil {
			resp.LatestRun = toLoopRollupRunView(run, byID[run.ProjectID])
		}
	}

	resp.Ship = s.buildLoopShipRollup(workspaceID, projects)

	return c.JSON(http.StatusOK, resp)
}

// buildLoopShipRollup folds the workspace ship rollup from the cached
// per-project dashboard rollups (each already carrying the per-locale ship
// states applyShipStates derived). Returns nil when no project has a usable
// cached rollup — the honest "no data" answer; nothing is recomputed here.
func (s *Server) buildLoopShipRollup(workspaceID string, projects []*store.Project) *loopRollupShipView {
	ship := &loopRollupShipView{Basis: loopShipBasisCached, TotalProjects: len(projects)}
	for _, p := range projects {
		stream := projectDefaultStream(p)
		cached, ok := s.dashboardCache.Load(dashboardCacheKey(workspaceID, p.ID, stream))
		if !ok {
			continue
		}
		entry, ok := cached.(*dashboardCacheEntry)
		if !ok || entry.stats == nil || time.Since(entry.expiresAt) > loopRollupCacheGrace {
			continue
		}
		row := loopRollupShipProject{ProjectID: p.ID, ProjectName: p.Name, Stream: stream}
		for _, ls := range entry.stats.LocaleStats {
			switch ls.ShipState {
			case store.ShipStateGoverned:
				row.Governed++
			case store.ShipStateAIShippable:
				row.AIShippable++
			default:
				// Pending, plus any future/unknown state: not shippable.
				row.Pending++
			}
		}
		ship.Governed += row.Governed
		ship.AIShippable += row.AIShippable
		ship.Pending += row.Pending
		ship.CountedProjects++
		ship.Projects = append(ship.Projects, row)
	}
	if ship.CountedProjects == 0 {
		return nil
	}
	// Most pending first (name-tied for determinism): the client links its
	// ship card at Projects[0], the project with the most catch-up to review.
	sort.SliceStable(ship.Projects, func(i, j int) bool {
		if ship.Projects[i].Pending != ship.Projects[j].Pending {
			return ship.Projects[i].Pending > ship.Projects[j].Pending
		}
		return ship.Projects[i].ProjectName < ship.Projects[j].ProjectName
	})
	return ship
}
