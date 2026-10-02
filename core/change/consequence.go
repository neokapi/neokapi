package change

import (
	"time"

	"github.com/neokapi/neokapi/core/model"
)

// Role is an edition's place in its block.
type Role string

const (
	// RoleAuthoritative is the edition every other edition is derived from:
	// the source.
	RoleAuthoritative Role = "authoritative"
	// RoleDerived is any other edition: a translation, or a variant such as a
	// channel edition.
	RoleDerived Role = "derived"
)

// Consequence is an edition's status and origin after an edit.
type Consequence struct {
	Status model.Status
	Origin model.Origin
}

// Consequences returns what an applied content change does to an edition's
// status and origin. before is the edition as it stood, the zero Edition for
// one the change creates. It is the one place this is decided, for every
// surface:
//
//   - Anyone who changes the authoritative edition drops it to written: a
//     source approval binds the wording it approved. Its origin stays. The
//     derived editions now read stale through their basis, which the result
//     lists in Invalidates.
//   - A person who changes a derived edition makes it translated, with a human
//     origin; an established edition drops to translated.
//   - An agent that changes a derived edition makes it translated, with an
//     agent origin naming the agent (Engine) and its session (Reference). An
//     agent never records a decision.
//   - A tool in a flow leaves status and origin as they were, a new edition
//     with none: the tool records what it produced with the provenance
//     operation (a draft, with the tool's origin), which it sends after the
//     content.
//
// A review decision is not an edit; decide records it.
func Consequences(actor Actor, role Role, before model.Edition, now time.Time) Consequence {
	if role == RoleAuthoritative {
		status := before.Status
		if status == model.Status(model.SourceStatusEstablished) {
			status = model.Status(model.SourceStatusWritten)
		}
		return Consequence{Status: status, Origin: before.Origin}
	}
	switch actor.Kind {
	case ActorPerson:
		return Consequence{
			Status: model.Status(model.TargetStatusTranslated),
			Origin: model.Origin{Kind: model.OriginHuman, Timestamp: now.UTC().Format(time.RFC3339)},
		}
	case ActorAgent:
		return Consequence{
			Status: model.Status(model.TargetStatusTranslated),
			Origin: model.Origin{Kind: model.OriginAgent, Engine: actor.Name, Reference: actor.Session, Timestamp: now.UTC().Format(time.RFC3339)},
		}
	default:
		return Consequence{Status: before.Status, Origin: before.Origin}
	}
}
