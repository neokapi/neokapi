package server

import (
	"context"
	"log/slog"
	"strings"

	platev "github.com/neokapi/neokapi/bowrain/core/event"
	"github.com/neokapi/neokapi/bowrain/knowledge"
)

// pushChangeSetGroup is the durable consumer-group name for the summons of
// change-sets a push proposed. Stable by design (see contextGraphGroup).
const pushChangeSetGroup = "push-changeset-summons"

// pushWorkerSource is the event source the push worker publishes under.
const pushWorkerSource = "sync-worker"

// subscribePushChangeSets summons the reviewers of a change-set a push
// proposed, and closes the review tasks of the ones it superseded.
//
// A push proposes its governed terminology edits in the worker, inside the
// transaction that stores its content, so the change-set is submitted after
// the request that carried it was answered. The worker publishes
// changeset.submitted and changeset.superseded once the push commits; this is
// where the summons a submit through the REST route makes in the request
// (HandleSubmitChangeSet) is made for them.
func (s *Server) subscribePushChangeSets() {
	if s.EventBus == nil || s.KnowledgeStore == nil {
		return
	}
	s.EventBus.SubscribeGroup(pushChangeSetGroup, func(ev platev.Event) error {
		if ev.Source != pushWorkerSource {
			return nil
		}
		switch ev.Type {
		case knowledge.EventChangeSetSubmitted:
			s.summonPushedChangeSet(ev)
		case knowledge.EventChangeSetSuperseded:
			if id := ev.Data["changeset_id"]; id != "" {
				s.closeChangeSetReviewTasks(context.Background(), ev.WorkspaceID, id, ev.Actor)
			}
		}
		return nil
	})
}

// summonPushedChangeSet opens the review task for a change-set a push
// submitted, notifies its reviewers, and records its blast radius.
func (s *Server) summonPushedChangeSet(ev platev.Event) {
	ctx := context.Background()
	wsSlug, csID := ev.Data["workspace_slug"], ev.Data["changeset_id"]
	if ev.WorkspaceID == "" || csID == "" {
		return
	}
	cs, err := s.KnowledgeStore.GetChangeSet(ctx, ev.WorkspaceID, csID)
	if err != nil || cs == nil {
		slog.Warn("push change-set summons skipped: change-set unreadable",
			"workspace_id", ev.WorkspaceID, "change_set_id", csID, "error", err)
		return
	}
	ops, err := s.KnowledgeStore.ListOps(ctx, ev.WorkspaceID, csID)
	if err != nil {
		slog.Warn("push change-set summons skipped: operations unreadable",
			"workspace_id", ev.WorkspaceID, "change_set_id", csID, "error", err)
		return
	}
	governed, _ := knowledge.ChangeSetIsGoverned(changeSetOpValues(ops))
	s.summonChangeSetReviewersIn(ctx, wsSlug, strings.TrimSuffix(s.Config.AppPublicURL, "/"), cs, len(ops), governed)
	s.recordChangeSetImpact(ctx, wsSlug, ev.WorkspaceID, csID)
}
