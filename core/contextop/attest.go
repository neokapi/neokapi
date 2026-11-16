package contextop

import (
	"strings"
	"time"
)

// ItemKind names the store an attested item lives in.
type ItemKind string

const (
	// ItemOp is an operation of this log that carries a subject: a term rule
	// or a note. Its revision is Record.Revision.
	ItemOp ItemKind = "op"
	// ItemConcept is a concept of the terms store. Its revision is the
	// concept's revision there.
	ItemConcept ItemKind = "concept"
	// ItemVoiceRule is one rule of a voice profile. Its ID is the rule's id
	// within the profile, and Profile names the profile.
	ItemVoiceRule ItemKind = "voice_rule"
)

// ItemRef names one item a person can attest, in whichever store holds it.
type ItemRef struct {
	Kind    ItemKind `json:"kind"`
	ID      string   `json:"id"`
	Profile string   `json:"profile,omitempty"`
}

// Key is the item's identity as one string, for indexing attestations.
func (r ItemRef) Key() string {
	return strings.Join([]string{string(r.Kind), r.Profile, r.ID}, "\x00")
}

// ReviewBasis is the ground an attestation is valid on, decided by the
// writer that records it and never chosen by the reviewer.
type ReviewBasis string

const (
	// BasisPeer is a review of an item another person wrote.
	BasisPeer ReviewBasis = "peer"
	// BasisAgentAuthored is a review, by the member an agent or a tool ran
	// for, of what that agent or tool wrote.
	BasisAgentAuthored ReviewBasis = "agent_authored"
	// BasisSoloOwner is the workspace owner reviewing their own item where no
	// other member could review it.
	BasisSoloOwner ReviewBasis = "solo_owner"
	// BasisSelfWarned is a reviewer's own item admitted under a policy that
	// warns rather than blocks.
	BasisSelfWarned ReviewBasis = "self_warned"
)

// VerdictKeep is the one verdict an attestation records: the reviewer read
// the item at the revision named and keeps it. Retiring or changing an item
// is an operation of its own and needs no attestation.
const VerdictKeep = "keep"

// Attestation is what an attest operation carries: a person read one item at
// one revision and keeps it. It changes nothing in force.
type Attestation struct {
	Item     ItemRef     `json:"item"`
	Revision string      `json:"revision"`
	Verdict  string      `json:"verdict"`
	Basis    ReviewBasis `json:"basis"`
}

// Review is the latest attestation of an item, as the fold reads it.
type Review struct {
	// ID is the attest operation's id.
	ID       string      `json:"id"`
	By       Actor       `json:"by"`
	At       time.Time   `json:"at"`
	Revision string      `json:"revision"`
	Basis    ReviewBasis `json:"basis"`
}

// Current reports whether the review covers the item at revision.
func (r *Review) Current(revision string) bool {
	return r != nil && r.Revision != "" && r.Revision == revision
}

// Reviews returns the latest attestation of every item the records attest,
// keyed by ItemRef.Key. The records are read in id order, so a later
// attestation of an item replaces an earlier one. An attestation set aside by
// a reset is left out, and so is one an agent or a tool recorded: only a
// person reviews (PersonDecides), and a server reads an operation a tool
// pushed as the tool's, whatever actor the operation names.
func Reviews(records []Record) map[string]*Review {
	out := map[string]*Review{}
	var latest map[string]string
	for _, r := range records {
		if r.Kind != KindAttest || r.Attest == nil || r.Status == StatusReset {
			continue
		}
		if r.Actor.Kind != "" && r.Actor.Kind != ActorPerson {
			continue
		}
		key := r.Attest.Item.Key()
		if latest == nil {
			latest = map[string]string{}
		}
		if prev, ok := latest[key]; ok && prev > r.ID {
			continue
		}
		latest[key] = r.ID
		out[key] = &Review{
			ID:       r.ID,
			By:       r.Actor,
			At:       r.At.UTC(),
			Revision: r.Attest.Revision,
			Basis:    r.Attest.Basis,
		}
	}
	return out
}

// sameScope reports whether two scopes say the same thing, so a keep that
// restates the scope its target already holds leaves the revision alone.
func sameScope(a, b Scope) bool {
	levelOf := func(l Level) Level {
		if l == "" {
			return LevelProject
		}
		return l
	}
	if levelOf(a.Level) != levelOf(b.Level) || a.AllProfiles != b.AllProfiles || len(a.Coordinates) != len(b.Coordinates) {
		return false
	}
	for k, v := range a.Coordinates {
		if b.Coordinates[k] != v {
			return false
		}
	}
	return sameInstant(a.ValidFrom, b.ValidFrom) && sameInstant(a.ValidTo, b.ValidTo)
}

func sameInstant(a, b *time.Time) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return a.Equal(*b)
}
