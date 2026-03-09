package server

import (
	"encoding/json"
	"net/http"
	"strconv"

	bstore "github.com/gokapi/gokapi/bowrain/store"
	"github.com/labstack/echo/v4"
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

	userID, _ := c.Get("user_id").(string)
	err := s.ReviewQueueStore.Decide(c.Request().Context(), itemID, bstore.DecideRequest{
		Decision: req.Decision,
		Comment:  req.Comment,
		Edits:    req.Edits,
		UserID:   userID,
	})
	if err != nil {
		return c.JSON(http.StatusUnprocessableEntity, ErrorResponse{Error: err.Error()})
	}

	return c.JSON(http.StatusOK, map[string]any{"ok": true})
}

// HandleAssignReviewItem assigns a review item to a user.
func (s *Server) HandleAssignReviewItem(c echo.Context) error {
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

	userID, _ := c.Get("user_id").(string)
	decided, err := s.ReviewQueueStore.BatchDecide(c.Request().Context(), req.ItemIDs, bstore.DecideRequest{
		Decision: req.Decision,
		UserID:   userID,
	})
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}

	return c.JSON(http.StatusOK, map[string]any{"ok": true, "decided": decided})
}

// HandleSyncReviewDecisions processes offline review decisions from the mobile app.
func (s *Server) HandleSyncReviewDecisions(c echo.Context) error {
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
	var synced int
	var conflicts []string

	for _, d := range req.Decisions {
		err := s.ReviewQueueStore.Decide(c.Request().Context(), d.ItemID, bstore.DecideRequest{
			Decision: d.Decision,
			Comment:  d.Comment,
			Edits:    d.Edits,
			UserID:   userID,
		})
		if err != nil {
			conflicts = append(conflicts, d.ItemID)
		} else {
			synced++
		}
	}

	return c.JSON(http.StatusOK, map[string]any{
		"synced":    synced,
		"conflicts": conflicts,
	})
}
