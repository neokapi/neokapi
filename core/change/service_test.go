package change_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/change/changetest"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/safeio"
)

// memHome is a home that keeps documents in memory: each document a list of
// blocks with their editions, and a head that every commit moves. It holds a
// bilingual document, so every edition lives in the document.
type memHome struct {
	mu   sync.Mutex
	docs map[string]*memDoc
	// openErr, when set, is what opening any document returns.
	openErr error
	// beforeSettle, when set, is called once a document is staged and
	// before its commit lock is taken.
	beforeSettle func(doc string)
	// failCommit, when set, is the document whose commit fails.
	failCommit string
}

type memDoc struct {
	// commit is the document's commit lock.
	commit sync.Mutex
	head   int
	blocks []memBlock
}

type memBlock struct {
	key          string
	translatable bool
	editions     map[model.EditionKey][]model.Run
}

func newMemHome(docs map[string][]memBlock) *memHome {
	h := &memHome{docs: map[string]*memDoc{}}
	for name, bs := range docs {
		h.docs[name] = &memDoc{blocks: bs}
	}
	return h
}

func textBlock(key, en string, targets ...string) memBlock {
	b := memBlock{key: key, translatable: true, editions: map[model.EditionKey][]model.Run{{}: {model.TextR(en)}}}
	for i := 0; i+1 < len(targets); i += 2 {
		b.editions[model.EditionKey{Locale: model.LocaleID(targets[i])}] = []model.Run{model.TextR(targets[i+1])}
	}
	return b
}

// build makes the model blocks a session reads: fresh ones, so nothing a
// stage does reaches the home before it commits.
func (d *memDoc) build() []*model.Block {
	out := make([]*model.Block, 0, len(d.blocks))
	for i, mb := range d.blocks {
		b := model.NewRunsBlock(fmt.Sprintf("tu%d", i+1), slices.Clone(mb.editions[model.EditionKey{}]))
		b.Name = mb.key
		b.SourceLocale = "en"
		b.Translatable = mb.translatable
		for k, runs := range mb.editions {
			if k.IsZero() {
				continue
			}
			b.SetTarget(k.Locale, &model.Target{Runs: slices.Clone(runs)})
		}
		out = append(out, b)
	}
	return out
}

func capture(blocks []*model.Block) []memBlock {
	out := make([]memBlock, 0, len(blocks))
	for _, b := range blocks {
		mb := memBlock{key: b.Name, translatable: b.Translatable, editions: map[model.EditionKey][]model.Run{}}
		for _, k := range b.Editions() {
			ed, _ := b.Edition(k)
			if b.IsSourceEdition(k) {
				k = model.EditionKey{}
			}
			mb.editions[k] = ed.Runs
		}
		out = append(out, mb)
	}
	return out
}

func (h *memHome) Name() string { return "memory" }

func (h *memHome) Open(_ context.Context, doc string) (change.Session, error) {
	if h.openErr != nil {
		return nil, h.openErr
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, ok := h.docs[doc]; !ok {
		return nil, &change.Error{Code: change.CodeNotFound, Message: "no document " + doc}
	}
	return &memSession{h: h, doc: doc}, nil
}

func (h *memHome) snapshot(doc string) string {
	h.mu.Lock()
	defer h.mu.Unlock()
	d := h.docs[doc]
	var b strings.Builder
	fmt.Fprintf(&b, "head %d\n", d.head)
	for _, mb := range d.blocks {
		keys := make([]string, 0, len(mb.editions))
		for k := range mb.editions {
			t, _ := k.MarshalText()
			keys = append(keys, string(t))
		}
		slices.Sort(keys)
		for _, k := range keys {
			ek, _ := model.ParseEditionKey(k)
			fmt.Fprintf(&b, "%s@%s=%s\n", mb.key, k, model.RunsEditText(mb.editions[ek]))
		}
	}
	return b.String()
}

type memSession struct {
	h   *memHome
	doc string
}

func (s *memSession) Info() change.DocInfo {
	return change.DocInfo{Doc: s.doc, Format: "memory", SourceLocale: "en", Editions: change.EditionsInFile}
}

func (s *memSession) Place(model.EditionKey) change.Place {
	return change.Place{Kind: change.PlaceInDocument}
}

func (s *memSession) headOf() int {
	s.h.mu.Lock()
	defer s.h.mu.Unlock()
	return s.h.docs[s.doc].head
}

func (s *memSession) current() ([]*model.Block, int) {
	s.h.mu.Lock()
	defer s.h.mu.Unlock()
	d := s.h.docs[s.doc]
	return d.build(), d.head
}

func (s *memSession) Read(_ context.Context, _ change.Want, fn func(*model.Block) error) (string, error) {
	blocks, head := s.current()
	for _, b := range blocks {
		if err := fn(b); err != nil {
			if errors.Is(err, change.ErrStop) {
				break
			}
			return "", err
		}
	}
	return fmt.Sprintf("head:%d", head), nil
}

func (s *memSession) Stage(_ context.Context, _ change.Want, e change.Editor) (change.Staged, error) {
	st := &memStaged{s: s, e: e}
	if err := st.run(); err != nil {
		return nil, err
	}
	return st, nil
}

func (s *memSession) Close() error { return nil }

type memStaged struct {
	s       *memSession
	e       change.Editor
	head    int
	blocks  []*model.Block
	changed bool
	locked  bool
	written bool
}

func (st *memStaged) run() error {
	st.blocks, st.head = st.s.current()
	st.changed = false
	st.e.Begin()
	for _, b := range st.blocks {
		keys, err := st.e.Edit(b)
		if err != nil {
			return err
		}
		st.changed = st.changed || len(keys) > 0
	}
	return st.e.End()
}

func (st *memStaged) Files() []change.StagedFile {
	before := fmt.Sprintf("head:%d", st.head)
	after := before
	if st.changed {
		after = fmt.Sprintf("head:%d", st.head+1)
	}
	return []change.StagedFile{{File: st.s.doc, Before: before, After: after, Written: st.written}}
}

func (st *memStaged) Diff() string        { return "" }
func (st *memStaged) LockKeys() []string  { return []string{st.s.doc} }
func (st *memStaged) doc() *memDoc        { return st.s.h.docs[st.s.doc] }
func (st *memStaged) headNow() (head int) { return st.s.headOf() }

func (st *memStaged) Lock(_ context.Context, key string) error {
	if st.locked {
		return nil
	}
	if key != st.s.doc {
		return fmt.Errorf("no lock %s", key)
	}
	if hook := st.s.h.beforeSettle; hook != nil {
		hook(st.s.doc)
	}
	st.doc().commit.Lock()
	st.locked = true
	return nil
}

func (st *memStaged) Settle(ctx context.Context) error {
	if err := st.Lock(ctx, st.s.doc); err != nil {
		return err
	}
	if st.headNow() == st.head {
		return nil
	}
	return st.run()
}

func (st *memStaged) Commit(context.Context) error {
	if !st.changed {
		return nil
	}
	if st.s.h.failCommit == st.s.doc {
		return errors.New("disk full")
	}
	st.s.h.mu.Lock()
	defer st.s.h.mu.Unlock()
	d := st.doc()
	d.blocks = capture(st.blocks)
	d.head++
	st.written = true
	return nil
}

func (st *memStaged) Release() error {
	if st.locked {
		st.locked = false
		st.doc().commit.Unlock()
	}
	return nil
}

// memFormats knows the in-memory format.
type memFormats struct{}

func (memFormats) Facts(name string) (change.FormatFacts, bool) {
	if name != "memory" {
		return change.FormatFacts{}, false
	}
	return change.FormatFacts{Name: "memory", Editable: true, Interchange: true}, true
}

var (
	svcPerson = change.Actor{Kind: change.ActorPerson, Name: "ada"}
	svcAgent  = change.Actor{Kind: change.ActorAgent, Name: "claude", Session: "s1"}
)

func newMemService(h *memHome, opts ...change.Option) *change.Service {
	return change.NewService(memFormats{}, change.OneHome(h), opts...)
}

func readBlock(t *testing.T, svc *change.Service, doc, key string) change.BlockRead {
	t.Helper()
	page, err := svc.Read(context.Background(), change.ReadRequest{Doc: doc, Blocks: []string{key}})
	require.NoError(t, err)
	require.Len(t, page.Blocks, 1)
	return page.Blocks[0]
}

func edit(at change.Ref, ifMatch, text string) change.Op {
	return change.Op{Kind: change.KindSetContent, At: at, IfMatch: ifMatch, Body: &change.SetContent{Text: &text}}
}

func atEdition(r change.Ref, edition string) change.Ref {
	k, err := model.ParseEditionKey(edition)
	if err != nil {
		panic(err)
	}
	r.Edition = k
	return r
}

func TestService_Conformance(t *testing.T) {
	changetest.Run(t, func(t *testing.T) changetest.Env {
		h := newMemHome(map[string][]memBlock{
			"a": {textBlock("one", "First"), textBlock("two", "Second")},
			"b": {textBlock("three", "Third")},
		})
		return changetest.Env{
			Service:         newMemService(h),
			DocA:            "a",
			DocB:            "b",
			Snapshot:        func(t *testing.T, doc string) []byte { return []byte(h.snapshot(doc)) },
			SetBeforeSettle: func(fn func(string)) { h.beforeSettle = fn },
		}
	})
}

// denyAgents refuses every operation an agent sends.
type denyAgents struct{}

func (denyAgents) Permit(actor change.Actor, _ *change.Set, op change.Op) *change.Error {
	if actor.Kind == change.ActorAgent && op.Kind == change.KindTerm {
		return &change.Error{Code: change.CodeNotPermitted, Message: "an agent observes a term; a person keeps it"}
	}
	return nil
}

func TestService_PolicyRefusesBeforeAnythingIsRead(t *testing.T) {
	h := newMemHome(map[string][]memBlock{"a": {textBlock("one", "First")}})
	svc := newMemService(h, change.WithPolicy(denyAgents{}))
	b := readBlock(t, svc, "a", "one")
	before := h.snapshot("a")
	res, err := svc.Apply(context.Background(), change.Set{Ops: []change.Op{
		edit(b.Ref, b.Rev, "Edited"),
		{Kind: change.KindTerm, Body: &change.Term{Action: "upsert", Term: "handbook"}},
	}}, svcAgent)
	require.NoError(t, err)
	assert.Equal(t, change.SetRefused, res.Status)
	assert.Equal(t, change.CodeNotPermitted, res.Ops[1].Error.Code)
	assert.Equal(t, change.OpNotApplied, res.Ops[0].Status)
	assert.Equal(t, before, h.snapshot("a"))
}

// wordCheck fails every edition that holds a word.
type wordCheck struct {
	word   string
	seen   [][]change.EditionChange
	report bool
	// previewing records, per call, whether the call's context was a
	// preview's.
	previewing []bool
}

func (c *wordCheck) Check(ctx context.Context, changes []change.EditionChange) ([]change.CheckOutcome, string, error) {
	c.seen = append(c.seen, changes)
	c.previewing = append(c.previewing, change.Previewing(ctx))
	out := make([]change.CheckOutcome, len(changes))
	find := func(runs []model.Run) []change.Finding {
		if strings.Contains(model.RunsText(runs), c.word) {
			return []change.Finding{{Rule: "terms.vocabulary", Message: "avoid " + c.word, Fails: !c.report}}
		}
		return nil
	}
	for i, ch := range changes {
		out[i] = change.CheckOutcome{Before: find(ch.Before), After: find(ch.After)}
	}
	return out, "gov_1", nil
}

func TestService_CommitCheck(t *testing.T) {
	newHome := func() *memHome {
		return newMemHome(map[string][]memBlock{"a": {textBlock("one", "Please utilize the form"), textBlock("two", "Clean text")}})
	}

	t.Run("an introduced failing finding refuses the change set under enforce", func(t *testing.T) {
		h := newHome()
		check := &wordCheck{word: "utilize"}
		svc := newMemService(h, change.WithCommitCheck(check))
		b := readBlock(t, svc, "a", "two")
		before := h.snapshot("a")
		res, err := svc.Apply(context.Background(), change.Set{Ops: []change.Op{edit(b.Ref, b.Rev, "Now utilize it")}}, svcPerson)
		require.NoError(t, err)
		require.Equal(t, change.SetRefused, res.Status)
		require.NotNil(t, res.Ops[0].Error)
		assert.Equal(t, change.CodeGateFailed, res.Ops[0].Error.Code)
		require.Len(t, res.Ops[0].Findings, 1)
		assert.Equal(t, "terms.vocabulary", res.Ops[0].Findings[0].Rule)
		require.NotNil(t, res.Ops[0].Findings[0].At)
		assert.Equal(t, "two", res.Ops[0].Findings[0].At.Block)
		assert.Equal(t, before, h.snapshot("a"), "nothing is written")
		require.Len(t, check.seen, 1)
		require.Len(t, check.seen[0], 1)
		assert.Equal(t, "Clean text", model.RunsText(check.seen[0][0].Before))
		assert.Equal(t, "Now utilize it", model.RunsText(check.seen[0][0].After))
	})

	t.Run("a preview's check is told it is a preview", func(t *testing.T) {
		h := newHome()
		check := &wordCheck{word: "utilize"}
		svc := newMemService(h, change.WithCommitCheck(check))
		b := readBlock(t, svc, "a", "two")
		for _, mode := range []change.Mode{change.ModePreview, change.ModeApply} {
			res, err := svc.Apply(context.Background(), change.Set{Mode: mode, Ops: []change.Op{edit(b.Ref, b.Rev, "Now use it")}}, svcPerson)
			require.NoError(t, err)
			require.NotEqual(t, change.SetRefused, res.Status, "%+v", res.Ops)
		}
		assert.Equal(t, []bool{true, false}, check.previewing, "a preview's hooks read the stores that exist and create none")
	})

	t.Run("a violation the edition already had is not held against the edit", func(t *testing.T) {
		h := newHome()
		svc := newMemService(h, change.WithCommitCheck(&wordCheck{word: "utilize"}))
		b := readBlock(t, svc, "a", "one")
		res, err := svc.Apply(context.Background(), change.Set{Ops: []change.Op{edit(b.Ref, b.Rev, "Please utilize this form")}}, svcPerson)
		require.NoError(t, err)
		require.Equal(t, change.SetApplied, res.Status, "%+v", res.Ops)
	})

	t.Run("a removed edition reaches the check with nothing after it", func(t *testing.T) {
		h := newMemHome(map[string][]memBlock{"a": {textBlock("one", "Please utilize the form", "nb", "Vennligst bruk skjemaet")}})
		check := &wordCheck{word: "utilize"}
		svc := newMemService(h, change.WithCommitCheck(check))
		b := readBlock(t, svc, "a", "one")
		nb := b.Editions["nb"]
		res, err := svc.Apply(context.Background(), change.Set{Ops: []change.Op{
			{Kind: change.KindRemoveEdition, At: atEdition(b.Ref, "nb"), IfMatch: nb.Rev, Body: &change.RemoveEdition{}},
		}}, svcPerson)
		require.NoError(t, err)
		require.Equal(t, change.SetApplied, res.Status, "%+v", res.Ops)
		require.Len(t, check.seen, 1)
		require.Len(t, check.seen[0], 1)
		removed := check.seen[0][0]
		assert.Equal(t, change.RoleDerived, removed.Role)
		assert.Equal(t, nb.Rev, removed.BeforeRev)
		assert.Equal(t, model.AbsentRevision, removed.AfterRev, "a removed edition is absent after the change")
		assert.Nil(t, removed.After)
		assert.Equal(t, "Vennligst bruk skjemaet", model.RunsText(removed.Before))
	})

	t.Run("a person's report gate lands the edit and records the override", func(t *testing.T) {
		h := newHome()
		rec := &memRecorder{}
		svc := newMemService(h, change.WithCommitCheck(&wordCheck{word: "utilize"}), change.WithRecorder(rec))
		b := readBlock(t, svc, "a", "two")
		res, err := svc.Apply(context.Background(), change.Set{Gate: change.GateReport, Ops: []change.Op{edit(b.Ref, b.Rev, "Now utilize it")}}, svcPerson)
		require.NoError(t, err)
		require.Equal(t, change.SetApplied, res.Status, "%+v", res.Ops)
		require.Len(t, res.Docs, 1)
		require.Len(t, res.Docs[0].Findings, 1)
		require.Len(t, rec.records, 1)
		assert.Len(t, rec.records[0].Overridden, 1)
		assert.Equal(t, "gov_1", rec.records[0].Fingerprint)
	})
}

// memRecorder keeps what it is handed.
type memRecorder struct {
	records []change.Record
}

func (r *memRecorder) Record(_ context.Context, rec change.Record) (string, error) {
	r.records = append(r.records, rec)
	return fmt.Sprintf("op_%d", len(r.records)), nil
}

func TestService_RecordsWhatLanded(t *testing.T) {
	h := newMemHome(map[string][]memBlock{"a": {textBlock("one", "First", "nb", "Første")}})
	rec := &memRecorder{}
	svc := newMemService(h, change.WithRecorder(rec), change.WithOrigin("desktop"))
	b := readBlock(t, svc, "a", "one")
	nb := b.Editions["nb"]

	res, err := svc.Apply(context.Background(), change.Set{Mode: change.ModePreview, Ops: []change.Op{edit(b.Ref, b.Rev, "Initial")}}, svcPerson)
	require.NoError(t, err)
	require.Equal(t, change.SetPreviewed, res.Status)
	assert.Empty(t, rec.records, "a preview records nothing")

	set := change.Set{Note: "fix the wording", Ops: []change.Op{
		edit(b.Ref, b.Rev, "Initial"),
		edit(atEdition(b.Ref, "nb"), nb.Rev, "Innledende"),
	}}
	res, err = svc.Apply(context.Background(), set, svcPerson)
	require.NoError(t, err)
	require.Equal(t, change.SetApplied, res.Status, "%+v", res.Ops)
	require.NotNil(t, res.Record)
	assert.Equal(t, "op_1", *res.Record)
	require.Len(t, rec.records, 1)
	r := rec.records[0]
	assert.Equal(t, "desktop", r.Origin)
	assert.Equal(t, svcPerson, r.Actor)
	require.NotNil(t, r.Set)
	assert.Equal(t, "fix the wording", r.Set.Note)
	require.Len(t, r.Transitions, 2)
	assert.Equal(t, change.RoleAuthoritative, r.Transitions[0].Role)
	assert.Equal(t, b.Rev, r.Transitions[0].BeforeRev)
	assert.Equal(t, res.Ops[0].After, r.Transitions[0].AfterRev)
	assert.Equal(t, model.ComputeContentHash("Initial"), r.Transitions[0].ContentHash)
	assert.Equal(t, change.RoleDerived, r.Transitions[1].Role)
	assert.Equal(t, "nb", r.Transitions[1].Ref.EditionText())
	assert.Equal(t, res.Ops[0].After, r.Transitions[1].Basis, "the derived edition records the authoritative revision it was made against")
	assert.Equal(t, []change.Invalidation{{Edition: "nb", Reason: change.ReasonBasisMoved}}, res.Ops[0].Invalidates)

	_, err = svc.Apply(context.Background(), set, svcPerson)
	require.NoError(t, err)
	assert.Len(t, rec.records, 1, "a refused change set records nothing")
}

// memAssets applies decisions and terms in memory.
type memAssets struct {
	prepared, applied []string
	refuse            string
	// during, when set, is called as each operation is applied.
	during func()
}

func (a *memAssets) Prepare(_ context.Context, _ change.Actor, op change.Op, target *change.DecisionTarget) *change.Error {
	a.prepared = append(a.prepared, string(op.Kind))
	if body, ok := op.Body.(*change.Term); ok && body.Term == a.refuse {
		return &change.Error{Code: change.CodeInvalid, Field: "term", Message: "refused"}
	}
	return nil
}

func (a *memAssets) Apply(_ context.Context, _ change.Actor, _ *change.Set, op change.Op, target *change.DecisionTarget) (change.OpStatus, *change.Error) {
	if a.during != nil {
		a.during()
	}
	switch body := op.Body.(type) {
	case *change.Decide:
		a.applied = append(a.applied, fmt.Sprintf("decide %s %s@%s %s %q on %q", body.Outcome, target.Ref.Block, target.Ref.EditionText(), target.Rev, target.Text, target.SourceText))
	case *change.Term:
		a.applied = append(a.applied, "term "+body.Term)
	}
	return change.OpApplied, nil
}

func TestService_DecisionsAndAssetsFollowTheContent(t *testing.T) {
	h := newMemHome(map[string][]memBlock{"a": {textBlock("one", "First", "nb", "Første")}})
	assets := &memAssets{}
	svc := newMemService(h, change.WithAssets(assets))
	b := readBlock(t, svc, "a", "one")
	nb := b.Editions["nb"]

	res, err := svc.Apply(context.Background(), change.Set{Ops: []change.Op{
		{Kind: change.KindTerm, Body: &change.Term{Action: "upsert", Term: "handbook"}},
		edit(atEdition(b.Ref, "nb"), nb.Rev, "Innledende"),
		{Kind: change.KindDecide, At: atEdition(b.Ref, "nb"), IfMatch: nb.Rev, Body: &change.Decide{Outcome: change.OutcomeEstablish}},
	}}, svcPerson)
	require.NoError(t, err)
	require.Equal(t, change.SetApplied, res.Status, "%+v", res.Ops)
	after := res.Ops[1].After
	assert.Equal(t, []string{"term handbook", `decide establish one@nb ` + after + ` "Innledende" on "First"`}, assets.applied,
		"assets apply in the order of the change set, after the content, and a decision binds to the content that landed")
	assert.Equal(t, change.OpApplied, res.Ops[2].Status)
	assert.Equal(t, after, res.Ops[2].After)

	t.Run("a decision on content that moved is stale", func(t *testing.T) {
		res, err := svc.Apply(context.Background(), change.Set{Ops: []change.Op{
			{Kind: change.KindDecide, At: atEdition(b.Ref, "nb"), IfMatch: nb.Rev, Body: &change.Decide{Outcome: change.OutcomeEstablish}},
		}}, svcPerson)
		require.NoError(t, err)
		require.Equal(t, change.SetRefused, res.Status)
		assert.Equal(t, change.CodeStale, res.Ops[0].Error.Code)
		require.NotNil(t, res.Ops[0].Current)
		assert.Equal(t, "Innledende", res.Ops[0].Current.Text)
	})

	t.Run("an asset the store would refuse refuses the content too", func(t *testing.T) {
		assets.refuse = "bad"
		cur := readBlock(t, svc, "a", "one")
		before := h.snapshot("a")
		res, err := svc.Apply(context.Background(), change.Set{Ops: []change.Op{
			edit(cur.Ref, cur.Rev, "Changed"),
			{Kind: change.KindTerm, Body: &change.Term{Action: "upsert", Term: "bad"}},
		}}, svcPerson)
		require.NoError(t, err)
		require.Equal(t, change.SetRefused, res.Status)
		assert.Equal(t, change.OpNotApplied, res.Ops[0].Status)
		assert.Equal(t, before, h.snapshot("a"))
	})

	t.Run("without a host for assets they are unsupported", func(t *testing.T) {
		res, err := newMemService(h).Apply(context.Background(), change.Set{Ops: []change.Op{
			{Kind: change.KindTerm, Body: &change.Term{Action: "upsert", Term: "handbook"}},
		}}, svcPerson)
		require.NoError(t, err)
		require.Equal(t, change.SetRefused, res.Status)
		assert.Equal(t, change.CodeUnsupported, res.Ops[0].Error.Code)
	})
}

func TestService_Refusals(t *testing.T) {
	h := newMemHome(map[string][]memBlock{
		"a":   {textBlock("one", "First"), textBlock("dup", "Twice"), textBlock("dup", "Again"), {key: "code", editions: map[model.EditionKey][]model.Run{{}: {model.TextR("x := 1")}}}},
		"big": {textBlock("one", "First")},
	})
	svc := newMemService(h)
	ctx := context.Background()
	one := readBlock(t, svc, "a", "one")

	tests := []struct {
		name string
		op   change.Op
		code change.Code
	}{
		{"an operation that names no document is invalid", edit(change.Ref{Block: "one"}, one.Rev, "x"), change.CodeInvalid},
		{"a document the home does not hold is not found", edit(change.Ref{Doc: "missing", Block: "one"}, one.Rev, "x"), change.CodeNotFound},
		{"a key two blocks share is ambiguous", edit(change.Ref{Doc: "a", Block: "dup"}, "*", "x"), change.CodeAmbiguous},
		{"a block the document marks as not content is not editable", edit(change.Ref{Doc: "a", Block: "code"}, "*", "y := 2"), change.CodeUnsupported},
		{"an operation the format does not support is unsupported",
			change.Op{Kind: change.KindSetAttribute, At: one.Ref, IfMatch: one.Rev, Body: &change.SetAttribute{Code: "1", Name: "href", Value: "x"}}, change.CodeUnsupported},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			res, err := svc.Apply(ctx, change.Set{Ops: []change.Op{tc.op}}, svcPerson)
			require.NoError(t, err)
			require.Equal(t, change.SetRefused, res.Status, "%+v", res.Ops)
			require.NotNil(t, res.Ops[0].Error)
			assert.Equal(t, tc.code, res.Ops[0].Error.Code, res.Ops[0].Error.Message)
		})
	}

	t.Run("a near key is named among the candidates of a block not found", func(t *testing.T) {
		res, err := svc.Apply(ctx, change.Set{Ops: []change.Op{edit(change.Ref{Doc: "a", Block: "onee"}, "*", "x")}}, svcPerson)
		require.NoError(t, err)
		require.NotNil(t, res.Ops[0].Error)
		assert.Equal(t, change.CodeNotFound, res.Ops[0].Error.Code)
		require.NotEmpty(t, res.Ops[0].Error.Candidates)
		assert.Equal(t, "one", res.Ops[0].Error.Candidates[0].Key)
	})

	t.Run("a resource bound the home hits is budget_exceeded", func(t *testing.T) {
		h.openErr = fmt.Errorf("read big: %w", safeio.DefaultBudget().WithMaxBytes(10).CheckSize(11))
		defer func() { h.openErr = nil }()
		res, err := svc.Apply(ctx, change.Set{Ops: []change.Op{edit(change.Ref{Doc: "big", Block: "one"}, "*", "x")}}, svcPerson)
		require.NoError(t, err)
		require.NotNil(t, res.Ops[0].Error)
		assert.Equal(t, change.CodeBudgetExceeded, res.Ops[0].Error.Code)
	})
}

func TestService_Describe(t *testing.T) {
	svc := newMemService(newMemHome(map[string][]memBlock{"a": {textBlock("one", "First")}}))
	ctx := context.Background()
	d, err := svc.Describe(ctx, change.DescribeRequest{Doc: "a"})
	require.NoError(t, err)
	assert.Equal(t, "memory", d.Format)
	assert.Equal(t, change.EditionsInFile, d.Editions)
	require.NotNil(t, d.Ops.SetContent)
	assert.Equal(t, []string{"text", "runs"}, d.Ops.SetContent.Forms)
	assert.NotNil(t, d.Ops.ReplaceText)
	assert.NotNil(t, d.Ops.RemoveEdition, "a format that holds editions in one file removes one")
	assert.Nil(t, d.Ops.SetAttribute)
	assert.Nil(t, d.Ops.Mark)
	js, err := json.Marshal(d)
	require.NoError(t, err)
	assert.Contains(t, string(js), `"insert_block":null`, "every operation is listed, null when refused")

	_, err = svc.Describe(ctx, change.DescribeRequest{Format: "nonesuch"})
	var ce *change.Error
	require.ErrorAs(t, err, &ce)
	assert.Equal(t, change.CodeNotFound, ce.Code)

	plugged := newMemService(newMemHome(nil), change.WithDescriber(func(f change.FormatFacts) change.Description {
		f.Edit.WritableAttrs = map[string][]string{"link:hyperlink": {"href"}}
		return change.DescribeFormat(f)
	}))
	d, err = plugged.Describe(ctx, change.DescribeRequest{Format: "memory"})
	require.NoError(t, err)
	assert.Equal(t, map[string][]string{"link:hyperlink": {"href"}}, d.Ops.SetAttribute, "the capability table plugs in through one function")
}

// A document the home cannot open for a reason that is no refusal is an
// error from Describe, never an empty description.
func TestService_DescribeReportsAFailedOpen(t *testing.T) {
	h := newMemHome(map[string][]memBlock{"a": {textBlock("one", "First")}})
	h.openErr = errors.New("the disk is gone")
	d, err := newMemService(h).Describe(context.Background(), change.DescribeRequest{Doc: "a"})
	require.Error(t, err)
	assert.Nil(t, d)
	var ce *change.Error
	assert.NotErrorAs(t, err, &ce, "a failure that is no refusal is no *change.Error")
	assert.Contains(t, err.Error(), "the disk is gone")
}

func TestService_ReadsShowEditionsAndStaleness(t *testing.T) {
	h := newMemHome(map[string][]memBlock{"a": {textBlock("one", "First", "nb", "Første", "de", "Erste")}})
	var authRev string
	states := editionStates(func(b *model.Block, k model.EditionKey) (change.EditionState, bool) {
		if k.Locale == "nb" {
			return change.EditionState{Status: "translated", Basis: authRev}, true
		}
		return change.EditionState{Status: "draft", Basis: "r:0000000000000000"}, true
	})
	svc := newMemService(h, change.WithEditionStates(states))
	b := readBlock(t, svc, "a", "one")
	authRev = b.Rev
	b = readBlock(t, svc, "a", "one")
	require.Contains(t, b.Editions, "nb")
	require.Contains(t, b.Editions, "de")
	assert.False(t, b.Editions["nb"].Stale, "made from the authoritative edition as it stands")
	assert.Equal(t, "translated", b.Editions["nb"].Status)
	assert.True(t, b.Editions["de"].Stale, "made from an older authoritative edition")
	assert.Equal(t, "Erste", b.Editions["de"].Text)
	assert.Equal(t, []change.Kind{change.KindSetContent, change.KindReplaceText, change.KindRemoveEdition}, b.Ops)
}

type editionStates func(b *model.Block, k model.EditionKey) (change.EditionState, bool)

func (f editionStates) EditionState(_ context.Context, _ change.DocInfo, b *model.Block, k model.EditionKey) (change.EditionState, bool) {
	return f(b, k)
}

// TestService_EachOperationReportsItsOwnOutcome pins that a refusal on one
// block leaves the operations on another block of the document to report
// what they would have done (not_applied, blocked by the refusal), and that
// an operation held back by a refusal on its own block is not_applied, never
// a block that is not there.
func TestService_EachOperationReportsItsOwnOutcome(t *testing.T) {
	h := newMemHome(map[string][]memBlock{"a": {textBlock("one", "First"), textBlock("two", "Second")}})
	svc := newMemService(h, change.WithAssets(&memAssets{}))
	one := readBlock(t, svc, "a", "one")
	two := readBlock(t, svc, "a", "two")
	stale := "r:0000000000000000"

	tests := []struct {
		name string
		ops  []change.Op
	}{
		{"a stale edit before a valid one on another block", []change.Op{edit(one.Ref, stale, "x"), edit(two.Ref, two.Rev, "y")}},
		{"a stale decision before an edit of its block", []change.Op{
			{Kind: change.KindDecide, At: two.Ref, IfMatch: stale, Body: &change.Decide{Outcome: change.OutcomeEstablish}},
			edit(two.Ref, two.Rev, "y"),
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			before := h.snapshot("a")
			res, err := svc.Apply(context.Background(), change.Set{Ops: tc.ops}, svcPerson)
			require.NoError(t, err)
			require.Equal(t, change.SetRefused, res.Status)
			require.NotNil(t, res.Ops[0].Error)
			assert.Equal(t, change.CodeStale, res.Ops[0].Error.Code)
			assert.Equal(t, change.OpNotApplied, res.Ops[1].Status, "%+v", res.Ops[1].Error)
			require.NotNil(t, res.Ops[1].BlockedBy)
			assert.Equal(t, 0, *res.Ops[1].BlockedBy)
			assert.Equal(t, before, h.snapshot("a"))
		})
	}
}

// TestService_OneBlockAddressedByTwoKeys pins that two operations naming one
// block by two of the keys it answers to (its name and its id) both reach it.
func TestService_OneBlockAddressedByTwoKeys(t *testing.T) {
	h := newMemHome(map[string][]memBlock{"a": {textBlock("one", "First", "nb", "Første")}})
	svc := newMemService(h)
	b := readBlock(t, svc, "a", "one")
	byID := b.Ref
	byID.Block = "tu1"
	res, err := svc.Apply(context.Background(), change.Set{Ops: []change.Op{
		edit(b.Ref, b.Rev, "Initial"),
		edit(atEdition(byID, "nb"), b.Editions["nb"].Rev, "Innledende"),
	}}, svcPerson)
	require.NoError(t, err)
	require.Equal(t, change.SetApplied, res.Status, "%+v", res.Ops)
	got := readBlock(t, svc, "a", "one")
	assert.Equal(t, "Initial", got.Text)
	assert.Equal(t, "Innledende", got.Editions["nb"].Text)
}

// TestService_TheCommitCheckSeesThePassThatCommits pins that when the home
// applies a change set again at commit, because the document moved, the
// commit check runs over that second pass: an operation that writes
// whatever is there (if_match *) can introduce a finding the first pass did
// not have, and that pass is the one that lands.
func TestService_TheCommitCheckSeesThePassThatCommits(t *testing.T) {
	tests := []struct {
		name   string
		gate   change.Gate
		status change.SetStatus
	}{
		{"enforce refuses it", change.GateEnforce, change.SetRefused},
		{"report lands it with the finding", change.GateReport, change.SetApplied},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := newMemHome(map[string][]memBlock{"a": {textBlock("one", "Clean text")}})
			check := &wordCheck{word: "utilize"}
			svc := newMemService(h, change.WithCommitCheck(check))
			b := readBlock(t, svc, "a", "one")
			h.beforeSettle = func(string) {
				h.beforeSettle = nil
				// Another writer lands first; the edit below applies again
				// to what it wrote.
				h.mu.Lock()
				h.docs["a"].blocks[0].editions[model.EditionKey{}] = []model.Run{model.TextR("Clean utiltext")}
				h.docs["a"].head++
				h.mu.Unlock()
			}
			find := "text"
			res, err := svc.Apply(context.Background(), change.Set{Gate: tc.gate, Ops: []change.Op{{
				Kind: change.KindReplaceText, At: b.Ref, IfMatch: change.AnyRevision,
				Body: &change.ReplaceText{Edits: []change.TextEdit{{Find: &find, Text: "ize"}}},
			}}}, svcPerson)
			require.NoError(t, err)
			require.Len(t, check.seen, 2, "the check runs over the first pass and again over the pass that commits")
			assert.Equal(t, "Clean ize", model.RunsText(check.seen[0][0].After))
			assert.Equal(t, "Clean utilize", model.RunsText(check.seen[1][0].After))
			require.Equal(t, tc.status, res.Status, "%+v", res.Ops)
			require.Len(t, res.Ops[0].Findings, 1, "the finding of the pass that commits is reported")
			assert.Equal(t, "avoid utilize", res.Ops[0].Findings[0].Message)
			if tc.status == change.SetRefused {
				assert.Equal(t, change.CodeGateFailed, res.Ops[0].Error.Code)
				assert.Equal(t, "Clean utiltext", readBlock(t, svc, "a", "one").Text, "nothing is written")
			} else {
				assert.Equal(t, "Clean utilize", readBlock(t, svc, "a", "one").Text)
			}
		})
	}
}

// TestService_APartialCommitReportsWhatLanded pins what an I/O error during
// the final commits reports: partial, with the documents that landed written
// and their operations applied, the rest not_applied, the decisions and
// assets not_applied because the content they bind to did not all land, and
// a record of what did.
func TestService_APartialCommitReportsWhatLanded(t *testing.T) {
	h := newMemHome(map[string][]memBlock{
		"a": {textBlock("one", "First", "nb", "Første")},
		"b": {textBlock("two", "Second")},
	})
	h.failCommit = "b"
	rec := &memRecorder{}
	assets := &memAssets{}
	svc := newMemService(h, change.WithRecorder(rec), change.WithAssets(assets))
	one := readBlock(t, svc, "a", "one")
	two := readBlock(t, svc, "b", "two")

	res, err := svc.Apply(context.Background(), change.Set{Ops: []change.Op{
		edit(one.Ref, one.Rev, "Initial"),
		edit(two.Ref, two.Rev, "Next"),
		{Kind: change.KindDecide, At: atEdition(one.Ref, "nb"), IfMatch: one.Editions["nb"].Rev, Body: &change.Decide{Outcome: change.OutcomeEstablish}},
		{Kind: change.KindTerm, Body: &change.Term{Action: "upsert", Term: "handbook"}},
	}}, svcPerson)
	require.NoError(t, err)
	require.Equal(t, change.SetPartial, res.Status, "%+v", res.Ops)
	assert.Equal(t, change.OpApplied, res.Ops[0].Status)
	assert.Equal(t, change.OpNotApplied, res.Ops[1].Status)
	assert.Equal(t, change.OpNotApplied, res.Ops[2].Status, "a decision waits for content that all landed")
	assert.Equal(t, change.OpNotApplied, res.Ops[3].Status, "so does an asset")
	assert.Empty(t, assets.applied)
	written := map[string]bool{}
	for _, d := range res.Docs {
		written[d.Doc] = d.Written
	}
	assert.Equal(t, map[string]bool{"a": true, "b": false}, written)
	require.Len(t, rec.records, 1, "what landed is recorded")
	require.Len(t, rec.records[0].Transitions, 1)
	assert.Equal(t, "a", rec.records[0].Transitions[0].Ref.Doc)
	require.NotNil(t, res.Record)

	t.Run("a failure before anything landed is an error", func(t *testing.T) {
		h.failCommit = "a"
		cur := readBlock(t, svc, "a", "one")
		_, err := svc.Apply(context.Background(), change.Set{Ops: []change.Op{edit(cur.Ref, cur.Rev, "Again")}}, svcPerson)
		require.Error(t, err)
	})
}

// TestService_DecisionsAreAppliedUnderTheCommitLock pins that the service
// holds a document's commit lock until the decisions on it are applied, so
// no other writer's content can land between the commit and the decision.
func TestService_DecisionsAreAppliedUnderTheCommitLock(t *testing.T) {
	h := newMemHome(map[string][]memBlock{"a": {textBlock("one", "First", "nb", "Første")}})
	var heldDuringDecision bool
	assets := &memAssets{during: func() {
		if h.docs["a"].commit.TryLock() {
			h.docs["a"].commit.Unlock()
			return
		}
		heldDuringDecision = true
	}}
	svc := newMemService(h, change.WithAssets(assets))
	b := readBlock(t, svc, "a", "one")
	nb := b.Editions["nb"]
	res, err := svc.Apply(context.Background(), change.Set{Ops: []change.Op{
		edit(atEdition(b.Ref, "nb"), nb.Rev, "Innledende"),
		{Kind: change.KindDecide, At: atEdition(b.Ref, "nb"), IfMatch: nb.Rev, Body: &change.Decide{Outcome: change.OutcomeEstablish}},
	}}, svcPerson)
	require.NoError(t, err)
	require.Equal(t, change.SetApplied, res.Status, "%+v", res.Ops)
	assert.True(t, heldDuringDecision, "the commit lock is held while the decision is applied")
	assert.True(t, h.docs["a"].commit.TryLock(), "and released once the change set is done")
	h.docs["a"].commit.Unlock()
}
