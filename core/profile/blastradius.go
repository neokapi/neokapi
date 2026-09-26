package profile

import (
	"fmt"
	"slices"
)

// EvalBlock is the minimal content unit the blast-radius evaluator scores: a
// block's identity, the collection it belongs to, and its text. Callers load
// these from their content store; core/profile stays free of any store dependency.
type EvalBlock struct {
	BlockID        string
	CollectionID   string
	CollectionName string
	Text           string
}

// EvaluateBlastRadius reports the impact of moving from the baseline word rules
// to the candidate word rules across a set of blocks: the number shown before a
// rule is promoted, so a team sees what a change will do before it lands. For
// each block it matches both rule sets and diffs the results:
//
//   - NewViolations    — matches the candidate raises that the baseline did not
//     (the content a newly-promoted rule would start flagging).
//   - ResolvedViolations — matches the baseline raised that the candidate does not.
//   - AffectedBlocks   — blocks whose match set changed at all.
//   - Improved/Degraded — blocks whose compliance score rose / fell.
//   - FailingCount     — new violations that fail a check (the riskiest).
//
// Results are broken down per collection. Only the word rules, the part a
// promoted correction changes, are scored here; subjective and model-backed
// checks are out of scope for a deterministic blast-radius preview.
func EvaluateBlastRadius(blocks []EvalBlock, baseline, candidate []TermRuleSet) BlastRadius {
	// Collections starts as a non-nil empty slice so it marshals to JSON `[]`,
	// never `null` — clients (the web blast-radius preview) index `.length`/`.map`
	// on it directly and a null crashes the render.
	br := BlastRadius{TotalBlocks: len(blocks), Collections: []CollectionBlastRadius{}}

	type colAcc struct {
		cbr      *CollectionBlastRadius
		sumDelta float64
		scored   int
	}
	cols := map[string]*colAcc{}
	var colOrder []string

	for _, b := range blocks {
		baseHits := MatchTermRules(baseline, b.Text)
		candHits := MatchTermRules(candidate, b.Text)
		baseKeys := hitKeySet(baseHits)
		candKeys := hitKeySet(candHits)

		newV, resolvedV, newFailing := 0, 0, 0
		prescribed := false
		for k, h := range candKeys {
			if _, ok := baseKeys[k]; !ok {
				newV++
				if h.fails {
					newFailing++
				}
				if h.replacement != "" {
					prescribed = true
				}
			}
		}
		for k := range baseKeys {
			if _, ok := candKeys[k]; !ok {
				resolvedV++
			}
		}

		baseScore := CalculateScore(findingsFromHits(baseHits)).Overall
		candScore := CalculateScore(findingsFromHits(candHits)).Overall
		delta := candScore - baseScore

		changed := newV > 0 || resolvedV > 0
		if changed {
			br.AffectedBlocks++
		}
		if prescribed {
			br.PrescribedBlocks++
		}
		if delta > 0 {
			br.ImprovedBlocks++
		} else if delta < 0 {
			br.DegradedBlocks++
		}
		br.NewViolations += newV
		br.ResolvedViolations += resolvedV
		br.FailingCount += newFailing

		if changed {
			acc := cols[b.CollectionID]
			if acc == nil {
				acc = &colAcc{cbr: &CollectionBlastRadius{
					CollectionID:   b.CollectionID,
					CollectionName: b.CollectionName,
				}}
				cols[b.CollectionID] = acc
				colOrder = append(colOrder, b.CollectionID)
			}
			acc.cbr.AffectedBlocks++
			acc.sumDelta += float64(delta)
			acc.scored++
		}
	}

	for _, id := range colOrder {
		acc := cols[id]
		if acc.scored > 0 {
			acc.cbr.AvgScoreDelta = acc.sumDelta / float64(acc.scored)
		}
		br.Collections = append(br.Collections, *acc.cbr)
	}
	return br
}

// CandidateWithRule returns baseline with the suggested rule applied to its
// rules: the candidate to evaluate a promotion against. The baseline is left
// as it was.
func CandidateWithRule(baseline []TermRuleSet, r SuggestedRule) []TermRuleSet {
	out := slices.Clone(baseline)
	for i, set := range out {
		if set.Kind == VocabForbidden && !set.Suggested {
			out[i].Rules, _ = ApplySuggestedRule(set.Rules, r)
			return out
		}
	}
	rules, _ := ApplySuggestedRule(nil, r)
	return append(out, TermRuleSet{Rules: rules, Kind: VocabForbidden})
}

// keyedHit carries the two properties a diff of two hit sets reads: whether the
// violation fails, and whether the rule that raised it says what to write
// instead.
type keyedHit struct {
	fails       bool
	replacement string
}

// hitKeySet keys each hit by category and byte range so the same violation in the
// same text is comparable between the baseline and candidate runs (both score the
// identical text, so positions align).
func hitKeySet(hits []VocabHit) map[string]keyedHit {
	m := make(map[string]keyedHit, len(hits))
	for _, h := range hits {
		m[fmt.Sprintf("%s|%d|%d", h.Category, h.Start, h.End)] = keyedHit{
			fails:       h.Fails,
			replacement: h.Replacement,
		}
	}
	return m
}

// findingsFromHits projects hits onto the minimal findings the score needs
// (category and whether it fails drive the penalty weights).
func findingsFromHits(hits []VocabHit) []VoiceFinding {
	fs := make([]VoiceFinding, len(hits))
	for i, h := range hits {
		fs[i] = VoiceFinding{Category: string(h.Category), Fails: h.Fails, Suggested: h.Suggested}
	}
	return fs
}
