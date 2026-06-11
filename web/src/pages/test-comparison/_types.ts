export interface TestComparisonData {
  generatedAt: string;
  okapiVersion: string;
  neokapiVersion: string;
  goCommitSHA?: string;
  okapiTag?: string;
  filters: FilterComparison[];
  summary: Summary;
}

export interface Summary {
  totalFiltersOkapi: number;
  totalFiltersBridge: number;
  totalFiltersNative: number;
  totalFiltersBoth: number;
  totalTestsOkapi: number;
  totalTestsBridge: number;
  totalTestsNative: number;
  coveragePct: number;
  totalFuncsBridge?: number;
  totalFuncsNative?: number;
  categoryCounts?: Record<string, number>;
  // Backward compat (old JSON may have these)
  totalFiltersNeokapi?: number;
  totalTestsNeokapi?: number;
}

export type SpecKind = "top_level" | "subfilter";

export interface FilterComparison {
  /** Canonical row id: the neokapi native format directory name (e.g. "csv",
   * "fixedwidth", "plaintext"). Rows are keyed by native format so each format
   * gets exactly one row. */
  filterName: string;
  /** Mirror of filterName, kept for backward compat with older JSON. */
  nativeFilterName?: string;
  /** Okapi filter package id(s) whose @Test classes route to this native format
   * (e.g. csv ← ["table"], xml ← ["xml","xmlstream"]). Rendered as a secondary
   * label so a user can navigate the Okapi side. Empty for neokapi-only formats
   * (jsx, mo, exec, formats, memorytest, versifiedtext — no Okapi equivalent). */
  okapiFilterIds?: string[];
  /** Mirror of spec.yaml `kind:` — present only when the filter has a spec.yaml. Empty/missing
   * is treated as `top_level`. `subfilter` filters render in their own dashboard section
   * because they're invoked through a parent and have no top-level bridge schema. */
  specKind?: SpecKind;
  okapi: FilterResult | null;
  bridge: FilterResult | null;
  native: FilterResult | null;
  testCases: TestCaseRow[];
  coverage: CoverageStats | null;
  spec?: SpecSummary; // present when the filter has a spec.yaml driving the parity runner
  specDrift?: SpecDriftEntry[]; // okapi_refs in spec.yaml that don't match the pinned Okapi @Test set
  specConfigDrift?: SpecConfigDriftEntry[]; // spec.config[].key entries not in the bridge composite schema
  // Backward compat (old JSON may have this)
  neokapi?: FilterResult | null;
}

/** One stale okapi_ref entry surfaced by contract-audit's drift check. */
export interface SpecDriftEntry {
  featureId: string;
  okapiRef: string; // ClassName#methodName
  reason: string; // currently always "missing-from-okapi"
}

/** One spec.config[] key that doesn't appear in the bridge composite JSON Schema. */
export interface SpecConfigDriftEntry {
  key: string;
  okapiParam: string; // Java field name from spec, may be empty
  reason: string; // currently always "missing-from-bridge-schema"
}

/** Per-filter feature coverage summary from the format spec runner. */
export interface SpecSummary {
  features: SpecFeature[];
  pass: number;
  fail: number;
  skip: number;
  parityWarn: number;
  expectedFail: number;
}

export interface SpecFeature {
  id: string;
  examples: SpecExample[];
}

export type SpecExampleStatus =
  | "pass"
  | "fail"
  | "skip"
  | "expected_fail"
  | "parity_warn";

/**
 * Fault attribution for an expected_fail / parity_warn example: which side
 * diverges and why. Only `native-bug` is alarming (neokapi is wrong); every
 * other category is correct-by-design or an upstream/transport issue, so the
 * native side is the correct one.
 */
export type DivergenceKind =
  | "native-bug"
  | "bridge-gap"
  | "okapi-bug"
  | "scope-diff"
  | "default-diff"
  | "missing-filter"
  | "fixture"
  | "contract";

/** Short chip label per divergence category. */
export const divergenceLabels: Record<DivergenceKind, string> = {
  "native-bug": "native bug",
  "bridge-gap": "bridge gap",
  "okapi-bug": "Okapi bug",
  "scope-diff": "scope diff",
  "default-diff": "default diff",
  "missing-filter": "missing filter",
  fixture: "fixture",
  contract: "by design",
};

/** Plain-language explanation of each divergence category, shown on hover. */
export const divergenceDescriptions: Record<DivergenceKind, string> = {
  "native-bug": "neokapi's native reader is wrong here — a real bug to fix.",
  "bridge-gap":
    "okapi-bridge can't receive neokapi's config/rules over gRPC (or the bridge filter failed to run). The native reader is correct.",
  "okapi-bug":
    "Upstream Okapi's filter is wrong here. The native reader is correct.",
  "scope-diff":
    "The Okapi filter has a different feature scope or representation (both pass spec independently). The native reader is correct.",
  "default-diff":
    "Okapi's default configuration differs; with the same semantic config the results match. The native reader is correct.",
  "missing-filter":
    "The bridge doesn't ship this okf_ filter. The native reader is correct.",
  fixture:
    "A test-infrastructure / synthetic-fixture artefact, not a behavioral divergence.",
  contract:
    "A parse-error-by-design (the input yields no blocks). The native reader is correct.",
};

/**
 * Chip color per divergence category. Only `native-bug` is danger-red; all
 * others are neutral/info so the dashboard doesn't alarm on correct-by-design
 * or upstream divergences.
 */
export const divergenceColors: Record<DivergenceKind, string> = {
  "native-bug": "#dc2626", // danger red — the only "must fix"
  "bridge-gap": "#3b82f6", // info blue
  "okapi-bug": "#0ea5e9", // sky — upstream issue
  "scope-diff": "#64748b", // neutral slate
  "default-diff": "#64748b", // neutral slate
  "missing-filter": "#94a3b8", // muted
  fixture: "#a8a29e", // muted stone
  contract: "#7c8b9c", // neutral
};

export interface SpecExample {
  name: string;
  status: SpecExampleStatus;
  mode?: string; // head-to-head | bridge-only
  detail?: string;
  /** Fault attribution for expected_fail / parity_warn examples. */
  divergence?: DivergenceKind;
}

export interface FilterResult {
  suites: TestSuite[];
  total: number;
  passed: number;
  failed: number;
  skipped: number;
  errors: number;
  funcs?: number;
}

export interface TestSuite {
  name: string;
  tests: TestCase[];
  total: number;
  passed: number;
  failed: number;
  skipped: number;
  errors: number;
  durationMs: number;
}

export interface TestCase {
  name: string;
  className?: string;
  status: "pass" | "fail" | "skip" | "error";
  durationMs: number;
}

/** Test state classification. */
export type TestState = "implemented" | "pending" | "skipped" | "unmapped";

/** State filter applied from the summary bar to the format list. */
export type StateFilter =
  | null
  | "implemented"
  | "not-applicable"
  | "pending"
  | "unmapped";

/** Auto-classified category for not-applicable tests. */
export type SkipCategory =
  | "subfilter"
  | "vendor"
  | "roundtrip"
  | "testdata"
  | "java-api"
  | "regex"
  | "config"
  | "format"
  | "dita"
  | "feature"
  | "not-implemented"
  | "no-native"
  | "abstract"
  | "deferred"
  | "acknowledged"
  | "other";

/** Human-readable labels for skip categories. */
export const skipCategoryLabels: Record<SkipCategory, string> = {
  subfilter: "Subfilter",
  vendor: "Vendor Extension",
  roundtrip: "Roundtrip",
  testdata: "Test Data",
  "java-api": "Java API",
  regex: "Regex",
  config: "Config",
  format: "Wrong Format",
  dita: "DITA",
  feature: "Feature",
  "not-implemented": "Not Implemented",
  "no-native": "No Native Reader (bridge-only)",
  abstract: "Abstract Base Class",
  deferred: "Covered Indirectly",
  acknowledged: "Reviewed Gap",
  other: "Other",
};

/** Colors for skip categories. */
export const skipCategoryColors: Record<SkipCategory, string> = {
  subfilter: "#8b5cf6",
  vendor: "#ec4899",
  roundtrip: "#06b6d4",
  testdata: "#f59e0b",
  "java-api": "#6366f1",
  regex: "#14b8a6",
  config: "#f97316",
  format: "#ef4444",
  dita: "#a855f7",
  feature: "#3b82f6",
  "not-implemented": "#64748b",
  "no-native": "#0ea5e9",
  abstract: "#7c3aed",
  deferred: "#22d3ee",
  acknowledged: "#a8a29e",
  other: "#94a3b8",
};

/** Row in the unified test case table. */
export interface TestCaseRow {
  /** Display name for the test (Java method or Go func). */
  testName: string;
  /** Java class (short name), empty for bridge/native-only rows. */
  javaClass: string;
  /** Okapi status or empty string. */
  okapiStatus: string;
  /** Okapi source file path (relative to Okapi repo root). */
  okapiFile?: string;
  /** Bridge Go test name, empty if not mapped. */
  bridgeTest: string;
  /** Bridge status or empty string. */
  bridgeStatus: string;
  /** Bridge test source file path (relative to neokapi repo root). */
  bridgeFile?: string;
  /** Bridge test source line number (1-based). */
  bridgeLine?: number;
  /** Native Go test name, empty if not mapped. */
  nativeTest: string;
  /** Native status or empty string. */
  nativeStatus: string;
  /** Native test source file path (relative to neokapi repo root). */
  nativeFile?: string;
  /** Native test source line number (1-based). */
  nativeLine?: number;
  /** Skip/unmapped reason. */
  skipReason?: string;
  /** Test state: implemented, pending, skipped, or unmapped. */
  testState?: TestState;
  /** Number of Go subtests under the bridge test function. */
  bridgeSubtests?: number;
  /** Number of Go subtests under the native test function. */
  nativeSubtests?: number;
  /** Auto-classified skip category. */
  skipCategory?: SkipCategory;
  /** >1 when this row collapses N JUnit parameterized invocations (fixtures). */
  params?: number;
  /** For a not-applicable row whose reason claims the behavior is covered
   * elsewhere: the native Go test that verifies it, with a source link, so
   * the claim is verifiable rather than asserted. */
  coveredByTest?: string;
  coveredByFile?: string;
  coveredByLine?: number;
}

/** Wire format from the Go testcompare tool (annotation-based). */
export interface TestCaseMatch {
  javaClass: string;
  javaMethod: string;
  params?: number;
  okapiStatus: string;
  okapiFile?: string;
  bridgeTest: string;
  bridgeStatus: string;
  bridgeFile?: string;
  bridgeLine?: number;
  nativeTest: string;
  nativeStatus: string;
  nativeFile?: string;
  nativeLine?: number;
  skipReason?: string;
  testState?: TestState;
  bridgeSubtests?: number;
  nativeSubtests?: number;
  skipCategory?: SkipCategory;
  coveredByTest?: string;
  coveredByFile?: string;
  coveredByLine?: number;
}

export interface CoverageStats {
  totalOkapi: number;
  bridgeMapped: number;
  bridgePassing: number;
  nativeMapped: number;
  nativePassing: number;
  coveragePct: number;
  skippedCount?: number;
  pendingCount?: number;
  implementedPct?: number;
  notApplicableCount?: number;
  categoryCounts?: Record<string, number>;
  bridgeAndNative?: number;
  bridgeOnly?: number;
  nativeOnly?: number;
}

/**
 * Normalize a FilterComparison from JSON that may use old field names.
 * Always builds unified testCases rows — from annotations if available,
 * otherwise from raw suite data.
 */
export function normalizeFilter(f: FilterComparison): FilterComparison {
  const bridge = f.bridge ?? f.neokapi ?? null;
  const native = f.native ?? null;

  // If the Go tool provided annotation-based testCases, convert them.
  // Otherwise build rows from suite data so the table always renders.
  const rawTestCases = (f as any).testCases as TestCaseMatch[] | undefined;
  const testCases =
    rawTestCases && rawTestCases.length > 0
      ? convertAnnotatedRows(rawTestCases)
      : buildRowsFromSuites(f.okapi, bridge, native);

  const specKind = (f as { specKind?: string }).specKind;
  return {
    filterName: f.filterName,
    nativeFilterName: f.nativeFilterName,
    okapiFilterIds: f.okapiFilterIds,
    specKind:
      specKind === "subfilter"
        ? "subfilter"
        : specKind === "top_level"
          ? "top_level"
          : undefined,
    okapi: f.okapi ?? null,
    bridge,
    native,
    testCases,
    coverage: f.coverage ?? null,
    spec: f.spec,
    specDrift: f.specDrift,
    specConfigDrift: f.specConfigDrift,
  };
}

/** Convert annotation-based TestCaseMatch to display rows. */
function convertAnnotatedRows(matches: TestCaseMatch[]): TestCaseRow[] {
  return matches.map((m) => ({
    testName: m.javaMethod,
    javaClass: shortClass(m.javaClass),
    okapiStatus: m.okapiStatus ?? "",
    okapiFile: m.okapiFile,
    bridgeTest: m.bridgeTest ?? "",
    bridgeStatus: m.bridgeStatus ?? "",
    bridgeFile: m.bridgeFile,
    bridgeLine: m.bridgeLine,
    nativeTest: m.nativeTest ?? "",
    nativeStatus: m.nativeStatus ?? "",
    nativeFile: m.nativeFile,
    nativeLine: m.nativeLine,
    skipReason: m.skipReason,
    testState: m.testState,
    bridgeSubtests: m.bridgeSubtests,
    nativeSubtests: m.nativeSubtests,
    skipCategory: m.skipCategory,
    params: m.params,
    coveredByTest: m.coveredByTest,
    coveredByFile: m.coveredByFile,
    coveredByLine: m.coveredByLine,
  }));
}

/**
 * Build unified rows from raw suite data when annotations aren't available.
 * Each Okapi test becomes a row; bridge and native tests that don't map to
 * any Okapi row are appended as separate rows.
 */
function buildRowsFromSuites(
  okapi: FilterResult | null,
  bridge: FilterResult | null,
  native: FilterResult | null,
): TestCaseRow[] {
  const rows: TestCaseRow[] = [];

  // 1) All Okapi tests
  if (okapi) {
    for (const suite of okapi.suites) {
      for (const tc of suite.tests) {
        rows.push({
          testName: tc.name,
          javaClass: shortClass(tc.className ?? suite.name),
          okapiStatus: tc.status,
          bridgeTest: "",
          bridgeStatus: "",
          nativeTest: "",
          nativeStatus: "",
        });
      }
    }
  }

  // 2) All Bridge tests as separate rows (below Okapi)
  if (bridge) {
    for (const suite of bridge.suites) {
      for (const tc of suite.tests) {
        rows.push({
          testName: tc.name,
          javaClass: "",
          okapiStatus: "",
          bridgeTest: tc.name,
          bridgeStatus: tc.status,
          nativeTest: "",
          nativeStatus: "",
        });
      }
    }
  }

  // 3) All Native tests as separate rows
  if (native) {
    for (const suite of native.suites) {
      for (const tc of suite.tests) {
        rows.push({
          testName: tc.name,
          javaClass: "",
          okapiStatus: "",
          bridgeTest: "",
          bridgeStatus: "",
          nativeTest: tc.name,
          nativeStatus: tc.status,
        });
      }
    }
  }

  return rows;
}

function shortClass(fqn: string): string {
  const idx = fqn.lastIndexOf(".");
  return idx >= 0 ? fqn.slice(idx + 1) : fqn;
}

/**
 * Normalize Summary from JSON that may use old field names.
 */
export function normalizeSummary(s: Summary): Summary {
  return {
    ...s,
    totalFiltersBridge: s.totalFiltersBridge ?? s.totalFiltersNeokapi ?? 0,
    totalTestsBridge: s.totalTestsBridge ?? s.totalTestsNeokapi ?? 0,
    totalFiltersNative: s.totalFiltersNative ?? 0,
    totalTestsNative: s.totalTestsNative ?? 0,
    totalFuncsBridge: s.totalFuncsBridge,
    totalFuncsNative: s.totalFuncsNative,
    coveragePct: s.coveragePct ?? 0,
  };
}
