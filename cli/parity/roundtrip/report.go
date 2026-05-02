//go:build parity

package roundtrip

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"
)

// parityRecord is one (format, fixture, engine) outcome captured by
// the harness. The aggregator turns the slice of records into the
// summary table the report writer emits.
type parityRecord struct {
	Format         string
	Fixture        string
	Engine         string
	Required       Tier
	Achieved       Tier
	Skipped        bool
	SkipMsg        string
	Reason         string
	GotSize        int
	RefSize        int
	RawDiffOffset  int // -1 when byte-equal
	NormDiffOffset int // -1 when canonical-equal or normalizer absent
	Normalizer     string
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

// FlushParityReport writes a Markdown report of every recorded
// fixture/engine outcome to w. The report has two sections:
//
//  1. Summary table: per (format, engine), tier histogram so we see at
//     a glance which engines reach which tiers on which formats.
//  2. Divergent detail: per (format, engine) that has any divergent
//     fixture, a table listing fixture, sizes, first-diff offset, and
//     a sample of the diff so humans can spot patterns (line endings,
//     whitespace, encoding, …) without re-running.
//
// Skipped engines are omitted from the detail section — they have no
// data to drill into.
func FlushParityReport(w io.Writer) error {
	records := snapshotParityRecords()
	if len(records) == 0 {
		_, err := fmt.Fprintln(w, "# Parity report\n\n_no records collected_")
		return err
	}

	if err := writeSummaryTable(w, records); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w); err != nil {
		return err
	}
	if err := writeDivergentDetail(w, records); err != nil {
		return err
	}
	return nil
}

type aggKey struct{ Format, Engine string }

type aggVal struct {
	Total       int
	Byte        int
	Canonical   int
	Semantic    int
	Divergent   int
	Skipped     int
	WorstSample string
}

func aggregate(records []parityRecord) (map[aggKey]*aggVal, []aggKey) {
	aggs := map[aggKey]*aggVal{}
	for _, r := range records {
		k := aggKey{r.Format, r.Engine}
		v, ok := aggs[k]
		if !ok {
			v = &aggVal{}
			aggs[k] = v
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
	keys := make([]aggKey, 0, len(aggs))
	for k := range aggs {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].Format != keys[j].Format {
			return keys[i].Format < keys[j].Format
		}
		return keys[i].Engine < keys[j].Engine
	})
	return aggs, keys
}

func writeSummaryTable(w io.Writer, records []parityRecord) error {
	aggs, keys := aggregate(records)
	if _, err := fmt.Fprintln(w, "# Parity report"); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, "## Summary"); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, "Tier counts per (format, engine). `byte` = byte-equal vs okapi reference; `canon` = canonical-equal after normalization; `sem` = semantic-equal; `div` = divergent at every tier; `skip` = engine not asserted (intentional skip)."); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, "| Format | Engine | Total | byte | canon | sem | div | skip | first divergent fixture |"); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, "|---|---|---:|---:|---:|---:|---:|---:|---|"); err != nil {
		return err
	}
	for _, k := range keys {
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

// writeDivergentDetail emits per (format, engine) sections listing the
// divergent fixtures with size, first-diff offset, and the raw
// reason/snippet from the comparison. This is the drill-down the
// summary alone can't give: a reader can scan the table for patterns
// ("everything's off by ~20 bytes — maybe line endings") without
// re-running the test.
func writeDivergentDetail(w io.Writer, records []parityRecord) error {
	// Group divergent records by (format, engine).
	type detailKey aggKey
	groups := map[detailKey][]parityRecord{}
	for _, r := range records {
		if r.Skipped || r.Achieved == TierByteEqual || r.Achieved == TierCanonicalEqual || r.Achieved == TierSemanticEqual {
			continue
		}
		k := detailKey{r.Format, r.Engine}
		groups[k] = append(groups[k], r)
	}
	if len(groups) == 0 {
		_, err := fmt.Fprintln(w, "## Divergent detail\n\n_no divergent fixtures — every engine reached its required tier_")
		return err
	}

	keys := make([]detailKey, 0, len(groups))
	for k := range groups {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].Format != keys[j].Format {
			return keys[i].Format < keys[j].Format
		}
		return keys[i].Engine < keys[j].Engine
	})

	if _, err := fmt.Fprintln(w, "## Divergent detail"); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, "Per (format, engine) breakdown of fixtures whose output didn't reach the required tier. `Δbytes` is engine-output size minus reference size (negative = engine emitted fewer bytes). `first diff` is the offset of the first byte that differs; the snippet shows up to ~32 bytes of context from each side."); err != nil {
		return err
	}
	for _, k := range keys {
		group := groups[k]
		// Sort fixtures by name so the table is stable across runs.
		sort.Slice(group, func(i, j int) bool { return group[i].Fixture < group[j].Fixture })
		if _, err := fmt.Fprintf(w, "\n### %s / %s (%d divergent)\n\n", k.Format, k.Engine, len(group)); err != nil {
			return err
		}
		if _, err := fmt.Fprintln(w, "| Fixture | got | ref | Δbytes | first diff | reason |"); err != nil {
			return err
		}
		if _, err := fmt.Fprintln(w, "|---|---:|---:|---:|---:|---|"); err != nil {
			return err
		}
		for _, r := range group {
			delta := r.GotSize - r.RefSize
			if _, err := fmt.Fprintf(w, "| %s | %d | %d | %+d | %s | %s |\n",
				r.Fixture, r.GotSize, r.RefSize, delta, formatDiffOffset(r), escapeMarkdownCell(r.Reason)); err != nil {
				return err
			}
		}
	}
	return nil
}

// formatDiffOffset renders the byte-diff column for a divergent row.
// Shows the raw byte offset; when a normalizer was tried, also shows
// the normalized offset so we can see how much of the gap was style
// (raw≠norm: stylistic; raw=norm or norm=-1: real divergence).
func formatDiffOffset(r parityRecord) string {
	if r.Achieved == TierByteEqual {
		return "—"
	}
	if r.Normalizer != "" && r.NormDiffOffset >= 0 {
		return fmt.Sprintf("raw@%d, norm@%d", r.RawDiffOffset, r.NormDiffOffset)
	}
	return fmt.Sprintf("@%d", r.RawDiffOffset)
}

// escapeMarkdownCell makes the comparison reason safe to drop into a
// table cell: pipes break the column boundary; CR/LF break the row.
func escapeMarkdownCell(s string) string {
	r := strings.NewReplacer(
		"|", `\|`,
		"\r", " ",
		"\n", " ",
	)
	return r.Replace(s)
}
