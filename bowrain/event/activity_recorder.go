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

func (r *ActivityRecorder) handleEvent(ev platev.Event) {
	a := r.mapEventToActivity(ev)
	if a == nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := r.store.Create(ctx, a); err != nil {
		slog.Warn("activity recorder failed to persist activity", "event_id", ev.ID, "event_type", ev.Type, "error", err)
	}
}

func (r *ActivityRecorder) mapEventToActivity(ev platev.Event) *bstore.Activity {
	a := &bstore.Activity{
		ProjectID:   ev.ProjectID,
		ActorID:     ev.Actor,
		ActorName:   ev.Data["actor_name"],
		Data:        ev.Data,
		WorkspaceID: ev.Data["workspace_slug"],
	}
	if a.ActorID == "" {
		a.ActorID = "system"
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

	// Block events (only record updates, not individual creates for volume)
	case platev.EventBlockUpdated:
		a.Type = bstore.ActivityBlockTranslated
		a.EntityType = "block"
		a.EntityID = ev.Data["block_id"]
		a.Summary = "updated block " + ev.Data["block_id"]

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
		a.Summary = "flow failed"

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

	// Brand voice
	case platev.EventBrandVoiceDrift:
		a.Type = bstore.ActivityBrandDrift
		a.EntityType = "brand"
		a.Summary = "brand voice drift detected"

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

	// Connector sync
	case platev.EventSyncCompleted:
		a.Type = bstore.ActivityConnectorSynced
		a.EntityType = "connector"
		a.Summary = "connector sync completed"

	default:
		// Skip events we don't map to activities.
		return nil
	}

	return a
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
