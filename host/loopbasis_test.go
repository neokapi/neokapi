package host

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/gate"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/state"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The basis grading, over the two classes of record that share the index: a
// person's decision, and the loop's record of a target it wrote. They are graded
// against the source by the same comparison, which is the point — drift is drift
// whoever produced the translation — and they differ in what a rewritten
// TRANSLATION does to them.

// gradeBlock builds the block a grading is asked about.
func gradeBlock(source, target string) *model.Block {
	b := model.NewBlock("u1", source)
	b.Name = "u1"
	b.Translatable = true
	if target != "" {
		b.SetTargetText("nb", target)
	}
	return b
}

func TestReviewedIndex_GradeBasis(t *testing.T) {
	const (
		scope     = "d-doc"
		source    = "Apple"
		rewritten = "Apricot"
		target    = "Eple"
		edited    = "Eplet"
	)
	decision := state.Decision{ReviewState: "approved"}

	tests := []struct {
		name string
		rec  state.UnitState
		// the content in front of the reader
		source, target string
		wantBasis      basisVerdict
		wantApplies    bool
	}{
		{
			name:      "no record at all is basisNone, whatever is on disk",
			source:    rewritten,
			target:    target,
			wantBasis: basisNone,
		},
		{
			name: "a decision whose source moved is stale",
			rec: state.UnitState{
				Status: model.TargetStatusEstablished, Decision: decision,
				TargetHash: state.TargetHash(target), ContentHash: state.SourceHash(source),
			},
			source:    rewritten,
			target:    target,
			wantBasis: basisStale,
		},
		{
			name: "a decision on the source the project holds still applies",
			rec: state.UnitState{
				Status: model.TargetStatusEstablished, Decision: decision,
				TargetHash: state.TargetHash(target), ContentHash: state.SourceHash(source),
			},
			source:      source,
			target:      target,
			wantBasis:   basisCurrent,
			wantApplies: true,
		},
		{
			name: "a decision written before the basis was tracked is unknown, never stale",
			rec: state.UnitState{
				Status: model.TargetStatusEstablished, Decision: decision,
				TargetHash: state.TargetHash(target),
			},
			source:      rewritten,
			target:      target,
			wantBasis:   basisUnknown,
			wantApplies: true,
		},
		{
			// The whole point of the loop's own record: the same rewrite, under
			// a translation nobody has decided, reads the same way.
			name: "a basis the loop recorded, source moved, is stale",
			rec: state.UnitState{
				Status:     model.TargetStatusTranslated,
				TargetHash: state.TargetHash(target), ContentHash: state.SourceHash(source),
			},
			source:    rewritten,
			target:    target,
			wantBasis: basisStale,
		},
		{
			name: "a basis the loop recorded, source unchanged, is current and decides nothing",
			rec: state.UnitState{
				Status:     model.TargetStatusTranslated,
				TargetHash: state.TargetHash(target), ContentHash: state.SourceHash(source),
			},
			source:    source,
			target:    target,
			wantBasis: basisCurrent,
		},
		{
			// A person took the translation over. Their wording renders whatever
			// they had in front of them, which the record cannot name, so the
			// loop stops speaking for the unit rather than re-drafting over it.
			name: "a basis the loop recorded, translation rewritten by hand, is unknown",
			rec: state.UnitState{
				Status:     model.TargetStatusTranslated,
				TargetHash: state.TargetHash(target), ContentHash: state.SourceHash(source),
			},
			source:    rewritten,
			target:    edited,
			wantBasis: basisUnknown,
		},
		{
			// A decision reads its rewritten translation differently: it is
			// still a decision ABOUT a source, and coverage says the source
			// moved so the scope does not ship.
			name: "a decision whose translation was rewritten still grades its source",
			rec: state.UnitState{
				Status: model.TargetStatusEstablished, Decision: decision,
				TargetHash: state.TargetHash(target), ContentHash: state.SourceHash(source),
			},
			source:    rewritten,
			target:    edited,
			wantBasis: basisStale,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			idx := reviewedIndex{byUnit: map[string]reviewedEntry{}, aiReviews: map[string]aiReviewEntry{}}
			if tc.rec.TargetHash != "" || tc.rec.ContentHash != "" {
				rec := tc.rec
				rec.Unit, rec.Scope, rec.Variant = "u1", scope, model.Variant("nb")
				loadStateInto(idx, rec)
			}
			b := gradeBlock(tc.source, tc.target)
			e, basis, applies := idx.grade(scope, b, "nb")
			assert.Equal(t, tc.wantBasis, basis)
			assert.Equal(t, tc.wantApplies, applies)
			if !tc.wantApplies {
				assert.False(t, approvesTarget(e, applies), "nothing applies, so nothing approves")
			}
		})
	}
}

// loadStateInto mirrors what loadReviewedCorrections does for one record, so the
// table above exercises the classification the store feeds rather than a
// hand-built entry.
func loadStateInto(idx reviewedIndex, u state.UnitState) {
	key := reviewUnitKey(u.Scope, u.Unit, string(u.Variant.Locale))
	switch u.Status {
	case model.TargetStatusEstablished, model.TargetStatusDraft:
		idx.byUnit[key] = reviewedEntry{
			status: u.Status, targetHash: u.TargetHash,
			contentHash: u.ContentHash, by: u.Decision.By, decided: true,
		}
	default:
		if u.TargetHash != "" && u.Decision.ReviewState == "" {
			idx.byUnit[key] = reviewedEntry{
				status: u.Status, targetHash: u.TargetHash, contentHash: u.ContentHash,
			}
		}
	}
}

// A loop-written basis never moves a unit on the ladder. It carries a status so
// the record says what the target is, and `apply` has to ignore it: reading it
// would report every translation the loop produced at whatever rung the record
// happened to name.
func TestReviewedIndex_ApplyIgnoresARecordedBasis(t *testing.T) {
	idx := reviewedIndex{byUnit: map[string]reviewedEntry{}, aiReviews: map[string]aiReviewEntry{}}
	loadStateInto(idx, state.UnitState{
		Unit: "u1", Scope: "d-doc", Variant: model.Variant("nb"),
		Status:     model.TargetStatusTranslated,
		TargetHash: state.TargetHash("Eple"), ContentHash: state.SourceHash("Apple"),
	})
	b := gradeBlock("Apple", "Eple")

	read := idx.apply(string(model.TargetStatusTranslated), "d-doc", b, "nb")
	assert.Equal(t, string(model.TargetStatusTranslated), read.state)
	assert.Equal(t, basisCurrent, read.basis)
	assert.False(t, read.rejectedOwed, "a basis the loop recorded is no verdict")
	assert.False(t, idx.decided("d-doc", b, "nb"), "the loop deciding its own output is what this forbids")
}

// recordFlowWrite records, through the project's edit recorder, that a flow
// wrote the locale edition of every block of the source document as the
// change service reads it now: one hash-only content.edit with actor
// tool:translate, each transition carrying the source as its basis and the
// stamp producer left.
func recordFlowWrite(t *testing.T, a *App, recipe, doc string, locale model.LocaleID, producer model.Origin) {
	t.Helper()
	ctx := context.Background()
	root := filepath.Dir(recipe)
	svc, err := a.ChangeService(ctx, ChangeServiceOptions{Project: recipe, SourceLocale: "en"})
	require.NoError(t, err)
	k := model.EditionKey{Locale: locale}
	var transitions []change.Transition
	_, err = svc.ReadEach(ctx, change.ReadRequest{Doc: doc, Editions: []model.EditionKey{k}}, func(b *model.Block, r change.BlockRead) error {
		ed, ok := b.Edition(k)
		if !ok {
			return nil
		}
		ed.Origin = producer
		b.SetEdition(k, ed)
		transitions = append(transitions, change.Transition{
			Ref: change.Ref{Doc: doc, Block: r.Ref.Block, Edition: k}, Role: change.RoleDerived,
			BeforeRev: model.AbsentRevision, AfterRev: model.EditionRevision(b, k),
			Basis: model.EditionRevision(b, b.EditionKeyOf(b.Authoritative(model.AuthorityPolicy{}))), Block: b,
		})
		return nil
	})
	require.NoError(t, err)
	require.NotEmpty(t, transitions, "the document holds a %s translation", locale)
	rec, err := a.EditRecorder(ctx, root)
	require.NoError(t, err)
	_, err = rec.Record(ctx, change.Record{
		Actor: flowActor("translate"), Origin: flowOrigin("translate"),
		Docs:        []change.DocResult{{Doc: doc, Home: "file", Written: true}},
		Transitions: transitions,
	})
	require.NoError(t, err)
}

// staleCount is how many units coverage grades stale across the project.
func staleCount(t *testing.T, a *App, recipe string) int {
	t.Helper()
	proj, err := project.LoadWithOptions(recipe, project.LoadOptions{SkipRequiresCheck: true})
	require.NoError(t, err)
	root := filepath.Dir(recipe)
	units, err := a.UnitsFromProject(proj, root, "")
	require.NoError(t, err)
	tally, err := a.ProjectCoverageTally(context.Background(), proj, root, units, nil)
	require.NoError(t, err)
	stale := 0
	for _, lc := range tally.Rollup(gate.RuleSet{}) {
		stale += lc.Stale
	}
	return stale
}

// TestLoopBasis_ASourceEditUnderTheLoopsTranslationReadsStale is the basis the
// block history keeps for an undecided translation: the loop's own write is
// graded against the source it was made from, and only the loop's.
func TestLoopBasis_ASourceEditUnderTheLoopsTranslationReadsStale(t *testing.T) {
	a, cmd, recipe := newFlowProject(t, project.MaterializeManual)
	root := filepath.Dir(recipe)
	src := filepath.Join(root, "src", "en.json")
	runOnePass(t, a, cmd, recipe)
	require.Zero(t, staleCount(t, a, recipe), "the loop's translations were made from the source in front of it")

	require.NoError(t, os.WriteFile(src,
		[]byte(`{"greeting": "Hello there", "farewell": "Goodbye now", "thanks": "Thank you"}`+"\n"), 0o644))
	assert.Equal(t, 1, staleCount(t, a, recipe), "the source moved out from under the loop's translation")

	runOnePass(t, a, cmd, recipe)
	assert.Zero(t, staleCount(t, a, recipe), "the loop re-drafted it from the source it holds now")

	// A person rewrites a translation by hand. It is no longer the loop's
	// work, so a later source edit under it does not read stale and the loop
	// leaves it where they put it.
	target := filepath.Join(root, "src", "qps.json")
	data, err := os.ReadFile(target)
	require.NoError(t, err)
	var catalog map[string]string
	require.NoError(t, json.Unmarshal(data, &catalog))
	catalog["farewell"] = "Written by a person"
	edited, err := json.Marshal(catalog)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(target, edited, 0o644))
	require.NoError(t, os.WriteFile(src,
		[]byte(`{"greeting": "Hello there", "farewell": "Goodbye for now", "thanks": "Thank you"}`+"\n"), 0o644))
	assert.Zero(t, staleCount(t, a, recipe), "a person's translation has no basis the loop can grade")
}

// TestLoopBasis_ADecisionOnTheLoopsTranslationKeepsItsProducer: a reviewer's
// decision on a translation the loop wrote starts from the flow's record, so
// the producer's stamp rides along as it does from a record in the store.
func TestLoopBasis_ADecisionOnTheLoopsTranslationKeepsItsProducer(t *testing.T) {
	root := writeStalenessProject(t)
	recipe := filepath.Join(root, "kapi.yaml")
	a := &App{}
	a.InitRegistries()
	ctx := context.Background()
	recordFlowWrite(t, a, recipe, "locales/en/app.json", "fr", model.Origin{Kind: model.OriginAI, ContextFingerprint: "fp-produced"})

	_, err := a.ApplyReviewDecision(ctx, recipe, "en",
		ReviewUnitRef{File: "locales/fr/app.json", Key: "greeting", Locale: "fr"}, ReviewDecisionRejected, "")
	require.NoError(t, err)

	st, err := a.OpenProjectState(ctx, root)
	require.NoError(t, err)
	u, ok := st.Get(ctx, state.Key{Scope: a.DocumentScope(ctx, root, filepath.Join(root, "locales", "en", "app.json")),
		Unit: "greeting", Variant: model.Variant("fr")})
	require.True(t, ok)
	assert.Equal(t, "rejected", u.Decision.ReviewState)
	assert.Equal(t, "fp-produced", u.Origin.ContextFingerprint, "the producer's stamp rides along")
	assert.Equal(t, state.SourceHash("Hello there"), u.ContentHash, "a rejection keeps the basis the loop recorded")
	assert.Equal(t, "fp-produced", u.GoverningBasis())
}
