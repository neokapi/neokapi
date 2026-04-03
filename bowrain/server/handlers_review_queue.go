package server

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"
	platauth "github.com/neokapi/neokapi/bowrain/core/auth"

	bstore "github.com/neokapi/neokapi/bowrain/store"
)

// HandleListReviewQueue returns review items for a project with filtering and pagination.
func (s *Server) HandleListReviewQueue(c echo.Context) error {
	projectID := c.Param("id")
	if s.ReviewQueueStore == nil {
		return c.JSON(http.StatusOK, bstore.ReviewQueueResult{Items: []bstore.ReviewItem{}})
	}

	q := bstore.ReviewQueueQuery{
		ProjectID: projectID,
		Type:      bstore.ReviewItemType(c.QueryParam("type")),
		Status:    bstore.ReviewItemStatus(c.QueryParam("status")),
		Locale:    c.QueryParam("locale"),
		Cursor:    c.QueryParam("cursor"),
	}

	assignedTo := c.QueryParam("assigned_to")
	if assignedTo == "me" {
		userID, _ := c.Get("user_id").(string)
		q.AssignedTo = userID
	} else if assignedTo != "" {
		q.AssignedTo = assignedTo
	}

	if limit := c.QueryParam("limit"); limit != "" {
		if l, err := strconv.Atoi(limit); err == nil && l > 0 {
			q.Limit = l
		}
	}

	result, err := s.ReviewQueueStore.ListItems(c.Request().Context(), q)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}
	if result.Items == nil {
		result.Items = []bstore.ReviewItem{}
	}
	return c.JSON(http.StatusOK, result)
}

// HandleGetReviewQueueItem returns a single review item.
func (s *Server) HandleGetReviewQueueItem(c echo.Context) error {
	if s.ReviewQueueStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "review queue not configured"})
	}

	itemID := c.Param("itemId")
	item, err := s.ReviewQueueStore.GetItem(c.Request().Context(), itemID)
	if err != nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: "review item not found"})
	}
	return c.JSON(http.StatusOK, item)
}

// HandleDecideReviewItem applies an approve/reject decision to a review item.
func (s *Server) HandleDecideReviewItem(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermReview); err != nil {
		return err
	}

	if s.ReviewQueueStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "review queue not configured"})
	}

	itemID := c.Param("itemId")
	var req struct {
		Decision string          `json:"decision"`
		Comment  string          `json:"comment"`
		Edits    json.RawMessage `json:"edits"`
	}
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	}

	if req.Decision != "approve" && req.Decision != "reject" {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "decision must be 'approve' or 'reject'"})
	}

	ctx := c.Request().Context()

	// Load the item to check language permission and for side effects.
	item, err := s.ReviewQueueStore.GetItem(ctx, itemID)
	if err != nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: "review item not found"})
	}
	if item.Locale != "" {
		if err := s.requireLanguagePermission(c, platauth.PermReview, item.Locale); err != nil {
			return err
		}
	}

	userID, _ := c.Get("user_id").(string)
	err = s.ReviewQueueStore.Decide(ctx, itemID, bstore.DecideRequest{
		Decision: req.Decision,
		Comment:  req.Comment,
		Edits:    req.Edits,
		UserID:   userID,
	})
	if err != nil {
		return c.JSON(http.StatusUnprocessableEntity, ErrorResponse{Error: err.Error()})
	}

	// Process side effects (termbase creation, rejected terms, DNT entries).
	wsSlug, _ := c.Get("workspace_slug").(string)
	go s.processDecisionSideEffects(context.Background(), item, wsSlug)

	return c.JSON(http.StatusOK, map[string]any{"ok": true})
}

// HandleAssignReviewItem assigns a review item to a user.
func (s *Server) HandleAssignReviewItem(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermManageMembers); err != nil {
		return err
	}

	if s.ReviewQueueStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "review queue not configured"})
	}

	itemID := c.Param("itemId")
	var req struct {
		UserID string `json:"user_id"`
	}
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	}

	err := s.ReviewQueueStore.Assign(c.Request().Context(), itemID, req.UserID)
	if err != nil {
		return c.JSON(http.StatusUnprocessableEntity, ErrorResponse{Error: err.Error()})
	}

	return c.JSON(http.StatusOK, map[string]any{"ok": true})
}

// HandleSplitReviewItem splits occurrences from a review item into a new item.
func (s *Server) HandleSplitReviewItem(c echo.Context) error {
	if s.ReviewQueueStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "review queue not configured"})
	}

	itemID := c.Param("itemId")
	var req struct {
		OccurrenceIDs []string `json:"occurrence_ids"`
	}
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	}

	newItem, err := s.ReviewQueueStore.SplitItem(c.Request().Context(), itemID, req.OccurrenceIDs)
	if err != nil {
		return c.JSON(http.StatusUnprocessableEntity, ErrorResponse{Error: err.Error()})
	}

	original, _ := s.ReviewQueueStore.GetItem(c.Request().Context(), itemID)

	return c.JSON(http.StatusOK, map[string]any{
		"original": original,
		"new_item": newItem,
	})
}

// HandleBatchDecideReviewItems applies the same decision to multiple review items.
func (s *Server) HandleBatchDecideReviewItems(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermReview); err != nil {
		return err
	}

	if s.ReviewQueueStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "review queue not configured"})
	}

	var req struct {
		ItemIDs  []string `json:"item_ids"`
		Decision string   `json:"decision"`
	}
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	}

	if req.Decision != "approve" && req.Decision != "reject" {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "decision must be 'approve' or 'reject'"})
	}

	ctx := c.Request().Context()

	// Check language permission for each item before deciding.
	for _, id := range req.ItemIDs {
		item, getErr := s.ReviewQueueStore.GetItem(ctx, id)
		if getErr != nil {
			continue
		}
		if item.Locale != "" {
			if err := s.requireLanguagePermission(c, platauth.PermReview, item.Locale); err != nil {
				return err
			}
		}
	}

	userID, _ := c.Get("user_id").(string)
	decided, err := s.ReviewQueueStore.BatchDecide(ctx, req.ItemIDs, bstore.DecideRequest{
		Decision: req.Decision,
		UserID:   userID,
	})
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}

	// Process side effects for each decided item.
	wsSlug, _ := c.Get("workspace_slug").(string)
	go func() {
		bgCtx := context.Background()
		for _, id := range req.ItemIDs {
			if item, getErr := s.ReviewQueueStore.GetItem(bgCtx, id); getErr == nil {
				s.processDecisionSideEffects(bgCtx, item, wsSlug)
			}
		}
	}()

	return c.JSON(http.StatusOK, map[string]any{"ok": true, "decided": decided})
}

// HandleSyncReviewDecisions processes offline review decisions from the mobile app.
func (s *Server) HandleSyncReviewDecisions(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermReview); err != nil {
		return err
	}

	if s.ReviewQueueStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "review queue not configured"})
	}

	var req struct {
		Decisions []struct {
			ItemID   string          `json:"item_id"`
			Decision string          `json:"decision"`
			Comment  string          `json:"comment"`
			Edits    json.RawMessage `json:"edits"`
		} `json:"decisions"`
	}
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	}

	userID, _ := c.Get("user_id").(string)
	ctx := c.Request().Context()
	wsSlug, _ := c.Get("workspace_slug").(string)
	var synced int
	var conflicts []string
	var decidedIDs []string

	for _, d := range req.Decisions {
		err := s.ReviewQueueStore.Decide(ctx, d.ItemID, bstore.DecideRequest{
			Decision: d.Decision,
			Comment:  d.Comment,
			Edits:    d.Edits,
			UserID:   userID,
		})
		if err != nil {
			conflicts = append(conflicts, d.ItemID)
		} else {
			synced++
			decidedIDs = append(decidedIDs, d.ItemID)
		}
	}

	// Process side effects for decided items.
	go func() {
		bgCtx := context.Background()
		for _, id := range decidedIDs {
			if item, getErr := s.ReviewQueueStore.GetItem(bgCtx, id); getErr == nil {
				s.processDecisionSideEffects(bgCtx, item, wsSlug)
			}
		}
	}()

	return c.JSON(http.StatusOK, map[string]any{
		"synced":    synced,
		"conflicts": conflicts,
	})
}
