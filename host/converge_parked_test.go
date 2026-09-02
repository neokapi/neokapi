package host

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/gate"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
)

// #1936. `materialize: on-converge` promises that a locale which did not clear
// its ship gate does not get its files written. The convergence pass wrote them
// itself, through the collection's own `target:` binding, before the policy was
// ever consulted — so the gate held back a second, redundant write and every
// consumer that reads target files (a site build, a bundler, a publishing
// connector) still found the unreviewed draft exactly where it looks.
//
// These tests assert the observable end state: after the run, does the parked
// locale have files on disk.

// parkedProject is a project whose ship gate needs human review, so no
// unattended run can clear it, over one source whose every string the seeded
// content memory answers — a locale that is fully drafted and still parked,
// which is the case the gate exists for.
func parkedProject(t *testing.T, shipGate gate.Gate) (*App, *EnvCommand, string, string) {
	t.Helper()
	a, cmd, recipe, dir := recipeFormatProject(t,
		map[string]string{"src/app.json": `{"title":"Tide window","subtitle":"When the forecast allows this movement."}`},
		[]project.Collection{{
			Name: "app",
			Content: []project.ContentItem{{
				Path:   "src/app.json",
				Format: &project.FormatSpec{Name: "json"},
				Target: "site/locales/{lang}.json",
			}},
		}}, nil)
	proj, err := project.Load(recipe)
	require.NoError(t, err)
	proj.ShipGate = shipGate
	require.NoError(t, project.Save(recipe, proj))
	seedMemory(t, a, recipe, map[string]string{
		"Tide window": "Tidevannsvindu",
		"When the forecast allows this movement.": "Når varselet tillater denne bevegelsen.",
	})
	return a, cmd, recipe, dir
}

// TestConverge_ParkedLocaleReachesNoDisk: a locale that cannot clear its ship
// gate ends the run with no target file at all. The draft exists — it is in the
// project block store, and `kapi status` reports the coverage — but nothing a
// build globs can see it.
func TestConverge_ParkedLocaleReachesNoDisk(t *testing.T) {
	// A reviewed bar an unattended run cannot reach: nothing in this run
	// records a human decision, so nb stays parked however well it drafts.
	a, cmd, recipe, dir := parkedProject(t, gate.Gate{
		"translated": {Pct: 100},
		"reviewed":   {Pct: 50},
	})

	out := converge(t, a, cmd, recipe)
	require.False(t, out.Converged, "an unreviewable gate must leave the locale parked")
	require.Len(t, out.Locales, 1)
	require.False(t, out.Locales[0].Shippable, "the gate must report the locale as withheld")

	_, err := os.Stat(filepath.Join(dir, "site", "locales", "nb.json"))
	assert.True(t, os.IsNotExist(err),
		"the gate says withheld, so the unreviewed draft must not be on disk where a build reads it")

	// And the draft tree the pass used is gone with the run: derived state, not
	// a second delivery surface.
	entries, _ := os.ReadDir(filepath.Join(dir, ".kapi", "work", "drafts"))
	assert.Empty(t, entries, "the run's draft tree must not outlive the run")
}

// TestConverge_ShippableLocaleIsDelivered is the discriminating control: the
// same project with a gate the run can clear delivers its files. Without it,
// "withhold everything" would pass the test above.
func TestConverge_ShippableLocaleIsDelivered(t *testing.T) {
	a, cmd, recipe, dir := parkedProject(t, gate.Gate{"translated": {Pct: 100}})

	out := converge(t, a, cmd, recipe)
	require.True(t, out.Converged)
	require.Len(t, out.Locales, 1)
	require.True(t, out.Locales[0].Shippable)

	body, err := os.ReadFile(filepath.Join(dir, "site", "locales", "nb.json"))
	require.NoError(t, err, "a locale that cleared its gate must be delivered")
	assert.Contains(t, string(body), "Tidevannsvindu")
	assert.Contains(t, string(body), "Når varselet tillater denne bevegelsen.")
}

// TestConverge_ParkedLocaleUnderManualPolicyKeepsWriting is the boundary. Only
// a policy that claims a gate gets one: `materialize: manual` promises the
// opposite — delivery is a separate `kapi merge` — and claims nothing about
// withholding, so its passes keep writing where the recipe points. Narrowing
// this would change what a default `kapi up` produces, which is a different
// decision from making `on-converge` mean what it says.
func TestConverge_ParkedLocaleUnderManualPolicyKeepsWriting(t *testing.T) {
	a, cmd, recipe, dir := parkedProject(t, gate.Gate{
		"translated": {Pct: 100},
		"reviewed":   {Pct: 50},
	})
	proj, err := project.Load(recipe)
	require.NoError(t, err)
	proj.Defaults.Materialize = project.MaterializeManual
	require.NoError(t, project.Save(recipe, proj))

	out := converge(t, a, cmd, recipe)
	require.False(t, out.Converged)
	assert.Zero(t, out.MaterializedFiles, "manual never materializes")

	body, rerr := os.ReadFile(filepath.Join(dir, "site", "locales", "nb.json"))
	require.NoError(t, rerr, "the pass writes where the recipe points under manual")
	assert.Contains(t, string(body), "Tidevannsvindu")
}

// TestConverge_ParkedLocaleReadsTheSameToUpAndStatus: the run's closing report
// and `kapi status` describe the parked locale identically (#2024).
//
// The run's draft tree goes with the run, so a figure derived from it would
// describe a tree nobody can open. Both readings come off the record that
// outlives the run instead: the project block store, which holds the drafts the
// gate withheld (#2356). The locale reads fully translated and short of its
// `reviewed` bar, from `up` and from `status` alike.
func TestConverge_ParkedLocaleReadsTheSameToUpAndStatus(t *testing.T) {
	a, cmd, recipe, dir := parkedProject(t, gate.Gate{
		"translated": {Pct: 100},
		"reviewed":   {Pct: 50},
	})

	out := converge(t, a, cmd, recipe)
	require.Len(t, out.Locales, 1)
	require.False(t, out.Locales[0].Shippable, "the gate withholds the locale")

	_, err := os.Stat(filepath.Join(dir, "site", "locales", "nb.json"))
	require.True(t, os.IsNotExist(err), "and nothing was delivered")
	assert.Equal(t, 100, out.Locales[0].Pct["translated"],
		"the store holds a translation for every unit the pass drafted")
	assert.Zero(t, out.Locales[0].Pct["reviewed"], "nobody reviewed them")

	// The same derivation `kapi status` runs, over the same tree, reaches the
	// same figure, which is the whole property.
	proj, lerr := project.Load(recipe)
	require.NoError(t, lerr)
	units, uerr := a.UnitsFromProject(proj, dir, "")
	require.NoError(t, uerr)
	cov, cerr := a.ComputeShipCoverage(cmd.Context(), proj, dir, units, nil)
	require.NoError(t, cerr)
	require.Len(t, cov, 1)
	assert.Equal(t, cov[0].Pct["translated"], out.Locales[0].Pct["translated"],
		"up and status must publish one `translated` figure for one locale")
	assert.Equal(t, cov[0].Shippable, out.Locales[0].Shippable)
}

// TestConverge_ParkedDraftSurvivesInTheStore: withheld is not lost. The locale's
// work is in the project block store, and materializing it — the deliberate act
// `--materialize` or `kapi merge` is — puts it on disk.
func TestConverge_ParkedDraftSurvivesInTheStore(t *testing.T) {
	a, cmd, recipe, dir := parkedProject(t, gate.Gate{
		"translated": {Pct: 100},
		"reviewed":   {Pct: 50},
	})

	out := converge(t, a, cmd, recipe)
	require.False(t, out.Converged)
	target := filepath.Join(dir, "site", "locales", "nb.json")
	_, err := os.Stat(target)
	require.True(t, os.IsNotExist(err))

	proj, err := project.Load(recipe)
	require.NoError(t, err)
	written, merr := a.materializeFromProjectStore(cmd.Context(), os.Stderr, proj, recipe,
		[]model.LocaleID{"nb"}, false)
	require.NoError(t, merr)
	assert.Equal(t, 1, written, "the withheld draft is in the store, not discarded")
	body, rerr := os.ReadFile(target)
	require.NoError(t, rerr)
	assert.Contains(t, string(body), "Tidevannsvindu")
}
