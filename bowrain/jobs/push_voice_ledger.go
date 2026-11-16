package jobs

import (
	"context"

	"github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/bowrain/voicewrite"
	"github.com/neokapi/neokapi/core/workspace"
)

// contextObjectPutter stores ledger segments on a push's transaction.
type contextObjectPutter interface {
	PutContextObjects(ctx context.Context, projectID, pushedBy string, batch []store.ContextObject) (int, error)
}

// pushVoiceLedger records a push's voice profile writes in the pushed
// project's ledger, on the push's transaction (voicewrite.Ledger).
//
// The push's own ledger objects are stored after the collections are
// reconciled, so an operation the client recorded for a voice profile write
// is looked for among them as well as in what the project already holds.
type pushVoiceLedger struct {
	tx      contextObjectPutter
	held    store.ContextObjectStore
	project voicewrite.Project
	pushed  []store.ContextObject
	// pusher is the member whose push carries pushed.
	pusher string
}

var _ voicewrite.Ledger = (*pushVoiceLedger)(nil)

// newPushVoiceLedger is the ledger a push records its voice profile writes
// in: the project's, under the key the push binds or the key the project's
// context is already bound to. A project with neither holds no ledger, and
// the writes are recorded nowhere.
func newPushVoiceLedger(ctx context.Context, deps *WorkerDeps, tx contextObjectPutter, projectID, pusher string, l pushLedger) *pushVoiceLedger {
	out := &pushVoiceLedger{tx: tx, pusher: pusher, project: voicewrite.Project{ID: projectID, Key: workspace.ProjectKey(l.Key)}}
	if objects, ok := store.ContextObjects(deps.ContentStore); ok {
		out.held = objects
		if out.project.Key == "" {
			if key, err := objects.ContextKey(ctx, projectID); err == nil {
				out.project.Key = workspace.ProjectKey(key)
			}
		}
	}
	for _, obj := range l.Objects {
		out.pushed = append(out.pushed, store.ContextObject{Name: obj.Name, Data: obj.Data})
	}
	return out
}

// Projects implements voicewrite.Ledger.
func (l *pushVoiceLedger) Projects(context.Context) ([]voicewrite.Project, error) {
	if l.project.Key == "" {
		return nil, nil
	}
	return []voicewrite.Project{l.project}, nil
}

// Find implements voicewrite.Ledger: the push carries the operation, pushed
// by the pusher, or the project held it before the push.
func (l *pushVoiceLedger) Find(ctx context.Context, p voicewrite.Project, opID string) (voicewrite.HeldOp, bool, error) {
	if held, ok := voicewrite.FindIn(l.pushed, p.Key, opID); ok {
		held.PushedBy = l.pusher
		return held, true, nil
	}
	if l.held == nil {
		return voicewrite.HeldOp{}, false, nil
	}
	return voicewrite.FindStored(ctx, l.held, p, opID)
}

// Put implements voicewrite.Ledger.
func (l *pushVoiceLedger) Put(ctx context.Context, projectID, pushedBy string, obj store.ContextObject) error {
	_, err := l.tx.PutContextObjects(ctx, projectID, pushedBy, []store.ContextObject{obj})
	return err
}
