package event

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"maps"
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
	store    *bstore.ActivityStore
	bus      platev.EventBus
	sub      *platev.Subscription
	projects ProjectWorkspaceResolver
}

// ActivityRecorderOption configures an ActivityRecorder.
type ActivityRecorderOption func(*ActivityRecorder)

// WithProjectWorkspaces files an event that names a project and no workspace
// under the project's workspace id. Without it such an event is filed under
// an empty workspace, where no feed lists it. The store-emitted project and
// stream events are of this kind.
func WithProjectWorkspaces(r ProjectWorkspaceResolver) ActivityRecorderOption {
	return func(ar *ActivityRecorder) { ar.projects = r }
}

// NewActivityRecorder creates and starts an activity recorder.
func NewActivityRecorder(store *bstore.ActivityStore, bus platev.EventBus, opts ...ActivityRecorderOption) *ActivityRecorder {
	r := &ActivityRecorder{store: store, bus: bus}
	for _, opt := range opts {
		opt(r)
	}
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

	if a.WorkspaceID == "" && a.ProjectID != "" && r.projects != nil {
		// A deleted project resolves to nothing, and redelivering its event
		// would not change that, so a failed lookup files the entry as is.
		wsID, err := r.projects.WorkspaceForProject(ctx, a.ProjectID)
		if err != nil {
			slog.Warn("activity recorder could not resolve the project's workspace", "event_id", ev.ID, "project_id", a.ProjectID, "error", err)
		}
		a.WorkspaceID = wsID
	}

	var err error
	if a.Kind == bstore.FeedKindReviewSummary {
		a.ID = reviewRollupID(a)
		err = r.store.AddToRollup(ctx, ev.ID, a)
	} else {
		err = r.store.Create(ctx, a)
	}
	if err != nil {
		slog.Warn("activity recorder failed to persist activity", "event_id", ev.ID, "event_type", ev.Type, "error", err)
		return err
	}
	return nil
}

// reviewRollupID names the roll-up row of a review decision: one row per
// workspace, project, actor, language and UTC hour. The hour is the one the
// row's first decision fell in, which is also the row's time in the feed.
func reviewRollupID(a *bstore.Activity) string {
	locale := ""
	if len(a.Locales) > 0 {
		locale = a.Locales[0]
	}
	hour := a.CreatedAt.UTC().Truncate(time.Hour).Format(time.RFC3339)
	sum := sha256.Sum256([]byte(strings.Join([]string{a.WorkspaceID, a.ProjectID, a.ActorID, locale, hour}, "\x00")))
	return "rvw_" + hex.EncodeToString(sum[:12])
}

// actorKindOf is the actor kind an event names, or the one its actor implies:
// an event with no actor was the system's (a tool), any other a person's.
func actorKindOf(ev platev.Event) string {
	switch k := ev.Data["actor_kind"]; k {
	case bstore.ActorKindPerson, bstore.ActorKindAgent, bstore.ActorKindTool:
		return k
	}
	if ev.Actor == "" || ev.Actor == "system" {
		return bstore.ActorKindTool
	}
	return bstore.ActorKindPerson
}

// splitList splits a comma-joined list, dropping empty entries.
func splitList(s string) []string {
	var out []string
	for part := range strings.SplitSeq(s, ",") {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}

// countsOf builds a counts map from the event's integer fields, keyed as the
// feed counts them. A field that is absent or not a number is left out.
func countsOf(data map[string]string, fields map[string]string) map[string]int {
	counts := map[string]int{}
	for key, field := range fields {
		if n := atoiOr(data[field], -1); n >= 0 {
			counts[key] = n
		}
	}
	return counts
}

// flatContextCounts folds a context change's counts into the feed's flat
// counts: one total per item kind and one per operation.
func flatContextCounts(c platev.ContextCounts) map[string]int {
	counts := map[string]int{}
	for kind, ops := range c {
		for op, n := range ops {
			counts[string(kind)] += n
			counts[string(op)] += n
		}
	}
	return counts
}

func (r *ActivityRecorder) mapEventToActivity(ev platev.Event) *bstore.Activity {
	a := &bstore.Activity{
		ProjectID: ev.ProjectID,
		ActorID:   ev.Actor,
		ActorName: ev.Data["actor_name"],
		Data:      ev.Data,
		// An explicit slug on the event wins, then the event's workspace id,
		// then (in handleEvent) the workspace of the project it names. Every
		// feed reads a workspace by both its slug and its id, so a row under
		// either is listed. A row under an empty workspace is listed nowhere,
		// which is why the project fallback exists.
		WorkspaceID: ev.Data["workspace_slug"],
		// The time the event happened, so a redelivered event sorts where it
		// happened rather than where it was recorded.
		CreatedAt: ev.Timestamp,
		ActorKind: actorKindOf(ev),
		Session:   ev.Data["session"],
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
		a.Type = bstore.ActivityProjectDeleted
		a.EntityType = "project"
		a.EntityID = ev.ProjectID
		a.Summary = "deleted project"

	// Push/pull
	case platev.EventPushCompleted:
		a.Type = bstore.ActivityItemPushed
		a.EntityType = "item"
		a.EntityID = ev.Data["push_id"]
		a.Summary = pushSummary(ev.Data)
		a.Kind = bstore.FeedKindPush
		a.Stream = ev.Data["stream"]
		if a.Stream == "" {
			a.Stream = ev.Data["branch"]
		}
		a.Counts = countsOf(ev.Data, map[string]string{
			"files": "files_count", "blocks": "blocks_count",
			"files_removed": "files_removed", "collections": "collections_count",
		})
	case platev.EventPullCompleted:
		a.Type = bstore.ActivityItemPulled
		a.EntityType = "item"
		a.Summary = "pulled content"

	// Block events never reach the feed: a push or converge touches
	// thousands of blocks, and the feed is for people: the push- and
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
		a.Type = bstore.ActivityStreamDeleted
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
		a.EntityID = qualityGateEntity(ev.Data)
		a.Summary = qualityGateSummary(ev.Data, "passed")
	case platev.EventQualityGateFail:
		a.Type = bstore.ActivityGateFailed
		a.EntityType = "gate"
		a.EntityID = qualityGateEntity(ev.Data)
		a.Summary = qualityGateSummary(ev.Data, "failed")

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
		a.Type = bstore.ActivityCollectionCreated
		a.EntityType = "collection"
		a.EntityID = ev.Data["collection_id"]
		a.Summary = "created collection " + ev.Data["name"]

	// Recipe changes: what a project declares, with the values before and
	// after the change.
	case platev.EventRecipeChanged:
		change := platev.ParseRecipeChange(ev.Data)
		if change.Empty() {
			return nil
		}
		a.Type = bstore.ActivityRecipeChanged
		a.EntityType = "project"
		a.EntityID = ev.ProjectID
		a.Kind = bstore.FeedKindRecipeChange
		a.Stream = change.Stream
		a.Counts = change.Counts()
		a.Locales = change.Locales()
		a.Coordinates = change.Coordinates()
		a.Summary = recipeChangeSummary(a.Counts)

	// Review decisions: rolled up into one row per actor, language and hour
	// (handleEvent), so a reviewer who approves 300 blocks in a morning files
	// a few rows rather than 300.
	case platev.EventReviewDecided, platev.EventReviewBulkApproved:
		locale := ev.Data["locale"]
		if locale == "" {
			return nil
		}
		a.Type = bstore.ActivityReviewSummary
		a.EntityType = "locale"
		a.EntityID = locale
		a.Kind = bstore.FeedKindReviewSummary
		a.Stream = ev.Data["stream"]
		a.Locales = []string{locale}
		a.Summary = "reviewed translations in " + locale
		// The row stands for many decisions; no one decision's data is its
		// own.
		a.Data = map[string]string{"locale": locale}
		if a.Stream != "" {
			a.Data["stream"] = a.Stream
		}
		if ev.Type == platev.EventReviewBulkApproved {
			n := atoiOr(ev.Data["approved"], 0)
			if n <= 0 {
				return nil
			}
			a.Counts = map[string]int{"bulk_approved": n}
		} else {
			decision := ev.Data["decision"]
			if decision == "" {
				return nil
			}
			a.Counts = map[string]int{decision: 1}
		}

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

	// Concepts, evidence, and discussion: the ordinary (ungoverned) edits to
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

	// Convergence runs: the loop itself, and what the feed is read to answer
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
		a.Kind = bstore.FeedKindConvergenceRun
		a.Locales = splitList(ev.Data["locales"])
		a.Counts = countsOf(ev.Data, map[string]string{
			"passes": "passes", "locales": "locales_count", "produced": "produced_count",
		})
		if ev.Actor == "" {
			a.ActorName = "convergence"
		}

	// Context changes: writes to a project's context ledger and decisions
	// about voice rules and profiles. An event that counts no item carries no
	// context change and stays out of the feed.
	case platev.EventContextRecorded, platev.EventVoiceProfileUpdated,
		platev.EventVoiceRulePromoted, platev.EventVoiceRuleAutoPromoted,
		platev.EventVoiceRuleRejected, platev.EventVoiceRuleDemoted:
		change := platev.ParseContextChange(ev.Data)
		if change.Counts.Total() == 0 {
			return nil
		}
		a.Type = bstore.ActivityContextChanged
		a.EntityType = string(change.ItemKind)
		if a.EntityType == "" {
			a.EntityType = "context"
		}
		a.EntityID = change.ItemID
		a.Summary = change.Counts.Describe()
		a.Kind = bstore.FeedKindContextChange
		a.Counts = flatContextCounts(change.Counts)
		// A voice event names the workspace as its project; a voice profile
		// belongs to the workspace, so the row names no project.
		if ev.ProjectID != "" && ev.ProjectID == ev.WorkspaceID {
			a.ProjectID = ""
		}

	default:
		// Skip events we don't map to activities.
		return nil
	}

	if strings.HasPrefix(string(ev.Type), changeSetEventPrefix) {
		a.Kind = bstore.FeedKindChangeSet
		// A change set belongs to the workspace; one tied to a project names
		// it in its data, and the project feed lists it there.
		if a.ProjectID == "" {
			a.ProjectID = ev.Data["project_id"]
		}
		// The step the change set reached, from the event's type. The
		// event's own map is shared with every other subscriber, so the row
		// gets a copy.
		a.Data = maps.Clone(ev.Data)
		if a.Data == nil {
			a.Data = map[string]string{}
		}
		a.Data["state"] = strings.TrimPrefix(string(ev.Type), changeSetEventPrefix)
		// The set's ops by type, when the event carries them.
		if raw := ev.Data["changeset_ops"]; raw != "" {
			ops := map[string]int{}
			if json.Unmarshal([]byte(raw), &ops) == nil {
				a.Counts = ops
			}
		}
	}
	return a
}

// changeSetEventPrefix marks the governed change-set events
// (knowledge.EventChangeSet*), the project feed's change_set kind.
const changeSetEventPrefix = "changeset."

// recipeChangeSummary is the workspace feed's line for a recipe change.
func recipeChangeSummary(counts map[string]int) string {
	var parts []string
	for _, k := range []struct{ key, noun string }{
		{"collections_added", "added %s"},
		{"collections_removed", "removed %s"},
		{"coordinates_changed", "moved %s"},
		{"targets_added", "added %s"},
		{"targets_removed", "removed %s"},
		{"settings_changed", "changed %s"},
	} {
		n := counts[k.key]
		if n == 0 {
			continue
		}
		unit := "collection"
		switch {
		case strings.HasPrefix(k.key, "targets"):
			unit = "language"
		case k.key == "settings_changed":
			unit = "setting"
		}
		parts = append(parts, fmt.Sprintf(k.noun, pluralCount(n, unit)))
	}
	if len(parts) == 0 {
		return "changed the recipe"
	}
	return strings.Join(parts, " · ")
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
// per-locale standing. It never enumerates the locales; the event's data
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

// qualityGateSummary is the feed line for a quality gate event, naming the gate
// and the language when the event carries them.
func qualityGateSummary(data map[string]string, outcome string) string {
	gate, locale := data["gate_name"], data["locale"]
	if gate == "" || locale == "" {
		return "quality gate " + outcome
	}
	return "quality gate " + gate + " " + outcome + " for " + locale
}

// qualityGateEntity identifies the gate an event is about, as gate:locale.
func qualityGateEntity(data map[string]string) string {
	if data["gate_name"] == "" || data["locale"] == "" {
		return ""
	}
	return data["gate_name"] + ":" + data["locale"]
}
