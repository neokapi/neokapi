package server

import (
	"context"
	"fmt"

	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	"github.com/neokapi/neokapi/bowrain/core/store"
	bstore "github.com/neokapi/neokapi/bowrain/store"
	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/venue"
)

// changeSender is who sends a change set to a stream, as the server knows
// them: the user and what their permissions allow, per language for the
// language-scoped permissions. A nil allows is the server itself, running a
// job a person already started.
type changeSender struct {
	userID string
	name   string
	allows func(perm platauth.Permission, locale string) bool
}

// streamPolicy is the server's change.Policy: the role permissions the sender
// holds (RBAC, with the language scope), the access state of each block
// (ABAC), and the rules every surface holds an agent to. The kapi host holds a
// change set to the same rules for an agent; the permissions are the server's.
//
//   - A translation needs translate for its language, the source needs edit
//     source, and a source a connector syncs is edited at the connector.
//   - A block held as restricted takes an edit from its owner or a reviewer of
//     the language; a published block from a project manager.
//   - decide establish needs review for the language; reject and withdraw
//     need translate, and moving an established translation needs review
//     (checked when the decision is prepared, where the status is known).
//   - A note sits on a block's own edition. Anyone who may read the content
//     adds one; its author or a project manager rewrites or removes it. An
//     entity is marked with edit source.
//   - An agent sends neither gate report, if_match "*", nor a decision other
//     than advise, which records a pre-review and takes translate for the
//     language. A pre-review is an agent's: the review queue shows it as AI
//     advice, so a person sends establish, reject or withdraw instead. Only a
//     tool records provenance, and a tool decides nothing.
type streamPolicy struct {
	ctx    context.Context //nolint:containedctx // the policy answers within one request
	s      *Server
	proj   *store.Project
	stream string
	wsID   string
	sender changeSender
	rows   *rowLookup
	synced map[string]*change.Error
}

var _ change.Policy = (*streamPolicy)(nil)

func (p *streamPolicy) Permit(actor change.Actor, set *change.Set, op change.Op) *change.Error {
	switch actor.Kind {
	case change.ActorPerson, change.ActorAgent, change.ActorTool:
	default:
		return &change.Error{Code: change.CodeNotPermitted,
			Message: fmt.Sprintf("a sender of kind %q may not send a change set to a stream", actor.Kind)}
	}
	decide, _ := op.Body.(*change.Decide)
	if actor.Kind == change.ActorAgent {
		switch {
		case set != nil && set.Gate == change.GateReport:
			return notPermittedTo("an agent", "choose gate report",
				"an edit lands over the findings it introduces only when a person overrides them; fix the wording, or ask a person")
		case blindWrite(op):
			return notPermittedTo("an agent", `send if_match "*"`,
				"an agent writes over the revision it read; read the edition and send its revision")
		case decide != nil && decide.Outcome != change.OutcomeAdvise:
			return notPermittedTo("an agent", "decide "+string(decide.Outcome),
				"a review decision is a person's; record a pre-review with outcome advise")
		}
	}
	if actor.Kind == change.ActorPerson && decide != nil && decide.Outcome == change.OutcomeAdvise {
		return notPermittedTo("a person", "decide advise",
			"a pre-review is an agent's, and the review queue shows it as AI advice; establish or reject the translation")
	}
	if op.Kind == change.KindProvenance && actor.Kind != change.ActorTool {
		return notPermittedTo("a "+string(actor.Kind), "record provenance", "only a tool says how it produced an edition")
	}
	if actor.Kind == change.ActorTool {
		if decide != nil {
			return notPermittedTo("a tool", "decide", "a review decision is a person's")
		}
		// The server's own job: the request that started it was authorized.
		return nil
	}
	switch op.Kind {
	case change.KindTerm, change.KindMemory, change.KindRecipe:
		// A stream applies none of them; the service refuses them as
		// unsupported.
		return nil
	case change.KindDecide:
		locale, _ := p.editionLocale(op.At.Edition)
		if decide != nil && decide.Outcome == change.OutcomeEstablish {
			return p.need(platauth.PermReview, locale, "establish a translation")
		}
		return p.need(platauth.PermTranslate, locale, "decide on a translation")
	case change.KindAnnotate, change.KindUnannotate:
		switch annotationType(op) {
		case noteAnnotation:
			if _, source := p.editionLocale(op.At.Edition); !source {
				return &change.Error{Code: change.CodeNotPermitted, Field: "at/edition",
					Message: "a note is written on the block's own edition; a stream keeps no note on a translation"}
			}
			if err := p.need(platauth.PermViewContent, "", "write a note"); err != nil {
				return err
			}
			return p.mayChangeNote(op)
		case string(model.OverlayEntity):
			return p.need(platauth.PermEditSource, "", "mark an entity")
		}
	}
	locale, source := p.editionLocale(op.At.Edition)
	if source {
		if err := p.need(platauth.PermEditSource, "", "edit the source"); err != nil {
			return err
		}
		if err := p.sourceSynced(op.At.Doc); err != nil {
			return err
		}
	} else if err := p.need(platauth.PermTranslate, locale, "translate into "+string(locale)); err != nil {
		return err
	}
	return p.access(op, locale)
}

// editionLocale is the language of edition k, and whether k is the source.
func (p *streamPolicy) editionLocale(k model.EditionKey) (model.LocaleID, bool) {
	src := p.proj.DefaultSourceLanguage
	if k.IsZero() || (k.Tone == "" && k.Channel == "" && model.NormalizeLocale(k.Locale) == model.NormalizeLocale(src)) {
		return src, true
	}
	return k.Locale, false
}

// need refuses the operation unless the sender holds perm, for locale when it
// is language-scoped.
func (p *streamPolicy) need(perm platauth.Permission, locale model.LocaleID, what string) *change.Error {
	if p.sender.allows == nil || p.sender.allows(perm, string(locale)) {
		return nil
	}
	return &change.Error{Code: change.CodeNotPermitted,
		Message: fmt.Sprintf("you may not %s: it takes the %s permission", what, perm.String())}
}

// access is the block's access state (ABAC): a restricted block takes an edit
// from its owner or a reviewer of the language, a published one from a
// project manager.
func (p *streamPolicy) access(op change.Op, locale model.LocaleID) *change.Error {
	as, ok := p.s.ContentStore.(store.BlockAccessStore)
	if !ok || op.At.Block == "" {
		return nil
	}
	row := p.rows.find(p.ctx, op.At.Doc, op.At.Block)
	if row == nil {
		return nil
	}
	state, owner, err := as.GetBlockAccess(p.ctx, p.proj.ID, p.stream, row.Block.ID)
	if err != nil {
		return nil
	}
	switch state {
	case bstore.BlockAccessPublished:
		return p.need(platauth.PermManageProject, "", "edit a published block")
	case bstore.BlockAccessRestricted:
		if owner != "" && owner == p.sender.userID {
			return nil
		}
		return p.need(platauth.PermReview, locale, "edit a restricted block")
	}
	return nil
}

// mayChangeNote lets anyone who may read the content add a note, and a note's
// author or a project manager rewrite or remove one that exists: an annotate
// naming the id of a note the block holds takes that note's place.
func (p *streamPolicy) mayChangeNote(op change.Op) *change.Error {
	var id, what string
	switch body := op.Body.(type) {
	case *change.Annotate:
		id, what = body.ID, "rewrite"
	case *change.Unannotate:
		id, what = body.ID, "remove"
	}
	if id == "" {
		return nil
	}
	row := p.rows.find(p.ctx, op.At.Doc, op.At.Block)
	if row == nil {
		return nil
	}
	span := row.Block.OverlaySpan(model.OverlayType(noteAnnotation), id)
	if span == nil {
		// An annotate with a new id adds a note; the service refuses an
		// unannotate that names none.
		return nil
	}
	if author := span.Props[notePropAuthorID]; author != "" && author == p.sender.userID {
		return nil
	}
	if err := p.need(platauth.PermManageProject, "", what+" another person's note"); err != nil {
		err.Message = "only the note's author or a project manager can " + what + " a note"
		return err
	}
	return nil
}

// sourceSynced refuses a source edit to an item a connector syncs: the source
// is edited at the connector and comes back with the next sync.
func (p *streamPolicy) sourceSynced(item string) *change.Error {
	if err, ok := p.synced[item]; ok {
		return err
	}
	coll := p.s.collectionForItem(p.ctx, p.proj.ID, p.stream, item)
	conn, projectSourced := p.s.projectSourceConnector(p.ctx, p.wsID, p.proj.ID)
	var refusal *change.Error
	if projectSourced || collectionConnected(coll) {
		refusal = &change.Error{Code: change.CodeNotPermitted, Message: sourceSyncedMessage(conn, projectSourced, coll)}
	}
	if p.synced == nil {
		p.synced = map[string]*change.Error{}
	}
	p.synced[item] = refusal
	return refusal
}

// notPermittedTo is a refusal naming who tried what, and why not.
func notPermittedTo(who, what, why string) *change.Error {
	return &change.Error{Code: change.CodeNotPermitted, Message: fmt.Sprintf("%s may not %s: %s", who, what, why)}
}

// blindWrite reports whether op names change.AnyRevision as the revision it
// read.
func blindWrite(op change.Op) bool {
	if op.IfMatch == change.AnyRevision {
		return true
	}
	if d, ok := op.Body.(*change.DeleteBlock); ok && d != nil {
		for _, rev := range d.IfMatch {
			if rev == change.AnyRevision {
				return true
			}
		}
	}
	return false
}

// annotationType is the type an annotate or unannotate names.
func annotationType(op change.Op) string {
	switch b := op.Body.(type) {
	case *change.Annotate:
		return b.Type
	case *change.Unannotate:
		return b.Type
	}
	return ""
}

// rowLookup reads the rows of a stream by item and block key, once each, for
// the decisions a policy and a review make about a block before the change
// set reads it.
type rowLookup struct {
	s       *Server
	project string
	stream  string
	rows    map[[2]string]*venue.StoredBlock
}

func newRowLookup(s *Server, projectID, stream string) *rowLookup {
	return &rowLookup{s: s, project: projectID, stream: stream, rows: map[[2]string]*venue.StoredBlock{}}
}

// find returns the row of item keyed key (its durable key, name or row id),
// or nil.
func (l *rowLookup) find(ctx context.Context, item, key string) *venue.StoredBlock {
	k := [2]string{item, key}
	if sb, ok := l.rows[k]; ok {
		return sb
	}
	var found *venue.StoredBlock
	if ws, ok := l.s.ContentStore.(store.BlockWriteStore); ok && item != "" && key != "" {
		if rows, err := ws.ItemBlocks(ctx, l.project, l.stream, item, []string{key}); err == nil && len(rows) > 0 {
			found = rows[0]
		}
	}
	l.rows[k] = found
	return found
}
