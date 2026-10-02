package change

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"slices"
	"strconv"
	"time"

	"github.com/neokapi/neokapi/core/model"
)

// ActorKind says what kind of sender a change has.
type ActorKind string

const (
	// ActorPerson is someone using kapi, Kapi Desktop or a Bowrain editor.
	ActorPerson ActorKind = "person"
	// ActorAgent is an AI working through kapi's agent surfaces.
	ActorAgent ActorKind = "agent"
	// ActorTool is a tool running in a flow.
	ActorTool ActorKind = "tool"
)

// Actor is who sends a change. The transport sets it; a change set never
// names its own sender.
type Actor struct {
	Kind    ActorKind `json:"kind"`
	Name    string    `json:"name,omitempty"`
	Session string    `json:"session,omitempty"`
}

// Disposition says what a guard violation does to the operation that would
// cause it.
type Disposition string

const (
	// Enforce refuses the operation with a guard error. It is the default.
	Enforce Disposition = "enforce"
	// Report lands the operation and lists the violation among its findings.
	Report Disposition = "report"
)

// BlockEnv is what ApplyBlock needs besides a block and its operations.
type BlockEnv struct {
	// Actor is who sends the operations.
	Actor Actor
	// Authority names the authoritative edition. The zero policy is the
	// edition the block was read in.
	Authority model.AuthorityPolicy
	// RequireBasis refuses a write to a derived edition whose basis is not the
	// authoritative edition's revision when the operations began.
	RequireBasis bool
	// Preview computes every result and changes nothing.
	Preview bool
	// Guards is what an inline-code guard violation does, and with it a code
	// the reference does not hold and an overlay span that ends inside a
	// plural or select. A tool in a flow applies with Report: its drafts meet
	// the ship gates later, as its governance findings do.
	Guards Disposition
	// Vocabulary resolves a code's editing constraints when the code carries
	// none of its own. Nil is model.DefaultVocabulary.
	Vocabulary *model.VocabularyRegistry
	// Now is the clock that stamps a person's or an agent's edit. Nil is
	// time.Now.
	Now func() time.Time
}

// ApplyBlock applies content operations to one block in memory and returns
// one result per operation, in order. It is the one function that changes a
// block's content.
//
// The operations apply in order, and each sees the result of the operations
// before it. Every precondition (IfMatch, and Basis under RequireBasis) is
// checked first, against the block as it stood when ApplyBlock was called. The
// operations are all-or-nothing: when one is refused, the block is left as it
// was, the refused operations say why, and every other operation is
// not_applied with BlockedBy naming the first refusal.
//
// ApplyBlock applies set_content, replace_text, remove_edition, annotate,
// unannotate and the in-process provenance operation. set_attribute, mark,
// insert_block, delete_block and native need a capability a format declares,
// and are refused as unsupported here. decide and the asset operations are
// applied by the change service, not to a block, and are refused as invalid.
// The document and block an operation's At names are the caller's to route;
// ApplyBlock reads only the edition.
func ApplyBlock(b *model.Block, ops []Op, env BlockEnv) []OpResult {
	results := make([]OpResult, len(ops))
	w := newWorkset(b, env)

	refused := -1
	refuse := func(i int, err *Error) {
		results[i].Status = OpRefused
		results[i].Error = err
		if refused < 0 || i < refused {
			refused = i
		}
	}
	for i, op := range ops {
		results[i] = OpResult{I: i, Op: op.Kind}
		if op.Kind != KindTerm && op.Kind != KindMemory && op.Kind != KindRecipe && op.Kind != KindInsertBlock && op.Kind != KindNative {
			at := op.At
			results[i].At = &at
		}
		if err := w.admit(op); err != nil {
			refuse(i, err)
			continue
		}
		if err, cur := w.precondition(op); err != nil {
			refuse(i, err)
			results[i].Current = cur
		}
	}

	if refused < 0 {
		for i, op := range ops {
			if err := w.apply(op, &results[i]); err != nil {
				refuse(i, err)
				break
			}
		}
	}

	if refused >= 0 {
		for i := range results {
			if results[i].Status == OpRefused {
				continue
			}
			results[i] = OpResult{I: i, Op: results[i].Op, At: results[i].At, Status: OpNotApplied, BlockedBy: &refused}
		}
		return results
	}
	if env.Preview {
		for i := range results {
			if results[i].Status == OpApplied {
				results[i].Status = OpPreviewed
			}
		}
		return results
	}
	w.commit()
	return results
}

// admit refuses an operation ApplyBlock does not apply, or one whose shape is
// wrong.
func (w *workset) admit(op Op) *Error {
	spec, ok := specOf(op.Kind)
	if !ok {
		return errorf(CodeInvalid, "unknown operation %q", op.Kind)
	}
	if op.Body == nil || op.Body.opKind() != op.Kind {
		return errorf(CodeInvalid, "%s operation has a %T body", op.Kind, op.Body)
	}
	switch op.Kind {
	case KindSetAttribute, KindMark, KindInsertBlock, KindDeleteBlock, KindNative:
		return &Error{Code: CodeUnsupported, Capability: string(op.Kind),
			Message: fmt.Sprintf("%s needs a capability the format declares; no format declares it for a block in memory", op.Kind)}
	case KindDecide, KindTerm, KindMemory, KindRecipe:
		return errorf(CodeInvalid, "%s is applied by the change service, not to a block", op.Kind)
	case KindProvenance:
		if w.env.Actor.Kind != ActorTool {
			return errorf(CodeNotPermitted, "only a tool in a flow records provenance")
		}
	}
	if err := validateBody(op, ""); err != nil {
		return err
	}
	if spec.ifMatch == ifMatchRequired && op.IfMatch == "" {
		return &Error{Code: CodeInvalid, Field: "if_match",
			Message: `if_match is required: send the revision you read, "absent" to create, or "*" to write whatever is there`}
	}
	if op.IfMatch != "" {
		if err := checkIfMatch(spec.ifMatch, op.IfMatch, ""); err != nil {
			err.Field = "if_match"
			return err
		}
	}
	if op.Basis != "" && !spec.basis {
		return &Error{Code: CodeInvalid, Field: "basis", Message: fmt.Sprintf("%s takes no basis; only set_content records one", op.Kind)}
	}
	if op.Basis != "" && !validRevision(op.Basis) {
		return &Error{Code: CodeInvalid, Field: "basis", Message: fmt.Sprintf("%q is not a revision", op.Basis)}
	}
	return nil
}

// precondition checks an operation's IfMatch and, under RequireBasis, its
// Basis against the block as it stood at the start.
func (w *workset) precondition(op Op) (*Error, *Current) {
	if op.Kind == KindProvenance || op.Kind == KindUnannotate {
		return nil, nil
	}
	st := w.state(op.At.Edition)
	switch op.IfMatch {
	case "", AnyRevision:
	case model.AbsentRevision:
		if op.Kind != KindSetContent {
			return &Error{Code: CodeInvalid, Field: "if_match", Message: fmt.Sprintf("%s changes an edition that exists; absent names one that does not", op.Kind)}, nil
		}
		if st.startPresent {
			return &Error{Code: CodeStale, Field: "if_match",
				Message: fmt.Sprintf("edition %s exists at %s", w.label(st), st.startRevision())}, st.startCurrent()
		}
	default:
		if rev := st.startRevision(); rev != op.IfMatch {
			return &Error{Code: CodeStale, Field: "if_match",
				Message: fmt.Sprintf("edition %s is at %s, not %s", w.label(st), rev, op.IfMatch)}, st.startCurrent()
		}
	}
	if op.Kind != KindSetContent {
		return nil, nil
	}
	if w.role(st) == RoleAuthoritative {
		if op.Basis != "" {
			return &Error{Code: CodeInvalid, Field: "basis",
				Message: "basis names the authoritative edition a derived edition was made from; the authoritative edition has none"}, nil
		}
		return nil, nil
	}
	if !w.env.RequireBasis {
		return nil, nil
	}
	if op.Basis == "" {
		return &Error{Code: CodeInvalid, Field: "basis", Message: "require_basis needs the basis of every derived-edition write"}, nil
	}
	auth := w.state(w.auth)
	if rev := auth.startRevision(); rev != op.Basis {
		return &Error{Code: CodeStale, Field: "basis",
			Message: fmt.Sprintf("the authoritative edition %s is at %s, not %s", w.label(auth), rev, op.Basis)}, auth.startCurrent()
	}
	return nil, nil
}

// apply applies one admitted operation to the workset.
func (w *workset) apply(op Op, res *OpResult) *Error {
	switch body := op.Body.(type) {
	case *SetContent:
		return w.setContent(op, body, res)
	case *ReplaceText:
		return w.replaceText(op, body, res)
	case *RemoveEdition:
		return w.removeEdition(op, res)
	case *Annotate:
		return w.annotate(op, body, res)
	case *Unannotate:
		return w.unannotate(op, body, res)
	case *Provenance:
		return w.provenance(op, body, res)
	}
	return errorf(CodeInvalid, "%s is not applied to a block", op.Kind)
}

// edState is one edition of the block as the operations see it.
type edState struct {
	key model.EditionKey
	// slot is the Variant of the edition's overlays: nil for the edition Source
	// holds.
	slot *model.VariantKey

	startPresent bool
	startRuns    []model.Run
	startRev     string

	present  bool
	ed       model.Edition
	overlays []model.Overlay

	content         bool
	removed         bool
	overlaysChanged bool
}

func (st *edState) startRevision() string {
	if st.startRev == "" {
		if !st.startPresent {
			st.startRev = model.AbsentRevision
		} else {
			st.startRev = model.RunsRevision(st.key, st.startRuns)
		}
	}
	return st.startRev
}

func (st *edState) startCurrent() *Current {
	if !st.startPresent {
		return &Current{Rev: model.AbsentRevision}
	}
	return &Current{Rev: st.startRevision(), Text: model.RunsEditText(st.startRuns)}
}

func (st *edState) revision() string {
	if !st.present {
		return model.AbsentRevision
	}
	return model.RunsRevision(st.key, st.ed.Runs)
}

// workset holds the editions the operations touch, so nothing reaches the
// block until every operation has applied.
type workset struct {
	b    *model.Block
	env  BlockEnv
	auth model.EditionKey
	// states are the editions touched so far, in the order first touched. A
	// block has few editions, so a list is searched rather than a map built.
	states []*edState
}

func newWorkset(b *model.Block, env BlockEnv) *workset {
	return &workset{b: b, env: env, auth: b.EditionKeyOf(b.Authoritative(env.Authority))}
}

// state returns the working state of the edition k reaches.
func (w *workset) state(k model.EditionKey) *edState {
	key := w.b.EditionKeyOf(k)
	for _, st := range w.states {
		if st.key == key {
			return st
		}
	}
	ed, ok := w.b.Edition(k)
	st := &edState{key: key, startPresent: ok, startRuns: ed.Runs, present: ok, ed: ed}
	if !w.b.IsSourceEdition(k) {
		slot := key
		st.slot = &slot
	}
	for _, o := range w.b.Overlays {
		if overlayOn(o, st.slot) {
			o.Spans = slices.Clone(o.Spans)
			st.overlays = append(st.overlays, o)
		}
	}
	w.states = append(w.states, st)
	return st
}

func (w *workset) role(st *edState) Role {
	if st.key == w.auth {
		return RoleAuthoritative
	}
	return RoleDerived
}

// label names an edition in a message.
func (w *workset) label(st *edState) string {
	if t := keyText(st.key); t != "" {
		return t
	}
	return "(the document's own)"
}

func overlayOn(o model.Overlay, slot *model.VariantKey) bool {
	if slot == nil || o.Variant == nil {
		return slot == nil && o.Variant == nil
	}
	return o.Variant.Canonical() == slot.Canonical()
}

// commit writes every changed edition and its overlays to the block.
func (w *workset) commit() {
	for _, st := range w.states {
		switch {
		case st.removed:
			w.b.RemoveEdition(st.key)
		case st.content:
			w.b.SetEdition(st.key, st.ed)
		}
		if st.overlaysChanged {
			replaceOverlays(w.b, st.slot, st.overlays)
		}
	}
}

// replaceOverlays puts overlays in place of the block's overlays on one
// edition, where the first of them stood.
func replaceOverlays(b *model.Block, slot *model.VariantKey, overlays []model.Overlay) {
	out := make([]model.Overlay, 0, len(b.Overlays)+len(overlays))
	placed := false
	for _, o := range b.Overlays {
		if !overlayOn(o, slot) {
			out = append(out, o)
			continue
		}
		if !placed {
			out = append(out, overlays...)
			placed = true
		}
	}
	if !placed {
		out = append(out, overlays...)
	}
	b.Overlays = out
}

// now is the clock that stamps an edit.
func (w *workset) now() time.Time {
	if w.env.Now != nil {
		return w.env.Now()
	}
	return time.Now().UTC()
}

// rewrite moves an edition to new runs: its overlays follow, its status and
// origin take the consequences of the edit, and the result records the
// revisions and what the edit invalidated.
func (w *workset) rewrite(st *edState, newRuns []model.Run, rebase OverlayRebase, res *OpResult) *Error {
	old := st.ed.Runs
	if len(st.overlays) > 0 {
		switch {
		case rebase.Drop:
			st.overlays = nil
		default:
			edits := rebase.Edits
			if edits == nil {
				edits = diffEdits(old, newRuns)
			}
			tmp := &model.Block{Overlays: st.overlays}
			model.RemapOverlays(tmp, st.slot, old, newRuns, edits)
			st.overlays = tmp.Overlays
		}
		for _, o := range st.overlays {
			for _, s := range o.Spans {
				if !s.Range.Resolves(newRuns) {
					return guardf(SubcodeBadPosition, "the %s overlay on edition %s would point outside the new content", o.Type, w.label(st))
				}
			}
		}
		st.overlaysChanged = true
	}
	role := w.role(st)
	c := Consequences(w.env.Actor, role, st.ed, !st.present, w.now())
	st.ed = model.Edition{Runs: newRuns, Status: c.Status, Origin: c.Origin, Score: st.ed.Score}
	st.present, st.content, st.removed = true, true, false
	res.Before = st.startRevision()
	res.After = st.revision()
	res.Status = OpApplied
	if role == RoleAuthoritative {
		res.Invalidates = w.invalidated()
	}
	return nil
}

// invalidated lists the derived editions a change to the authoritative
// edition leaves on an older basis.
func (w *workset) invalidated() []Invalidation {
	var out []Invalidation
	for _, k := range w.b.Editions() {
		st := w.state(k)
		if st.key == w.auth || !st.present {
			continue
		}
		out = append(out, Invalidation{Edition: keyText(st.key), Reason: ReasonBasisMoved})
	}
	for _, st := range w.states {
		if st.key == w.auth || !st.present || st.startPresent {
			continue
		}
		out = append(out, Invalidation{Edition: keyText(st.key), Reason: ReasonBasisMoved})
	}
	return out
}

// setContent applies set_content.
func (w *workset) setContent(op Op, body *SetContent, res *OpResult) *Error {
	st := w.state(op.At.Edition)
	role := w.role(st)
	cur := st.ed.Runs
	if len(body.Path) > 0 && !st.present {
		return &Error{Code: CodeNotFound, Field: "path", Message: fmt.Sprintf("edition %s does not exist; create it whole, then edit a branch", w.label(st))}
	}
	if !st.present && op.IfMatch != model.AbsentRevision && op.IfMatch != AnyRevision {
		return &Error{Code: CodeNotFound, Message: fmt.Sprintf("edition %s does not exist", w.label(st))}
	}

	// The reference code set: the content being replaced, or, for a new
	// edition, the authoritative edition's. A derived edition may also take
	// back a code the authoritative edition holds.
	var ref []model.Run
	switch {
	case len(body.Path) > 0:
		seq, ok := model.ResolveRunPath(cur, body.Path)
		if !ok {
			branch, ok := newBranchReference(cur, body.Path)
			if !ok {
				return &Error{Code: CodeNotFound, Field: "path", Message: fmt.Sprintf("path %s reaches no plural form or select case of edition %s", pathText(body.Path), w.label(st))}
			}
			seq = branch
		}
		ref = seq
	case st.present:
		ref = cur
	default:
		ref = w.state(w.auth).ed.Runs
	}
	var restorable []model.Run
	if role == RoleDerived {
		restorable = w.state(w.auth).ed.Runs
	}

	var seq []model.Run
	var findings []Finding
	var err *Error
	if body.Text != nil {
		if model.HasStructuredRuns(ref) {
			return guardf(SubcodeStructureLost, "edition %s holds a plural or select; replace one branch with path, or the whole structure with runs", w.label(st))
		}
		parsed := model.ParseRunsEditText(*body.Text, ref)
		seq, findings, err = resolveTextCodes(parsed, ref, restorable, w.env.Guards == Report)
	} else {
		seq, findings, err = reconcileRuns(body.Runs, ref, restorable, w.env.Guards == Report)
	}
	if err != nil {
		return err
	}
	res.Findings = append(res.Findings, findings...)
	if err = w.checkCodes(ref, seq, restorable, res); err != nil {
		return err
	}

	newRuns := seq
	if len(body.Path) > 0 {
		var ok bool
		if newRuns, ok = replaceAtPath(cur, body.Path, seq); !ok {
			return &Error{Code: CodeNotFound, Field: "path", Message: fmt.Sprintf("path %s reaches no plural form or select case", pathText(body.Path))}
		}
	}
	if role == RoleDerived {
		res.Basis = op.Basis
		if res.Basis == "" {
			// The derived edition is made against the authoritative edition as
			// the operations before this one left it.
			auth := w.state(w.auth)
			res.Basis = auth.startRevision()
			if auth.content {
				res.Basis = auth.revision()
			}
		}
	}
	if st.present && sameRuns(cur, newRuns) {
		rev := st.revision()
		res.Before, res.After, res.Status = st.startRevision(), rev, OpUnchanged
		return nil
	}
	return w.rewrite(st, newRuns, body.Overlays, res)
}

// replaceText applies replace_text.
func (w *workset) replaceText(op Op, body *ReplaceText, res *OpResult) *Error {
	st := w.state(op.At.Edition)
	if !st.present {
		return &Error{Code: CodeNotFound, Message: fmt.Sprintf("edition %s does not exist", w.label(st))}
	}
	cur := st.ed.Runs

	type pathEdit struct {
		i          int
		start, end int
		span       Resolved
		text       string
	}
	groups := map[string][]pathEdit{}
	paths := map[string]model.RunPath{}
	res.Resolved = make([]Resolved, len(body.Edits))
	for i, e := range body.Edits {
		field := "edits/" + strconv.Itoa(i)
		seq, ok := model.ResolveRunPath(cur, e.Path)
		if !ok {
			return &Error{Code: CodeNotFound, Field: field + "/path", Message: fmt.Sprintf("path %s reaches no plural form or select case", pathText(e.Path))}
		}
		start, end, err := resolveSelection(seq, e.Selection, e.Path, field)
		if err != nil {
			return err
		}
		span := Resolved{Path: e.Path, Start: posAt(seq, start), End: posAt(seq, end)}
		res.Resolved[i] = span
		key := pathText(e.Path)
		paths[key] = e.Path
		groups[key] = append(groups[key], pathEdit{i: i, start: start, end: end, span: span, text: e.Text})
	}

	// Edit the deepest sequences first: an edit at one level can renumber the
	// runs a deeper path walks through.
	keys := sortedKeys(groups)
	slices.SortStableFunc(keys, func(a, b string) int { return len(paths[b]) - len(paths[a]) })
	next := cur
	var topEdits []model.RunEdit
	for _, key := range keys {
		edits := groups[key]
		slices.SortStableFunc(edits, func(a, b pathEdit) int { return a.start - b.start })
		for j := 1; j < len(edits); j++ {
			if edits[j].start < edits[j-1].end {
				return &Error{Code: CodeGuard, Subcode: SubcodeOverlap, Field: "edits/" + strconv.Itoa(edits[j].i),
					Message: fmt.Sprintf("edit %d overlaps edit %d", edits[j].i, edits[j-1].i)}
			}
		}
		path := paths[key]
		seq, _ := model.ResolveRunPath(next, path)
		textEdits := make([]model.TextEdit, len(edits))
		for j, e := range edits {
			textEdits[j] = model.TextEdit{Start: e.start, End: e.end, Replacement: e.text}
		}
		edited := model.ApplyTextEdits(seq, textEdits)
		if len(path) == 0 {
			next = edited
			for _, e := range edits {
				s, en := model.SpanAnchor(e.span.Start, e.span.End).TextSpan(cur)
				topEdits = append(topEdits, model.RunEdit{Start: s, End: en, NewLen: len([]rune(e.text))})
			}
			continue
		}
		var ok bool
		if next, ok = replaceAtPath(next, path, edited); !ok {
			return &Error{Code: CodeNotFound, Field: "edits", Message: fmt.Sprintf("path %s reaches no plural form or select case", key)}
		}
	}
	if err := w.checkCodes(cur, next, nil, res); err != nil {
		return err
	}
	if sameRuns(cur, next) {
		res.Before, res.After, res.Status = st.startRevision(), st.revision(), OpUnchanged
		return nil
	}
	rebase := OverlayRebase{}
	if len(keys) == 1 && keys[0] == pathText(nil) {
		rebase.Edits = topEdits
		if rebase.Edits == nil {
			rebase.Edits = []model.RunEdit{}
		}
	}
	return w.rewrite(st, next, rebase, res)
}

// removeEdition applies remove_edition.
func (w *workset) removeEdition(op Op, res *OpResult) *Error {
	st := w.state(op.At.Edition)
	if w.role(st) == RoleAuthoritative || w.b.IsSourceEdition(st.key) {
		return &Error{Code: CodeInvalid, Field: "at/edition", Message: fmt.Sprintf("edition %s is the one the document is written in; it cannot be removed", w.label(st))}
	}
	if !st.present {
		if op.IfMatch == AnyRevision {
			res.Before, res.After, res.Status = model.AbsentRevision, model.AbsentRevision, OpUnchanged
			return nil
		}
		return &Error{Code: CodeNotFound, Message: fmt.Sprintf("edition %s does not exist", w.label(st))}
	}
	res.Before = st.startRevision()
	st.present, st.removed, st.content = false, true, false
	st.ed = model.Edition{}
	if len(st.overlays) > 0 {
		st.overlays = nil
		st.overlaysChanged = true
	}
	res.After = model.AbsentRevision
	res.Status = OpApplied
	return nil
}

// annotate applies annotate.
func (w *workset) annotate(op Op, body *Annotate, res *OpResult) *Error {
	st := w.state(op.At.Edition)
	if !st.present {
		return &Error{Code: CodeNotFound, Message: fmt.Sprintf("edition %s does not exist", w.label(st))}
	}
	if body.Spans != nil || body.Replace {
		return w.writeSpans(st, body, res)
	}
	anchor := model.BlockAnchor()
	if body.Anchor != nil {
		anchor = *body.Anchor
	}
	if !anchor.Resolves(st.ed.Runs) {
		msg := "the anchor does not resolve in edition " + w.label(st)
		if w.env.Guards == Report && endsInsideStructure(anchor, st.ed.Runs) {
			res.Before = st.revision()
			res.After = res.Before
			res.Status = OpUnchanged
			res.Findings = append(res.Findings, Finding{Rule: "guard." + string(SubcodeBadPosition), Message: msg + "; the annotation is not written"})
			return nil
		}
		return &Error{Code: CodeGuard, Subcode: SubcodeBadPosition, Field: "anchor", Message: msg}
	}
	var value model.Payload
	if len(body.Value) > 0 {
		value = annotationValue(body.Type, body.Value)
	}
	res.Before = st.revision()
	res.After = res.Before

	typ := model.OverlayType(body.Type)
	oi := -1
	for i, o := range st.overlays {
		if o.Type == typ && o.Layer == model.LayerPrimary {
			oi = i
			break
		}
	}
	if oi < 0 {
		st.overlays = append(st.overlays, model.Overlay{Type: typ, Variant: st.slot})
		oi = len(st.overlays) - 1
	}
	o := &st.overlays[oi]
	id := body.ID
	if id == "" {
		id = mintID(body.Type, o.Spans)
	}
	span := model.Span{ID: id, Range: anchor, Value: value}
	res.ID = id
	for i, s := range o.Spans {
		if s.ID != id {
			continue
		}
		if reflect.DeepEqual(s.Range, span.Range) && samePayload(s.Value, span.Value) {
			res.Status = OpUnchanged
			return nil
		}
		o.Spans[i] = span
		st.overlaysChanged = true
		res.Status = OpApplied
		return nil
	}
	o.Spans = append(o.Spans, span)
	st.overlaysChanged = true
	res.Status = OpApplied
	return nil
}

// writeSpans writes the whole spans of an in-process annotate to the
// edition's overlay of the type and layer.
//
// A span must resolve in the edition. A detector that anchors over the
// flattened text (model.RunsText, which reads a plural or select through its
// other branch) can place an end inside that structure, where no run position
// reaches. Under Report such a span is left out and named among the findings,
// and the spans that resolve are written; a segmentation layer, which the
// bilingual writers read as the edition's whole segment list, is written whole
// or not at all. Any other span that does not resolve, and every one under
// Enforce, refuses the operation.
func (w *workset) writeSpans(st *edState, body *Annotate, res *OpResult) *Error {
	spans := body.Spans
	var skipped []model.Span
	for i, s := range body.Spans {
		if s.Range.Resolves(st.ed.Runs) {
			continue
		}
		if w.env.Guards != Report || !endsInsideStructure(s.Range, st.ed.Runs) {
			return &Error{Code: CodeGuard, Subcode: SubcodeBadPosition, Field: "spans/" + strconv.Itoa(i),
				Message: fmt.Sprintf("the %s span %q does not resolve in edition %s", body.Type, s.ID, w.label(st))}
		}
		skipped = append(skipped, s)
	}
	res.Before = st.revision()
	res.After = res.Before
	if len(skipped) > 0 {
		if model.OverlayType(body.Type) == model.OverlaySegmentation {
			res.Status = OpUnchanged
			res.Findings = append(res.Findings, Finding{Rule: "guard." + string(SubcodeBadPosition),
				Message: fmt.Sprintf("the segmentation span %q does not resolve in edition %s; a segmentation layer is written whole, so none of its %d spans is written", skipped[0].ID, w.label(st), len(body.Spans))})
			return nil
		}
		spans = make([]model.Span, 0, len(body.Spans)-len(skipped))
		for _, s := range body.Spans {
			if s.Range.Resolves(st.ed.Runs) {
				spans = append(spans, s)
			}
		}
		for _, s := range skipped {
			res.Findings = append(res.Findings, Finding{Rule: "guard." + string(SubcodeBadPosition),
				Message: fmt.Sprintf("the %s span %q does not resolve in edition %s and is not written", body.Type, s.ID, w.label(st))})
		}
		if len(spans) == 0 && !body.Replace {
			res.Status = OpUnchanged
			return nil
		}
	}
	res.Status = OpApplied
	typ := model.OverlayType(body.Type)
	oi := slices.IndexFunc(st.overlays, func(o model.Overlay) bool { return o.Type == typ && o.Layer == body.Layer })
	switch {
	case body.Replace && len(spans) == 0:
		if oi < 0 {
			res.Status = OpUnchanged
			return nil
		}
		st.overlays = slices.Delete(st.overlays, oi, oi+1)
	case oi < 0:
		st.overlays = append(st.overlays, model.Overlay{Type: typ, Variant: st.slot, Layer: body.Layer, Spans: slices.Clone(spans)})
	case body.Replace:
		st.overlays[oi].Spans = slices.Clone(spans)
	default:
		st.overlays[oi].Spans = append(st.overlays[oi].Spans, spans...)
	}
	st.overlaysChanged = true
	return nil
}

// unannotate applies unannotate.
func (w *workset) unannotate(op Op, body *Unannotate, res *OpResult) *Error {
	st := w.state(op.At.Edition)
	typ := model.OverlayType(body.Type)
	if body.All {
		res.Before = st.revision()
		res.After = res.Before
		kept := slices.DeleteFunc(slices.Clone(st.overlays), func(o model.Overlay) bool { return o.Type == typ })
		if len(kept) == len(st.overlays) {
			res.Status = OpUnchanged
			return nil
		}
		st.overlays = kept
		st.overlaysChanged = true
		res.Status = OpApplied
		return nil
	}
	for oi := range st.overlays {
		o := &st.overlays[oi]
		if o.Type != typ {
			continue
		}
		for i, s := range o.Spans {
			if s.ID != body.ID {
				continue
			}
			o.Spans = slices.Delete(o.Spans, i, i+1)
			if len(o.Spans) == 0 {
				st.overlays = slices.Delete(st.overlays, oi, oi+1)
			}
			st.overlaysChanged = true
			res.Before = st.revision()
			res.After = res.Before
			res.Status = OpApplied
			return nil
		}
	}
	return &Error{Code: CodeNotFound, Field: "id", Message: fmt.Sprintf("edition %s has no %s annotation %q", w.label(st), body.Type, body.ID)}
}

// provenance applies the in-process provenance operation. It records how a
// tool produced a derived edition, on the target ladder, so it never reaches
// the edition the block was read in.
func (w *workset) provenance(op Op, body *Provenance, res *OpResult) *Error {
	st := w.state(op.At.Edition)
	if st.slot == nil {
		return &Error{Code: CodeInvalid, Field: "at/edition",
			Message: fmt.Sprintf("provenance records how a tool produced a derived edition; edition %s is the one the block was read in", w.label(st))}
	}
	rev := st.revision()
	res.Before, res.After = rev, rev
	if !st.present {
		res.Status = OpUnchanged
		return nil
	}
	next := st.ed
	next.Status, next.Origin = body.Status, body.Origin
	if body.Score != nil {
		next.Score = *body.Score
	}
	if next.Status == st.ed.Status && next.Origin == st.ed.Origin && next.Score == st.ed.Score {
		res.Status = OpUnchanged
		return nil
	}
	st.ed = next
	st.content = true
	res.Status = OpApplied
	return nil
}

// mintID returns the first id of the form <type>-<n> no span uses.
func mintID(typ string, spans []model.Span) string {
	used := make(map[string]bool, len(spans))
	for _, s := range spans {
		used[s.ID] = true
	}
	for n := 1; ; n++ {
		id := typ + "-" + strconv.Itoa(n)
		if !used[id] {
			return id
		}
	}
}

// annotationValue decodes an annotation payload into its registered type when
// that loses nothing, and keeps it verbatim otherwise.
func annotationValue(typ string, raw json.RawMessage) model.Payload {
	p := model.DecodePayload(typ, raw)
	if back, err := json.Marshal(p); err == nil && sameJSON(back, raw) {
		return p
	}
	return &model.RawAnnotation{Kind: typ, Body: append(json.RawMessage(nil), raw...)}
}

func samePayload(a, b model.Payload) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	ja, errA := json.Marshal(a)
	jb, errB := json.Marshal(b)
	return errA == nil && errB == nil && a.TypeName() == b.TypeName() && sameJSON(ja, jb)
}

func sameJSON(a, b []byte) bool {
	var va, vb any
	if json.Unmarshal(a, &va) != nil || json.Unmarshal(b, &vb) != nil {
		return bytes.Equal(a, b)
	}
	return reflect.DeepEqual(va, vb)
}

// sameRuns reports whether two run sequences are the same content.
func sameRuns(a, b []model.Run) bool {
	return bytes.Equal(model.CanonicalRunsJSON(a), model.CanonicalRunsJSON(b))
}

// pathText renders a run path as JSON, for messages and grouping.
func pathText(p model.RunPath) string {
	if len(p) == 0 {
		return "[]"
	}
	b, err := json.Marshal(p)
	if err != nil {
		return fmt.Sprint([]model.RunPathStep(p))
	}
	return string(b)
}
