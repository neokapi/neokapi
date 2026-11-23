package venue

import (
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/partjoin"
	"github.com/neokapi/neokapi/core/ref"
	"github.com/neokapi/neokapi/terms"
)

// The freshness ref's component definitions.
//
// A component hash means something only when both sides compute it the same
// way. Each definition lives here once, and the push client, the plugin and
// the server all fold with it. A second definition with another byte layout
// would make two correct sides disagree and report a governance conflict on
// every write.
//
// The context component has no function here: it is ContextHashOf
// (context.go), which the push negotiates on.

// DecisionsComponent folds a decision ledger into the ref's decisions
// component: one identity per (item, unit, variant) for each record that
// decides something (UnitDecision.IsDecision), folded by ref.Fold. Records
// that say only what was produced are left out, so a run that drafts moves
// nothing a client asserts.
//
// Order-independent by construction, because the fold sorts its keys. That
// matters here: the committed record arrives in shard order and the server's
// ledger arrives in SQL order, and the two must agree whenever they hold the
// same decisions.
func DecisionsComponent(decisions []UnitDecision) string {
	if len(decisions) == 0 {
		return ""
	}
	parts := make(map[string]string, len(decisions))
	for _, d := range decisions {
		// Only decisions are folded. A record that says only what was produced
		// for a unit moves as drafting runs, and a client that read the
		// component before a run has missed no decision.
		if d.Unit == "" || d.Variant == "" || !d.IsDecision() {
			continue
		}
		parts[decisionKey(d)] = DecisionIdentity(d)
	}
	if len(parts) == 0 {
		return ""
	}
	return ref.Fold(parts)
}

// DecisionRecordsHash folds every keyed record, decisions and records of what
// was produced alike. A producer compares it with the fold it last sent to
// tell whether its committed record has changed since; the decisions component
// cannot answer that, because it leaves records of what was produced out.
func DecisionRecordsHash(decisions []UnitDecision) string {
	parts := make(map[string]string, len(decisions))
	for _, d := range decisions {
		if d.Unit == "" || d.Variant == "" {
			continue
		}
		parts[decisionKey(d)] = DecisionIdentity(d)
	}
	if len(parts) == 0 {
		return ""
	}
	return ref.Fold(parts)
}

// decisionKey is the ledger's own primary key, rendered for the fold.
func decisionKey(d UnitDecision) string {
	return partjoin.NUL(d.ItemName, d.Unit, d.Variant)
}

// DecisionIdentity hashes one decision over the fields that carry the decision
// itself.
//
// It covers exactly the fields SameDecision compares, which is exactly what a
// store treats as a change. So a record a store would skip as unchanged cannot
// move the decisions component, and a component that moved means a decision
// did. Updated is excluded for that reason: it orders conflicting
// records for last-writer-wins, and a replay that only bumped it writes
// nothing, so counting it would report a change no reader could find.
func DecisionIdentity(d UnitDecision) string {
	return ref.Identity(decisionFields(d)...)
}

// SameDecision reports whether two records say the same thing about a unit. It
// is the idempotency test a store applies before writing, and the equality the
// decisions component is folded from.
func SameDecision(a, b UnitDecision) bool {
	return slices.Equal(decisionFields(a), decisionFields(b))
}

// decisionFields lists a decision's identity-bearing fields in a fixed order.
// One list serves both the equality test and the hash, so the two cannot drift.
//
// The governing fingerprint is appended only when the record carries one. A
// record written before the field existed then keeps the identity it always
// had, so upgrading either end moves no project's decisions component; a
// record that gains one is a real change a store must write.
func decisionFields(d UnitDecision) []string {
	parked := "false"
	if d.Parked {
		parked = "true"
	}
	fields := []string{
		d.Status,
		d.Revision,
		d.Basis,
		d.ReviewState,
		d.DecidedBy,
		d.DecidedAt,
		identityNote(d),
		parked,
		d.Assignee,
	}
	if d.GoverningFingerprint != "" {
		fields = append(fields, d.GoverningFingerprint)
	}
	// Reasons are left out. A client that predates them computes this
	// identity without them, and the decisions component the two ends compare
	// must agree for such a client, or it would resend every decision on
	// every push. Reasons are chosen together with the decision, which stamps
	// DecidedAt, so a decision with other reasons has another identity
	// anyway. A store compares reasons on its own (SameReasons).
	return fields
}

// identityNote is the note as the decision identity reads it: the form a
// released client wrote, with the agent host folded in
// (change.LegacyNote). A client older than AgentHost computes the identity over
// that note, so a record held as note and host keeps the identity it had
// and no push resends it. Nothing stores this form.
func identityNote(d UnitDecision) string {
	text, host := change.SplitNote(d.Note)
	if d.AgentHost != "" {
		host = d.AgentHost
	}
	return change.LegacyNote(text, host)
}

// SplitNote is d with its note as the person typed it and the agent host in
// AgentHost, for a record whose note a released client wrote with the host
// folded in. A store applies it on the way in; a record without the phrase
// comes back as it is.
func SplitNote(d UnitDecision) UnitDecision {
	text, host := change.SplitNote(d.Note)
	if host == "" {
		return d
	}
	d.Note = text
	if d.AgentHost == "" {
		d.AgentHost = host
	}
	return d
}

// CarryHost is next with the agent host stored keeps, when next does not say
// and is the same act of deciding (CarryReasons). A client older than the
// field echoes a decision it pulled without its host.
func CarryHost(stored, next UnitDecision) UnitDecision {
	if next.AgentHost == "" && stored.ReviewState == next.ReviewState && stored.DecidedBy == next.DecidedBy &&
		stored.DecidedAt == next.DecidedAt && stored.Revision == next.Revision {
		next.AgentHost = stored.AgentHost
	}
	return next
}

// DecisionReasons are the reason codes a person may give a rejection or a
// correction, in the order a reason sheet offers them.
var DecisionReasons = []string{"wrong_term", "meaning", "tone", "placeholder", "grammar"}

// ReasonReviewPolicy is the reason a review plan's settle pass records on
// each approval it makes: the translation cleared the project's bars, and
// no person looked at it.
const ReasonReviewPolicy = "review_policy"

// knownReason reports whether r is a reason a decision may carry.
func knownReason(r string) bool {
	return r == ReasonReviewPolicy || slices.Contains(DecisionReasons, r)
}

// MaxDecisionNote is the longest note, in characters, a decision keeps.
const MaxDecisionNote = 2000

// DecisionInputError is a note or a reason a decision may not carry. Field
// names which: "note" or "reasons".
type DecisionInputError struct {
	Field   string
	Message string
}

func (e *DecisionInputError) Error() string { return e.Message }

// CheckDecisionInput refuses a note longer than MaxDecisionNote characters
// and a reason that is neither a DecisionReasons code nor
// ReasonReviewPolicy. Every place that takes a decision from a person or a
// client asks it, so each refuses with the same words and none cuts or
// drops what a person wrote.
func CheckDecisionInput(note string, reasons []string) *DecisionInputError {
	if n := utf8.RuneCountInString(note); n > MaxDecisionNote {
		return &DecisionInputError{Field: "note",
			Message: fmt.Sprintf("the note is %d characters long; a decision keeps at most %d", n, MaxDecisionNote)}
	}
	for _, r := range reasons {
		if !knownReason(r) {
			return &DecisionInputError{Field: "reasons",
				Message: fmt.Sprintf("%q is not a reason: want one of %s", r,
					strings.Join(append(slices.Clone(DecisionReasons), ReasonReviewPolicy), ", "))}
		}
	}
	return nil
}

// CheckStatedInput is CheckDecisionInput over what next states that stored
// does not already hold: the note when it differs, and the reasons stored
// does not carry. A writer passing on a record it did not author (a push
// echo, a job, a replay) is never refused for what the ledger holds. So one
// record stored before the bound cannot fail every push after it. stored is
// nil for a unit with no record.
func CheckStatedInput(stored *UnitDecision, next UnitDecision) *DecisionInputError {
	note := next.Note
	if stored != nil && stored.Note == next.Note {
		note = ""
	}
	var reasons []string
	for _, r := range next.Reasons {
		if stored == nil || !slices.Contains(stored.Reasons, r) {
			reasons = append(reasons, r)
		}
	}
	return CheckDecisionInput(note, reasons)
}

// NormalizeReasons is reasons as a decision stores them: each once, sorted.
// It keeps every reason it is given; CheckDecisionInput is what refuses an
// unknown one. A nil list stays nil (the sender did not say), and a non-nil
// list stays non-nil, so the difference survives.
func NormalizeReasons(reasons []string) []string {
	if reasons == nil {
		return nil
	}
	out := make([]string, 0, len(reasons))
	for _, r := range reasons {
		if !slices.Contains(out, r) {
			out = append(out, r)
		}
	}
	slices.Sort(out)
	return out
}

// SameReasons reports whether two records carry the same reasons, a nil list
// and an empty one alike.
func SameReasons(a, b UnitDecision) bool {
	return slices.Equal(NormalizeReasons(a.Reasons), NormalizeReasons(b.Reasons))
}

// CarryReasons is next with the reasons stored keeps, when next does not say
// (a nil list) and is the same act of deciding: the same verdict by the same
// person at the same time on the same translation. A client older than the
// field echoes a decision it pulled without its reasons; this keeps the
// echo from erasing them.
func CarryReasons(stored, next UnitDecision) UnitDecision {
	if next.Reasons == nil && stored.ReviewState == next.ReviewState && stored.DecidedBy == next.DecidedBy &&
		stored.DecidedAt == next.DecidedAt && stored.Revision == next.Revision {
		next.Reasons = stored.Reasons
	}
	return next
}

// TermsComponent folds a terminology set into the ref's terms component: one
// identity per concept and per relation, folded by ref.Fold.
//
// The concept identity covers the fields a concept diff is computed against:
// domain, definition, and each term's locale, text, status and grammatical
// metadata. Volatile bookkeeping (timestamps, server-minted properties) is
// excluded: a component that moved on a touch nobody made would send every
// project back to a full re-fetch for nothing.
func TermsComponent(concepts []terms.Concept, relations []terms.ConceptRelation) string {
	if len(concepts) == 0 && len(relations) == 0 {
		return ""
	}
	parts := make(map[string]string, len(concepts)+len(relations))
	for _, c := range concepts {
		parts["concept\x00"+c.ID] = conceptIdentity(c)
	}
	for _, r := range relations {
		parts["relation\x00"+r.ID] = relationIdentity(r)
	}
	return ref.Fold(parts)
}

// conceptIdentity hashes one concept over its governed surface.
func conceptIdentity(c terms.Concept) string {
	// A store returns a concept's terms in insertion order, which is not an
	// identity: the same concept re-read after an unrelated edit must hash the
	// same, so the term list is folded rather than concatenated in place.
	byTerm := make(map[string]string, len(c.Terms))
	for _, t := range c.Terms {
		key := partjoin.NUL(string(t.Locale), t.Text)
		byTerm[key] = ref.Identity(string(t.Status), t.PartOfSpeech, t.Gender, t.Note)
	}
	return ref.Identity(c.Domain, c.Definition, ref.Fold(byTerm))
}

// relationIdentity hashes one typed relation over its endpoints, its label and
// its note.
func relationIdentity(r terms.ConceptRelation) string {
	return ref.Identity(r.SourceID, r.TargetID, r.RelationType, r.Note)
}
