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

	// bridgeForcePseudoSourceBase mirrors NativeEngine's xliff2 special
	// case: okapi unconditionally pseudo-translates source rather than
	// existing target. Set true for filters whose okapi reference
	// behavior overwrites the on-disk target verbatim.
	bridgeForcePseudoSourceBase bool

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
	// xliff2). Per-fixture engine skips are declared in
	// core/formats/<format>/parity-annotations.yaml and resolved at
	// runtime via roundtrip.LookupSkip — see runOneFixture.
	formatDefaultSkip fileSkip

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

			// Pre-batch the okapi reference for all non-skipped fixtures of
			// this format. One JVM cold-start per format instead of per
			// fixture — that's the load-bearing fix for the parity CI
			// timeout. Per-fixture OkapiEngine.RoundTrip then becomes a
			// cache lookup. Skips are honored: fixtures the okapi engine
			// cannot process are excluded from the batch.
			batchInputs := make([]string, 0, len(files))
			for _, f := range files {
				if okapiSkippedForFixture(scan, filepath.Base(f)) {
					continue
				}
				batchInputs = append(batchInputs, f)
			}
			okapiCache := roundtrip.RunOkapiBatch(t, scan.filterClass, scan.okapiParamConfig, "", "", batchInputs)

			for _, f := range files {
				f := f
				t.Run(filepath.Base(f), func(t *testing.T) {
					runOneFixture(t, scan, f, okapiCache)
				})
			}
		})
	}
}

// okapiSkippedForFixture is true when the formatScan's per-fixture or
// format-default skip list excludes the okapi reference engine for this
// fixture. Used to keep the pre-batch lean — there's no point shelling
// out to the bridge for a fixture we'd skip anyway, and some skips
// exist specifically because the okapi engine errors on that file.
func okapiSkippedForFixture(scan formatScan, base string) bool {
	for _, e := range scan.formatDefaultSkip.Engines {
		if e == "okapi" {
			return true
		}
	}
	if perFile, ok := roundtrip.LookupSkip(string(scan.formatID), base); ok {
		for _, e := range perFile.Engines {
			if e == "okapi" {
				return true
			}
		}
	}
	return false
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

func runOneFixture(t *testing.T, scan formatScan, abs string, okapiCache *roundtrip.OkapiBatchCache) {
	t.Helper()
	base := filepath.Base(abs)
	// Merge format-default + per-file skip. Per-file engines extend the
	// default set; the per-file reason wins when set, otherwise default.
	// Per-file skip is loaded from core/formats/<format>/parity-annotations.yaml
	// via the annotation system — the legacy in-code fileSkip map was
	// migrated there so the dashboard sees the same source of truth.
	skipSet := map[string]bool{}
	reason := scan.formatDefaultSkip.Reason
	for _, e := range scan.formatDefaultSkip.Engines {
		skipSet[e] = true
	}
	if perFile, ok := roundtrip.LookupSkip(string(scan.formatID), base); ok {
		for _, e := range perFile.Engines {
			skipSet[e] = true
		}
		if perFile.Reason != "" {
			reason = perFile.Reason
		}
	}
	if skipSet["okapi"] {
		// Record a Skipped parityRecord so the coverage map sees this
		// fixture exists and is part of the scan, even though okapi
		// can't process it. Without this the format gets misclassified
		// as scan-missing or no-upstream when really we DO have a scan
		// — it just produces no comparable output.
		roundtrip.RecordOkapiSkip(string(scan.formatID), base, reason)
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
			FilterClass:           scan.filterClass,
			FilterParams:          scan.bridgeParams,
			ForcePseudoSourceBase: scan.bridgeForcePseudoSourceBase,
		}
	}
	okapi := &roundtrip.OkapiEngine{
		FilterClass: scan.filterClass,
		ParamConfig: scan.okapiParamConfig,
		BatchCache:  okapiCache,
		InputPath:   abs,
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
			//
			// useCodeFinder + the rules below carve XML-style inline
			// markup (`<mrk mtype="seg">`, `<lb/>`, `</mrk>`) and named
			// entity references (`&lt;`, `&amp;`) out of the source as
			// placeholder runs so they round-trip as literal codes
			// instead of being pseudo-translated character by character.
			// okapi's mosestext filter recognises the same constructs as
			// inline codes by default; this is the parity-equivalent
			// config on the native side.
			formatID:    "mosestext",
			filterClass: "okf_mosestext",
			sources:     []string{"okapi/filters/mosestext/src/test/resources"},
			extensions:  []string{".txt"},
			nativeConfig: map[string]any{
				"useCodeFinder": true,
				"codeFinderRules": []any{
					`<[^>]+>`,
					`&[a-zA-Z][a-zA-Z0-9]*;`,
				},
			},
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
			formatID:    "markdown",
			filterClass: "okf_markdown",
			sources:     []string{"integration-tests/okapi/src/test/resources/markdown"},
			extensions:  []string{".md"},
			// okf_markdown's Java reset() defaults translateCodeBlocks=true
			// and translateIndentedCodeBlocks=true (single neokapi flag
			// covers both). Native default keeps both off — code is
			// usually verbatim in localisation contracts. Mirror okapi here
			// so the parity contract "same semantic config → same bytes"
			// holds without changing the native default.
			//
			// translateHTMLBlocks is intentionally NOT set: okapi runs
			// HTML blocks through an HTML subfilter that extracts only
			// inline translatable text (<summary>Loadbot log</summary>
			// → just "Loadbot log"). Native's TranslateHTMLBlocks=true
			// path treats the whole block as one Block, which would
			// over-translate tag text. HTML-block-content extraction is
			// tracked as a separate divergence (DirectShape.md,
			// test-html-block-newline.md) requiring HTML subfilter work.
			nativeConfig: map[string]any{
				"translateCodeBlocks": true,
			},
			// MarkdownCanonical absorbs the last residual cosmetic
			// differences between okapi's MarkdownFilter writer and the
			// native reader/writer pair: blockquote `>>` vs `> >`
			// spacing (okapi's flexmark visitor emits one space per
			// nest level regardless of source), all-whitespace lines
			// (okapi's LineTrimingWriter strips them in some contexts
			// but its skeleton writer re-adds the prefix in others —
			// we collapse both forms to bare `\n`), and findIndent
			// off-by-one on list-item soft-break continuation lines.
			normalizer: roundtrip.MarkdownCanonical{},
			minTier: map[string]roundtrip.Tier{
				"native": roundtrip.TierCanonicalEqual,
			},
		},
		{
			formatID:    "wiki",
			filterClass: "okf_wiki",
			sources:     []string{"integration-tests/okapi/src/test/resources/wikitext"},
			extensions:  []string{".wiki"},
			normalizer:  roundtrip.IgnoreTrailingNewline{},
		},

		// ── Key-value & structured data ───────────────────────────
		{
			// Bridge passes ~6 of 24 fixtures; the rest are flagged
			// per-file via poBridgeSkips().
			formatID:    "po",
			filterClass: "okf_po",
			sources:     []string{"integration-tests/okapi/src/test/resources/po"},
			extensions:  []string{".po"},
			// okf_po defaults to useCodeFinder=true with printf-style
			// patterns so `%s`, `%d`, `%1$s`, `{0}`, etc. are extracted
			// as inline codes (Spans) and not pseudo-translated as text.
			// Native po defaults to useCodeFinder=false, so `%s` becomes
			// `%ś` after pseudo. Match okapi by enabling the same rules.
			nativeConfig: map[string]any{
				"useCodeFinder": true,
				"codeFinderRules": []any{
					// okapi POFilter default rules, copied verbatim from
					// net/sf/okapi/filters/po/Parameters.reset(). The
					// conversion-letter set is exactly
					// [dioxXucsfeEgGpn] — letters like b, t, h, H, S are
					// translatable text in okapi's view, so matching them
					// here would break canonical-equality on fixtures
					// like Test_nautilus.af.po (`%b`) and
					// Test_DrupalRussianCP1251.po (`%t`, `%u`).
					`%(([-0+#]?)[-0+#]?)((\d\$)?)(([\d\*]*)(\.[\d\*]*)?)[dioxXucsfeEgGpn]`,
					`\{\d[^\\]*?\}`,
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
				roundtrip.POCharsetCase{},
				roundtrip.POJoinContinuations{},
			}},
		},
		{
			formatID:    "properties",
			filterClass: "okf_properties",
			sources:     []string{"integration-tests/okapi/src/test/resources/property"},
			extensions:  []string{".properties"},
			// okapi's PropertiesFilter defaults to useCodeFinder=true
			// with HTML-tag and Java-escape rules. Native defaults to
			// off, so without configuring it here HTML markup inside
			// translatable values gets pseudo-translated character by
			// character (`<b>` → `<ƀ>`).
			nativeConfig: map[string]any{
				"useCodeFinder": true,
				"codeFinderRules": []any{
					`<[^>]+>`,
					`\{\d+(?:,[^}]+)?\}`,
				},
				// Decode `\:` `\=` `\#` `\!` so values match okapi's
				// Java-spec parsing; the writer re-escapes leading `:`
				// and `=` to keep the value parseable.
				"useJavaEscapes": true,
			},
			normalizer: roundtrip.Chain{Steps: []roundtrip.Normalizer{
				roundtrip.LFLineEndings{},
				roundtrip.LowerHexUnicodeEscape{},
				roundtrip.IgnoreTrailingNewline{},
			}},
			minTier: map[string]roundtrip.Tier{"native": roundtrip.TierDivergent},
		},
		{
			formatID:    "json",
			filterClass: "okf_json",
			sources:     []string{"integration-tests/okapi/src/test/resources/json"},
			extensions:  []string{".json"},
			// JSON normalizer reaches canonical-equal when fixtures
			// differ only in whitespace (e.g. `"k" : "v"` vs `"k": "v"`)
			// or string escape choices that encoding/json normalizes.
			normalizer: roundtrip.JSONCanonical{},
		},
		{
			formatID:    "yaml",
			filterClass: "okf_yaml",
			sources:     []string{"integration-tests/okapi/src/test/resources/yaml"},
			extensions:  []string{".yaml", ".yml"},
			// Okapi's okf_yaml extracts every scalar (including bool /
			// int / null) as translatable text. Native defaults to
			// strings-only — for parity we mirror okapi by extracting
			// non-string scalars too. Without this, native preserves
			// `true` as a boolean while okapi pseudo-translates it to
			// `ţŕũē`, producing a real-looking divergence on every
			// fixture that has booleans/numbers.
			//
			// The codeFinder rules mirror okapi's TextModificationStep
			// inline-code detection for yaml fixtures:
			//   - `{{var}}` Mustache-style placeholders
			//   - `%[a-zA-Z]` printf-style date/format codes (matches
			//     okapi's preservation of `%d %b %Y %H:%M:%S` in
			//     fixtures like en (1)/(3).yml)
			// Without these, native pseudo-translates the placeholder
			// text while okapi's pseudo preserves it.
			nativeConfig: map[string]any{
				"extractNonStrings": true,
				"useCodeFinder":     true,
				// okapi YAMLFilter default rules, copied verbatim from
				// net/sf/okapi/filters/yaml/Parameters.reset(). The
				// printf letter set is wider than POFilter's because
				// it also accepts strftime-style specifiers
				// (Y/y/B/b/H/h/S/M/m/A/Z) — matches the okapi sample
				// `%s, %d, {1}, \\n, \\r, \\t, {{var}} etc.`
				"codeFinderRules": []any{
					`%(([-0+#]?)[-0+#]?)((\d\$)?)(([\d\*]*)(\.[\d\*]*)?)[dioxXucsfeEgGpnYyBbHhSMmAZ]`,
					`(\\r\\n)|\\a|\\b|\\f|\\n|\\r|\\t|\\v`,
					`\{\{\w.*?\}\}`,
				},
			},
			// YAML normalizer reaches canonical-equal when fixtures
			// differ only in indentation, quote style, or block-vs-flow
			// — both sides round-trip through gopkg.in/yaml.v3.
			normalizer: roundtrip.YAMLCanonical{},
		},
		{
			// Phpcontent has no integration-tests dir — fall back to
			// the unit-test resources dir.
			formatID:    "phpcontent",
			filterClass: "okf_phpcontent",
			sources:     []string{"okapi/filters/php/src/test/resources"},
			extensions:  []string{".phpcnt"},
			// okapi normalises heredoc line-endings to LF and emits a
			// trailing newline; native preserves source CRLF and omits
			// the trailing newline. Both are valid PHP — fold to canonical.
			normalizer: roundtrip.Chain{Steps: []roundtrip.Normalizer{
				roundtrip.LFLineEndings{},
				roundtrip.IgnoreTrailingNewline{},
			}},
		},

		// ── Tabular ───────────────────────────────────────────────
		{
			// Bridge passes ~7 of 15 fixtures; the rest are flagged
			// per-file via csvBridgeSkips().
			formatID:    "csv",
			filterClass: "okf_commaseparatedvalues",
			sources:     []string{"integration-tests/okapi/src/test/resources/table"},
			extensions:  []string{".csv"},
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
			formatID:    "doxygen",
			filterClass: "okf_doxygen",
			sources:     []string{"integration-tests/okapi/src/test/resources/doxygen"},
			extensions:  []string{".h", ".py"},
			// special_commands.h has cosmetic whitespace divergences
			// (blank-line indentation inside non-star-decorated blocks,
			// paragraph reflow in \note / @copydoc, trailing space on
			// one prose line). The canonical normalizer absorbs these.
			normalizer: roundtrip.DoxygenCanonical{},
		},

		// ── XML / bilingual exchange formats ──────────────────────
		{
			// okf_xml is the ITS-based XML filter (in
			// okapi/filters/its/...). 8 fixtures fail bridge due to
			// inline-code marker emission diverging from the
			// in-process okapi reference.
			formatID:    "xml",
			filterClass: "okf_xml",
			sources:     []string{"integration-tests/okapi/src/test/resources/xml"},
			extensions:  []string{".xml"},
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
			formatID:    "xliff",
			filterClass: "okf_xliff",
			sources:     []string{"integration-tests/okapi/src/test/resources/xliff"},
			extensions:  []string{".xlf"},
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
			// match okapi's lossy U+FFFD output, plus mirror okapi's
			// XLIFFFilter.java:2278 "drop divergent seg-source" rule
			// at read time so RB-12-Test02.xlf's id="11withWarning"
			// builds source segments from the un-segmented <source>
			// rather than the inconsistent seg-source. See
			// docs/internals/research/xliff-okapi-compat-quirks.md.
			nativeConfig: map[string]any{
				"okapiCompat": map[string]any{
					"simulateBrokenWindows1252Read": true,
					"unwrapSingleSegMrk":            true,
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
					"lowercaseLangSubtag":           true,
					"stripPhaseDateAttr":            true,
					"stripCDataCREntities":          true,
					"hoistAltTransNotes":            true,
					"stripAltTransSegSource":        true,
					"reorderHeaderToolToEnd":        true,
					"simulateBrokenWindows1252Read": true,
					// Disabled: blanket strip is more aggressive than
					// okapi. okapi PRESERVES `approved="yes"` whenever the
					// source trans-unit had a `<target>` element (Manual-12,
					// RB-11, SF-12-Test02 all have targets — okapi keeps
					// `approved`). The conditional rule (next flag) is the
					// faithful one.
					// "stripTransUnitApprovedAttr": true,
					//
					// Mirrors okapi XLIFFFilter.java:2475 +
					// XLIFFSkeletonWriter.java:756 — `approved` is dropped
					// when the source trans-unit has no `<target>` element
					// (the property only gets attached during target
					// processing, which doesn't run when source has no
					// target). SF-12-Test03 has 944 trans-units with
					// approved="no"; only TU id="1" has both source and
					// target, so it keeps approved while the other 943 lose it.
					"stripApprovedWhenNoSourceTarget": true,
					//
					// Now enabled: matches okapi's XLIFFFilter.java:2278
					// rule — drops <seg-source> and unwraps target's mrk
					// when source content != seg-source content.
					// Implemented as a writer post-process pass.
					"unwrapSingleSegMrk": true,
					//
					// Mirrors okapi XMLEncoder._encode (XMLEncoder.java:101-110):
					// charset-aware entity escaping ONLY fires when the source
					// declared a non-UTF-8 encoding. The writer reads the
					// per-layer `xliff:source-encoding` property: when set
					// (e.g. SF-12-Test03 declared windows-1252), chars > U+00FF
					// are emitted as `&#xNNNN;` entities; when absent
					// (Manual-12-AltTrans, RB-11, SF-12-Test02 — all UTF-8
					// sources), literal UTF-8 passes through. So this flag is
					// safe to enable globally — it's an automatic no-op for
					// UTF-8 sources.
					"escapeBeyondLatin1AsEntities": true,
				},
			},
		},
		{
			// okapi-bridge release 9b9521c added EnsureFilterWriterStep so
			// XLIFF2's null-filterWriter no longer NPEs the pseudo
			// pipeline, and resolveFilterClass now prefers the canonical
			// `xliff2.XLIFF2Filter` over `rainbowkit.XLIFF2Filter`. Both
			// engines now run the full xliff2 fixture set end to end.
			// Native achieves canonical-equal (XML formatting/attribute
			// order diffs absorbed by the normalizer). Bridge achieves
			// byte-equal on all 18 fixtures now that PseudoTranslationStep
			// correctly skips ignorable TextParts (matching the streaming
			// path's segments-only semantics).
			formatID:    "xliff2",
			filterClass: "okf_xliff2",
			sources:     []string{"integration-tests/okapi/src/test/resources/xliff2"},
			extensions:  []string{".xlf", ".xlf2"},
			// Same XMLCanonical config as okf_xliff: sort attributes (okapi
			// reorders), strip namespace declarations (encoding/xml mangles
			// xmlns:foo into _xmlns:foo when the prefix is also used as a
			// namespace, which produces asymmetric noise between native and
			// reference even when the underlying structure agrees).
			normalizer: roundtrip.XMLCanonical{SortAttrs: true, StripNamespaceDecls: true},
			// XLIFF 2 toolkit unconditionally overwrites the on-disk
			// trgLang and pseudo-translates source rather than the
			// authored target. Mirror that for both engines so the
			// pseudo input matches the okapi reference.
			bridgeForcePseudoSourceBase: true,
			minTier: map[string]roundtrip.Tier{
				"native": roundtrip.TierCanonicalEqual,
				"bridge": roundtrip.TierByteEqual,
			},
		},
		{
			formatID:    "tmx",
			filterClass: "okf_tmx",
			sources:     []string{"integration-tests/okapi/src/test/resources/tmx"},
			extensions:  []string{".tmx"},
			// TMX is XML; same canonical normalizer.
			normalizer: roundtrip.XMLCanonical{SortAttrs: true},
			// Bridge reaches canonical-equal on 3 fixtures (ImportTest2A,
			// ImportTest2B, simple) where okapi's writer reorders attrs or
			// reformats whitespace vs the bridge's serialization — exactly
			// what XMLCanonical{SortAttrs:true} normalizes away. The data
			// is semantically identical; accepting at TierCanonical is the
			// honest contract here.
			minTier: map[string]roundtrip.Tier{
				"bridge": roundtrip.TierCanonicalEqual,
			},
		},
		{
			formatID:    "ts",
			filterClass: "okf_ts",
			sources:     []string{"integration-tests/okapi/src/test/resources/ts"},
			extensions:  []string{".ts"},
			// TS is XML (Qt Linguist); same canonical normalizer.
			normalizer: roundtrip.XMLCanonical{SortAttrs: true},
		},
		{
			// Trados TTX bilingual XML — 4 fixtures upstream. The
			// upstream .ttx files are UTF-16 LE with BOM (TRADOS
			// convention); native handles that via encoding.ToUTF8
			// in the ttx reader (see core/formats/ttx/reader.go).
			//
			// Like xliff2, TTX is bilingual on disk: each <Tu> has
			// <Tuv Lang="EN-US"> + <Tuv Lang="FR-FR"> pairs. Okapi's
			// PseudoTranslationStep ignores the existing target and
			// pseudo-translates the source, then writes the pseudo'd
			// source into the target Tuv. bridgeForcePseudoSourceBase
			// mirrors that for the bridge daemon (the Go side's
			// applyPseudoToBlock defaults to using the existing target
			// as the pseudo base — same flag fix that xliff2 uses).
			//
			// Per-fixture divergences are annotated in
			// core/formats/ttx/parity-annotations.yaml.
			formatID:                    "ttx",
			filterClass:                 "okf_ttx",
			sources:                     []string{"okapi/filters/ttx/src/test/resources"},
			extensions:                  []string{".ttx"},
			normalizer:                  roundtrip.XMLCanonical{SortAttrs: true},
			bridgeForcePseudoSourceBase: true,
			minTier: map[string]roundtrip.Tier{
				"native": roundtrip.TierDivergent,
				"bridge": roundtrip.TierDivergent,
			},
		},
		{
			// TXML bilingual XML — 3 fixtures: Test01.docx.txml,
			// Test02.html.txml, Test03.mif.txml. Native lowercases
			// targetlocale and drops <target> elements present in the
			// source; documented in core/formats/txml/parity-annotations.yaml.
			formatID:    "txml",
			filterClass: "okf_txml",
			sources:     []string{"okapi/filters/txml/src/test/resources"},
			extensions:  []string{".txml"},
			normalizer:  roundtrip.XMLCanonical{SortAttrs: true},
			minTier: map[string]roundtrip.Tier{
				"native": roundtrip.TierDivergent,
				"bridge": roundtrip.TierDivergent,
			},
		},
		{
			// Vignette CMS XML — 1 fixture (Test01.xml). XML extension
			// shared with many formats; cherry-pick via explicitFiles
			// to keep this scan tight. The harness drives src=en/tgt=fr;
			// Test01.xml carries only en_US/es_ES/zh_CN locale instances,
			// so Okapi's locale-pair-driven VignetteFilter extracts
			// nothing (no LOCALE_ID == fr) and emits the file unchanged.
			// Native now honours the requested target locale the same way
			// (reader.go emitBlocks bilingual gate), so both engines are
			// byte-equal to the reference.
			formatID:      "vignette",
			filterClass:   "okf_vignette",
			explicitFiles: []string{"okapi/filters/vignette/src/test/resources/Test01.xml"},
			normalizer:    roundtrip.XMLCanonical{SortAttrs: true},
			minTier: map[string]roundtrip.Tier{
				"native": roundtrip.TierByteEqual,
				"bridge": roundtrip.TierByteEqual,
			},
		},

		// ── Subtitle / timed-text ─────────────────────────────────
		{
			// Default mergeCaptions=true mutates timestamps and
			// splits merged target text across cues; both engines
			// want the same override.
			formatID:    "vtt",
			filterClass: "okf_vtt",
			sources:     []string{"integration-tests/okapi/src/test/resources/vtt"},
			extensions:  []string{".vtt"},
			okapiParamConfig: `#v1
timeFormat=HH:mm:ss.SSS
maxLinesPerCaption.i=2
maxCharsPerLine.i=47
cjkCharsPerLine.i=18
mergeCaptions.b=false
`,
			bridgeParams: map[string]string{"mergeCaptions": "false"},
			// VTT chain: line-ending normalisation, plus a cue-body
			// flatten that folds okapi's maxCharsPerLine word-wrap
			// (47-char soft breaks) back into a single line per cue.
			// Both engines render the same WebVTT semantics — okapi
			// just word-wraps on output and native preserves the
			// source line shape. Collapsing inside cue bodies makes
			// the two byte-shapes equivalent for canonical comparison
			// without forcing native to mirror okapi's wrap heuristic
			// (which involves CJK width rules + maxLines fold).
			normalizer: roundtrip.Chain{Steps: []roundtrip.Normalizer{
				roundtrip.LFLineEndings{},
				roundtrip.VTTCueFlattenWS{},
			}},
		},
		{
			// Same mergeCaptions story as VTT.
			formatID:    "ttml",
			filterClass: "okf_ttml",
			sources:     []string{"integration-tests/okapi/src/test/resources/ttml"},
			extensions:  []string{".ttml"},
			okapiParamConfig: `#v1
timeFormat=HH:mm:ss.SSS
maxLinesPerCaption.i=2
maxCharsPerLine.i=47
cjkCharsPerLine.i=18
mergeCaptions.b=false
`,
			bridgeParams: map[string]string{"mergeCaptions": "false"},
			// Native default replaces <br/> with a space; okapi preserves
			// <br/> as literal text inside the translatable unit. Override
			// to match okapi's semantic — without this, native diverges
			// on every fixture that contains <br/>.
			nativeConfig: map[string]any{
				"escapeBR": false,
			},
			// TTML is XML; same canonical normalizer.
			normalizer: roundtrip.XMLCanonical{SortAttrs: true},
		},
		// srt + regex are intentionally NOT scanned here:
		//
		//   srt — upstream Okapi has no dedicated okf_subrip filter;
		//   tikal routes .srt through okf_regex with okf_regex@SRT.fprm
		//   (a 24-line param block including a regex like
		//   ^\d\d:\d\d:\d\d(.*?)\n(.*?)(\n\n|\z) that extracts cue text
		//   only — sequence numbers and timestamps land in skeleton).
		//   Native srt has a proper SubRip implementation (cue number +
		//   timestamps + multi-line text). They produce semantically
		//   different output: native preserves full cue structure;
		//   okapi-regex just text-substitutes inside cues. Wiring this
		//   would surface divergence on every fixture without a clear
		//   bug signal, so it's deferred until we add a "semantic-srt"
		//   normalizer or pick a different reference engine.
		//
		//   regex — every fixture in okapi/filters/regex/src/test/resources
		//   has its own okf_regex@<name>.fprm. The harness currently
		//   passes one okapiParamConfig per format; per-fixture .fprm
		//   wiring is a separate test-infrastructure change (formatScan
		//   needs a fixtureToFprm map and the engines need to consume it).

		// ── Misc text formats ─────────────────────────────────────
		{
			// /dtd/ exists in integration-tests but is empty; fall
			// back to the unit-test resources where Test01/Test02 live.
			formatID:    "dtd",
			filterClass: "okf_dtd",
			sources:     []string{"okapi/filters/dtd/src/test/resources"},
			extensions:  []string{".dtd"},
			// okapi DTDFilter defaults useCodeFinder=true with one rule —
			// HTML tag detection — and nothing else. Native default is off,
			// so HTML markup inside entity values gets pseudo-translated
			// character by character (`<i>HTML</i>` → `<ĩ>ĤŢMĹ</ĩ>`).
			nativeConfig: map[string]any{
				"useCodeFinder": true,
				"codeFinderRules": []any{
					`</?([A-Z0-9a-z]*)\b[^>]*>`,
				},
			},
			// okapi strips blank lines between declarations on round-
			// trip; our skeleton-driven writer preserves them. okapi
			// also re-parses every declaration through `com.wutka.dtd
			// .DTDParser` and re-serialises through `DTDOutput`, which
			// inlines parameter-entity refs (`%name;` → its value),
			// folds quote style to `"`, normalises spacing inside
			// content-model parens (`(a, b)` ↔ `(a,b)` ↔ `( a | b)`),
			// and strips per-line leading whitespace. Run the source-
			// preserving native bytes and the okapi reference through
			// a shared canonicaliser so all those stylistic choices
			// collapse to one form before comparison.
			normalizer: roundtrip.Chain{Steps: []roundtrip.Normalizer{
				roundtrip.DTDCanonical{},
				roundtrip.IgnoreTrailingNewline{},
				roundtrip.CollapseBlankLines{},
			}},
		},
		{
			formatID:    "tex",
			filterClass: "okf_tex",
			sources:     []string{"integration-tests/okapi/src/test/resources/tex"},
			extensions:  []string{".tex"},
			// okapi normalises CRLF → LF on round-trip; native preserves
			// source line endings. Both are valid LaTeX. okapi also
			// appends an extra trailing newline on output that the source
			// doesn't have — IgnoreTrailingNewline folds that asymmetry.
			normalizer: roundtrip.Chain{Steps: []roundtrip.Normalizer{
				roundtrip.LFLineEndings{},
				roundtrip.IgnoreTrailingNewline{},
			}},
		},
		{
			formatID:    "transtable",
			filterClass: "okf_transtable",
			sources:     []string{"integration-tests/okapi/src/test/resources/transtable"},
			extensions:  []string{".txt"},
		},

		// ── Binary / compound formats ─────────────────────────────
		{
			// Upstream filter dir has the curated round-trip set
			// (~70 .idml fixtures). Bridge passes ~24 of 70; the
			// remaining 46 are skipped per-file via idmlBridgeSkips()
			// (kept in coverage_skips_test.go to keep this file readable).
			formatID:    "idml",
			filterClass: "okf_idml",
			sources:     []string{"okapi/filters/idml/src/test/resources"},
			extensions:  []string{".idml"},
			isZip:       true,
			// IDML is a zip of XML. okapi emits XML decls with
			// single-quoted attrs ('1.0' encoding='UTF-8'); native
			// emits double-quoted ("1.0" encoding="UTF-8"). Beyond the
			// decl, okapi rewrites every IDML zip entry through its
			// own XML reader/writer cycle which strips
			// non-significant inter-element whitespace, reorders
			// attributes, and (importantly) alphabetises the children
			// of `<Properties>` containers (`BasedOn,PreviewColor,
			// AppliedFont` becomes `AppliedFont,BasedOn,
			// PreviewColor`). Pure byte parity is unrealistic, so we
			// chain XMLCanonical with attr-sort + child-sort to
			// capture the structural equivalence.
			normalizer: roundtrip.ZipEntryNormalizer{Inner: roundtrip.Chain{Steps: []roundtrip.Normalizer{
				roundtrip.StripXMLDeclaration{},
				roundtrip.XMLCanonical{
					SortAttrs:             true,
					SortChildElements:     true,
					MergeAdjacentCSRs:     true,
					MergeDefaultCSRs:      true,
					StripEmptyIDMLContent: true,
					StripIDMLACEPIs:       true,
					UnwrapIDMLXMLElement:  true,
					StripEmptyIDMLPSRCSR:  true,
				},
			}}},
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
			formatID:    "openxml",
			filterClass: "okf_openxml",
			sources:     []string{"okapi/filters/openxml/src/test/resources"},
			extensions:  []string{".docx"},
			isZip:       true,
			// OOXML is zip-of-XML. Pure byte parity is unrealistic:
			//   - encoding/xml always emits explicit close tags
			//     (`<w:sz w:val="28"></w:sz>`) while okapi self-closes
			//     empty elements (`<w:sz w:val="28"/>`).
			//   - native preserves the source's multi-line xmlns
			//     declarations (one per line) while okapi inlines them.
			//   - native always emits `xml:space="preserve"` on every
			//     `<w:t>` text run; okapi only emits it when leading/
			//     trailing whitespace actually needs preserving.
			//   - okapi strips revision-tracking IDs (`w:rsidR`,
			//     `w14:paraId`, …) on round-trip; native preserves
			//     whatever was authored.
			//   - okapi alphabetises sibling order in `<docDefaults>`
			//     (pPrDefault before rPrDefault) and other containers;
			//     native preserves source order.
			// Chain XMLCanonical with the openxml-specific options so
			// both engines re-emit through the same encoding/xml
			// pipeline — the structural noise cancels and the
			// underlying content compares cleanly. Most fixtures still
			// diverge on real semantic differences (lang attribute
			// translation, dropped pPrDefault, mc:AlternateContent
			// rewrites) that are openxml writer bugs, not normalisation
			// concerns; surfacing those past the noise is the goal.
			normalizer: roundtrip.ZipEntryNormalizer{Inner: roundtrip.Chain{Steps: []roundtrip.Normalizer{
				roundtrip.StripXMLDeclaration{},
				roundtrip.XMLCanonical{
					SortAttrs:             true,
					SortChildElements:     true,
					StripRevisionIDs:      true,
					StripXMLSpacePreserve: true,
				},
			}}},
		},
		{
			// Bridge passes ~2 of 41 fixtures; the rest are flagged
			// per-file via mifBridgeSkips().
			formatID:    "mif",
			filterClass: "okf_mif",
			sources:     []string{"integration-tests/okapi/src/test/resources/mif"},
			extensions:  []string{".mif"},
		},
		{
			// PDF — 3 upstream fixtures. PDF is binary; native's pdf
			// writer is extraction-only (no synthesis path), so it
			// produces empty output by design. Bridge runs through
			// okapi's PDFFilter which has a (lossy) write path.
			// minTier=TierDivergent for bridge surfaces the bridge↔
			// okapi-reference round-trip; native engine is skipped
			// pending a write-side implementation.
			formatID:    "pdf",
			filterClass: "okf_pdf",
			sources:     []string{"okapi/filters/pdf/src/test/resources"},
			extensions:  []string{".pdf", ".PDF"},
			minTier: map[string]roundtrip.Tier{
				"bridge": roundtrip.TierDivergent,
			},
			formatDefaultSkip: fileSkip{
				Engines: []string{"native"},
				Reason:  "native pdf writer is extraction-only; no synthesis path produces output bytes",
			},
		},
		{
			// RTF — 6 upstream fixtures. Per the existing PARITY_NOTES
			// commentary, upstream Okapi's only RTF round-trip path
			// goes through okf_tradosrtf (a separate filter that
			// expects TRADOS bilingual RTF, not plain RTF). The
			// okf_rtf filter exists but doesn't have an end-to-end
			// pseudo pipeline that matches the upstream test fixtures.
			// formatDefaultSkip[okapi] keeps the scan visible — the
			// dashboard shows "all-okapi-skipped" rather than
			// scan-missing — without crashing on the okapi reference.
			formatID:    "rtf",
			filterClass: "okf_rtf",
			sources:     []string{"okapi/filters/rtf/src/test/resources"},
			extensions:  []string{".rtf"},
			formatDefaultSkip: fileSkip{
				Engines: []string{"okapi"},
				Reason:  "upstream Okapi has no usable okf_rtf pseudo pipeline (only okf_tradosrtf works end-to-end); the .rtf corpus here is reference material for tradosrtf",
			},
		},

		{
			// OpenDocument Format (.odt/.ods/.odp/.odg). Zip of XML,
			// same shape as openxml/idml. Upstream filter dir holds
			// ~142 fixtures shared between okf_odf (content.xml-only
			// inner filter / flat .fodt) and okf_openoffice (zip
			// wrapper). neokapi's odf spec.yaml binds to okf_openoffice
			// — matches what's needed to unpack the zip and dispatch
			// to ODFFilter for each inner XML stream.
			//
			// Bridge daemon round-trips OO containers correctly after
			// okapi-bridge#11's two-pass refactor (pass 1 reads with
			// filter A, pass 2 opens a fresh filter B for write so
			// the source-side close() in nextInZipFile can't race the
			// writer). minTier=TierDivergent for both engines because
			// native vs okapi reference have real semantic differences
			// on every fixture (Okapi's ODFFilter inlines drawing/
			// script elements differently from native); per-fixture
			// annotation is a separate follow-up.
			formatID:    "odf",
			filterClass: "okf_openoffice",
			sources:     []string{"okapi/filters/openoffice/src/test/resources"},
			extensions:  []string{".odt", ".ods", ".odp", ".odg"},
			isZip:       true,
			normalizer: roundtrip.ZipEntryNormalizer{Inner: roundtrip.Chain{Steps: []roundtrip.Normalizer{
				roundtrip.StripXMLDeclaration{},
				roundtrip.XMLCanonical{
					SortAttrs:           true,
					SortChildElements:   true,
					StripNamespaceDecls: true,
				},
			}}},
			minTier: map[string]roundtrip.Tier{
				"native": roundtrip.TierDivergent,
				"bridge": roundtrip.TierDivergent,
			},
		},
	}
}
