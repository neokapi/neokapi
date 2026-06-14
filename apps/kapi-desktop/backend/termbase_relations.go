package backend

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/neokapi/neokapi/core/graph"
	"github.com/neokapi/neokapi/core/id"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/termbase"
)

// This file gives the Apache desktop the visual concept/relation editing that
// the deleted CLI relation commands used to provide. The methods drive a LOCAL
// SQLite termbase through the existing handle model (App.tbHandles) and return
// snake_case DTOs that mirror the @neokapi/concept-ui view shapes, so the
// framework concept UI can browse, relate, and re-status concepts against the
// author's own local copy. There is no governance gate here — that lives on the
// server, applied later on push.

// --- DTOs ---

// ValidityDTO is the frontend-facing temporal/tag scoping on a term or relation.
// Times are RFC-3339 strings; an empty bound means "open" on that side. A nil
// ValidityDTO means "always valid, everywhere" (mirrors graph.Validity).
type ValidityDTO struct {
	ValidFrom string            `json:"valid_from,omitempty"`
	ValidTo   string            `json:"valid_to,omitempty"`
	Tags      map[string]string `json:"tags,omitempty"`
}

// RelationDTO is a frontend-facing typed edge between two concepts (1-hop).
type RelationDTO struct {
	ID       string       `json:"id"`
	SourceID string       `json:"source_id"`
	TargetID string       `json:"target_id"`
	Type     string       `json:"type"` // graph.Label* upper form, e.g. "BROADER"
	Note     string       `json:"note,omitempty"`
	Validity *ValidityDTO `json:"validity,omitempty"`
}

// AddRelationRequest is the request to create a relation from the subject
// concept. The backend generates the relation ID and validates the type and
// that both concepts exist (the framework AddRelation enforces this).
type AddRelationRequest struct {
	SourceID  string            `json:"source_id"`
	TargetID  string            `json:"target_id"`
	Type      string            `json:"type"` // graph.Label* upper form, e.g. "RELATED"
	Note      string            `json:"note,omitempty"`
	ValidFrom string            `json:"valid_from,omitempty"`
	ValidTo   string            `json:"valid_to,omitempty"`
	Tags      map[string]string `json:"tags,omitempty"`
}

// SetTermStatusRequest transitions one term's lifecycle status (and optionally
// its validity window) within a concept. The matching term is identified by
// (locale, text). Local copies have no governance gate, so any known status is
// accepted; the transition is governed later, on push.
type SetTermStatusRequest struct {
	ConceptID string            `json:"concept_id"`
	Locale    string            `json:"locale"`
	Text      string            `json:"text"`
	Status    string            `json:"status"`
	ValidFrom string            `json:"valid_from,omitempty"`
	ValidTo   string            `json:"valid_to,omitempty"`
	Tags      map[string]string `json:"tags,omitempty"`
}

// --- Validity conversion helpers ---

// validityToDTO converts a framework validity to its frontend DTO, or nil when
// the validity is unbounded.
func validityToDTO(v *graph.Validity) *ValidityDTO {
	if v == nil {
		return nil
	}
	dto := &ValidityDTO{Tags: v.Tags}
	if v.ValidFrom != nil {
		dto.ValidFrom = v.ValidFrom.Format(time.RFC3339)
	}
	if v.ValidTo != nil {
		dto.ValidTo = v.ValidTo.Format(time.RFC3339)
	}
	return dto
}

// validityFromDTO converts a frontend validity DTO to the framework type, or
// nil when the DTO is nil/empty. Unparseable instants are dropped rather than
// failing the whole write — the editor never sends partial garbage, and a tag-
// only validity stays valid.
func validityFromDTO(dto *ValidityDTO) *graph.Validity {
	if dto == nil {
		return nil
	}
	return validityFromFields(dto.ValidFrom, dto.ValidTo, dto.Tags)
}

// validityFromFields builds a framework validity from the flat request fields.
// Returns nil when all three are empty (an unbounded validity).
func validityFromFields(validFrom, validTo string, tags map[string]string) *graph.Validity {
	var v graph.Validity
	has := false
	if validFrom != "" {
		if t, err := time.Parse(time.RFC3339, validFrom); err == nil {
			v.ValidFrom = &t
			has = true
		}
	}
	if validTo != "" {
		if t, err := time.Parse(time.RFC3339, validTo); err == nil {
			v.ValidTo = &t
			has = true
		}
	}
	if len(tags) > 0 {
		v.Tags = tags
		has = true
	}
	if !has {
		return nil
	}
	return &v
}

// relationToDTO converts a framework relation to its frontend DTO.
func relationToDTO(r termbase.ConceptRelation) RelationDTO {
	return RelationDTO{
		ID:       r.ID,
		SourceID: r.SourceID,
		TargetID: r.TargetID,
		Type:     r.RelationType,
		Note:     r.Note,
		Validity: validityToDTO(r.Validity),
	}
}

// --- Relation methods ---

// GetRelations returns every relation touching the concept, in either
// direction (incoming and outgoing), so the concept dashboard's relations
// widget can group and label neighbours from a single read.
func (a *App) GetRelations(handle, conceptID string) ([]RelationDTO, error) {
	tb, ok := a.tbHandles.Get(handle)
	if !ok {
		return nil, fmt.Errorf("termbase handle %q not found", handle)
	}
	rels, err := tb.RelationsOf(context.Background(), conceptID, nil)
	if err != nil {
		return nil, fmt.Errorf("get relations for %q: %w", conceptID, err)
	}
	dtos := make([]RelationDTO, 0, len(rels))
	for _, r := range rels {
		dtos = append(dtos, relationToDTO(r))
	}
	return dtos, nil
}

// AddRelation creates a typed relation from req.SourceID to req.TargetID. The
// relation ID is generated here; the framework AddRelation validates the type
// (must be a known graph.Label*) and that both concepts exist, returning an
// error otherwise. The relation type is normalised to its upper form.
func (a *App) AddRelation(handle string, req AddRelationRequest) (RelationDTO, error) {
	tb, ok := a.tbHandles.Get(handle)
	if !ok {
		return RelationDTO{}, fmt.Errorf("termbase handle %q not found", handle)
	}
	rel := termbase.ConceptRelation{
		ID:           id.New(),
		SourceID:     req.SourceID,
		TargetID:     req.TargetID,
		RelationType: strings.ToUpper(strings.TrimSpace(req.Type)),
		Note:         req.Note,
		Validity:     validityFromFields(req.ValidFrom, req.ValidTo, req.Tags),
		CreatedAt:    time.Now(),
	}
	if err := tb.AddRelation(context.Background(), rel); err != nil {
		return RelationDTO{}, fmt.Errorf("add relation: %w", err)
	}
	return relationToDTO(rel), nil
}

// RemoveRelation deletes a relation by ID.
func (a *App) RemoveRelation(handle, relationID string) error {
	tb, ok := a.tbHandles.Get(handle)
	if !ok {
		return fmt.Errorf("termbase handle %q not found", handle)
	}
	if err := tb.DeleteRelation(context.Background(), relationID); err != nil {
		return fmt.Errorf("remove relation %q: %w", relationID, err)
	}
	return nil
}

// --- Term status method ---

// SetTermStatus transitions the lifecycle status (and optional validity window)
// of the term identified by (req.Locale, req.Text) within req.ConceptID, then
// persists the concept. This is the author's own local copy: any known status
// is accepted with no governance gate (governance runs on the server, on push).
func (a *App) SetTermStatus(handle string, req SetTermStatusRequest) error {
	tb, ok := a.tbHandles.Get(handle)
	if !ok {
		return fmt.Errorf("termbase handle %q not found", handle)
	}
	if !termbase.KnownTermStatus(model.TermStatus(req.Status)) {
		return fmt.Errorf("unknown term status %q", req.Status)
	}

	concept, found, err := tb.GetConcept(context.Background(), req.ConceptID)
	if err != nil {
		return fmt.Errorf("load concept %q: %w", req.ConceptID, err)
	}
	if !found {
		return fmt.Errorf("concept %q not found", req.ConceptID)
	}

	matched := false
	for i := range concept.Terms {
		t := &concept.Terms[i]
		if string(t.Locale) == req.Locale && t.Text == req.Text {
			t.Status = model.TermStatus(req.Status)
			if v := validityFromFields(req.ValidFrom, req.ValidTo, req.Tags); v != nil {
				t.Validity = v
			}
			matched = true
			break
		}
	}
	if !matched {
		return fmt.Errorf("term %q (%s) not found in concept %q", req.Text, req.Locale, req.ConceptID)
	}

	concept.UpdatedAt = time.Now()
	if err := tb.AddConcept(context.Background(), concept); err != nil { // same ID = update
		return fmt.Errorf("persist term status: %w", err)
	}
	return nil
}

// --- Concept view read ---

// GetConceptForView returns one concept with full term status + validity for
// the concept dashboard (geography/constraints panels), or nil when it no
// longer exists. Unlike GetConcept it returns an error so the frontend adapter
// can distinguish a missing concept (nil, nil) from a backend failure.
func (a *App) GetConceptForView(handle, conceptID string) (*ConceptDTO, error) {
	tb, ok := a.tbHandles.Get(handle)
	if !ok {
		return nil, fmt.Errorf("termbase handle %q not found", handle)
	}
	concept, found, err := tb.GetConcept(context.Background(), conceptID)
	if err != nil {
		return nil, fmt.Errorf("get concept %q: %w", conceptID, err)
	}
	if !found {
		return nil, nil
	}
	dto := conceptToDTO(concept)
	return &dto, nil
}
