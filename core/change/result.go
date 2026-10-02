package change

import "github.com/neokapi/neokapi/core/model"

// SetStatus is the outcome of a whole change set.
type SetStatus string

const (
	// SetApplied: every operation landed.
	SetApplied SetStatus = "applied"
	// SetRefused: an operation was refused and nothing was written.
	SetRefused SetStatus = "refused"
	// SetPreviewed: the change set was computed and checked, and nothing was
	// written because the mode was preview.
	SetPreviewed SetStatus = "previewed"
	// SetPartial: an I/O error interrupted the final writes. The documents
	// that landed say written.
	SetPartial SetStatus = "partial"
)

// OpStatus is the outcome of one operation.
type OpStatus string

const (
	// OpApplied: the operation changed what it addresses.
	OpApplied OpStatus = "applied"
	// OpUnchanged: what it addresses already says that.
	OpUnchanged OpStatus = "unchanged"
	// OpRefused: the operation was refused; Error says why.
	OpRefused OpStatus = "refused"
	// OpNotApplied: another operation of the change set was refused, so this
	// one was not written; BlockedBy names it.
	OpNotApplied OpStatus = "not_applied"
	// OpPreviewed: the operation would apply; the mode was preview.
	OpPreviewed OpStatus = "previewed"
)

// Result is the outcome of a change set.
type Result struct {
	// Schema is ResultSchemaID.
	Schema string    `json:"schema"`
	Status SetStatus `json:"status"`
	// Record is the id of the recorded edit, when something was written and
	// recorded.
	Record *string     `json:"record"`
	Docs   []DocResult `json:"docs"`
	Ops    []OpResult  `json:"ops"`
}

// DocResult is the outcome for one document.
type DocResult struct {
	Doc string `json:"doc"`
	// Home is where the document's text lives: file, workspace or
	// stream:<id>.
	Home    string `json:"home,omitempty"`
	Written bool   `json:"written"`
	// Before and After are the document digests around the change.
	Before   string    `json:"before,omitempty"`
	After    *string   `json:"after"`
	Findings []Finding `json:"findings,omitempty"`
}

// Finding is one thing a check of the changed content found.
type Finding struct {
	// Rule names the rule, such as terms.vocabulary or guard.codes_changed.
	Rule    string `json:"rule"`
	Message string `json:"message"`
	// Fails says whether the finding fails a gate; an advisory or suggested
	// rule reports without failing.
	Fails     bool `json:"fails"`
	Suggested bool `json:"suggested,omitempty"`
	At        *Ref `json:"at,omitempty"`
}

// OpResult is the outcome of one operation.
type OpResult struct {
	// I is the operation's index in the change set.
	I      int      `json:"i"`
	Op     Kind     `json:"op"`
	Status OpStatus `json:"status"`
	At     *Ref     `json:"at,omitempty"`
	// Before and After are the edition's revisions around the operation.
	Before string `json:"before,omitempty"`
	After  string `json:"after,omitempty"`
	// Basis is the authoritative edition's revision a derived edition was
	// made from, as recorded.
	Basis string `json:"basis,omitempty"`
	// ID is the id of the annotation an annotate wrote.
	ID string `json:"id,omitempty"`
	// Resolved echoes every position the operation resolved, in the order of
	// its edits, so the sender learns the canonical form.
	Resolved []Resolved `json:"resolved,omitempty"`
	// Invalidates lists the derived editions whose basis this operation moved.
	Invalidates []Invalidation `json:"invalidates,omitempty"`
	// Findings are guard findings the operation landed with, under a report
	// disposition.
	Findings []Finding `json:"findings,omitempty"`
	Error    *Error    `json:"error,omitempty"`
	// Current is the edition as it stands, on a stale refusal.
	Current *Current `json:"current,omitempty"`
	// BlockedBy is the index of the refused operation that kept this one from
	// being written.
	BlockedBy *int `json:"blocked_by,omitempty"`
}

// Resolved is a span the service resolved, with the path of the run sequence
// it lies in.
type Resolved struct {
	Path  model.RunPath `json:"path,omitempty"`
	Start model.RunPos  `json:"start"`
	End   model.RunPos  `json:"end"`
}

// Invalidation names a derived edition an edit made stale.
type Invalidation struct {
	Edition string `json:"edition"`
	// Reason is basis_moved: the authoritative edition it was made from
	// changed.
	Reason string `json:"reason"`
}

// ReasonBasisMoved is the reason an edit to the authoritative edition gives
// for every derived edition.
const ReasonBasisMoved = "basis_moved"

// Current is an edition as it stands, sent with a stale refusal so the sender
// can rebase without another read.
type Current struct {
	Rev string `json:"rev"`
	// Text is the edition in the placeholder form a read shows.
	Text string `json:"text,omitempty"`
}
