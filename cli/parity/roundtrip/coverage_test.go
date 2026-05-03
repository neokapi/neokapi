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

	// writerOverlay is a curated map applied to the native writer's
	// WriterConfig at parity time, used solely to align native output
	// with okapi's defaults. These are NOT format defaults — they live
	// here, next to the parity test, so the "we set this to mimic
	// okapi" intent stays visible. Each entry should carry an inline
	// comment explaining the okapi behavior it mirrors.
	writerOverlay map[string]any

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

	// normalizer, when non-nil, is forwarded to every Case so the
	// canonical-tier comparison can declare semantically-equivalent
	// outputs as TierCanonicalEqual (e.g. case-insensitive \uXXXX
	// escapes, sorted XML attributes, ignored whitespace).
	normalizer roundtrip.Normalizer

	// minTier overrides the per-engine required tier. Default is
	// TierByteEqual for every engine. Use this to grade an engine on
	// "must reach canonical-equal" while still surfacing actual
	// achievement in the report.
	minTier map[string]roundtrip.Tier
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
	companions := discoverCompanions(t, abs)

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
			Bytes:      body,
			Filename:   base,
			Companions: companions,
		},
		IsZip:           scan.isZip,
		ExpectedSkipped: expectedSkipped,
		SkipReason:      reason,
		Normalizer:      scan.normalizer,
		MinTier:         scan.minTier,
	},
		&roundtrip.NativeEngine{
			FormatID:      scan.formatID,
			ReaderConfig:  scan.nativeConfig,
			WriterOverlay: scan.writerOverlay,
		},
		bridge,
		okapi,
	)
}

// discoverCompanions returns sibling files in the input's directory
// that are likely required to parse the input correctly. The heuristic:
// any file whose basename starts with the input's stem (the basename
// without extension) followed by `_` or `.` qualifies. This catches
// patterns like:
//
//   - okf_xml: Translate2.xml needs Translate2_LinkedRules.xml
//   - okf_dtd: foo.dtd needs foo.ent (entity references)
//   - any test fixture: foo.html with foo.css / foo.js companions
//
// Unrelated files in the same directory (e.g. Translate1.xml when the
// input is Translate2.xml) are not picked up because their stem differs.
// The returned map is keyed by basename so engines can re-create the
// directory layout in their tmpDir without touching the source paths.
func discoverCompanions(t *testing.T, inputAbs string) map[string][]byte {
	t.Helper()
	dir := filepath.Dir(inputAbs)
	base := filepath.Base(inputAbs)
	stem := strings.TrimSuffix(base, filepath.Ext(base))
	entries, err := os.ReadDir(dir)
	if err != nil {
		// Directory unreadable is not fatal — the input itself was
		// readable, so just proceed without companions and let the
		// engine surface any "missing referenced file" error.
		return nil
	}
	companions := map[string][]byte{}
	for _, e := range entries {
		if e.IsDir() || e.Name() == base {
			continue
		}
		name := e.Name()
		// Match `<stem>_*` or `<stem>.*` — the second covers e.g.
		// `foo.dtd` ↔ `foo.ent`. Bare prefix (`Translate2something`)
		// is intentionally not matched to keep the heuristic tight.
		if !strings.HasPrefix(name, stem+"_") && !strings.HasPrefix(name, stem+".") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatalf("read companion %q: %v", name, err)
		}
		companions[name] = data
	}
	if len(companions) == 0 {
		return nil
	}
	return companions
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
			formatID:    "plaintext",
			filterClass: "okf_plaintext",
			sources:     []string{"integration-tests/okapi/src/test/resources/plaintext"},
			extensions:  []string{".txt"},
			normalizer:  roundtrip.LFLineEndings{},
			minTier:     map[string]roundtrip.Tier{"native": roundtrip.TierDivergent},
		},
		{
			formatID:      "paraplaintext",
			filterClass:   "okf_paraplaintext",
			explicitFiles: []string{"integration-tests/okapi/src/test/resources/plaintext/test_paragraphs1.txt"},
			normalizer:    roundtrip.LFLineEndings{},
			minTier:       map[string]roundtrip.Tier{"native": roundtrip.TierDivergent},
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
		},
		{
			// Mosestext: upstream ships Test01/Test02 and an XLIFF
			// backref pair; we round-trip the .txt source files.
			formatID:          "mosestext",
			filterClass:       "okf_mosestext",
			sources:           []string{"okapi/filters/mosestext/src/test/resources"},
			extensions:        []string{".txt"},
		},

		// ── HTML / markup ─────────────────────────────────────────
		{
			// Native html writer emits a constant 197-byte stub
			// regardless of input — the merge step doesn't write
			// the target back into the document. Bridge passes ~9
			// of 69 fixtures; the rest are flagged per-file via
			// htmlBridgeSkips() in coverage_skips_test.go.
			formatID:    "html",
			filterClass: "okf_html",
			sources:     []string{"integration-tests/okapi/src/test/resources/html"},
			extensions:  []string{".html"},
			skip:        htmlBridgeSkips(),
			// HTMLCanonical reaches canonical-equal for fixtures whose
			// semantic structure agrees but whose byte form differs in
			// attribute quoting/order, inter-tag whitespace, or
			// transport-meta injection (okapi adds <meta http-equiv>,
			// native preserves source).
			normalizer: roundtrip.HTMLCanonical{},
		},
		{
			// Bridge passes ~14 of 46 fixtures; the rest are
			// flagged per-file via markdownBridgeSkips().
			formatID:          "markdown",
			filterClass:       "okf_markdown",
			sources:           []string{"integration-tests/okapi/src/test/resources/markdown"},
			extensions:        []string{".md"},
			skip:              markdownBridgeSkips(),
		},
		{
			formatID:          "wiki",
			filterClass:       "okf_wiki",
			sources:           []string{"integration-tests/okapi/src/test/resources/wikitext"},
			extensions:        []string{".wiki"},
			normalizer:        roundtrip.IgnoreTrailingNewline{},
		},

		// ── Key-value & structured data ───────────────────────────
		{
			// Bridge passes ~6 of 24 fixtures; the rest are flagged
			// per-file via poBridgeSkips().
			formatID:    "po",
			filterClass: "okf_po",
			sources:     []string{"integration-tests/okapi/src/test/resources/po"},
			extensions:  []string{".po"},
			skip:        poBridgeSkips(),
			// okf_po defaults to useCodeFinder=true with printf-style
			// patterns so `%s`, `%d`, `%1$s`, `{0}`, etc. are extracted
			// as inline codes (Spans) and not pseudo-translated as text.
			// Native po defaults to useCodeFinder=false, so `%s` becomes
			// `%ś` after pseudo. Match okapi by enabling the same rules.
			nativeConfig: map[string]any{
				"useCodeFinder": true,
				"codeFinderRules": []any{
					`%(([-0+#]?)[-0+#]?)((\d\$)?)(\d*)(\.\d+)?[bBhHsScCdoxXeEfgGaAtTn%]`,
					`(\\r\\n)|\\a|\\b|\\f|\\n|\\r|\\t|\\v`,
					`\{\d.*?\}`,
				},
			},
			// PO chain: many fixtures differ only in BOM presence and
			// line-ending choice (okapi preserves source CRLF + BOM,
			// native emits LF + no BOM). Both are valid PO; chain them
			// to reach canonical-equal where those are the only diffs.
			normalizer: roundtrip.Chain{Steps: []roundtrip.Normalizer{
				roundtrip.StripBOM{},
				roundtrip.LFLineEndings{},
				roundtrip.IgnoreTrailingNewline{},
			}},
		},
		{
			formatID:    "properties",
			filterClass: "okf_properties",
			sources:     []string{"integration-tests/okapi/src/test/resources/property"},
			extensions:  []string{".properties"},
			// Skip lifted into observation mode: native runs and the
			// report shows actual achievement, but we don't fail the
			// test until a writer fix or normalizer brings native into
			// canonical-equal. Today the gap is wider than just style
			// (line endings + hex case + extra extracted entries).
			normalizer: roundtrip.Chain{Steps: []roundtrip.Normalizer{
				roundtrip.LFLineEndings{},
				roundtrip.LowerHexUnicodeEscape{},
				roundtrip.IgnoreTrailingNewline{},
			}},
			minTier: map[string]roundtrip.Tier{"native": roundtrip.TierDivergent},
		},
		{
			formatID:          "json",
			filterClass:       "okf_json",
			sources:           []string{"integration-tests/okapi/src/test/resources/json"},
			extensions:        []string{".json"},
			// JSON normalizer reaches canonical-equal when fixtures
			// differ only in whitespace (e.g. `"k" : "v"` vs `"k": "v"`)
			// or string escape choices that encoding/json normalizes.
			normalizer: roundtrip.JSONCanonical{},
		},
		{
			formatID:          "yaml",
			filterClass:       "okf_yaml",
			sources:           []string{"integration-tests/okapi/src/test/resources/yaml"},
			extensions:        []string{".yaml", ".yml"},
			// Okapi's okf_yaml extracts every scalar (including bool /
			// int / null) as translatable text. Native defaults to
			// strings-only — for parity we mirror okapi by extracting
			// non-string scalars too. Without this, native preserves
			// `true` as a boolean while okapi pseudo-translates it to
			// `ţŕũē`, producing a real-looking divergence on every
			// fixture that has booleans/numbers.
			nativeConfig: map[string]any{
				"extractNonStrings": true,
			},
			// YAML normalizer reaches canonical-equal when fixtures
			// differ only in indentation, quote style, or block-vs-flow
			// — both sides round-trip through gopkg.in/yaml.v3.
			normalizer: roundtrip.YAMLCanonical{},
			skip: map[string]fileSkip{
				// snakeyaml recursion fixtures: native YAML reader
				// doesn't bound its alias resolution and loops
				// forever — real native bug worth a fix.
				"beanring-3.yaml":           {Engines: []string{"native"}, Reason: "native YAML reader hangs on self-referencing anchors"},
				"no-children-1.yaml":        {Engines: []string{"native"}, Reason: "native YAML reader hangs on self-referencing anchors"},
				"no-children-2.yaml":        {Engines: []string{"native"}, Reason: "native YAML reader hangs on self-referencing anchors"},
				"scalar_sample.yml":         {Engines: []string{"native"}, Reason: "native YAML reader hangs on self-referencing anchors"},
				"no-children-1-pretty.yaml": {Engines: []string{"okapi"}, Reason: "Okapi YAML parser rejects !!timestamp tag"},
				"Test03.yml":                {Engines: []string{"okapi"}, Reason: "Okapi YAML parser rejects !!timestamp tag"},
			},
		},
		{
			// Phpcontent has no integration-tests dir — fall back to
			// the unit-test resources dir.
			formatID:          "phpcontent",
			filterClass:       "okf_phpcontent",
			sources:           []string{"okapi/filters/php/src/test/resources"},
			extensions:        []string{".phpcnt"},
		},

		// ── Tabular ───────────────────────────────────────────────
		{
			// Bridge passes ~7 of 15 fixtures; the rest are flagged
			// per-file via csvBridgeSkips().
			formatID:    "csv",
			filterClass: "okf_commaseparatedvalues",
			sources:     []string{"integration-tests/okapi/src/test/resources/table"},
			extensions:  []string{".csv"},
			skip:        csvBridgeSkips(),
			// okf_commaseparatedvalues defaults to "translate column 1
			// across all rows including the first" (no header). Native
			// csv defaults to "translate every cell of every data row,
			// header is row 0". The two extraction surfaces overlap
			// only at column 1 of row 1+, so any fixture with content
			// outside that intersection diverges. Mirror okapi's
			// surface for parity.
			nativeConfig: map[string]any{
				"hasHeader":           false,
				"translatableColumns": []any{1},
			},
		},
		{
			// No integration-tests source for tsv; cherry-pick the
			// few real tsv fixtures from the shared table dir.
			formatID:      "tsv",
			filterClass:   "okf_tabseparatedvalues",
			explicitFiles: []string{"okapi/filters/table/src/test/resources/test_tsv_simple.txt"},
			// Same column-vs-row extraction divergence as csv — okapi's
			// okf_tabseparatedvalues defaults to "translate column 1
			// across all rows including the first" while native tsv
			// defaults to "header + everything in data rows". Mirror
			// okapi for parity.
			nativeConfig: map[string]any{
				"hasHeader":           false,
				"translatableColumns": []any{1},
			},
		},
		{
			formatID:    "fixedwidth",
			filterClass: "okf_fixedwidthcolumns",
			explicitFiles: []string{
				"okapi/filters/table/src/test/resources/fwc_test4.txt",
				"okapi/filters/table/src/test/resources/fwc_test5.txt",
			},
		},

		// ── Code/markup ───────────────────────────────────────────
		{
			formatID:          "doxygen",
			filterClass:       "okf_doxygen",
			sources:           []string{"integration-tests/okapi/src/test/resources/doxygen"},
			extensions:        []string{".h", ".py"},
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
			writerOverlay: map[string]any{
				// okapi always emits a `<?xml version="1.0" encoding="UTF-8"?>`
				// prologue even when the source had none; native preserves
				// the source's actual prologue (often nothing).
				"emitDeclaration":     true,
				"declarationEncoding": "UTF-8",
			},
			// XML normalizer to reach canonical-equal when okapi and
			// native produce semantically-identical XML that differs in
			// attribute ordering, namespace prefix, or non-significant
			// whitespace. Both sides go through encoding/xml round-trip
			// so the encoder's namespace mangling cancels.
			normalizer: roundtrip.XMLCanonical{SortAttrs: true},
		},
		{
			// 15 fixtures fail bridge against the in-process okapi
			// reference: segmentation / alt-trans / inline-code
			// handling divergences across various xliff dialects.
			formatID:          "xliff",
			filterClass:       "okf_xliff",
			sources:           []string{"integration-tests/okapi/src/test/resources/xliff"},
			extensions:        []string{".xlf"},
			// XLIFF is XML; sort attrs handles okapi reordering.
			// Collapse text whitespace mirrors okapi's translatable-
			// text normalisation (multi-line indented source becomes
			// single-line trimmed text on round-trip). Strip ns-decls
			// because okapi's writer redeclares the default xmlns at
			// many element depths whereas neokapi declares once at
			// the root — both forms are semantically identical.
			normalizer: roundtrip.XMLCanonical{SortAttrs: true, CollapseTextWhitespace: true, StripNamespaceDecls: true},
			// Reader-side okapi-compat: simulate okapi's broken
			// windows-1252 handling so SF-12-Test03's transcoded chars
			// match okapi's lossy U+FFFD output. See
			// docs/internals/research/xliff-okapi-compat-quirks.md.
			nativeConfig: map[string]any{
				"okapiCompat": map[string]any{
					"simulateBrokenWindows1252Read": true,
				},
			},
			// Writer-side okapi-compat: enable every flag so neokapi's
			// xliff writer reproduces okapi's quirks byte-for-byte for
			// parity. NONE of these are on by default in production —
			// neokapi's defaults follow the XLIFF 1.2 spec and intuitive
			// output choices. See xliff.OkapiCompatConfig and
			// docs/internals/research/xliff-okapi-compat-quirks.md for per-flag
			// rationale, fixture references, and spec citations.
			writerOverlay: map[string]any{
				"okapiCompat": map[string]any{
					// Always-safe flags: these align with okapi's defaults
					// across every fixture we've inspected.
					"lowercaseLangSubtag": true,
					"stripPhaseDateAttr":  true,
					"stripCDataCREntities": true,
					"hoistAltTransNotes":  true,
					"reorderHeaderToolToEnd": true,
					"simulateBrokenWindows1252Read": true,
					// Disabled: okapi PRESERVES `approved="yes"` on
					// trans-units in every fixture inspected (Manual-12,
					// RB-11, SF-12-Test02). Earlier conjecture that okapi
					// strips it was based on a misread fixture. Leave the
					// flag in OkapiCompatConfig for future use but don't
					// enable it here.
					// "stripTransUnitApprovedAttr": true,
					//
					// Disabled: okapi only unwraps single-mrk segmentation
					// in narrow conditions we haven't fully characterized.
					// Blanket-enabling regresses translate_no.xlf
					// (translate="no" trans-units retain their mrk wrapper)
					// and other fixtures. Tracked in
					// docs/internals/research/xliff-okapi-compat-quirks.md.
					// "unwrapSingleSegMrk": true,
					//
					// Disabled: okapi outputs literal UTF-8 for non-ASCII
					// text content (Manual-12-AltTrans `Ţēxţ`, RB-11
					// `Ƥàŕàĝŕàƥĥē`, SF-12-Test02 `Versión`). The flag
					// remains for future use against fixtures where okapi
					// does emit `&#xNNNN;` (still under investigation).
					// "escapeNonASCIIAsEntities": true,
				},
			},
			skip: map[string]fileSkip{
				"lqiTest.xlf":                 {Engines: []string{"okapi"}, Reason: "okf_xliff needs lqiTestIssues.xml in the same dir; harness now copies companions but okapi still rejects this fixture"},
				"ImplementationPlan.docx.xlf": {Engines: []string{"bridge", "native"}, Reason: "bridge inline-code/alt-trans divergence vs okapi reference"},
				"RB-12-Test02.xlf":            {Engines: []string{"bridge", "native"}, Reason: "bridge ResourceBundle-flavour xliff divergence vs okapi reference"},
				"segmentation2.xlf":           {Engines: []string{"bridge", "native"}, Reason: "bridge segmentation handling divergence"},
				"invalid_xml_entity.xlf":      {Engines: []string{"native"}, Reason: "intentionally-malformed fixture: contains `&#x03;` and `&#x1F;` character references that resolve to XML-disallowed C0 control chars; okapi tolerates them, encoding/xml rejects them by spec"},
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
			// TMX is XML; same canonical normalizer.
			normalizer: roundtrip.XMLCanonical{SortAttrs: true},
			skip: map[string]fileSkip{
				"code_fail.tmx":          {Engines: []string{"okapi"}, Reason: "intentionally-malformed test fixture; okf_tmx rejects with 'no <tuv> set to source language'"},
				"code_id_difference.tmx": {Engines: []string{"okapi"}, Reason: "intentionally-malformed test fixture for code-id mismatch detection"},
				"ImportTest2A.tmx":       {Engines: []string{"bridge", "native"}, Reason: "bridge tmx writer XML serialization divergence on this fixture"},
				"ImportTest2B.tmx":       {Engines: []string{"bridge", "native"}, Reason: "bridge tmx writer XML serialization divergence on this fixture"},
				"simple.tmx":             {Engines: []string{"bridge", "native"}, Reason: "bridge tmx writer XML serialization divergence on this fixture"},
			},
		},
		{
			// Bridge passes 1 of 9 fixtures; the rest are flagged
			// per-file via tsBridgeSkips().
			formatID:          "ts",
			filterClass:       "okf_ts",
			sources:           []string{"integration-tests/okapi/src/test/resources/ts"},
			extensions:        []string{".ts"},
			skip:              tsBridgeSkips(),
			// TS is XML (Qt Linguist); same canonical normalizer.
			normalizer: roundtrip.XMLCanonical{SortAttrs: true},
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
			bridgeParams: map[string]string{"mergeCaptions": "false"},
			// VTT is line-oriented; line-ending diffs are the common
			// stylistic mismatch.
			normalizer: roundtrip.LFLineEndings{},
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
			bridgeParams: map[string]string{"mergeCaptions": "false"},
			// TTML is XML; same canonical normalizer.
			normalizer: roundtrip.XMLCanonical{SortAttrs: true},
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
		},
		{
			formatID:          "tex",
			filterClass:       "okf_tex",
			sources:           []string{"integration-tests/okapi/src/test/resources/tex"},
			extensions:        []string{".tex"},
		},
		{
			formatID:          "transtable",
			filterClass:       "okf_transtable",
			sources:           []string{"integration-tests/okapi/src/test/resources/transtable"},
			extensions:        []string{".txt"},
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
			skip:              idmlBridgeSkips(),
		},
		{
			// 5 fixtures crash upstream Okapi's icml merge; 7 more
			// diverge in bridge's inline-rewrite path. Bridge passes
			// 2 of 9 testable fixtures. icmlMergedSkips() returns
			// both buckets in one map.
			formatID:    "icml",
			filterClass: "okf_icml",
			sources:     []string{"integration-tests/okapi/src/test/resources/icml"},
			extensions:  []string{".icml", ".wcml"},
			skip:        icmlMergedSkips(),
			// ICML is Adobe InDesign XML; same canonical normalizer as
			// other XML formats reaches canonical-equal when source and
			// reference differ only in attribute order or non-significant
			// whitespace.
			normalizer: roundtrip.XMLCanonical{SortAttrs: true},
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
			skip:              openxmlBridgeSkips(),
		},
		{
			// Bridge passes ~2 of 41 fixtures; the rest are flagged
			// per-file via mifBridgeSkips().
			formatID:          "mif",
			filterClass:       "okf_mif",
			sources:           []string{"integration-tests/okapi/src/test/resources/mif"},
			extensions:        []string{".mif"},
			skip:              mifBridgeSkips(),
		},
	}
}
