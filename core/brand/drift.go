package brand

import (
	"math"
	"sort"
)

// DriftConfig configures brand-compliance drift detection. A workspace sets a
// floor, a drop tolerance, or both; either condition firing counts as drift.
type DriftConfig struct {
	RecentDays int // size of the "recent" window in days (<=0 → 7)
	MinScore   int // absolute floor: a recent average below this is drift (0 → disabled)
	DropPoints int // relative: a recent average this many points below the baseline is drift (0 → disabled)
}

func (c DriftConfig) withDefaults() DriftConfig {
	if c.RecentDays <= 0 {
		c.RecentDays = 7
	}
	if c.MinScore == 0 && c.DropPoints == 0 {
		c.DropPoints = 10 // a 10-point decline is the default drift signal
	}
	return c
}

// DriftResult is the outcome of a drift analysis over a project's score trend.
type DriftResult struct {
	Drifted     bool    `json:"drifted"`
	RecentAvg   float64 `json:"recent_avg"`
	BaselineAvg float64 `json:"baseline_avg"`
	Drop        float64 `json:"drop"` // baseline_avg - recent_avg (positive = decline)
	RecentDays  int     `json:"recent_days"`
	RecentCount int     `json:"recent_count"` // blocks scored in the recent window
	Reason      string  `json:"reason,omitempty"`
}

// AnalyzeDrift splits a project's daily score trend into a recent window and a
// preceding baseline window, computes the count-weighted average score of each,
// and reports whether aggregate brand compliance has drifted — either by falling
// below an absolute floor or by dropping materially from its baseline. It is the
// detection kernel behind the brand.voice.drift alert; trends come from the
// store and can be in any date order.
func AnalyzeDrift(trends []*ScoreTrend, cfg DriftConfig) DriftResult {
	cfg = cfg.withDefaults()
	res := DriftResult{RecentDays: cfg.RecentDays}
	if len(trends) == 0 {
		return res
	}

	// Sort by date ascending so the last cfg.RecentDays entries are "recent".
	sorted := append([]*ScoreTrend(nil), trends...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].Date < sorted[j].Date })

	split := len(sorted) - cfg.RecentDays
	if split < 0 {
		split = 0
	}
	baseline := sorted[:split]
	recent := sorted[split:]

	recentAvg, recentCount := weightedAvg(recent)
	baselineAvg, _ := weightedAvg(baseline)
	res.RecentAvg = round1(recentAvg)
	res.RecentCount = recentCount
	if len(baseline) == 0 {
		// No baseline to compare against; the recent window is its own baseline.
		baselineAvg = recentAvg
	}
	res.BaselineAvg = round1(baselineAvg)
	res.Drop = round1(baselineAvg - recentAvg)

	if recentCount == 0 {
		return res // nothing scored recently — no signal
	}
	switch {
	case cfg.MinScore > 0 && recentAvg < float64(cfg.MinScore):
		res.Drifted = true
		res.Reason = "recent average below the floor"
	case cfg.DropPoints > 0 && res.Drop >= float64(cfg.DropPoints):
		res.Drifted = true
		res.Reason = "recent average dropped from baseline"
	}
	return res
}

func weightedAvg(trends []*ScoreTrend) (avg float64, count int) {
	var sum float64
	for _, t := range trends {
		if t == nil {
			continue
		}
		sum += t.AvgScore * float64(t.Count)
		count += t.Count
	}
	if count == 0 {
		return 0, 0
	}
	return sum / float64(count), count
}

func round1(f float64) float64 {
	return math.Round(f*10) / 10
}
