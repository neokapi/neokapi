package client

// The terminology and context ledger parts of a push travel in the commit
// manifest beside the content. The venue applies the manifest in one
// transaction, so a push lands whole or not at all: a refused terminology edit
// or ledger object leaves the content where it was, and the other way round.

// TermsEdits is the terminology part of a push: the project's concept and
// relation edits, classified against the workspace baseline it last pulled.
// Ordinary edits apply directly; governed ones travel as Proposal, which the
// venue records as one change-set in review.
type TermsEdits struct {
	Creates         []CreateConceptParams `json:"creates,omitempty"`
	Updates         []ConceptEdit         `json:"updates,omitempty"`
	RelationAdds    []RelationEdit        `json:"relation_adds,omitempty"`
	RelationRemoves []RelationRemoval     `json:"relation_removes,omitempty"`
	Proposal        *TermsProposal        `json:"proposal,omitempty"`
}

// ConceptEdit is an ordinary edit of one concept the workspace holds.
type ConceptEdit struct {
	ConceptID string              `json:"concept_id"`
	Concept   UpdateConceptParams `json:"concept"`
}

// RelationEdit adds an ordinary relation from SourceID.
type RelationEdit struct {
	SourceID string            `json:"source_id"`
	Relation AddRelationParams `json:"relation"`
}

// RelationRemoval removes one relation of SourceID.
type RelationRemoval struct {
	SourceID   string `json:"source_id"`
	RelationID string `json:"relation_id"`
}

// TermsProposal is the governed half of the terminology part: the operations
// one change-set carries into review. Origin admits one open change-set at a
// time on the venue, so a push repeating the same operations returns the
// change-set already in review.
type TermsProposal struct {
	Name        string             `json:"name"`
	Description string             `json:"description,omitempty"`
	Origin      string             `json:"origin,omitempty"`
	Ops         []ChangeSetOpInput `json:"ops"`
	// ExpectedTermsRef is the terms component of the ref the project last
	// observed. The operations are a diff against that state, so the venue
	// refuses the push when its terminology has moved since.
	ExpectedTermsRef string `json:"expected_terms_ref,omitempty"`
}

// Empty reports whether the edits change nothing.
func (t *TermsEdits) Empty() bool {
	return t == nil || (len(t.Creates) == 0 && len(t.Updates) == 0 &&
		len(t.RelationAdds) == 0 && len(t.RelationRemoves) == 0 &&
		(t.Proposal == nil || len(t.Proposal.Ops) == 0))
}

// Applied counts the ordinary concept and relation edits.
func (t *TermsEdits) Applied() (concepts, relations int) {
	if t == nil {
		return 0, 0
	}
	return len(t.Creates) + len(t.Updates), len(t.RelationAdds) + len(t.RelationRemoves)
}

// Proposed counts the governed operations.
func (t *TermsEdits) Proposed() int {
	if t == nil || t.Proposal == nil {
		return 0
	}
	return len(t.Proposal.Ops)
}

// CarryTerms sends the project's terminology edits in the push, so they apply
// in the same transaction as the content.
func CarryTerms(edits *TermsEdits) PushOption {
	return func(s *pushSettings) {
		if !edits.Empty() {
			s.terms = edits
		}
	}
}

// CarryContextObjects sends the context ledger objects the venue does not hold
// (segments, and the blobs they name) in the push, so the project's context
// lands in the same transaction as its content.
func CarryContextObjects(objs []ContextObjectWire) PushOption {
	return func(s *pushSettings) { s.ledger = objs }
}
