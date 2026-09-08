package host

import (
	"bytes"
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

// What the local venue owes a unit a reviewer turned down.
//
// A rejection on a source nobody has rewritten leaves both hashes where the
// record already had them, so the basis grading reads the unit as settled. It
// is not: a person said the wording will not do, and the loop owes a draft
// until something has replaced it (#2564). The count is reported apart from the
// stale one, it withholds the scope, and it clears itself the moment a pass
// writes different wording.

// rejectionFixture is the staleness project with a reviewer's rejection
// recorded against the source and translation both still on disk.
type rejectionFixture struct {
	app    *App
	root   string
	recipe string
}

func newRejectionFixture(t *testing.T) *rejectionFixture {
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
		Status:      model.TargetStatusDraft,
		TargetHash:  state.TargetHash("Bonjour"),
		ContentHash: state.SourceHash("Hello there"),
		Decision:    state.Decision{ReviewState: "rejected", At: "2026-09-01T00:00:00Z"},
	}))
	return &rejectionFixture{app: a, root: root, recipe: filepath.Join(root, "kapi.yaml")}
}

// coverage rolls the project up the way `kapi status` does, with no ship gate,
// so the verdict under test is the one the rollup withholds on its own.
func (f *rejectionFixture) coverage(t *testing.T) []LocaleCoverage {
	t.Helper()
	ctx := context.Background()
	proj, err := project.LoadWithOptions(f.recipe, project.LoadOptions{SkipRequiresCheck: true})
	require.NoError(t, err)
	units, err := f.app.UnitsFromProject(proj, f.root, "")
	require.NoError(t, err)
	tally, err := f.app.ProjectCoverageTally(ctx, proj, f.root, units, nil)
	require.NoError(t, err)
	return tally.Rollup(gate.RuleSet{})
}

// redraft writes different wording into the target file, which is what a
// convergence pass leaves behind and what moves the unit off the pairing the
// reviewer refused.
func (f *rejectionFixture) redraft(t *testing.T, text string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(f.root, "locales", "fr", "app.json"),
		[]byte("{\n  \"greeting\": \""+text+"\"\n}\n"), 0o644))
	f.app = &App{}
	f.app.InitRegistries()
}

// TestCoverage_RejectedUnitOwesADraft: the refused unit is counted as owed a
// draft, is not counted as stale, and holds the scope out of shipping with no
// gate declared. A pass that writes different wording settles it.
func TestCoverage_RejectedUnitOwesADraft(t *testing.T) {
	f := newRejectionFixture(t)

	rows := f.coverage(t)
	require.Len(t, rows, 1)
	assert.Equal(t, 1, rows[0].RejectedAwaitingDraft,
		"a reviewer refused the wording and nothing has replaced it")
	assert.Zero(t, rows[0].Stale, "nothing rewrote the source, so nothing is stale")
	assert.Zero(t, rows[0].StaleAwaitingDraft)
	assert.Zero(t, rows[0].StaleAwaitingReview)
	assert.False(t, rows[0].Shippable, "an ungated scope has nothing else to catch it")
	assert.False(t, rows[0].Verified)
	assert.Equal(t, 100, rows[0].Pct["draft"], "the unit holds a target, so it is at the draft rung")

	f.redraft(t, "Salut")
	rows = f.coverage(t)
	require.Len(t, rows, 1)
	assert.Zero(t, rows[0].RejectedAwaitingDraft,
		"the loop drafted something else, so the rejection no longer describes what is on disk")
	assert.True(t, rows[0].Shippable)
}

// TestCoverage_RejectedUnitUnderAMovedSourceIsCountedOnce: a unit that is both
// stale and rejected belongs to the stale count and to nothing else, so the two
// counts stay disjoint and a caller subtracting both subtracts each unit once.
func TestCoverage_RejectedUnitUnderAMovedSourceIsCountedOnce(t *testing.T) {
	f := newRejectionFixture(t)
	require.NoError(t, os.WriteFile(filepath.Join(f.root, "locales", "en", "app.json"),
		[]byte("{\n  \"greeting\": \"Hi there\"\n}\n"), 0o644))
	f.app = &App{}
	f.app.InitRegistries()

	rows := f.coverage(t)
	require.Len(t, rows, 1)
	assert.Equal(t, 1, rows[0].Stale)
	assert.Equal(t, 1, rows[0].StaleAwaitingDraft, "the loop owes it a translation of the new wording")
	assert.Zero(t, rows[0].RejectedAwaitingDraft, "and it is not counted a second time")
}

// TestStatusOutput_NamesTheRefusedUnits: the status report says what a refused
// unit is waiting for, and the ship cell says why the scope is held. Both are
// silent when nothing was refused.
func TestStatusOutput_NamesTheRefusedUnits(t *testing.T) {
	out := StatusOutput{Locales: []LocaleCoverage{{
		Locale: "fr", Total: 10, Pct: map[string]int{"draft": 100},
		RejectedAwaitingDraft: 3,
	}}}
	var buf bytes.Buffer
	require.NoError(t, out.FormatText(&buf))
	assert.Contains(t, buf.String(), "3 unit(s) were turned down in review")
	assert.Contains(t, buf.String(), "blocked: rejected")

	clean := StatusOutput{Locales: []LocaleCoverage{{
		Locale: "fr", Total: 10, Pct: map[string]int{"draft": 100},
	}}}
	buf.Reset()
	require.NoError(t, clean.FormatText(&buf))
	assert.NotContains(t, buf.String(), "turned down")
}

// TestConvergeOutput_NamesTheRefusedUnits: a run that ends with wording a
// reviewer refused still in the file says so, rather than reporting the locale
// at 100% draft and leaving the reader to wonder why it does not ship.
func TestConvergeOutput_NamesTheRefusedUnits(t *testing.T) {
	out := ConvergeOutput{
		Flow: "translate", Passes: 1,
		Locales: []ConvergeLocaleResult{{
			Locale: "fr", Pct: map[string]int{"draft": 100}, Rejected: 2,
		}},
	}
	var buf bytes.Buffer
	require.NoError(t, out.FormatText(&buf))
	assert.Contains(t, buf.String(), "2 unit(s) still hold wording a reviewer turned down")
	assert.Equal(t, 2, out.RejectedUnits())
}
