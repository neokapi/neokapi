package server

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	platev "github.com/neokapi/neokapi/bowrain/core/event"
	"github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/bowrain/knowledge"
	"github.com/neokapi/neokapi/core/id"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/terms"
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
		Range: model.RangeAnchorForBytes(block.Source, req.Start, req.End),
		Value: &model.EntityAnnotation{
			Text:   req.Text,
			Type:   model.EntityType(req.Type),
			DNT:    req.DNT,
			Source: model.ExtractionSourceManual,
			Locale: model.LocaleID(req.Locale),
		},
	})

	if err := s.ContentStore.StoreBlocksForItem(ctx, projectID, "main", itemName, []*model.Block{block}); err != nil {
		return serverErr(c, err)
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

	// span.Value is a pointer into the block's overlay, so these edits mutate it
	// in place — no re-store of the span needed.
	if req.Type != "" {
		entity.Type = model.EntityType(req.Type)
	}
	if req.DNT != nil {
		entity.DNT = *req.DNT
	}

	if err := s.ContentStore.StoreBlocksForItem(ctx, projectID, "main", itemName, []*model.Block{block}); err != nil {
		return serverErr(c, err)
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
		return serverErr(c, err)
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
		return serverErr(c, err)
	}

	return c.JSON(http.StatusOK, map[string]any{"ok": true, "term_candidate_key": tcKey})
}

// HandlePromoteEntityToConcept promotes a marked entity to a real terms
// concept — distinct from HandlePromoteEntity, which only creates a term
// *candidate* review item. Creating the concept fires concept.created, which the
// RV-E/RV-F re-check subscriber reacts to automatically (re-checking existing
// targets against the new term), so a promoted entity flows straight into the
// governed terminology loop. It reuses the ordinary AddConcept curation path
// (the same one HandleCreateConcept uses).
//
// POST /:ws/:id/blocks/:ref/:bid/entities/:idx/promote-to-concept
func (s *Server) HandlePromoteEntityToConcept(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermManageTerms); err != nil {
		return err
	}
	if s.ContentStore == nil || s.wsStores == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "store not configured"})
	}

	ws := c.Param("ws")
	wsID, _ := c.Get("workspace_id").(string)
	actor, _ := c.Get("user_id").(string)
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

	concept, err := s.promoteEntityToConcept(ctx, ws, wsID, actor, projectID, streamParam(c), entity)
	if err != nil {
		return serverErr(c, err)
	}
	return c.JSON(http.StatusCreated, map[string]any{"ok": true, "concept": editorConceptToInfo(concept)})
}

// promoteEntityToConcept creates a terms store concept from a marked entity and fires
// concept.created (which the RV-E/RV-F re-check reacts to automatically). Split
// from the HTTP handler so the promotion mapping is testable directly.
func (s *Server) promoteEntityToConcept(ctx context.Context, wsSlug, wsID, actor, projectID, stream string, entity *model.EntityAnnotation) (terms.Concept, error) {
	// The concept's source term lives in the entity's locale, falling back to the
	// project's source language when the entity carries none.
	loc := entity.Locale
	if loc == "" {
		if proj, perr := s.ContentStore.GetProject(ctx, projectID); perr == nil {
			loc = proj.DefaultSourceLanguage
		}
	}

	// Promote to an APPROVED term — ordinary curation, not a governed transition
	// (forbidden/preferred creation is governed and refused on the direct path).
	now := time.Now()
	concept := terms.Concept{
		ID:        id.New(),
		ProjectID: projectID,
		Domain:    string(entity.Type),
		Source:    terms.TermSourceTerminology,
		Terms: []terms.Term{
			{Text: entity.Text, Locale: loc, Status: model.TermApproved},
		},
		CreatedAt: now,
		UpdatedAt: now,
	}
	// Preserve the entity's DNT signal as concept metadata (matching the term
	// candidate → concept mapping in review_effects.go).
	if entity.DNT {
		concept.Properties = map[string]string{"translatability": string(model.TranslatabilityDNT)}
	}

	tb, err := s.wsStores.getTerms(wsSlug)
	if err != nil {
		return concept, err
	}
	if stream != "" && stream != "main" {
		err = tb.AddConceptWithStream(ctx, concept, stream)
	} else {
		err = tb.AddConcept(ctx, concept)
	}
	if err != nil {
		return concept, err
	}

	// concept.created drives the RV-E/RV-F fan-out re-check automatically. Published
	// directly (context-free) to match publishKnowledgeEvents' concept-event shape.
	if s.EventBus != nil {
		s.EventBus.Publish(platev.Event{
			ID:           id.New(),
			Type:         knowledge.EventConceptCreated,
			Source:       "knowledge",
			WorkspaceID:  wsID,
			Actor:        actor,
			Data:         map[string]string{"concept_id": concept.ID},
			ResourceType: "concept",
			ResourceID:   concept.ID,
			Timestamp:    time.Now().UTC(),
		})
	}
	return concept, nil
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
