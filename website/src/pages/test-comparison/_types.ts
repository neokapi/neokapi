export interface TestComparisonData {
  generatedAt: string;
  okapiVersion: string;
  gokapiVersion: string;
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
  // Backward compat (old JSON may have these)
  totalFiltersGokapi?: number;
  totalTestsGokapi?: number;
}

export interface FilterComparison {
  filterName: string;
  okapi: FilterResult | null;
  bridge: FilterResult | null;
  native: FilterResult | null;
  testCases: TestCaseMatch[];
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

export interface TestCaseMatch {
  javaClass: string;
  javaMethod: string;
  okapiStatus: 'pass' | 'fail' | 'skip' | 'error';
  bridgeTest: string;
  bridgeStatus: 'pass' | 'fail' | 'skip' | 'error' | '';
  nativeTest: string;
  nativeStatus: 'pass' | 'fail' | 'skip' | 'error' | '';
}

export interface CoverageStats {
  totalOkapi: number;
  bridgeMapped: number;
  bridgePassing: number;
  nativeMapped: number;
  nativePassing: number;
  coveragePct: number;
}

/**
 * Normalize a FilterComparison from JSON that may use old field names.
 * Maps `gokapi` -> `bridge` and fills in missing new fields with defaults.
 */
export function normalizeFilter(f: FilterComparison): FilterComparison {
  return {
    filterName: f.filterName,
    okapi: f.okapi ?? null,
    bridge: f.bridge ?? f.gokapi ?? null,
    native: f.native ?? null,
    testCases: f.testCases ?? [],
    coverage: f.coverage ?? null,
  };
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
    coveragePct: s.coveragePct ?? 0,
  };
}
