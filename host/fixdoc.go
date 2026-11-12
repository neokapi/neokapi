package host

import (
	"context"
	"errors"
	"os"
	"slices"
	"unicode"
	"unicode/utf8"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/contextop"
)

// DocFix is one fix to apply to a document: the name the caller gives it, the
// operation, and the words the operation replaces as the finding quotes them.
type DocFix struct {
	ID   string
	Op   change.Op
	From string
}

// DocFixRefusal is a fix that did not land, with the reason in one clause and
// the change service's error when it gave one.
type DocFixRefusal struct {
	ID     string
	Reason string
	Error  *change.Error
}

// Message is the refusal as a sentence a person reads: the change service's
// own message when it gave one, otherwise the reason.
func (r DocFixRefusal) Message() string {
	if r.Error != nil && r.Error.Message != "" {
		return r.Error.Message
	}
	if r.Reason == "" {
		return ""
	}
	first, size := utf8.DecodeRuneInString(r.Reason)
	return string(unicode.ToUpper(first)) + r.Reason[size:] + "."
}

// DocFixRequest is a set of fixes to one document and who applies them.
type DocFixRequest struct {
	// Service is the change service the document resolves through.
	Service *change.Service
	// Actor is the person or agent applying the fixes. An agent's edits are
	// noted in the project's context.
	Actor contextop.Actor
	// Recipe is the project's recipe, empty outside a project.
	Recipe string
	// Note rides on the change set.
	Note string
	// File is the document's file on disk, which the fixes may change only in
	// the words they replace.
	File string
	// Fixes are the fixes, in the order they apply.
	Fixes []DocFix
	// DryRun previews the change set and writes nothing.
	DryRun bool
}

// DocFixResult is what ApplyDocFixes did.
type DocFixResult struct {
	// Result is the change set that landed, or was previewed in a dry run.
	// Nil when none did.
	Result *change.Result
	// Fixed names the fixes in Result that landed (or would, in a dry run).
	Fixed []string
	// Refused are the fixes that did not land.
	Refused []DocFixRefusal
}

// ApplyDocFixes applies a chosen set of fixes to one document as one change
// set, the way kapi fix applies a document's fixes. The set is previewed
// first. When the service refuses an operation, that fix is set aside with its
// reason and the rest are previewed again, so one fix that does not fit leaves
// the others to land. A preview whose file differs from the file on disk
// anywhere but in the words the fixes replace is not applied, and every fix is
// refused. Two fixes with the same operation apply once and land together.
func (a *App) ApplyDocFixes(ctx context.Context, req DocFixRequest) (*DocFixResult, error) {
	out := &DocFixResult{}
	ops := groupDocFixes(req.Fixes)
	actor := changeActorOf(req.Actor)
	for len(ops) > 0 {
		set := change.Set{Mode: change.ModePreview, Note: req.Note, Ops: make([]change.Op, len(ops))}
		for i, o := range ops {
			set.Ops[i] = o.op
		}
		original, rerr := os.ReadFile(req.File)
		preview, err := req.Service.Apply(ctx, set, actor)
		if err != nil {
			if refuseAll(ops, err, out) {
				return out, nil
			}
			return nil, err
		}
		if preview.Status != change.SetPreviewed {
			ops = out.dropRefused(ops, preview)
			continue
		}
		if rerr != nil || !keepsOtherBytes(original, ops, preview) {
			for _, o := range ops {
				out.refuse(o, fixReasonOtherBytes, nil)
			}
			return out, nil
		}
		if req.DryRun {
			out.landed(ops, preview)
			return out, nil
		}
		set.Mode = change.ModeApply
		var before map[int]string
		if actor.Kind == change.ActorAgent {
			before = wordingBefore(ctx, req.Service, set)
		}
		res, err := req.Service.Apply(ctx, set, actor)
		if err != nil {
			if refuseAll(ops, err, out) {
				return out, nil
			}
			return nil, err
		}
		switch res.Status {
		case change.SetApplied, change.SetPartial:
			a.noteAgentEdits(ctx, req.Recipe, req.Actor, appliedWording(set, res, before))
			out.landed(ops, res)
			return out, nil
		}
		ops = out.dropRefused(ops, res)
	}
	return out, nil
}

// docFixOp is one operation and the fixes that carry it.
type docFixOp struct {
	op   change.Op
	ids  []string
	from string
}

// groupDocFixes folds fixes with the same operation into one, in the order
// the first of them comes.
func groupDocFixes(fixes []DocFix) []docFixOp {
	var ops []docFixOp
	seen := map[string]int{}
	for _, f := range fixes {
		key := opKey(f.Op)
		if i, dup := seen[key]; dup {
			ops[i].ids = append(ops[i].ids, f.ID)
			continue
		}
		seen[key] = len(ops)
		ops = append(ops, docFixOp{op: f.Op, ids: []string{f.ID}, from: f.From})
	}
	return ops
}

// refuseAll records every fix as refused when err is the change service
// refusing the change set, and reports whether it was.
func refuseAll(ops []docFixOp, err error, out *DocFixResult) bool {
	ce, ok := errors.AsType[*change.Error](err)
	if !ok || ce == nil {
		return false
	}
	for _, o := range ops {
		out.refuse(o, fixRefusalReason(ce), ce)
	}
	return true
}

// refuse records the fixes of an operation that did not land.
func (r *DocFixResult) refuse(o docFixOp, reason string, e *change.Error) {
	for _, id := range o.ids {
		r.Refused = append(r.Refused, DocFixRefusal{ID: id, Reason: reason, Error: e})
	}
}

// dropRefused records the operation that refused a change set and returns the
// others. A change set refused as a whole lands nothing.
func (r *DocFixResult) dropRefused(ops []docFixOp, res *change.Result) []docFixOp {
	i := refusedOp(res)
	if i < 0 || i >= len(ops) {
		for _, o := range ops {
			r.refuse(o, fixRefusalReason(res.Error), res.Error)
		}
		return nil
	}
	var e *change.Error
	if i < len(res.Ops) {
		e = res.Ops[i].Error
	}
	r.refuse(ops[i], fixRefusalReason(e), e)
	return slices.Delete(ops, i, i+1)
}

// landed records the change set that landed and the fixes in it.
func (r *DocFixResult) landed(ops []docFixOp, res *change.Result) {
	r.Result = res
	for i, o := range ops {
		if i < len(res.Ops) && res.Ops[i].Status == change.OpNotApplied {
			continue
		}
		r.Fixed = append(r.Fixed, o.ids...)
	}
}
