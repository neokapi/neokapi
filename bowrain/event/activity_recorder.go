package event

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	platev "github.com/neokapi/neokapi/bowrain/core/event"
	"github.com/neokapi/neokapi/bowrain/knowledge"
	bstore "github.com/neokapi/neokapi/bowrain/store"
)

// ActivityRecorder subscribes to all events and creates curated activities
// for human consumption. Unlike the AuditLogger (which records raw events),
// the ActivityRecorder produces aggregated, human-readable activity records.
type ActivityRecorder struct {
	store *bstore.ActivityStore
	bus   platev.EventBus
	sub   *platev.Subscription
}

// NewActivityRecorder creates and starts an activity recorder.
func NewActivityRecorder(store *bstore.ActivityStore, bus platev.EventBus) *ActivityRecorder {
	r := &ActivityRecorder{store: store, bus: bus}
	r.sub = bus.SubscribeGroup("activity-recorder", r.handleEvent)
	return r
}

// Close unsubscribes from the event bus.
func (r *ActivityRecorder) Close() {
	if r.sub != nil {
		r.bus.Unsubscribe(r.sub)
	}
}

// handleEvent files one activity, and reports whether it landed.
//
// The feed is the surface people read to answer what happened, so a write that
// failed is worth redelivering. The entry is keyed on the event id, so the
// second delivery of an event already filed is a no-op.
func (r *ActivityRecorder) handleEvent(ev platev.Event) error {
	a := r.mapEventToActivity(ev)
	if a == nil {
		return nil
	}
	a.EventID = ev.ID

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := r.store.Create(ctx, a); err != nil {
		slog.Warn("activity recorder failed to persist activity", "event_id", ev.ID, "event_type", ev.Type, "error", err)
		return err
	}
	return nil
}

func (r *ActivityRecorder) mapEventToActivity(ev platev.Event) *bstore.Activity {
	a := &bstore.Activity{
		ProjectID: ev.ProjectID,
		ActorID:   ev.Actor,
		ActorName: ev.Data["actor_name"],
		Data:      ev.Data,
		// The feed lists by workspace *slug* (GET /:ws/activities), so an
		// explicit slug on the event wins: the knowledge path sets
		// Event.WorkspaceID to the workspace UUID and carries the slug here,
		// and preferring the UUID filed those rows where no feed could list
		// them. The event's own field is the fallback the sync path needs — it
		// sets WorkspaceID and carries neither Data key, which once filed 17k
		// activities under an empty workspace.
		WorkspaceID: ev.Data["workspace_slug"],
	}
	if a.ActorID == "" {
		a.ActorID = "system"
	}
	if a.WorkspaceID == "" {
		a.WorkspaceID = ev.WorkspaceID
	}
	if a.WorkspaceID == "" {
		a.WorkspaceID = ev.Data["workspace_id"]
	}

	switch ev.Type {
	// Project lifecycle
	case platev.EventProjectCreated:
		a.Type = bstore.ActivityProjectCreated
		a.EntityType = "project"
		a.EntityID = ev.ProjectID
		a.Summary = "created project " + ev.Data["name"]
	case platev.EventProjectUpdated:
		a.Type = bstore.ActivityProjectUpdated
		a.EntityType = "project"
		a.EntityID = ev.ProjectID
		a.Summary = "updated project " + ev.Data["name"]
	case platev.EventProjectDeleted:
		a.Type = bstore.ActivityProjectCreated // reuse for deletion display
		a.EntityType = "project"
		a.EntityID = ev.ProjectID
		a.Summary = "deleted project"

	// Push/pull
	case platev.EventPushCompleted:
		a.Type = bstore.ActivityItemPushed
		a.EntityType = "item"
		a.Summary = pushSummary(ev.Data)
	case platev.EventPullCompleted:
		a.Type = bstore.ActivityItemPulled
		a.EntityType = "item"
		a.Summary = "pulled content"

	// Block events never reach the feed: a push or converge touches
	// thousands of blocks, and the feed is for people — the push- and
	// run-level entries above are the readable record of that work.

	// Streams
	case platev.EventStreamCreated:
		a.Type = bstore.ActivityStreamCreated
		a.EntityType = "stream"
		a.EntityID = ev.Data["stream"]
		a.Summary = "created stream " + ev.Data["stream"]
		a.Stream = ev.Data["stream"]
	case platev.EventStreamMerged:
		a.Type = bstore.ActivityStreamMerged
		a.EntityType = "stream"
		a.EntityID = ev.Data["stream"]
		a.Summary = "merged stream " + ev.Data["stream"]
	case platev.EventStreamDeleted:
		a.Type = bstore.ActivityStreamCreated // reuse
		a.EntityType = "stream"
		a.EntityID = ev.Data["stream"]
		a.Summary = "deleted stream " + ev.Data["stream"]

	case platev.EventStreamLocked:
		a.Type = bstore.ActivityStreamLocked
		a.EntityType = "stream"
		a.EntityID = ev.Data["stream"]
		a.Summary = "locked stream " + ev.Data["stream"]
	case platev.EventStreamUnlocked:
		a.Type = bstore.ActivityStreamUnlocked
		a.EntityType = "stream"
		a.EntityID = ev.Data["stream"]
		a.Summary = "unlocked stream " + ev.Data["stream"]
	case platev.EventStreamTagged:
		a.Type = bstore.ActivityStreamTagged
		a.EntityType = "stream"
		a.EntityID = ev.Data["stream"]
		a.Summary = "tagged stream " + ev.Data["stream"] + " as " + ev.Data["tag"]

	// Flows
	case platev.EventFlowCompleted:
		a.Type = bstore.ActivityFlowCompleted
		a.EntityType = "flow"
		a.Summary = "flow completed"
	case platev.EventFlowFailed:
		a.Type = bstore.ActivityFlowFailed
		a.EntityType = "flow"
		a.EntityID = ev.Data["job_id"]
		a.Summary = flowFailedSummary(ev.Data)

	// Extraction
	case platev.EventExtractionCompleted:
		a.Type = bstore.ActivityExtractionDone
		a.EntityType = "extraction"
		a.Summary = "extraction completed"

	// Quality gates
	case platev.EventQualityGatePass:
		a.Type = bstore.ActivityGatePassed
		a.EntityType = "gate"
		a.Summary = "quality gate passed"
	case platev.EventQualityGateFail:
		a.Type = bstore.ActivityGateFailed
		a.EntityType = "gate"
		a.Summary = "quality gate failed"

	// Voice
	case platev.EventVoiceDrift:
		a.Type = bstore.ActivityVoiceDrift
		a.EntityType = "brand"
		a.Summary = "voice drift detected"

	// Versions
	case platev.EventVersionCreated:
		a.Type = bstore.ActivityVersionCreated
		a.EntityType = "version"
		a.EntityID = ev.Data["version_id"]
		a.Summary = "created version " + ev.Data["label"]

	// Collections
	case platev.EventCollectionCreated:
		a.Type = bstore.ActivityProjectUpdated
		a.EntityType = "collection"
		a.EntityID = ev.Data["collection_id"]
		a.Summary = "created collection " + ev.Data["name"]

	// Governed change-sets. The graph's lifecycle is audit-logged separately;
	// these cases are what puts it in the feed people actually read.
	case knowledge.EventChangeSetSubmitted:
		a.Type = bstore.ActivityReviewAssigned
		a.EntityType = "changeset"
		a.EntityID = ev.Data["changeset_id"]
		a.Summary = "submitted a change for review"
	case knowledge.EventChangeSetApproved:
		a.Type = bstore.ActivityReviewDecided
		a.EntityType = "changeset"
		a.EntityID = ev.Data["changeset_id"]
		a.Summary = "approved a change"
	case knowledge.EventChangeSetRejected:
		a.Type = bstore.ActivityReviewDecided
		a.EntityType = "changeset"
		a.EntityID = ev.Data["changeset_id"]
		a.Summary = "rejected a change"
	case knowledge.EventChangeSetMerged:
		a.Type = bstore.ActivityReviewDecided
		a.EntityType = "changeset"
		a.EntityID = ev.Data["changeset_id"]
		a.Summary = "merged a change into the workspace terms"
	case knowledge.EventChangeSetCreated:
		a.Type = bstore.ActivityChangeSetOpened
		a.EntityType = "changeset"
		a.EntityID = ev.Data["changeset_id"]
		a.Summary = "opened an experiment"
	case knowledge.EventChangeSetAbandoned:
		a.Type = bstore.ActivityChangeSetAbandoned
		a.EntityType = "changeset"
		a.EntityID = ev.Data["changeset_id"]
		a.Summary = "abandoned an experiment"
	case knowledge.EventChangeSetSuperseded:
		a.Type = bstore.ActivityChangeSetSuperseded
		a.EntityType = "changeset"
		a.EntityID = ev.Data["changeset_id"]
		a.Summary = "superseded an experiment with a newer proposal"

	// Pilots: a change-set bound to a project's content stream, tried before it
	// is merged everywhere.
	case knowledge.EventPilotStarted:
		a.Type = bstore.ActivityPilotStarted
		a.EntityType = "changeset"
		a.EntityID = ev.Data["changeset_id"]
		a.Stream = ev.Data["stream"]
		a.Summary = pilotSummary("started piloting a change", ev.Data)
	case knowledge.EventPilotStopped:
		a.Type = bstore.ActivityPilotStopped
		a.EntityType = "changeset"
		a.EntityID = ev.Data["changeset_id"]
		a.Stream = ev.Data["stream"]
		a.Summary = pilotSummary("stopped piloting a change", ev.Data)

	// Concepts, evidence, and discussion — the ordinary (ungoverned) edits to
	// the graph, which are most of what a steward's day looks like.
	case knowledge.EventConceptCreated:
		a.Type = bstore.ActivityConceptCreated
		a.EntityType = "concept"
		a.EntityID = ev.Data["concept_id"]
		a.Summary = "created a concept"
	case knowledge.EventConceptUpdated:
		a.Type = bstore.ActivityConceptUpdated
		a.EntityType = "concept"
		a.EntityID = ev.Data["concept_id"]
		a.Summary = "updated a concept"
	case knowledge.EventConceptDeleted:
		a.Type = bstore.ActivityConceptDeleted
		a.EntityType = "concept"
		a.EntityID = ev.Data["concept_id"]
		a.Summary = "deleted a concept"
	case knowledge.EventConceptTermStatusChanged:
		a.Type = bstore.ActivityConceptTermStatus
		a.EntityType = "concept"
		a.EntityID = ev.Data["concept_id"]
		a.Summary = "changed a term's status"
	case knowledge.EventConceptRelationAdded:
		a.Type = bstore.ActivityConceptRelationAdded
		a.EntityType = "concept"
		a.EntityID = ev.Data["concept_id"]
		a.Summary = "connected two concepts"
	case knowledge.EventConceptRelationRemoved:
		a.Type = bstore.ActivityConceptRelationRemoved
		a.EntityType = "concept"
		a.EntityID = ev.Data["concept_id"]
		a.Summary = "removed a relation between concepts"
	case knowledge.EventObservationAdded:
		a.Type = bstore.ActivityObservationAdded
		a.EntityType = "concept"
		a.EntityID = ev.Data["concept_id"]
		a.Summary = "recorded an observation"
	case knowledge.EventConceptCommentAdded:
		a.Type = bstore.ActivityCommentAdded
		a.EntityType = "concept"
		a.EntityID = ev.Data["concept_id"]
		a.Summary = "commented on a concept"

	// Connector sync
	case platev.EventSyncCompleted:
		a.Type = bstore.ActivityConnectorSynced
		a.EntityType = "connector"
		a.Summary = "connector sync completed"

	// Convergence runs — the loop itself, and what the feed is read to answer
	// "did the loop run, and what came of it" with. Mapped for every terminal
	// state, because a failed or parked run is the one worth scanning for; the
	// summary carries the real outcome word.
	case platev.EventConvergenceRunCompleted:
		a.Type = bstore.ActivityFlowCompleted
		if ev.Data["state"] == "failed" {
			a.Type = bstore.ActivityFlowFailed
		}
		a.EntityType = "run"
		a.EntityID = ev.Data["run_id"]
		a.Summary = convergenceRunSummary(ev.Data)

	default:
		// Skip events we don't map to activities.
		return nil
	}

	return a
}

// pilotSummary names where a pilot runs when the event says, because "started
// piloting a change" alone leaves the reader asking the obvious next question.
func pilotSummary(verb string, data map[string]string) string {
	project, stream := data["project_id"], data["stream"]
	switch {
	case project != "" && stream != "":
		return verb + " on " + project + " · " + stream
	case project != "":
		return verb + " on " + project
	default:
		return verb
	}
}

// flowFailedSummary is the line the activity feed shows for a failed job.
//
// A bare "flow failed" is accurate and useless: the feed's whole job is to let
// someone scan it and know whether to look closer. The event carries the kind,
// what was being worked on, and the recorded reason, so the summary says all
// three.
func flowFailedSummary(data map[string]string) string {
	kind := data["job_kind"]
	if kind == "" {
		kind = "background"
	}
	s := kind + " job failed"
	if item := data["item"]; item != "" {
		s += ": " + item
		if locale := data["target_locale"]; locale != "" {
			s += " → " + locale
		}
	}
	if reason := data["error"]; reason != "" {
		s += ": " + reason
	}
	return s
}

// pushSummary builds a compact, human-readable summary for a completed push.
//
// It prefers the structured counts the sync worker now emits
// ("files_count"/"blocks_count") and renders e.g. "pushed 474 files · 20,345
// blocks", so the activity feed never enumerates every pushed path. When those
// fields are absent (an older event shape), it falls back to counting the
// comma-joined "items" list rather than embedding it verbatim.
func pushSummary(data map[string]string) string {
	files := atoiOr(data["files_count"], -1)
	if files < 0 {
		// Legacy event: derive the file count from the joined item list.
		if items := data["items"]; items != "" {
			files = len(strings.Split(items, ","))
		} else {
			files = 0
		}
	}
	blocks := atoiOr(data["blocks_count"], -1)

	if files == 0 {
		return "pushed content"
	}

	s := "pushed " + pluralCount(files, "file")
	if blocks >= 0 {
		s += " · " + pluralCount(blocks, "block")
	}
	return s
}

// pluralCount renders a count with a thousands-separated number and a
// naively-pluralized noun, e.g. pluralCount(1, "file") == "1 file" and
// pluralCount(20345, "block") == "20,345 blocks".
func pluralCount(n int, noun string) string {
	unit := noun
	if n != 1 {
		unit += "s"
	}
	return fmt.Sprintf("%s %s", groupThousands(n), unit)
}

// groupThousands formats a non-negative integer with comma thousands
// separators (e.g. 20345 -> "20,345").
func groupThousands(n int) string {
	s := strconv.Itoa(n)
	neg := ""
	if n < 0 {
		neg = "-"
		s = s[1:]
	}
	if len(s) <= 3 {
		return neg + s
	}
	var b strings.Builder
	pre := len(s) % 3
	if pre > 0 {
		b.WriteString(s[:pre])
	}
	for i := pre; i < len(s); i += 3 {
		if b.Len() > 0 {
			b.WriteByte(',')
		}
		b.WriteString(s[i : i+3])
	}
	return neg + b.String()
}

// convergenceRunSummary is the feed line for a finished convergence run.
//
// It says what the run did, not merely that it ended: the outcome word the run
// row records (converged | parked | failed | canceled), the machine-readable
// stall reason when one parked it, and the run's magnitude folded out of its
// per-locale standing. It never enumerates the locales — the event's data
// carries the list for a drill-down, the summary carries the count.
func convergenceRunSummary(data map[string]string) string {
	state := data["state"]
	if state == "" {
		state = "finished"
	}
	s := "convergence run " + state
	if reason := data["stall_reason"]; reason != "" {
		s += " (" + reason + ")"
	}

	var detail []string
	if locales := atoiOr(data["locales_count"], 0); locales > 0 {
		detail = append(detail, pluralCount(locales, "language"))
	}
	if produced := atoiOr(data["produced_count"], -1); produced >= 0 {
		detail = append(detail, pluralCount(produced, "translation")+" produced")
	}
	if len(detail) > 0 {
		s += ": " + strings.Join(detail, " · ")
	}
	return s
}

// atoiOr parses s as an int, returning def when s is empty or invalid.
func atoiOr(s string, def int) int {
	if s == "" {
		return def
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return n
}
