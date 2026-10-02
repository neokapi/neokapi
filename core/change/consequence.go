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
// status and origin. before is the edition as it stood; created says the
// change creates the edition, and before is then the zero Edition. It is the
// one place this is decided, for every surface:
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
//   - A tool in a flow that changes the wording of a derived edition makes it
//     a draft: nobody has read the wording the tool wrote, so no earlier
//     approval covers it. The origin stays when the edition records one, so a
//     translation a tool later rewrites (unredacted, recased) still names who
//     translated it; an edition that records none takes the tool's. An
//     edition a tool creates takes the tool's origin and no status. A tool
//     that produced the content records its own status and provenance with
//     the provenance operation, sent after the content.
//
// A review decision is not an edit; decide records it.
func Consequences(actor Actor, role Role, before model.Edition, created bool, now time.Time) Consequence {
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
		origin := before.Origin
		if origin == (model.Origin{}) {
			origin = model.Origin{Tool: actor.Name}
		}
		if created {
			return Consequence{Origin: origin}
		}
		return Consequence{Status: model.Status(model.TargetStatusDraft), Origin: origin}
	}
}
