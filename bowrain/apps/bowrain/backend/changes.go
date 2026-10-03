package backend

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/model"
	apiclient "github.com/neokapi/neokapi/host/venue/client"
)

// editorStream is the stream the desktop editor works on: the whole desktop
// editor surface is pinned to the project's main stream.
const editorStream = "main"

// ApplyChanges applies a change set (kapi.change/v1, as JSON) to a project's
// main stream and returns the result (kapi.change-result/v1, as JSON). It is how
// the desktop editor saves a translation, decides on one and annotates a block,
// each operation naming the revision of the edition the editor showed.
//
// Connected, the server applies the change set and judges every precondition,
// and a change set it applied is applied to the local cache too. When the
// server is out of reach the change set applies to the cache and queues with
// its preconditions (changeSetOp), and the server judges them when the
// connection returns. In local mode, with no server, the cache is the project
// and judges them itself.
//
// The result carries refusals too: a stale refusal names the edition as it now
// stands, which the editor shows before it asks whether to apply the edit
// again. An error means the change set got no result.
func (a *App) ApplyChanges(projectID, set string) (string, error) {
	parsed, err := change.Decode(strings.NewReader(set))
	if err != nil {
		ce, ok := errors.AsType[*change.Error](err)
		if !ok {
			ce = &change.Error{Code: change.CodeInvalid, Message: err.Error()}
		}
		return marshalResult(change.ErrorResult(ce))
	}
	ctx := context.Background()

	if a.isConnected() {
		client, ws := a.editorRemote()
		res, err := client.ApplyChanges(ctx, ws, projectID, editorStream, []byte(set))
		switch {
		case err == nil:
			if res.Status == change.SetApplied || res.Status == change.SetPartial {
				// The server has it; the cache follows so an offline reader sees
				// it. The server judged the preconditions, so the cache does not.
				_ = a.applyCached(ctx, projectID, parsed, cacheFollows)
			}
			return marshalResult(res)
		case permanentRejection(err):
			return "", err
		}
		a.goOffline()
	}

	if a.isOffline() {
		res := a.applyCached(ctx, projectID, parsed, cacheStandsIn)
		if res.Status == change.SetApplied {
			a.enqueue(changeSetOp{ProjectID: projectID, Stream: editorStream, Set: json.RawMessage(set)})
		}
		return marshalResult(res)
	}
	return marshalResult(a.applyCached(ctx, projectID, parsed, cacheIsTheProject))
}

func marshalResult(res *change.Result) (string, error) {
	out, err := json.Marshal(res)
	if err != nil {
		return "", fmt.Errorf("encode the change result: %w", err)
	}
	return string(out), nil
}

// cacheRole says what the local cache is to a change set applied to it.
type cacheRole int

const (
	// cacheIsTheProject: local mode, with no server. The cache judges every
	// precondition and holds every annotation.
	cacheIsTheProject cacheRole = iota
	// cacheStandsIn: the server is out of reach, and the change set queues for
	// it. The server judges the preconditions on replay against the content as
	// it then stands, so the cache applies the change to whatever it holds: a
	// cache that lost a detail of a translation must not refuse an edit the
	// server would take. Annotations wait for the server, which keeps them.
	cacheStandsIn
	// cacheFollows: the server applied the change set, and the cache catches
	// up with it as cacheStandsIn does.
	cacheFollows
)

// applyCached applies a change set to the project's blocks in the local cache,
// all or nothing. Blocks are addressed by the id the editor payload carries.
// Content operations apply through change.ApplyBlock; a decision moves the
// translation's status the way the server moves it (decideCached). While the
// cache stands in for the server, a block it does not hold is left to the
// server.
func (a *App) applyCached(ctx context.Context, projectID string, set change.Set, role cacheRole) *change.Result {
	res := &change.Result{Schema: change.ResultSchemaID, Status: change.SetApplied, Docs: []change.DocResult{}, Ops: make([]change.OpResult, len(set.Ops))}
	preview := set.Mode == change.ModePreview

	// The operations of each block, in the order the change set names them.
	order := []string{}
	byBlock := map[string][]int{}
	for i, op := range set.Ops {
		at := op.At
		res.Ops[i] = change.OpResult{I: i, Op: op.Kind, At: &at}
		if _, ok := byBlock[op.At.Block]; !ok {
			order = append(order, op.At.Block)
		}
		byBlock[op.At.Block] = append(byBlock[op.At.Block], i)
	}

	refused := -1
	refuse := func(i int, e *change.Error) {
		res.Ops[i].Status = change.OpRefused
		res.Ops[i].Error = e
		if refused < 0 || i < refused {
			refused = i
		}
	}
	var changed []*model.Block
	for _, blockID := range order {
		idx := byBlock[blockID]
		sb, err := a.store.GetBlock(ctx, projectID, editorStream, blockID)
		if err != nil || sb == nil || sb.Block == nil {
			for _, i := range idx {
				if role != cacheIsTheProject {
					// The cache never held the block (a review queue entry is
					// read without caching it); the server judges the change.
					res.Ops[i].Status = change.OpApplied
					continue
				}
				refuse(i, &change.Error{Code: change.CodeNotFound, Field: "at/block",
					Message: fmt.Sprintf("the project holds no block %q", blockID)})
			}
			continue
		}
		b := sb.Block

		var content []change.Op
		var contentIdx, decisions []int
		for _, i := range idx {
			op := set.Ops[i]
			switch op.Kind {
			case change.KindDecide:
				decisions = append(decisions, i)
			case change.KindAnnotate, change.KindUnannotate:
				if role != cacheIsTheProject {
					res.Ops[i].Status = change.OpApplied
					continue
				}
				content, contentIdx = append(content, op), append(contentIdx, i)
			case change.KindSetContent, change.KindReplaceText, change.KindSetAttribute, change.KindMark, change.KindRemoveEdition:
				if role != cacheIsTheProject {
					op.IfMatch = change.AnyRevision
				}
				content, contentIdx = append(content, op), append(contentIdx, i)
			default:
				refuse(i, &change.Error{Code: change.CodeUnsupported, Capability: string(op.Kind),
					Message: fmt.Sprintf("the desktop editor sends no %s operation", op.Kind)})
			}
		}

		// Every if_match names the edition as it stood when the change set
		// began, so a decision is judged against the revision its edition
		// held before this set's content operations applied.
		began := make(map[int]change.Current, len(decisions))
		for _, i := range decisions {
			at := set.Ops[i].At.Edition
			began[i] = change.Current{Rev: model.EditionRevision(b, at), Text: b.TargetText(at.Locale)}
		}

		blockChanged := false
		if len(content) > 0 {
			for j, r := range change.ApplyBlock(b, content, change.BlockEnv{
				Actor:   change.Actor{Kind: change.ActorPerson},
				Preview: preview,
			}) {
				i := contentIdx[j]
				r.I, r.BlockedBy = i, nil
				res.Ops[i] = r
				switch r.Status {
				case change.OpRefused:
					refuse(i, r.Error)
				case change.OpApplied, change.OpPreviewed:
					blockChanged = true
				}
			}
		}
		for _, i := range decisions {
			op := set.Ops[i]
			if start := began[i]; role == cacheIsTheProject && op.IfMatch != change.AnyRevision && op.IfMatch != start.Rev {
				refuse(i, &change.Error{Code: change.CodeStale, Field: "if_match",
					Message: fmt.Sprintf("the %s translation is at %s, not %s", op.At.Edition.Locale, start.Rev, op.IfMatch)})
				res.Ops[i].Current = &start
				continue
			}
			rev := model.EditionRevision(b, op.At.Edition)
			if op.At.Edition.Locale == "" {
				refuse(i, &change.Error{Code: change.CodeUnsupported, Capability: "decide",
					Message: "a review decision is on a translation; the source is reviewed where it is written"})
				continue
			}
			moved, cerr := decideCached(b, op.At.Edition.Locale, op.Body.(*change.Decide).Outcome)
			if cerr != nil {
				refuse(i, cerr)
				continue
			}
			res.Ops[i].Before, res.Ops[i].After = rev, rev
			res.Ops[i].Status = change.OpUnchanged
			if moved {
				res.Ops[i].Status = change.OpApplied
				blockChanged = true
			}
		}
		if blockChanged {
			changed = append(changed, b)
		}
	}

	if refused >= 0 {
		res.Status = change.SetRefused
		for i := range res.Ops {
			if res.Ops[i].Status != change.OpRefused {
				blockedBy := refused
				res.Ops[i] = change.OpResult{I: i, Op: res.Ops[i].Op, At: res.Ops[i].At, Status: change.OpNotApplied, BlockedBy: &blockedBy}
			}
		}
		return res
	}
	if preview {
		res.Status = change.SetPreviewed
		for i := range res.Ops {
			if res.Ops[i].Status == change.OpApplied {
				res.Ops[i].Status = change.OpPreviewed
			}
		}
		return res
	}
	if len(changed) > 0 {
		// The blocks carry the ids the server gave them, so they are stored as
		// they are rather than mapped through an item.
		if err := a.store.StoreBlocks(ctx, projectID, editorStream, changed); err != nil {
			return change.ErrorResult(&change.Error{Code: change.CodeUnreachable,
				Message: "the local cache could not store the change: " + err.Error()})
		}
	}
	return res
}

// decideCached applies a review decision to a cached block's translation in
// one language, the way the server applies it: establish moves a translation
// with text to established, reject moves it to draft so it re-enters the work
// queue, and withdraw moves it to translated. It reports whether the block
// changed. advise is refused, as the server refuses it: a pre-review is an
// agent's, and the server keeps none.
func decideCached(b *model.Block, locale model.LocaleID, outcome change.Outcome) (bool, *change.Error) {
	t := b.Target(locale)
	switch outcome {
	case change.OutcomeEstablish:
		if t == nil || strings.TrimSpace(b.TargetText(locale)) == "" {
			return false, &change.Error{Code: change.CodeUnsupported, Capability: "decide.establish",
				Message: fmt.Sprintf("block %s has no %s translation to establish: translate it first", b.ID, locale)}
		}
		if t.Status.Rank() >= model.TargetStatusEstablished.Rank() {
			return false, nil
		}
		t.Status = model.TargetStatusEstablished
		return true, nil
	case change.OutcomeReject, change.OutcomeWithdraw:
		if t == nil {
			// Nothing to move. A block reviewed under the block-wide flag keeps
			// it until a decision clears it.
			if _, ok := b.Properties[legacyTranslationStatusProperty]; ok {
				delete(b.Properties, legacyTranslationStatusProperty)
				return true, nil
			}
			return false, nil
		}
		to := model.TargetStatusTranslated
		if outcome == change.OutcomeReject {
			to = model.TargetStatusDraft
		}
		if t.Status == to {
			return false, nil
		}
		t.Status = to
		return true, nil
	}
	return false, &change.Error{Code: change.CodeUnsupported, Capability: "decide." + string(outcome),
		Message: "the server keeps no pre-reviews; a person decides"}
}

// refusal is the error a change set the server refused replays as: a
// StatusError with the status the server answers that refusal with, so the
// replay retires a permanent refusal (stale, not permitted, a failing check)
// and tries again later on one the server could not complete (unreachable). It
// is nil for a change set that landed.
func refusal(res *change.Result) error {
	if res.Status != change.SetRefused {
		return nil
	}
	e := res.Error
	for i := 0; e == nil && i < len(res.Ops); i++ {
		if res.Ops[i].Status == change.OpRefused {
			e = res.Ops[i].Error
		}
	}
	if e == nil {
		e = &change.Error{Code: change.CodeInvalid, Message: "the server refused the change set"}
	}
	body, _ := json.Marshal(res)
	statusErr := apiclient.NewStatusError("changes", e.Code.HTTPStatus(), body)
	statusErr.Code = string(e.Code)
	statusErr.Message = e.Message
	return statusErr
}
