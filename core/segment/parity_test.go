//go:build cgo

package segment_test

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/segment"
	_ "github.com/neokapi/neokapi/core/segment/srx"
	_ "github.com/neokapi/neokapi/core/segment/uax29"
)

// TestSRXParityWithOkapi checks that neokapi's hybrid SRX engine (ICU/UAX-29
// base + Okapi's defaultSegmentation.srx exceptions) segments a multilingual
// corpus the same way the real Okapi does. The golden fixtures are produced from
// the actual Okapi SRXSegmenter by scripts/srx-parity/gen-golden.sh; segment
// texts are compared whitespace-trimmed so trim-policy differences (neokapi
// attaches inter-segment whitespace to the following segment; Okapi trims) do
// not count as divergences — what is compared is where the sentence boundaries
// fall.
func TestSRXParityWithOkapi(t *testing.T) {
	if !segment.HasBaseBreaker() {
		t.Skip("no ICU base breaker linked — hybrid parity needs cgo/ICU")
	}

	type golden struct {
		Locale   string   `json:"locale"`
		Text     string   `json:"text"`
		Segments []string `json:"segments"`
	}

	f, err := os.Open("srx/testdata/parity/golden.jsonl")
	if err != nil {
		t.Fatalf("open golden: %v", err)
	}
	defer f.Close()

	// One engine per locale, reused across lines (mirrors how a flow runs).
	engines := map[string]segment.Segmenter{}
	engineFor := func(loc string) segment.Segmenter {
		if e, ok := engines[loc]; ok {
			return e
		}
		e, err := segment.Build("srx", segment.BaseConfig{}, nil)
		if err != nil {
			t.Fatalf("Build: %v", err)
		}
		engines[loc] = e
		return e
	}

	var total, mismatched int
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1024*1024), 1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var g golden
		if err := json.Unmarshal([]byte(line), &g); err != nil {
			t.Fatalf("bad golden line: %v", err)
		}
		total++

		runs := []model.Run{{Text: &model.TextRun{Text: g.Text}}}
		spans, err := engineFor(g.Locale).Segment(context.Background(), runs, model.LocaleID(g.Locale))
		if err != nil {
			t.Errorf("[%s] Segment(%q): %v", g.Locale, g.Text, err)
			mismatched++
			continue
		}
		got := make([]string, 0, len(spans))
		for i := range spans {
			s := strings.TrimSpace(model.RunsText(spans[i].Range.ExtractRuns(runs)))
			if s != "" {
				got = append(got, s)
			}
		}
		if !equalStrings(got, g.Segments) {
			mismatched++
			t.Errorf("[%s] %q\n  okapi: %q\n  neokapi: %q", g.Locale, g.Text, g.Segments, got)
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("scan: %v", err)
	}

	t.Logf("SRX parity: %d/%d corpus lines match Okapi (%d divergent)", total-mismatched, total, mismatched)
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
