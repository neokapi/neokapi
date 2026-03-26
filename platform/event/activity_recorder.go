package event

import (
	"context"
	"log"
	"time"

	bstore "github.com/neokapi/neokapi/bowrain/store"
	platev "github.com/neokapi/neokapi/platform/event"
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
	r.sub = bus.SubscribeAll(r.handleEvent)
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
		log.Printf("WARNING: activity recorder failed to persist activity for event %s (type=%s): %v", ev.ID, ev.Type, err)
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
		a.Summary = "pushed content"
		if items := ev.Data["items"]; items != "" {
			a.Summary = "pushed " + items
		}
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
