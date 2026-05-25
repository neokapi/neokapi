package server

import (
	"context"
	"fmt"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"
	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	"github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/core/model"
)

// CreateEntityRequest creates a new entity annotation on a block.
type CreateEntityRequest struct {
	Text   string `json:"text"`
	Type   string `json:"type"`
	Start  int    `json:"start"`
	End    int    `json:"end"`
	DNT    bool   `json:"dnt"`
	Locale string `json:"locale,omitempty"`
}

// UpdateEntityRequest updates an existing entity annotation.
type UpdateEntityRequest struct {
	Type string `json:"type,omitempty"`
	DNT  *bool  `json:"dnt,omitempty"`
}

// HandleCreateEntity adds a new entity annotation to a block (manual marking).
func (s *Server) HandleCreateEntity(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermEditSource); err != nil {
		return err
	}

	if s.ContentStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "store not configured"})
	}

	projectID := projectParam(c)
	blockID := c.Param("bid")
	itemName := c.QueryParam("item")

	var req CreateEntityRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	}

	ctx := c.Request().Context()
	block, err := getBlock(ctx, s.ContentStore, projectID, itemName, blockID)
	if err != nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: "block not found"})
	}

	if block.Annotations == nil {
		block.Annotations = make(map[string]model.Annotation)
	}

	// Find next entity index.
	idx := nextAnnotationIndex(block.Annotations, "entity:")

	ann := &model.EntityAnnotation{
		Text:     req.Text,
		Type:     model.EntityType(req.Type),
		Position: model.RunRangeForBytes(block.Source, req.Start, req.End),
		DNT:      req.DNT,
		Source:   model.ExtractionSourceManual,
		Locale:   model.LocaleID(req.Locale),
	}

	key := fmt.Sprintf("entity:%d", idx)
	block.Annotations[key] = ann

	if err := s.ContentStore.StoreBlocksForItem(ctx, projectID, "main", itemName, []*model.Block{block}); err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}

	return c.JSON(http.StatusCreated, EntityInfoResponse{
		Key:    key,
		Text:   req.Text,
		Type:   req.Type,
		Start:  req.Start,
		End:    req.End,
		DNT:    req.DNT,
		Source: "manual",
		Locale: req.Locale,
	})
}

// HandleUpdateEntity updates an existing entity annotation on a block.
func (s *Server) HandleUpdateEntity(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermEditSource); err != nil {
		return err
	}

	if s.ContentStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "store not configured"})
	}

	projectID := projectParam(c)
	blockID := c.Param("bid")
	entityKey := "entity:" + c.Param("idx")
	itemName := c.QueryParam("item")

	var req UpdateEntityRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	}

	ctx := c.Request().Context()
	block, err := getBlock(ctx, s.ContentStore, projectID, itemName, blockID)
	if err != nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: "block not found"})
	}

	ann, ok := block.Annotations[entityKey]
	if !ok {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: "entity not found"})
	}

	entity, ok := ann.(*model.EntityAnnotation)
	if !ok {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "not an entity annotation"})
	}

	if req.Type != "" {
		entity.Type = model.EntityType(req.Type)
	}
	if req.DNT != nil {
		entity.DNT = *req.DNT
	}

	block.Annotations[entityKey] = entity
	if err := s.ContentStore.StoreBlocksForItem(ctx, projectID, "main", itemName, []*model.Block{block}); err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}

	return c.NoContent(http.StatusNoContent)
}

// HandleDeleteEntity removes an entity annotation from a block.
func (s *Server) HandleDeleteEntity(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermEditSource); err != nil {
		return err
	}

	if s.ContentStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "store not configured"})
	}

	projectID := projectParam(c)
	blockID := c.Param("bid")
	entityKey := "entity:" + c.Param("idx")
	itemName := c.QueryParam("item")

	ctx := c.Request().Context()
	block, err := getBlock(ctx, s.ContentStore, projectID, itemName, blockID)
	if err != nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: "block not found"})
	}

	if _, ok := block.Annotations[entityKey]; !ok {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: "entity not found"})
	}

	delete(block.Annotations, entityKey)
	if err := s.ContentStore.StoreBlocksForItem(ctx, projectID, "main", itemName, []*model.Block{block}); err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}

	return c.NoContent(http.StatusNoContent)
}

// HandlePromoteEntity promotes an entity annotation to a term candidate review item.
func (s *Server) HandlePromoteEntity(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermManageTerms); err != nil {
		return err
	}

	if s.ContentStore == nil || s.ReviewQueueStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "store not configured"})
	}

	projectID := projectParam(c)
	blockID := c.Param("bid")
	entityKey := "entity:" + c.Param("idx")
	itemName := c.QueryParam("item")

	ctx := c.Request().Context()
	block, err := getBlock(ctx, s.ContentStore, projectID, itemName, blockID)
	if err != nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: "block not found"})
	}

	ann, ok := block.Annotations[entityKey]
	if !ok {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: "entity not found"})
	}

	entity, ok := ann.(*model.EntityAnnotation)
	if !ok {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "not an entity annotation"})
	}

	// Create a term candidate annotation from the entity.
	candidate := &model.TermCandidateAnnotation{
		Text:            entity.Text,
		Category:        model.TermCategoryGeneral,
		Translatability: model.TranslatabilityConsistent,
		Confidence:      1.0, // manual promotion = high confidence
		Position:        entity.Position,
		Locale:          entity.Locale,
		Source:          model.ExtractionSourceManual,
		Status:          model.CandidateStatusPending,
	}

	if entity.DNT {
		candidate.Translatability = model.TranslatabilityDNT
	}

	// Add term-candidate annotation.
	tcIdx := nextAnnotationIndex(block.Annotations, "term-candidate:")
	tcKey := fmt.Sprintf("term-candidate:%d", tcIdx)
	block.Annotations[tcKey] = candidate

	if err := s.ContentStore.StoreBlocksForItem(ctx, projectID, "main", itemName, []*model.Block{block}); err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}

	return c.JSON(http.StatusOK, map[string]any{"ok": true, "term_candidate_key": tcKey})
}

// getBlock loads a single block by project, item, and block ID.
func getBlock(ctx context.Context, cs store.ContentStore, projectID, itemName, blockID string) (*model.Block, error) {
	blocks, err := cs.GetBlocks(ctx, store.BlockQuery{
		ProjectID: projectID,
		Stream:    "main",
		ItemName:  itemName,
	})
	if err != nil {
		return nil, err
	}
	for _, sb := range blocks {
		if sb.Block.ID == blockID {
			return sb.Block, nil
		}
	}
	return nil, fmt.Errorf("block %s not found", blockID)
}

// nextAnnotationIndex finds the next available index for a given annotation prefix.
func nextAnnotationIndex(annotations map[string]model.Annotation, prefix string) int {
	max := -1
	for key := range annotations {
		if len(key) > len(prefix) && key[:len(prefix)] == prefix {
			if idx, err := strconv.Atoi(key[len(prefix):]); err == nil && idx > max {
				max = idx
			}
		}
	}
	return max + 1
}
