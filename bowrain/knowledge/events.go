package knowledge

import platev "github.com/neokapi/neokapi/bowrain/core/event"

// Knowledge-graph event types (AD-021). These are wired through the platform
// event bus → audit chain, notifications, SSE, and desktop watch by the server
// layer; this package only defines the constants. They reuse the platform's
// EventType so they interoperate with the existing bus and subscription
// machinery.
const (
	// Concept lifecycle.
	EventConceptCreated EventType = "concept.created"
	EventConceptUpdated EventType = "concept.updated"
	EventConceptDeleted EventType = "concept.deleted"

	// Concept term and relation changes.
	EventConceptTermStatusChanged EventType = "concept.term.status_changed"
	EventConceptRelationAdded     EventType = "concept.relation.added"
	EventConceptRelationRemoved   EventType = "concept.relation.removed"

	// Evidence and discussion.
	EventObservationAdded    EventType = "observation.added"
	EventConceptCommentAdded EventType = "concept.comment.added"

	// Change-set lifecycle.
	EventChangeSetCreated   EventType = "changeset.created"
	EventChangeSetSubmitted EventType = "changeset.submitted"
	EventChangeSetApproved  EventType = "changeset.approved"
	EventChangeSetRejected  EventType = "changeset.rejected"
	EventChangeSetMerged    EventType = "changeset.merged"
	EventChangeSetAbandoned EventType = "changeset.abandoned"

	// Pilots.
	EventPilotStarted EventType = "pilot.started"
	EventPilotStopped EventType = "pilot.stopped"
)

// EventType aliases the platform event type so knowledge-graph events flow
// through the same bus, audit chain, and subscriptions as the rest of the
// platform.
type EventType = platev.EventType
