package server

import (
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/neokapi/neokapi/bowrain/billing"
	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	"github.com/neokapi/neokapi/bowrain/jobs"
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

	// Enrich with workspace metadata from AuthStore.
	ownerResolver := &ownerEmailResolver{authStore: s.AuthStore}
	type adminWorkspace struct {
		ID                 string  `json:"id"`
		Name               string  `json:"name"`
		Slug               string  `json:"slug"`
		OwnerEmail         string  `json:"owner_email"`
		Plan               string  `json:"plan"`
		Status             string  `json:"status"`
		CreditUsagePercent float64 `json:"credit_usage_percent"`
		CreditsUsed        int64   `json:"credits_used"`
		CreditsTotal       int64   `json:"credits_total"`
		MemberCount        int     `json:"member_count"`
		CreatedAt          string  `json:"created_at"`
	}

	workspaces := make([]adminWorkspace, 0, len(filtered))
	for _, sub := range filtered {
		aw := adminWorkspace{
			ID:        sub.WorkspaceID,
			Plan:      string(sub.Plan),
			Status:    sub.Status,
			CreatedAt: sub.CreatedAt.Format(time.RFC3339),
		}

		// Enrich from AuthStore workspace metadata.
		if s.AuthStore != nil {
			if ws, err := s.AuthStore.GetWorkspace(ctx, sub.WorkspaceID); err == nil {
				aw.Name = ws.Name
				aw.Slug = ws.Slug
			}
			if members, err := s.AuthStore.ListMembers(ctx, sub.WorkspaceID); err == nil {
				aw.MemberCount = len(members)
			}
			aw.OwnerEmail = ownerResolver.GetOwnerEmail(ctx, sub.WorkspaceID)
		}

		// Enrich with credit data.
		if alloc, err := s.BillingStore.GetCurrentAllocation(ctx, sub.WorkspaceID); err == nil && alloc != nil {
			aw.CreditsUsed = alloc.CreditsUsed
			aw.CreditsTotal = alloc.CreditsTotal
			if alloc.CreditsTotal > 0 {
				aw.CreditUsagePercent = float64(alloc.CreditsUsed) / float64(alloc.CreditsTotal) * 100
			}
		}

		workspaces = append(workspaces, aw)
	}

	return c.JSON(http.StatusOK, map[string]any{
		"workspaces": workspaces,
		"total":      len(workspaces),
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

	// Build flat response matching AdminWorkspaceDetail frontend type.
	result := map[string]any{
		"id":     wsID,
		"plan":   "",
		"status": "",
	}

	if sub != nil {
		result["plan"] = sub.Plan
		result["status"] = sub.Status
		result["stripe_customer_id"] = nilIfEmpty(sub.StripeCustomerID)
		result["stripe_subscription_id"] = nilIfEmpty(sub.StripeSubscriptionID)
		result["seat_count"] = sub.SeatCount
		result["created_at"] = sub.CreatedAt.Format(time.RFC3339)
		if !sub.CurrentPeriodStart.IsZero() {
			result["current_period_start"] = sub.CurrentPeriodStart.Format(time.RFC3339)
		}
		if !sub.CurrentPeriodEnd.IsZero() {
			result["current_period_end"] = sub.CurrentPeriodEnd.Format(time.RFC3339)
		}
		if sub.CancelAt != nil {
			result["cancel_at"] = sub.CancelAt.Format(time.RFC3339)
		}
	}

	// Credit summary.
	var creditsUsed, creditsTotal int64
	var creditUsagePercent float64
	if alloc != nil {
		creditsUsed = alloc.CreditsUsed
		creditsTotal = alloc.CreditsTotal
		if alloc.CreditsTotal > 0 {
			creditUsagePercent = float64(alloc.CreditsUsed) / float64(alloc.CreditsTotal) * 100
		}
	}
	result["credits_used"] = creditsUsed
	result["credits_total"] = creditsTotal
	result["credit_usage_percent"] = creditUsagePercent

	// Workspace metadata + members from AuthStore.
	type memberEntry struct {
		UserID   string `json:"user_id"`
		Email    string `json:"email"`
		Name     string `json:"name"`
		Role     string `json:"role"`
		JoinedAt string `json:"joined_at"`
	}

	var members []memberEntry
	if s.AuthStore != nil {
		if ws, err := s.AuthStore.GetWorkspace(ctx, wsID); err == nil {
			result["name"] = ws.Name
			result["slug"] = ws.Slug
		}

		if mems, err := s.AuthStore.ListMembers(ctx, wsID); err == nil {
			result["member_count"] = len(mems)
			for _, m := range mems {
				entry := memberEntry{
					UserID:   m.UserID,
					Role:     string(m.Role),
					JoinedAt: m.JoinedAt.Format(time.RFC3339),
				}
				if u, err := s.AuthStore.GetUser(ctx, m.UserID); err == nil {
					entry.Email = u.Email
					entry.Name = u.Name
				}
				members = append(members, entry)
			}
		}

		ownerResolver := &ownerEmailResolver{authStore: s.AuthStore}
		result["owner_email"] = ownerResolver.GetOwnerEmail(ctx, wsID)
	}
	if members == nil {
		members = []memberEntry{}
	}
	result["members"] = members

	// Recent activity from billing events.
	type activityEntry struct {
		ID          string `json:"id"`
		Type        string `json:"type"`
		Description string `json:"description"`
		CreatedAt   string `json:"created_at"`
	}
	var activity []activityEntry
	if events, err := s.BillingStore.ListBillingEvents(ctx, 10, 0, ""); err == nil {
		for _, evt := range events {
			if evt.WorkspaceID == wsID {
				activity = append(activity, activityEntry{
					ID:          strconv.FormatInt(evt.ID, 10),
					Type:        evt.EventType,
					Description: evt.Detail,
					CreatedAt:   evt.CreatedAt.Format(time.RFC3339),
				})
			}
		}
	}
	if activity == nil {
		activity = []activityEntry{}
	}
	result["recent_activity"] = activity

	return c.JSON(http.StatusOK, result)
}

// HandleAdminGetLedger returns the credit ledger for a workspace.
// GET /api/admin/workspaces/:id/ledger
func (s *Server) HandleAdminGetLedger(c echo.Context) error {
	if s.BillingStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "billing not configured"})
	}

	wsID := c.Param("id")
	ctx := c.Request().Context()

	// Default to last 90 days.
	to := time.Now().UTC()
	from := to.AddDate(0, -3, 0)

	entries, err := s.BillingStore.GetLedger(ctx, wsID, from, to)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}
	if entries == nil {
		entries = []billing.LedgerEntry{}
	}

	return c.JSON(http.StatusOK, entries)
}

// HandleAdminGetModelUsage returns AI token usage grouped by model and operation.
// GET /api/admin/workspaces/:id/model-usage
func (s *Server) HandleAdminGetModelUsage(c echo.Context) error {
	if s.QuotaStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "usage tracking not configured"})
	}

	pgStore, ok := s.QuotaStore.(*jobs.PgQuotaStore)
	if !ok {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "model usage not available"})
	}

	wsID := c.Param("id")
	ctx := c.Request().Context()

	now := time.Now().UTC()
	from := billing.WeekStart(now)
	to := now

	if v := c.QueryParam("from"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			from = t
		}
	}
	if v := c.QueryParam("to"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			to = t
		}
	}

	usage, err := pgStore.GetUsageByModel(ctx, wsID, from, to)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}

	runnerUsage, _ := pgStore.GetRunnerUsage(ctx, wsID, from, to)

	return c.JSON(http.StatusOK, map[string]any{
		"model_usage":  usage,
		"runner_usage": runnerUsage,
		"from":         from,
		"to":           to,
	})
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

	// Sync the plan to the workspace record.
	if s.AuthStore != nil {
		syncer := &planSyncAdapter{authStore: s.AuthStore}
		if err := syncer.SyncWorkspacePlan(ctx, wsID, req.Plan, ""); err != nil {
			log.Printf("admin: failed to sync plan for workspace %s: %v", wsID, err)
		}
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

// HandleAdminListUsers lists/searches users by email, or returns paginated results.
// GET /api/admin/users?q=search&limit=50&offset=0
func (s *Server) HandleAdminListUsers(c echo.Context) error {
	if s.AuthStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "auth store not configured"})
	}

	query := c.QueryParam("q")
	ctx := c.Request().Context()

	if query != "" {
		// Search by name or email (fuzzy match).
		users, err := s.AuthStore.SearchUsers(ctx, query, 20)
		if err != nil {
			// Fallback to exact email match.
			user, err2 := s.AuthStore.GetUserByEmail(ctx, query)
			if err2 != nil {
				return c.JSON(http.StatusOK, map[string]any{"users": []any{}, "total": 0})
			}
			return c.JSON(http.StatusOK, map[string]any{"users": []*platauth.User{user}, "total": 1})
		}
		return c.JSON(http.StatusOK, map[string]any{"users": users, "total": len(users)})
	}

	// No query — return paginated list of all users.
	limit, _ := strconv.Atoi(c.QueryParam("limit"))
	offset, _ := strconv.Atoi(c.QueryParam("offset"))
	if limit <= 0 {
		limit = 50
	}

	users, err := s.AuthStore.ListUsers(ctx, limit, offset)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}
	if users == nil {
		users = []*platauth.User{}
	}

	return c.JSON(http.StatusOK, map[string]any{
		"users": users,
		"total": len(users),
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

// ---------------------------------------------------------------------------
// Impersonation
// ---------------------------------------------------------------------------

// HandleAdminImpersonate creates a short-lived API token for viewing a workspace
// as a customer. Records an audit note with the admin's identity.
// POST /api/admin/workspaces/:id/impersonate
func (s *Server) HandleAdminImpersonate(c echo.Context) error {
	if s.AuthStore == nil || s.Services == nil || s.Services.Auth == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "auth not configured"})
	}

	wsID := c.Param("id")
	ctx := c.Request().Context()
	adminEmail, _ := c.Get("admin_email").(string)

	// Look up workspace.
	ws, err := s.AuthStore.GetWorkspace(ctx, wsID)
	if err != nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: "workspace not found"})
	}

	// Find the workspace owner to impersonate as.
	members, err := s.AuthStore.ListMembers(ctx, wsID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "failed to list members"})
	}
	var ownerID string
	for _, m := range members {
		if m.Role == platauth.RoleOwner {
			ownerID = m.UserID
			break
		}
	}
	if ownerID == "" && len(members) > 0 {
		ownerID = members[0].UserID // fallback to first member
	}
	if ownerID == "" {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "workspace has no members"})
	}

	// Create a 1-hour API token.
	expiresAt := time.Now().Add(1 * time.Hour)
	token, plaintext, err := s.Services.Auth.CreateAPIToken(
		ctx, ownerID, wsID, "admin-impersonation", `["*"]`, &expiresAt,
	)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "failed to create token"})
	}

	// Record audit note.
	if s.BillingStore != nil {
		_ = s.BillingStore.AddNote(ctx, &billing.WorkspaceNote{
			WorkspaceID: wsID,
			AuthorEmail: adminEmail,
			Content:     fmt.Sprintf("Admin impersonation by %s — token %s expires %s", adminEmail, token.TokenPrefix, expiresAt.Format(time.RFC3339)),
		})
	}

	// Derive app URL from request origin.
	appURL := deriveAppURL(c.Request().Header.Get("Origin"))

	return c.JSON(http.StatusOK, map[string]any{
		"url":        fmt.Sprintf("%s/%s", appURL, ws.Slug),
		"token":      plaintext,
		"expires_at": expiresAt.Format(time.RFC3339),
	})
}

// deriveAppURL converts a ctrl origin to the customer app URL.
// "https://ctrl.dev.bowrain.cloud" → "https://dev.bowrain.cloud"
// "https://ctrl.bowrain.cloud" → "https://bowrain.cloud"
// fallback: "https://dev.bowrain.cloud"
func deriveAppURL(origin string) string {
	if origin == "" {
		return "https://dev.bowrain.cloud"
	}
	// Strip scheme.
	host := origin
	if idx := strings.Index(host, "://"); idx >= 0 {
		host = host[idx+3:]
	}
	host = strings.TrimPrefix(host, "ctrl.")
	return "https://" + host
}

// ---------------------------------------------------------------------------
// Member management
// ---------------------------------------------------------------------------

// HandleAdminAddMember adds a user to a workspace.
// POST /api/admin/workspaces/:id/members
func (s *Server) HandleAdminAddMember(c echo.Context) error {
	if s.AuthStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "auth not configured"})
	}

	wsID := c.Param("id")
	ctx := c.Request().Context()

	var req struct {
		UserID string `json:"user_id"`
		Role   string `json:"role"`
	}
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	}
	if req.UserID == "" {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "user_id is required"})
	}

	role := platauth.Role(req.Role)
	if !platauth.ValidRoles[role] {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "invalid role: " + req.Role})
	}

	// Verify user exists.
	if _, err := s.AuthStore.GetUser(ctx, req.UserID); err != nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: "user not found"})
	}

	// Check if already a member — update role instead.
	if existing, err := s.AuthStore.GetMembership(ctx, wsID, req.UserID); err == nil && existing != nil {
		if existing.Role == role {
			return c.JSON(http.StatusConflict, ErrorResponse{Error: "user is already a member with this role"})
		}
		if err := s.AuthStore.UpdateRole(ctx, wsID, req.UserID, role); err != nil {
			return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		}
	} else {
		if err := s.AuthStore.AddMember(ctx, wsID, req.UserID, role); err != nil {
			return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		}
	}

	// Record audit note.
	adminEmail, _ := c.Get("admin_email").(string)
	if s.BillingStore != nil {
		_ = s.BillingStore.AddNote(ctx, &billing.WorkspaceNote{
			WorkspaceID: wsID,
			AuthorEmail: adminEmail,
			Content:     fmt.Sprintf("Admin %s added user %s as %s", adminEmail, req.UserID, req.Role),
		})
	}

	return c.NoContent(http.StatusOK)
}

// nilIfEmpty returns nil for empty strings, otherwise the string value.
func nilIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}
