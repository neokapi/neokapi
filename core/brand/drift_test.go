package brand

import "testing"

func trend(date string, avg float64, count int) *ScoreTrend {
	return &ScoreTrend{Date: date, AvgScore: avg, Count: count}
}

func TestAnalyzeDrift(t *testing.T) {
	tests := []struct {
		name       string
		trends     []*ScoreTrend
		cfg        DriftConfig
		wantDrift  bool
		wantReason string
	}{
		{
			name:      "empty trend is not drift",
			trends:    nil,
			cfg:       DriftConfig{RecentDays: 2},
			wantDrift: false,
		},
		{
			name: "steady scores do not drift",
			trends: []*ScoreTrend{
				trend("2026-05-01", 90, 10), trend("2026-05-02", 91, 10),
				trend("2026-05-03", 90, 10), trend("2026-05-04", 92, 10),
			},
			cfg:       DriftConfig{RecentDays: 2, DropPoints: 10},
			wantDrift: false,
		},
		{
			name: "a material drop from baseline drifts",
			trends: []*ScoreTrend{
				trend("2026-05-01", 95, 10), trend("2026-05-02", 94, 10),
				trend("2026-05-03", 78, 10), trend("2026-05-04", 80, 10),
			},
			cfg:        DriftConfig{RecentDays: 2, DropPoints: 10},
			wantDrift:  true,
			wantReason: "recent average dropped from baseline",
		},
		{
			name: "below the absolute floor drifts even if steady",
			trends: []*ScoreTrend{
				trend("2026-05-01", 60, 10), trend("2026-05-02", 61, 10),
			},
			cfg:        DriftConfig{RecentDays: 2, MinScore: 70},
			wantDrift:  true,
			wantReason: "recent average below the floor",
		},
		{
			name: "count-weighted: a tiny low day does not dominate",
			trends: []*ScoreTrend{
				trend("2026-05-01", 90, 100), trend("2026-05-02", 90, 100),
				trend("2026-05-03", 50, 1), trend("2026-05-04", 89, 100),
			},
			cfg:       DriftConfig{RecentDays: 2, DropPoints: 10},
			wantDrift: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := AnalyzeDrift(tt.trends, tt.cfg)
			if got.Drifted != tt.wantDrift {
				t.Errorf("Drifted = %v, want %v (%+v)", got.Drifted, tt.wantDrift, got)
			}
			if tt.wantReason != "" && got.Reason != tt.wantReason {
				t.Errorf("Reason = %q, want %q", got.Reason, tt.wantReason)
			}
		})
	}
}

func TestAnalyzeDrift_DefaultsAndUnorderedInput(t *testing.T) {
	// Unsorted dates, zero-value cfg (defaults: 7-day window, 10-point drop).
	trends := []*ScoreTrend{
		trend("2026-05-10", 70, 5), // recent (only one day, but window is 7)
		trend("2026-05-01", 95, 5), // baseline
	}
	got := AnalyzeDrift(trends, DriftConfig{})
	if got.RecentDays != 7 {
		t.Errorf("RecentDays default = %d, want 7", got.RecentDays)
	}
	// With a 7-day recent window and only two points one day apart, both fall in
	// the recent window → no baseline → not drifted.
	if got.Drifted {
		t.Errorf("expected no drift when all points are within the recent window: %+v", got)
	}
}
