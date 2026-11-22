package venue

// A push carries more than content. Beside the blocks it carries the declared
// structure (collections, coordinates, voice), the project's terminology edits
// and, when the project's context lives on the venue, the context ledger
// objects the venue does not hold. The venue applies all of them in one
// transaction. These are what it reports back about the last two, after the
// transaction commits.

// PushTermsOutcome is what the terminology part of a push did. Ordinary edits
// apply directly; governed edits are bundled into one change-set that waits
// for review.
type PushTermsOutcome struct {
	// ConceptsApplied counts the concepts created or edited directly.
	ConceptsApplied int `json:"concepts_applied"`
	// RelationsApplied counts the relations added or removed directly.
	RelationsApplied int `json:"relations_applied"`
	// ConceptsProposed counts the governed operations in the change-set.
	ConceptsProposed int `json:"concepts_proposed"`
	// ChangesetID names the change-set in review, empty when the push
	// proposed nothing.
	ChangesetID string `json:"changeset_id,omitempty"`
	// ChangesetUnchanged reports that the change-set already in review held
	// the same operations, so ChangesetID names that one and nothing new was
	// proposed.
	ChangesetUnchanged bool `json:"changeset_unchanged,omitempty"`
}

// Empty reports whether the terminology part did nothing.
func (o *PushTermsOutcome) Empty() bool {
	return o == nil || (o.ConceptsApplied == 0 && o.RelationsApplied == 0 && o.ConceptsProposed == 0)
}

// PushLedgerOutcome is what the context ledger part of a push did.
type PushLedgerOutcome struct {
	// Objects counts the ledger objects (segments and the blobs they name)
	// the push carried.
	Objects int `json:"objects"`
	// Operations counts the ledger operations the venue did not hold before.
	Operations int `json:"operations"`
	// Bound reports that this push bound the venue project's context to the
	// project key the ledger names.
	Bound bool `json:"bound,omitempty"`
}

// PushParts is what a venue reports about the terminology and ledger parts
// of an applied push. A part the push did not carry is nil.
type PushParts struct {
	Terms  *PushTermsOutcome  `json:"terms,omitempty"`
	Ledger *PushLedgerOutcome `json:"ledger,omitempty"`
}

// Empty reports whether the push carried neither part.
func (p PushParts) Empty() bool { return p.Terms == nil && p.Ledger == nil }
