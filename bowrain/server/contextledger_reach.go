package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	"github.com/neokapi/neokapi/core/contextop"
	"github.com/neokapi/neokapi/core/workspace"
)

// LedgerMember is who writes to a project's context ledger: the member, the
// permissions the request resolved for them, and the coordinates their grants
// reach.
type LedgerMember struct {
	UserID      string
	Permissions platauth.Permission
	Reach       platauth.CoordinateReach
}

// ledgerMemberOf reads the signed-in member of a request.
func ledgerMemberOf(c echo.Context) LedgerMember {
	userID, _ := c.Get("user_id").(string)
	perms, _ := c.Get("project_permissions").(platauth.Permission)
	return LedgerMember{UserID: userID, Permissions: perms, Reach: callerReach(c)}
}

// ErrOutsideReach reports a context operation that governs a point the
// member's grants do not reach, or that the member's permissions do not
// cover. The error is a *ReachError.
var ErrOutsideReach = errors.New("the operation governs a point outside the coordinates you hold")

// ReachError names the operation refused and why.
type ReachError struct {
	Kind  contextop.Kind
	ID    string
	Point platauth.CoordinateFilter
	Reach platauth.CoordinateReach
	// Permission is set when the member lacks the permission the kind needs,
	// rather than the reach.
	Permission platauth.Permission
	// Target is set when the operation acts on one the log does not hold, so
	// the point it governs cannot be read.
	Target string
}

func (e *ReachError) Error() string {
	if e.Permission != 0 {
		return fmt.Sprintf("context operation %s (%s) needs the %s permission", workspace.ShortOpID(e.ID), e.Kind, e.Permission)
	}
	if e.Target != "" {
		return fmt.Sprintf("context operation %s (%s) acts on operation %s, which the project's context does not hold",
			workspace.ShortOpID(e.ID), e.Kind, workspace.ShortOpID(e.Target))
	}
	return fmt.Sprintf("context operation %s (%s) governs %s, outside the coordinates you hold (%s)",
		workspace.ShortOpID(e.ID), e.Kind, describePoint(e.Point), describeReach(e.Reach))
}

// Is reports ErrOutsideReach.
func (e *ReachError) Is(target error) bool { return target == ErrOutsideReach }

// describePoint renders a point as the scope grammar writes it.
func describePoint(p platauth.CoordinateFilter) string {
	if len(p) == 0 {
		return "the whole project"
	}
	return p.String()
}

// describeReach renders a reach as the scope grammar writes each filter.
func describeReach(r platauth.CoordinateReach) string {
	parts := make([]string, 0, len(r))
	for _, f := range r {
		parts = append(parts, f.String())
	}
	slices.Sort(parts)
	return strings.Join(parts, "; ")
}

// governingKinds are the context operations that establish, change or set
// aside what governs content: what a person decides. The other kinds say what
// somebody saw (observe, correct, signal, check) or take back one's own
// suggestion (withdraw), and govern nothing.
var governingKinds = []contextop.Kind{
	contextop.KindKeep, contextop.KindDrop, contextop.KindImport, contextop.KindEdit,
	contextop.KindWiden, contextop.KindReset, contextop.KindEstablish,
}

// governingPermission is the permission a governing operation needs, the one
// the server's terms routes gate on.
const governingPermission = platauth.PermManageTerms

// checkReach refuses a context operation that governs a point outside the
// member's reach. An operation that acts on another (a keep, a drop) governs
// the point of the operation it names, looked up in byID, and its own
// coordinates when it carries any; a widening governs the point it widens to
// even when that is the whole project. Any other operation governs its own
// coordinates, and one with none governs the whole project, which only an
// unbounded reach covers.
func checkReach(ctx context.Context, member LedgerMember, op workspace.Op, byID map[string]workspace.Op) error {
	kind, ok := strings.CutPrefix(op.Kind, contextop.OpKindPrefix)
	if !ok || !slices.Contains(governingKinds, contextop.Kind(kind)) {
		return nil
	}
	if !member.Permissions.Has(governingPermission) {
		return &ReachError{Kind: contextop.Kind(kind), ID: op.ID, Permission: governingPermission}
	}
	if contextop.Kind(kind) == contextop.KindEstablish && followsFromLog(ctx, op, byID) {
		return nil
	}
	own, target := scopeOf(op)
	var points []map[string]string
	if target == "" || len(own) > 0 || contextop.Kind(kind) == contextop.KindWiden {
		points = append(points, own)
	}
	if target != "" {
		t, ok := byID[target]
		if !ok {
			// An operation the log does not hold has no point to read, so
			// what acts on it is refused rather than let through.
			return &ReachError{Kind: contextop.Kind(kind), ID: op.ID, Reach: member.Reach, Target: target}
		}
		at, _ := scopeOf(t)
		points = append(points, at)
	}
	if len(points) == 0 {
		points = append(points, nil)
	}
	if member.Reach.Unconstrained() {
		return nil
	}
	for _, point := range points {
		if !member.Reach.Reaches(point) {
			return &ReachError{Kind: contextop.Kind(kind), ID: op.ID, Point: point, Reach: member.Reach}
		}
	}
	return nil
}

// scopeOf reads the coordinates and the target a context operation's payload
// carries.
func scopeOf(op workspace.Op) (coordinates map[string]string, target string) {
	var body struct {
		Scope struct {
			Coordinates map[string]string `json:"coordinates"`
		} `json:"scope"`
		Target string `json:"target"`
	}
	if len(op.Payload) > 0 {
		_ = json.Unmarshal(op.Payload, &body)
	}
	return body.Scope.Coordinates, body.Target
}

// followsFromLog reports whether an establish operation is one settling
// records over the operations byID holds. Settling (contextop.Ledger.Settle)
// runs over those operations, less op itself, under the policy in force, and
// op is accepted only when settling records an establishment with its content
// address and its payload byte for byte: target, evidence, scope and actor.
// Settling follows from the log rather than from anyone's reach, so such an
// establishment is accepted at any point. One that settling would not record,
// whatever address it claims, is checked as any decision is.
func followsFromLog(ctx context.Context, op workspace.Op, byID map[string]workspace.Op) bool {
	log := &settleLog{}
	for id, held := range byID {
		if id != op.ID {
			log.ops = append(log.ops, held)
		}
	}
	slices.SortFunc(log.ops, func(a, b workspace.Op) int { return strings.Compare(a.ID, b.ID) })
	for i := range log.ops {
		log.ops[i].Seq = int64(i + 1)
	}
	if _, err := contextop.NewLedger(log, contextop.PersonDecides).Settle(ctx, contextop.Filter{}); err != nil {
		return false
	}
	for _, rec := range log.recorded {
		if rec.Address == op.Address && compactJSON(rec.Payload) == compactJSON(op.Payload) {
			return true
		}
	}
	return false
}

// settleLog is a log settling reads and records into without writing
// anything: what it records is kept apart, for followsFromLog to compare.
type settleLog struct {
	ops      []workspace.Op
	recorded []workspace.Op
}

func (l *settleLog) Select(_ context.Context, q workspace.OpQuery) ([]workspace.Op, error) {
	out := []workspace.Op{}
	for _, op := range l.ops {
		if op.Seq <= q.After ||
			(q.KindPrefix != "" && !strings.HasPrefix(op.Kind, q.KindPrefix)) ||
			(q.Project != "" && op.Project != q.Project) ||
			(q.Subject != "" && op.Subject != q.Subject) {
			continue
		}
		out = append(out, op)
	}
	return out, nil
}

func (l *settleLog) Record(_ context.Context, ops ...workspace.Op) ([]workspace.Op, error) {
	out := make([]workspace.Op, len(ops))
	for i, op := range ops {
		if op.ID == "" {
			op.ID = workspace.NewOpID(time.Now().UTC(), "")
		}
		l.recorded = append(l.recorded, op)
		out[i] = op
	}
	return out, nil
}
