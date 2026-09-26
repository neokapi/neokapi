package projector

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"reflect"
	"sync"
	"time"

	coreprofile "github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/core/state"
	"github.com/neokapi/neokapi/core/workspace"
	"github.com/neokapi/neokapi/memory"
	"github.com/neokapi/neokapi/memory/kmb"
	"github.com/neokapi/neokapi/terms"
	"github.com/neokapi/neokapi/voice"
)

// sink takes the steps a store call made: a projector records and applies
// them at once, and a batch holds them until it commits.
type sink interface {
	put(ctx context.Context, kind string, s step) error
}

// put records and applies one step as an operation of its own.
func (p *Projector) put(ctx context.Context, kind string, s step) error {
	return p.commit(ctx, []pending{{kind: kind, steps: []step{s}}})
}

// Batch collects the writes of one pass into one operation per subsystem,
// recorded and applied when it commits. Reads through its stores answer from
// the projection as it stood before the batch, so a batch suits a pass that
// writes what it read elsewhere: an import, an absorbed record.
type Batch struct {
	p     *Projector
	mu    sync.Mutex
	order []string
	steps map[string][]step
	// concepts records the concepts the batch has put (true) or deleted
	// (false), so a relation between two concepts the same batch puts is
	// accepted before either is applied.
	concepts map[string]bool
}

// Batch starts a batch of writes.
func (p *Projector) Batch() *Batch {
	return &Batch{p: p, steps: map[string][]step{}, concepts: map[string]bool{}}
}

func (b *Batch) put(_ context.Context, kind string, s step) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if _, seen := b.steps[kind]; !seen {
		b.order = append(b.order, kind)
	}
	b.steps[kind] = append(b.steps[kind], s)
	for _, c := range s.PutConcepts {
		b.concepts[c.ID] = true
	}
	for _, id := range s.DeleteConcepts {
		b.concepts[id] = false
	}
	return nil
}

// pendingConcept reports what the batch has done to a concept: put it
// (held true) or deleted it (held false). known is false for a concept the
// batch has not touched.
func (b *Batch) pendingConcept(id string) (held, known bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	held, known = b.concepts[id]
	return held, known
}

// Commit records and applies what the batch collected. A batch that collected
// nothing records nothing.
func (b *Batch) Commit(ctx context.Context) error {
	b.mu.Lock()
	writes := make([]pending, 0, len(b.order))
	for _, kind := range b.order {
		writes = append(writes, pending{kind: kind, steps: b.steps[kind]})
	}
	b.order, b.steps = nil, map[string][]step{}
	b.mu.Unlock()
	return b.p.commit(ctx, writes)
}

// Terms is the batch's view of the terms store.
func (b *Batch) Terms() *Terms { return b.p.terms(b) }

// Memory is the batch's view of the content memory.
func (b *Batch) Memory() *Memory { return b.p.memory(b) }

// Voice is the batch's view of the voice profiles.
func (b *Batch) Voice() *Voice { return b.p.voice(b) }

// Terms is the project's terms store as a writer sees it: reads answer from
// the projection, and every write is recorded in the log and applied by the
// projector. It is nil on a build whose store has no terms.
func (p *Projector) Terms() *Terms { return p.terms(p) }

func (p *Projector) terms(to sink) *Terms {
	if p.st.Terms == nil {
		return nil
	}
	return &Terms{SQLiteStore: p.st.Terms, to: to}
}

// Memory is the project's content memory as a writer sees it. It is nil on a
// build whose store has no content memory.
func (p *Projector) Memory() *Memory { return p.memory(p) }

func (p *Projector) memory(to sink) *Memory {
	if p.st.Memory == nil {
		return nil
	}
	return &Memory{SQLiteStore: p.st.Memory, to: to}
}

// Voice is the project's voice profiles as a writer sees them. It is nil on a
// build whose store has no voice profiles.
func (p *Projector) Voice() *Voice { return p.voice(p) }

func (p *Projector) voice(to sink) *Voice {
	if p.st.Voice == nil {
		return nil
	}
	return &Voice{SQLiteStore: p.st.Voice, to: to}
}

// Terms is a terms store whose writes go through the projector.
type Terms struct {
	*terms.SQLiteStore
	to sink
}

var _ terms.Store = (*Terms)(nil)

// AddConcept puts a concept.
func (t *Terms) AddConcept(ctx context.Context, c terms.Concept) error {
	return t.AddConceptWithStream(ctx, c, "")
}

// AddConceptWithStream puts a concept on a stream. A concept the store
// already holds with the same content is left as it is, so reading one file
// twice writes nothing the second time and moves no timestamp.
func (t *Terms) AddConceptWithStream(ctx context.Context, c terms.Concept, stream string) error {
	if c.ID == "" {
		return terms.ErrConceptIDRequired
	}
	for _, term := range c.Terms {
		if term.Status != "" && !terms.KnownTermStatus(term.Status) {
			return fmt.Errorf("term %q (%s): unknown status %q", term.Text, term.Locale, term.Status)
		}
	}
	c = terms.NormalizedConcept(c)
	if held, ok, err := t.GetConcept(ctx, c.ID); err == nil && ok && terms.SameConcept(held, c) {
		return nil
	}
	return t.to.put(ctx, KindTerms, step{Stream: stream, PutConcepts: []terms.Concept{c}})
}

// DeleteConcept removes a concept. Removing one the store does not hold
// writes nothing and answers as the store does.
func (t *Terms) DeleteConcept(ctx context.Context, id string) error {
	if _, ok, err := t.GetConcept(ctx, id); err != nil || !ok {
		return t.SQLiteStore.DeleteConcept(ctx, id)
	}
	return t.to.put(ctx, KindTerms, step{DeleteConcepts: []string{id}})
}

// AddRelation puts a relation between two concepts.
func (t *Terms) AddRelation(ctx context.Context, rel terms.ConceptRelation) error {
	return t.AddRelationWithStream(ctx, rel, "")
}

// AddRelationWithStream puts a relation on a stream, after the checks the
// store makes, so a relation the store would refuse is never recorded.
func (t *Terms) AddRelationWithStream(ctx context.Context, rel terms.ConceptRelation, stream string) error {
	if err := terms.ValidateRelation(rel); err != nil {
		return err
	}
	for _, end := range [][2]string{{"source", rel.SourceID}, {"target", rel.TargetID}} {
		role, id := end[0], end[1]
		if b, inBatch := t.to.(*Batch); inBatch {
			if held, known := b.pendingConcept(id); known {
				if !held {
					return fmt.Errorf("%s concept not found: %s", role, id)
				}
				continue
			}
		}
		if _, ok, err := t.GetConcept(ctx, id); err != nil {
			return err
		} else if !ok {
			return fmt.Errorf("%s concept not found: %s", role, id)
		}
	}
	return t.to.put(ctx, KindTerms, step{Stream: stream, PutRelations: []terms.ConceptRelation{rel}})
}

// DeleteRelation removes a relation.
func (t *Terms) DeleteRelation(ctx context.Context, id string) error {
	return t.to.put(ctx, KindTerms, step{DeleteRelations: []string{id}})
}

// Memory is a content memory whose writes go through the projector.
type Memory struct {
	*memory.SQLiteStore
	to sink
}

var (
	_ memory.Store     = (*Memory)(nil)
	_ memory.BulkAdder = (*Memory)(nil)
)

// Add puts an entry.
func (m *Memory) Add(ctx context.Context, e memory.Entry) error {
	return m.AddWithStream(ctx, e, "")
}

// AddWithStream puts an entry on a stream. An entry the store already holds
// with the same content is left as it is.
func (m *Memory) AddWithStream(ctx context.Context, e memory.Entry, stream string) error {
	if err := checkEntry(e); err != nil {
		return err
	}
	if m.holds(ctx, e) {
		return nil
	}
	return m.to.put(ctx, KindMemory, entryStep([]memory.Entry{e}, stream, false))
}

// BulkAddWithStream puts many entries in one write, skipping those the store
// already holds with the same content. As with the store itself, the caller
// rebuilds the search indexes afterwards.
func (m *Memory) BulkAddWithStream(ctx context.Context, entries []memory.Entry, stream string) error {
	fresh := make([]memory.Entry, 0, len(entries))
	for _, e := range entries {
		if err := checkEntry(e); err != nil {
			return fmt.Errorf("bulk add entry %s: %w", e.ID, err)
		}
		if !m.holds(ctx, e) {
			fresh = append(fresh, e)
		}
	}
	if len(fresh) == 0 {
		return nil
	}
	return m.to.put(ctx, KindMemory, entryStep(fresh, stream, true))
}

// Delete removes an entry. Removing one the store does not hold writes
// nothing.
func (m *Memory) Delete(ctx context.Context, id string) error {
	if _, ok, err := m.GetEntry(ctx, id); err != nil || !ok {
		return err
	}
	return m.to.put(ctx, KindMemory, step{DeleteEntries: []string{id}})
}

// CreateImportSession records an import session.
func (m *Memory) CreateImportSession(ctx context.Context, s memory.ImportSession) error {
	return m.to.put(ctx, KindMemory, sessionStep([]memory.ImportSession{s}))
}

// UpdateImportSessionCount records how many entries an import session wrote.
func (m *Memory) UpdateImportSessionCount(ctx context.Context, id string, count int) error {
	return m.to.put(ctx, KindMemory, step{SessionCounts: map[string]int{id: count}})
}

// DeleteImportSession removes an import session.
func (m *Memory) DeleteImportSession(ctx context.Context, id string) error {
	return m.to.put(ctx, KindMemory, step{DeleteSessions: []string{id}})
}

// checkEntry makes the checks the store makes before writing, so an entry the
// store would refuse is never recorded.
func checkEntry(e memory.Entry) error {
	if e.ID == "" {
		return memory.ErrEntryIDRequired
	}
	if len(e.Variants) == 0 {
		return memory.ErrEntryNoVariants
	}
	return nil
}

// holds reports whether writing e would leave the stored entry as it is. A
// write replaces the variants it carries locale by locale and everything else
// wholesale, so the question is whether the stored entry with e's variants
// laid over it is the stored entry.
func (m *Memory) holds(ctx context.Context, e memory.Entry) bool {
	held, ok, err := m.GetEntry(ctx, e.ID)
	if err != nil || !ok {
		return false
	}
	memory.NormalizeEntryLocales(&e)
	merged := e
	merged.Variants = maps.Clone(held.Variants)
	maps.Copy(merged.Variants, e.Variants)
	merged.CreatedAt, merged.UpdatedAt = held.CreatedAt, held.UpdatedAt
	merged.Origins = make([]memory.Origin, len(e.Origins))
	copy(merged.Origins, e.Origins)
	for i := range merged.Origins {
		if merged.Origins[i].AddedAt.IsZero() && i < len(held.Origins) {
			merged.Origins[i].AddedAt = held.Origins[i].AddedAt
		}
	}
	a := kmb.FromModel([]memory.Entry{held}, nil).Entries[0]
	b := kmb.FromModel([]memory.Entry{merged}, nil).Entries[0]
	return sameJSON(a, b)
}

// Voice is a voice-profile store whose writes go through the projector.
type Voice struct {
	*voice.SQLiteStore
	to sink
}

// CreateProfile creates a profile, filling the timestamps and version the
// store would, onto the profile given.
func (v *Voice) CreateProfile(ctx context.Context, prof *coreprofile.VoiceProfile) error {
	return v.to.put(ctx, KindVoice, step{CreateProfiles: []*coreprofile.VoiceProfile{prof}})
}

// UpdateProfile replaces a profile, archiving the version it replaces. A
// profile whose content has not changed is left as it is, at the version it
// holds.
func (v *Voice) UpdateProfile(ctx context.Context, prof *coreprofile.VoiceProfile) error {
	held, err := v.GetProfile(ctx, prof.ID)
	if err != nil {
		return v.SQLiteStore.UpdateProfile(ctx, prof)
	}
	if sameProfile(held, prof) {
		prof.Version, prof.UpdatedAt = held.Version, held.UpdatedAt
		return nil
	}
	if err := v.to.put(ctx, KindVoice, step{UpdateProfiles: []*coreprofile.VoiceProfile{prof}}); err != nil {
		return err
	}
	if now, gerr := v.GetProfile(ctx, prof.ID); gerr == nil {
		prof.Version, prof.UpdatedAt = now.Version, now.UpdatedAt
	}
	return nil
}

// DeleteProfile removes a profile.
func (v *Voice) DeleteProfile(ctx context.Context, id string) error {
	if _, err := v.GetProfile(ctx, id); err != nil {
		return v.SQLiteStore.DeleteProfile(ctx, id)
	}
	return v.to.put(ctx, KindVoice, step{DeleteProfiles: []string{id}})
}

// sameProfile reports whether updating held with prof would change what the
// profile says. Omitted constraints keep the held ones, as the store does.
func sameProfile(held, prof *coreprofile.VoiceProfile) bool {
	a, b := *held, *prof
	if b.Constraints == nil {
		b.Constraints = a.Constraints
	}
	for _, p := range []*coreprofile.VoiceProfile{&a, &b} {
		p.Version, p.VersionNote = 0, ""
		p.CreatedAt, p.UpdatedAt = time.Time{}, time.Time{}
		p.CreatedBy = ""
	}
	return sameJSON(a, b)
}

// Rules is the store of rules widened to the whole workspace, as
// core/contextop writes it: widening and narrowing go through the projector.
type Rules struct{ p *Projector }

// Rules hands out the workspace rule store for contextop.Widen and Narrow.
func (p *Projector) Rules() *Rules { return &Rules{p: p} }

// WidenRule puts a rule in force across the workspace.
func (r *Rules) WidenRule(ctx context.Context, rule workspace.Rule) error {
	if rule.ID == "" {
		return workspace.ErrNoRuleID
	}
	return r.p.put(ctx, KindRules, step{WidenRules: []workspace.Rule{rule}})
}

// NarrowRule takes a rule back out of force.
func (r *Rules) NarrowRule(ctx context.Context, id string) error {
	if id == "" {
		return workspace.ErrNoRuleID
	}
	return r.p.put(ctx, KindRules, step{NarrowRules: []string{id}})
}

// WidenedRules reads the rules in force across the workspace.
func (r *Rules) WidenedRules(ctx context.Context, kind string) ([]workspace.Rule, error) {
	if r.p.log == nil {
		return nil, nil
	}
	return r.p.log.WidenedRules(ctx, kind)
}

// sameJSON compares two values by what they encode to, which is what a store
// holds of them.
func sameJSON(a, b any) bool {
	x, errA := json.Marshal(a)
	y, errB := json.Marshal(b)
	return errA == nil && errB == nil && reflect.DeepEqual(x, y)
}

// StandaloneMemory is a content-memory file a person named, which is not a
// projection of any log: its writes apply to the file directly. A project's
// own content memory is written through Projector.Memory instead.
func StandaloneMemory(tm *memory.SQLiteStore) *Memory {
	if tm == nil {
		return nil
	}
	return (&Projector{st: Stores{Memory: tm}, lock: &sync.Mutex{}}).Memory()
}

// StandaloneTerms is a terms file a person named, written directly.
func StandaloneTerms(tb *terms.SQLiteStore) *Terms {
	if tb == nil {
		return nil
	}
	return (&Projector{st: Stores{Terms: tb}, lock: &sync.Mutex{}}).Terms()
}

// Units is the journal the unit decision ledger records through
// (state.WorkStore.SetJournal): each entry is one operation, addressed by the
// entry's own content address, so recording one decision twice, here or on
// another machine, is one operation.
func (p *Projector) Units() state.Journal { return unitJournal{p} }

type unitJournal struct{ p *Projector }

func (j unitJournal) RecordEntries(ctx context.Context, entries []state.JournalEntry) error {
	writes := make([]pending, 0, len(entries))
	for _, e := range entries {
		w := pending{kind: KindUnit, steps: []step{{Entries: []state.JournalEntry{e}}}}
		if !e.Held {
			// A re-assertion of an old entry is a new event; a first recording
			// is the entry itself. The project is in the address because two
			// projects reaching one decision about their own units hold it
			// once each.
			w.address = "unit:" + string(j.p.key) + ":" + e.ID
		}
		writes = append(writes, w)
	}
	return j.p.commit(ctx, writes)
}
