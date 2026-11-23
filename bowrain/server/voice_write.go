package server

import (
	"context"

	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/bowrain/storage"
	bstore "github.com/neokapi/neokapi/bowrain/store"
	platvoice "github.com/neokapi/neokapi/bowrain/voice"
	"github.com/neokapi/neokapi/bowrain/voicewrite"
	coreprofile "github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/core/workspace"
)

// writeVoiceProfile writes a workspace voice profile and records the write
// on the context ledger of every project in the workspace whose context is
// bound (voicewrite.Apply). Every server surface that writes a voice profile
// calls it; a push writes through the same function in the worker.
//
// Where the voice store and the content store, which holds the ledger's
// segments, share one database, the profile and its record are written in
// one transaction. Otherwise voicewrite.Apply writes the profile first and
// the record after it: a record that fails then leaves a write with no
// author and returns the error, where the other order could leave a record
// of a write that never happened.
func (s *Server) writeVoiceProfile(ctx context.Context, wsID string, w voicewrite.Write) (voicewrite.Result, error) {
	objects, hasLedger := s.contextObjects()
	vs, vok := s.VoiceStore.(*platvoice.PostgresVoiceStore)
	cs, cok := postgresContentStore(s.ContentStore)
	if hasLedger && vok && cok && vs.DB() == cs.DB() {
		var res voicewrite.Result
		err := vs.DB().Transition(ctx, func(tx storage.Runner) error {
			applier := cs.Bind(tx)
			// Every read the write makes runs on this transaction, never a
			// second pooled connection.
			ledger := &workspaceVoiceLedger{wsID: wsID, reads: cs.LedgerReadsOn(tx),
				put: func(ctx context.Context, projectID string, pushedBy platstore.Pusher, obj platstore.ContextObject) error {
					_, err := applier.PutContextObjects(ctx, projectID, pushedBy, []platstore.ContextObject{obj})
					return err
				}}
			var err error
			res, err = voicewrite.Apply(ctx, vs.Bind(tx), ledger, w)
			return err
		})
		return res, err
	}
	var ledger voicewrite.Ledger
	if hasLedger {
		ledger = &workspaceVoiceLedger{wsID: wsID, reads: pooledLedgerReads{ContentStore: s.ContentStore, ContextObjectStore: objects},
			put: func(ctx context.Context, projectID string, pushedBy platstore.Pusher, obj platstore.ContextObject) error {
				return objects.PutContextObjects(ctx, projectID, pushedBy, []platstore.ContextObject{obj})
			}}
	}
	return voicewrite.Apply(ctx, s.VoiceStore, ledger, w)
}

// memberVoiceWrite is a write of a voice profile by the actor a request acts
// as: a signed-in member, or a tool under a member's tool token.
func memberVoiceWrite(a requestActor, before, after *coreprofile.VoiceProfile, note string) voicewrite.Write {
	return memberVoiceUpdate(a, before, after, "", note)
}

// memberVoiceUpdate is memberVoiceWrite for an update based on revision base.
func memberVoiceUpdate(a requestActor, before, after *coreprofile.VoiceProfile, base, note string) voicewrite.Write {
	return voicewrite.Write{
		Before:   before,
		After:    after,
		Base:     base,
		Actor:    a.contextActor(),
		PushedBy: a.pusher(),
		Note:     note,
	}
}

// postgresContentStore finds the PostgreSQL content store behind a content
// store, looking through any decorator that names what it wraps.
func postgresContentStore(cs platstore.ContentStore) (*bstore.PostgresStore, bool) {
	for cs != nil {
		if pg, ok := cs.(*bstore.PostgresStore); ok {
			return pg, true
		}
		inner, ok := cs.(interface{ Unwrap() platstore.ContentStore })
		if !ok {
			return nil, false
		}
		cs = inner.Unwrap()
	}
	return nil, false
}

// workspaceVoiceLedger records a voice profile write in the ledger of every
// project of a workspace whose context is bound to a project key. A voice
// profile belongs to the workspace, so each project that may bind it holds
// the record of who wrote it.
type workspaceVoiceLedger struct {
	wsID  string
	reads voiceLedgerReads
	put   func(ctx context.Context, projectID string, pushedBy platstore.Pusher, obj platstore.ContextObject) error
}

// voiceLedgerReads is what the ledger of a voice write reads.
type voiceLedgerReads interface {
	ListProjects(ctx context.Context) ([]*platstore.Project, error)
	ContextKey(ctx context.Context, projectID string) (string, error)
	voicewrite.ContextObjectReader
}

// pooledLedgerReads is the ledger reads through the pool, for a write that
// runs in no transaction of its own.
type pooledLedgerReads struct {
	platstore.ContentStore
	platstore.ContextObjectStore
}

// ListProjects resolves the method both embedded stores could offer.
func (r pooledLedgerReads) ListProjects(ctx context.Context) ([]*platstore.Project, error) {
	return r.ContentStore.ListProjects(ctx)
}

var _ voicewrite.Ledger = (*workspaceVoiceLedger)(nil)

// Projects implements voicewrite.Ledger.
func (l *workspaceVoiceLedger) Projects(ctx context.Context) ([]voicewrite.Project, error) {
	if l.wsID == "" {
		return nil, nil
	}
	projects, err := l.reads.ListProjects(ctx)
	if err != nil {
		return nil, err
	}
	var out []voicewrite.Project
	for _, p := range projects {
		if p == nil || p.WorkspaceID != l.wsID {
			continue
		}
		key, err := l.reads.ContextKey(ctx, p.ID)
		if err != nil {
			return nil, err
		}
		if key != "" {
			out = append(out, voicewrite.Project{ID: p.ID, Key: workspace.ProjectKey(key)})
		}
	}
	return out, nil
}

// Find implements voicewrite.Ledger.
func (l *workspaceVoiceLedger) Find(ctx context.Context, p voicewrite.Project, opID string) (voicewrite.HeldOp, bool, error) {
	return voicewrite.FindStored(ctx, l.reads, p, opID)
}

// Put implements voicewrite.Ledger.
func (l *workspaceVoiceLedger) Put(ctx context.Context, projectID string, pushedBy platstore.Pusher, obj platstore.ContextObject) error {
	return l.put(ctx, projectID, pushedBy, obj)
}
