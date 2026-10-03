package backend

import (
	"bytes"
	"encoding/json"
	"errors"
	"time"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/model"
)

// FailedChange is a queued offline change that did not reach the server, with
// what it was, so a person can make it again. Status is "failed" for a change
// the server refused on replay (an edit to a translation someone changed in
// the meantime, a decision it did not accept), and "dropped" for an entry an
// earlier version queued under a kind this version no longer sends.
type FailedChange struct {
	ID     int64  `json:"id"`
	Status string `json:"status"`
	// Operation is the kind the entry was queued under: change_set for an
	// edit, a decision, a note or an entity mark, and the content-memory,
	// terms and item kinds otherwise.
	Operation string `json:"operation"`
	// Edits are the operations a change set carried, or the one edit a
	// dropped entry carried.
	Edits []FailedEdit `json:"edits,omitempty"`
	// Reason is why it did not reach the server.
	Reason   string    `json:"reason"`
	QueuedAt time.Time `json:"queued_at"`
}

// FailedEdit is one operation of a failed change: what it did, where, and the
// wording it carried.
type FailedEdit struct {
	// Op is the change-set operation (set_content, replace_text,
	// remove_edition, decide, annotate, unannotate).
	Op string `json:"op"`
	// Outcome is a decision's outcome (establish, reject, withdraw).
	Outcome string `json:"outcome,omitempty"`
	// Type is an annotation's type (note, entity).
	Type  string `json:"type,omitempty"`
	Item  string `json:"item,omitempty"`
	Block string `json:"block,omitempty"`
	// Locale is the translation's language; empty for the source.
	Locale string `json:"locale,omitempty"`
	// Text is the wording the operation carried, with inline codes as the
	// placeholders a read shows.
	Text string `json:"text,omitempty"`
}

// failedChangeOf reads what a queued entry was.
func failedChangeOf(c PendingChange) FailedChange {
	out := FailedChange{ID: c.ID, Status: c.Status, Operation: c.Operation, Reason: c.LastError, QueuedAt: c.CreatedAt}
	switch opKind(c.Operation) {
	case opChangeSet:
		out.Edits = changeSetEdits(c.Payload)
	case opUpdateBlockTarget, opUpdateBlockTargetRuns, opReviewBlock:
		out.Edits = retiredEdits(opKind(c.Operation), c.Payload)
	}
	return out
}

// changeSetEdits lists the operations of a queued change set.
func changeSetEdits(payload string) []FailedEdit {
	var queued changeSetOp
	if err := json.Unmarshal([]byte(payload), &queued); err != nil {
		return nil
	}
	set, err := change.Decode(bytes.NewReader(queued.Set))
	if err != nil {
		return nil
	}
	edits := make([]FailedEdit, 0, len(set.Ops))
	for _, op := range set.Ops {
		e := FailedEdit{Op: string(op.Kind), Item: op.At.Doc, Block: op.At.Block, Locale: string(op.At.Edition.Locale)}
		switch body := op.Body.(type) {
		case *change.SetContent:
			e.Text = contentText(body.Content)
		case *change.ReplaceText:
			if len(body.Edits) == 1 {
				e.Text = body.Edits[0].Text
			}
		case *change.Decide:
			e.Outcome = string(body.Outcome)
		case *change.Unannotate:
			e.Type = body.Type
		case *change.Annotate:
			e.Type = body.Type
			var v struct {
				Text string `json:"text"`
			}
			if json.Unmarshal(body.Value, &v) == nil {
				e.Text = v.Text
			}
		}
		edits = append(edits, e)
	}
	return edits
}

// contentText is a content body's wording, with inline codes as placeholders.
func contentText(c change.Content) string {
	if c.Text != nil {
		return *c.Text
	}
	return model.RunsEditText(c.Runs)
}

// retiredEdits reads the one edit an entry of a retired kind carried: a
// translation saved as text or as runs, or a review decision. The runs an
// earlier version queued are in a form this version no longer reads, so such
// an entry names its block and language without its wording.
func retiredEdits(kind opKind, payload string) []FailedEdit {
	var p struct {
		ItemName     string `json:"item_name"`
		BlockID      string `json:"block_id"`
		TargetLocale string `json:"target_locale"`
		Text         string `json:"text"`
		Reviewed     bool   `json:"reviewed"`
		Status       string `json:"status"`
	}
	if json.Unmarshal([]byte(payload), &p) != nil {
		return nil
	}
	e := FailedEdit{Op: string(change.KindSetContent), Item: p.ItemName, Block: p.BlockID, Locale: p.TargetLocale, Text: p.Text}
	if kind == opReviewBlock {
		e = FailedEdit{Op: string(change.KindDecide), Item: p.ItemName, Block: p.BlockID, Locale: p.TargetLocale}
		switch {
		case p.Reviewed:
			e.Outcome = string(change.OutcomeEstablish)
		case p.Status == string(model.TargetStatusDraft):
			e.Outcome = string(change.OutcomeReject)
		default:
			e.Outcome = string(change.OutcomeWithdraw)
		}
	}
	return []FailedEdit{e}
}

// GetFailedChanges lists the offline changes that did not reach the server,
// oldest first: those it refused on replay and those an earlier version queued
// under a kind this version no longer sends. Each says what it was, so a
// person can make it again.
func (a *App) GetFailedChanges() ([]FailedChange, error) {
	if a.offlineQueue == nil {
		return []FailedChange{}, nil
	}
	entries, err := a.offlineQueue.Failed()
	if err != nil {
		return nil, err
	}
	out := make([]FailedChange, 0, len(entries))
	for _, c := range entries {
		out = append(out, failedChangeOf(c))
	}
	return out, nil
}

// GetFailedChangeIDs lists the ids of the offline changes that did not reach
// the server, oldest first, without reading what each was. The frontend polls
// it and calls GetFailedChanges only when the ids change, because that call
// reads and decodes every entry's payload, the bytes of an upload included.
func (a *App) GetFailedChangeIDs() ([]int64, error) {
	if a.offlineQueue == nil {
		return []int64{}, nil
	}
	return a.offlineQueue.FailedIDs()
}

// DismissFailedChange removes one failed or dropped change from the list.
func (a *App) DismissFailedChange(id int64) error {
	if a.offlineQueue == nil {
		return errors.New("there is no offline queue")
	}
	return a.offlineQueue.Dismiss(id)
}

// DismissFailedChanges removes every failed and dropped change from the list.
func (a *App) DismissFailedChanges() error {
	if a.offlineQueue == nil {
		return nil
	}
	return a.offlineQueue.DismissAll()
}
