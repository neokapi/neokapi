package server

import (
	"encoding/json"
	"slices"
	"strings"

	"github.com/labstack/echo/v4"

	platev "github.com/neokapi/neokapi/bowrain/core/event"
	"github.com/neokapi/neokapi/core/contextop"
	"github.com/neokapi/neokapi/core/workspace"
)

// The sources a context change names (platev.ContextChange.Source).
const (
	contextSourcePush   = "push"
	contextSourceDecide = "decide"
	contextSourceVoice  = "voice"
)

// contextResourceType is the audit resource type of a context change.
const contextResourceType = "context"

// contextOpOperations maps each ledger operation kind to the operation a
// context change counts it under. A signal and a check record evidence about
// other operations and change no item, so they are absent and go uncounted.
var contextOpOperations = map[contextop.Kind]platev.ContextOperation{
	contextop.KindObserve:   platev.ContextOpAdd,
	contextop.KindImport:    platev.ContextOpAdd,
	contextop.KindCorrect:   platev.ContextOpEdit,
	contextop.KindEdit:      platev.ContextOpEdit,
	contextop.KindWiden:     platev.ContextOpEdit,
	contextop.KindKeep:      platev.ContextOpKeep,
	contextop.KindEstablish: platev.ContextOpKeep,
	contextop.KindDrop:      platev.ContextOpDrop,
	contextop.KindWithdraw:  platev.ContextOpRetire,
	contextop.KindReset:     platev.ContextOpRetire,
}

// contextItemKinds maps a ledger subject kind to the item kind a context
// change counts it under. An operation whose subject cannot be read is a
// decision.
var contextItemKinds = map[contextop.SubjectKind]platev.ContextItemKind{
	contextop.SubjectTerm:   platev.ContextItemTermRule,
	contextop.SubjectNote:   platev.ContextItemNote,
	contextop.SubjectMemory: platev.ContextItemMemory,
}

// ledgerOpHead is the part of a context operation's payload a context change
// reads.
type ledgerOpHead struct {
	Actor contextop.Actor `json:"actor"`
	Voice json.RawMessage `json:"voice"`
}

// subjectResolver returns the subject kind a context operation is about,
// following the operation it acts on, and false when that cannot be read.
type subjectResolver func(op workspace.Op, kind contextop.Kind) (contextop.SubjectKind, bool)

// countLedgerOps counts the context operations of ops by item kind and
// operation, and returns the actors that recorded them. Operations outside
// the context vocabulary are skipped.
func countLedgerOps(ops []workspace.Op, resolve subjectResolver) (platev.ContextCounts, []contextop.Actor) {
	counts := platev.ContextCounts{}
	var actors []contextop.Actor
	for _, op := range ops {
		name, ok := strings.CutPrefix(op.Kind, contextop.OpKindPrefix)
		if !ok {
			continue
		}
		kind := contextop.Kind(name)
		operation, ok := contextOpOperations[kind]
		if !ok {
			continue
		}
		var head ledgerOpHead
		if len(op.Payload) > 0 {
			_ = json.Unmarshal(op.Payload, &head)
		}
		item := platev.ContextItemDecision
		if len(head.Voice) > 0 && string(head.Voice) != "null" {
			item = platev.ContextItemVoiceProfile
		} else if subject, ok := resolve(op, kind); ok {
			if mapped, ok := contextItemKinds[subject]; ok {
				item = mapped
			}
		}
		counts.Add(item, operation, 1)
		if !slices.Contains(actors, head.Actor) {
			actors = append(actors, head.Actor)
		}
	}
	return counts, actors
}

// ledgerOpSubject is the part of a context operation's payload a sample
// reads: the subject it states, or the operation it acts on.
type ledgerOpSubject struct {
	Subject contextop.Subject `json:"subject"`
	Target  string            `json:"target"`
}

// sampleLedgerOps adds to change the first few term rules and notes ops
// touch. An operation that acts on another (a keep, a drop) is sampled with
// the subject of the one it names, which lookup returns.
func sampleLedgerOps(change *platev.ContextChange, ops []workspace.Op, lookup func(id string) (contextop.Subject, bool)) {
	for _, op := range ops {
		if len(change.Samples) >= platev.MaxContextSamples {
			return
		}
		name, ok := strings.CutPrefix(op.Kind, contextop.OpKindPrefix)
		if !ok {
			continue
		}
		operation, ok := contextOpOperations[contextop.Kind(name)]
		if !ok {
			continue
		}
		var body ledgerOpSubject
		if len(op.Payload) > 0 {
			_ = json.Unmarshal(op.Payload, &body)
		}
		subject := body.Subject
		if subject.Kind == contextop.SubjectNone && body.Target != "" && lookup != nil {
			subject, _ = lookup(body.Target)
		}
		change.AddSample(contextSample(subject, operation))
	}
}

// contextSample is the sample a ledger subject gives: a term rule's term and
// replacement, or a note's text. Other subjects give an empty sample, which
// AddSample skips.
func contextSample(subject contextop.Subject, operation platev.ContextOperation) platev.ContextSample {
	switch subject.Kind {
	case contextop.SubjectTerm:
		if rule, ok := subject.Rule(); ok {
			return platev.ContextSample{
				Kind: platev.ContextItemTermRule, Operation: operation,
				Term: rule.Term, Replacement: rule.Replacement,
			}
		}
	case contextop.SubjectNote:
		return platev.ContextSample{Kind: platev.ContextItemNote, Operation: operation, Text: subject.Text}
	}
	return platev.ContextSample{}
}

// pushedActor is the actor kind and session a push of operations is
// attributed to. Operations that share one kind name it; a push mixing kinds
// that holds an agent's operations is an agent's, so a reviewer sees an
// agent touched it. The session is named when exactly one is present.
func pushedActor(actors []contextop.Actor) (platev.ContextActorKind, string) {
	kind := platev.ContextActorPerson
	var sessions []string
	for i, a := range actors {
		k := platev.ContextActorKind(a.Kind)
		if k == "" {
			k = platev.ContextActorPerson
		}
		switch {
		case i == 0:
			kind = k
		case k == platev.ContextActorAgent:
			kind = k
		}
		if a.Session != "" && !slices.Contains(sessions, a.Session) {
			sessions = append(sessions, a.Session)
		}
	}
	if len(sessions) == 1 {
		return kind, sessions[0]
	}
	return kind, ""
}

// requestSession is the agent session a request carries, when it carries
// one, and the actor kind the request acts as.
func requestSession(c echo.Context) (platev.ContextActorKind, string) {
	if session, _ := c.Get("bravo_session_id").(string); session != "" {
		return platev.ContextActorAgent, session
	}
	return platev.ContextActorPerson, ""
}

// emitContextChange publishes a context change made through the request,
// attributed to the signed-in member and recorded by the audit logger and the
// activity recorder. A change that counts no item is not published.
func (s *Server) emitContextChange(c echo.Context, evType platev.EventType, projectID, resourceID string, change platev.ContextChange) {
	if change.Counts.Total() == 0 {
		return
	}
	if change.WorkspaceSlug == "" {
		change.WorkspaceSlug = c.Param("ws")
	}
	if change.ActorName == "" {
		change.ActorName, _ = c.Get("name").(string)
	}
	if resourceID == "" {
		resourceID = projectID
	}
	s.emitAudit(c, auditEvent{
		Type:         evType,
		ProjectID:    projectID,
		ResourceType: contextResourceType,
		ResourceID:   resourceID,
		Data:         change.Data(),
	})
}

// emitPushedContext publishes the context change a push of context objects
// made: the operations the project did not hold, and the binding when the
// push bound the project's context to its key.
func (s *Server) emitPushedContext(c echo.Context, projectID string, accepted acceptedContext) {
	if s.EventBus == nil {
		return
	}
	counts, actors := countLedgerOps(accepted.fresh, func(op workspace.Op, kind contextop.Kind) (contextop.SubjectKind, bool) {
		subject, err := subjectKindOf(op, kind, accepted.byID)
		return subject, err == nil
	})
	if accepted.bound {
		counts.Add(platev.ContextItemBinding, platev.ContextOpAdd, 1)
	}
	kind, session := pushedActor(actors)
	change := platev.ContextChange{
		ActorKind: kind,
		Session:   session,
		Source:    contextSourcePush,
		Counts:    counts,
	}
	if len(accepted.fresh) == 1 {
		change.ItemID = accepted.fresh[0].ID
	}
	sampleLedgerOps(&change, accepted.fresh, func(id string) (contextop.Subject, bool) {
		target, ok := accepted.byID[id]
		if !ok {
			return contextop.Subject{}, false
		}
		var body ledgerOpSubject
		_ = json.Unmarshal(target.Payload, &body)
		return body.Subject, true
	})
	s.emitContextChange(c, platev.EventContextRecorded, projectID, "", change)
}

// emitDecidedContext publishes the context change one Decide answer made.
// The item is the suggestion the answer was given to.
func (s *Server) emitDecidedContext(c echo.Context, st *decideState, answered string, recorded []workspace.Op) {
	if s.EventBus == nil {
		return
	}
	counts, _ := countLedgerOps(recorded, func(op workspace.Op, kind contextop.Kind) (contextop.SubjectKind, bool) {
		own, target := subjectOf(op)
		if own != "" || target == "" {
			return own, true
		}
		r, ok := st.byID[target]
		return r.Subject.Kind, ok
	})
	kind, session := requestSession(c)
	change := platev.ContextChange{
		ActorKind: kind,
		ActorName: st.reader.Name,
		Session:   session,
		Source:    contextSourceDecide,
		ItemID:    answered,
		Counts:    counts,
	}
	sampleLedgerOps(&change, recorded, func(id string) (contextop.Subject, bool) {
		r, ok := st.byID[id]
		return r.Subject, ok
	})
	s.emitContextChange(c, platev.EventContextRecorded, st.project.ID, answered, change)
}

// voiceRuleChange is the context change a decision about one voice rule
// makes: the rule's term is its id, and the profile it was decided for rides
// in the event's detail.
func voiceRuleChange(c echo.Context, actor platev.ContextActorKind, op platev.ContextOperation, term string) platev.ContextChange {
	_, session := requestSession(c)
	name, _ := c.Get("name").(string)
	counts := platev.ContextCounts{}
	counts.Add(platev.ContextItemVoiceRule, op, 1)
	return platev.ContextChange{
		WorkspaceSlug: c.Param("ws"),
		ActorKind:     actor,
		ActorName:     name,
		Session:       session,
		Source:        contextSourceVoice,
		ItemID:        term,
		Counts:        counts,
		Samples:       []platev.ContextSample{{Kind: platev.ContextItemVoiceRule, Operation: op, Term: term}},
	}
}

// voiceProfileChange is the context change an edit of a whole voice profile
// makes.
func voiceProfileChange(c echo.Context, profileID string) platev.ContextChange {
	actor, session := requestSession(c)
	counts := platev.ContextCounts{}
	counts.Add(platev.ContextItemVoiceProfile, platev.ContextOpEdit, 1)
	return platev.ContextChange{
		WorkspaceSlug: c.Param("ws"),
		ActorKind:     actor,
		Session:       session,
		Source:        contextSourceVoice,
		ItemID:        profileID,
		Counts:        counts,
	}
}

// voiceProfileData is the event detail of an edit of a whole voice profile:
// detail with the context change the edit makes written beside it.
func voiceProfileData(c echo.Context, profileID string, detail map[string]string) map[string]string {
	change := voiceProfileChange(c, profileID)
	change.Extra = detail
	return change.Data()
}
