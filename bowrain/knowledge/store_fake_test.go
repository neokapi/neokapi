package knowledge

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"
)

// memStore is an in-memory knowledge Store for the write-side tests. It mirrors
// the PostgreSQL store's observable semantics for the methods the merge and
// pilot engines exercise: AppendOp assigns the next seq, AddRevision
// auto-numbers when Rev is zero, LatestRev returns 0 for an unknown concept,
// SetChangeSetStatus / SetMergeResult validate the lifecycle edge, and
// RemovePilot reports "not found" for an absent pilot (so StopPilot's
// idempotency is genuinely exercised). Markets, observations, and comments are
// stored faithfully but unused by these tests.
type memStore struct {
	markets      map[string]*Market
	observations map[string]*Observation
	comments     map[string]*Comment
	revisions    map[string][]*ConceptRevision // key: ws|conceptID
	changesets   map[string]*ChangeSet         // key: ws|id
	ops          map[string][]*ChangeSetOp     // key: ws|changesetID
	reviews      map[string][]*ChangeSetReview // key: ws|changesetID
	pilots       map[string]*Pilot             // key: ws|cs|proj|stream
}

func newMemStore() *memStore {
	return &memStore{
		markets:      map[string]*Market{},
		observations: map[string]*Observation{},
		comments:     map[string]*Comment{},
		revisions:    map[string][]*ConceptRevision{},
		changesets:   map[string]*ChangeSet{},
		ops:          map[string][]*ChangeSetOp{},
		reviews:      map[string][]*ChangeSetReview{},
		pilots:       map[string]*Pilot{},
	}
}

func csKey(ws, id string) string   { return ws + "|" + id }
func revKey(ws, cid string) string { return ws + "|" + cid }

// --- Markets ---------------------------------------------------------------

func (s *memStore) CreateMarket(_ context.Context, m *Market) error {
	s.markets[csKey(m.WorkspaceID, m.ID)] = m
	return nil
}

func (s *memStore) GetMarket(_ context.Context, ws, id string) (*Market, error) {
	m, ok := s.markets[csKey(ws, id)]
	if !ok {
		return nil, fmt.Errorf("market %s not found", id)
	}
	return m, nil
}

func (s *memStore) UpdateMarket(_ context.Context, m *Market) error {
	s.markets[csKey(m.WorkspaceID, m.ID)] = m
	return nil
}

func (s *memStore) DeleteMarket(_ context.Context, ws, id string) error {
	delete(s.markets, csKey(ws, id))
	return nil
}

func (s *memStore) ListMarkets(_ context.Context, ws string) ([]*Market, error) {
	var out []*Market
	for _, m := range s.markets {
		if m.WorkspaceID == ws {
			out = append(out, m)
		}
	}
	return out, nil
}

// --- Observations ----------------------------------------------------------

func (s *memStore) AddObservation(_ context.Context, o *Observation) error {
	s.observations[csKey(o.WorkspaceID, o.ID)] = o
	return nil
}

func (s *memStore) DeleteObservation(_ context.Context, ws, id string) error {
	delete(s.observations, csKey(ws, id))
	return nil
}

func (s *memStore) ListObservationsByConcept(_ context.Context, ws, conceptID string) ([]*Observation, error) {
	var out []*Observation
	for _, o := range s.observations {
		if o.WorkspaceID == ws && o.ConceptID == conceptID {
			out = append(out, o)
		}
	}
	return out, nil
}

// --- Comments --------------------------------------------------------------

func (s *memStore) AddComment(_ context.Context, c *Comment) error {
	s.comments[csKey(c.WorkspaceID, c.ID)] = c
	return nil
}

func (s *memStore) DeleteComment(_ context.Context, ws, id string) error {
	delete(s.comments, csKey(ws, id))
	return nil
}

func (s *memStore) ResolveComment(_ context.Context, ws, id string, resolved bool) error {
	if c, ok := s.comments[csKey(ws, id)]; ok {
		c.Resolved = resolved
	}
	return nil
}

func (s *memStore) ListCommentsByConcept(_ context.Context, ws, conceptID string) ([]*Comment, error) {
	var out []*Comment
	for _, c := range s.comments {
		if c.WorkspaceID == ws && c.ConceptID == conceptID {
			out = append(out, c)
		}
	}
	return out, nil
}

func (s *memStore) ListCommentsByChangeset(_ context.Context, ws, changesetID string) ([]*Comment, error) {
	var out []*Comment
	for _, c := range s.comments {
		if c.WorkspaceID == ws && c.ChangesetID == changesetID {
			out = append(out, c)
		}
	}
	return out, nil
}

// --- Concept revisions -----------------------------------------------------

func (s *memStore) AddRevision(_ context.Context, r *ConceptRevision) error {
	key := revKey(r.WorkspaceID, r.ConceptID)
	if r.Rev == 0 {
		var max int64
		for _, e := range s.revisions[key] {
			if e.Rev > max {
				max = e.Rev
			}
		}
		r.Rev = max + 1
	}
	if r.CreatedAt.IsZero() {
		r.CreatedAt = time.Now().UTC()
	}
	cp := *r
	s.revisions[key] = append(s.revisions[key], &cp)
	return nil
}

func (s *memStore) ListRevisions(_ context.Context, ws, conceptID string) ([]*ConceptRevision, error) {
	list := append([]*ConceptRevision(nil), s.revisions[revKey(ws, conceptID)]...)
	sort.Slice(list, func(i, j int) bool { return list[i].Rev < list[j].Rev })
	return list, nil
}

func (s *memStore) LatestRev(_ context.Context, ws, conceptID string) (int64, error) {
	var max int64
	for _, e := range s.revisions[revKey(ws, conceptID)] {
		if e.Rev > max {
			max = e.Rev
		}
	}
	return max, nil
}

// --- Change-sets -----------------------------------------------------------

func (s *memStore) CreateChangeSet(_ context.Context, cs *ChangeSet) error {
	if cs.Status == "" {
		cs.Status = ChangeSetDraft
	}
	if cs.CreatedAt.IsZero() {
		cs.CreatedAt = time.Now().UTC()
	}
	cs.UpdatedAt = cs.CreatedAt
	cp := *cs
	s.changesets[csKey(cs.WorkspaceID, cs.ID)] = &cp
	return nil
}

func (s *memStore) GetChangeSet(_ context.Context, ws, id string) (*ChangeSet, error) {
	cs, ok := s.changesets[csKey(ws, id)]
	if !ok {
		return nil, fmt.Errorf("change-set %s not found", id)
	}
	cp := *cs
	return &cp, nil
}

func (s *memStore) ListChangeSets(_ context.Context, ws string, status ChangeSetStatus) ([]*ChangeSet, error) {
	var out []*ChangeSet
	for _, cs := range s.changesets {
		if cs.WorkspaceID != ws {
			continue
		}
		if status != "" && cs.Status != status {
			continue
		}
		cp := *cs
		out = append(out, &cp)
	}
	return out, nil
}

func (s *memStore) UpdateChangeSet(_ context.Context, cs *ChangeSet) error {
	existing, ok := s.changesets[csKey(cs.WorkspaceID, cs.ID)]
	if !ok {
		return fmt.Errorf("change-set %s not found", cs.ID)
	}
	existing.Name = cs.Name
	existing.Description = cs.Description
	existing.UpdatedAt = time.Now().UTC()
	return nil
}

func (s *memStore) SetChangeSetStatus(_ context.Context, ws, id string, to ChangeSetStatus) error {
	if to == ChangeSetMerged {
		return errors.New("use SetMergeResult to merge a change-set, not SetChangeSetStatus")
	}
	cs, ok := s.changesets[csKey(ws, id)]
	if !ok {
		return fmt.Errorf("change-set %s not found", id)
	}
	if err := ValidateStatusTransition(cs.Status, to); err != nil {
		return err
	}
	now := time.Now().UTC()
	if cs.Status == ChangeSetDraft && to == ChangeSetInReview {
		cs.SubmittedAt = &now
	}
	cs.Status = to
	cs.UpdatedAt = now
	return nil
}

func (s *memStore) SetMergeResult(_ context.Context, ws, id, mergedBy string, mergedAt time.Time) error {
	cs, ok := s.changesets[csKey(ws, id)]
	if !ok {
		return fmt.Errorf("change-set %s not found", id)
	}
	if err := ValidateStatusTransition(cs.Status, ChangeSetMerged); err != nil {
		return err
	}
	if mergedAt.IsZero() {
		mergedAt = time.Now().UTC()
	}
	cs.Status = ChangeSetMerged
	cs.MergedBy = mergedBy
	cs.MergedAt = &mergedAt
	cs.UpdatedAt = mergedAt
	return nil
}

// --- Change-set ops --------------------------------------------------------

func (s *memStore) AppendOp(_ context.Context, op *ChangeSetOp) error {
	key := csKey(op.WorkspaceID, op.ChangesetID)
	op.Seq = int64(len(s.ops[key]))
	if op.CreatedAt.IsZero() {
		op.CreatedAt = time.Now().UTC()
	}
	cp := *op
	s.ops[key] = append(s.ops[key], &cp)
	return nil
}

func (s *memStore) RemoveOp(_ context.Context, ws, changesetID string, seq int64) error {
	key := csKey(ws, changesetID)
	kept := s.ops[key][:0:0]
	for _, op := range s.ops[key] {
		if op.Seq != seq {
			kept = append(kept, op)
		}
	}
	s.ops[key] = kept
	return nil
}

func (s *memStore) ListOps(_ context.Context, ws, changesetID string) ([]*ChangeSetOp, error) {
	list := append([]*ChangeSetOp(nil), s.ops[csKey(ws, changesetID)]...)
	sort.Slice(list, func(i, j int) bool { return list[i].Seq < list[j].Seq })
	return list, nil
}

// --- Reviews ---------------------------------------------------------------

func (s *memStore) AddReview(_ context.Context, r *ChangeSetReview) error {
	key := csKey(r.WorkspaceID, r.ChangesetID)
	if r.CreatedAt.IsZero() {
		r.CreatedAt = time.Now().UTC()
	}
	for i, ex := range s.reviews[key] {
		if ex.Reviewer == r.Reviewer {
			cp := *r
			s.reviews[key][i] = &cp
			return nil
		}
	}
	cp := *r
	s.reviews[key] = append(s.reviews[key], &cp)
	return nil
}

func (s *memStore) ListReviews(_ context.Context, ws, changesetID string) ([]*ChangeSetReview, error) {
	return append([]*ChangeSetReview(nil), s.reviews[csKey(ws, changesetID)]...), nil
}

// --- Pilots ----------------------------------------------------------------

func pilotKey(ws, cs, proj, stream string) string {
	return ws + "|" + cs + "|" + proj + "|" + stream
}

func (s *memStore) AddPilot(_ context.Context, p *Pilot) error {
	if p.CreatedAt.IsZero() {
		p.CreatedAt = time.Now().UTC()
	}
	cp := *p
	s.pilots[pilotKey(p.WorkspaceID, p.ChangesetID, p.ProjectID, p.Stream)] = &cp
	return nil
}

func (s *memStore) RemovePilot(_ context.Context, ws, cs, proj, stream string) error {
	key := pilotKey(ws, cs, proj, stream)
	if _, ok := s.pilots[key]; !ok {
		return fmt.Errorf("pilot %s/%s/%s not found", cs, proj, stream)
	}
	delete(s.pilots, key)
	return nil
}

func (s *memStore) ListPilots(_ context.Context, ws, changesetID string) ([]*Pilot, error) {
	var out []*Pilot
	for _, p := range s.pilots {
		if p.WorkspaceID == ws && p.ChangesetID == changesetID {
			cp := *p
			out = append(out, &cp)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].ProjectID != out[j].ProjectID {
			return out[i].ProjectID < out[j].ProjectID
		}
		return out[i].Stream < out[j].Stream
	})
	return out, nil
}

func (s *memStore) ListPilotsForStream(_ context.Context, ws, proj, stream string) ([]*Pilot, error) {
	var out []*Pilot
	for _, p := range s.pilots {
		if p.WorkspaceID == ws && p.ProjectID == proj && p.Stream == stream {
			cp := *p
			out = append(out, &cp)
		}
	}
	return out, nil
}

func (s *memStore) Close() error { return nil }

// compile-time check that the fake satisfies the Store interface.
var _ Store = (*memStore)(nil)
