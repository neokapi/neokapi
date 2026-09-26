// Package contextop records how a project's context came to be.
//
// A project starts with no terms, no voice rules and an empty content memory,
// and the context grows out of ordinary work. Someone notices a fact or the
// form the project uses for a word, a person keeps it, checks enforce it, and
// the next correction is evidence for the rule after that. Every step is an
// operation: who did it, what it was about, where the evidence was seen, what
// governed at the time, and what became of it.
//
// # Append only
//
// Operations go into the workspace's operation log (core/workspace) and are
// never edited. A status is not a column that moves: keeping a suggestion
// writes a `keep` operation naming it, dropping writes a `drop`, withdrawing
// writes a `withdraw` and reverting writes a `revert`. [Ledger.Records] reads
// the log and reports the status each subject-bearing operation ended up with,
// so the history is the whole record and a surface can replay it.
//
// # Suggestions advise, established rules bind
//
// Everything recorded about a project is a suggestion, and it advises the
// moment it is recorded. [Resolve] and [Resolution.RuleSets] project the
// suggestions at a point into rule sets marked advisory, which the vocabulary
// matcher reports at neutral severity, below every gate threshold. A
// suggestion therefore shows up in a check and can never fail one. A person
// keeping it is what establishes it, and keeping writes the rule into the
// terms store or the content memory, where every existing reader already
// looks. What a person imports or edits directly is established from the
// start. This log is the history and the source of suggestions, not a second
// home for established rules.
//
// # Contested
//
// Two suggestions about one word that name different forms to use disagree:
// both advise, both are marked contested, and neither can be kept until a
// person chooses. A suggestion that contradicts an established rule is
// contested and the rule stays in force. A person's own correction that
// reverses an established rule contests the rule, which then reports instead
// of failing, so a person's edit never fails their own build.
//
// # Scope
//
// What is learned is scoped to the point where its evidence was seen, which for
// a new project is that project. Keeping is also the moment a person may widen
// a rule to a broader point, or to the whole workspace, where it answers in
// every project. [Resolve] puts workspace-wide rules beneath project ones, so
// the most specific rule about a term wins.
package contextop

import (
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/core/workspace"
)

// OpKindPrefix marks the operations this package writes into the workspace log.
// The workspace owns the vocabulary of its own kinds (workspace.OpRegisterProject
// and its neighbours); every kind under this prefix belongs here.
const OpKindPrefix = "context."

// ActorKind says what sort of actor performed an operation. It is what the
// policy reads, so the rights an actor class has are decided in one place.
type ActorKind string

const (
	// ActorPerson is a human working in the project.
	ActorPerson ActorKind = "person"
	// ActorAgent is a coding agent acting under a session of its own.
	ActorAgent ActorKind = "agent"
	// ActorTool is an automated pass with no session: a convergence run, an
	// importer.
	ActorTool ActorKind = "tool"
)

// Actor is who performed an operation.
type Actor struct {
	// Kind is the class of actor, which decides what it may do.
	Kind ActorKind `json:"kind"`
	// Name identifies the actor within its kind: a person's name or handle, an
	// agent's client name, a tool's name. It may be empty for a person working
	// alone.
	Name string `json:"name,omitempty"`
	// Session groups the operations one agent run recorded, so a whole session
	// can be reviewed or reverted together. Empty for a person and for a tool.
	Session string `json:"session,omitempty"`
}

// String renders an actor for a log line: "agent claude/abc123", "person asgeir",
// or the bare kind when it carries no name.
func (a Actor) String() string {
	out := string(a.Kind)
	if out == "" {
		out = string(ActorPerson)
	}
	if a.Name != "" {
		out += " " + a.Name
	}
	if a.Session != "" {
		out += "/" + a.Session
	}
	return out
}

// Kind says what an operation did.
type Kind string

const (
	// KindObserve records something somebody noticed: a fact in prose, or the
	// form the project uses for a word with the forms it avoids, which is a
	// term rule. It advises from the moment it is recorded.
	KindObserve Kind = "observe"
	// KindCorrect records that someone changed wording from one form to another
	// at a location. It carries both wordings, and it may carry the term rule
	// drawn from them.
	KindCorrect Kind = "correct"
	// KindImport records one context file a person read into the project's
	// stores. What a person imports is established from the start.
	KindImport Kind = "import"
	// KindEdit records a rule or a store a person wrote directly, with `kapi
	// apply` or an editor. It is established from the start.
	KindEdit Kind = "edit"
	// KindKeep establishes an earlier suggestion, optionally with edits to the
	// rule. Only a person keeps.
	KindKeep Kind = "keep"
	// KindDrop is a person setting a suggestion aside. It stops answering.
	KindDrop Kind = "drop"
	// KindWithdraw is the author of a suggestion taking it back, in the session
	// that recorded it. It stops answering.
	KindWithdraw Kind = "withdraw"
	// KindRevert undoes an earlier operation, or every operation one session
	// recorded, and retracts whatever they put in force.
	KindRevert Kind = "revert"
	// KindWiden moves an established rule to a broader point, or to the whole
	// workspace.
	KindWiden Kind = "widen"
	// KindSignal records evidence about a suggestion that no person recorded
	// by hand: the change that reached the default branch, or how often the
	// project's content writes the preferred and the rejected forms. It names
	// the suggestion it bears on and carries a [Signal].
	KindSignal Kind = "signal"
	// KindEstablish records that settling established a suggestion, naming
	// the operations the establishment rested on (Record.Because). Its id is
	// derived from that evidence, so two machines settling the same thing
	// record one operation. It is applied like a keep.
	KindEstablish Kind = "establish"
)

// Kinds is every operation kind, in the order a reader meets them.
var Kinds = []Kind{KindObserve, KindCorrect, KindImport, KindEdit, KindKeep, KindDrop, KindWithdraw, KindRevert, KindWiden, KindSignal, KindEstablish}

// Valid reports whether k is one of the declared kinds.
func (k Kind) Valid() bool { return slices.Contains(Kinds, k) }

// Bears reports whether an operation of this kind carries a subject of its own.
// The kinds that do are what a status is folded onto; the rest act on them.
func (k Kind) Bears() bool {
	return k == KindObserve || k == KindCorrect || k == KindImport || k == KindEdit
}

// Status is what became of a subject-bearing operation. It is folded from the
// log rather than stored, because the log is never edited.
type Status string

const (
	// StatusSuggested is an operation nothing has acted on. Its rule advises
	// and never fails a check.
	StatusSuggested Status = "suggested"
	// StatusEstablished is a rule a person kept, imported or wrote. It is in
	// force at the severity it carries.
	StatusEstablished Status = "established"
	// StatusContested is a rule that disagrees with another: two suggestions
	// naming different forms for one word, a suggestion that contradicts an
	// established rule, or an established rule a person's own correction
	// reversed. It advises and fails no check until a person chooses.
	// Record.ContestedBy names the other side.
	StatusContested Status = "contested"
	// StatusWithdrawn is a suggestion its author took back in the session that
	// recorded it. It stops answering.
	StatusWithdrawn Status = "withdrawn"
	// StatusDropped is a suggestion a person set aside. It stops answering.
	StatusDropped Status = "dropped"
	// StatusReverted is an operation somebody undid, alone or with the rest of
	// its session. It stops answering.
	StatusReverted Status = "reverted"
)

// Statuses is every status a subject-bearing operation can hold.
var Statuses = []Status{StatusSuggested, StatusEstablished, StatusContested, StatusWithdrawn, StatusDropped, StatusReverted}

// Valid reports whether s is one of the declared statuses.
func (s Status) Valid() bool { return slices.Contains(Statuses, s) }

// Answers reports whether a subject at this status still says anything: a
// suggestion and a contested rule advise, an established rule binds, and the
// other three are silent.
func (s Status) Answers() bool {
	return s == StatusSuggested || s == StatusEstablished || s == StatusContested
}

// Advises reports whether a subject at this status advises without binding.
func (s Status) Advises() bool {
	return s == StatusSuggested || s == StatusContested
}

// SubjectKind names what an operation is about.
type SubjectKind string

const (
	// SubjectNone is an operation that acts on another rather than carrying a
	// subject of its own.
	SubjectNone SubjectKind = ""
	// SubjectTerm is a term rule: the form to avoid (and its other forms), what
	// to write instead, how hard it bites.
	SubjectTerm SubjectKind = "term"
	// SubjectMemory is a source and target pair for the content memory.
	SubjectMemory SubjectKind = "memory"
	// SubjectNote is prose: a fact worth recording that states no rule.
	SubjectNote SubjectKind = "note"
)

// MemoryPair is a source and its translation, for the content memory.
type MemoryPair struct {
	Source       string `json:"source"`
	Target       string `json:"target"`
	SourceLocale string `json:"source_locale,omitempty"`
	TargetLocale string `json:"target_locale"`
}

// Subject is what an operation is about. Exactly one field is set, and Kind
// says which.
type Subject struct {
	Kind   SubjectKind       `json:"kind,omitempty"`
	Term   *profile.TermRule `json:"term,omitempty"`
	Memory *MemoryPair       `json:"memory,omitempty"`
	// Text is the prose of a note, or what the observer said about a term rule
	// in their own words.
	Text string `json:"text,omitempty"`
}

// Rule returns the term rule this subject states, and whether it states one. A
// content-memory pair and a note answer with nothing.
func (s Subject) Rule() (profile.TermRule, bool) {
	if s.Kind == SubjectTerm && s.Term != nil {
		return *s.Term, true
	}
	return profile.TermRule{}, false
}

// Describe renders the subject as the one line a log prints for it.
func (s Subject) Describe() string {
	if s.Kind == SubjectNone {
		return ""
	}
	if phrase := s.Phrase(); phrase != "" {
		return string(s.Kind) + " " + phrase
	}
	return string(s.Kind)
}

// Phrase renders what the subject says without naming its kind, for a line
// that names the kind elsewhere: `"Quickcast", not "Quick cast"` for a term
// rule, the quoted prose of a note.
func (s Subject) Phrase() string {
	switch s.Kind {
	case SubjectTerm:
		rule, ok := s.Rule()
		if !ok {
			return ""
		}
		if rule.Replacement == "" {
			return strconv.Quote(rule.Term)
		}
		avoid := make([]string, 0, 1+len(rule.Forms))
		for _, form := range append([]string{rule.Term}, rule.Forms...) {
			avoid = append(avoid, strconv.Quote(form))
		}
		return fmt.Sprintf("%q, not %s", rule.Replacement, strings.Join(avoid, ", "))
	case SubjectMemory:
		if s.Memory == nil {
			return ""
		}
		return fmt.Sprintf("%q into %s", s.Memory.Source, s.Memory.TargetLocale)
	case SubjectNote:
		return strconv.Quote(s.Text)
	}
	return ""
}

// Correction is the wording someone changed, carried by a correct operation.
type Correction struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// Evidence is where the subject was seen: a file in the project, a unit inside
// it, and the wording as it stood. A rule with no evidence behind it is a
// preference somebody typed; a rule with evidence can be argued with.
type Evidence struct {
	// Path is project-relative and slash-separated.
	Path string `json:"path,omitempty"`
	// Unit is the block or unit key inside the file.
	Unit string `json:"unit,omitempty"`
	// Quote is the text the subject was seen in.
	Quote string `json:"quote,omitempty"`
}

// Basis is the governance in force when the operation was recorded. A rule
// suggested under one voice profile and kept under another is a rule that
// deserves a second look, and the basis is what makes that visible.
type Basis struct {
	// Profile and Channel are the point the project resolved.
	Profile string `json:"profile,omitempty"`
	Channel string `json:"channel,omitempty"`
	// ProfileID and ProfileVersion identify the voice profile that governed.
	ProfileID      string `json:"profile_id,omitempty"`
	ProfileVersion string `json:"profile_version,omitempty"`
	// Fingerprint is the governing context fingerprint
	// (profile.GovernanceContext), so a staleness gate can tell whether the
	// basis still holds.
	Fingerprint string `json:"fingerprint,omitempty"`
}

// Level says how far a subject's answer reaches.
type Level string

const (
	// LevelProject keeps a rule in the project whose evidence produced it. It
	// is where everything starts.
	LevelProject Level = "project"
	// LevelWorkspace puts a rule in every project of the workspace. Only a
	// person widens to it.
	LevelWorkspace Level = "workspace"
)

// Scope is where a subject answers: how far it reaches, and the coordinates of
// the point its evidence was seen at.
//
// Widening to a broader point drops the axes the rule should stop being
// specific about, so a rule seen in one product's reference pages can be
// widened to the brand by dropping product and mode. Widening to the project
// drops the profile the evidence was seen under, and widening to the workspace
// drops the project too.
type Scope struct {
	Level Level `json:"level,omitempty"`
	// Coordinates are the axes of the point (project.MergeCoordinates), omitted
	// where the rule answers regardless of them.
	Coordinates map[string]string `json:"coordinates,omitempty"`
	// AllProfiles says a project-level rule holds under every profile of the
	// project, rather than under the profile its evidence was seen under
	// (Basis.Profile). A person sets it by widening the rule to the project.
	AllProfiles bool `json:"all_profiles,omitempty"`
}

// Covers reports whether a subject at this scope answers at the point given.
// A coordinate the scope names must match; an axis the scope leaves out is one
// the rule does not care about.
func (s Scope) Covers(point map[string]string) bool {
	for axis, want := range s.Coordinates {
		if point[axis] != want {
			return false
		}
	}
	return true
}

// Describe renders a scope for a log line.
func (s Scope) Describe() string {
	level := s.Level
	if level == "" {
		level = LevelProject
	}
	if s.AllProfiles && level == LevelProject {
		level += " (every profile)"
	}
	if len(s.Coordinates) == 0 {
		return string(level)
	}
	axes := make([]string, 0, len(s.Coordinates))
	for axis, value := range s.Coordinates {
		axes = append(axes, axis+"="+value)
	}
	sortStrings(axes)
	return string(level) + " " + strings.Join(axes, ",")
}

// Record is one operation as the log holds it.
type Record struct {
	// ID names the operation in every log that holds it (workspace.NewOpID):
	// time-ordered, so a merged log folds in one order on every machine.
	ID string `json:"id"`
	// Short is the id as a log line shows it and a person types it: its first
	// characters (workspace.ShortOpID). Every verb that takes an id resolves
	// an unambiguous prefix, so the short form is enough.
	Short string `json:"short,omitempty"`
	// Seq is the operation's position in the folded log, for ordering inside
	// one reading. It is not stored and not stable across readings.
	Seq int64 `json:"-"`
	// Project is the project whose work produced the operation. A widened rule
	// keeps it, because provenance survives widening.
	Project workspace.ProjectKey `json:"project,omitempty"`
	// Actor is who did it.
	Actor Actor `json:"actor"`
	// Kind is what was done.
	Kind Kind `json:"kind"`
	// Subject is what it was about, empty for an operation that acts on another.
	Subject Subject `json:"subject,omitzero"`
	// Correction is the wording a correct operation changed.
	Correction *Correction `json:"correction,omitempty"`
	// Evidence is where the subject was seen.
	Evidence []Evidence `json:"evidence,omitempty"`
	// Basis is what governed when it was recorded.
	Basis Basis `json:"basis,omitzero"`
	// Scope is how far the subject answers.
	Scope Scope `json:"scope,omitzero"`
	// Target is the operation this one acts on, empty for a subject-bearing
	// operation.
	Target string `json:"target,omitempty"`
	// TargetSession is the session a revert undoes, for a revert that names one
	// rather than a single operation.
	TargetSession string `json:"target_session,omitempty"`
	// Note is whatever the actor said about the operation.
	Note string `json:"note,omitempty"`
	// At is Go's clock at the moment the log accepted it, in UTC.
	At time.Time `json:"at"`
	// Status is what became of the operation, folded from the operations that
	// name it. An operation that acts on another carries StatusEstablished
	// unless something reverted it.
	Status Status `json:"status"`
	// ContestedBy names the operations on the other side of a disagreement,
	// for a record at StatusContested.
	ContestedBy []string `json:"contested_by,omitempty"`
	// Established reports that the subject is established: a person kept it,
	// imported it or wrote it, or settling established it on evidence that
	// includes a person's signal. It stays true while a correction contests
	// the rule, which is what tells a contested rule from a contested
	// suggestion.
	Established bool `json:"established,omitempty"`
	// Signal is the evidence a signal operation carries.
	Signal *Signal `json:"signal,omitempty"`
	// Because names the operations an establish operation rested on.
	Because []string `json:"because,omitempty"`
	// Standing counts the evidence for and against a suggestion, folded from
	// the log. It is set on a term rule that settling considers.
	Standing *Standing `json:"standing,omitempty"`

	// pending names the person signals a suggestion would be established on,
	// when the log supports establishing it and no establish operation has
	// recorded that yet. Ledger.Settle records it.
	pending []string
}

// Rule is the term rule this operation states, with the scope it answers at,
// and whether it states one.
func (r Record) Rule() (profile.TermRule, bool) { return r.Subject.Rule() }

// payload is the part of a Record that is written into the workspace log. The
// id, the sequence, the project, the timestamp and the folded status are the
// log's own and are not repeated here.
type payload struct {
	Actor         Actor       `json:"actor"`
	Subject       Subject     `json:"subject,omitzero"`
	Correction    *Correction `json:"correction,omitempty"`
	Evidence      []Evidence  `json:"evidence,omitempty"`
	Basis         Basis       `json:"basis,omitzero"`
	Scope         Scope       `json:"scope,omitzero"`
	Target        string      `json:"target,omitempty"`
	TargetSession string      `json:"target_session,omitempty"`
	Note          string      `json:"note,omitempty"`
	Signal        *Signal     `json:"signal,omitempty"`
	Because       []string    `json:"because,omitempty"`
}

// encode renders a record as the workspace operation that carries it.
func encode(r Record) (workspace.Op, error) {
	body, err := json.Marshal(payload{
		Actor:         r.Actor,
		Subject:       r.Subject,
		Correction:    r.Correction,
		Evidence:      r.Evidence,
		Basis:         r.Basis,
		Scope:         r.Scope,
		Target:        r.Target,
		TargetSession: r.TargetSession,
		Note:          r.Note,
		Signal:        r.Signal,
		Because:       r.Because,
	})
	if err != nil {
		return workspace.Op{}, fmt.Errorf("contextop: encode %s operation: %w", r.Kind, err)
	}
	return workspace.Op{
		Address: address(r),
		Project: r.Project,
		Kind:    OpKindPrefix + string(r.Kind),
		Payload: body,
		At:      r.At,
	}, nil
}

// decode reads a workspace operation back as a record. ok is false for an
// operation this package did not write.
func decode(op workspace.Op) (Record, bool, error) {
	kind, found := strings.CutPrefix(op.Kind, OpKindPrefix)
	if !found {
		return Record{}, false, nil
	}
	var body payload
	if len(op.Payload) > 0 {
		if err := json.Unmarshal(op.Payload, &body); err != nil {
			return Record{}, false, fmt.Errorf("contextop: read operation %s: %w", op.ID, err)
		}
	}
	return Record{
		ID:            op.ID,
		Short:         workspace.ShortOpID(op.ID),
		Project:       op.Project,
		Actor:         body.Actor,
		Kind:          Kind(kind),
		Subject:       body.Subject,
		Correction:    body.Correction,
		Evidence:      body.Evidence,
		Basis:         body.Basis,
		Scope:         body.Scope,
		Target:        body.Target,
		TargetSession: body.TargetSession,
		Note:          body.Note,
		Signal:        body.Signal,
		Because:       body.Because,
		At:            op.At.UTC(),
	}, true, nil
}

// ShortID renders an operation id the way a log line shows it.
func ShortID(id string) string { return workspace.ShortOpID(id) }

// ShortIDs renders operation ids the way a log line shows them.
func ShortIDs(ids []string) []string {
	out := make([]string, len(ids))
	for i, id := range ids {
		out[i] = workspace.ShortOpID(id)
	}
	return out
}

// sortStrings sorts in place without pulling the sort package into a hot path
// that only ever orders a handful of axis names.
func sortStrings(in []string) {
	for i := 1; i < len(in); i++ {
		for j := i; j > 0 && in[j] < in[j-1]; j-- {
			in[j], in[j-1] = in[j-1], in[j]
		}
	}
}
