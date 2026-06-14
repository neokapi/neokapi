package project

import (
	"sort"
	"strings"
	"time"
)

// ConceptBaseline is the snapshot a concept pull records into the sync cache:
// the governed concepts and typed relations fetched from the workspace
// knowledge graph and written into the project's bound termbase. A later
// concept push diffs the local termbase against this baseline to decide what
// changed and how to route it — ordinary edits (definitions, notes, new
// proposed terms, non-governed relations) go up directly through the concept
// REST endpoints, while governed edits (a term banned/promoted, a forbidden
// status removed, a REPLACED_BY relation, a concept delete) are bundled into a
// single reviewed change-set. The baseline carries only the fields a diff
// needs, deliberately omitting volatile metadata (timestamps) that would
// produce spurious diffs.
type ConceptBaseline struct {
	// PulledAt is when the snapshot was taken.
	PulledAt time.Time `json:"pulled_at"`

	// Concepts holds the pulled concepts keyed by concept ID.
	Concepts map[string]BaselineConcept `json:"concepts,omitempty"`

	// Relations holds the pulled typed relations keyed by relation ID.
	Relations map[string]BaselineRelation `json:"relations,omitempty"`
}

// BaselineConcept is the diff-relevant state of one pulled concept.
type BaselineConcept struct {
	Domain     string         `json:"domain,omitempty"`
	Definition string         `json:"definition,omitempty"`
	Terms      []BaselineTerm `json:"terms,omitempty"`
}

// BaselineTerm is the diff-relevant state of one term within a concept.
// Status is the lifecycle status string; together with the term identity
// (locale + lowered text) it lets a push tell a governed status transition
// from an ordinary edit.
type BaselineTerm struct {
	Text         string `json:"text"`
	Locale       string `json:"locale"`
	Status       string `json:"status"`
	PartOfSpeech string `json:"part_of_speech,omitempty"`
	Gender       string `json:"gender,omitempty"`
	Note         string `json:"note,omitempty"`
}

// BaselineRelation is the diff-relevant state of one typed relation.
type BaselineRelation struct {
	SourceID     string `json:"source_id"`
	TargetID     string `json:"target_id"`
	RelationType string `json:"relation_type"`
	Note         string `json:"note,omitempty"`
}

// TermIdentity keys a term within its concept by locale + lowered text, the
// same identity the server's governed-transition check uses. It is exported so
// the concept-sync diff and its tests share one definition of term identity.
func (t BaselineTerm) TermIdentity() string {
	return t.Locale + "|" + strings.ToLower(t.Text)
}

// SortedConceptIDs returns the baseline's concept IDs in a deterministic order
// so a diff over the baseline produces stable output.
func (b *ConceptBaseline) SortedConceptIDs() []string {
	if b == nil {
		return nil
	}
	ids := make([]string, 0, len(b.Concepts))
	for id := range b.Concepts {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}
