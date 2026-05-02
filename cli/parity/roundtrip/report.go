//go:build parity

package roundtrip

import (
	"fmt"
	"io"
	"sort"
	"sync"
)

// parityRecord is one (format, fixture, engine) outcome captured by
// the harness. The aggregator turns the slice of records into the
// summary table the report writer emits.
type parityRecord struct {
	Format   string
	Fixture  string
	Engine   string
	Required Tier
	Achieved Tier
	Skipped  bool
	SkipMsg  string
	Reason   string
	GotSize  int
	RefSize  int
}

var (
	parityRecordsMu sync.Mutex
	parityRecords   []parityRecord
)

// recordParityResult appends one fixture/engine outcome. Safe for
// concurrent calls from parallel sub-tests.
func recordParityResult(r parityRecord) {
	parityRecordsMu.Lock()
	defer parityRecordsMu.Unlock()
	parityRecords = append(parityRecords, r)
}

// resetParityRecords clears the buffer. Useful in tests.
func resetParityRecords() {
	parityRecordsMu.Lock()
	defer parityRecordsMu.Unlock()
	parityRecords = nil
}

// snapshotParityRecords returns a copy of the current records. The
// caller can read it without holding the lock.
func snapshotParityRecords() []parityRecord {
	parityRecordsMu.Lock()
	defer parityRecordsMu.Unlock()
	out := make([]parityRecord, len(parityRecords))
	copy(out, parityRecords)
	return out
}

// FlushParityReport writes a Markdown summary of every recorded
// fixture/engine outcome to w. The summary aggregates per (format,
// engine) into a tier histogram so we can see at a glance which
// engines on which formats reach byte-equal vs canonical-equal vs
// divergent vs skipped.
func FlushParityReport(w io.Writer) error {
	records := snapshotParityRecords()
	if len(records) == 0 {
		_, err := fmt.Fprintln(w, "# Parity report\n\n_no records collected_")
		return err
	}

	type aggKey struct{ Format, Engine string }
	type aggVal struct {
		Total       int
		Byte        int
		Canonical   int
		Semantic    int
		Divergent   int
		Skipped     int
		WorstSample string // first divergent fixture, for spot-checking
	}
	aggs := map[aggKey]*aggVal{}
	keys := map[aggKey]struct{}{}
	for _, r := range records {
		k := aggKey{r.Format, r.Engine}
		v, ok := aggs[k]
		if !ok {
			v = &aggVal{}
			aggs[k] = v
			keys[k] = struct{}{}
		}
		v.Total++
		switch {
		case r.Skipped:
			v.Skipped++
		case r.Achieved == TierByteEqual:
			v.Byte++
		case r.Achieved == TierCanonicalEqual:
			v.Canonical++
		case r.Achieved == TierSemanticEqual:
			v.Semantic++
		default:
			v.Divergent++
			if v.WorstSample == "" {
				v.WorstSample = r.Fixture
			}
		}
	}

	sortedKeys := make([]aggKey, 0, len(keys))
	for k := range keys {
		sortedKeys = append(sortedKeys, k)
	}
	sort.Slice(sortedKeys, func(i, j int) bool {
		if sortedKeys[i].Format != sortedKeys[j].Format {
			return sortedKeys[i].Format < sortedKeys[j].Format
		}
		return sortedKeys[i].Engine < sortedKeys[j].Engine
	})

	if _, err := fmt.Fprintln(w, "# Parity report"); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, ""); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, "Tier counts per (format, engine). `byte` = byte-equal vs okapi reference; `canon` = canonical-equal after normalization; `sem` = semantic-equal; `div` = divergent at every tier; `skip` = engine not asserted (intentional skip)."); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, ""); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, "| Format | Engine | Total | byte | canon | sem | div | skip | first divergent fixture |"); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, "|---|---|---:|---:|---:|---:|---:|---:|---|"); err != nil {
		return err
	}
	for _, k := range sortedKeys {
		v := aggs[k]
		sample := v.WorstSample
		if sample == "" {
			sample = "—"
		}
		if _, err := fmt.Fprintf(w, "| %s | %s | %d | %d | %d | %d | %d | %d | %s |\n",
			k.Format, k.Engine, v.Total, v.Byte, v.Canonical, v.Semantic, v.Divergent, v.Skipped, sample); err != nil {
			return err
		}
	}

	return nil
}
