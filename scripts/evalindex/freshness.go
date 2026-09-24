package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// stampKeys lists the timestamp field names accepted across harness datasets.
var stampKeys = []string{"generated", "generatedAt", "generated_at", "ranAt", "timestamp", "date"}

// Freshness records the measurement date. The page computes age at render time
// so a committed index remains valid as time passes.
type Freshness struct {
	// Date is the measurement time the dataset records, RFC3339 or a bare day.
	Date string `json:"date,omitempty"`
	// Undated marks a dataset with no timestamp anywhere in it. Worse than an
	// old date: an old date can be judged, and this cannot.
	Undated bool `json:"undated,omitempty"`
}

// readFreshness finds a dataset's own timestamp.
//
// The search is shallow-first and bounded: a stamp at the top level is the
// dataset's own, and one three levels down is probably a record inside it. Two
// levels covers the wrappers that hold reports by mode or by model without
// reaching into individual results.
// at, when set, is the key inside the dataset this card's numbers live under.
//
// A dataset may contain independent reports. When at is set, search only that
// report so another mode's newer timestamp cannot make this result look fresh.
func readFreshness(root, rel, at string) Freshness {
	body, err := os.ReadFile(filepath.Join(root, rel))
	if err != nil {
		return Freshness{Undated: true}
	}
	var doc any
	if json.Unmarshal(body, &doc) != nil {
		return Freshness{Undated: true}
	}
	if at != "" {
		m, ok := doc.(map[string]any)
		if !ok {
			return Freshness{Undated: true}
		}
		section, ok := m[at]
		if !ok {
			// A missing report has no measurement date; do not use a sibling's date.
			return Freshness{Undated: true}
		}
		doc = section
	}

	stamp := findStamp(doc, 0)
	if stamp == "" {
		return Freshness{Undated: true}
	}

	if _, ok := parseStamp(stamp); !ok {
		// A string sitting under a date-shaped key that is not a date tells a
		// reader nothing, and printing it would look like a date.
		return Freshness{Undated: true}
	}
	return Freshness{Date: stamp}
}

// findStamp returns the newest timestamp at or near the top of a document.
//
// Newest rather than first: a dataset that appends runs holds many dates, and
// the one that matters is when it was last refreshed. Taking the first would
// report a suite as stale on the strength of its oldest surviving entry.
func findStamp(o any, depth int) string {
	if depth > 2 {
		return ""
	}
	best := ""
	switch v := o.(type) {
	case map[string]any:
		for _, k := range stampKeys {
			if raw, ok := v[k].(string); ok && raw != "" {
				best = newer(best, raw)
			}
		}
		for _, child := range v {
			best = newer(best, findStamp(child, depth+1))
		}
	case []any:
		for _, child := range v {
			best = newer(best, findStamp(child, depth+1))
		}
	}
	return best
}

func newer(a, b string) string {
	if b == "" {
		return a
	}
	if a == "" {
		return b
	}
	ta, aok := parseStamp(a)
	tb, bok := parseStamp(b)
	switch {
	case aok && bok && tb.After(ta):
		return b
	case !aok && bok:
		return b
	default:
		return a
	}
}

func parseStamp(s string) (time.Time, bool) {
	for _, layout := range []string{time.RFC3339, "2006-01-02T15:04:05Z", "2006-01-02"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}
