package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/neokapi/neokapi/bowrain/knowledge"
	"github.com/neokapi/neokapi/terms"
)

// proposeGovernedChange opens a change-set holding ops and submits it for review.
// A path that otherwise writes concepts directly uses it for a governed change,
// such as a concept created with its do-not-translate flag. Nothing the ops
// describe is written until a reviewer approves the change-set and it merges,
// and reviewers are summoned as for a change-set submitted through the API.
func (s *Server) proposeGovernedChange(ctx context.Context, wsSlug, wsID, actor, name string, ops []knowledge.ChangeSetOp) (*knowledge.ChangeSet, error) {
	if s.KnowledgeStore == nil {
		return nil, errKnowledgeUnavailable
	}
	if len(ops) == 0 {
		return nil, errors.New("a proposal needs at least one op")
	}
	for _, op := range ops {
		if err := knowledge.ValidateOp(op); err != nil {
			return nil, err
		}
	}
	governed, err := knowledge.ChangeSetIsGoverned(ops)
	if err != nil {
		return nil, err
	}

	cs := &knowledge.ChangeSet{WorkspaceID: wsID, Name: name, Status: knowledge.ChangeSetDraft, CreatedBy: actor}
	if err := s.KnowledgeStore.CreateChangeSet(ctx, cs); err != nil {
		return nil, fmt.Errorf("open change-set: %w", err)
	}
	for _, op := range ops {
		op.WorkspaceID, op.ChangesetID, op.CreatedBy = wsID, cs.ID, actor
		if err := s.KnowledgeStore.AppendOp(ctx, &op); err != nil {
			return nil, fmt.Errorf("add op to change-set %s: %w", cs.ID, err)
		}
	}
	if err := s.KnowledgeStore.SetChangeSetStatus(ctx, wsID, cs.ID, knowledge.ChangeSetInReview); err != nil {
		return nil, fmt.Errorf("submit change-set %s: %w", cs.ID, err)
	}
	cs.Status = knowledge.ChangeSetInReview

	s.publishKnowledgeEventsIn(wsSlug, actor, []knowledge.MergeEvent{
		changesetEvent(knowledge.EventChangeSetCreated, wsID, cs.ID, actor),
		changesetEvent(knowledge.EventChangeSetSubmitted, wsID, cs.ID, actor),
	})
	s.summonChangeSetReviewersIn(context.WithoutCancel(ctx), wsSlug, strings.TrimSuffix(s.Config.AppPublicURL, "/"), cs, len(ops), governed)
	return cs, nil
}

// conceptCreateOp is the op that proposes creating concept.
func conceptCreateOp(concept terms.Concept) (knowledge.ChangeSetOp, error) {
	payload, err := json.Marshal(knowledge.ConceptCreatePayload{Concept: concept})
	if err != nil {
		return knowledge.ChangeSetOp{}, fmt.Errorf("encode concept %s: %w", concept.ID, err)
	}
	return knowledge.ChangeSetOp{Op: knowledge.OpConceptCreate, Payload: payload}, nil
}
