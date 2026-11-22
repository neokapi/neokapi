// Package voicewrite writes a workspace voice profile and records the write
// on the context ledger, as one change. Beside the record it writes the
// voice.write operation a checkout applies to its own voice store, so a
// profile written on the server reaches every checkout of the project. Every surface that writes a voice
// profile on the server (a push, the web editor, a context-scan approval)
// goes through Apply. The ledger is the only record of who wrote a profile:
// the voice store holds the profile as it stands and the versions it
// superseded, never their authors.
package voicewrite

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/core/contextop"
	coreprofile "github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/core/projector"
	"github.com/neokapi/neokapi/core/workspace"
)

// Writer is the writer id the server's ledger segments are named under
// (workspace.SegmentName), beside the writer ids of the machines that push.
const Writer = "bowrain"

// Project is one project whose ledger records a write: its id and the
// project key its context is bound to.
type Project struct {
	ID  string
	Key workspace.ProjectKey
}

// Ledger is where a write's operation is recorded.
type Ledger interface {
	// Projects are the projects whose ledgers record the write. A project
	// whose context no push has bound to a key holds no ledger, and is not
	// listed.
	Projects(ctx context.Context) ([]Project, error)
	// Find returns the operation id names in a project's ledger, as it
	// holds it or as the change Apply runs in will add it, and the member
	// who pushed its segment. ok is false when neither holds it.
	Find(ctx context.Context, p Project, opID string) (op HeldOp, ok bool, err error)
	// Put stores one ledger segment of a project, stamped as pushed by
	// pushedBy.
	Put(ctx context.Context, projectID string, pushedBy platstore.Pusher, obj platstore.ContextObject) error
}

// HeldOp is an operation a ledger holds and the member who pushed it.
type HeldOp struct {
	Op       workspace.Op
	PushedBy platstore.Pusher
}

// Write is one write of a voice profile.
type Write struct {
	// Before is the profile as it stood, nil when the write creates it.
	Before *coreprofile.VoiceProfile
	// After is the profile to store, nil when the write removes it. A create
	// names the id the profile is stored under.
	After *coreprofile.VoiceProfile
	// Actor is who wrote it. A person is recorded with no name: a reader
	// names the operation after the member who pushed its segment (PushedBy),
	// as it does an operation the command line records.
	Actor contextop.Actor
	// PushedBy is who the ledger segment is stamped as pushed by: the member,
	// and the tool when a tool token made the write. A reader names an
	// operation in a tool's segment after the tool.
	PushedBy platstore.Pusher
	// Note is what the operation says about where the write came from.
	Note string
	// Recorded is the id of an operation the writer's own client recorded for
	// this write and carries beside it (a push). It stands as the record only
	// when it describes this write (recordedThis); otherwise the write is
	// recorded as PushedBy's.
	Recorded string
}

// Result is what Apply did.
type Result struct {
	// Profile is the profile as the store holds it after the write, nil after
	// a removal.
	Profile *coreprofile.VoiceProfile
	// Op is the id of the operation that records the write: the one Apply
	// appended, or Recorded when a project held it. Empty when no project
	// holds a ledger.
	Op string
}

// Apply writes the profile through voice and records the write on ledger.
// The caller binds voice and ledger to one transaction where the stores share
// a database, so the profile and its record land together or not at all. A
// nil ledger records nothing.
func Apply(ctx context.Context, voice coreprofile.Store, ledger Ledger, w Write) (Result, error) {
	var res Result
	switch {
	case w.Before == nil && w.After == nil:
		return res, errors.New("voicewrite: no profile to write")
	case w.Before == nil:
		if err := voice.CreateProfile(ctx, w.After); err != nil {
			return res, fmt.Errorf("create voice profile %q: %w", w.After.Name, err)
		}
	case w.After == nil:
		if err := voice.DeleteProfile(ctx, w.Before.ID); err != nil {
			return res, fmt.Errorf("delete voice profile %q: %w", w.Before.Name, err)
		}
	default:
		if err := voice.UpdateProfile(ctx, w.After); err != nil {
			return res, fmt.Errorf("update voice profile %q: %w", w.After.Name, err)
		}
	}
	var stored *coreprofile.VoiceProfile
	if w.After != nil {
		var err error
		if stored, err = voice.GetProfile(ctx, w.After.ID); err != nil {
			return res, fmt.Errorf("read voice profile %q back: %w", w.After.Name, err)
		}
	}
	res.Profile = stored
	if ledger == nil {
		return res, nil
	}

	projects, err := ledger.Projects(ctx)
	if err != nil {
		return res, fmt.Errorf("list the ledgers to record the voice profile write in: %w", err)
	}
	var record []Project
	for _, p := range projects {
		if w.Recorded != "" {
			held, ok, err := ledger.Find(ctx, p, w.Recorded)
			if err != nil {
				return res, err
			}
			if ok && recordedThis(held, w.PushedBy, stored) {
				res.Op = w.Recorded
				continue
			}
		}
		record = append(record, p)
	}
	if len(record) == 0 {
		return res, nil
	}

	edit, err := contextop.NewVoiceEdit(w.Before, stored)
	if err != nil {
		return res, err
	}
	what := "edited"
	switch {
	case w.Before == nil:
		what = "created"
	case stored == nil:
		what = "removed"
	}
	text := "voice profile " + edit.Profile + " " + what
	if phrase := edit.Phrase(); phrase != "" {
		text += ": " + phrase
	}
	actor := w.Actor
	if actor.Kind == "" || actor.Kind == contextop.ActorPerson {
		actor = contextop.Actor{Kind: contextop.ActorPerson, Session: actor.Session}
	}
	put, remove := storeWrite(w.Before, stored)
	log := &segmentLog{ledger: ledger, projects: record, pushedBy: w.PushedBy, put: put, remove: remove}
	appended, err := contextop.NewLedger(log, writerDecided).Append(ctx, contextop.Record{
		Actor:   actor,
		Kind:    contextop.KindEdit,
		Subject: contextop.Subject{Kind: contextop.SubjectNote, Text: text},
		Note:    w.Note,
		Voice:   edit,
	})
	if err != nil {
		return res, fmt.Errorf("record the voice profile write: %w", err)
	}
	res.Op = appended.ID
	return res, nil
}

// storeWrite is what a checkout's voice store is to put and remove for a
// write that turned before into stored: the profile as written, stored under
// the id a checkout spells from its name, and the profile under its former
// name when the write removed or renamed it.
func storeWrite(before, stored *coreprofile.VoiceProfile) (put []*coreprofile.VoiceProfile, remove []string) {
	if stored != nil {
		prof := coreprofile.Authored(stored)
		prof.ID = ProfileSlug(stored.Name)
		prof.Version = 0
		put = append(put, prof)
	}
	if before != nil {
		if id := ProfileSlug(before.Name); stored == nil || id != ProfileSlug(stored.Name) {
			remove = append(remove, id)
		}
	}
	return put, remove
}

// recordedThis reports whether a held operation is the record of the write
// that left stored: a voice edit of the same profile whose fingerprint is the
// stored profile's, pushed by the member, and the tool, making this write. Naming any other
// operation would credit the write to that operation's actor, so a client
// cannot borrow another member's or an agent's record.
func recordedThis(held HeldOp, pushedBy platstore.Pusher, stored *coreprofile.VoiceProfile) bool {
	if stored == nil || pushedBy.UserID == "" || held.PushedBy != pushedBy ||
		!strings.HasPrefix(held.Op.Kind, contextop.OpKindPrefix+string(contextop.KindEdit)) {
		return false
	}
	var body struct {
		Voice *contextop.VoiceEdit `json:"voice"`
	}
	if json.Unmarshal(held.Op.Payload, &body) != nil || body.Voice == nil {
		return false
	}
	edit := body.Voice
	if !SameProfile(edit.Profile, stored.ID, stored.Name) || edit.Wrote == "" {
		return false
	}
	// A checkout stores the profile under its own id, which the fingerprint
	// covers; the server's copy is compared under that id.
	local := *stored
	local.ID = edit.Profile
	wrote, err := contextop.VoiceFingerprint(&local)
	return err == nil && wrote == edit.Wrote
}

// SameProfile reports whether a profile an operation names is the stored
// profile with id and name: the same id, or the name a checkout spells its
// profile id from.
func SameProfile(named, id, name string) bool {
	return named != "" && (named == id || ProfileSlug(named) == ProfileSlug(name))
}

// ProfileSlug spells a profile name the way a checkout's profile id does
// (coreprofile.Slug).
func ProfileSlug(name string) string { return coreprofile.Slug(name) }

// writerDecided admits every operation Apply appends. The surface that calls
// Apply has already decided the write is allowed (a member's permission, a
// push the commit handler accepted), and the operation records that write
// rather than asking for it.
func writerDecided(contextop.Transition) error { return nil }

// segmentLog is the log Apply appends through: each operation is written as
// a segment of its own to every project it records the write in, under one
// id. The same segment then holds the voice.write operation that puts the
// profiles of put into a checkout's voice store and removes those of remove.
// It reads nothing back, as an edit names no operation to resolve.
type segmentLog struct {
	ledger   Ledger
	projects []Project
	pushedBy platstore.Pusher
	put      []*coreprofile.VoiceProfile
	remove   []string
}

var _ contextop.Log = (*segmentLog)(nil)

// Select implements contextop.Log.
func (l *segmentLog) Select(context.Context, workspace.OpQuery) ([]workspace.Op, error) {
	return nil, nil
}

// Record implements contextop.Log.
func (l *segmentLog) Record(ctx context.Context, ops ...workspace.Op) ([]workspace.Op, error) {
	out := make([]workspace.Op, len(ops))
	for i, op := range ops {
		op.At = op.At.UTC()
		if op.ID == "" {
			op.ID = workspace.NewOpID(op.At, "")
		}
		storeID := workspace.NewOpID(op.At, op.ID)
		for _, p := range l.projects {
			op.Project = p.Key
			segment := []workspace.Op{op}
			if len(l.put) > 0 || len(l.remove) > 0 {
				write, err := projector.VoiceWrite(p.Key, op.At, projector.Origin{By: Writer, Cause: op.ID}, l.put, l.remove)
				if err != nil {
					return nil, err
				}
				write.ID = storeID
				segment = append(segment, write)
			}
			data, err := workspace.EncodeSegment(segment)
			if err != nil {
				return nil, err
			}
			obj := platstore.ContextObject{Name: workspace.SegmentName(Writer, op.ID), Data: data}
			if err := l.ledger.Put(ctx, p.ID, l.pushedBy, obj); err != nil {
				return nil, fmt.Errorf("record the voice profile write in project %s: %w", p.ID, err)
			}
		}
		out[i] = op
	}
	return out, nil
}

// FindIn returns the operation id names among the ledger segments of
// objects, read as operations of key, with the member who pushed its segment.
func FindIn(objects []platstore.ContextObject, key workspace.ProjectKey, opID string) (HeldOp, bool) {
	for _, obj := range objects {
		if !strings.HasPrefix(obj.Name, workspace.RemoteLogDir) {
			continue
		}
		ops, err := workspace.DecodeSegment(obj.Data, key)
		if err != nil {
			continue
		}
		for _, op := range ops {
			if op.ID == opID {
				return HeldOp{Op: op, PushedBy: obj.PushedBy}, true
			}
		}
	}
	return HeldOp{}, false
}

// FindStored returns the operation id names in the project's stored ledger,
// with the member who pushed its segment.
func FindStored(ctx context.Context, objects platstore.ContextObjectStore, p Project, opID string) (HeldOp, bool, error) {
	names, err := objects.ListContextObjects(ctx, p.ID, workspace.RemoteLogDir)
	if err != nil {
		return HeldOp{}, false, err
	}
	for _, name := range names {
		obj, err := objects.GetContextObject(ctx, p.ID, name)
		if err != nil {
			return HeldOp{}, false, err
		}
		if held, ok := FindIn([]platstore.ContextObject{*obj}, p.Key, opID); ok {
			return held, true, nil
		}
	}
	return HeldOp{}, false, nil
}
