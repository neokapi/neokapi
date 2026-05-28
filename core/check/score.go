package check

import "sort"

// CategoryScore is the per-category breakdown within a Score.
type CategoryScore struct {
	Category string `json:"category"`
	Score    int    `json:"score"` // 0-100
	Penalty  int    `json:"penalty"`
	Issues   int    `json:"issues"`
}

// Score is the aggregate result of running a checkset over a block (or corpus).
// The Findings are the substantive output; Overall is a convenience roll-up that
// is honest only when calibrated (see WithWordCount for length-normalization).
type Score struct {
	Overall    int             `json:"overall"` // 0-100
	Categories []CategoryScore `json:"categories"`
	Findings   []Finding       `json:"findings"`
	WordCount  int             `json:"word_count,omitempty"`
	Normalized bool            `json:"normalized,omitempty"`
}

// scoreOptions configures CalculateScore.
type scoreOptions struct {
	wordCount int
	normalize bool
}

// ScoreOption tunes score aggregation.
type ScoreOption func(*scoreOptions)

// WithWordCount records the block's word count on the Score and enables
// length-normalization: penalties are scaled to a per-100-word rate so a long
// paragraph with one nit does not score the same as a one-word string with the
// same nit. Without it, scoring is the raw 100 − Σpenalty roll-up.
func WithWordCount(words int) ScoreOption {
	return func(o *scoreOptions) {
		o.wordCount = words
		if words > 0 {
			o.normalize = true
		}
	}
}

// CalculateScore aggregates findings into a Score. By default Overall is
// 100 − Σ(severity penalties), clamped to [0,100], with a per-category
// breakdown. Pass WithWordCount to length-normalize the roll-up.
func CalculateScore(findings []Finding, opts ...ScoreOption) Score {
	var o scoreOptions
	for _, opt := range opts {
		opt(&o)
	}

	penalties := make(map[string]int)
	counts := make(map[string]int)
	order := make([]string, 0)
	total := 0
	for _, f := range findings {
		if _, seen := penalties[f.Category]; !seen {
			order = append(order, f.Category)
		}
		w := SeverityWeight(f.Severity)
		penalties[f.Category] += w
		counts[f.Category]++
		total += w
	}
	sort.Strings(order)

	scale := func(penalty int) int {
		if o.normalize && o.wordCount > 0 {
			// Penalty per 100 words: a nit in a 5-word string bites harder than
			// the same nit in a 200-word paragraph.
			norm := float64(penalty) * 100.0 / float64(o.wordCount)
			s := 100 - int(norm+0.5)
			return clamp(s)
		}
		return clamp(100 - penalty)
	}

	categories := make([]CategoryScore, 0, len(order))
	for _, cat := range order {
		categories = append(categories, CategoryScore{
			Category: cat,
			Score:    scale(penalties[cat]),
			Penalty:  penalties[cat],
			Issues:   counts[cat],
		})
	}

	return Score{
		Overall:    scale(total),
		Categories: categories,
		Findings:   findings,
		WordCount:  o.wordCount,
		Normalized: o.normalize,
	}
}

func clamp(s int) int {
	if s < 0 {
		return 0
	}
	if s > 100 {
		return 100
	}
	return s
}
