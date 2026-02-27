export interface TestComparisonData {
  generatedAt: string;
  okapiVersion: string;
  gokapiVersion: string;
  filters: FilterComparison[];
  summary: Summary;
}

export interface Summary {
  totalFiltersOkapi: number;
  totalFiltersGokapi: number;
  totalFiltersBoth: number;
  totalTestsOkapi: number;
  totalTestsGokapi: number;
}

export interface FilterComparison {
  filterName: string;
  okapi: FilterResult | null;
  gokapi: FilterResult | null;
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
