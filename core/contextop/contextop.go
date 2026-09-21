// Package contextop records how a project's context came to be.
//
// A project starts with no terms, no voice rules and an empty content memory,
// and the context grows out of ordinary work. Someone notices a fact, proposes
// a rule with the evidence behind it, a person confirms it, checks enforce it,
// and the next correction is evidence for the rule after that. Every step is an
// operation: who did it, what it was about, where the evidence was seen, what
// governed at the time, and what became of it.
//
// # Append only
//
// Operations go into the workspace's operation log (core/workspace) and are
// never edited. A status is not a column that moves: confirming a proposal
// writes a `confirm` operation naming it, discarding writes a `discard`, and
// reverting writes a `revert`. [Ledger.Records] reads the log and reports the status each
// subject-bearing operation ended up with, so the history is the whole record
// and a surface can replay it.
//
// # Candidates advise, confirmed rules bind
//
// A proposal takes effect the moment it is recorded, as advice. [Resolve] and
// [Resolution.RuleSets] project the candidates at a point into rule sets marked
// advisory, which the
// vocabulary matcher reports at neutral severity, below every gate threshold. A
// candidate therefore shows up in a check and can never fail one. Confirming is
// what makes a rule bind, at the severity the rule carries, and confirmation
// writes it into the terms store, the voice profile or the content memory,
// where every existing reader already looks. This log is the history and the
// source of candidates, not a second home for confirmed rules.
//
// # Scope
//
// What is learned is scoped to the point where its evidence was seen, which for
// a new project is that project. Confirming is also the moment a person may
// widen a rule to a broader point, or to the whole workspace, where it answers
// in every project. [Resolve] puts workspace-wide rules beneath project ones,
// so the most specific rule about a term wins.
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
	// KindObserve records a fact somebody noticed, with no rule implied: a
	// product name as it is written, a spelling the documents keep, who the
	// text addresses.
	KindObserve Kind = "observe"
	// KindPropose records a candidate term rule or voice rule with the evidence
	// behind it. It advises from the moment it is recorded.
	KindPropose Kind = "propose"
	// KindCorrect records that someone changed wording from one form to another
	// at a location. It carries both wordings, and it may carry a proposal
	// drawn from them.
	KindCorrect Kind = "correct"
	// KindConfirm makes an earlier operation's rule binding, optionally with
	// edits to the rule.
	KindConfirm Kind = "confirm"
	// KindDiscard rejects an earlier operation. Its subject stops answering.
	KindDiscard Kind = "discard"
	// KindRevert undoes an earlier operation, or every operation one session
	// recorded, and retracts whatever they put in force.
	KindRevert Kind = "revert"
	// KindWiden moves a confirmed rule to a broader point, or to the whole
	// workspace.
	KindWiden Kind = "widen"
)

// Kinds is every operation kind, in the order a reader meets them.
var Kinds = []Kind{KindObserve, KindPropose, KindCorrect, KindConfirm, KindDiscard, KindRevert, KindWiden}

// Valid reports whether k is one of the declared kinds.
func (k Kind) Valid() bool { return slices.Contains(Kinds, k) }

// Bears reports whether an operation of this kind carries a subject of its own.
// The three that do are what a status is folded onto; the rest act on them.
func (k Kind) Bears() bool {
	return k == KindObserve || k == KindPropose || k == KindCorrect
}

// Status is what became of a subject-bearing operation. It is folded from the
// log rather than stored, because the log is never edited.
type Status string

const (
	// StatusCandidate is an operation nothing has acted on. Its rule advises
	// and never fails a check.
	StatusCandidate Status = "candidate"
	// StatusConfirmed is a rule a person made binding. It is in force at the
	// severity it carries.
	StatusConfirmed Status = "confirmed"
	// StatusDiscarded is a proposal somebody rejected. It stops answering.
	StatusDiscarded Status = "discarded"
	// StatusReverted is an operation somebody undid, alone or with the rest of
	// its session. It stops answering.
	StatusReverted Status = "reverted"
)

// Statuses is every status a subject-bearing operation can hold.
var Statuses = []Status{StatusCandidate, StatusConfirmed, StatusDiscarded, StatusReverted}

// Valid reports whether s is one of the declared statuses.
func (s Status) Valid() bool { return slices.Contains(Statuses, s) }

// Answers reports whether a subject at this status still says anything: a
// candidate advises, a confirmed rule binds, and the other two are silent.
func (s Status) Answers() bool {
	return s == StatusCandidate || s == StatusConfirmed
}

// SubjectKind names what an operation is about.
type SubjectKind string

const (
	// SubjectNone is an operation that acts on another rather than carrying a
	// subject of its own.
	SubjectNone SubjectKind = ""
	// SubjectTerm is a term rule: one word, what to write instead, how hard it
	// bites.
	SubjectTerm SubjectKind = "term"
	// SubjectVoice is a rule in a voice profile's vocabulary.
	SubjectVoice SubjectKind = "voice"
	// SubjectMemory is a source and target pair for the content memory.
	SubjectMemory SubjectKind = "memory"
	// SubjectNote is prose: a fact worth recording that states no rule.
	SubjectNote SubjectKind = "note"
)

// VoiceRule is one vocabulary rule of a voice profile: the list it sits in and
// the rule itself. The lists are the ones `kapi apply` writes, so a confirmed
// rule reaches the profile through the path a person's own edit takes.
type VoiceRule struct {
	// List is "forbidden", "competitor" or "preferred".
	List string `json:"list"`
	// Rule is the constraint on the word.
	Rule profile.TermRule `json:"rule"`
}

// VoiceLists are the vocabulary lists a voice rule may name.
var VoiceLists = []string{"forbidden", "competitor", "preferred"}

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
	Voice  *VoiceRule        `json:"voice,omitempty"`
	Memory *MemoryPair       `json:"memory,omitempty"`
	// Text is the prose of a note.
	Text string `json:"text,omitempty"`
}

// Rule returns the term rule this subject states, and whether it states one. A
// voice rule and a term rule are the same constraint written in two places, so
// both answer; a content-memory pair and a note answer with nothing.
func (s Subject) Rule() (profile.TermRule, bool) {
	switch {
	case s.Kind == SubjectTerm && s.Term != nil:
		return *s.Term, true
	case s.Kind == SubjectVoice && s.Voice != nil:
		return s.Voice.Rule, true
	}
	return profile.TermRule{}, false
}

// Describe renders the subject as the one line a log prints for it.
func (s Subject) Describe() string {
	switch s.Kind {
	case SubjectTerm, SubjectVoice:
		rule, ok := s.Rule()
		if !ok {
			return string(s.Kind)
		}
		if rule.Replacement != "" {
			return fmt.Sprintf("%s %q, use %q", s.Kind, rule.Term, rule.Replacement)
		}
		return fmt.Sprintf("%s %q", s.Kind, rule.Term)
	case SubjectMemory:
		if s.Memory == nil {
			return string(s.Kind)
		}
		return fmt.Sprintf("memory %q into %s", s.Memory.Source, s.Memory.TargetLocale)
	case SubjectNote:
		return "note " + strconv.Quote(s.Text)
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
// proposed under one voice profile and confirmed under another is a rule that
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
// widened to the brand by dropping product and mode. Widening to the workspace
// drops the project too.
type Scope struct {
	Level Level `json:"level,omitempty"`
	// Coordinates are the axes of the point (project.MergeCoordinates), omitted
	// where the rule answers regardless of them.
	Coordinates map[string]string `json:"coordinates,omitempty"`
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
	// ID addresses the operation. It is the position the workspace log gave it,
	// which is what a person types at `kapi context confirm`.
	ID string `json:"id"`
	// Seq is that position as a number, for ordering.
	Seq int64 `json:"seq"`
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
	// name it. An operation that acts on another carries StatusConfirmed unless
	// something reverted it.
	Status Status `json:"status"`
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
	})
	if err != nil {
		return workspace.Op{}, fmt.Errorf("contextop: encode %s operation: %w", r.Kind, err)
	}
	return workspace.Op{
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
			return Record{}, false, fmt.Errorf("contextop: read operation %d: %w", op.Seq, err)
		}
	}
	return Record{
		ID:            FormatID(op.Seq),
		Seq:           op.Seq,
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
		At:            op.At.UTC(),
	}, true, nil
}

// FormatID renders a log position as the id a person types.
func FormatID(seq int64) string { return strconv.FormatInt(seq, 10) }

// ParseID reads an id back as a log position. A leading "#" is accepted,
// because that is how a log line reads aloud.
func ParseID(id string) (int64, error) {
	trimmed := strings.TrimPrefix(strings.TrimSpace(id), "#")
	seq, err := strconv.ParseInt(trimmed, 10, 64)
	if err != nil || seq <= 0 {
		return 0, fmt.Errorf("contextop: %q is not an operation id", id)
	}
	return seq, nil
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
