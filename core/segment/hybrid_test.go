//go:build cgo

package segment_test

import (
	"context"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/segment"
	_ "github.com/neokapi/neokapi/core/segment/srx"
	_ "github.com/neokapi/neokapi/core/segment/uax29"
)

// With ICU linked, the default "srx" engine loads Okapi's full ruleset and runs
// the useIcu4jBreakRules hybrid: ICU supplies sentence breaks, SRX rules
// suppress the abbreviation false-positives. This exercises that the 11k-line
// Okapi file parses, every selected rule compiles under regexp2, and the
// hybrid combination behaves.
func TestHybridDefaultEngineSegments(t *testing.T) {
	if !segment.HasBaseBreaker() {
		t.Skip("no ICU base breaker linked")
	}
	eng, err := segment.Build("srx", segment.BaseConfig{}, nil)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	runsOf := func(s string) []model.Run {
		return []model.Run{{Text: &model.TextRun{Text: s}}}
	}

	cases := []struct {
		name string
		text string
		want int // expected segment count
	}{
		{"two plain sentences", "The cat sat. The dog ran.", 2},
		{"abbreviation not a break", "Dr. Smith arrived today.", 1},
		{"honorific mid-sentence", "I met Mr. Smith in Washington.", 1},
		{"three sentences", "One thing happened. Then another. And a third.", 3},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			spans, err := eng.Segment(context.Background(), runsOf(tc.text), model.LocaleID("en"))
			if err != nil {
				t.Fatalf("Segment: %v", err)
			}
			if len(spans) != tc.want {
				t.Errorf("text %q: got %d segments, want %d", tc.text, len(spans), tc.want)
				for i := range spans {
					t.Logf("  seg %d: %q", i, model.RunsText(spans[i].Range.ExtractRuns(runsOf(tc.text))))
				}
			}
		})
	}
}
