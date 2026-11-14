package knowledge

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	platev "github.com/neokapi/neokapi/bowrain/core/event"
	"github.com/neokapi/neokapi/core/id"
)

// ErrSubmitRefused wraps a status transition the store refused while a
// change-set was submitted: the draft, or an open change-set of its origin,
// was no longer in the state the submit needed. The REST route answers it
// with 409.
var ErrSubmitRefused = errors.New("change-set submit refused")

// SubmitResult is what submitting a change-set did.
type SubmitResult struct {
	// ID names the change-set in review: the one submitted, or, when
	// Unchanged, the open change-set of the same origin that already holds
	// the same ops.
	ID string
	// Unchanged reports that an open change-set of the same origin held the
	// same ops, so the submitted draft was abandoned and ID names that one.
	Unchanged bool
	// Superseded lists the open change-sets of the same origin that this one
	// replaced, in the order they were superseded.
	Superseded []string
}

// SubmitChangeSet moves the draft cs, holding ops, into review.
//
// A change-set that carries an origin is an automated proposal, and an origin
// admits one open proposal at a time. An open change-set of the origin in
// review with the same ops makes the submit idempotent: the draft is abandoned
// and the open one is returned. Any other open change-set of the origin is
// superseded by this one, so repeated pushes converge to a single reviewable
// change-set.
//
// The caller checks that cs is a non-empty draft, and publishes the events
// and summons that follow.
func SubmitChangeSet(ctx context.Context, st Store, cs *ChangeSet, ops []*ChangeSetOp) (SubmitResult, error) {
	res := SubmitResult{ID: cs.ID}
	if cs.Origin != "" {
		open, err := st.ListOpenChangeSetsByOrigin(ctx, cs.WorkspaceID, cs.Origin)
		if err != nil {
			return res, err
		}
		for _, prior := range open {
			if prior.ID == cs.ID {
				continue
			}
			priorOps, err := st.ListOps(ctx, cs.WorkspaceID, prior.ID)
			if err != nil {
				return res, err
			}
			if prior.Status == ChangeSetInReview && SameOps(priorOps, ops) {
				if err := st.SetChangeSetStatus(ctx, cs.WorkspaceID, cs.ID, ChangeSetAbandoned); err != nil {
					return res, fmt.Errorf("%w: %w", ErrSubmitRefused, err)
				}
				res.ID, res.Unchanged = prior.ID, true
				return res, nil
			}
			if err := st.SupersedeChangeSet(ctx, cs.WorkspaceID, prior.ID, cs.ID); err != nil {
				return res, fmt.Errorf("%w: %w", ErrSubmitRefused, err)
			}
			res.Superseded = append(res.Superseded, prior.ID)
		}
	}
	if err := st.SetChangeSetStatus(ctx, cs.WorkspaceID, cs.ID, ChangeSetInReview); err != nil {
		return res, fmt.Errorf("%w: %w", ErrSubmitRefused, err)
	}
	return res, nil
}

// ChangeSetDigest is what an event about a change-set shows of it: its name
// and the number of its ops by op type.
type ChangeSetDigest struct {
	Name string
	Ops  map[string]int
}

// DigestChangeSet counts a change-set's ops by op type.
func DigestChangeSet(name string, ops []*ChangeSetOp) *ChangeSetDigest {
	sum := &ChangeSetDigest{Name: name, Ops: map[string]int{}}
	for _, op := range ops {
		if op != nil {
			sum.Ops[string(op.Op)]++
		}
	}
	return sum
}

// PlatformEvent is the platform event a knowledge event is published as. The
// activity recorder and the notification dispatcher key on the workspace slug,
// so it rides in the data beside the ids the event names. fallbackActor
// attributes an event that names no actor, and summary, when set, adds the
// change-set's name and op counts so the feed can show what the set holds.
func PlatformEvent(ev MergeEvent, source, wsSlug, fallbackActor string, summary *ChangeSetDigest, at time.Time) platev.Event {
	actor := ev.Actor
	if actor == "" {
		actor = fallbackActor
	}
	data := map[string]string{}
	put := func(k, v string) {
		if v != "" {
			data[k] = v
		}
	}
	put("workspace_slug", wsSlug)
	put("changeset_id", ev.ChangesetID)
	put("on_behalf_of", ev.OnBehalfOf)
	put("review_basis", string(ev.ReviewBasis))
	put("concept_id", ev.ConceptID)
	put("profile_id", ev.ProfileID)
	put("project_id", ev.ProjectID)
	put("stream", ev.Stream)
	if summary != nil {
		put("changeset_name", summary.Name)
		if len(summary.Ops) > 0 {
			if raw, err := json.Marshal(summary.Ops); err == nil {
				data["changeset_ops"] = string(raw)
			}
		}
	}
	resType, resID := EventResource(ev)
	return platev.Event{
		ID:           id.New(),
		Type:         ev.Type,
		Source:       source,
		WorkspaceID:  ev.WorkspaceID,
		Actor:        actor,
		Data:         data,
		ResourceType: resType,
		ResourceID:   resID,
		Timestamp:    at,
	}
}

// EventResource maps a knowledge event to the audit resource (type, ID) that
// best identifies what it touched.
func EventResource(ev MergeEvent) (resourceType, resourceID string) {
	switch {
	case ev.ConceptID != "":
		return "concept", ev.ConceptID
	case ev.ChangesetID != "":
		return "changeset", ev.ChangesetID
	case ev.ProfileID != "":
		return "voice_profile", ev.ProfileID
	default:
		return "", ""
	}
}
