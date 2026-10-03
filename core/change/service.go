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
	formats   Formats
	homes     Homes
	check     CommitCheck
	policy    Policy
	recorder  Recorder
	assets    Assets
	states    EditionStates
	describer Describer
	origin    string
	now       func() time.Time
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

// WithDescriber replaces DescribeFormat as what decides a format's
// operations.
func WithDescriber(d Describer) Option { return func(s *Service) { s.describer = d } }

// WithOrigin names the surface that applies change sets through the service
// (apply, desktop, flow:<name>, merge, pull), for the record.
func WithOrigin(origin string) Option { return func(s *Service) { s.origin = origin } }

// WithClock sets the clock that stamps an edit.
func WithClock(now func() time.Time) Option { return func(s *Service) { s.now = now } }

// NewService returns a service over homes. formats answers what the service
// knows about a format, for Describe and for the operations a read lists.
func NewService(formats Formats, homes Homes, opts ...Option) *Service {
	s := &Service{formats: formats, homes: homes, describer: DescribeFormat, origin: "apply"}
	for _, o := range opts {
		o(s)
	}
	if s.describer == nil {
		s.describer = DescribeFormat
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
	// What the commit check found for each document, as of the pass over it
	// that commits: the introduced findings, a person's overridden ones
	// under gate report, and the governance fingerprint the check used.
	findings     map[*docPlan][]Finding
	overridden   map[*docPlan][]Finding
	fingerprints map[*docPlan]string
}

func (r *applyRun) refuse(i int, err *Error) {
	r.res.Ops[i].Status = OpRefused
	r.res.Ops[i].Error = err
	if r.refused < 0 || i < r.refused {
		r.refused = i
	}
}

// refuseAfresh refuses operation i, dropping what an earlier step said the
// operation did.
func (r *applyRun) refuseAfresh(i int, err *Error) {
	res := r.res.Ops[i]
	r.res.Ops[i] = OpResult{I: i, Op: res.Op, At: res.At}
	r.refuse(i, err)
}

// Apply applies a change set as actor and returns its result. Decoding and
// schema validation are the caller's: set is a decoded change set.
//
// An error is returned only when the change set could not be carried out for
// a reason that is not a refusal (a document that does not parse, a context
// that ended, a rename that failed before anything landed); a refusal is a
// result with status refused.
func (s *Service) Apply(ctx context.Context, set Set, actor Actor) (*Result, error) {
	if set.Mode == ModePreview {
		ctx = context.WithValue(ctx, previewKey{}, true)
	}
	// The service resolves references in its own copy, so the record keeps
	// the change set as it was sent.
	work := set
	work.Ops = slices.Clone(set.Ops)
	r := &applyRun{s: s, sent: &set, set: &work, actor: actor, byDoc: map[string]*docPlan{}, staged: map[*docPlan]Staged{}, refused: -1, viaFile: map[int]bool{},
		findings: map[*docPlan][]Finding{}, overridden: map[*docPlan][]Finding{}, fingerprints: map[*docPlan]string{},
		res: &Result{Schema: ResultSchemaID, Ops: make([]OpResult, len(set.Ops)), Docs: []DocResult{}}}
	for i, op := range set.Ops {
		r.res.Ops[i] = OpResult{I: i, Op: op.Kind, At: refOf(op)}
	}
	// The commit locks are held until the decisions are applied and the
	// change recorded, so a decision binds to the content this change set
	// landed, and the records of one document follow the order its commits
	// took.
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
	r.sharedFiles()
	if r.refused >= 0 {
		return r.finish(), nil
	}
	r.prepareAssets(ctx)
	if r.refused >= 0 {
		return r.finish(), nil
	}
	for _, p := range r.plans {
		if err := r.checkPlan(ctx, p); err != nil {
			return nil, err
		}
	}
	if r.refused >= 0 {
		return r.finish(), nil
	}

	if set.Mode == ModePreview {
		r.preview()
		return r.res, nil
	}

	if err := r.settle(ctx); err != nil {
		return nil, err
	}
	if r.refused >= 0 {
		return r.finish(), nil
	}
	all, err := r.commit(ctx)
	if err != nil {
		return nil, err
	}
	if !all {
		// An I/O error stopped the renames after some files landed. The
		// decisions and assets bind to content that did not all land, so
		// none is applied; what landed is recorded.
		r.res.Status = SetPartial
		r.holdAssets()
		r.record(ctx)
		return r.res, nil
	}
	r.applyAssets(ctx)
	r.res.Status = SetApplied
	if r.refused >= 0 {
		// An asset the store refused after the content landed.
		r.res.Status = SetPartial
	}
	r.record(ctx)
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
				desc: r.s.describe(sess), byKey: map[string][]int{}, results: r.res.Ops}
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
		case KindNative:
			// No format declares one yet; supports refused it above.
			continue
		case KindInsertBlock, KindDeleteBlock:
			r.planStructural(p, i)
			continue
		}
		place := p.sess.Place(op.At.Edition)
		switch place.Kind {
		case PlaceNone:
			why := place.Why
			if why == "" {
				why = "the document holds one edition, and outside a project there is no file for another"
			}
			r.refuse(i, &Error{Code: CodeUnsupported, Capability: "edition",
				Message: fmt.Sprintf("edition %s of %s has nowhere to live: %s", keyText(op.At.Edition), p.info.Doc, why)})
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
	r.conflicts(p)
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
		if len(p.byKey) == 0 && len(p.structural) == 0 {
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
					r.refuseAfresh(i, e)
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

// sharedFiles refuses a document that needs a commit lock another document
// of the change set needs too: one file named twice, as two members of one
// archive are, or a file and a link to it. Each document stages the file from
// what it read, so the second rename would drop the first one's edit, and
// two locks on one file in one process wait for each other.
func (r *applyRun) sharedFiles() {
	owner := map[string]*docPlan{}
	for _, p := range r.plans {
		st := r.staged[p]
		if st == nil {
			continue
		}
		for _, k := range st.LockKeys() {
			q, ok := owner[k]
			if !ok {
				owner[k] = p
				continue
			}
			if q == p {
				continue
			}
			for _, i := range p.ops {
				if r.res.Ops[i].Status != OpRefused {
					r.refuse(i, &Error{Code: CodeInvalid, Field: "at/doc",
						Message: fmt.Sprintf("%s and %s are kept in one file, which one change set writes through one document; send the operations on %s in a change set of their own", q.info.Doc, p.info.Doc, p.info.Doc)})
				}
			}
			break
		}
	}
}

// checkPlan checks a document's staged pass before it is committed: that
// every edition the pass changed reaches the file that holds it, and what the
// commit check finds on the changed editions. It runs again when Settle
// applied the change a second time, because that pass is the one that
// commits.
func (r *applyRun) checkPlan(ctx context.Context, p *docPlan) error {
	st := r.staged[p]
	if st == nil {
		return nil
	}
	r.reachesFile(p, st)
	if r.refusedIn(p) {
		return nil
	}
	return r.checkCommit(ctx, p)
}

// refusedIn reports whether an operation of p was refused.
func (r *applyRun) refusedIn(p *docPlan) bool {
	return slices.ContainsFunc(p.ops, func(i int) bool { return r.res.Ops[i].Status == OpRefused })
}

// reachesFile refuses a change the home's writer dropped: an edition the pass
// changed whose file the stage leaves as it was. An operation reported
// applied has reached the bytes; one the format has nowhere to keep is
// refused rather than reported applied and lost.
func (r *applyRun) reachesFile(p *docPlan, st Staged) {
	files := st.Files()
	for ci, ch := range p.changes {
		f := fileOf(files, p.sess.Place(ch.Ref.Edition))
		if f != nil && f.After != f.Before {
			continue
		}
		file := p.info.Doc
		if f != nil && f.File != "" {
			file = f.File
		}
		for _, i := range p.changeOps[ci] {
			if r.res.Ops[i].Status == OpRefused {
				continue
			}
			r.refuseAfresh(i, &Error{Code: CodeUnsupported, Capability: "edition", Field: "at/edition",
				Message: fmt.Sprintf("writing %s in the %s format leaves it as it was, so the change to edition %s of block %s would be dropped: the file holds no such edition",
					file, p.info.Format, editionText(ch.Ref), ch.Ref.Block)})
		}
	}
}

// fileOf is the staged file that holds an edition living at place: the
// document's own file, or the edition's file of its own.
func fileOf(files []StagedFile, place Place) *StagedFile {
	for i := range files {
		f := &files[i]
		switch place.Kind {
		case PlaceOwnFile:
			if f.Edition != nil && f.File == place.File {
				return f
			}
		default:
			if f.Edition == nil {
				return f
			}
		}
	}
	return nil
}

// editionText names the edition a reference addresses, in a message.
func editionText(ref Ref) string {
	if ref.Edition.IsZero() {
		return "(the document's own)"
	}
	return keyText(ref.Edition)
}

// checkCommit runs the commit check over the editions a document's pass
// changed. A failing finding the change introduces refuses its operations
// under the enforce gate, and lands with them under report.
func (r *applyRun) checkCommit(ctx context.Context, p *docPlan) error {
	delete(r.findings, p)
	delete(r.overridden, p)
	delete(r.fingerprints, p)
	if r.s.check == nil || len(p.changes) == 0 {
		return nil
	}
	outcomes, fp, err := r.s.check.Check(ctx, p.changes)
	if err != nil {
		return fmt.Errorf("check %s: %w", p.info.Doc, err)
	}
	r.fingerprints[p] = fp
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
		r.findings[p] = append(r.findings[p], intro...)
		if r.set.Gate == GateReport {
			if r.actor.Kind == ActorPerson {
				r.overridden[p] = append(r.overridden[p], intro...)
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
	return nil
}

// preview fills the result of a preview: every operation that would apply is
// previewed, and each document carries the diff of what it would write.
func (r *applyRun) preview() {
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
		docs := r.docResults(p, staged)
		if len(docs) > 0 {
			docs[0].Diff = staged.Diff()
			docs[0].Findings = r.findings[p]
		}
		r.res.Docs = append(r.res.Docs, docs...)
	}
}

// settle takes the commit locks of every staged document, all of them in the
// order of their keys, and then makes sure each document's change still
// applies. A document whose home applied the change again is checked again.
func (r *applyRun) settle(ctx context.Context) error {
	type lock struct {
		key string
		p   *docPlan
	}
	var locks []lock
	for _, p := range r.plans {
		if st := r.staged[p]; st != nil {
			for _, k := range st.LockKeys() {
				locks = append(locks, lock{k, p})
			}
		}
	}
	slices.SortStableFunc(locks, func(a, b lock) int { return strings.Compare(a.key, b.key) })
	for _, l := range locks {
		if err := r.staged[l.p].Lock(ctx, l.key); err != nil {
			if e := asError(err); e != nil {
				r.refuseAll(l.p, e)
				return nil
			}
			return fmt.Errorf("lock %s: %w", l.p.info.Doc, err)
		}
	}
	for _, p := range r.plans {
		st := r.staged[p]
		if st == nil {
			continue
		}
		passes := p.passes
		err := st.Settle(ctx)
		if err == nil {
			if p.passes != passes {
				if err := r.checkPlan(ctx, p); err != nil {
					return err
				}
				if r.refused >= 0 {
					return nil
				}
			}
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
		r.refuseAll(p, e)
		return nil
	}
	return nil
}

// refuseAll refuses every operation of p with err.
func (r *applyRun) refuseAll(p *docPlan, err *Error) {
	for _, i := range p.ops {
		r.refuseAfresh(i, err)
	}
}

// commit makes every staged change the head of its document. It reports
// whether every file landed. A failure before any file was written is
// returned as an error; one after leaves a partial result whose documents say
// which files were written and whose operations on the files that were not
// are not_applied.
func (r *applyRun) commit(ctx context.Context) (bool, error) {
	wroteAny, all := false, true
	for _, p := range r.plans {
		st := r.staged[p]
		if st == nil {
			continue
		}
		cerr := st.Commit(ctx)
		docs := r.docResults(p, st)
		wrote := slices.ContainsFunc(docs, func(d DocResult) bool { return d.Written })
		if cerr != nil {
			if !wroteAny && !wrote {
				return false, fmt.Errorf("commit %s: %w", p.info.Doc, cerr)
			}
			all = false
			r.unland(p, st.Files())
		}
		wroteAny = wroteAny || wrote
		if len(docs) > 0 {
			docs[0].Findings = r.findings[p]
		}
		r.res.Docs = append(r.res.Docs, docs...)
		r.invalidate(p)
	}
	return all, nil
}

// unland marks not_applied the operations of p whose edition lives in a file
// the interrupted commit did not write.
func (r *applyRun) unland(p *docPlan, files []StagedFile) {
	for _, i := range p.ops {
		res := &r.res.Ops[i]
		if res.Status != OpApplied || r.set.Ops[i].Kind == KindDecide {
			continue
		}
		if f := fileOf(files, p.sess.Place(r.set.Ops[i].At.Edition)); f != nil && f.Written {
			continue
		}
		*res = OpResult{I: i, Op: res.Op, At: res.At, Status: OpNotApplied}
	}
}

// holdAssets marks the decisions and asset operations not_applied, for a
// change set whose content did not all land.
func (r *applyRun) holdAssets() {
	for i, op := range r.set.Ops {
		switch op.Kind {
		case KindDecide, KindTerm, KindMemory, KindRecipe:
		default:
			continue
		}
		if res := &r.res.Ops[i]; res.Status != OpRefused {
			*res = OpResult{I: i, Op: res.Op, At: res.At, Status: OpNotApplied}
		}
	}
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
		if k := r.set.Ops[i].Kind; k == KindInsertBlock || k == KindDeleteBlock {
			// A block added or removed leaves no translation on an older
			// basis: a new block has none yet, and a removed one takes its
			// translations with it.
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
func (r *applyRun) docResults(p *docPlan, st Staged) []DocResult {
	var out []DocResult
	for _, f := range st.Files() {
		d := DocResult{Doc: p.info.Doc, Home: p.home.Name(), Before: f.Before, Written: f.Written}
		if f.File != "" && f.File != p.info.Doc {
			d.File = f.File
		}
		if f.Edition != nil {
			d.Edition = keyText(*f.Edition)
		}
		if f.After != f.Before {
			after := f.After
			d.After = &after
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
			r.refuseAfresh(i, err)
			continue
		}
		r.res.Ops[i].Status = status
	}
}

// record records what the change set landed: the transitions of the editions
// whose files were written, the governance the commit check used on them, and
// the findings a person chose to land with.
func (r *applyRun) record(ctx context.Context) {
	if r.s.recorder == nil {
		return
	}
	var (
		transitions []Transition
		overridden  []Finding
		fps         []string
	)
	for _, p := range r.plans {
		st := r.staged[p]
		if st == nil {
			continue
		}
		files := st.Files()
		landed := false
		for _, c := range p.changes {
			if f := fileOf(files, p.sess.Place(c.Ref.Edition)); f == nil || !f.Written {
				continue
			}
			landed = true
			b := c.Block
			transitions = append(transitions, Transition{
				EditionChange: c,
				ContentHash:   model.ComputeContentHash(b.SourceText()),
				ContextHash:   model.ComputeContextHash(b.Name, b.Type, b.Properties),
			})
		}
		if !landed {
			continue
		}
		overridden = append(overridden, r.overridden[p]...)
		if fp := r.fingerprints[p]; fp != "" && !slices.Contains(fps, fp) {
			fps = append(fps, fp)
		}
	}
	if len(transitions) == 0 {
		return
	}
	rec := Record{Actor: r.actor, Origin: r.s.origin, Fingerprint: strings.Join(fps, ","), Overridden: overridden,
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
			for _, d := range r.docResults(p, st) {
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
