package server

import (
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"
	"github.com/neokapi/neokapi/bowrain/billing"
)

// ---------------------------------------------------------------------------
// Workspace management
// ---------------------------------------------------------------------------

// HandleAdminListWorkspaces lists all workspaces with plan/usage summary.
// GET /api/admin/workspaces
// Query params: q (search), plan (filter by plan), status (filter by status),
//
//	limit, offset (pagination)
func (s *Server) HandleAdminListWorkspaces(c echo.Context) error {
	if s.BillingStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "billing not configured"})
	}

	limit, _ := strconv.Atoi(c.QueryParam("limit"))
	offset, _ := strconv.Atoi(c.QueryParam("offset"))
	if limit <= 0 {
		limit = 50
	}

	ctx := c.Request().Context()
	subs, err := s.BillingStore.ListSubscriptions(ctx, limit, offset)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}

	// Filter by plan/status if requested.
	planFilter := c.QueryParam("plan")
	statusFilter := c.QueryParam("status")

	var filtered []*billing.Subscription
	for _, sub := range subs {
		if planFilter != "" && string(sub.Plan) != planFilter {
			continue
		}
		if statusFilter != "" && sub.Status != statusFilter {
			continue
		}
		filtered = append(filtered, sub)
	}

	return c.JSON(http.StatusOK, map[string]any{
		"workspaces": filtered,
		"total":      len(filtered),
	})
}

// HandleAdminGetWorkspace returns full workspace detail: subscription, credits, members, usage.
// GET /api/admin/workspaces/:id
func (s *Server) HandleAdminGetWorkspace(c echo.Context) error {
	if s.BillingStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "billing not configured"})
	}

	wsID := c.Param("id")
	ctx := c.Request().Context()

	sub, _ := s.BillingStore.GetSubscription(ctx, wsID)
	alloc, _ := s.BillingStore.GetCurrentAllocation(ctx, wsID)
	overrides, _ := s.BillingStore.GetFeatureOverrides(ctx, wsID)
	notes, _ := s.BillingStore.ListNotes(ctx, wsID)

	result := map[string]any{
		"workspace_id":      wsID,
		"subscription":      sub,
		"credit_allocation": alloc,
		"feature_overrides": overrides,
		"notes":             notes,
	}

	return c.JSON(http.StatusOK, result)
}

// HandleAdminUpdatePlan overrides the plan for a workspace.
// PUT /api/admin/workspaces/:id/plan
func (s *Server) HandleAdminUpdatePlan(c echo.Context) error {
	if s.BillingStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "billing not configured"})
	}

	wsID := c.Param("id")
	adminEmail, _ := c.Get("admin_email").(string)

	var req struct {
		Plan string `json:"plan"`
	}
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "invalid request body"})
	}

	plan := billing.Plan(req.Plan)
	if !billing.ValidPlans[plan] {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "invalid plan: " + req.Plan})
	}

	ctx := c.Request().Context()
	sub, err := s.BillingStore.GetSubscription(ctx, wsID)
	if err != nil {
		// Create a new subscription record.
		sub = &billing.Subscription{
			WorkspaceID: wsID,
			Plan:        plan,
			Status:      "active",
			SeatCount:   1,
		}
	} else {
		sub.Plan = plan
	}

	if err := s.BillingStore.UpsertSubscription(ctx, sub); err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}

	// Record billing event.
	_ = s.BillingStore.RecordBillingEvent(ctx, &billing.BillingEvent{
		WorkspaceID: wsID,
		EventType:   "plan_changed",
		Detail:      "Plan changed to " + req.Plan + " by " + adminEmail,
	})

	return c.JSON(http.StatusOK, sub)
}

// HandleAdminGrantCredits grants bonus credits to a workspace.
// POST /api/admin/workspaces/:id/credits
func (s *Server) HandleAdminGrantCredits(c echo.Context) error {
	if s.BillingStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "billing not configured"})
	}

	wsID := c.Param("id")
	adminEmail, _ := c.Get("admin_email").(string)

	var req struct {
		Amount int64  `json:"amount"`
		Reason string `json:"reason"`
	}
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "invalid request body"})
	}
	if req.Amount <= 0 {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "amount must be positive"})
	}

	ctx := c.Request().Context()
	if err := s.BillingStore.GrantCredits(ctx, wsID, req.Amount, "grant"); err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}

	// Record event with reason.
	_ = s.BillingStore.RecordBillingEvent(ctx, &billing.BillingEvent{
		WorkspaceID: wsID,
		EventType:   "credits_granted",
		Detail:      "Granted " + strconv.FormatInt(req.Amount, 10) + " credits by " + adminEmail + ": " + req.Reason,
	})

	// Also add an internal note.
	_ = s.BillingStore.AddNote(ctx, &billing.WorkspaceNote{
		WorkspaceID: wsID,
		AuthorEmail: adminEmail,
		Content:     "Granted " + strconv.FormatInt(req.Amount, 10) + " credits: " + req.Reason,
	})

	return c.JSON(http.StatusOK, map[string]any{
		"granted": req.Amount,
		"reason":  req.Reason,
	})
}

// ---------------------------------------------------------------------------
// Feature overrides
// ---------------------------------------------------------------------------

// HandleAdminGetFeatureOverrides returns feature overrides for a workspace.
// GET /api/admin/workspaces/:id/feature-overrides
func (s *Server) HandleAdminGetFeatureOverrides(c echo.Context) error {
	if s.BillingStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "billing not configured"})
	}

	wsID := c.Param("id")
	overrides, err := s.BillingStore.GetFeatureOverrides(c.Request().Context(), wsID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}

	return c.JSON(http.StatusOK, map[string]any{
		"overrides": overrides,
	})
}

// HandleAdminSetFeatureOverrides sets a per-workspace feature override.
// PUT /api/admin/workspaces/:id/feature-overrides
func (s *Server) HandleAdminSetFeatureOverrides(c echo.Context) error {
	if s.BillingStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "billing not configured"})
	}

	wsID := c.Param("id")
	adminEmail, _ := c.Get("admin_email").(string)

	var req billing.FeatureOverride
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "invalid request body"})
	}

	req.WorkspaceID = wsID
	req.CreatedBy = adminEmail

	if err := s.BillingStore.SetFeatureOverride(c.Request().Context(), &req); err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}

	// Record billing event.
	action := "enabled"
	if !req.Enabled {
		action = "disabled"
	}
	_ = s.BillingStore.RecordBillingEvent(c.Request().Context(), &billing.BillingEvent{
		WorkspaceID: wsID,
		EventType:   "feature_override",
		Detail:      string(req.Feature) + " " + action + " by " + adminEmail + ": " + req.Reason,
	})

	return c.JSON(http.StatusOK, req)
}

// ---------------------------------------------------------------------------
// Internal notes
// ---------------------------------------------------------------------------

// HandleAdminGetNotes returns internal notes for a workspace.
// GET /api/admin/workspaces/:id/notes
func (s *Server) HandleAdminGetNotes(c echo.Context) error {
	if s.BillingStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "billing not configured"})
	}

	wsID := c.Param("id")
	notes, err := s.BillingStore.ListNotes(c.Request().Context(), wsID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}

	return c.JSON(http.StatusOK, map[string]any{
		"notes": notes,
	})
}

// HandleAdminAddNote adds an internal note to a workspace.
// POST /api/admin/workspaces/:id/notes
func (s *Server) HandleAdminAddNote(c echo.Context) error {
	if s.BillingStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "billing not configured"})
	}

	wsID := c.Param("id")
	adminEmail, _ := c.Get("admin_email").(string)

	var req struct {
		Content string `json:"content"`
	}
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "invalid request body"})
	}
	if req.Content == "" {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "content is required"})
	}

	note := &billing.WorkspaceNote{
		WorkspaceID: wsID,
		AuthorEmail: adminEmail,
		Content:     req.Content,
	}

	if err := s.BillingStore.AddNote(c.Request().Context(), note); err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}

	return c.JSON(http.StatusCreated, note)
}

// ---------------------------------------------------------------------------
// Users
// ---------------------------------------------------------------------------

// HandleAdminListUsers lists/searches users by email.
// GET /api/admin/users?q=search
func (s *Server) HandleAdminListUsers(c echo.Context) error {
	if s.AuthStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "auth store not configured"})
	}

	query := c.QueryParam("q")
	ctx := c.Request().Context()

	// AuthStore currently supports lookup by email only.
	if query == "" {
		return c.JSON(http.StatusOK, map[string]any{
			"users": []any{},
			"total": 0,
		})
	}

	user, err := s.AuthStore.GetUserByEmail(ctx, query)
	if err != nil {
		return c.JSON(http.StatusOK, map[string]any{
			"users": []any{},
			"total": 0,
		})
	}

	return c.JSON(http.StatusOK, map[string]any{
		"users": []any{user},
		"total": 1,
	})
}

// HandleAdminGetUser returns detailed user info with workspace memberships.
// GET /api/admin/users/:id
func (s *Server) HandleAdminGetUser(c echo.Context) error {
	if s.AuthStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "auth store not configured"})
	}

	userID := c.Param("id")
	ctx := c.Request().Context()

	user, err := s.AuthStore.GetUser(ctx, userID)
	if err != nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: "user not found"})
	}

	workspaces, err := s.AuthStore.ListWorkspaces(ctx, userID)
	if err != nil {
		workspaces = nil
	}

	return c.JSON(http.StatusOK, map[string]any{
		"user":       user,
		"workspaces": workspaces,
	})
}

// ---------------------------------------------------------------------------
// Platform-wide endpoints
// ---------------------------------------------------------------------------

// HandleAdminGetMetrics returns platform-wide KPIs.
// GET /api/admin/metrics
func (s *Server) HandleAdminGetMetrics(c echo.Context) error {
	if s.BillingStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "billing not configured"})
	}

	metrics, err := s.BillingStore.GetPlatformMetrics(c.Request().Context())
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}

	return c.JSON(http.StatusOK, metrics)
}

// HandleAdminListEvents returns recent billing events.
// GET /api/admin/events?type=&limit=&offset=
func (s *Server) HandleAdminListEvents(c echo.Context) error {
	if s.BillingStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "billing not configured"})
	}

	limit, _ := strconv.Atoi(c.QueryParam("limit"))
	offset, _ := strconv.Atoi(c.QueryParam("offset"))
	if limit <= 0 {
		limit = 50
	}
	eventType := c.QueryParam("type")

	events, err := s.BillingStore.ListBillingEvents(c.Request().Context(), limit, offset, eventType)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}

	return c.JSON(http.StatusOK, map[string]any{
		"events": events,
	})
}

// HandleAdminGetUpsells returns ranked upsell opportunities.
// GET /api/admin/upsells
func (s *Server) HandleAdminGetUpsells(c echo.Context) error {
	if s.BillingStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "billing not configured"})
	}

	opportunities, err := s.BillingStore.GetUpsellOpportunities(c.Request().Context())
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}

	return c.JSON(http.StatusOK, map[string]any{
		"upsells": opportunities,
	})
}

// HandleAdminListOverrides returns all feature overrides across all workspaces.
// GET /api/admin/overrides
func (s *Server) HandleAdminListOverrides(c echo.Context) error {
	if s.BillingStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "billing not configured"})
	}

	overrides, err := s.BillingStore.ListAllFeatureOverrides(c.Request().Context())
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}

	return c.JSON(http.StatusOK, map[string]any{
		"overrides": overrides,
	})
}
