package change

import (
	"fmt"
	"slices"
	"strings"

	"github.com/neokapi/neokapi/core/model"
)

// BlockKey is the key a change set addresses a block by: the durable key
// reconciliation assigned (Unit) where there is one, else the name the format
// reports, else the reader's id. It is the storage key ladder
// (convergence.BlockKey) and is what a read reports.
func BlockKey(b *model.Block) string {
	if b.Unit != "" {
		return b.Unit
	}
	if b.Name != "" {
		return b.Name
	}
	return b.ID
}

// docPlan is the operations of a change set addressed to one document. It is
// the Editor the document's home runs over the document's blocks.
type docPlan struct {
	svc   *Service
	set   *Set
	actor Actor
	home  Home
	sess  Session
	info  DocInfo
	desc  Description
	// ops are the indexes of the operations addressed to the document, in
	// the order of the change set.
	ops []int
	// byKey maps a block key to the operations addressed to it, in order.
	byKey map[string][]int
	// results is the change set's result list, shared with the service.
	results []OpResult
	want    Want

	// What one pass found. Begin resets it.
	seen      map[string]int
	first     map[string]Candidate
	changes   []EditionChange
	changeOps [][]int
	decisions map[int]*DecisionTarget
	cands     map[string]*candidates
	refused   bool
}

// candidateLimit bounds the near keys a not_found refusal names.
const candidateLimit = 3

// candidateKeyLimit is the number of distinct missing keys for which a pass
// looks for near keys. Past it, a refusal names none, so the cost of a pass
// stays linear in the document.
const candidateKeyLimit = 64

func (p *docPlan) Begin() {
	p.seen = map[string]int{}
	p.first = map[string]Candidate{}
	p.changes = nil
	p.changeOps = nil
	p.decisions = map[int]*DecisionTarget{}
	p.refused = false
	if len(p.byKey) <= candidateKeyLimit {
		p.cands = make(map[string]*candidates, len(p.byKey))
		for k := range p.byKey {
			p.cands[k] = &candidates{key: k}
		}
	} else {
		p.cands = nil
	}
	for _, i := range p.ops {
		op := p.set.Ops[i]
		p.results[i] = OpResult{I: i, Op: op.Kind, At: refOf(op)}
	}
}

// match returns the key b is addressed by and the operations addressed to it.
func (p *docPlan) match(b *model.Block) (string, []int) {
	for _, k := range []string{b.Unit, b.Name, b.ID} {
		if k == "" {
			continue
		}
		if ops, ok := p.byKey[k]; ok {
			return k, ops
		}
	}
	return "", nil
}

func (p *docPlan) refuse(i int, err *Error) {
	p.results[i].Status = OpRefused
	p.results[i].Error = err
	p.refused = true
}

func (p *docPlan) Edit(b *model.Block) ([]model.EditionKey, error) {
	key, ops := p.match(b)
	if ops == nil {
		p.observe(b)
		return nil, nil
	}
	p.seen[key]++
	if p.seen[key] > 1 {
		cands := []Candidate{p.first[key], candidateOf(b)}
		for _, i := range ops {
			p.results[i] = OpResult{I: i, Op: p.set.Ops[i].Kind, At: p.results[i].At}
			p.refuse(i, &Error{Code: CodeAmbiguous, Field: "at/block", Candidates: cands,
				Message: fmt.Sprintf("%s holds more than one block keyed %q", p.info.Doc, key)})
		}
		return nil, nil
	}
	p.first[key] = candidateOf(b)

	var content []Op
	var contentIdx, decides []int
	for _, i := range ops {
		op := p.set.Ops[i]
		if op.Kind == KindDecide {
			decides = append(decides, i)
			continue
		}
		content = append(content, op)
		contentIdx = append(contentIdx, i)
	}

	// A decision names the revision its sender read, checked against the
	// content as it stood when the change set began.
	for _, i := range decides {
		op := p.set.Ops[i]
		p.results[i].At = p.canonical(b, op.At.Edition)
		if op.IfMatch == AnyRevision {
			continue
		}
		if rev := model.EditionRevision(b, op.At.Edition); rev != op.IfMatch {
			ed, _ := b.Edition(op.At.Edition)
			cur := &Current{Rev: rev}
			if rev != model.AbsentRevision {
				cur.Text = model.RunsEditText(ed.Runs)
			}
			p.refuse(i, &Error{Code: CodeStale, Field: "if_match",
				Message: fmt.Sprintf("edition %s of block %s is at %s, not %s", editionLabel(b, op.At.Edition), key, rev, op.IfMatch)})
			p.results[i].Current = cur
		}
	}

	if !b.Translatable && len(content) > 0 {
		preview := ApplyBlock(b, content, p.env(true))
		for j, r := range preview {
			if r.Status == OpPreviewed || r.Status == OpApplied {
				i := contentIdx[j]
				p.results[i].At = p.canonical(b, p.set.Ops[i].At.Edition)
				p.refuse(i, &Error{Code: CodeUnsupported, Capability: "editable",
					Message: fmt.Sprintf("%s marks block %s as content an edit does not change, such as code", p.info.Doc, key)})
			}
		}
	}
	if p.refused {
		return nil, nil
	}

	type before struct {
		key  model.EditionKey
		rev  string
		runs []model.Run
		ops  []int
	}
	var touched []*before
	for j, op := range content {
		k := b.EditionKeyOf(op.At.Edition)
		at := slices.IndexFunc(touched, func(t *before) bool { return t.key == k })
		if at < 0 {
			ed, _ := b.Edition(op.At.Edition)
			touched = append(touched, &before{key: k, rev: model.EditionRevision(b, op.At.Edition), runs: ed.Runs})
			at = len(touched) - 1
		}
		touched[at].ops = append(touched[at].ops, contentIdx[j])
	}

	results := ApplyBlock(b, content, p.env(false))
	for j, r := range results {
		i := contentIdx[j]
		r.I = i
		r.At = p.canonical(b, p.set.Ops[i].At.Edition)
		if r.BlockedBy != nil {
			g := contentIdx[*r.BlockedBy]
			r.BlockedBy = &g
		}
		p.results[i] = r
		if r.Status == OpRefused {
			p.refused = true
		}
	}
	if p.refused {
		return nil, nil
	}

	var changed []model.EditionKey
	authKey := b.EditionKeyOf(b.Authoritative(model.AuthorityPolicy{}))
	for _, t := range touched {
		applied := slices.ContainsFunc(t.ops, func(i int) bool { return p.results[i].Status == OpApplied })
		if !applied {
			continue
		}
		changed = append(changed, t.key)
		after := model.EditionRevision(b, t.key)
		if after == t.rev {
			continue
		}
		ed, ok := b.Edition(t.key)
		role := RoleDerived
		if t.key == authKey {
			role = RoleAuthoritative
		}
		ch := EditionChange{
			Ref:       *p.canonical(b, t.key),
			Role:      role,
			Key:       key,
			BeforeRev: t.rev,
			AfterRev:  after,
			Block:     b,
		}
		if t.rev != model.AbsentRevision {
			ch.Before = t.runs
		}
		if ok {
			ch.After = ed.Runs
		}
		for _, i := range t.ops {
			if r := p.results[i]; r.Basis != "" {
				ch.Basis = r.Basis
			}
		}
		p.changes = append(p.changes, ch)
		p.changeOps = append(p.changeOps, t.ops)
	}

	for _, i := range decides {
		if p.results[i].Status == OpRefused {
			continue
		}
		op := p.set.Ops[i]
		k := b.EditionKeyOf(op.At.Edition)
		role := RoleDerived
		if k == authKey {
			role = RoleAuthoritative
		}
		rev := model.EditionRevision(b, op.At.Edition)
		p.decisions[i] = &DecisionTarget{Doc: p.info, Ref: *p.results[i].At, Place: p.sess.Place(op.At.Edition), Rev: rev, Role: role}
		// The decision lands once the content has; until then it stands as
		// one that would.
		p.results[i].Status = OpApplied
		p.results[i].Before = op.IfMatch
		p.results[i].After = rev
	}
	return changed, nil
}

// observe looks at a block no operation addresses, for the near keys a
// not_found refusal names.
func (p *docPlan) observe(b *model.Block) {
	if p.cands == nil {
		return
	}
	k := BlockKey(b)
	for _, c := range p.cands {
		c.offer(k, b)
	}
}

func (p *docPlan) End() error {
	for _, i := range p.ops {
		r := &p.results[i]
		if r.Status != "" {
			continue
		}
		op := p.set.Ops[i]
		var cands []Candidate
		if c := p.cands[op.At.Block]; c != nil {
			cands = c.list()
		}
		p.refuse(i, &Error{Code: CodeNotFound, Field: "at/block", Candidates: cands,
			Message: fmt.Sprintf("%s holds no block keyed %q", p.info.Doc, op.At.Block)})
	}
	if p.refused {
		return ErrRefused
	}
	return nil
}

// env is the BlockEnv the change set's operations apply under.
func (p *docPlan) env(preview bool) BlockEnv {
	guards := Enforce
	if p.actor.Kind == ActorTool {
		// A tool's drafts meet the ship gates later, as its governance
		// findings do.
		guards = Report
	}
	return BlockEnv{
		Actor:        p.actor,
		RequireBasis: p.set.RequireBasis,
		Preview:      preview,
		Guards:       guards,
		Now:          p.svc.now,
	}
}

// canonical is the reference an operation on edition k of b echoes: the
// document's canonical reference, the block's key, and the edition key with
// the document's own edition left empty.
func (p *docPlan) canonical(b *model.Block, k model.EditionKey) *Ref {
	r := Ref{Doc: p.info.Doc, Block: BlockKey(b)}
	if !b.IsSourceEdition(k) {
		r.Edition = k.Canonical()
	}
	return &r
}

// editionLabel names an edition of b in a message.
func editionLabel(b *model.Block, k model.EditionKey) string {
	if b.IsSourceEdition(k) {
		return "(the document's own)"
	}
	return keyText(k)
}

// refOf is the reference an operation's result echoes before the operation
// is routed: what it was sent with.
func refOf(op Op) *Ref {
	switch op.Kind {
	case KindTerm, KindMemory, KindRecipe:
		return nil
	}
	at := op.At
	return &at
}

// candidateOf describes b as a candidate a refusal names.
func candidateOf(b *model.Block) Candidate {
	ed, _ := b.Edition(model.EditionKey{})
	return Candidate{Key: BlockKey(b), Rev: model.EditionRevision(b, model.EditionKey{}), Text: firstChars(model.RunsEditText(ed.Runs), 80)}
}

// firstChars is the first n characters of s.
func firstChars(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}

// candidates keeps the few block keys nearest one key no block matched.
type candidates struct {
	key   string
	items []scored
}

type scored struct {
	score int
	c     Candidate
}

// offer considers block b, keyed k.
func (c *candidates) offer(k string, b *model.Block) {
	s := nearness(c.key, k)
	if s <= 0 {
		return
	}
	if len(c.items) == candidateLimit && s <= c.items[len(c.items)-1].score {
		return
	}
	c.items = append(c.items, scored{score: s, c: candidateOf(b)})
	slices.SortStableFunc(c.items, func(a, b scored) int { return b.score - a.score })
	if len(c.items) > candidateLimit {
		c.items = c.items[:candidateLimit]
	}
}

func (c *candidates) list() []Candidate {
	out := make([]Candidate, 0, len(c.items))
	for _, s := range c.items {
		out = append(out, s.c)
	}
	return out
}

// nearness scores how alike two block keys are: the length of the prefix they
// share, and more when they end in the same segment. Zero is not alike.
func nearness(want, got string) int {
	n := 0
	for n < len(want) && n < len(got) && want[n] == got[n] {
		n++
	}
	last := func(s string) string {
		if i := strings.LastIndexAny(s, "/."); i >= 0 {
			return s[i+1:]
		}
		return s
	}
	if last(want) != "" && last(want) == last(got) {
		n += 2
	}
	if strings.EqualFold(want, got) {
		n += 4
	}
	return n
}
