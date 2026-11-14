package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"

	platev "github.com/neokapi/neokapi/bowrain/core/event"
	"github.com/neokapi/neokapi/bowrain/knowledge"
	"github.com/neokapi/neokapi/bowrain/termspush"
	"github.com/neokapi/neokapi/core/venue"
)

// errPushAlreadyApplied rolls back a transition whose job an earlier run
// already applied (PushApplier.ClaimPushApply).
var errPushAlreadyApplied = errors.New("sync: this push job was already applied")

// pushApplyRecord is what one push job applied, stored in the transaction
// that applied it. A job redelivered after that transaction committed applies
// nothing again; it finishes from this record, publishing the same events and
// recording the same report as the run that applied it.
type pushApplyRecord struct {
	Parts       venue.PushParts            `json:"parts"`
	Governance  venue.PushGovernance       `json:"governance"`
	Stored      int                        `json:"stored"`
	Items       []string                   `json:"items,omitempty"`
	Removed     int                        `json:"removed,omitempty"`
	Collections int                        `json:"collections,omitempty"`
	Undeclared  []string                   `json:"undeclared,omitempty"`
	TermsEvents []knowledge.MergeEvent     `json:"terms_events,omitempty"`
	Digest      *knowledge.ChangeSetDigest `json:"digest,omitempty"`
	SubmittedID string                     `json:"submitted_id,omitempty"`
}

func newPushApplyRecord(parts venue.PushParts, outcome *pushOutcome, ctxResult *contextReconcileResult, terms *termspush.Result) pushApplyRecord {
	rec := pushApplyRecord{Parts: parts}
	if outcome != nil {
		rec.Governance = outcome.Governance
		rec.Stored = outcome.Stored
		rec.Items = outcome.ItemNames
		rec.Removed = outcome.Removed
	}
	if ctxResult != nil {
		rec.Collections = ctxResult.total()
		rec.Undeclared = ctxResult.Undeclared
	}
	if terms != nil {
		rec.TermsEvents = terms.Events
		rec.Digest = terms.Digest
		if terms.Submitted != nil {
			rec.SubmittedID = terms.Submitted.ID
		}
	}
	return rec
}

// termsResult is the record's terminology part as Apply returned it, for the
// events publishPushParts sends.
func (r pushApplyRecord) termsResult() *termspush.Result {
	if len(r.TermsEvents) == 0 {
		return nil
	}
	res := &termspush.Result{Outcome: r.Parts.Terms, Events: r.TermsEvents, Digest: r.Digest}
	if r.SubmittedID != "" {
		res.Submitted = &knowledge.ChangeSet{ID: r.SubmittedID}
	}
	return res
}

// finishReplayedPush completes a push job an earlier run applied: it records
// the report the producer reads, marks the job completed, reclaims the staging
// blobs and publishes the events the push owes. Events are delivered at least
// once: a run that died after publishing them publishes them again here.
func finishReplayedPush(ctx context.Context, deps *WorkerDeps, job *TranslationJob, manifest *syncPushManifest, stream, manifestKey string, raw []byte) error {
	var rec pushApplyRecord
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &rec); err != nil {
			markJobFailed(ctx, deps, job.ID, "push apply record unreadable")
			return fmt.Errorf("read the push apply record: %w", err)
		}
	}
	emitLog(deps, job.StepID, "info", "This push was already applied; finishing it", nil)
	recordPushParts(ctx, deps, job.ID, rec.Parts)
	recordPushGovernance(ctx, deps, job.ID, rec.Governance)
	if rec.Stored > 0 && deps.SyncCache != nil {
		deps.SyncCache.InvalidateProject(ctx, job.ProjectID)
	}
	if err := deps.JobStore.UpdateJobStatus(ctx, job.ID, StatusCompleted, ""); err != nil {
		return fmt.Errorf("mark sync push job completed: %w", err)
	}
	reclaimPushBlobs(ctx, deps, job.ID, manifestKey, manifest.Chunks)
	workspaceID := ""
	if p, err := deps.ContentStore.GetProject(ctx, job.ProjectID); err == nil && p != nil {
		workspaceID = p.WorkspaceID
	}
	publishPushCompleted(deps, job.ProjectID, stream, job.PushID, manifest, rec)
	publishPushParts(deps, job.ProjectID, workspaceID, manifest.WorkspaceSlug, manifest.ActorID, rec.termsResult(), manifest.ContextEvent)
	return nil
}

// reclaimPushBlobs deletes the manifest and every chunk blob it referenced.
// A failed delete orphans a blob rather than losing content, so it warns.
func reclaimPushBlobs(ctx context.Context, deps *WorkerDeps, jobID, manifestKey string, chunks []syncChunkRef) {
	if err := deps.BlobStore.Delete(ctx, manifestKey); err != nil {
		slog.Warn("could not delete sync push manifest blob",
			"job_id", jobID, "manifest_key", manifestKey, "error", err)
	}
	for _, chunkRef := range chunks {
		if err := deps.BlobStore.Delete(ctx, chunkRef.Hash); err != nil {
			slog.Warn("could not delete sync push chunk blob",
				"job_id", jobID, "chunk_hash", chunkRef.Hash, "error", err)
		}
	}
}

// publishPushCompleted publishes EventPushCompleted for a push that stored
// content or declared collections.
//
// A push that lands only context (a recipe declaring a new collection, moving
// a point, renaming a channel) stores no blocks, and the consumers that care
// are exactly the ones a context change is for: the workspace context graph,
// and the channel-slug equivalence the workspace observes across its projects.
func publishPushCompleted(deps *WorkerDeps, projectID, stream, pushID string, manifest *syncPushManifest, rec pushApplyRecord) {
	if (rec.Stored == 0 && rec.Collections == 0) || deps.EventBus == nil {
		return
	}
	// The full item list rides on "items" because downstream consumers
	// (per-locale automations in server/automation.go) fan out by item name.
	// The activity summary is built from the structured counts.
	deps.EventBus.Publish(platev.Event{
		Type:      platev.EventPushCompleted,
		Source:    "sync-worker",
		ProjectID: projectID,
		Actor:     manifest.ActorID,
		Data: map[string]string{
			"stream":       stream,
			"items":        strings.Join(rec.Items, ","),
			"items_sample": strings.Join(sampleItemNames(rec.Items, 3), ","),
			"files_count":  strconv.Itoa(len(rec.Items)),
			"blocks_count": strconv.Itoa(rec.Stored),
			// Files the source no longer has, which the push removed, and the
			// collections the recipe declared.
			"files_removed":     strconv.Itoa(rec.Removed),
			"collections_count": strconv.Itoa(rec.Collections),
			"push_id":           pushID,
			// Collections the recipe stopped declaring. Reported, never acted
			// on: the server keeps them and their content.
			"undeclared_collections": strings.Join(rec.Undeclared, ","),
			"workspace_slug":         manifest.WorkspaceSlug,
		},
	})
}
