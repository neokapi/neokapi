//go:build parity

package roundtrip

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/neokapi/neokapi/cli/parity"
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

	// CanonClass classifies a canonical-equal outcome as faithful (native
	// preserves source; okapi re-serializes) or closeable (native loses
	// source info). Meaningful only when Achieved == TierCanonicalEqual.
	CanonClass CanonClass

	// Annotation, when non-nil, carries the per-fixture metadata loaded
	// from core/formats/<format>/parity-annotations.yaml. nil = no
	// annotation declared for this fixture. The harness fills this in
	// when recording each result.
	Annotation *Annotation
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

// RecordOkapiSkip is called from the coverage harness when a fixture's
// `okapi` engine is skipped via the per-format annotation system, so
// the round-trip never runs. We emit a Skipped parityRecord against
// the "native" engine slot so the coverage map / dashboard can still
// see this fixture exists and is part of the scan — without it,
// scan-having formats whose every fixture is okapi-skipped (e.g.
// transtable) get misclassified as scan-missing.
//
// We pick "native" as the engine name (rather than a synthetic
// "okapi-skipped") because buildCoverageMap counts native records to
// derive roundtrip_fixtures, and this is the cheapest way to make
// transtable visible without changing that aggregation rule.
func RecordOkapiSkip(format, fixture, reason string) {
	var ann *Annotation
	if a, ok := LookupAnnotation(format, fixture); ok {
		ann = &a
	}
	recordParityResult(parityRecord{
		Format:     format,
		Fixture:    fixture,
		Engine:     "native",
		Required:   TierDivergent,
		Skipped:    true,
		SkipMsg:    reason,
		Annotation: ann,
	})
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

// UnannotatedDivergence pairs a divergent (format, fixture, engine)
// triple with the parityRecord that produced it. Used by the
// fail-new CI gate to surface unannotated divergences.
type UnannotatedDivergence struct {
	Format  string
	Fixture string
	Engine  string
	Reason  string
}

// UnannotatedDivergences returns one entry per (format, fixture,
// engine) that reached the divergent tier without a corresponding
// annotation in core/formats/<format>/parity-annotations.yaml.
// "Divergent" here means the achieved tier was strictly worse than
// the required tier — meeting MinTier with canonical-equal counts as
// passing, not as a divergence to annotate.
//
// The CI gate (TestParityFailNew) uses this to refuse merges that
// add new divergences without documenting them. Existing divergences
// are grandfathered by annotation; clearing one means deleting the
// annotation entry once the underlying bug is fixed.
//
// Records collected per fixture/engine are deduplicated by
// (format, fixture) — a single missing annotation surfaces once, not
// once per engine. The dashboard is annotation-keyed at the fixture
// level, so per-engine entries would just be noise.
func UnannotatedDivergences() []UnannotatedDivergence {
	records := snapshotParityRecords()
	seen := map[string]bool{}
	var out []UnannotatedDivergence
	for _, r := range records {
		if r.Skipped {
			continue
		}
		if r.Achieved <= r.Required {
			continue
		}
		if r.Annotation != nil {
			continue
		}
		key := r.Format + "/" + r.Fixture
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, UnannotatedDivergence{
			Format:  r.Format,
			Fixture: r.Fixture,
			Engine:  r.Engine,
			Reason:  r.Reason,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Format != out[j].Format {
			return out[i].Format < out[j].Format
		}
		return out[i].Fixture < out[j].Fixture
	})
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
		Total, Byte, Canon, CanonFaithful, CanonCloseable, Sem, Div, Skip int
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
			switch r.CanonClass {
			case CanonFaithful:
				v.CanonFaithful++
			case CanonCloseable:
				v.CanonCloseable++
			}
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
	// `canon` splits into faithful (native preserves source, okapi
	// re-serializes — expected) + closeable (native loses source info —
	// real work). `byte%` is byte-equal alone; `faithful%` adds faithful
	// canon — the honest "as faithful as okapi or better" figure.
	if _, err := fmt.Fprintln(w, "| Engine | Total | byte | canon (faith/close) | sem | div | skip | byte% | faithful% |"); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, "|---|---:|---:|---:|---:|---:|---:|---:|---:|"); err != nil {
		return err
	}
	for _, e := range engines {
		v := totals[e]
		// Percentage is byte-equal divided by asserted (total minus skipped) —
		// skipped fixtures don't represent failures, so they shouldn't drag the
		// percentage down. faithful% adds faithful canon to the numerator.
		asserted := v.Total - v.Skip
		var pct, faithfulPct float64
		if asserted > 0 {
			pct = 100 * float64(v.Byte) / float64(asserted)
			faithfulPct = 100 * float64(v.Byte+v.CanonFaithful) / float64(asserted)
		}
		if _, err := fmt.Fprintf(w, "| %s | %d | %d | %d (%d/%d) | %d | %d | %d | %.1f%% | %.1f%% |\n",
			e, v.Total, v.Byte, v.Canon, v.CanonFaithful, v.CanonCloseable, v.Sem, v.Div, v.Skip, pct, faithfulPct); err != nil {
			return err
		}
	}
	return nil
}

type aggKey struct{ Format, Engine string }

type aggVal struct {
	Total          int
	Byte           int
	Canonical      int
	CanonFaithful  int
	CanonCloseable int
	Semantic       int
	Divergent      int
	Skipped        int
	WorstSample    string
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
			switch r.CanonClass {
			case CanonFaithful:
				v.CanonFaithful++
			case CanonCloseable:
				v.CanonCloseable++
			}
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

// fixturesJSON is the on-disk shape consumed by the /parity dashboard.
// It carries per-engine totals, a per-format breakdown that nests every
// divergent fixture's first-diff offset and reason snippet, and a
// coverage map describing each format's parity status across the
// bridge / native / round-trip axes.
type fixturesJSON struct {
	GeneratedAt string                  `json:"generated_at"`
	Engines     map[string]engineTotals `json:"engines"`
	Formats     []formatBreakdown       `json:"formats"`
	CoverageMap []coverageMapEntry      `json:"coverage_map"`
}

// coverageMapEntry is one row of the coverage map: a single format's
// status across the bridge / native / round-trip axes. Surfaces "what's
// pending" — a format with a bridge filter but no Go port, or with a Go
// port whose round-trip scan isn't wired up despite upstream having
// fixtures, lights up as actionable work.
//
// Status values:
//
//   - "covered"        — bridge + native + round-trip fixtures present
//                        AND the suite actually exercises them.
//   - "scan-missing"   — bridge + native present, upstream Okapi has
//                        fixtures for this format (.odt files for odf,
//                        .pdf for pdf, …), but our cli/parity/roundtrip
//                        coverageScans() doesn't include it. Adding a
//                        scan entry is the work.
//   - "no-upstream"    — bridge + native present, and upstream Okapi
//                        truly has no test corpus to run. Either no
//                        upstream pipeline produces a usable reference
//                        (rtf has only tradosrtf; txml NPEs on merge)
//                        or the scan needs special setup the harness
//                        can't autodetect (srt needs .fprm rules).
//   - "bridge-only"    — bridge filter exists, no Go port yet. Typically
//                        binary-corpus formats (pdf, rtf, archive,
//                        sdlpackage, pensieve, …) where neokapi hasn't
//                        built a native reader/writer.
//   - "native-only"    — Go port exists, no bridge filter. neokapi-only
//                        formats (jsx, klf, versifiedtext, messageformat) —
//                        no Okapi reference to compare against.
type coverageMapEntry struct {
	ID                string `json:"id"`
	BridgeFilter      string `json:"bridge_filter,omitempty"`
	Native            bool   `json:"native"`
	RoundtripFixtures int    `json:"roundtrip_fixtures"`
	// UpstreamFixtures is the count of files in the upstream Okapi
	// testdata tarball whose extension matches one of this format's
	// known extensions. Best-effort — relies on the format-extension
	// map below being kept in sync. Zero = nothing upstream (true gap
	// is on the Okapi side); non-zero but RoundtripFixtures==0 = we
	// have a scan-wiring gap on the neokapi side.
	UpstreamFixtures int    `json:"upstream_fixtures"`
	NativeByte       int    `json:"native_byte,omitempty"`
	NativeCanon      int    `json:"native_canon,omitempty"`
	NativeDiv        int    `json:"native_div,omitempty"`
	Annotations      int    `json:"annotations,omitempty"`
	Status           string `json:"status"`
}

type engineTotals struct {
	Total int `json:"total"`
	Byte  int `json:"byte"`
	Canon int `json:"canon"`
	// CanonFaithful / CanonCloseable partition Canon by CanonClass:
	// faithful = native preserves source, okapi re-serializes (expected,
	// don't chase); closeable = native loses source info (real work).
	// Unclassified canon falls into neither and only ever under-states
	// faithful parity.
	CanonFaithful  int     `json:"canon_faithful"`
	CanonCloseable int     `json:"canon_closeable"`
	Sem            int     `json:"sem"`
	Div            int     `json:"div"`
	Skip           int     `json:"skip"`
	BytePct        float64 `json:"byte_pct"`
	// FaithfulPct = (byte + canon_faithful) / asserted. The honest
	// headline: how much of the corpus native handles at least as
	// faithfully as okapi (byte-identical, or canonically-equal because
	// okapi re-serialized).
	FaithfulPct float64 `json:"faithful_pct"`
}

type formatBreakdown struct {
	Format         string `json:"format"`
	Engine         string `json:"engine"`
	Total          int    `json:"total"`
	Byte           int    `json:"byte"`
	Canon          int    `json:"canon"`
	CanonFaithful  int    `json:"canon_faithful"`
	CanonCloseable int    `json:"canon_closeable"`
	Sem            int    `json:"sem"`
	Div            int    `json:"div"`
	Skip           int    `json:"skip"`
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
	Fixture        string          `json:"fixture"`
	Required       string          `json:"required"`
	Achieved       string          `json:"achieved"`
	GotSize        int             `json:"got_size"`
	RefSize        int             `json:"ref_size"`
	Delta          int             `json:"delta"`
	RawDiffOffset  int             `json:"raw_diff_offset"`
	NormDiffOffset int             `json:"norm_diff_offset,omitempty"`
	Normalizer     string          `json:"normalizer,omitempty"`
	Reason         string          `json:"reason"`
	Annotation     *annotationJSON `json:"annotation,omitempty"`
}

// annotationJSON is the dashboard-facing slice of an Annotation. The
// loader's full Annotation also carries the optional Skip directive,
// which is harness-internal — the dashboard only needs the metadata
// fields, plus an issue_url synthesised from Issue.
type annotationJSON struct {
	Severity    string `json:"severity,omitempty"`
	Issue       int    `json:"issue,omitempty"`
	IssueURL    string `json:"issue_url,omitempty"`
	Summary     string `json:"summary,omitempty"`
	SpecRef     string `json:"spec_ref,omitempty"`
	NotesAnchor string `json:"notes_anchor,omitempty"`
}

// annotationIssueRepo is the GitHub repo whose issue numbers are
// auto-linked in the dashboard. Single-source-of-truth for the URL
// template — change here if the project ever moves repos.
const annotationIssueRepo = "neokapi/neokapi"

// toAnnotationJSON projects a loader Annotation into the dashboard
// shape, synthesising the issue URL from the issue number.
func toAnnotationJSON(a *Annotation) *annotationJSON {
	if a == nil {
		return nil
	}
	out := &annotationJSON{
		Severity:    a.Severity,
		Issue:       a.Issue,
		Summary:     a.Summary,
		SpecRef:     a.SpecRef,
		NotesAnchor: a.NotesAnchor,
	}
	if a.Issue > 0 {
		out.IssueURL = fmt.Sprintf("https://github.com/%s/issues/%d", annotationIssueRepo, a.Issue)
	}
	return out
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
	type engTot struct {
		Total, Byte, Canon, CanonFaithful, CanonCloseable, Sem, Div, Skip int
	}
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
			switch r.CanonClass {
			case CanonFaithful:
				v.CanonFaithful++
			case CanonCloseable:
				v.CanonCloseable++
			}
		case r.Achieved == TierSemanticEqual:
			v.Sem++
		default:
			v.Div++
		}
	}
	for name, v := range engines {
		asserted := v.Total - v.Skip
		var pct, faithfulPct float64
		if asserted > 0 {
			pct = 100 * float64(v.Byte) / float64(asserted)
			faithfulPct = 100 * float64(v.Byte+v.CanonFaithful) / float64(asserted)
		}
		out.Engines[name] = engineTotals{
			Total:          v.Total,
			Byte:           v.Byte,
			Canon:          v.Canon,
			CanonFaithful:  v.CanonFaithful,
			CanonCloseable: v.CanonCloseable,
			Sem:            v.Sem,
			Div:            v.Div,
			Skip:           v.Skip,
			BytePct:        pct,
			FaithfulPct:    faithfulPct,
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
			Format:         k.Format,
			Engine:         k.Engine,
			Total:          v.Total,
			Byte:           v.Byte,
			Canon:          v.Canonical,
			CanonFaithful:  v.CanonFaithful,
			CanonCloseable: v.CanonCloseable,
			Sem:            v.Semantic,
			Div:            v.Divergent,
			Skip:           v.Skipped,
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
					Annotation:     toAnnotationJSON(r.Annotation),
				})
			}
		}
		out.Formats = append(out.Formats, fb)
	}

	out.CoverageMap = buildCoverageMap(records)

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

// buildCoverageMap joins four sources to produce the per-format
// coverage rows: the upstream bridge filter universe (bridgeFilterIDs),
// the native package universe (core/formats/ subdirs walked at runtime),
// the round-trip coverage actually exercised by this test run (the
// records buffer), and a count of how many fixtures upstream Okapi
// ships for each format (extension-matched file count under
// $sandbox/okapi/). The upstream count is what lets us distinguish
// "scan-missing" (we need to wire it up) from "no-upstream" (there's
// genuinely nothing to test against).
func buildCoverageMap(records []parityRecord) []coverageMapEntry {
	native := discoverNativeFormats()
	bridge := bridgeFilterIDs()
	upstream := countUpstreamFixtures()
	roundtrip := map[string]struct {
		fixtures, byteCount, canon, div, annotated int
	}{}
	for _, r := range records {
		if r.Engine != "native" {
			continue
		}
		v := roundtrip[r.Format]
		v.fixtures++
		switch {
		case r.Achieved == TierByteEqual:
			v.byteCount++
		case r.Achieved == TierCanonicalEqual:
			v.canon++
		case r.Achieved == TierDivergent:
			v.div++
		}
		if r.Annotation != nil {
			v.annotated++
		}
		roundtrip[r.Format] = v
	}

	ids := map[string]bool{}
	for _, id := range native {
		ids[id] = true
	}
	for id := range bridge {
		ids[id] = true
	}
	for id := range roundtrip {
		ids[id] = true
	}

	out := make([]coverageMapEntry, 0, len(ids))
	for id := range ids {
		hasNative := false
		for _, n := range native {
			if n == id {
				hasNative = true
				break
			}
		}
		bridgeFilter := bridge[id]
		rt := roundtrip[id]
		up := upstream[id]
		status := classifyCoverage(hasNative, bridgeFilter != "", rt.fixtures, up)
		out = append(out, coverageMapEntry{
			ID:                id,
			BridgeFilter:      bridgeFilter,
			Native:            hasNative,
			RoundtripFixtures: rt.fixtures,
			UpstreamFixtures:  up,
			NativeByte:        rt.byteCount,
			NativeCanon:       rt.canon,
			NativeDiv:         rt.div,
			Annotations:       rt.annotated,
			Status:            status,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		// Surface actionable gaps first: scan-missing (we're sitting on
		// upstream fixtures we don't exercise), then bridge-only (need a
		// Go port), then no-upstream and native-only (no work the
		// dashboard can directly drive), then fully-covered. Stable
		// within bucket by format name.
		ri := coverageStatusRank(out[i].Status)
		rj := coverageStatusRank(out[j].Status)
		if ri != rj {
			return ri < rj
		}
		return out[i].ID < out[j].ID
	})
	return out
}

func classifyCoverage(hasNative, hasBridge bool, roundtripFixtures, upstreamFixtures int) string {
	switch {
	case hasNative && hasBridge && roundtripFixtures > 0:
		return "covered"
	case hasNative && hasBridge && roundtripFixtures == 0 && upstreamFixtures > 0:
		return "scan-missing"
	case hasNative && hasBridge && roundtripFixtures == 0 && upstreamFixtures == 0:
		return "no-upstream"
	case hasBridge && !hasNative:
		return "bridge-only"
	case hasNative && !hasBridge:
		return "native-only"
	default:
		return "unknown"
	}
}

func coverageStatusRank(s string) int {
	switch s {
	case "scan-missing":
		return 0
	case "bridge-only":
		return 1
	case "no-upstream":
		return 2
	case "native-only":
		return 3
	case "covered":
		return 4
	default:
		return 5
	}
}

// countUpstreamFixtures walks each format's source directories within
// the unpacked Okapi testdata tarball and counts files with that
// format's known extensions. Returns format ID → file count. Restricts
// per-format walks to per-format dirs (vs walking the whole tarball
// once) to avoid inflating counts for formats sharing extensions —
// .txt belongs to plaintext, mosestext, fixedwidth, regex, transtable,
// splicedlines all at once, and a global walk would attribute every
// .txt match to every one of them.
//
// Best-effort: when the sandbox is unavailable (e.g. running the
// report writer outside of a parity test), returns an empty map and
// classification falls back to no-upstream for anything without a scan.
func countUpstreamFixtures() map[string]int {
	out := map[string]int{}
	s, err := parity.LoadSandbox()
	if err != nil || s == nil || s.OkapiTestDataDir == "" {
		return out
	}
	root := s.OkapiTestDataDir
	for fmtID, src := range formatSourceDirs() {
		extSet := map[string]bool{}
		for _, e := range src.Extensions {
			extSet[strings.ToLower(e)] = true
		}
		if len(extSet) == 0 {
			continue
		}
		for _, dir := range src.Dirs {
			abs := filepath.Join(root, dir)
			info, err := os.Stat(abs)
			if err != nil || !info.IsDir() {
				continue
			}
			_ = filepath.WalkDir(abs, func(path string, d os.DirEntry, err error) error {
				if err != nil || d.IsDir() {
					return nil
				}
				if extSet[strings.ToLower(filepath.Ext(path))] {
					out[fmtID]++
				}
				return nil
			})
		}
	}
	return out
}

type upstreamSource struct {
	Dirs       []string
	Extensions []string
}

// formatSourceDirs returns the per-format upstream Okapi source
// directories and accepted extensions used to count "fixtures Okapi
// ships for this format". Mirrors coverageScans() for scanned formats
// and fills in conventional paths for unscanned ones (typical patterns:
// `okapi/filters/<id>/src/test/resources` and
// `integration-tests/okapi/src/test/resources/<id>`).
func formatSourceDirs() map[string]upstreamSource {
	return map[string]upstreamSource{
		"csv":           {[]string{"integration-tests/okapi/src/test/resources/table"}, []string{".csv"}},
		"doxygen":       {[]string{"integration-tests/okapi/src/test/resources/doxygen"}, []string{".h", ".py"}},
		"dtd":           {[]string{"okapi/filters/dtd/src/test/resources"}, []string{".dtd"}},
		"fixedwidth":    {[]string{"okapi/filters/table/src/test/resources"}, []string{".txt"}},
		"html":          {[]string{"integration-tests/okapi/src/test/resources/html"}, []string{".html", ".htm"}},
		"icml":          {[]string{"integration-tests/okapi/src/test/resources/icml"}, []string{".icml", ".wcml"}},
		"idml":          {[]string{"okapi/filters/idml/src/test/resources"}, []string{".idml"}},
		"json":          {[]string{"integration-tests/okapi/src/test/resources/json"}, []string{".json"}},
		"markdown":      {[]string{"integration-tests/okapi/src/test/resources/markdown"}, []string{".md"}},
		"mif":           {[]string{"integration-tests/okapi/src/test/resources/mif"}, []string{".mif"}},
		"mosestext":     {[]string{"okapi/filters/mosestext/src/test/resources"}, []string{".txt"}},
		"odf":           {[]string{"okapi/filters/openoffice/src/test/resources", "integration-tests/okapi/src/test/resources/openoffice"}, []string{".odt", ".ods", ".odp", ".odg"}},
		"openoffice":    {[]string{"okapi/filters/openoffice/src/test/resources", "integration-tests/okapi/src/test/resources/openoffice"}, []string{".odt", ".ods", ".odp", ".odg", ".sxw", ".sxc", ".sxi"}},
		"openxml":       {[]string{"okapi/filters/openxml/src/test/resources"}, []string{".docx", ".xlsx", ".pptx"}},
		"paraplaintext": {[]string{"integration-tests/okapi/src/test/resources/plaintext"}, []string{".txt"}},
		"pdf":           {[]string{"okapi/filters/pdf/src/test/resources"}, []string{".pdf"}},
		"phpcontent":    {[]string{"okapi/filters/php/src/test/resources"}, []string{".phpcnt", ".php"}},
		"plaintext":     {[]string{"integration-tests/okapi/src/test/resources/plaintext"}, []string{".txt"}},
		"po":            {[]string{"integration-tests/okapi/src/test/resources/po"}, []string{".po", ".pot"}},
		"properties":    {[]string{"integration-tests/okapi/src/test/resources/property"}, []string{".properties"}},
		"regex":         {[]string{"okapi/filters/regex/src/test/resources"}, []string{".txt", ".srt"}},
		"rtf":           {[]string{"okapi/filters/rtf/src/test/resources"}, []string{".rtf"}},
		"splicedlines":  {[]string{"okapi/filters/plaintext/src/test/resources"}, []string{".txt"}},
		"srt":           {[]string{"okapi/filters/regex/src/test/resources"}, []string{".srt"}},
		"tex":           {[]string{"integration-tests/okapi/src/test/resources/tex"}, []string{".tex"}},
		"tmx":           {[]string{"integration-tests/okapi/src/test/resources/tmx"}, []string{".tmx"}},
		"transtable":    {[]string{"integration-tests/okapi/src/test/resources/transtable"}, []string{".txt"}},
		"ts":            {[]string{"integration-tests/okapi/src/test/resources/ts"}, []string{".ts"}},
		"tsv":           {[]string{"okapi/filters/table/src/test/resources"}, []string{".tsv", ".txt"}},
		"ttml":          {[]string{"integration-tests/okapi/src/test/resources/ttml"}, []string{".ttml", ".xml"}},
		"ttx":           {[]string{"okapi/filters/ttx/src/test/resources"}, []string{".ttx"}},
		"txml":          {[]string{"okapi/filters/txml/src/test/resources"}, []string{".txml"}},
		"vignette":      {[]string{"okapi/filters/vignette/src/test/resources"}, []string{".vignette", ".xml"}},
		"vtt":           {[]string{"integration-tests/okapi/src/test/resources/vtt"}, []string{".vtt"}},
		"wiki":          {[]string{"integration-tests/okapi/src/test/resources/wikitext"}, []string{".wiki"}},
		"xliff":         {[]string{"integration-tests/okapi/src/test/resources/xliff"}, []string{".xlf", ".xliff"}},
		"xliff2":        {[]string{"integration-tests/okapi/src/test/resources/xliff2"}, []string{".xlf", ".xlf2", ".xliff"}},
		"xml":           {[]string{"integration-tests/okapi/src/test/resources/xml"}, []string{".xml"}},
		"yaml":          {[]string{"integration-tests/okapi/src/test/resources/yaml"}, []string{".yaml", ".yml"}},
	}
}

// discoverNativeFormats walks core/formats/<name>/ in the repo tree to
// enumerate the native format packages. A subdir counts as a format if
// it contains spec.yaml — the project-wide convention for a registered
// format. Returns format IDs (subdir names) sorted alphabetically.
//
// Walking the filesystem (rather than calling into the registry) keeps
// this independent of registration order and free of init-time side
// effects.
func discoverNativeFormats() []string {
	root, err := findRepoRoot()
	if err != nil {
		return nil
	}
	formatsDir := filepath.Join(root, "core", "formats")
	entries, err := os.ReadDir(formatsDir)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if _, err := os.Stat(filepath.Join(formatsDir, e.Name(), "spec.yaml")); err == nil {
			out = append(out, e.Name())
		}
	}
	sort.Strings(out)
	return out
}

// bridgeFilterIDs returns the static bridge-filter ID map used by the
// coverage map. Keys are bare format IDs (matching native subdir names
// when both exist); values are the okf_<id> bridge filter ID.
//
// Single source of truth is cli/parity/formats/spec.go, but importing
// that test package from this test package would create a cycle on the
// parity build tag. The mapping below is hand-maintained instead and
// must be updated when filters are added to the bridge manifest.
//
// "tsv" → okf_tabseparatedvalues, "csv" → okf_commaseparatedvalues:
// these are the two filter IDs that don't follow the okf_<id> pattern;
// every other entry strips the okf_ prefix to get the bare ID.
func bridgeFilterIDs() map[string]string {
	pairs := [][2]string{
		{"archive", "okf_archive"},
		{"csv", "okf_commaseparatedvalues"},
		{"doxygen", "okf_doxygen"},
		{"dtd", "okf_dtd"},
		{"fixedwidth", "okf_fixedwidthcolumns"},
		{"html", "okf_html"},
		{"icml", "okf_icml"},
		{"idml", "okf_idml"},
		{"json", "okf_json"},
		{"markdown", "okf_markdown"},
		{"mif", "okf_mif"},
		{"mosestext", "okf_mosestext"},
		{"odf", "okf_odf"},
		{"openoffice", "okf_openoffice"},
		{"openxml", "okf_openxml"},
		{"paraplaintext", "okf_paraplaintext"},
		{"pdf", "okf_pdf"},
		{"pensieve", "okf_pensieve"},
		{"phpcontent", "okf_phpcontent"},
		{"plaintext", "okf_plaintext"},
		{"po", "okf_po"},
		{"properties", "okf_properties"},
		{"rainbowkit", "okf_rainbowkit"},
		{"regex", "okf_regex"},
		{"rtf", "okf_rtf"},
		{"sdlpackage", "okf_sdlpackage"},
		{"splicedlines", "okf_splicedlines"},
		{"srt", "okf_regex-srt"},
		{"tex", "okf_tex"},
		{"tmx", "okf_tmx"},
		{"transifex", "okf_transifex"},
		{"transtable", "okf_transtable"},
		{"ts", "okf_ts"},
		{"tsv", "okf_tabseparatedvalues"},
		{"ttml", "okf_ttml"},
		{"ttx", "okf_ttx"},
		{"txml", "okf_txml"},
		{"vignette", "okf_vignette"},
		{"vtt", "okf_vtt"},
		{"wiki", "okf_wiki"},
		{"xini", "okf_xini"},
		{"xinirainbowkit", "okf_xinirainbowkit"},
		{"xliff", "okf_xliff"},
		{"xliff2", "okf_xliff2"},
		{"xml", "okf_xml"},
		{"yaml", "okf_yaml"},
	}
	out := make(map[string]string, len(pairs))
	for _, p := range pairs {
		out[p[0]] = p[1]
	}
	return out
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
