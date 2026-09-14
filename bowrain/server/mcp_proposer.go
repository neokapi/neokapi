package server

import (
	"context"
	"fmt"

	"github.com/neokapi/neokapi/bowrain/knowledge"
	"github.com/neokapi/neokapi/terms"
)

// changeSetProposerAdapter lets the MCP terms tools propose a governed concept
// through the path the server's direct creation paths use.
type changeSetProposerAdapter struct{ s *Server }

// ProposeConcept proposes concept in a change-set of its own, in the workspace
// the tool named by slug.
func (a *changeSetProposerAdapter) ProposeConcept(ctx context.Context, workspace, actor string, concept terms.Concept) (string, error) {
	wsID := workspace
	if a.s.AuthStore != nil {
		if ws, err := a.s.AuthStore.GetWorkspaceBySlug(ctx, workspace); err == nil && ws != nil {
			wsID = ws.ID
		}
	}
	op, err := conceptCreateOp(concept)
	if err != nil {
		return "", err
	}
	name := concept.ID
	if len(concept.Terms) > 0 {
		name = concept.Terms[0].Text
	}
	cs, err := a.s.proposeGovernedChange(ctx, workspace, wsID, actor, fmt.Sprintf("Add %q as do-not-translate", name), []knowledge.ChangeSetOp{op})
	if err != nil {
		return "", err
	}
	return cs.ID, nil
}
