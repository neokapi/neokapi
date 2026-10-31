package server

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/core/contextop"
	"github.com/neokapi/neokapi/core/workspace"
)

// contextLedgerWriter is the writer id the server's segments are named under
// (workspace.SegmentName), beside the writer ids of the machines that push.
const contextLedgerWriter = "bowrain"

// ErrNoContextKey reports a write to the context ledger of a project whose
// context no push has bound to a project key yet.
var ErrNoContextKey = errors.New("the project's context has no project key yet: push it from kapi first")

// ContextLedger returns the context ledger of a project, read from its
// context objects and written as member. Every operation the ledger records
// carries the member as its actor, a person under the member's display name,
// whatever actor the caller filled in. A governing operation (a keep, a drop,
// an edit) is refused with ErrOutsideReach unless the member holds
// PermManageTerms at every point it governs.
func (s *Server) ContextLedger(ctx context.Context, projectID string, member LedgerMember) (*contextop.Ledger, error) {
	objects, ok := s.contextObjects()
	if !ok {
		return nil, errors.New("context objects not configured")
	}
	log := &contextLog{
		objects: objects,
		cache:   s.contextLogs.project(projectID),
		project: projectID,
		actor:   contextop.Actor{Kind: contextop.ActorPerson, Name: s.displayName(ctx, member.UserID)},
		member:  member,
		nameOf:  s.displayName,
	}
	return contextop.NewLedger(log, contextop.PersonDecides), nil
}

// ContextSettler returns the context ledger a project is settled through
// (contextop.Ledger.Settle). Every operation it records carries
// contextop.SettleActor, the tool settling acts as, which the server sets
// itself; no caller names it. Settling follows from the log, so the settler
// governs every point of the project.
func (s *Server) ContextSettler(projectID string) (*contextop.Ledger, error) {
	objects, ok := s.contextObjects()
	if !ok {
		return nil, errors.New("context objects not configured")
	}
	log := &contextLog{
		objects: objects,
		cache:   s.contextLogs.project(projectID),
		project: projectID,
		actor:   contextop.SettleActor,
		member:  LedgerMember{UserID: contextop.SettleActor.Name, Permissions: governingPermissions},
		nameOf:  s.displayName,
	}
	return contextop.NewLedger(log, contextop.PersonDecides), nil
}

// contextLog is a project's operation log (contextop.Log) over its context
// objects: the segments under log/ that the machines sharing the project
// pushed and the server wrote.
type contextLog struct {
	objects platstore.ContextObjectStore
	cache   *projectContextLog
	project string
	// actor is stamped on every context operation Record writes.
	actor contextop.Actor
	// member is who writes: the segments Record writes are stamped with the
	// member's id, and a governing operation is checked against the member's
	// permissions and reach.
	member LedgerMember
	// nameOf is the name a member is shown by. A context operation by a
	// person with no name is read as by the member who pushed its segment,
	// under this name.
	nameOf func(ctx context.Context, userID string) string
}

var _ contextop.Log = (*contextLog)(nil)

// Select implements contextop.Log. It returns the operations of every
// segment under log/ that decodes as operations of the project's key, merged
// in id order and numbered from 1 in that order (Op.Seq), less those the
// query leaves out.
//
// A context operation whose actor is a person with no name, as the command
// line records one and a sync pushes it, is returned under the name of the
// member who pushed its segment. The log itself holds the operation as it was
// pushed, so a later push of the same operation still matches it.
func (l *contextLog) Select(ctx context.Context, q workspace.OpQuery) ([]workspace.Op, error) {
	_, ops, pushers, err := l.cache.readPushers(ctx, l.objects, l.project)
	if err != nil {
		return nil, err
	}
	names := map[string]string{}
	nameOf := func(userID string) string {
		name, ok := names[userID]
		if !ok {
			name = l.nameOf(ctx, userID)
			names[userID] = name
		}
		return name
	}
	out := []workspace.Op{}
	for _, op := range ops {
		if op.Seq <= q.After ||
			(q.KindPrefix != "" && !strings.HasPrefix(op.Kind, q.KindPrefix)) ||
			(q.Project != "" && op.Project != q.Project) ||
			(q.Subject != "" && op.Subject != q.Subject) {
			continue
		}
		if pusher := pushers[op.ID]; pusher != "" && l.nameOf != nil && strings.HasPrefix(op.Kind, contextop.OpKindPrefix) {
			op.Payload = nameUnnamedPerson(op.Payload, func() string { return nameOf(pusher) })
		}
		out = append(out, op)
		if q.Limit > 0 && len(out) == q.Limit {
			break
		}
	}
	return out, nil
}

// Record implements contextop.Log. The operations the log does not hold
// already, by id or by content address, are written as one new segment,
// log/bowrain/<first-op-id>.jsonl, under the project's key; an operation it
// holds is returned as held. Each context operation is stamped with the log's
// actor.
func (l *contextLog) Record(ctx context.Context, ops ...workspace.Op) ([]workspace.Op, error) {
	key, held, err := l.cache.read(ctx, l.objects, l.project)
	if err != nil {
		return nil, err
	}
	if key == "" {
		return nil, ErrNoContextKey
	}
	byID := make(map[string]workspace.Op, len(held))
	byAddress := map[string]workspace.Op{}
	after := ""
	for _, op := range held {
		byID[op.ID] = op
		if op.Address != "" {
			byAddress[op.Address] = op
		}
		after = max(after, op.ID)
	}
	now := time.Now().UTC()
	out := make([]workspace.Op, len(ops))
	var fresh []workspace.Op
	for i, op := range ops {
		if op.Kind == "" {
			return nil, errors.New("record a context operation: the operation has no kind")
		}
		switch op.Project {
		case "":
			op.Project = key
		case key:
		default:
			return nil, fmt.Errorf("record a context operation: an operation of project %q, not %q", op.Project, key)
		}
		if h, ok := byID[op.ID]; ok && op.ID != "" {
			out[i] = h
			continue
		}
		if h, ok := byAddress[op.Address]; ok && op.Address != "" {
			out[i] = h
			continue
		}
		if op.ID != "" && !workspace.ValidOpID(op.ID) {
			return nil, fmt.Errorf("record a context operation: %q is not an operation id", op.ID)
		}
		if strings.HasPrefix(op.Kind, contextop.OpKindPrefix) {
			if err := checkReach(ctx, l.member, op, byID); err != nil {
				return nil, err
			}
			if op.Payload, err = stampActor(op.Payload, l.actor); err != nil {
				return nil, err
			}
		}
		if op.At.IsZero() {
			op.At = now
		}
		op.At = op.At.UTC()
		if op.ID == "" {
			op.ID = workspace.NewOpID(op.At, after)
		}
		after = max(after, op.ID)
		op.Seq = 0
		byID[op.ID] = op
		if op.Address != "" {
			byAddress[op.Address] = op
		}
		fresh = append(fresh, op)
		out[i] = op
	}
	if len(fresh) == 0 {
		return out, nil
	}
	slices.SortFunc(fresh, func(a, b workspace.Op) int { return strings.Compare(a.ID, b.ID) })
	data, err := workspace.EncodeSegment(fresh)
	if err != nil {
		return nil, err
	}
	name := workspace.SegmentName(contextLedgerWriter, fresh[0].ID)
	if err := l.objects.PutContextObjects(ctx, l.project, l.member.UserID, []platstore.ContextObject{{Name: name, Data: data}}); err != nil {
		return nil, fmt.Errorf("record context operations: %w", err)
	}
	return out, nil
}

// stampActor sets the actor of a context operation to the log's actor,
// whatever actor the payload names.
func stampActor(payload []byte, actor contextop.Actor) ([]byte, error) {
	body := map[string]json.RawMessage{}
	if len(payload) > 0 {
		if err := json.Unmarshal(payload, &body); err != nil {
			return nil, fmt.Errorf("record a context operation: the payload is not a JSON object: %w", err)
		}
	}

	stamped, err := json.Marshal(actor)
	if err != nil {
		return nil, err
	}
	body["actor"] = stamped
	return json.Marshal(body)
}

// nameUnnamedPerson returns a context operation's payload with name as its
// actor's name when the actor is a person with no name, and the payload as
// it is otherwise, or when it does not read as a JSON object.
func nameUnnamedPerson(payload []byte, name func() string) []byte {
	body := map[string]json.RawMessage{}
	if len(payload) == 0 || json.Unmarshal(payload, &body) != nil {
		return payload
	}
	var actor contextop.Actor
	if raw, ok := body["actor"]; ok && json.Unmarshal(raw, &actor) != nil {
		return payload
	}
	if actor.Name != "" || (actor.Kind != "" && actor.Kind != contextop.ActorPerson) {
		return payload
	}
	actor.Kind, actor.Name = contextop.ActorPerson, name()
	if actor.Name == "" {
		return payload
	}
	stamped, err := json.Marshal(actor)
	if err != nil {
		return payload
	}
	body["actor"] = stamped
	out, err := json.Marshal(body)
	if err != nil {
		return payload
	}
	return out
}

// contextLogCache holds each project's decoded log between requests. The
// zero value is ready to use.
type contextLogCache struct {
	mu       sync.Mutex
	projects map[string]*projectContextLog
}

// project returns the cache entry of one project, creating it on first use.
func (c *contextLogCache) project(projectID string) *projectContextLog {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.projects == nil {
		c.projects = map[string]*projectContextLog{}
	}
	p, ok := c.projects[projectID]
	if !ok {
		p = &projectContextLog{}
		c.projects[projectID] = p
	}
	return p
}

// projectContextLog is one project's log as last read, keyed by the project
// key and by the names and digests of the segments it was read from. A
// segment is read once: an object is never replaced, so a later reading
// fetches only the segments new since the last.
type projectContextLog struct {
	mu       sync.Mutex
	key      workspace.ProjectKey
	list     string
	segments map[string]readSegment
	ops      []workspace.Op
	// pushers names the member who pushed the segment each operation was
	// read from, by operation id.
	pushers map[string]string
}

// readSegment is one segment as it was decoded, and the member who pushed it.
type readSegment struct {
	digest   string
	ops      []workspace.Op
	pushedBy string
}

// read returns the project's key and its log, merged in id order and numbered
// from 1. The slice is the caller's own.
func (p *projectContextLog) read(ctx context.Context, objects platstore.ContextObjectStore, projectID string) (workspace.ProjectKey, []workspace.Op, error) {
	key, ops, _, err := p.readPushers(ctx, objects, projectID)
	return key, ops, err
}

// readPushers is read, with the member who pushed the segment each operation
// was read from, by operation id. The map is shared and read-only.
func (p *projectContextLog) readPushers(ctx context.Context, objects platstore.ContextObjectStore, projectID string) (workspace.ProjectKey, []workspace.Op, map[string]string, error) {
	raw, err := objects.ContextKey(ctx, projectID)
	if err != nil {
		return "", nil, nil, err
	}
	key := workspace.ProjectKey(raw)
	digests, err := objects.DigestContextObjects(ctx, projectID, workspace.RemoteLogDir)
	if err != nil {
		return "", nil, nil, err
	}
	list := listDigest(key, digests)

	p.mu.Lock()
	defer p.mu.Unlock()
	if p.ops != nil && p.list == list {
		return key, slices.Clone(p.ops), p.pushers, nil
	}
	if p.key != key {
		p.segments = nil
	}
	segments := make(map[string]readSegment, len(digests))
	if key != "" {
		for name, digest := range digests {
			if seg, ok := p.segments[name]; ok && seg.digest == digest {
				segments[name] = seg
				continue
			}
			obj, err := objects.GetContextObject(ctx, projectID, name)
			if err != nil {
				return "", nil, nil, err
			}
			ops, err := workspace.DecodeSegment(obj.Data, key)
			if err != nil {
				return "", nil, nil, fmt.Errorf("context ledger: segment %s: %w", name, err)
			}
			segments[name] = readSegment{digest: digest, ops: ops, pushedBy: obj.PushedBy}
		}
	}
	p.key, p.list, p.segments, p.ops = key, list, segments, mergeSegments(segments)
	p.pushers = segmentPushers(segments)
	return key, slices.Clone(p.ops), p.pushers, nil
}

// segmentPushers names the member who pushed each operation's segment, by
// operation id. Where two segments hold one id, the first in name order
// names it, as mergeSegments keeps that segment's operation.
func segmentPushers(segments map[string]readSegment) map[string]string {
	names := make([]string, 0, len(segments))
	for name := range segments {
		names = append(names, name)
	}
	slices.Sort(names)
	out := map[string]string{}
	for _, name := range names {
		seg := segments[name]
		for _, op := range seg.ops {
			if _, ok := out[op.ID]; !ok && seg.pushedBy != "" {
				out[op.ID] = seg.pushedBy
			}
		}
	}
	return out
}

// mergeSegments merges the operations of every segment in id order, holding
// one operation per id and per content address, and numbers them from 1. The
// segments are read in name order, so where two hold one id the operation
// kept is the same on every reading.
func mergeSegments(segments map[string]readSegment) []workspace.Op {
	names := make([]string, 0, len(segments))
	for name := range segments {
		names = append(names, name)
	}
	slices.Sort(names)
	all := []workspace.Op{}
	for _, name := range names {
		all = append(all, segments[name].ops...)
	}
	slices.SortStableFunc(all, func(a, b workspace.Op) int { return strings.Compare(a.ID, b.ID) })
	ids := map[string]bool{}
	addresses := map[string]bool{}
	out := all[:0]
	for _, op := range all {
		if ids[op.ID] || (op.Address != "" && addresses[op.Address]) {
			continue
		}
		ids[op.ID] = true
		if op.Address != "" {
			addresses[op.Address] = true
		}
		op.Seq = int64(len(out) + 1)
		out = append(out, op)
	}
	return out
}

// listDigest names a project key and a list of segments with their digests.
func listDigest(key workspace.ProjectKey, digests map[string]string) string {
	names := make([]string, 0, len(digests))
	for name := range digests {
		names = append(names, name)
	}
	slices.Sort(names)
	h := sha256.New()
	h.Write([]byte(key))
	for _, name := range names {
		h.Write([]byte{0})
		h.Write([]byte(name))
		h.Write([]byte{0})
		h.Write([]byte(digests[name]))
	}
	return hex.EncodeToString(h.Sum(nil))
}

// segmentKeys returns the project keys a segment's operations name, in the
// order they first appear. Bytes that are not operations name none.
func segmentKeys(data []byte) []workspace.ProjectKey {
	var keys []workspace.ProjectKey
	for raw := range bytes.SplitSeq(data, []byte("\n")) {
		if len(bytes.TrimSpace(raw)) == 0 {
			continue
		}
		var line struct {
			ID      string `json:"id"`
			Project string `json:"project"`
		}
		if json.Unmarshal(raw, &line) != nil || !workspace.ValidOpID(line.ID) || line.Project == "" {
			continue
		}
		if key := workspace.ProjectKey(line.Project); !slices.Contains(keys, key) {
			keys = append(keys, key)
		}
	}
	return keys
}
