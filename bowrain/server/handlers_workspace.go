package server

import (
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/neokapi/neokapi/bowrain/analytics"
	"github.com/neokapi/neokapi/bowrain/auth"
	"github.com/neokapi/neokapi/bowrain/billing"
	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	platev "github.com/neokapi/neokapi/bowrain/core/event"
	"github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/core/model"
)

// WorkspaceRequest is the request body for creating/updating a workspace.
type WorkspaceRequest struct {
	Name string `json:"name"`
	Slug string `json:"slug"`
	// Description is a pointer so an omitted field leaves the value untouched,
	// while an explicit "" clears the description (rename form can blank it).
	Description         *string                    `json:"description,omitempty"`
	LogoURL             string                     `json:"logo_url,omitempty"`
	DashboardVisibility string                     `json:"dashboard_visibility,omitempty"`
	PulseTermSources    *platauth.PulseTermSources `json:"pulse_term_sources,omitempty"`
	// PreferredModel selects a platform AI model for this workspace. A pointer
	// so an omitted field leaves the value untouched; an explicit "" resets to
	// the platform default. Honored only when the admin has enabled customer
	// model choice and the model is in the enabled set.
	PreferredModel *string `json:"preferred_model,omitempty"`
	// BrandVoiceProfileID is the workspace-level default brand-voice profile —
	// the base rung of the hierarchical resolver that a project/stream/collection
	// binding overrides. A pointer so an omitted field is untouched; an explicit
	// "" clears the default.
	BrandVoiceProfileID *string `json:"brand_voice_profile_id,omitempty"`
}

// MemberRequest is the request body for adding a member to a workspace.
type MemberRequest struct {
	UserID string `json:"user_id"`
	Role   string `json:"role"`
}

// RoleUpdateRequest is the request body for updating a member's role.
type RoleUpdateRequest struct {
	Role string `json:"role"`
}

func (s *Server) HandleCreateWorkspace(c echo.Context) error {
	if s.AuthStore == nil {
		return apiErr(c, http.StatusServiceUnavailable, "auth not configured")
	}
	// Signups gate (ctrl-managed): when an admin closes signups, block the
	// self-serve workspace-creation entry point.
	if !s.PlatformConfig.SignupsOpen() {
		return apiErr(c, http.StatusForbidden, "signups are currently closed")
	}

	var req WorkspaceRequest
	if err := c.Bind(&req); err != nil {
		return apiErr(c, http.StatusBadRequest, err.Error())
	}
	if req.Name == "" {
		return apiErr(c, http.StatusBadRequest, "name is required")
	}
	if req.Slug == "" {
		return apiErr(c, http.StatusBadRequest, "slug is required")
	}

	description := ""
	if req.Description != nil {
		description = *req.Description
	}
	w := &platauth.Workspace{
		Name:        req.Name,
		Slug:        req.Slug,
		Description: description,
		LogoURL:     req.LogoURL,
	}

	// Add the creator as owner of the new workspace.
	userID, _ := c.Get("user_id").(string)
	ctx := c.Request().Context()
	if s.Services != nil && s.Services.Auth != nil && userID != "" {
		if err := s.Services.Auth.CreateWorkspaceWithOwner(ctx, w, userID); err != nil {
			return apiErr(c, http.StatusInternalServerError, err.Error())
		}
		w.Role = platauth.RoleOwner
	} else {
		if err := s.AuthStore.CreateWorkspace(ctx, w); err != nil {
			return apiErr(c, http.StatusInternalServerError, err.Error())
		}
	}

	// Every new workspace receives its one-time trial credit grant (idempotent,
	// once per workspace, ever) — the AI credits it lives on until it converts
	// to a paid plan with a monthly allowance.
	billing.EnsureTrialGrant(ctx, s.BillingStore, w.ID)

	// Set up the trial for new workspaces using the ctrl-managed new-workspace
	// defaults (plan + trial length), so an admin can change them without a
	// redeploy. Falls back to the product defaults (Pro, 14 days) when unset.
	// The trial grants plan FEATURES only; its credits are the grant above.
	var planSyncer billing.WorkspacePlanSyncer
	if s.AuthStore != nil {
		planSyncer = s.planSyncer()
	}
	billing.SetupTrialWith(ctx, s.BillingStore, w.ID,
		billing.Plan(s.PlatformConfig.DefaultPlan()), s.PlatformConfig.TrialDays(), planSyncer)

	// The trial is local and card-free, so it never surfaces as a Stripe
	// webhook — this call site is where trial_started is known (epic 018).
	if s.BillingStore != nil {
		props := analytics.Props(w.ID, "")
		props["plan"] = s.PlatformConfig.DefaultPlan()
		props["trial_days"] = s.PlatformConfig.TrialDays()
		s.trackEvent(userID, analytics.EventTrialStarted, props)
	}

	// Re-read workspace to reflect synced plan in the response.
	if updated, err := s.AuthStore.GetWorkspace(ctx, w.ID); err == nil {
		w = updated
	}
	w.Role = platauth.RoleOwner

	return c.JSON(http.StatusCreated, w)
}

func (s *Server) HandleListWorkspaces(c echo.Context) error {
	if s.AuthStore == nil {
		return apiErr(c, http.StatusServiceUnavailable, "auth not configured")
	}
	userID := c.Get("user_id")
	if userID == nil {
		return apiErr(c, http.StatusUnauthorized, "not authenticated")
	}
	workspaces, err := s.AuthStore.ListWorkspaces(c.Request().Context(), userID.(string))
	if err != nil {
		return apiErr(c, http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, workspaces)
}

func (s *Server) HandleGetWorkspace(c echo.Context) error {
	if s.AuthStore == nil {
		return apiErr(c, http.StatusServiceUnavailable, "auth not configured")
	}
	w, err := s.AuthStore.GetWorkspaceBySlug(c.Request().Context(), c.Param("ws"))
	if err != nil {
		return apiErr(c, http.StatusNotFound, err.Error())
	}
	// Enrich with the current user's role if available.
	if userID, ok := c.Get("user_id").(string); ok && userID != "" {
		if m, err := s.AuthStore.GetMembership(c.Request().Context(), w.ID, userID); err == nil {
			w.Role = m.Role
		}
	}
	return c.JSON(http.StatusOK, workspaceWithFeatures{
		Workspace: w,
		Features:  s.resolveWorkspaceFeatures(c, w.Plan),
	})
}

// workspaceWithFeatures augments a workspace response with the caller's resolved
// feature entitlements, so the web app can gate UI (e.g. the @bravo surface) on
// the same source of truth the server enforces with PlanGuard. Embedding the
// workspace pointer keeps every existing field in the JSON and just adds
// "features".
type workspaceWithFeatures struct {
	*platauth.Workspace
	Features map[string]bool `json:"features"`
}

// resolveWorkspaceFeatures computes the caller's effective entitlements for the
// workspace, honoring the per-workspace overrides loaded onto the context by
// FeatureOverridesMiddleware. Only client-gated features are surfaced.
func (s *Server) resolveWorkspaceFeatures(c echo.Context, plan string) map[string]bool {
	overrides := billing.OverridesFromContext(c)
	p := billing.Plan(plan)
	surfaced := []billing.Feature{billing.FeatureBravo}
	features := make(map[string]bool, len(surfaced))
	for _, f := range surfaced {
		features[string(f)] = billing.HasFeature(p, f, overrides)
	}
	return features
}

func (s *Server) HandleUpdateWorkspace(c echo.Context) error {
	if s.AuthStore == nil {
		return apiErr(c, http.StatusServiceUnavailable, "auth not configured")
	}

	// Verify the calling user has admin or owner role.
	if err := s.requireRole(c, platauth.RoleAdmin, platauth.RoleOwner); err != nil {
		return err
	}

	var req WorkspaceRequest
	if err := c.Bind(&req); err != nil {
		return apiErr(c, http.StatusBadRequest, err.Error())
	}

	// Look up workspace by slug.
	w, err := s.AuthStore.GetWorkspaceBySlug(c.Request().Context(), c.Param("ws"))
	if err != nil {
		return apiErr(c, http.StatusNotFound, err.Error())
	}

	if req.Name != "" {
		w.Name = req.Name
	}
	oldSlug := w.Slug
	slugChanged := req.Slug != "" && req.Slug != oldSlug
	if slugChanged {
		ctx := c.Request().Context()
		if err := platauth.ValidateWorkspaceSlug(req.Slug); err != nil {
			return apiErr(c, http.StatusBadRequest, err.Error())
		}
		if existing, err := s.AuthStore.GetWorkspaceBySlug(ctx, req.Slug); err == nil && existing.ID != w.ID {
			return apiErr(c, http.StatusConflict, "slug is already in use")
		}
		if _, _, reserved, err := s.AuthStore.IsSlugReserved(ctx, req.Slug); err != nil {
			return apiErr(c, http.StatusInternalServerError, "check reserved slug: "+err.Error())
		} else if reserved {
			return apiErr(c, http.StatusConflict, "slug is reserved from a recent rename")
		}
		w.Slug = req.Slug
	}
	if req.Description != nil {
		w.Description = *req.Description
	}
	if req.LogoURL != "" {
		w.LogoURL = req.LogoURL
	}
	if req.DashboardVisibility != "" {
		if platauth.ValidDashboardVisibility[platauth.DashboardVisibility(req.DashboardVisibility)] {
			newVis := platauth.DashboardVisibility(req.DashboardVisibility)
			if w.Type == platauth.WorkspaceTypePersonal && newVis != platauth.DashboardPrivate {
				return apiErr(c, http.StatusForbidden, "personal workspaces cannot be exposed publicly")
			}
			w.DashboardVisibility = newVis
			// Auto-generate an access key when switching to unlisted (if none exists).
			if newVis == platauth.DashboardUnlisted && w.PulseAccessKey == "" {
				b := make([]byte, 16)
				if _, err := rand.Read(b); err != nil {
					return apiErr(c, http.StatusInternalServerError, "generate access key: "+err.Error())
				}
				w.PulseAccessKey = hex.EncodeToString(b)
			}
		}
	}
	if req.PulseTermSources != nil {
		w.PulseTermSources = *req.PulseTermSources
	}
	if req.PreferredModel != nil {
		pref := *req.PreferredModel
		if pref != "" {
			// Only accept a preferred model when the admin has opened model
			// choice to customers and the chosen model is in the enabled set.
			if !s.PlatformConfig.AICustomerChoice() {
				return apiErr(c, http.StatusForbidden, "model choice is not enabled for this platform")
			}
			if !s.PlatformConfig.IsModelEnabled(pref) {
				return apiErr(c, http.StatusBadRequest, "model is not available")
			}
		}
		w.PreferredModel = pref
	}
	if req.BrandVoiceProfileID != nil {
		w.BrandVoiceProfileID = *req.BrandVoiceProfileID
	}
	if err := s.AuthStore.UpdateWorkspace(c.Request().Context(), w); err != nil {
		return apiErr(c, http.StatusInternalServerError, "update workspace: "+err.Error())
	}
	// Reserve the old slug for the configured grace period so it cannot be
	// reused for impersonation. Reservation failure does not undo the rename
	// — log and continue; PurgeExpiredSlugReservations will GC stale entries.
	if slugChanged {
		until := time.Now().UTC().Add(auth.SlugReservationWindow)
		if err := s.AuthStore.ReserveSlug(c.Request().Context(), w.ID, oldSlug, until); err != nil {
			slog.WarnContext(c.Request().Context(), "reserve old workspace slug", "workspace_id", w.ID, "old_slug", oldSlug, "error", err)
		}
	}
	// Enrich with the calling user's role so the frontend stays consistent.
	if userID, ok := c.Get("user_id").(string); ok && userID != "" {
		if m, mErr := s.AuthStore.GetMembership(c.Request().Context(), w.ID, userID); mErr == nil {
			w.Role = m.Role
		}
	}
	return c.JSON(http.StatusOK, w)
}

func (s *Server) HandleDeleteWorkspace(c echo.Context) error {
	if s.AuthStore == nil {
		return apiErr(c, http.StatusServiceUnavailable, "auth not configured")
	}

	// Verify the calling user has owner role.
	if err := s.requireRole(c, platauth.RoleOwner); err != nil {
		return err
	}

	w, err := s.AuthStore.GetWorkspaceBySlug(c.Request().Context(), c.Param("ws"))
	if err != nil {
		return apiErr(c, http.StatusNotFound, err.Error())
	}
	if w.Type == platauth.WorkspaceTypePersonal {
		return apiErr(c, http.StatusForbidden, "cannot delete personal workspace")
	}
	if err := s.AuthStore.DeleteWorkspace(c.Request().Context(), w.ID); err != nil {
		return apiErr(c, http.StatusInternalServerError, err.Error())
	}
	return c.NoContent(http.StatusNoContent)
}

func (s *Server) HandleListMembers(c echo.Context) error {
	if s.AuthStore == nil {
		return apiErr(c, http.StatusServiceUnavailable, "auth not configured")
	}
	w, err := s.AuthStore.GetWorkspaceBySlug(c.Request().Context(), c.Param("ws"))
	if err != nil {
		return apiErr(c, http.StatusNotFound, err.Error())
	}
	members, err := s.AuthStore.ListMembers(c.Request().Context(), w.ID)
	if err != nil {
		return apiErr(c, http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, members)
}

func (s *Server) HandleAddMember(c echo.Context) error {
	if err := s.requireRole(c, platauth.RoleAdmin, platauth.RoleOwner); err != nil {
		return err
	}
	if s.AuthStore == nil {
		return apiErr(c, http.StatusServiceUnavailable, "auth not configured")
	}

	var req MemberRequest
	if err := c.Bind(&req); err != nil {
		return apiErr(c, http.StatusBadRequest, err.Error())
	}

	ctx := c.Request().Context()
	w, err := s.AuthStore.GetWorkspaceBySlug(ctx, c.Param("ws"))
	if err != nil {
		return apiErr(c, http.StatusNotFound, err.Error())
	}

	// Enforce seat limit based on workspace plan.
	if w.Plan != "" {
		limit := billing.GetLimit(billing.Plan(w.Plan), "max-seats")
		if limit > 0 {
			members, err := s.AuthStore.ListMembers(ctx, w.ID)
			if err == nil && len(members) >= limit {
				return apiErr(c, http.StatusForbidden, "seat_limit_reached", map[string]any{
					"current": len(members),
					"limit":   limit,
				})
			}
		}
	}

	role := platauth.Role(req.Role)
	if err := s.AuthStore.AddMember(ctx, w.ID, req.UserID, role); err != nil {
		return apiErr(c, http.StatusBadRequest, err.Error())
	}
	s.emitAudit(c, auditEvent{
		Type:         platev.EventMemberAdded,
		WorkspaceID:  w.ID,
		ResourceType: "workspace_member",
		ResourceID:   req.UserID,
		Data:         map[string]string{"role": string(role), "scope": "workspace"},
	})
	return c.JSON(http.StatusCreated, map[string]string{"status": "added"})
}

func (s *Server) HandleUpdateMemberRole(c echo.Context) error {
	if err := s.requireRole(c, platauth.RoleAdmin, platauth.RoleOwner); err != nil {
		return err
	}
	if s.AuthStore == nil {
		return apiErr(c, http.StatusServiceUnavailable, "auth not configured")
	}

	var req RoleUpdateRequest
	if err := c.Bind(&req); err != nil {
		return apiErr(c, http.StatusBadRequest, err.Error())
	}

	w, err := s.AuthStore.GetWorkspaceBySlug(c.Request().Context(), c.Param("ws"))
	if err != nil {
		return apiErr(c, http.StatusNotFound, err.Error())
	}

	role := platauth.Role(req.Role)
	if err := s.AuthStore.UpdateRole(c.Request().Context(), w.ID, c.Param("uid"), role); err != nil {
		return apiErr(c, http.StatusBadRequest, err.Error())
	}
	s.emitAudit(c, auditEvent{
		Type:         platev.EventMemberRoleChanged,
		WorkspaceID:  w.ID,
		ResourceType: "workspace_member",
		ResourceID:   c.Param("uid"),
		After:        map[string]string{"role": string(role), "scope": "workspace"},
	})
	return c.JSON(http.StatusOK, map[string]string{"status": "updated"})
}

func (s *Server) HandleRemoveMember(c echo.Context) error {
	if err := s.requireRole(c, platauth.RoleAdmin, platauth.RoleOwner); err != nil {
		return err
	}
	if s.AuthStore == nil {
		return apiErr(c, http.StatusServiceUnavailable, "auth not configured")
	}
	w, err := s.AuthStore.GetWorkspaceBySlug(c.Request().Context(), c.Param("ws"))
	if err != nil {
		return apiErr(c, http.StatusNotFound, err.Error())
	}
	if err := s.AuthStore.RemoveMember(c.Request().Context(), w.ID, c.Param("uid")); err != nil {
		return apiErr(c, http.StatusNotFound, err.Error())
	}
	s.emitAudit(c, auditEvent{
		Type:         platev.EventMemberRemoved,
		WorkspaceID:  w.ID,
		ResourceType: "workspace_member",
		ResourceID:   c.Param("uid"),
		Data:         map[string]string{"scope": "workspace"},
	})
	return c.NoContent(http.StatusNoContent)
}

// HandleListWorkspaceProjects lists projects in a workspace, filtered by
// workspace_id. The default response is a summary view: each project carries
// precomputed aggregates (item_count, block_count, word_count, stream_count)
// and an empty items[] — the dashboard cards and stats bar render from the
// aggregates, so the list never ships every file row of every project.
// `?view=full` restores the legacy shape with the embedded per-item array
// (block/word counts per file) for callers that need it.
func (s *Server) HandleListWorkspaceProjects(c echo.Context) error {
	if s.Services == nil {
		return apiErr(c, http.StatusServiceUnavailable, "store not configured")
	}
	workspaceID, _ := c.Get("workspace_id").(string)
	fullView := c.QueryParam("view") == "full"
	ctx := c.Request().Context()
	allProjects, err := s.Services.Project.ListProjects(ctx)
	if err != nil {
		return apiErr(c, http.StatusInternalServerError, err.Error())
	}
	result := make([]*ProjectInfoResponse, 0)
	for _, p := range allProjects {
		if p.WorkspaceID != workspaceID {
			continue
		}
		info := projectToInfoResponse(p)
		if s.ContentStore != nil {
			stream := streamParamWithProject(c, p)
			// Fail-soft: a broken item must not blank the whole dashboard.
			if fullView {
				if items, _, err := editorBuildProjectItems(ctx, s.ContentStore, p.ID, stream); err == nil {
					info.Items = items
					info.ItemCount = len(items)
					for _, it := range items {
						info.BlockCount += it.BlockCount
						info.WordCount += it.WordCount
					}
				}
			} else if ic, bc, wc, err := editorBuildProjectSummary(ctx, s.ContentStore, p.ID, stream); err == nil {
				info.ItemCount, info.BlockCount, info.WordCount = ic, bc, wc
			}
			if streams, err := s.ContentStore.ListStreams(ctx, p.ID, false); err == nil {
				info.StreamCount = len(streams)
			}
		}
		result = append(result, info)
	}
	return c.JSON(http.StatusOK, result)
}

// HandleCreateWorkspaceProject creates a project in a workspace.
func (s *Server) HandleCreateWorkspaceProject(c echo.Context) error {
	if s.Services == nil {
		return apiErr(c, http.StatusServiceUnavailable, "store not configured")
	}

	var req ProjectRequest
	if err := c.Bind(&req); err != nil {
		return apiErr(c, http.StatusBadRequest, err.Error())
	}

	locales := make([]model.LocaleID, len(req.TargetLanguages))
	for i, l := range req.TargetLanguages {
		locales[i] = model.LocaleID(l)
	}

	workspaceID, _ := c.Get("workspace_id").(string)
	ctx := c.Request().Context()

	// Enforce project limit based on workspace plan.
	plan, _ := c.Get("workspace_plan").(string)
	if plan != "" {
		limit := billing.GetLimit(billing.Plan(plan), "max-projects")
		if limit > 0 {
			allProjects, err := s.Services.Project.ListProjects(ctx)
			if err == nil {
				count := 0
				for _, p := range allProjects {
					if p.WorkspaceID == workspaceID {
						count++
					}
				}
				if count >= limit {
					return apiErr(c, http.StatusForbidden, "project_limit_reached", map[string]any{
						"current": count,
						"limit":   limit,
					})
				}
			}
		}
	}

	p := &store.Project{
		Name:                  req.Name,
		DefaultSourceLanguage: model.LocaleID(req.DefaultSourceLanguage),
		TargetLanguages:       locales,
		WorkspaceID:           workspaceID,
	}
	if err := s.Services.Project.CreateProject(ctx, p); err != nil {
		return apiErr(c, http.StatusInternalServerError, err.Error())
	}
	if s.ContentStore != nil {
		_ = EnsureDefaultCollection(ctx, s.ContentStore, p.ID)
		_ = EnsureMainStream(ctx, s.ContentStore, p.ID)
	}
	return c.JSON(http.StatusCreated, p)
}
