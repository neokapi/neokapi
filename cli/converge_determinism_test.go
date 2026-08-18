package cli

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// One convergence, one outcome.
//
// The loop's premise is that a run is reproducible: the same recipe over the
// same committed inputs settles the same way, which is what makes a ship gate
// meaningful and a nightly's refusal informative. A locale that ships when the
// run is driven one way and parks when it is driven another makes both gates
// noisy and makes a sample's documented journey depend on how the reader
// invokes it (#2064).
//
// So the same convergence is driven here every way `kapi up` can be driven —
// the live renderer, the NDJSON stream agents and CI read, and the summary-only
// quiet run — each over its own pristine copy of samples/compass, and every run
// is held to ONE standing.

// convergeStanding is what a locale stands at after a run, read back off the
// tree the run leaves behind by `kapi status` — the one surface that is
// available whatever the run printed.
type convergeStanding struct {
	Locales []struct {
		Locale    string         `json:"locale"`
		Shippable bool           `json:"shippable"`
		Pct       map[string]int `json:"pct"`
	} `json:"locales"`
}

// fingerprintStanding renders a standing as one comparable line.
func fingerprintStanding(t *testing.T, statusJSON string) string {
	t.Helper()
	var st convergeStanding
	require.NoError(t, json.Unmarshal([]byte(statusJSON), &st), "status must emit JSON: %s", statusJSON)
	require.NotEmpty(t, st.Locales, "status must report the project's locales: %s", statusJSON)
	byLocale := map[string]string{}
	var names []string
	for _, lc := range st.Locales {
		names = append(names, lc.Locale)
		byLocale[lc.Locale] = fmt.Sprintf("%s translated=%d reviewed=%d ship=%v",
			lc.Locale, lc.Pct["translated"], lc.Pct["reviewed"], lc.Shippable)
	}
	// Coverage rows arrive in the derivation's order, which is not the reader's
	// concern; the standing is.
	slices.Sort(names)
	parts := make([]string, 0, len(names))
	for _, n := range names {
		parts = append(parts, byLocale[n])
	}
	return strings.Join(parts, " | ")
}

// TestUp_OneConvergenceOneOutcome drives the same convergence every way the
// verb can be driven, repeatedly, and holds every run to one standing.
//
// The mode that used to disagree is the one nobody watches: under --json (and
// under --quiet) the recipe's `source_language` never reached the loop, because
// --source-lang is registered with a default of "en" straight into the field the
// loop reads (host.AddProcessingFlags), so the `a.SourceLang == ""` fallback in
// RunDefaultFlowConverge could not fire and only the live-renderer branch
// applied the recipe. Converging an `en-GB` project at "en" misses every
// content-memory lookup: recycle fills nothing, the AI step drafts every unit
// in every locale over the project's approved wording, and no locale clears its
// ship gate. Repeats catch a scheduling-dependent outcome too — the locales of a
// pass converge concurrently — which is the other way one run could settle
// differently from another.
func TestUp_OneConvergenceOneOutcome(t *testing.T) {
	modes := []struct {
		name  string
		flags []string
		quiet bool
	}{
		{name: "live", flags: nil},
		{name: "json", flags: []string{"--json"}},
		{name: "quiet", quiet: true},
	}
	const repeats = 3

	standings := map[string]string{}
	for _, mode := range modes {
		for i := range repeats {
			a := processOnlyApp(t)
			a.Quiet = mode.quiet
			recipe, _ := compassCopy(t)

			out, err := runUp(t, a, recipe, mode.flags...)
			require.NoError(t, err, out)

			statusOut, serr := runCLI(t, NewStatusCmd(processOnlyApp(t)), "--project", recipe, "--json")
			require.NoError(t, serr, statusOut)
			fp := fingerprintStanding(t, statusOut)
			standings[fmt.Sprintf("%s#%d", mode.name, i)] = fp
		}
	}

	var first, firstKey string
	for k, v := range standings {
		if first == "" || k < firstKey {
			first, firstKey = v, k
		}
	}
	for k, v := range standings {
		assert.Equal(t, first, v,
			"one convergence, one outcome: %s settled differently from %s", k, firstKey)
	}

	// And the outcome itself is the one the sample documents: the languages the
	// committed content memory answers converge on recycled wording rather than
	// being re-drafted over it.
	assert.Contains(t, first, "nb translated=100 reviewed=89 ship=true",
		"the committed Norwegian wording is recycled, not re-drafted: %s", first)
}
