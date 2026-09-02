// Package insights turns what the context graph knows into signals a person
// can act on.
//
// The first of those is the context funnel: how much content the graph is
// actually driving, as opposed to merely sitting alongside. A graph can look
// healthy — profiles defined, terms curated, rules promoted — while almost
// nothing that ships was written under it. Coverage numbers hide that; a funnel
// cannot, because the interesting figure is the drop between stages rather than
// any stage on its own.
package insights

import "fmt"

// Stage names one step of the context funnel. The order is the order content
// moves through, and each stage is a strict subset of the one above it.
type Stage string

const (
	// StageIndexed is every translatable block the platform holds. The
	// denominator: content it knows about at all.
	StageIndexed Stage = "indexed"
	// StageGoverned is content that resolves to a profile — something in the
	// workspace → project → stream → collection ladder binds one, so there IS
	// an answer to "what applies here".
	StageGoverned Stage = "governed"
	// StageChecked is content that has actually been scored against that
	// profile. Governed but unchecked means the rules exist and nothing has
	// tested the content against them.
	StageChecked Stage = "checked"
	// StageProducedUnderContext is content whose *targets* were produced with
	// the profile in force — Origin.Profile is stamped, so the guidance
	// demonstrably reached the producer rather than being available in
	// principle.
	StageProducedUnderContext Stage = "produced_under_context"
)

// Counts are the raw inputs, gathered per stage. They are counts of the SAME
// unit at the first three stages (blocks) and of targets at the last, which is
// why the funnel reports the last stage's basis separately rather than
// pretending it is the same population.
type Counts struct {
	Indexed  int
	Governed int
	Checked  int
	// ProducedUnderContext counts TARGETS, not blocks: a block can have several
	// targets, each produced under a different profile version. Comparing it to
	// a block count would be a category error, so the funnel expresses it
	// against TargetsTotal instead.
	ProducedUnderContext int
	TargetsTotal         int
}

// Step is one stage with its share and the drop from the stage above.
type Step struct {
	Stage Stage `json:"stage"`
	Count int   `json:"count"`
	// ShareOfTotal is this stage as a fraction of the funnel's denominator,
	// rounded to whole percent. 0 when the denominator is 0.
	ShareOfTotal int `json:"share_of_total"`
	// DroppedFromPrevious is how much content did NOT make it from the stage
	// above. This is the number worth reading: it localises where the graph
	// stops having an effect.
	DroppedFromPrevious int `json:"dropped_from_previous"`
	// Basis names the denominator, because the last stage counts targets while
	// the first three count blocks. Without it a reader would compare two
	// populations and reach a confident wrong conclusion.
	Basis string `json:"basis"`
}

// Funnel is the computed answer.
type Funnel struct {
	Steps []Step `json:"steps"`
	// Headline is the share of targets produced with a profile in force — the
	// single number that answers "is the graph doing anything", kept alongside
	// the funnel rather than instead of it.
	Headline int `json:"headline"`
	// Narrative names the largest drop in plain words, so the number arrives
	// with its interpretation attached instead of waiting for someone to work
	// out which gap matters.
	Narrative string `json:"narrative"`
}

// Compute builds the funnel from raw counts.
//
// It deliberately does not error on inconsistent input (governed > indexed, and
// so on). Those would mean the queries disagree, which is a bug worth seeing in
// the output rather than a reason to return nothing: a funnel that refuses to
// render tells an operator less than one showing an impossible shape.
func Compute(c Counts) Funnel {
	blocks := "blocks"
	targets := "targets"

	steps := []Step{
		{Stage: StageIndexed, Count: c.Indexed, Basis: blocks},
		{Stage: StageGoverned, Count: c.Governed, Basis: blocks},
		{Stage: StageChecked, Count: c.Checked, Basis: blocks},
		{Stage: StageProducedUnderContext, Count: c.ProducedUnderContext, Basis: targets},
	}

	for i := range steps {
		denom := c.Indexed
		if steps[i].Basis == targets {
			denom = c.TargetsTotal
		}
		steps[i].ShareOfTotal = percent(steps[i].Count, denom)
		if i > 0 && steps[i-1].Basis == steps[i].Basis {
			steps[i].DroppedFromPrevious = max(0, steps[i-1].Count-steps[i].Count)
		}
	}

	return Funnel{
		Steps:     steps,
		Headline:  percent(c.ProducedUnderContext, c.TargetsTotal),
		Narrative: narrate(c),
	}
}

// percent rounds to the nearest whole percent, and treats an empty denominator
// as 0 rather than dividing — an empty project reports 0%, not a crash and not
// a misleading 100%.
func percent(n, total int) int {
	if total <= 0 {
		return 0
	}
	return (n*200/total + 1) / 2
}

// narrate names the biggest gap. Each phrasing says what the gap MEANS, because
// "35% checked" prompts the wrong follow-up question ("get it to 100") while
// "the rules exist but nothing has tested content against them" prompts the
// right one.
func narrate(c Counts) string {
	if c.Indexed == 0 {
		return "No content indexed yet, so there is nothing to govern."
	}

	ungoverned := c.Indexed - c.Governed
	unchecked := c.Governed - c.Checked
	unstamped := c.TargetsTotal - c.ProducedUnderContext

	switch {
	case ungoverned >= unchecked && ungoverned >= unstamped && ungoverned > 0:
		return fmt.Sprintf(
			"%d of %d blocks resolve to no profile: for that content there is no answer to "+
				"\"what applies here\". Binding a profile further up the ladder is the cheapest fix.",
			ungoverned, c.Indexed)
	case unchecked >= unstamped && unchecked > 0:
		return fmt.Sprintf(
			"%d governed blocks have never been scored: the rules exist and nothing has tested "+
				"the content against them.", unchecked)
	case unstamped > 0:
		return fmt.Sprintf(
			"%d targets carry no profile stamp: they were produced without the context reaching "+
				"the producer, so the graph did not shape what shipped.", unstamped)
	default:
		return "Every indexed block is governed, checked, and producing under a profile."
	}
}
