package server

import (
	"context"
	"errors"
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
	"github.com/neokapi/neokapi/core/venue"
	"github.com/neokapi/neokapi/terms"
)

// An entity on a block is an annotation of type entity on the block's own
// edition: a change set marks one with annotate ({"type": "entity", "anchor":
// …, "value": {"text", "type", "dnt", …}}), changes it with annotate under the
// same id, and removes it with unannotate, through the stream's changes route.
// Promoting a marked entity to a term candidate or a concept is its own action.

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

	var tcKey string
	updated, err := s.ContentStore.UpdateBlock(c.Request().Context(), projectID, streamParam(c), blockID, func(sb *venue.StoredBlock) error {
		block := sb.Block
		entityKey := entitySpanID(block, c.Param("idx"))
		entity, err := entityAt(block, entityKey)
		if err != nil {
			return err
		}
		span := block.OverlaySpan(model.OverlayEntity, entityKey)

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
		tcKey = fmt.Sprintf("term-candidate:%d", tcIdx)
		block.AddOverlaySpan(model.OverlayTermCandidate, model.Span{
			ID:    tcKey,
			Range: span.Range,
			Value: candidate,
		})
		return nil
	})
	if err != nil {
		return entityWriteErr(c, updated, err)
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
	itemName := c.QueryParam("item")

	ctx := c.Request().Context()
	block, err := getBlock(ctx, s.ContentStore, projectID, streamParam(c), itemName, blockID)
	if err != nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: "block not found"})
	}
	span := block.OverlaySpan(model.OverlayEntity, entitySpanID(block, c.Param("idx")))
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
	if concept.DoNotTranslate {
		// Proposed for review rather than created: see promoteEntityToConcept.
		return c.JSON(http.StatusAccepted, map[string]any{"ok": true, "proposed": true, "concept": editorConceptToInfo(concept)})
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
	// An entity marked do-not-translate yields a concept carrying the flag, and
	// creating one is governed: it is proposed for review, and nothing is written
	// until the change-set merges, which publishes concept.created then.
	if entity.DNT {
		concept.DoNotTranslate = true
		op, err := conceptCreateOp(concept)
		if err != nil {
			return concept, err
		}
		_, err = s.proposeGovernedChange(ctx, wsSlug, wsID, actor, fmt.Sprintf("Promote %q as do-not-translate", entity.Text), []knowledge.ChangeSetOp{op})
		return concept, err
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

// errEntityNotFound and errNotEntityAnnotation end an entity write that has
// nothing to act on; entityWriteErr answers them.
var (
	errEntityNotFound      = errors.New("entity not found")
	errNotEntityAnnotation = errors.New("not an entity annotation")
)

// entitySpanID is the id of the entity span a route's :idx names: the span's
// own id (what a read lists as the entity's key, and what annotate wrote), or
// the index an extraction numbered "entity:N" by.
func entitySpanID(block *model.Block, idx string) string {
	if block.OverlaySpan(model.OverlayEntity, idx) != nil {
		return idx
	}
	return "entity:" + idx
}

// entityAt is the entity annotation a block holds under key.
func entityAt(block *model.Block, key string) (*model.EntityAnnotation, error) {
	span := block.OverlaySpan(model.OverlayEntity, key)
	if span == nil {
		return nil, errEntityNotFound
	}
	entity, ok := span.Value.(*model.EntityAnnotation)
	if !ok {
		return nil, errNotEntityAnnotation
	}
	return entity, nil
}

// entityWriteErr answers an entity write that did not land: a block that is not
// stored, an entity that is not there, or a store error.
func entityWriteErr(c echo.Context, updated *venue.StoredBlock, err error) error {
	switch {
	case updated == nil:
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: "block not found"})
	case errors.Is(err, errEntityNotFound):
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: "entity not found"})
	case errors.Is(err, errNotEntityAnnotation):
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "not an entity annotation"})
	default:
		return serverErr(c, err)
	}
}

// getBlock loads a single block by project, stream, item, and block ID.
func getBlock(ctx context.Context, cs store.ContentStore, projectID, stream, itemName, blockID string) (*model.Block, error) {
	blocks, err := cs.GetBlocks(ctx, store.BlockQuery{
		ProjectID: projectID,
		Stream:    stream,
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
