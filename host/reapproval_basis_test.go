package host

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/gate"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/state"
)

// The basis a re-approval re-stamps.
//
// A unit whose source or governing context has moved out from under its
// translation reads stale, and stays stale through the re-draft: the loop may
// not write over a decision. What clears it is the next decision, and only an
// approval: a reviewer looking at the new draft under the governance in force
// and saying it stands. A rejection is a verdict too, and it endorses nothing,
// so the basis stays where the last approval left it and the unit stays stale.

// reapprovalFixture is the staleness project with one loop-written record for
// greeting/fr, stamped under a context the caller names.
type reapprovalFixture struct {
	app    *App
	root   string
	recipe string
	scope  string
	key    state.Key
	ref    ReviewUnitRef
}

func newReapprovalFixture(t *testing.T, stamp string) *reapprovalFixture {
	t.Helper()
	root := writeStalenessProject(t)
	a := &App{}
	a.InitRegistries()
	ctx := context.Background()

	st, err := a.OpenProjectState(ctx, root)
	require.NoError(t, err)
	scope := a.DocumentScope(ctx, root, filepath.Join(root, "locales", "en", "app.json"))
	require.NoError(t, st.Record(ctx, state.UnitState{
		Unit: "greeting", Variant: model.Variant("fr"), Scope: scope,
		Status:               model.TargetStatusTranslated,
		TargetHash:           state.TargetHash("Bonjour"),
		ContentHash:          state.SourceHash("Hello there"),
		GoverningFingerprint: stamp,
		Origin:               model.Origin{Kind: model.OriginAI, ContextFingerprint: stamp},
	}))

	return &reapprovalFixture{
		app: a, root: root, recipe: filepath.Join(root, "kapi.yaml"), scope: scope,
		key: state.Key{Scope: scope, Unit: "greeting", Variant: model.Variant("fr")},
		ref: ReviewUnitRef{File: "locales/fr/app.json", Key: "greeting", Locale: "fr"},
	}
}

// record reads back the unit's state record.
func (f *reapprovalFixture) record(t *testing.T) state.UnitState {
	t.Helper()
	st, err := f.app.OpenProjectState(context.Background(), f.root)
	require.NoError(t, err)
	u, ok := st.Get(context.Background(), f.key)
	require.True(t, ok, "the unit must hold a record")
	return u
}

// rewriteSource replaces the source wording, which is what puts the recorded
// basis behind the project.
func (f *reapprovalFixture) rewriteSource(t *testing.T, text string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(f.root, "locales", "en", "app.json"),
		[]byte("{\n  \"greeting\": \""+text+"\"\n}\n"), 0o644))
	f.app = &App{}
	f.app.InitRegistries()
}

// staleUnits counts the units coverage grades stale across the project.
func (f *reapprovalFixture) staleUnits(t *testing.T) int {
	t.Helper()
	ctx := context.Background()
	proj, err := project.LoadWithOptions(f.recipe, project.LoadOptions{SkipRequiresCheck: true})
	require.NoError(t, err)
	units, err := f.app.UnitsFromProject(proj, f.root, "")
	require.NoError(t, err)
	tally, err := f.app.ProjectCoverageTally(ctx, proj, f.root, units, nil)
	require.NoError(t, err)
	stale := 0
	for _, lc := range tally.Rollup(gate.RuleSet{}) {
		stale += lc.Stale
	}
	return stale
}

// TestApprovalRestampsTheBasis_RejectionDoesNot is the writer's half: an
// approval binds the record to the source in front of the reviewer and the
// context they decided under, and a rejection leaves both where they were.
func TestApprovalRestampsTheBasis_RejectionDoesNot(t *testing.T) {
	f := newReapprovalFixture(t, "fp-produced")
	ctx := context.Background()
	want := governingNow(t, f.app, f.recipe, f.root)
	require.NotEqual(t, "fp-produced", want)

	changed, err := f.app.ApplyReviewDecision(ctx, f.recipe, "en", f.ref, ReviewDecisionApproved, "")
	require.NoError(t, err)
	require.True(t, changed)

	approved := f.record(t)
	assert.Equal(t, state.SourceHash("Hello there"), approved.ContentHash,
		"the approval binds the source the reviewer read")
	assert.Equal(t, want, approved.GoverningFingerprint,
		"the approval binds the context it was made under")
	assert.Equal(t, "fp-produced", approved.Origin.ContextFingerprint,
		"the producer's stamp rides along untouched")

	// The source moves and the unit is re-drafted; the reviewer turns the new
	// draft down.
	f.rewriteSource(t, "Hi there")
	changed, err = f.app.ApplyReviewDecision(ctx, f.recipe, "en", f.ref, ReviewDecisionRejected, "not our wording")
	require.NoError(t, err)
	require.True(t, changed)

	rejected := f.record(t)
	assert.Equal(t, model.TargetStatusDraft, rejected.Status)
	assert.Equal(t, "rejected", rejected.Decision.ReviewState)
	assert.Equal(t, approved.ContentHash, rejected.ContentHash,
		"a rejection endorses nothing, so the basis stays where the approval left it")
	assert.Equal(t, approved.GoverningFingerprint, rejected.GoverningFingerprint,
		"and so does the context that basis stands under")
}

// TestReApprovalClearsStale_ReDraftAndRejectionDoNot is the reader's half over
// the source basis: what coverage grades stale before and after each verdict.
func TestReApprovalClearsStale_ReDraftAndRejectionDoNot(t *testing.T) {
	f := newReapprovalFixture(t, "fp-produced")
	ctx := context.Background()

	_, err := f.app.ApplyReviewDecision(ctx, f.recipe, "en", f.ref, ReviewDecisionApproved, "")
	require.NoError(t, err)
	require.Zero(t, f.staleUnits(t), "an approval of the current source is not stale")

	f.rewriteSource(t, "Hi there")
	assert.Equal(t, 1, f.staleUnits(t), "the source moved out from under the approval")

	// The loop re-drafts the unit and records what it produced. The decision is
	// not its to replace, so the unit stays stale until a person looks again.
	require.NoError(t, f.app.recordProducedBasis(ctx, f.project(t), f.root, func(string, string) bool { return true }))
	assert.Equal(t, 1, f.staleUnits(t), "a re-draft cannot decide, so it cannot clear the decision")

	_, err = f.app.ApplyReviewDecision(ctx, f.recipe, "en", f.ref, ReviewDecisionRejected, "")
	require.NoError(t, err)
	assert.Equal(t, 1, f.staleUnits(t), "a rejection leaves the unit stale")

	_, err = f.app.ApplyReviewDecision(ctx, f.recipe, "en", f.ref, ReviewDecisionApproved, "")
	require.NoError(t, err)
	assert.Zero(t, f.staleUnits(t), "the re-approval is the decision the basis records")
}

// project loads the fixture's recipe.
func (f *reapprovalFixture) project(t *testing.T) *project.KapiProject {
	t.Helper()
	proj, err := project.LoadWithOptions(f.recipe, project.LoadOptions{SkipRequiresCheck: true})
	require.NoError(t, err)
	return proj
}

// TestStalenessGate_ReApprovalClearsIt is the same rule on the governance axis:
// the gate compares the unit's governing basis, so an approval made under the
// context in force answers for the unit and a rejection reads through to the
// stamp the run left.
func TestStalenessGate_ReApprovalClearsIt(t *testing.T) {
	t.Run("an approval under the context in force clears the gate", func(t *testing.T) {
		f := newStalenessFixture(t)
		f.record(t, "a-context-that-no-longer-governs")
		g, _ := f.run(t)
		require.False(t, g.Pass, "the fixture starts behind the context")

		_, err := f.app.ApplyReviewDecision(context.Background(),
			filepath.Join(f.root, "kapi.yaml"), "en",
			ReviewUnitRef{File: "locales/fr/app.json", Key: "greeting", Locale: "fr"},
			ReviewDecisionApproved, "")
		require.NoError(t, err)

		g, judged := f.run(t)
		require.True(t, judged)
		assert.True(t, g.Pass, "a reviewer approved the answer under the context in force")
		assert.Empty(t, g.Findings)
	})

	t.Run("a rejection leaves the gate where it was", func(t *testing.T) {
		f := newStalenessFixture(t)
		f.record(t, "a-context-that-no-longer-governs")

		_, err := f.app.ApplyReviewDecision(context.Background(),
			filepath.Join(f.root, "kapi.yaml"), "en",
			ReviewUnitRef{File: "locales/fr/app.json", Key: "greeting", Locale: "fr"},
			ReviewDecisionRejected, "")
		require.NoError(t, err)

		g, judged := f.run(t)
		require.True(t, judged)
		assert.False(t, g.Pass, "a rejection vouches for nothing")
		require.NotEmpty(t, g.Findings)
		assert.Contains(t, g.Findings[0].Message, "superseded context")
	})

	t.Run("a sign-off answers for the unit as an approval does", func(t *testing.T) {
		f := newStalenessFixture(t)
		f.record(t, "a-context-that-no-longer-governs")

		_, err := f.app.ApproveReviewUnit(context.Background(),
			filepath.Join(f.root, "kapi.yaml"), "en", "fr", "locales/fr/app.json", "greeting",
			string(model.TargetStatusSignedOff))
		require.NoError(t, err)

		g, _ := f.run(t)
		assert.True(t, g.Pass)
	})
}

// TestGoverningBasis_OnlyAnApprovalVouches pins the record-level rule the gate
// reads, over the four kinds of record a project holds.
func TestGoverningBasis_OnlyAnApprovalVouches(t *testing.T) {
	origin := model.Origin{Kind: model.OriginAI, ContextFingerprint: "fp-produced"}
	tests := []struct {
		name string
		unit state.UnitState
		want string
	}{
		{
			name: "a loop-written basis reads the producer's stamp",
			unit: state.UnitState{Status: model.TargetStatusTranslated, Origin: origin,
				GoverningFingerprint: "fp-produced"},
			want: "fp-produced",
		},
		{
			name: "an approval vouches for the context it was made under",
			unit: state.UnitState{Status: model.TargetStatusReviewed, Origin: origin,
				GoverningFingerprint: "fp-approved", Decision: state.Decision{ReviewState: "approved"}},
			want: "fp-approved",
		},
		{
			name: "a sign-off vouches the same way",
			unit: state.UnitState{Status: model.TargetStatusSignedOff, Origin: origin,
				GoverningFingerprint: "fp-approved", Decision: state.Decision{ReviewState: "signed-off"}},
			want: "fp-approved",
		},
		{
			name: "a rejection reads through to the producer's stamp",
			unit: state.UnitState{Status: model.TargetStatusDraft, Origin: origin,
				GoverningFingerprint: "fp-approved", Decision: state.Decision{ReviewState: "rejected"}},
			want: "fp-produced",
		},
		{
			name: "an ungoverned approval says nothing",
			unit: state.UnitState{Status: model.TargetStatusReviewed,
				Decision: state.Decision{ReviewState: "approved"}},
			want: "",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, tc.unit.GoverningBasis())
		})
	}
}
