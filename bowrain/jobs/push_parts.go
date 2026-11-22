package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	platev "github.com/neokapi/neokapi/bowrain/core/event"
	"github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/bowrain/knowledge"
	"github.com/neokapi/neokapi/bowrain/termspush"
	"github.com/neokapi/neokapi/core/venue"
	apiclient "github.com/neokapi/neokapi/host/venue/client"
)

// The terminology and context ledger parts of a push.
//
// Both ride in the manifest beside the content and apply inside the push's
// transition, after the content: a refused terminology edit or ledger object
// rolls the content back with it, and the other way round. The commit handler
// checked both before it accepted the push (permissions, the shape of the
// edits, the reach of every ledger operation), so what can still fail here is
// what moved between the commit and the worker: a concept removed, a name
// taken, terminology that changed under a proposal.

// pushLedger is the context ledger part as the commit handler accepted it.
type pushLedger struct {
	Objects []apiclient.ContextObjectWire
	// Key is the project key the push binds the project's context to, empty
	// when the context was already bound when the handler checked.
	Key string
	// Operations counts the ledger operations the project did not hold when
	// the handler checked.
	Operations int
}

func (l pushLedger) empty() bool { return len(l.Objects) == 0 }

// applyPushLedger stores the ledger objects and binds the project's context
// on tx.
func applyPushLedger(ctx context.Context, tx store.PushApplier, projectID, actor string, l pushLedger) (*venue.PushLedgerOutcome, error) {
	if l.empty() {
		return nil, nil
	}
	out := &venue.PushLedgerOutcome{Objects: len(l.Objects), Operations: l.Operations}
	if l.Key != "" {
		now, err := tx.BindContextKey(ctx, projectID, l.Key)
		if err != nil {
			return nil, fmt.Errorf("bind the project's context: %w", err)
		}
		if now != l.Key {
			return nil, fmt.Errorf("this project's context belongs to project key %s, and the push carries project key %s", now, l.Key)
		}
		out.Bound = true
	}
	batch := make([]store.ContextObject, 0, len(l.Objects))
	for _, obj := range l.Objects {
		batch = append(batch, store.ContextObject{Name: obj.Name, Data: obj.Data})
	}
	if _, err := tx.PutContextObjects(ctx, projectID, pusherOf(ctx, actor), batch); err != nil {
		if exists, ok := errors.AsType[*store.ContextObjectExistsError](err); ok {
			return nil, fmt.Errorf("the project already holds context object %s with other bytes", exists.Name)
		}
		return nil, fmt.Errorf("store the context ledger: %w", err)
	}
	return out, nil
}

// pushTerms is the terminology part as the commit handler accepted it.
type pushTerms struct {
	Edits      *apiclient.TermsEdits
	Author     string
	OnBehalfOf string
}

// applyPushTerms applies the terminology edits on the transition's terms and
// knowledge stores.
func applyPushTerms(ctx context.Context, st PushStores, slug, workspaceID, stream string, t pushTerms) (*termspush.Result, error) {
	if t.Edits.Empty() {
		return nil, nil
	}
	if st.Terms == nil {
		return nil, errors.New("this worker cannot apply terminology edits in a push's transaction")
	}
	if slug == "" {
		return nil, errors.New("terminology edits need the project to be in a workspace")
	}
	tb, err := st.Terms(ctx, slug)
	if err != nil {
		return nil, fmt.Errorf("open the workspace's terms store: %w", err)
	}
	return termspush.Apply(ctx, termspush.Stores{Terms: tb, Knowledge: st.Knowledge}, termspush.Params{
		WorkspaceID: workspaceID,
		Stream:      stream,
		Actor:       t.Author,
		OnBehalfOf:  t.OnBehalfOf,
	}, t.Edits)
}

// recordPushParts writes what the terms and ledger parts did on the job row,
// where the producer reads it back through the status endpoint.
func recordPushParts(ctx context.Context, deps *WorkerDeps, jobID string, parts venue.PushParts) {
	if parts.Empty() || deps.JobStore == nil {
		return
	}
	payload, err := json.Marshal(parts)
	if err != nil {
		slog.WarnContext(ctx, "could not render the push parts report", "job_id", jobID, "error", err)
		return
	}
	if err := deps.JobStore.SetPushParts(ctx, jobID, string(payload)); err != nil {
		slog.WarnContext(ctx, "could not record what the push's terms and context parts did; the producer will not be told",
			"job_id", jobID, "error", err)
	}
}

// publishPushParts publishes the events the terms and ledger parts produced,
// once the transition has committed: the knowledge events of each concept
// and change-set the push touched, and the context change the ledger made.
func publishPushParts(deps *WorkerDeps, projectID, workspaceID, slug, actor string, terms *termspush.Result, contextEvent map[string]string) {
	if deps.EventBus == nil {
		return
	}
	now := time.Now().UTC()
	if terms != nil {
		for _, ev := range terms.Events {
			var digest *knowledge.ChangeSetDigest
			if terms.Submitted != nil && ev.ChangesetID == terms.Submitted.ID {
				digest = terms.Digest
			}
			deps.EventBus.Publish(knowledge.PlatformEvent(ev, "sync-worker", slug, actor, digest, now))
		}
	}
	if len(contextEvent) > 0 {
		deps.EventBus.Publish(platev.Event{
			Type:         platev.EventContextRecorded,
			Source:       "sync-worker",
			ProjectID:    projectID,
			WorkspaceID:  workspaceID,
			Actor:        actor,
			Data:         contextEvent,
			ResourceType: "context",
			ResourceID:   projectID,
			Timestamp:    now,
		})
	}
}
