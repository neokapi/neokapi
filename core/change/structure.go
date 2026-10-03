package change

import (
	"fmt"
	"maps"
	"slices"

	"github.com/neokapi/neokapi/core/model"
)

// insert_block and delete_block change a document's structure: which blocks
// it holds. ApplyBlock changes one block's content and cannot do either, and a
// skeleton has no slot for a block nobody read, so the service hands these
// operations to the home (Restructurer) and the format's writer writes the
// shell of the block added or removed. Each operation is checked against the
// document as the home read it before the pass:
//
//   - delete_block names the revision of the block's own edition and of any
//     other edition its sender read; leaving out the own edition is invalid,
//     and an edition named at another revision, or named and absent, is
//     stale. The block goes with every edition it holds, the document's own
//     and every translation in a file of its own, named or not;
//   - insert_block refuses a key a block of the document already answers
//     to, and an anchor no block answers to.
//
// The pass that follows reads what the home wrote, so the content of a new
// block is what the format reads back, and its revisions are the ones a
// later read reports.

// planStructural files an insert_block or delete_block of document p, and
// refuses one the document, its format or its home cannot carry.
func (r *applyRun) planStructural(p *docPlan, i int) {
	op := r.set.Ops[i]
	if r.viaFile[i] {
		r.refuse(i, &Error{Code: CodeInvalid, Field: "at/doc",
			Message: fmt.Sprintf("%s adds or removes a block of %s; send it to %s", op.Kind, p.info.Doc, p.info.Doc)})
		return
	}
	ss, ok := p.sess.(StructuralSession)
	if !ok || !slices.Contains(ss.Structural(), op.Kind) {
		r.refuse(i, &Error{Code: CodeUnsupported, Capability: string(op.Kind),
			Message: fmt.Sprintf("the %s home cannot write %s in %s", p.home.Name(), op.Kind, p.info.Doc)})
		return
	}
	if p.inserts == nil {
		p.inserts, p.deletes, p.newRuns, p.structKeys = map[string]int{}, map[string]int{}, map[int]map[model.EditionKey][]model.Run{}, map[string]bool{}
	}
	switch body := op.Body.(type) {
	case *DeleteBlock:
		key := op.At.Block
		if !op.At.Edition.IsZero() {
			r.refuse(i, &Error{Code: CodeInvalid, Field: "at/edition", Message: "delete_block removes a block with every edition; it names no edition"})
			return
		}
		if key == "" {
			r.refuse(i, &Error{Code: CodeInvalid, Field: "at/block", Message: "delete_block names no block"})
			return
		}
		p.deletes[key] = i
		p.structKeys[key] = true
		p.want.Blocks = appendNew(p.want.Blocks, key)
		// Every edition the block holds is removed with it, so the files of
		// all of them are read.
		for _, k := range p.info.Derived {
			if p.sess.Place(k).Kind == PlaceOwnFile {
				p.wantEdition(k)
			}
		}
	case *InsertBlock:
		if !r.planInsert(p, i, body) {
			return
		}
	default:
		return
	}
	p.want.Own = true
	p.want.Structural = true
	p.structural = append(p.structural, i)
}

// planInsert checks an insert_block and builds its content.
func (r *applyRun) planInsert(p *docPlan, i int, body *InsertBlock) bool {
	name := body.Name
	anchor := body.After
	if body.Before != "" {
		anchor = body.Before
	}
	switch {
	case name == "":
		r.refuse(i, &Error{Code: CodeInvalid, Field: "name",
			Message: fmt.Sprintf("a %s document names each block by its key; give the new block one in name", p.info.Format)})
		return false
	case name == anchor:
		r.refuse(i, &Error{Code: CodeInvalid, Field: "name", Message: fmt.Sprintf("the new block %s cannot sit beside itself", name)})
		return false
	}
	runs, err := p.buildInsert(body)
	if err != nil {
		r.refuse(i, err)
		return false
	}
	if _, ok := runs[model.EditionKey{}]; !ok {
		r.refuse(i, &Error{Code: CodeInvalid, Field: "editions",
			Message: fmt.Sprintf("the new block needs the content of %s's own edition (%s)", p.info.Doc, localeText(p.info.SourceLocale))})
		return false
	}
	for _, k := range slices.SortedFunc(maps.Keys(runs), compareKeys) {
		if k.IsZero() {
			continue
		}
		switch place := p.sess.Place(k); place.Kind {
		case PlaceOwnFile:
			p.wantEdition(k)
		case PlaceInDocument:
			r.refuse(i, &Error{Code: CodeUnsupported, Capability: string(KindInsertBlock), Field: "editions/" + keyText(k),
				Message: fmt.Sprintf("%s holds edition %s in the document, and insert_block writes a new block's own edition and the editions kept in files of their own", p.info.Doc, keyText(k))})
			return false
		default:
			why := place.Why
			if why == "" {
				why = "the document holds one edition, and outside a project there is no file for another"
			}
			r.refuse(i, &Error{Code: CodeUnsupported, Capability: "edition", Field: "editions/" + keyText(k),
				Message: fmt.Sprintf("edition %s of %s has nowhere to live: %s", keyText(k), p.info.Doc, why)})
			return false
		}
	}
	p.inserts[name] = i
	p.newRuns[i] = runs
	p.structKeys[name] = true
	p.want.Blocks = appendNew(p.want.Blocks, name)
	if anchor != "" {
		p.structKeys[anchor] = true
		p.want.Blocks = appendNew(p.want.Blocks, anchor)
	}
	return true
}

// buildInsert applies an insert_block's editions to an empty block under the
// rules every content operation obeys, and returns the content of each: the
// document's own edition under the zero key, each other under its key.
func (p *docPlan) buildInsert(body *InsertBlock) (map[model.EditionKey][]model.Run, *Error) {
	nb := &model.Block{ID: "new", Name: body.Name, SourceLocale: p.info.SourceLocale, Translatable: true, Properties: map[string]string{}}
	var ops []Op
	var keys []model.EditionKey
	for _, text := range slices.Sorted(maps.Keys(body.Editions)) {
		k, err := model.ParseEditionKey(text)
		if err != nil {
			return nil, &Error{Code: CodeInvalid, Field: "editions/" + text, Message: err.Error()}
		}
		c := body.Editions[text]
		ops = append(ops, Op{Kind: KindSetContent, At: Ref{Doc: p.info.Doc, Block: body.Name, Edition: k}, IfMatch: AnyRevision,
			Body: &SetContent{Content: c}})
		keys = append(keys, k)
	}
	for j, res := range ApplyBlock(nb, ops, p.env(false)) {
		if res.Status == OpRefused && res.Error != nil {
			e := *res.Error
			e.Field = "editions/" + keyText(keys[j])
			return nil, &e
		}
	}
	out := map[model.EditionKey][]model.Run{}
	for _, k := range keys {
		ed, _ := nb.Edition(k)
		at := k
		if nb.IsSourceEdition(k) {
			at = model.EditionKey{}
		}
		if _, dup := out[at.Canonical()]; dup {
			return nil, &Error{Code: CodeInvalid, Field: "editions/" + keyText(k),
				Message: fmt.Sprintf("editions names edition %s twice", editionLabel(nb, k))}
		}
		out[at.Canonical()] = ed.Runs
	}
	return out, nil
}

// wantEdition asks the home to join edition k from its own file.
func (p *docPlan) wantEdition(k model.EditionKey) {
	if !slices.ContainsFunc(p.want.Editions, func(x model.EditionKey) bool { return x.Canonical() == k.Canonical() }) {
		p.want.Editions = append(p.want.Editions, k.Canonical())
	}
}

// conflicts refuses the operations that address a block the change set adds
// or removes: the content of a new block goes in its insert_block, and a
// block removed takes every edition with it.
func (r *applyRun) conflicts(p *docPlan) {
	for key, ops := range p.byKey {
		j, inserted := p.inserts[key]
		if !inserted {
			var removed bool
			if j, removed = p.deletes[key]; !removed {
				continue
			}
		}
		for _, i := range ops {
			msg := fmt.Sprintf("operation %d removes block %s in this change set", j, key)
			if inserted {
				msg = fmt.Sprintf("operation %d adds block %s in this change set; send its content in that operation's editions", j, key)
			}
			r.refuse(i, &Error{Code: CodeInvalid, Field: "at/block", Message: msg})
		}
	}
}

// located is a block the read before a pass found under a key a structural
// operation names.
type located struct {
	block *model.Block
	index int
}

func (p *docPlan) StartStructure() {
	p.structured = true
	p.located = map[string][]located{}
	p.locIndex = 0
	p.structChanges, p.structChangeOps = nil, nil
	p.added, p.removed = map[string]int{}, map[string]int{}
	p.structCands = map[string]*candidates{}
	for _, i := range p.structural {
		op := p.set.Ops[i]
		p.results[i] = OpResult{I: i, Op: op.Kind, At: refOf(op)}
		for _, k := range p.structuralTargets(op) {
			p.structCands[k] = &candidates{key: k}
		}
	}
}

// structuralTargets are the keys of the blocks an operation needs to find:
// the block it removes, or the block a new one goes beside.
func (p *docPlan) structuralTargets(op Op) []string {
	switch body := op.Body.(type) {
	case *DeleteBlock:
		return []string{op.At.Block}
	case *InsertBlock:
		if body.After != "" {
			return []string{body.After}
		}
		if body.Before != "" {
			return []string{body.Before}
		}
	}
	return nil
}

func (p *docPlan) Locate(b *model.Block) {
	index := p.locIndex
	p.locIndex++
	keys := blockKeys(b)
	var own *model.Block
	for _, k := range keys {
		if !p.structKeys[k] {
			continue
		}
		if own == nil {
			own = copyBlock(b)
		}
		p.located[k] = append(p.located[k], located{block: own, index: index})
	}
	if own == nil {
		k := BlockKey(b)
		for _, c := range p.structCands {
			c.offer(k, b)
		}
	}
}

func (p *docPlan) Structure() ([]StructuralEdit, error) {
	var edits []StructuralEdit
	for _, i := range p.structural {
		op := p.set.Ops[i]
		switch body := op.Body.(type) {
		case *DeleteBlock:
			if e, ok := p.structureDelete(i, op, body); ok {
				edits = append(edits, e)
			}
		case *InsertBlock:
			if e, ok := p.structureInsert(i, body); ok {
				edits = append(edits, e)
			}
		}
	}
	if p.refusedAny(p.structural) {
		return nil, ErrRefused
	}
	return edits, nil
}

// find returns the one block the read found under key, or refuses i.
func (p *docPlan) find(i int, key, field string) (located, bool) {
	locs := p.located[key]
	switch {
	case len(locs) == 1:
		return locs[0], true
	case len(locs) > 1:
		cands := make([]Candidate, 0, len(locs))
		for _, l := range locs {
			cands = append(cands, candidateOf(l.block))
		}
		p.refuse(i, &Error{Code: CodeAmbiguous, Field: field, Candidates: cands,
			Message: fmt.Sprintf("%s holds more than one block keyed %q", p.info.Doc, key)})
	default:
		var cands []Candidate
		if c := p.structCands[key]; c != nil {
			cands = c.list()
		}
		p.refuse(i, &Error{Code: CodeNotFound, Field: field, Candidates: cands,
			Message: fmt.Sprintf("%s holds no block keyed %q", p.info.Doc, key)})
	}
	return located{}, false
}

func (p *docPlan) structureDelete(i int, op Op, body *DeleteBlock) (StructuralEdit, bool) {
	key := op.At.Block
	if j, ok := p.added[key]; ok {
		p.refuse(i, &Error{Code: CodeInvalid, Field: "at/block",
			Message: fmt.Sprintf("operation %d adds block %s in this change set; leave both out", j, key)})
		return StructuralEdit{}, false
	}
	if j, ok := p.removed[key]; ok {
		p.refuse(i, &Error{Code: CodeNotFound, Field: "at/block", Message: fmt.Sprintf("operation %d removes block %s already", j, key)})
		return StructuralEdit{}, false
	}
	loc, ok := p.find(i, key, "at/block")
	if !ok {
		return StructuralEdit{}, false
	}
	b := loc.block
	if !b.Translatable {
		p.refuse(i, &Error{Code: CodeUnsupported, Capability: "editable",
			Message: fmt.Sprintf("%s marks block %s as content an edit does not change, such as code", p.info.Doc, key)})
		return StructuralEdit{}, false
	}
	if err, cur := p.deletePrecondition(b, body); err != nil {
		p.refuse(i, err)
		p.results[i].Current = cur
		return StructuralEdit{}, false
	}
	// The block is removed under the key the operation names and the keys
	// it keeps across the edit, which the pass after reads it by.
	p.removed[key] = i
	for _, k := range structureKeys(b) {
		p.removed[k] = i
	}
	authKey := b.EditionKeyOf(b.Authoritative(model.AuthorityPolicy{}))
	for _, k := range b.Editions() {
		ed, _ := b.Edition(k)
		role := RoleDerived
		if k == authKey {
			role = RoleAuthoritative
		}
		p.structChanges = append(p.structChanges, EditionChange{Ref: *p.canonical(b, k), Role: role, Key: BlockKey(b),
			Before: ed.Runs, BeforeRev: model.EditionRevision(b, k), AfterRev: model.AbsentRevision, Block: b})
		p.structChangeOps = append(p.structChangeOps, []int{i})
	}
	res := &p.results[i]
	res.Status = OpApplied
	res.At = p.canonical(b, model.EditionKey{})
	res.Before = model.EditionRevision(b, model.EditionKey{})
	return StructuralEdit{Kind: KindDeleteBlock, Key: BlockKey(b), Block: b, Index: loc.index, AnchorIndex: -1, op: i}, true
}

// deletePrecondition checks delete_block's revisions: the map names the
// block's own edition, and every edition it names is at the revision named.
// An edition the map leaves out goes with the block whatever it holds.
func (p *docPlan) deletePrecondition(b *model.Block, body *DeleteBlock) (*Error, *Current) {
	named := map[model.EditionKey]string{}
	for _, text := range slices.Sorted(maps.Keys(body.IfMatch)) {
		k, err := model.ParseEditionKey(text)
		if err != nil {
			return &Error{Code: CodeInvalid, Field: "if_match/" + text, Message: err.Error()}, nil
		}
		ek := b.EditionKeyOf(k)
		if _, dup := named[ek]; dup {
			return &Error{Code: CodeInvalid, Field: "if_match/" + text,
				Message: fmt.Sprintf("if_match names edition %s twice", editionLabel(b, ek))}, nil
		}
		named[ek] = body.IfMatch[text]
	}
	key := BlockKey(b)
	own := b.EditionKeyOf(model.EditionKey{})
	if _, ok := named[own]; !ok {
		return &Error{Code: CodeInvalid, Field: "if_match/" + keyTextOf(b, own),
			Message: fmt.Sprintf("if_match names no revision of block %s's own edition %s; read the block and send the revision it reports", key, editionLabel(b, own))}, nil
	}
	for _, k := range slices.SortedFunc(maps.Keys(named), compareKeys) {
		want := named[k]
		ed, held := b.Edition(k)
		if !held {
			return &Error{Code: CodeStale, Field: "if_match/" + keyText(k),
				Message: fmt.Sprintf("block %s holds no edition %s", key, keyText(k))}, &Current{Rev: model.AbsentRevision}
		}
		if rev := model.EditionRevision(b, k); want != rev {
			return &Error{Code: CodeStale, Field: "if_match/" + keyTextOf(b, k),
				Message: fmt.Sprintf("edition %s of block %s is at %s, not %s", editionLabel(b, k), key, rev, want)}, &Current{Rev: rev, Text: model.RunsEditText(ed.Runs)}
		}
	}
	return nil, nil
}

func (p *docPlan) structureInsert(i int, body *InsertBlock) (StructuralEdit, bool) {
	name := body.Name
	if j, ok := p.added[name]; ok {
		p.refuse(i, &Error{Code: CodeInvalid, Field: "name", Message: fmt.Sprintf("operation %d adds a block keyed %s already", j, name)})
		return StructuralEdit{}, false
	}
	if _, removed := p.removed[name]; !removed {
		// A block holds the name when it keeps it across the edit; a
		// reader-local id that happens to read the same is renumbered.
		if at := slices.IndexFunc(p.located[name], func(l located) bool { return slices.Contains(structureKeys(l.block), name) }); at >= 0 {
			b := p.located[name][at].block
			ed, _ := b.Edition(model.EditionKey{})
			p.refuse(i, &Error{Code: CodeStale, Field: "name",
				Message: fmt.Sprintf("%s already holds a block keyed %s; change it with set_content", p.info.Doc, name)})
			p.results[i].Current = &Current{Rev: model.EditionRevision(b, model.EditionKey{}), Text: model.RunsEditText(ed.Runs)}
			return StructuralEdit{}, false
		}
	}
	e := StructuralEdit{Kind: KindInsertBlock, Key: name, Index: -1, AnchorIndex: -1, Editions: p.newRuns[i], op: i}
	e.Anchor, e.Before = body.After, false
	field := "after"
	if body.Before != "" {
		e.Anchor, e.Before, field = body.Before, true, "before"
	}
	if e.Anchor != "" {
		if _, ok := p.added[e.Anchor]; !ok {
			if j, gone := p.removed[e.Anchor]; gone {
				p.refuse(i, &Error{Code: CodeNotFound, Field: field,
					Message: fmt.Sprintf("operation %d removes block %s, so the new block cannot go beside it; add the new block first", j, e.Anchor)})
				return StructuralEdit{}, false
			}
			loc, ok := p.find(i, e.Anchor, field)
			if !ok {
				return StructuralEdit{}, false
			}
			e.Anchor, e.AnchorBlock, e.AnchorIndex = BlockKey(loc.block), loc.block, loc.index
		}
	}
	p.added[name] = i
	return e, true
}

func (p *docPlan) Refuse(e StructuralEdit, err *Error) {
	p.refuse(e.op, err)
}

// structuralOf reports the insert_block whose new block b is, or the
// delete_block that removed a block keyed as b is. Both compare the keys a
// block keeps across the edit: adding or removing a block renumbers the
// reader-local ids of the blocks after it.
func (p *docPlan) structuralOf(b *model.Block) (insert, remove int, ok bool) {
	keys := structureKeys(b)
	for _, k := range keys {
		if i, found := p.added[k]; found {
			return i, -1, true
		}
	}
	for _, k := range keys {
		if i, found := p.removed[k]; found {
			return -1, i, true
		}
	}
	return -1, -1, false
}

// spellValue is the value the document's writer writes for runs, an edition
// of b (nil for content no reader has read), as Capabilities.Values spells
// it.
func (p *docPlan) spellValue(b *model.Block, runs []model.Run) string {
	if v := p.info.Capabilities.Values; v != nil {
		return v.SpellValue(b, runs)
	}
	return model.RenderRunsWithData(runs)
}

// verifyInsert checks the new block b of insert_block i as the pass read it:
// each edition holds the content the operation gave it, as the format reads
// that content. An edition whose file holds no partner of the new block yet
// (a translation written for the first time) is given its content here, and
// its key returned as changed so the home writes that file.
func (p *docPlan) verifyInsert(i int, b *model.Block) []model.EditionKey {
	if !b.Translatable {
		p.refuse(i, &Error{Code: CodeUnsupported, Capability: string(KindInsertBlock),
			Message: fmt.Sprintf("the %s format reads the new block %s as content an edit does not change, so it cannot be added as a block", p.info.Format, BlockKey(b))})
		return nil
	}
	want := p.newRuns[i]
	authKey := b.EditionKeyOf(b.Authoritative(model.AuthorityPolicy{}))
	var changed []model.EditionKey
	var changes []EditionChange
	for _, k := range slices.SortedFunc(maps.Keys(want), compareKeys) {
		ed, held := b.Edition(k)
		if !held {
			b.SetEdition(k, model.Edition{Runs: want[k]})
			changed = append(changed, b.EditionKeyOf(k))
			ed, _ = b.Edition(k)
		} else if got, sent := p.spellValue(b, ed.Runs), p.spellValue(nil, want[k]); got != sent {
			p.refuse(i, &Error{Code: CodeUnsupported, Capability: string(KindInsertBlock), Field: "editions/" + keyTextOf(b, k),
				Message: fmt.Sprintf("the %s format reads edition %s of the new block %s as %q, not as given", p.info.Format, editionLabel(b, k), BlockKey(b), got)})
			return nil
		}
		role := RoleDerived
		if b.EditionKeyOf(k) == authKey {
			role = RoleAuthoritative
		}
		changes = append(changes, EditionChange{Ref: *p.canonical(b, k), Role: role, Key: BlockKey(b), After: ed.Runs,
			BeforeRev: model.AbsentRevision, AfterRev: model.EditionRevision(b, k), Block: b})
	}
	for _, ch := range changes {
		p.changes = append(p.changes, ch)
		p.changeOps = append(p.changeOps, []int{i})
	}
	res := &p.results[i]
	res.Status = OpApplied
	res.At = p.canonical(b, model.EditionKey{})
	res.After = model.EditionRevision(b, model.EditionKey{})
	return changed
}

// blockKeys are the keys a block answers to: its durable key, its name and
// its id.
func blockKeys(b *model.Block) []string {
	var out []string
	for _, k := range []string{b.Unit, b.Name, b.ID} {
		if k != "" && !slices.Contains(out, k) {
			out = append(out, k)
		}
	}
	return out
}

// structureKeys are the keys a block keeps when blocks are added to its
// document or removed: its durable key and its name, or its id when it has
// neither.
func structureKeys(b *model.Block) []string {
	var out []string
	for _, k := range []string{b.Unit, b.Name} {
		if k != "" && !slices.Contains(out, k) {
			out = append(out, k)
		}
	}
	if len(out) == 0 && b.ID != "" {
		out = append(out, b.ID)
	}
	return out
}

// copyBlock copies b so that what a home later does to it (removing the
// editions it joined) leaves the copy as it was read.
func copyBlock(b *model.Block) *model.Block {
	c := *b
	c.Targets = maps.Clone(b.Targets)
	c.Properties = maps.Clone(b.Properties)
	return &c
}

// keyTextOf names edition k of b as an if_match map names it.
func keyTextOf(b *model.Block, k model.EditionKey) string {
	if b.IsSourceEdition(k) && k.IsZero() && b.SourceLocale != "" {
		return keyText(model.EditionKey{Locale: b.SourceLocale})
	}
	return keyText(k)
}

func compareKeys(a, b model.EditionKey) int {
	switch x, y := keyText(a), keyText(b); {
	case x < y:
		return -1
	case x > y:
		return 1
	}
	return 0
}

func appendNew(list []string, s string) []string {
	if slices.Contains(list, s) {
		return list
	}
	return append(list, s)
}

func localeText(l model.LocaleID) string {
	if l == "" {
		return "its language"
	}
	return string(l)
}
