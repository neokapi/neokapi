package projector

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	coreprofile "github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/core/projectdb"
	"github.com/neokapi/neokapi/core/workspace"
	"github.com/neokapi/neokapi/memory"
	"github.com/neokapi/neokapi/memory/kmb"
	"github.com/neokapi/neokapi/terms"
)

// step is one write a store call made, in the form an operation carries it.
// Exactly one group of fields is set, and the operation's kind says which
// subsystem the step belongs to.
type step struct {
	Stream string `json:"stream,omitempty"`

	// terms.write
	PutConcepts     []terms.Concept         `json:"put_concepts,omitempty"`
	DeleteConcepts  []string                `json:"delete_concepts,omitempty"`
	PutRelations    []terms.ConceptRelation `json:"put_relations,omitempty"`
	DeleteRelations []string                `json:"delete_relations,omitempty"`

	// memory.write. Bulk says the entries were written in one transaction,
	// with the search indexes rebuilt afterwards, which is how they are
	// applied again.
	PutEntries     []kmb.Entry         `json:"put_entries,omitempty"`
	Bulk           bool                `json:"bulk,omitempty"`
	DeleteEntries  []string            `json:"delete_entries,omitempty"`
	PutSessions    []kmb.ImportSession `json:"put_sessions,omitempty"`
	SessionCounts  map[string]int      `json:"session_counts,omitempty"`
	DeleteSessions []string            `json:"delete_sessions,omitempty"`

	// voice.write
	CreateProfiles []*coreprofile.VoiceProfile `json:"create_profiles,omitempty"`
	UpdateProfiles []*coreprofile.VoiceProfile `json:"update_profiles,omitempty"`
	DeleteProfiles []string                    `json:"delete_profiles,omitempty"`

	// rules.write
	WidenRules  []workspace.Rule `json:"widen_rules,omitempty"`
	NarrowRules []string         `json:"narrow_rules,omitempty"`
}

// stamp fills every timestamp the store would take from the clock with the
// operation's own instant, so applying the step again writes the same rows.
func (s *step) stamp(at time.Time) {
	for i := range s.PutConcepts {
		c := &s.PutConcepts[i]
		if c.CreatedAt.IsZero() {
			c.CreatedAt = at
		}
		if c.UpdatedAt.IsZero() {
			c.UpdatedAt = at
		}
	}
	for i := range s.PutRelations {
		if s.PutRelations[i].CreatedAt.IsZero() {
			s.PutRelations[i].CreatedAt = at
		}
	}
	stamp := at.UTC().Format(time.RFC3339Nano)
	for i := range s.PutEntries {
		e := &s.PutEntries[i]
		if e.Created == "" {
			e.Created = stamp
		}
		if e.Updated == "" {
			e.Updated = stamp
		}
		for j := range e.Origins {
			if e.Origins[j].AddedAt == "" {
				e.Origins[j].AddedAt = stamp
			}
		}
	}
	for i := range s.PutSessions {
		if s.PutSessions[i].ImportedAt == "" {
			s.PutSessions[i].ImportedAt = stamp
		}
	}
	for _, p := range s.CreateProfiles {
		if p.CreatedAt.IsZero() {
			p.CreatedAt = at
		}
		if p.UpdatedAt.IsZero() {
			p.UpdatedAt = at
		}
		if p.Version == 0 {
			p.Version = 1
		}
	}
	for i := range s.WidenRules {
		if s.WidenRules[i].At.IsZero() {
			s.WidenRules[i].At = at
		}
	}
}

// entries reads the step's content-memory entries back as the store's own.
func (s step) entries() []memory.Entry {
	if len(s.PutEntries) == 0 {
		return nil
	}
	return (&kmb.File{Entries: s.PutEntries}).ModelEntries()
}

// sessions reads the step's import sessions back as the store's own.
func (s step) sessions() []memory.ImportSession {
	if len(s.PutSessions) == 0 {
		return nil
	}
	return (&kmb.File{ImportSessions: s.PutSessions}).ModelImportSessions()
}

// entryStep renders entries as the step that puts them.
func entryStep(entries []memory.Entry, stream string, bulk bool) step {
	return step{Stream: stream, PutEntries: kmb.FromModel(entries, nil).Entries, Bulk: bulk}
}

// sessionStep renders import sessions as the step that puts them.
func sessionStep(sessions []memory.ImportSession) step {
	return step{PutSessions: kmb.FromModel(nil, sessions).ImportSessions}
}

// applySteps writes one operation's steps into the store they belong to, in
// order, and stops at the first that fails: what a store call that failed
// wrote is what applying it again writes, so a rebuild meets the same state.
func (p *Projector) applySteps(ctx context.Context, kind string, steps []step) error {
	for _, s := range steps {
		if err := p.applyStep(ctx, kind, s); err != nil {
			return err
		}
	}
	return nil
}

// errNoSubsystem reports a write into a subsystem this build's store lacks.
var errNoSubsystem = projectdb.ErrNoStore

func (p *Projector) applyStep(ctx context.Context, kind string, s step) error {
	switch kind {
	case KindTerms:
		return p.applyTerms(ctx, s)
	case KindMemory:
		return p.applyMemory(ctx, s)
	case KindVoice:
		return p.applyVoice(ctx, s)
	case KindRules:
		return p.applyRules(ctx, s)
	}
	return fmt.Errorf("projector: %q is not an operation kind the projector applies", kind)
}

func (p *Projector) applyTerms(ctx context.Context, s step) error {
	tb := p.st.Terms
	if tb == nil {
		return errNoSubsystem
	}
	for _, c := range s.PutConcepts {
		if err := tb.AddConceptWithStream(ctx, c, s.Stream); err != nil {
			return err
		}
	}
	for _, id := range s.DeleteConcepts {
		if err := tb.DeleteConcept(ctx, id); err != nil && !notFound(err) {
			return err
		}
	}
	for _, rel := range s.PutRelations {
		if err := tb.AddRelationWithStream(ctx, rel, s.Stream); err != nil {
			return err
		}
	}
	for _, id := range s.DeleteRelations {
		if err := tb.DeleteRelation(ctx, id); err != nil && !notFound(err) {
			return err
		}
	}
	return nil
}

// applyMemory writes a content-memory step. A bulk step skips the search
// indexes, which are rebuilt once afterwards: by the writer that asked for a
// bulk write, as before, and by a catch-up or a rebuild after it has applied
// every operation.
func (p *Projector) applyMemory(ctx context.Context, s step) error {
	tm := p.st.Memory
	if tm == nil {
		return errNoSubsystem
	}
	if entries := s.entries(); len(entries) > 0 {
		if s.Bulk {
			if err := tm.BulkAddWithStream(ctx, entries, s.Stream); err != nil {
				return err
			}
		} else {
			for _, e := range entries {
				if err := tm.AddWithStream(ctx, e, s.Stream); err != nil {
					return err
				}
			}
		}
	}
	for _, id := range s.DeleteEntries {
		if err := tm.Delete(ctx, id); err != nil && !notFound(err) {
			return err
		}
	}
	for _, session := range s.sessions() {
		if _, held, err := tm.GetImportSession(ctx, session.ID); err != nil {
			return err
		} else if held {
			continue
		}
		if err := tm.CreateImportSession(ctx, session); err != nil {
			return err
		}
	}
	for id, n := range s.SessionCounts {
		if err := tm.UpdateImportSessionCount(ctx, id, n); err != nil && !notFound(err) {
			return err
		}
	}
	for _, id := range s.DeleteSessions {
		if err := tm.DeleteImportSession(ctx, id); err != nil && !notFound(err) {
			return err
		}
	}
	return nil
}

func (p *Projector) applyVoice(ctx context.Context, s step) error {
	store := p.st.Voice
	if store == nil {
		return errNoSubsystem
	}
	for _, prof := range s.CreateProfiles {
		copied := *prof
		if err := store.CreateProfile(ctx, &copied); err != nil {
			return err
		}
	}
	for _, prof := range s.UpdateProfiles {
		copied := *prof
		if err := store.UpdateProfile(ctx, &copied); err != nil {
			return err
		}
	}
	for _, id := range s.DeleteProfiles {
		if err := store.DeleteProfile(ctx, id); err != nil && !notFound(err) {
			return err
		}
	}
	return nil
}

func (p *Projector) applyRules(ctx context.Context, s step) error {
	if p.log == nil {
		return errors.New("projector: a rule widens to a workspace, and this store has none")
	}
	for _, rule := range s.WidenRules {
		if err := p.log.WidenRule(ctx, rule); err != nil {
			return err
		}
	}
	for _, id := range s.NarrowRules {
		if err := p.log.NarrowRule(ctx, id); err != nil {
			return err
		}
	}
	return nil
}

// bulkMemory reports whether any of an operation's steps wrote content memory
// in bulk, which leaves the search indexes to be rebuilt.
func bulkMemory(kind string, steps []step) bool {
	if kind != KindMemory {
		return false
	}
	for _, s := range steps {
		if s.Bulk && len(s.PutEntries) > 0 {
			return true
		}
	}
	return false
}

// rebuildIndexes brings the content memory's search indexes up to date after
// bulk writes.
func (p *Projector) rebuildIndexes(ctx context.Context) error {
	tm := p.st.Memory
	if tm == nil {
		return nil
	}
	if err := tm.RebuildSearchIndex(ctx); err != nil {
		return err
	}
	return tm.RebuildFuzzyIndex(ctx)
}

// notFound reports a store's answer to removing something it does not hold.
// Applying a removal again, in a rebuild or after another process applied it
// first, meets that answer and has nothing left to do.
func notFound(err error) bool {
	return err != nil && strings.Contains(err.Error(), "not found")
}
