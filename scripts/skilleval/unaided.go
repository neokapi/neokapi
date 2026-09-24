package main

import (
	"os"
	"path/filepath"
	"strings"
)

// The unaided control receives the same prompt and workspace without kapi.
// Paired outcomes measure the contribution of kapi in each scenario:
//
//   - ENABLED: the gate passes with kapi and fails without it.
//   - EASED: both pass, but the unaided arm takes more work.
//   - NEITHER: outcomes and effort are similar.
//   - HINDERED: the unaided arm passes and the kapi arm fails.
//
// These are observations of the evaluated runs, not proof that a task requires
// kapi. AllContributions supplies the shared list for summaries and dashboards.

// armSkill and armUnaided name the two conditions.
const (
	armSkill   = "skill"
	armUnaided = "unaided"
)

// stripKapiFromPath removes directories containing kapi or toolbox binaries.
// This prevents the control arm from using a developer-installed binary even
// when the kapi skill is absent.
func stripKapiFromPath(path string) string {
	var kept []string
	for _, dir := range filepath.SplitList(path) {
		if dir == "" || holdsKapiBinary(dir) {
			continue
		}
		kept = append(kept, dir)
	}
	return strings.Join(kept, string(os.PathListSeparator))
}

// kapiBinaries are the names whose presence disqualifies a PATH entry from the
// unaided arm.
var kapiBinaries = []string{"kapi", "kgrep", "ksed", "kcat", "kdiff", "kconv"}

func holdsKapiBinary(dir string) bool {
	for _, name := range kapiBinaries {
		if info, err := os.Stat(filepath.Join(dir, name)); err == nil && !info.IsDir() {
			return true
		}
	}
	return false
}

// Contribution is what the skill added on one scenario, measured rather than
// asserted.
type Contribution string

const (
	// ContributionEnabled: the gate is green with kapi and red without.
	ContributionEnabled Contribution = "enabled"
	// ContributionEased: green both ways, and the unaided arm did visibly more.
	ContributionEased Contribution = "eased"
	// ContributionNeither: same outcome, similar effort. Not evidence for kapi.
	ContributionNeither Contribution = "neither"
	// ContributionUnknown: no gate, so there is no outcome to compare. The
	// effort difference is still reported, but it settles nothing.
	ContributionUnknown Contribution = "unknown"
	// ContributionHindered: the unaided agent finished and the one with kapi did
	// not. It is the outcome an eval exists to surface, so it gets its own name
	// rather than being averaged into the ones that flatter.
	ContributionHindered Contribution = "hindered"
)

// AllContributions is every outcome, in the order a reader should take them:
// strongest claim first, and the two that are not claims at the end.
var AllContributions = []Contribution{
	ContributionEnabled, ContributionEased,
	ContributionHindered, ContributionNeither, ContributionUnknown,
}

// easedFactor is how much more work the unaided arm must do before the
// difference is called out rather than treated as noise.
//
// Agent runs vary run to run, so a 10% gap says nothing. Half again as many
// messages is a difference a reader would notice in their own session.
const easedFactor = 1.5

// contribution compares the two arms.
//
// Deliberately conservative: with no gate it returns unknown rather than
// guessing from effort alone, and a small effort gap is "neither" rather than
// a win. An eval that rounds ambiguity toward its own product is not evidence.
func contribution(withKapi, unaided []Run, gated bool) Contribution {
	if len(withKapi) == 0 || len(unaided) == 0 {
		return ContributionUnknown
	}
	if gated {
		w, u := gatesGreen(withKapi), gatesGreen(unaided)
		switch {
		case w > 0 && u == 0:
			return ContributionEnabled
		case w == 0 && u > 0:
			// The unaided agent finished and the one with kapi did not. Folding
			// this into "neither" was the first version, and it buried the one
			// outcome nobody would want buried: three scenarios landed here on
			// the first gated sweep, and the summary line called them the same
			// thing as a scenario where both arms sailed through.
			return ContributionHindered
		case w == 0:
			// Neither finished. Whatever this scenario shows, it is not that
			// kapi enabled the task.
			return ContributionNeither
		}
		// Green both ways: fall through to the effort comparison.
	}

	wm, um := medianMessages(withKapi), medianMessages(unaided)
	if wm == 0 || um == 0 {
		return ContributionUnknown
	}
	if float64(um) >= float64(wm)*easedFactor {
		return ContributionEased
	}
	if !gated {
		return ContributionUnknown
	}
	return ContributionNeither
}

func gatesGreen(runs []Run) int {
	n := 0
	for _, r := range runs {
		if r.Gate != nil && r.Gate.ExitCode == 0 {
			n++
		}
	}
	return n
}

// medianMessages is the middle of the per-run message counts, so one runaway
// pass does not decide the comparison.
func medianMessages(runs []Run) int {
	var counts []int
	for _, r := range runs {
		if r.Messages > 0 {
			counts = append(counts, r.Messages)
		}
	}
	if len(counts) == 0 {
		return 0
	}
	for i := 1; i < len(counts); i++ {
		for j := i; j > 0 && counts[j] < counts[j-1]; j-- {
			counts[j], counts[j-1] = counts[j-1], counts[j]
		}
	}
	return counts[len(counts)/2]
}
