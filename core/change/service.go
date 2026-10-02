package change

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/safeio"
)

// Service applies change sets to documents in their homes. Every surface (the
// CLI, MCP, Kapi Desktop, the browser build and the Bowrain server) builds one
// and calls it; nothing else writes content.
//
// Apply runs in two phases. It prepares every document a change set names
// (reads it, applies the operations in memory, checks the result) and refuses
// the whole set if any document refuses; only then does it commit each
// document, and after that apply the decisions and asset operations, which
// bind to the content that landed, and record the change.
type Service struct {
	formats  Formats
	homes    Homes
	check    CommitCheck
	policy   Policy
	recorder Recorder
	assets   Assets
	states   EditionStates
	caps     Capabilities
	origin   string
	now      func() time.Time
}

// Option configures a Service.
type Option func(*Service)

// WithCommitCheck sets the check that governs changed editions before
// anything is written. Without one nothing is checked.
func WithCommitCheck(c CommitCheck) Option { return func(s *Service) { s.check = c } }

// WithPolicy sets what decides which operations an actor may send. Without one
// every operation is permitted.
func WithPolicy(p Policy) Option { return func(s *Service) { s.policy = p } }

// WithRecorder sets where an applied change set is recorded. Without one
// nothing is recorded.
func WithRecorder(r Recorder) Option { return func(s *Service) { s.recorder = r } }

// WithAssets sets what applies decide and the asset operations. Without one
// they are refused as unsupported.
func WithAssets(a Assets) Option { return func(s *Service) { s.assets = a } }

// WithEditionStates sets what a read asks for the status and basis of a
// derived edition.
func WithEditionStates(e EditionStates) Option { return func(s *Service) { s.states = e } }

// WithCapabilities replaces DefaultCapabilities as what decides a format's
// operations.
func WithCapabilities(c Capabilities) Option { return func(s *Service) { s.caps = c } }

// WithOrigin names the surface that applies change sets through the service
// (apply, desktop, flow:<name>, merge, pull), for the record.
func WithOrigin(origin string) Option { return func(s *Service) { s.origin = origin } }

// WithClock sets the clock that stamps an edit.
func WithClock(now func() time.Time) Option { return func(s *Service) { s.now = now } }

// NewService returns a service over homes. formats answers what the service
// knows about a format, for Describe and for the operations a read lists.
func NewService(formats Formats, homes Homes, opts ...Option) *Service {
	s := &Service{formats: formats, homes: homes, caps: DefaultCapabilities, origin: "apply"}
	for _, o := range opts {
		o(s)
	}
	if s.caps == nil {
		s.caps = DefaultCapabilities
	}
	if s.now == nil {
		s.now = func() time.Time { return time.Now().UTC() }
	}
	return s
}

// open opens doc in its home.
func (s *Service) open(ctx context.Context, doc string) (Session, error) {
	if s.homes == nil {
		return nil, &Error{Code: CodeUnreachable, Message: "the change service has no home for documents"}
	}
	h, err := s.homes.For(doc)
	if err != nil {
		return nil, err
	}
	return h.Open(ctx, doc)
}

// asError turns an error a home returned into a refusal: an *Error as it is,
// a resource bound as budget_exceeded, a missing file as not_found. Any other
// error is not a refusal and is returned as nil.
func asError(err error) *Error {
	if err == nil {
		return nil
	}
	if e, ok := errors.AsType[*Error](err); ok {
		return e
	}
	if le, ok := errors.AsType[*safeio.LimitError](err); ok {
		return &Error{Code: CodeBudgetExceeded, Message: le.Error()}
	}
	for _, sentinel := range []error{safeio.ErrByteBudget, safeio.ErrTooDeep, safeio.ErrEntryTooLarge,
		safeio.ErrTotalTooLarge, safeio.ErrInflateRatio, safeio.ErrTooManyEntries} {
		if errors.Is(err, sentinel) {
			return &Error{Code: CodeBudgetExceeded, Message: err.Error()}
		}
	}
	if errors.Is(err, fs.ErrNotExist) {
		return &Error{Code: CodeNotFound, Message: err.Error()}
	}
	return nil
}

// applyRun is one call of Apply.
type applyRun struct {
	s *Service
	// sent is the change set as its sender sent it; set is the service's
	// working copy, with references resolved.
	sent    *Set
	set     *Set
	actor   Actor
	res     *Result
	plans   []*docPlan
	byDoc   map[string]*docPlan
	staged  map[*docPlan]Staged
	assets  []int
	refused int
	// viaFile marks the operations sent to the file of one edition of a
	// document, whose block keys are the keys that file reads with.
	viaFile map[int]bool
}

func (r *applyRun) refuse(i int, err *Error) {
	r.res.Ops[i].Status = OpRefused
	r.res.Ops[i].Error = err
	if r.refused < 0 || i < r.refused {
		r.refused = i
	}
}

// Apply applies a change set as actor and returns its result. Decoding and
// schema validation are the caller's: set is a decoded change set.
//
// An error is returned only when the change set could not be carried out for
// a reason that is not a refusal (a document that does not parse, a context
// that ended, a rename that failed before anything landed); a refusal is a
// result with status refused.
func (s *Service) Apply(ctx context.Context, set Set, actor Actor) (*Result, error) {
	// The service resolves references in its own copy, so the record keeps
	// the change set as it was sent.
	work := set
	work.Ops = slices.Clone(set.Ops)
	r := &applyRun{s: s, sent: &set, set: &work, actor: actor, byDoc: map[string]*docPlan{}, staged: map[*docPlan]Staged{}, refused: -1, viaFile: map[int]bool{},
		res: &Result{Schema: ResultSchemaID, Ops: make([]OpResult, len(set.Ops)), Docs: []DocResult{}}}
	for i, op := range set.Ops {
		r.res.Ops[i] = OpResult{I: i, Op: op.Kind, At: refOf(op)}
	}
	defer r.release()

	r.permit()
	if r.refused >= 0 {
		return r.finish(), nil
	}
	if err := r.route(ctx); err != nil {
		return nil, err
	}
	if r.refused >= 0 {
		return r.finish(), nil
	}
	if err := r.stage(ctx); err != nil {
		return nil, err
	}
	if r.refused >= 0 {
		return r.finish(), nil
	}
	r.prepareAssets(ctx)
	if r.refused >= 0 {
		return r.finish(), nil
	}
	findings, overridden, fingerprint, err := r.checkCommit(ctx)
	if err != nil {
		return nil, err
	}
	if r.refused >= 0 {
		return r.finish(), nil
	}

	if set.Mode == ModePreview {
		r.preview(findings)
		return r.res, nil
	}

	if err := r.settle(ctx); err != nil {
		return nil, err
	}
	if r.refused >= 0 {
		return r.finish(), nil
	}
	landed, err := r.commit(ctx, findings)
	if err != nil {
		return nil, err
	}
	if !landed {
		r.res.Status = SetPartial
		return r.res, nil
	}
	r.release()
	r.applyAssets(ctx)
	r.res.Status = SetApplied
	if r.refused >= 0 {
		// An asset the store refused after the content landed.
		r.res.Status = SetPartial
	}
	r.record(ctx, fingerprint, overridden)
	return r.res, nil
}

// permit asks the policy about every operation.
func (r *applyRun) permit() {
	if r.s.policy == nil {
		return
	}
	for i, op := range r.set.Ops {
		if err := r.s.policy.Permit(r.actor, r.sent, op); err != nil {
			r.refuse(i, err)
		}
	}
}

// route groups the operations by document, opens each document in its home,
// and checks what can be checked before reading anything: an operation the
// format does not support, and an edition with no home.
func (r *applyRun) route(ctx context.Context) error {
	order := []string{}
	byRaw := map[string][]int{}
	for i, op := range r.set.Ops {
		switch op.Kind {
		case KindTerm, KindMemory, KindRecipe:
			r.assets = append(r.assets, i)
			continue
		}
		if op.At.Doc == "" {
			r.refuse(i, &Error{Code: CodeInvalid, Field: "at/doc", Message: fmt.Sprintf("%s names no document", op.Kind)})
			continue
		}
		if _, ok := byRaw[op.At.Doc]; !ok {
			order = append(order, op.At.Doc)
		}
		byRaw[op.At.Doc] = append(byRaw[op.At.Doc], i)
	}
	for _, raw := range order {
		ops := byRaw[raw]
		var h Home
		var sess Session
		err := error(&Error{Code: CodeUnreachable, Message: "the change service has no home for documents"})
		if r.s.homes != nil {
			if h, err = r.s.homes.For(raw); err == nil {
				sess, err = h.Open(ctx, raw)
			}
		}
		if err != nil {
			e := asError(err)
			if e == nil {
				return fmt.Errorf("open %s: %w", raw, err)
			}
			for _, i := range ops {
				r.refuse(i, e)
			}
			continue
		}
		info := sess.Info()
		p := r.byDoc[info.Doc]
		if p == nil {
			p = &docPlan{svc: r.s, set: r.set, actor: r.actor, home: h, sess: sess, info: info,
				desc: r.s.describe(info), byKey: map[string][]int{}, results: r.res.Ops}
			r.byDoc[info.Doc] = p
			r.plans = append(r.plans, p)
		} else {
			_ = sess.Close()
		}
		for _, i := range ops {
			if info.Edition != nil {
				// The file of one edition: an operation sent to it addresses
				// that edition of the document.
				op := &r.set.Ops[i]
				switch {
				case op.At.Edition.IsZero():
					op.At.Edition = *info.Edition
				case op.At.Edition.Canonical() != info.Edition.Canonical():
					r.refuse(i, &Error{Code: CodeInvalid, Field: "at/edition",
						Message: fmt.Sprintf("%s holds edition %s of %s; address edition %s through %s", raw, keyText(*info.Edition), info.Doc, keyText(op.At.Edition), info.Doc)})
					continue
				}
				op.At.Doc = info.Doc
				r.viaFile[i] = true
			}
			p.ops = append(p.ops, i)
		}
	}
	for _, p := range r.plans {
		r.plan(ctx, p)
	}
	return nil
}

// plan files a document's operations by block, and refuses those the format
// or the home cannot carry.
func (r *applyRun) plan(ctx context.Context, p *docPlan) {
	slices.Sort(p.ops)
	resolver, _ := p.sess.(EditionKeyResolver)
	for _, i := range p.ops {
		op := r.set.Ops[i]
		if r.res.Ops[i].Status == OpRefused {
			continue
		}
		if err := p.desc.supports(op); err != nil {
			r.refuse(i, err)
			continue
		}
		switch op.Kind {
		case KindInsertBlock, KindNative:
			// No format declares either yet; supports refused them above.
			continue
		}
		place := p.sess.Place(op.At.Edition)
		switch place.Kind {
		case PlaceNone:
			r.refuse(i, &Error{Code: CodeUnsupported, Capability: "edition",
				Message: fmt.Sprintf("edition %s of %s has nowhere to live: the document holds one edition, and outside a project there is no file for another", keyText(op.At.Edition), p.info.Doc)})
			continue
		case PlaceOwnFile:
			if !slices.ContainsFunc(p.want.Editions, func(k model.EditionKey) bool { return k.Canonical() == op.At.Edition.Canonical() }) {
				p.want.Editions = append(p.want.Editions, op.At.Edition.Canonical())
			}
		default:
			if op.Kind != KindDecide {
				p.want.Own = true
			}
		}
		key := op.At.Block
		if resolver != nil && r.viaFile[i] && place.Kind == PlaceOwnFile {
			// A block named by its key in the edition's own file, as a person
			// who opened that file reads it.
			if k, ok, err := resolver.DocumentBlockKey(ctx, op.At.Edition, key); err == nil && ok {
				key = k
			}
		}
		if !slices.Contains(p.want.Blocks, key) {
			p.want.Blocks = append(p.want.Blocks, key)
		}
		p.byKey[key] = append(p.byKey[key], i)
	}
}

// EditionKeyResolver is implemented by a session that can translate the key
// of a block in an edition's own file into the key of the document's block it
// holds the edition of.
type EditionKeyResolver interface {
	DocumentBlockKey(ctx context.Context, edition model.EditionKey, key string) (string, bool, error)
}

// stage prepares every document.
func (r *applyRun) stage(ctx context.Context) error {
	for _, p := range r.plans {
		if len(p.byKey) == 0 {
			continue
		}
		staged, err := p.sess.Stage(ctx, p.want, p)
		if err != nil {
			if errors.Is(err, ErrRefused) {
				r.noteRefusals(p)
				continue
			}
			e := asError(err)
			if e == nil {
				return fmt.Errorf("prepare %s: %w", p.info.Doc, err)
			}
			for _, i := range p.ops {
				if r.res.Ops[i].Status != OpRefused {
					r.refuse(i, e)
				}
			}
			continue
		}
		r.staged[p] = staged
		r.noteRefusals(p)
	}
	return nil
}

// noteRefusals records the first operation a plan's pass refused.
func (r *applyRun) noteRefusals(p *docPlan) {
	for _, i := range p.ops {
		if r.res.Ops[i].Status == OpRefused && (r.refused < 0 || i < r.refused) {
			r.refused = i
		}
	}
}

// prepareAssets checks the decisions and asset operations before anything is
// written.
func (r *applyRun) prepareAssets(ctx context.Context) {
	for _, p := range r.plans {
		for _, i := range p.ops {
			if r.set.Ops[i].Kind != KindDecide || r.res.Ops[i].Status == OpRefused {
				continue
			}
			r.prepareAsset(ctx, i, p.decisions[i])
		}
	}
	for _, i := range r.assets {
		if r.res.Ops[i].Status == OpRefused {
			continue
		}
		r.prepareAsset(ctx, i, nil)
	}
}

func (r *applyRun) prepareAsset(ctx context.Context, i int, target *DecisionTarget) {
	op := r.set.Ops[i]
	if r.s.assets == nil {
		r.refuse(i, &Error{Code: CodeUnsupported, Capability: string(op.Kind),
			Message: fmt.Sprintf("%s is applied by a host that keeps decisions and context; this service has none", op.Kind)})
		return
	}
	if err := r.s.assets.Prepare(ctx, r.actor, op, target); err != nil {
		r.refuse(i, err)
	}
}

// checkCommit runs the commit check over every changed edition. A failing
// finding the change introduces refuses its operations under the enforce
// gate, and lands with them under report.
func (r *applyRun) checkCommit(ctx context.Context) (findings map[*docPlan][]Finding, overridden []Finding, fingerprint string, err error) {
	findings = map[*docPlan][]Finding{}
	if r.s.check == nil {
		return findings, nil, "", nil
	}
	var fps []string
	for _, p := range r.plans {
		if len(p.changes) == 0 {
			continue
		}
		outcomes, fp, cerr := r.s.check.Check(ctx, p.changes)
		if cerr != nil {
			return nil, nil, "", fmt.Errorf("check %s: %w", p.info.Doc, cerr)
		}
		if fp != "" && !slices.Contains(fps, fp) {
			fps = append(fps, fp)
		}
		for ci, o := range outcomes {
			if ci >= len(p.changes) {
				break
			}
			intro := Introduced(o)
			if len(intro) == 0 {
				continue
			}
			at := p.changes[ci].Ref
			for fi := range intro {
				intro[fi].At = &at
			}
			findings[p] = append(findings[p], intro...)
			if r.set.Gate == GateReport {
				if r.actor.Kind == ActorPerson {
					overridden = append(overridden, intro...)
				}
				for _, i := range p.changeOps[ci] {
					r.res.Ops[i].Findings = append(r.res.Ops[i].Findings, intro...)
				}
				continue
			}
			for _, i := range p.changeOps[ci] {
				r.res.Ops[i].Findings = append(r.res.Ops[i].Findings, intro...)
				r.refuse(i, &Error{Code: CodeGateFailed,
					Message: fmt.Sprintf("the edit introduces %d failing finding(s) in %s: %s", len(intro), at.String(), intro[0].Message)})
			}
		}
	}
	return findings, overridden, strings.Join(fps, ","), nil
}

// preview fills the result of a preview: every operation that would apply is
// previewed, and each document carries the diff of what it would write.
func (r *applyRun) preview(findings map[*docPlan][]Finding) {
	r.res.Status = SetPreviewed
	for i := range r.res.Ops {
		if st := r.res.Ops[i].Status; st == OpApplied || st == "" {
			r.res.Ops[i].Status = OpPreviewed
		}
	}
	for _, p := range r.plans {
		staged := r.staged[p]
		if staged == nil {
			continue
		}
		docs := r.docResults(p, staged, false)
		if len(docs) > 0 {
			docs[0].Diff = staged.Diff()
			docs[0].Findings = findings[p]
		}
		r.res.Docs = append(r.res.Docs, docs...)
	}
}

// settle takes every staged document's commit lock, in one order, and makes
// sure each change still applies.
func (r *applyRun) settle(ctx context.Context) error {
	plans := slices.Clone(r.plans)
	slices.SortFunc(plans, func(a, b *docPlan) int {
		var ka, kb string
		if st := r.staged[a]; st != nil {
			ka = st.LockKey()
		}
		if st := r.staged[b]; st != nil {
			kb = st.LockKey()
		}
		return strings.Compare(ka, kb)
	})
	for _, p := range plans {
		st := r.staged[p]
		if st == nil {
			continue
		}
		err := st.Settle(ctx)
		if err == nil {
			continue
		}
		if errors.Is(err, ErrRefused) {
			r.noteRefusals(p)
			return nil
		}
		e := asError(err)
		if e == nil {
			return fmt.Errorf("commit %s: %w", p.info.Doc, err)
		}
		for _, i := range p.ops {
			r.refuse(i, e)
		}
		return nil
	}
	return nil
}

// commit makes every staged change the head of its document. It reports false
// when an I/O error stopped it after some documents had landed; an error
// before any landed is returned as it is.
func (r *applyRun) commit(ctx context.Context, findings map[*docPlan][]Finding) (bool, error) {
	landed := 0
	for _, p := range r.plans {
		st := r.staged[p]
		if st == nil {
			continue
		}
		if err := st.Commit(ctx); err != nil {
			if landed == 0 {
				return false, fmt.Errorf("commit %s: %w", p.info.Doc, err)
			}
			for _, i := range p.ops {
				if r.res.Ops[i].Status == OpApplied || r.res.Ops[i].Status == OpUnchanged {
					r.res.Ops[i] = OpResult{I: i, Op: r.res.Ops[i].Op, At: r.res.Ops[i].At, Status: OpNotApplied}
				}
			}
			r.res.Docs = append(r.res.Docs, r.docResults(p, st, false)...)
			continue
		}
		landed++
		docs := r.docResults(p, st, true)
		if len(docs) > 0 {
			docs[0].Findings = findings[p]
		}
		r.res.Docs = append(r.res.Docs, docs...)
		r.invalidate(p)
	}
	return landed == len(r.staged), nil
}

// invalidate adds to an authoritative edit's invalidations the editions that
// live in files of their own, which the block did not hold when it was edited.
func (r *applyRun) invalidate(p *docPlan) {
	if len(p.info.Derived) == 0 {
		return
	}
	for _, i := range p.ops {
		res := &r.res.Ops[i]
		if res.Status != OpApplied || !r.authoritative(p, i) {
			continue
		}
		for _, k := range p.info.Derived {
			text := keyText(k)
			if !slices.ContainsFunc(res.Invalidates, func(v Invalidation) bool { return v.Edition == text }) {
				res.Invalidates = append(res.Invalidates, Invalidation{Edition: text, Reason: ReasonBasisMoved})
			}
		}
	}
}

// authoritative reports whether operation i changed the authoritative edition
// of its block.
func (r *applyRun) authoritative(p *docPlan, i int) bool {
	for ci, ops := range p.changeOps {
		if slices.Contains(ops, i) && p.changes[ci].Role == RoleAuthoritative {
			return true
		}
	}
	return false
}

// docResults reports a staged document's files.
func (r *applyRun) docResults(p *docPlan, st Staged, committed bool) []DocResult {
	var out []DocResult
	for _, f := range st.Files() {
		d := DocResult{Doc: p.info.Doc, Home: p.home.Name(), Before: f.Before}
		if f.File != "" && f.File != p.info.Doc {
			d.File = f.File
		}
		if f.Edition != nil {
			d.Edition = keyText(*f.Edition)
		}
		if f.After != f.Before {
			after := f.After
			d.After = &after
			d.Written = committed
		}
		out = append(out, d)
	}
	return out
}

// applyAssets applies the decisions and asset operations, in the order of the
// change set, once the content has landed.
func (r *applyRun) applyAssets(ctx context.Context) {
	targets := map[int]*DecisionTarget{}
	for _, p := range r.plans {
		maps.Copy(targets, p.decisions)
	}
	for i, op := range r.set.Ops {
		switch op.Kind {
		case KindDecide, KindTerm, KindMemory, KindRecipe:
		default:
			continue
		}
		if r.res.Ops[i].Status == OpRefused {
			continue
		}
		status, err := r.s.assets.Apply(ctx, r.actor, r.set, op, targets[i])
		if err != nil {
			r.refuse(i, err)
			continue
		}
		r.res.Ops[i].Status = status
	}
}

// record records the change set once its content has landed.
func (r *applyRun) record(ctx context.Context, fingerprint string, overridden []Finding) {
	if r.s.recorder == nil {
		return
	}
	var transitions []Transition
	for _, p := range r.plans {
		for _, c := range p.changes {
			b := c.Block
			transitions = append(transitions, Transition{
				EditionChange: c,
				ContentHash:   model.ComputeContentHash(b.SourceText()),
				ContextHash:   model.ComputeContextHash(b.Name, b.Type, b.Properties),
			})
		}
	}
	written := false
	for _, d := range r.res.Docs {
		written = written || d.Written
	}
	if !written && len(transitions) == 0 {
		return
	}
	rec := Record{Actor: r.actor, Origin: r.s.origin, Fingerprint: fingerprint, Overridden: overridden,
		Docs: r.res.Docs, Transitions: transitions}
	if r.actor.Kind != ActorTool {
		rec.Set = r.sent
	}
	id, err := r.s.recorder.Record(ctx, rec)
	if err != nil || id == "" {
		// The content is written; the next read finds a transition no record
		// explains and records it as observed.
		return
	}
	r.res.Record = &id
}

// finish completes a refused result: every operation not refused is
// not_applied, blocked by the first refusal.
func (r *applyRun) finish() *Result {
	r.res.Status = SetRefused
	first := r.refused
	for i := range r.res.Ops {
		if r.res.Ops[i].Status == OpRefused {
			continue
		}
		r.res.Ops[i] = OpResult{I: i, Op: r.res.Ops[i].Op, At: r.res.Ops[i].At, Status: OpNotApplied, BlockedBy: &first}
	}
	for _, p := range r.plans {
		if st := r.staged[p]; st != nil {
			for _, d := range r.docResults(p, st, false) {
				d.After = nil
				r.res.Docs = append(r.res.Docs, d)
			}
		}
	}
	return r.res
}

// release drops every staged change and closes every session.
func (r *applyRun) release() {
	for p, st := range r.staged {
		_ = st.Release()
		delete(r.staged, p)
	}
	for _, p := range r.plans {
		if p.sess != nil {
			_ = p.sess.Close()
			p.sess = nil
		}
	}
}
