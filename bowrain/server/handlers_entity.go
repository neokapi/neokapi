package server

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"

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

	// Find next entity index and add a positional entity overlay span.
	idx := nextOverlaySpanIndex(block, model.OverlayEntity, "entity:")
	key := fmt.Sprintf("entity:%d", idx)
	block.AddOverlaySpan(model.OverlayEntity, model.Span{
		ID:    key,
		Range: model.RunRangeForBytes(block.Source, req.Start, req.End),
		Value: &model.EntityAnnotation{
			Text:   req.Text,
			Type:   model.EntityType(req.Type),
			DNT:    req.DNT,
			Source: model.ExtractionSourceManual,
			Locale: model.LocaleID(req.Locale),
		},
	})

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

	span := block.OverlaySpan(model.OverlayEntity, entityKey)
	if span == nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: "entity not found"})
	}
	entity, ok := span.Value.(*model.EntityAnnotation)
	if !ok {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "not an entity annotation"})
	}

	// span.Value is a pointer into the block's facet, so these edits mutate it
	// in place — no re-store of the span needed.
	if req.Type != "" {
		entity.Type = model.EntityType(req.Type)
	}
	if req.DNT != nil {
		entity.DNT = *req.DNT
	}

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

	if !block.RemoveOverlaySpan(model.OverlayEntity, entityKey) {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: "entity not found"})
	}
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

	span := block.OverlaySpan(model.OverlayEntity, entityKey)
	if span == nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: "entity not found"})
	}
	entity, ok := span.Value.(*model.EntityAnnotation)
	if !ok {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "not an entity annotation"})
	}

	// Create a term candidate from the entity, at the same position.
	candidate := &model.TermCandidateAnnotation{
		Text:            entity.Text,
		Category:        model.TermCategoryGeneral,
		Translatability: model.TranslatabilityConsistent,
		Confidence:      1.0, // manual promotion = high confidence
		Locale:          entity.Locale,
		Source:          model.ExtractionSourceManual,
		Status:          model.CandidateStatusPending,
	}

	if entity.DNT {
		candidate.Translatability = model.TranslatabilityDNT
	}

	// Add the term-candidate overlay span at the entity's position.
	tcIdx := nextOverlaySpanIndex(block, model.OverlayTermCandidate, "term-candidate:")
	tcKey := fmt.Sprintf("term-candidate:%d", tcIdx)
	block.AddOverlaySpan(model.OverlayTermCandidate, model.Span{
		ID:    tcKey,
		Range: span.Range,
		Value: candidate,
	})

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

// nextOverlaySpanIndex finds the next available index for span IDs with the given
// prefix in the source-side overlay of type t (e.g. "entity:" → next "entity:N").
func nextOverlaySpanIndex(block *model.Block, t model.OverlayType, prefix string) int {
	max := -1
	if f := block.OverlayOf(t); f != nil {
		for _, s := range f.Spans {
			if strings.HasPrefix(s.ID, prefix) {
				if idx, err := strconv.Atoi(s.ID[len(prefix):]); err == nil && idx > max {
					max = idx
				}
			}
		}
	}
	return max + 1
}
