package brand

import "fmt"

// EvalBlock is the minimal content unit the blast-radius evaluator scores: a
// block's identity, the collection it belongs to, and its text. Callers load
// these from their content store; core/brand stays free of any store dependency.
type EvalBlock struct {
	BlockID        string
	CollectionID   string
	CollectionName string
	Text           string
}

// EvaluateBlastRadius reports the impact of moving the vocabulary checks from the
// baseline profile to the candidate profile across a set of blocks — the number
// shown before a rule is promoted so a team sees what a change will do before it
// lands. For each block it runs both profiles' vocabulary matchers and diffs the
// results:
//
//   - NewViolations    — matches the candidate raises that the baseline did not
//     (the content a newly-promoted rule would start flagging).
//   - ResolvedViolations — matches the baseline raised that the candidate does not.
//   - AffectedBlocks   — blocks whose match set changed at all.
//   - Improved/Degraded — blocks whose compliance score rose / fell.
//   - CriticalCount    — new violations at critical severity (the riskiest).
//
// Results are broken down per collection. Only the vocabulary checkset — the part
// a promoted correction-rule changes — is scored here; subjective and ML-backed
// checks are out of scope for a deterministic blast-radius preview.
func EvaluateBlastRadius(blocks []EvalBlock, baseline, candidate *VoiceProfile) BlastRadius {
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
		baseHits := MatchVocabulary(baseline, b.Text)
		candHits := MatchVocabulary(candidate, b.Text)
		baseKeys := hitKeySet(baseHits)
		candKeys := hitKeySet(candHits)

		newV, resolvedV, newCrit := 0, 0, 0
		for k, sev := range candKeys {
			if _, ok := baseKeys[k]; !ok {
				newV++
				if sev == SeverityCritical {
					newCrit++
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
		if delta > 0 {
			br.ImprovedBlocks++
		} else if delta < 0 {
			br.DegradedBlocks++
		}
		br.NewViolations += newV
		br.ResolvedViolations += resolvedV
		br.CriticalCount += newCrit

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

// CandidateWithRule returns a copy of baseline with the suggested rule applied —
// the candidate profile to evaluate a promotion against, without mutating the
// baseline.
func CandidateWithRule(baseline *VoiceProfile, r SuggestedRule) *VoiceProfile {
	c := baseline.Clone()
	ApplySuggestedRule(c, r)
	return c
}

// hitKeySet keys each hit by category and byte range so the same violation in the
// same text is comparable between the baseline and candidate runs (both score the
// identical text, so positions align).
func hitKeySet(hits []VocabHit) map[string]Severity {
	m := make(map[string]Severity, len(hits))
	for _, h := range hits {
		m[fmt.Sprintf("%s|%d|%d", h.Category, h.Start, h.End)] = h.Severity
	}
	return m
}

// findingsFromHits projects hits onto the minimal findings the score needs
// (category + severity drive the penalty weights).
func findingsFromHits(hits []VocabHit) []BrandVoiceFinding {
	fs := make([]BrandVoiceFinding, len(hits))
	for i, h := range hits {
		fs[i] = BrandVoiceFinding{Category: string(h.Category), Severity: h.Severity}
	}
	return fs
}
