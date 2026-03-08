export interface TestComparisonData {
  generatedAt: string;
  okapiVersion: string;
  gokapiVersion: string;
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
  totalFiltersGokapi?: number;
  totalTestsGokapi?: number;
}

export interface FilterComparison {
  filterName: string;
  nativeFilterName?: string; // native Go package name if different (e.g. "csv" for "table")
  okapi: FilterResult | null;
  bridge: FilterResult | null;
  native: FilterResult | null;
  testCases: TestCaseRow[];
  coverage: CoverageStats | null;
  // Backward compat (old JSON may have this)
  gokapi?: FilterResult | null;
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
  status: 'pass' | 'fail' | 'skip' | 'error';
  durationMs: number;
}

/** Test state classification. */
export type TestState = 'implemented' | 'pending' | 'skipped' | 'unmapped';

/** State filter applied from the summary bar to the format list. */
export type StateFilter =
  | null
  | 'implemented'
  | 'not-applicable'
  | 'pending'
  | 'unmapped';

/** Auto-classified category for not-applicable tests. */
export type SkipCategory =
  | 'subfilter'
  | 'vendor'
  | 'roundtrip'
  | 'testdata'
  | 'java-api'
  | 'regex'
  | 'config'
  | 'format'
  | 'dita'
  | 'feature'
  | 'not-implemented'
  | 'other';

/** Human-readable labels for skip categories. */
export const skipCategoryLabels: Record<SkipCategory, string> = {
  subfilter: 'Subfilter',
  vendor: 'Vendor Extension',
  roundtrip: 'Roundtrip',
  testdata: 'Test Data',
  'java-api': 'Java API',
  regex: 'Regex',
  config: 'Config',
  format: 'Wrong Format',
  dita: 'DITA',
  feature: 'Feature',
  'not-implemented': 'Not Implemented',
  other: 'Other',
};

/** Colors for skip categories. */
export const skipCategoryColors: Record<SkipCategory, string> = {
  subfilter: '#8b5cf6',
  vendor: '#ec4899',
  roundtrip: '#06b6d4',
  testdata: '#f59e0b',
  'java-api': '#6366f1',
  regex: '#14b8a6',
  config: '#f97316',
  format: '#ef4444',
  dita: '#a855f7',
  feature: '#3b82f6',
  'not-implemented': '#64748b',
  other: '#94a3b8',
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
  /** Bridge test source file path (relative to gokapi repo root). */
  bridgeFile?: string;
  /** Bridge test source line number (1-based). */
  bridgeLine?: number;
  /** Native Go test name, empty if not mapped. */
  nativeTest: string;
  /** Native status or empty string. */
  nativeStatus: string;
  /** Native test source file path (relative to gokapi repo root). */
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
}

/** Wire format from the Go testcompare tool (annotation-based). */
export interface TestCaseMatch {
  javaClass: string;
  javaMethod: string;
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
  const bridge = f.bridge ?? f.gokapi ?? null;
  const native = f.native ?? null;

  // If the Go tool provided annotation-based testCases, convert them.
  // Otherwise build rows from suite data so the table always renders.
  const rawTestCases = (f as any).testCases as TestCaseMatch[] | undefined;
  const testCases =
    rawTestCases && rawTestCases.length > 0
      ? convertAnnotatedRows(rawTestCases)
      : buildRowsFromSuites(f.okapi, bridge, native);

  return {
    filterName: f.filterName,
    nativeFilterName: f.nativeFilterName,
    okapi: f.okapi ?? null,
    bridge,
    native,
    testCases,
    coverage: f.coverage ?? null,
  };
}

/** Convert annotation-based TestCaseMatch to display rows. */
function convertAnnotatedRows(matches: TestCaseMatch[]): TestCaseRow[] {
  return matches.map((m) => ({
    testName: m.javaMethod,
    javaClass: shortClass(m.javaClass),
    okapiStatus: m.okapiStatus ?? '',
    okapiFile: m.okapiFile,
    bridgeTest: m.bridgeTest ?? '',
    bridgeStatus: m.bridgeStatus ?? '',
    bridgeFile: m.bridgeFile,
    bridgeLine: m.bridgeLine,
    nativeTest: m.nativeTest ?? '',
    nativeStatus: m.nativeStatus ?? '',
    nativeFile: m.nativeFile,
    nativeLine: m.nativeLine,
    skipReason: m.skipReason,
    testState: m.testState,
    bridgeSubtests: m.bridgeSubtests,
    nativeSubtests: m.nativeSubtests,
    skipCategory: m.skipCategory,
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
          bridgeTest: '',
          bridgeStatus: '',
          nativeTest: '',
          nativeStatus: '',
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
          javaClass: '',
          okapiStatus: '',
          bridgeTest: tc.name,
          bridgeStatus: tc.status,
          nativeTest: '',
          nativeStatus: '',
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
          javaClass: '',
          okapiStatus: '',
          bridgeTest: '',
          bridgeStatus: '',
          nativeTest: tc.name,
          nativeStatus: tc.status,
        });
      }
    }
  }

  return rows;
}

function shortClass(fqn: string): string {
  const idx = fqn.lastIndexOf('.');
  return idx >= 0 ? fqn.slice(idx + 1) : fqn;
}

/**
 * Normalize Summary from JSON that may use old field names.
 */
export function normalizeSummary(s: Summary): Summary {
  return {
    ...s,
    totalFiltersBridge: s.totalFiltersBridge ?? s.totalFiltersGokapi ?? 0,
    totalTestsBridge: s.totalTestsBridge ?? s.totalTestsGokapi ?? 0,
    totalFiltersNative: s.totalFiltersNative ?? 0,
    totalTestsNative: s.totalTestsNative ?? 0,
    totalFuncsBridge: s.totalFuncsBridge,
    totalFuncsNative: s.totalFuncsNative,
    coveragePct: s.coveragePct ?? 0,
  };
}
