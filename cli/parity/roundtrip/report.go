//go:build parity

package roundtrip

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"
	"time"
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
// fixture/engine outcome to w. The report has three sections:
//
//  1. Engine totals: one row per engine with overall byte/canon/sem/div/skip
//     counts so a single glance answers "is bridge holding parity? how much
//     of the native gap is structural vs stylistic?".
//  2. Per-format summary: per (format, engine) tier histogram so we see at
//     a glance which engines reach which tiers on which formats.
//  3. Divergent detail: per (format, engine) that has any divergent
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

	if _, err := fmt.Fprintln(w, "# Parity report"); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w); err != nil {
		return err
	}
	if err := writeEngineTotals(w, records); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w); err != nil {
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

// writeEngineTotals prints one row per engine with the global byte / canon
// / sem / div / skip counts and the byte-equal percentage. This is the
// "did anything regress vs last run?" view — the per-format table tells
// you where the gaps are; this tells you whether they grew or shrank.
func writeEngineTotals(w io.Writer, records []parityRecord) error {
	type tot struct {
		Total, Byte, Canon, Sem, Div, Skip int
	}
	totals := map[string]*tot{}
	var engines []string
	for _, r := range records {
		v, ok := totals[r.Engine]
		if !ok {
			v = &tot{}
			totals[r.Engine] = v
			engines = append(engines, r.Engine)
		}
		v.Total++
		switch {
		case r.Skipped:
			v.Skip++
		case r.Achieved == TierByteEqual:
			v.Byte++
		case r.Achieved == TierCanonicalEqual:
			v.Canon++
		case r.Achieved == TierSemanticEqual:
			v.Sem++
		default:
			v.Div++
		}
	}
	sort.Strings(engines)

	if _, err := fmt.Fprintln(w, "## Totals"); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, "| Engine | Total | byte | canon | sem | div | skip | byte% |"); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, "|---|---:|---:|---:|---:|---:|---:|---:|"); err != nil {
		return err
	}
	for _, e := range engines {
		v := totals[e]
		// Percentage is byte-equal divided by asserted (total minus skipped) —
		// skipped fixtures don't represent failures, so they shouldn't drag the
		// percentage down.
		asserted := v.Total - v.Skip
		var pct float64
		if asserted > 0 {
			pct = 100 * float64(v.Byte) / float64(asserted)
		}
		if _, err := fmt.Fprintf(w, "| %s | %d | %d | %d | %d | %d | %d | %.1f%% |\n",
			e, v.Total, v.Byte, v.Canon, v.Sem, v.Div, v.Skip, pct); err != nil {
			return err
		}
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
	if _, err := fmt.Fprintln(w, "## Per-format summary"); err != nil {
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

// fixturesJSON is the on-disk shape consumed by the /parity/fixtures
// drill-down dashboard. It carries per-engine totals plus a per-format
// breakdown that nests every divergent fixture's first-diff offset and
// reason snippet so a reader can answer "why does idml/06-hello-world.idml
// still divergent?" without re-running the test.
type fixturesJSON struct {
	GeneratedAt string                  `json:"generated_at"`
	Engines     map[string]engineTotals `json:"engines"`
	Formats     []formatBreakdown       `json:"formats"`
}

type engineTotals struct {
	Total   int     `json:"total"`
	Byte    int     `json:"byte"`
	Canon   int     `json:"canon"`
	Sem     int     `json:"sem"`
	Div     int     `json:"div"`
	Skip    int     `json:"skip"`
	BytePct float64 `json:"byte_pct"`
}

type formatBreakdown struct {
	Format string `json:"format"`
	Engine string `json:"engine"`
	Total  int    `json:"total"`
	Byte   int    `json:"byte"`
	Canon  int    `json:"canon"`
	Sem    int    `json:"sem"`
	Div    int    `json:"div"`
	Skip   int    `json:"skip"`
	// Fixtures lists every per-fixture entry that didn't reach byte-equal
	// — canonical-equal, semantic-equal, and divergent rows are all
	// included so the dashboard can drill into "remaining work toward
	// byte-equal". Byte-equal and intentionally-skipped fixtures stay
	// out (no work left). For canon/sem rows the Reason still carries
	// the *raw* byte diff (compare.go sets it before normalization
	// succeeds), which is exactly the gap the user wants to inspect.
	Fixtures []fixtureEntry `json:"fixtures,omitempty"`
}

type fixtureEntry struct {
	Fixture        string `json:"fixture"`
	Required       string `json:"required"`
	Achieved       string `json:"achieved"`
	GotSize        int    `json:"got_size"`
	RefSize        int    `json:"ref_size"`
	Delta          int    `json:"delta"`
	RawDiffOffset  int    `json:"raw_diff_offset"`
	NormDiffOffset int    `json:"norm_diff_offset,omitempty"`
	Normalizer     string `json:"normalizer,omitempty"`
	Reason         string `json:"reason"`
}

// FlushParityFixturesJSON writes the per-fixture parity dataset as JSON.
// The shape is consumed by the /parity/fixtures Docusaurus page; the
// Markdown report stays the canonical CLI surface. Every non-byte-equal
// fixture appears in the per-format Fixtures array (canon, sem, div),
// so the dashboard can surface the remaining work toward byte-equal.
// Byte-equal and intentionally-skipped fixtures stay out — they have
// no remaining work — and are summarised in the totals only.
func FlushParityFixturesJSON(w io.Writer) error {
	records := snapshotParityRecords()
	out := fixturesJSON{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Engines:     map[string]engineTotals{},
	}

	// Engine totals.
	type engTot struct{ Total, Byte, Canon, Sem, Div, Skip int }
	engines := map[string]*engTot{}
	for _, r := range records {
		v, ok := engines[r.Engine]
		if !ok {
			v = &engTot{}
			engines[r.Engine] = v
		}
		v.Total++
		switch {
		case r.Skipped:
			v.Skip++
		case r.Achieved == TierByteEqual:
			v.Byte++
		case r.Achieved == TierCanonicalEqual:
			v.Canon++
		case r.Achieved == TierSemanticEqual:
			v.Sem++
		default:
			v.Div++
		}
	}
	for name, v := range engines {
		asserted := v.Total - v.Skip
		var pct float64
		if asserted > 0 {
			pct = 100 * float64(v.Byte) / float64(asserted)
		}
		out.Engines[name] = engineTotals{
			Total:   v.Total,
			Byte:    v.Byte,
			Canon:   v.Canon,
			Sem:     v.Sem,
			Div:     v.Div,
			Skip:    v.Skip,
			BytePct: pct,
		}
	}

	// Per-format breakdown plus per-fixture detail for every
	// non-byte-equal fixture (canon, sem, div).
	aggs, keys := aggregate(records)
	type detailKey aggKey
	groups := map[detailKey][]parityRecord{}
	for _, r := range records {
		if r.Skipped || r.Achieved == TierByteEqual {
			continue
		}
		groups[detailKey{r.Format, r.Engine}] = append(groups[detailKey{r.Format, r.Engine}], r)
	}
	// Sort fixtures within a (format, engine) group by remaining-work
	// severity: divergent first, then semantic, canonical, byte (which
	// is filtered out anyway). Fixtures with the same tier sort by name
	// so the dashboard order is stable across runs.
	tierRank := func(t Tier) int {
		switch t {
		case TierDivergent:
			return 0
		case TierSemanticEqual:
			return 1
		case TierCanonicalEqual:
			return 2
		default:
			return 3
		}
	}
	for _, k := range keys {
		v := aggs[k]
		fb := formatBreakdown{
			Format: k.Format,
			Engine: k.Engine,
			Total:  v.Total,
			Byte:   v.Byte,
			Canon:  v.Canonical,
			Sem:    v.Semantic,
			Div:    v.Divergent,
			Skip:   v.Skipped,
		}
		if dets, ok := groups[detailKey{k.Format, k.Engine}]; ok {
			sort.Slice(dets, func(i, j int) bool {
				ri, rj := tierRank(dets[i].Achieved), tierRank(dets[j].Achieved)
				if ri != rj {
					return ri < rj
				}
				return dets[i].Fixture < dets[j].Fixture
			})
			for _, r := range dets {
				fb.Fixtures = append(fb.Fixtures, fixtureEntry{
					Fixture:        r.Fixture,
					Required:       r.Required.String(),
					Achieved:       r.Achieved.String(),
					GotSize:        r.GotSize,
					RefSize:        r.RefSize,
					Delta:          r.GotSize - r.RefSize,
					RawDiffOffset:  r.RawDiffOffset,
					NormDiffOffset: r.NormDiffOffset,
					Normalizer:     r.Normalizer,
					Reason:         r.Reason,
				})
			}
		}
		out.Formats = append(out.Formats, fb)
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
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
