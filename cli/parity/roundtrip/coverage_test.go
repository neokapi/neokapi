//go:build parity

package roundtrip_test

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/cli/parity"
	"github.com/neokapi/neokapi/cli/parity/roundtrip"
	"github.com/neokapi/neokapi/core/registry"
)

// fileSkip records a known divergence (or okapi-unsupported case) for
// one upstream fixture. Engines lists which engines disagree with the
// okapi engine's reference. The special engine name "okapi" means the
// okapi reference engine can't produce a usable output for this file,
// so the whole sub-test skips. Reason is a one-line note shown in test
// output — write what the divergence actually is, not just "broken".
type fileSkip struct {
	Engines []string
	Reason  string
}

// formatScan describes how to discover the upstream Okapi fixtures
// for one format and how to drive each engine against them. Mirrors
// what the corresponding upstream RoundTrip<X>IT.java exercises: every
// file with a matching extension under the listed source roots becomes
// one sub-test.
type formatScan struct {
	formatID registry.FormatID

	// filterClass is the upstream Okapi filter ID (e.g. "okf_html").
	// Used by both the okapi reference engine and the bridge engine.
	// Empty disables the bridge engine for this format; the okapi
	// engine still uses it as the comparator and so requires it.
	filterClass string

	// sources lists tarball-relative directories to scan. For most
	// formats this is integration-tests/okapi/src/test/resources/<format>;
	// some formats whose integration-tests dir is empty fall back to
	// okapi/filters/<filter>/src/test/resources.
	sources    []string
	extensions []string // case-insensitive (".json", ".xml", ...)
	recurse    bool     // when true, walk subdirectories too

	// explicitFiles overrides discovery — used when a format shares its
	// resource directory with sibling formats and we need to cherry-pick
	// (e.g. splicedlines fixtures live under plaintext/ and would
	// otherwise be picked up by both formats).
	explicitFiles []string

	isZip        bool
	nativeConfig map[string]any
	bridgeParams map[string]string

	// okapiParamConfig is forwarded to OkapiEngine as raw .fprm
	// content (e.g. "#v1\nmergeCaptions.b=false\n"). Used by VTT/TTML
	// to disable Okapi's caption-merging on round-trip.
	okapiParamConfig string

	// formatDefaultSkip applies to every discovered file in this
	// scan: use it when a real engine bug affects the whole format
	// (e.g. native's CRLF preservation in srt, bridge's read-only
	// xliff2). Per-file skip entries below extend this set.
	formatDefaultSkip fileSkip

	// skip records known divergences keyed by file basename. These
	// extend formatDefaultSkip with per-file engines/reasons.
	skip map[string]fileSkip
}

func TestRoundTrip_Coverage(t *testing.T) {
	s := parity.RequireSandbox(t)
	if s.OkapiTestDataDir == "" {
		t.Fatal("okapi test resources tarball not present in sandbox — run `make parity-test` from repo root")
	}
	for _, scan := range coverageScans() {
		scan := scan
		t.Run(string(scan.formatID), func(t *testing.T) {
			files := discoverFiles(t, s.OkapiTestDataDir, scan)
			if len(files) == 0 {
				t.Fatalf("format %q: no upstream fixtures discovered (sources=%v ext=%v) — typo or empty upstream dir?",
					scan.formatID, scan.sources, scan.extensions)
			}
			for _, f := range files {
				f := f
				t.Run(filepath.Base(f), func(t *testing.T) {
					runOneFixture(t, scan, f)
				})
			}
		})
	}
}

// discoverFiles returns absolute paths to every upstream fixture this
// scan should drive: explicitFiles when set, otherwise every file
// under any source directory whose extension matches.
func discoverFiles(t *testing.T, root string, scan formatScan) []string {
	t.Helper()
	if len(scan.explicitFiles) > 0 {
		out := make([]string, 0, len(scan.explicitFiles))
		for _, rel := range scan.explicitFiles {
			abs := filepath.Join(root, rel)
			if _, err := os.Stat(abs); err != nil {
				t.Fatalf("explicit fixture %q not found: %v", rel, err)
			}
			out = append(out, abs)
		}
		sort.Strings(out)
		return out
	}
	extSet := map[string]bool{}
	for _, e := range scan.extensions {
		extSet[strings.ToLower(e)] = true
	}
	var out []string
	for _, src := range scan.sources {
		srcAbs := filepath.Join(root, src)
		info, err := os.Stat(srcAbs)
		if err != nil || !info.IsDir() {
			t.Fatalf("source dir %q missing or not a dir: %v", src, err)
		}
		walkErr := filepath.WalkDir(srcAbs, func(path string, d os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if d.IsDir() {
				if !scan.recurse && path != srcAbs {
					return filepath.SkipDir
				}
				return nil
			}
			if extSet[strings.ToLower(filepath.Ext(path))] {
				out = append(out, path)
			}
			return nil
		})
		if walkErr != nil {
			t.Fatalf("walk %q: %v", src, walkErr)
		}
	}
	sort.Strings(out)
	return out
}

func runOneFixture(t *testing.T, scan formatScan, abs string) {
	t.Helper()
	base := filepath.Base(abs)
	// Merge format-default + per-file skip. Per-file engines extend the
	// default set; the per-file reason wins when set, otherwise default.
	skipSet := map[string]bool{}
	reason := scan.formatDefaultSkip.Reason
	for _, e := range scan.formatDefaultSkip.Engines {
		skipSet[e] = true
	}
	if perFile, ok := scan.skip[base]; ok {
		for _, e := range perFile.Engines {
			skipSet[e] = true
		}
		if perFile.Reason != "" {
			reason = perFile.Reason
		}
	}
	if skipSet["okapi"] {
		t.Skipf("okapi reference engine cannot process %s: %s", base, reason)
	}

	body, err := os.ReadFile(abs)
	if err != nil {
		t.Fatalf("read fixture %q: %v", abs, err)
	}

	var bridge *roundtrip.BridgeEngine
	if scan.filterClass != "" {
		bridge = &roundtrip.BridgeEngine{
			FilterClass:  scan.filterClass,
			FilterParams: scan.bridgeParams,
		}
	}
	okapi := &roundtrip.OkapiEngine{
		FilterClass: scan.filterClass,
		ParamConfig: scan.okapiParamConfig,
	}

	var expectedSkipped []string
	for e := range skipSet {
		if e != "okapi" {
			expectedSkipped = append(expectedSkipped, e)
		}
	}

	roundtrip.RunThreeWay(t, roundtrip.Case{
		Name:     base,
		FormatID: scan.formatID,
		Input: roundtrip.Input{
			Bytes:    body,
			Filename: base,
		},
		IsZip:           scan.isZip,
		ExpectedSkipped: expectedSkipped,
	},
		&roundtrip.NativeEngine{
			FormatID:     scan.formatID,
			ReaderConfig: scan.nativeConfig,
		},
		bridge,
		okapi,
	)
}

// coverageScans defines one scan per format. The set mirrors what the
// upstream Okapi RoundTrip<X>IT.java suite covers: same directories,
// same files. Formats with no upstream roundtrip suite (jsx, klf,
// versifiedtext, messageformat — neokapi-only) and formats where the
// upstream Okapi pipeline itself can't produce a reference (txml NPE
// on merge, rtf has only tradosrtf, epub has no okf_epub) are
// intentionally omitted; their neokapi-side correctness is covered by
// per-format unit tests under core/formats/<x>/.
//
// First-pass note: per-file `skip` entries get filled in iteratively
// after running the suite and triaging divergences. Each entry needs a
// one-line reason describing what actually diverges, so the next reader
// can tell engine bug vs harness limitation.
func coverageScans() []formatScan {
	return []formatScan{
		// ── Plain-text family ─────────────────────────────────────
		{
			// Upstream RoundTripPlainTextIT loops over /plaintext/
			// (which also contains the splicedlines and paraplaintext
			// fixtures — they get their own scans below).
			formatID:          "plaintext",
			filterClass:       "okf_plaintext",
			sources:           []string{"integration-tests/okapi/src/test/resources/plaintext"},
			extensions:        []string{".txt"},
			formatDefaultSkip: fileSkip{Engines: []string{"native"}, Reason: "native plaintext writer drops trailing newlines and mishandles BOM/line-ending preservation vs okapi"},
		},
		{
			formatID:          "paraplaintext",
			filterClass:       "okf_paraplaintext",
			explicitFiles:     []string{"integration-tests/okapi/src/test/resources/plaintext/test_paragraphs1.txt"},
			formatDefaultSkip: fileSkip{Engines: []string{"native"}, Reason: "native paraplaintext writer trailing-newline divergence vs okapi"},
		},
		{
			// Splicedlines is an okf_plaintext config variant exposed
			// in the bridge as `okf_splicedlines` (default config is
			// the backslash splicer, matching the canonical fixtures).
			formatID:    "splicedlines",
			filterClass: "okf_splicedlines",
			explicitFiles: []string{
				"okapi/filters/plaintext/src/test/resources/combined_lines.txt",
				"okapi/filters/plaintext/src/test/resources/combined_lines_end.txt",
				"okapi/filters/plaintext/src/test/resources/combined_lines2.txt",
			},
			formatDefaultSkip: fileSkip{Engines: []string{"native", "bridge"}, Reason: "native + bridge splicedlines writers byte-shape diverge from okapi"},
		},
		{
			// Mosestext: upstream ships Test01/Test02 and an XLIFF
			// backref pair; we round-trip the .txt source files.
			formatID:          "mosestext",
			filterClass:       "okf_mosestext",
			sources:           []string{"okapi/filters/mosestext/src/test/resources"},
			extensions:        []string{".txt"},
			formatDefaultSkip: fileSkip{Engines: []string{"native"}, Reason: "native mosestext writer flattens inline <mrk>/<seg> markup differently than okapi"},
		},

		// ── HTML / markup ─────────────────────────────────────────
		{
			// Native html writer emits a constant 197-byte stub
			// regardless of input — the merge step doesn't write
			// the target back into the document. Bridge: ~9 of 69
			// fixtures pass; the rest diverge in inline-code marker
			// emission against the in-process okapi reference.
			formatID:          "html",
			filterClass:       "okf_html",
			sources:           []string{"integration-tests/okapi/src/test/resources/html"},
			extensions:        []string{".html"},
			formatDefaultSkip: fileSkip{Engines: []string{"native", "bridge"}, Reason: "native html writer emits a fixed stub on merge; bridge inline-code marker emission diverges from okapi reference for most fixtures"},
		},
		{
			formatID:          "markdown",
			filterClass:       "okf_markdown",
			sources:           []string{"integration-tests/okapi/src/test/resources/markdown"},
			extensions:        []string{".md"},
			formatDefaultSkip: fileSkip{Engines: []string{"native", "bridge"}, Reason: "native + bridge markdown writers byte-shape diverge from okapi (paragraph/inline-code re-emission)"},
		},
		{
			formatID:          "wiki",
			filterClass:       "okf_wiki",
			sources:           []string{"integration-tests/okapi/src/test/resources/wikitext"},
			extensions:        []string{".wiki"},
			formatDefaultSkip: fileSkip{Engines: []string{"native"}, Reason: "native wiki writer trailing newline + byte-shape divergence"},
			skip: map[string]fileSkip{
				"dokuwiki.wiki":  {Engines: []string{"bridge", "native"}, Reason: "bridge merges adjacent wiki lines into a single block — segmentation divergence"},
				"mediawiki.wiki": {Engines: []string{"bridge", "native"}, Reason: "bridge segments mediawiki blocks differently than okapi"},
			},
		},

		// ── Key-value & structured data ───────────────────────────
		{
			formatID:          "po",
			filterClass:       "okf_po",
			sources:           []string{"integration-tests/okapi/src/test/resources/po"},
			extensions:        []string{".po"},
			formatDefaultSkip: fileSkip{Engines: []string{"native", "bridge"}, Reason: "native + bridge po writers msgstr/quoting/multiline divergence from okapi"},
		},
		{
			formatID:          "properties",
			filterClass:       "okf_properties",
			sources:           []string{"integration-tests/okapi/src/test/resources/property"},
			extensions:        []string{".properties"},
			formatDefaultSkip: fileSkip{Engines: []string{"native"}, Reason: "native properties writer emits uppercase \\uXXXX escapes; tikal uses lowercase"},
			skip: map[string]fileSkip{
				"Test01.properties": {Engines: []string{"bridge", "native"}, Reason: "bridge byte-shape divergence in addition to native escape-case bug"},
				"Test05.properties": {Engines: []string{"bridge", "native"}, Reason: "bridge byte-shape divergence on merge"},
			},
		},
		{
			formatID:          "json",
			filterClass:       "okf_json",
			sources:           []string{"integration-tests/okapi/src/test/resources/json"},
			extensions:        []string{".json"},
			formatDefaultSkip: fileSkip{Engines: []string{"native"}, Reason: "native json writer string-escape (\\uXXXX vs UTF-8) and whitespace divergence on merge"},
		},
		{
			formatID:          "yaml",
			filterClass:       "okf_yaml",
			sources:           []string{"integration-tests/okapi/src/test/resources/yaml"},
			extensions:        []string{".yaml", ".yml"},
			formatDefaultSkip: fileSkip{Engines: []string{"native"}, Reason: "native yaml writer doesn't preserve quoting style or flow/block layout — many byte-shape divergences"},
			skip: map[string]fileSkip{
				// snakeyaml recursion fixtures: native YAML reader
				// doesn't bound its alias resolution and loops
				// forever — real native bug worth a fix.
				"beanring-3.yaml":           {Engines: []string{"native"}, Reason: "native YAML reader hangs on self-referencing anchors"},
				"no-children-1.yaml":        {Engines: []string{"native"}, Reason: "native YAML reader hangs on self-referencing anchors"},
				"no-children-2.yaml":        {Engines: []string{"native"}, Reason: "native YAML reader hangs on self-referencing anchors"},
				"scalar_sample.yml":         {Engines: []string{"bridge", "native"}, Reason: "native hangs on self-ref anchors; bridge byte-shape divergence on merge"},
				"no-children-1-pretty.yaml": {Engines: []string{"okapi"}, Reason: "Okapi YAML parser rejects !!timestamp tag"},
				"Test03.yml":                {Engines: []string{"okapi"}, Reason: "Okapi YAML parser rejects !!timestamp tag"},
				// Bridge byte-shape divergences (folded/literal scalar
				// re-emission, attribute ordering, etc.).
				"en.yml":                      {Engines: []string{"bridge", "native"}, Reason: "bridge yaml writer byte-shape divergence on merge"},
				"en (1).yml":                  {Engines: []string{"bridge", "native"}, Reason: "bridge yaml writer byte-shape divergence on merge"},
				"en (2).yml":                  {Engines: []string{"bridge", "native"}, Reason: "bridge yaml writer byte-shape divergence on merge"},
				"en (3).yml":                  {Engines: []string{"bridge", "native"}, Reason: "bridge yaml writer byte-shape divergence on merge"},
				"en (5).yml":                  {Engines: []string{"bridge", "native"}, Reason: "bridge yaml writer byte-shape divergence on merge"},
				"folded_indented.yml":         {Engines: []string{"bridge", "native"}, Reason: "bridge folded-scalar re-emission divergence"},
				"folded_literal_examples.yml": {Engines: []string{"bridge", "native"}, Reason: "bridge folded/literal-scalar re-emission divergence (large gap)"},
				"literal.yml":                 {Engines: []string{"bridge", "native"}, Reason: "bridge literal-scalar re-emission divergence"},
			},
		},
		{
			// Phpcontent has no integration-tests dir — fall back to
			// the unit-test resources dir.
			formatID:          "phpcontent",
			filterClass:       "okf_phpcontent",
			sources:           []string{"okapi/filters/php/src/test/resources"},
			extensions:        []string{".phpcnt"},
			formatDefaultSkip: fileSkip{Engines: []string{"native"}, Reason: "native phpcontent writer byte-shape divergence on merge"},
		},

		// ── Tabular ───────────────────────────────────────────────
		{
			formatID:          "csv",
			filterClass:       "okf_commaseparatedvalues",
			sources:           []string{"integration-tests/okapi/src/test/resources/table"},
			extensions:        []string{".csv"},
			formatDefaultSkip: fileSkip{Engines: []string{"native", "bridge"}, Reason: "native + bridge csv writers table-semantics divergence (header/quoting/row-vs-cell) from okapi"},
		},
		{
			// No integration-tests source for tsv; cherry-pick the
			// few real tsv fixtures from the shared table dir.
			formatID:          "tsv",
			filterClass:       "okf_tabseparatedvalues",
			explicitFiles:     []string{"okapi/filters/table/src/test/resources/test_tsv_simple.txt"},
			formatDefaultSkip: fileSkip{Engines: []string{"native", "bridge"}, Reason: "native + bridge tsv writers table-semantics divergence (same shape as csv)"},
		},
		{
			formatID:    "fixedwidth",
			filterClass: "okf_fixedwidthcolumns",
			explicitFiles: []string{
				"okapi/filters/table/src/test/resources/fwc_test4.txt",
				"okapi/filters/table/src/test/resources/fwc_test5.txt",
			},
			formatDefaultSkip: fileSkip{Engines: []string{"native"}, Reason: "native fixedwidth writer column-spec/padding divergence"},
		},

		// ── Code/markup ───────────────────────────────────────────
		{
			formatID:          "doxygen",
			filterClass:       "okf_doxygen",
			sources:           []string{"integration-tests/okapi/src/test/resources/doxygen"},
			extensions:        []string{".h", ".py"},
			formatDefaultSkip: fileSkip{Engines: []string{"native", "bridge"}, Reason: "native + bridge doxygen writers comment-block tokenization divergence (@brief inline placeholders)"},
		},

		// ── XML / bilingual exchange formats ──────────────────────
		{
			// okf_xml is the ITS-based XML filter (in
			// okapi/filters/its/...). 8 fixtures fail bridge due to
			// inline-code marker emission diverging from the
			// in-process okapi reference.
			formatID:          "xml",
			filterClass:       "okf_xml",
			sources:           []string{"integration-tests/okapi/src/test/resources/xml"},
			extensions:        []string{".xml"},
			formatDefaultSkip: fileSkip{Engines: []string{"native"}, Reason: "native XML reader needs explicit ITS rules; default config extracts whole document as one block"},
			skip: map[string]fileSkip{
				"Translate2.xml":       {Engines: []string{"okapi"}, Reason: "okf_xml needs Translate2_LinkedRules.xml in the same dir; harness copies the input file alone"},
				"TestMultiLang.xml":    {Engines: []string{"bridge", "native"}, Reason: "bridge inline-code marker emission diverges from okapi reference"},
				"Translate1.xml":       {Engines: []string{"bridge", "native"}, Reason: "bridge inline-code marker emission diverges from okapi reference"},
				"strings.xml":          {Engines: []string{"bridge", "native"}, Reason: "bridge inline-code marker emission diverges from okapi reference"},
				"test01.xml":           {Engines: []string{"bridge", "native"}, Reason: "bridge inline-code marker emission diverges from okapi reference"},
				"test02.xml":           {Engines: []string{"bridge", "native"}, Reason: "bridge inline-code marker emission diverges from okapi reference"},
				"test03.xml":           {Engines: []string{"bridge", "native"}, Reason: "bridge inline-code marker emission diverges from okapi reference"},
				"test04.xml":           {Engines: []string{"bridge", "native"}, Reason: "bridge inline-code marker emission diverges from okapi reference"},
				"test08_utf8nobom.xml": {Engines: []string{"bridge", "native"}, Reason: "bridge inline-code marker emission diverges from okapi reference"},
			},
		},
		{
			// 15 fixtures fail bridge against the in-process okapi
			// reference: segmentation / alt-trans / inline-code
			// handling divergences across various xliff dialects.
			formatID:          "xliff",
			filterClass:       "okf_xliff",
			sources:           []string{"integration-tests/okapi/src/test/resources/xliff"},
			extensions:        []string{".xlf"},
			formatDefaultSkip: fileSkip{Engines: []string{"native"}, Reason: "native xliff writer adds extra xmlns declaration on roundtrip (Go encoding/xml shape)"},
			skip: map[string]fileSkip{
				"lqiTest.xlf":                 {Engines: []string{"okapi"}, Reason: "okf_xliff needs lqiTestIssues.xml in the same dir; harness copies the input file alone"},
				"ImplementationPlan.docx.xlf": {Engines: []string{"bridge", "native"}, Reason: "bridge inline-code/alt-trans divergence vs okapi reference"},
				"JMP-11-Test01.xlf":           {Engines: []string{"bridge", "native"}, Reason: "bridge byte-shape divergence on JMP xliff dialect"},
				"MQ-12-Test01.xlf":            {Engines: []string{"bridge", "native"}, Reason: "bridge MemoQ-flavour xliff divergence vs okapi reference"},
				"Manual-12-AltTrans.xlf":      {Engines: []string{"bridge", "native"}, Reason: "bridge alt-trans handling divergence vs okapi reference"},
				"PAS-10-Test01.xlf":           {Engines: []string{"bridge", "native"}, Reason: "bridge Passolo-flavour xliff divergence vs okapi reference"},
				"RB-11-Test01.xlf":            {Engines: []string{"bridge", "native"}, Reason: "bridge ResourceBundle-flavour xliff divergence vs okapi reference"},
				"RB-12-Test02.xlf":            {Engines: []string{"bridge", "native"}, Reason: "bridge ResourceBundle-flavour xliff divergence vs okapi reference"},
				"SF-12-Test03.xlf":            {Engines: []string{"bridge", "native"}, Reason: "bridge SDLFiletype divergence vs okapi reference"},
				"Xslt-Test01.xlf":             {Engines: []string{"bridge", "native"}, Reason: "bridge xslt-derived xliff divergence vs okapi reference"},
				"addingelements.xlf":          {Engines: []string{"bridge", "native"}, Reason: "bridge segmentation handling divergence — emits target before xliff extension elements"},
				"generalstructure.xlf":        {Engines: []string{"bridge", "native"}, Reason: "bridge byte-shape divergence on general-structure xliff fixture"},
				"invalid_xml_entity.xlf":      {Engines: []string{"bridge", "native"}, Reason: "bridge entity-handling divergence on intentionally-invalid fixture"},
				"lqiExtensions.xlf":           {Engines: []string{"bridge", "native"}, Reason: "bridge LQI extension handling divergence"},
				"mq-12-Test01-small.xlf":      {Engines: []string{"bridge", "native"}, Reason: "bridge MemoQ-flavour xliff divergence vs okapi reference"},
				"segmentation2.xlf":           {Engines: []string{"bridge", "native"}, Reason: "bridge segmentation handling divergence"},
			},
		},
		{
			// Bridge daemon reports "filter does not support writing:
			// okf_xliff2" — the upstream filter is read-only. The okapi
			// reference engine hits the same wall (FilterEventsToRawDocumentStep
			// requires an IFilterWriter), so the whole format is a known skip
			// at the okapi level.
			formatID:          "xliff2",
			filterClass:       "okf_xliff2",
			sources:           []string{"integration-tests/okapi/src/test/resources/xliff2"},
			extensions:        []string{".xlf", ".xlf2"},
			formatDefaultSkip: fileSkip{Engines: []string{"okapi"}, Reason: "okf_xliff2 filter is read-only; FilterEventsToRawDocumentStep cannot write it"},
		},
		{
			formatID:          "tmx",
			filterClass:       "okf_tmx",
			sources:           []string{"integration-tests/okapi/src/test/resources/tmx"},
			extensions:        []string{".tmx"},
			formatDefaultSkip: fileSkip{Engines: []string{"native", "bridge"}, Reason: "native + bridge tmx writers XML serialization shape divergence from okapi (declaration spacing, attribute order)"},
			skip: map[string]fileSkip{
				"code_fail.tmx":          {Engines: []string{"okapi"}, Reason: "intentionally-malformed test fixture; okf_tmx rejects with 'no <tuv> set to source language'"},
				"code_id_difference.tmx": {Engines: []string{"okapi"}, Reason: "intentionally-malformed test fixture for code-id mismatch detection"},
			},
		},
		{
			formatID:          "ts",
			filterClass:       "okf_ts",
			sources:           []string{"integration-tests/okapi/src/test/resources/ts"},
			extensions:        []string{".ts"},
			formatDefaultSkip: fileSkip{Engines: []string{"native", "bridge"}, Reason: "native ts writer emits <!DOCTYPE TS> without empty internal subset; bridge byte-shape divergence on most fixtures"},
		},

		// ── Subtitle / timed-text ─────────────────────────────────
		{
			// Default mergeCaptions=true mutates timestamps and
			// splits merged target text across cues; both engines
			// want the same override.
			formatID:   "vtt",
			filterClass: "okf_vtt",
			sources:    []string{"integration-tests/okapi/src/test/resources/vtt"},
			extensions: []string{".vtt"},
			okapiParamConfig: `#v1
timeFormat=HH:mm:ss.SSS
maxLinesPerCaption.i=2
maxCharsPerLine.i=47
cjkCharsPerLine.i=18
mergeCaptions.b=false
`,
			bridgeParams:      map[string]string{"mergeCaptions": "false"},
			formatDefaultSkip: fileSkip{Engines: []string{"native"}, Reason: "native vtt writer caption-layout/newline divergence"},
		},
		{
			// Same mergeCaptions story as VTT.
			formatID:   "ttml",
			filterClass: "okf_ttml",
			sources:    []string{"integration-tests/okapi/src/test/resources/ttml"},
			extensions: []string{".ttml"},
			okapiParamConfig: `#v1
timeFormat=HH:mm:ss.SSS
maxLinesPerCaption.i=2
maxCharsPerLine.i=47
cjkCharsPerLine.i=18
mergeCaptions.b=false
`,
			bridgeParams:      map[string]string{"mergeCaptions": "false"},
			formatDefaultSkip: fileSkip{Engines: []string{"native"}, Reason: "native ttml writer doesn't preserve <br/> caption layout"},
		},
		// srt: omitted from this scan — upstream tikal routes .srt via
		// `okf_regex-srt`, which loads regex rules from a packaged .fprm
		// resource. The bridge exposes `okf_regex` but the SRT-specific
		// rules need to be loaded as a sizable .fprm before round-tripping
		// works. Wire that in when there's a real signal worth catching.

		// ── Misc text formats ─────────────────────────────────────
		{
			// /dtd/ exists in integration-tests but is empty; fall
			// back to the unit-test resources where Test01/Test02 live.
			formatID:          "dtd",
			filterClass:       "okf_dtd",
			sources:           []string{"okapi/filters/dtd/src/test/resources"},
			extensions:        []string{".dtd"},
			formatDefaultSkip: fileSkip{Engines: []string{"native"}, Reason: "native dtd writer emits an extra leading newline before the first entity"},
			skip: map[string]fileSkip{
				"Test01.dtd": {Engines: []string{"bridge", "native"}, Reason: "bridge XML-escapes placeholder markers tikal emits; native still has the leading-newline bug"},
			},
		},
		{
			formatID:          "tex",
			filterClass:       "okf_tex",
			sources:           []string{"integration-tests/okapi/src/test/resources/tex"},
			extensions:        []string{".tex"},
			formatDefaultSkip: fileSkip{Engines: []string{"native", "bridge"}, Reason: "native tex writer drops preamble/postamble on merge; bridge merges paragraph blocks differently than okapi"},
		},
		{
			formatID:          "transtable",
			filterClass:       "okf_transtable",
			sources:           []string{"integration-tests/okapi/src/test/resources/transtable"},
			extensions:        []string{".txt"},
			formatDefaultSkip: fileSkip{Engines: []string{"native"}, Reason: "native transtable writer drops the TransTableV1 header and okpCtx columns on merge"},
		},

		// ── Binary / compound formats ─────────────────────────────
		{
			// Upstream filter dir has the curated round-trip set
			// (~70 .idml fixtures). Bridge passes ~24 of 70; the
			// remaining 46 are skipped per-file via idmlBridgeSkips()
			// (kept in coverage_skips_test.go to keep this file readable).
			formatID:          "idml",
			filterClass:       "okf_idml",
			sources:           []string{"okapi/filters/idml/src/test/resources"},
			extensions:        []string{".idml"},
			isZip:             true,
			formatDefaultSkip: fileSkip{Engines: []string{"native"}, Reason: "native idml writer doesn't merge target back into the .idml XML"},
			skip:              idmlBridgeSkips(),
		},
		{
			// Several files crash upstream Okapi's icml merge —
			// mark per-file.
			formatID:          "icml",
			filterClass:       "okf_icml",
			sources:           []string{"integration-tests/okapi/src/test/resources/icml"},
			extensions:        []string{".icml", ".wcml"},
			formatDefaultSkip: fileSkip{Engines: []string{"native", "bridge"}, Reason: "native icml writer XML-declaration shape divergence; bridge emits different inline run codes for most fixtures"},
			skip: map[string]fileSkip{
				"OpenofficeFootnoteTest.icml":                                {Engines: []string{"okapi"}, Reason: "upstream Okapi icml merge crashes on this fixture"},
				"TakeItNoItsYoursReallyTheExcellentInevitabilityOfFree.icml": {Engines: []string{"okapi"}, Reason: "upstream Okapi icml merge crashes on this fixture"},
				"TestArticle.icml":                                           {Engines: []string{"okapi"}, Reason: "upstream Okapi icml merge crashes on this fixture"},
				"ThreeParagraphFootnoteTest.icml":                            {Engines: []string{"okapi"}, Reason: "upstream Okapi icml merge crashes on this fixture"},
				"WordFootnoteTest.icml":                                      {Engines: []string{"okapi"}, Reason: "upstream Okapi icml merge crashes on this fixture"},
			},
		},
		{
			// 185 .docx fixtures in the upstream filter dir. Bridge
			// passes ~61 of 185; the other 124 are skipped per-file
			// via openxmlBridgeSkips() (in coverage_skips_test.go).
			formatID:          "openxml",
			filterClass:       "okf_openxml",
			sources:           []string{"okapi/filters/openxml/src/test/resources"},
			extensions:        []string{".docx"},
			isZip:             true,
			formatDefaultSkip: fileSkip{Engines: []string{"native"}, Reason: "native openxml writer skips target text on merge — output keeps original English"},
			skip:              openxmlBridgeSkips(),
		},
		{
			formatID:          "mif",
			filterClass:       "okf_mif",
			sources:           []string{"integration-tests/okapi/src/test/resources/mif"},
			extensions:        []string{".mif"},
			formatDefaultSkip: fileSkip{Engines: []string{"native", "bridge"}, Reason: "native mif writer produces output its reader can't re-extract; bridge picks up only some blocks"},
		},
	}
}
