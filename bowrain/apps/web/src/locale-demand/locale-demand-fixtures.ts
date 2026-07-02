// ---------------------------------------------------------------------------
// Locale demand — sample dataset (DESIGN PROTOTYPE)
//
// This module fakes what a demand-ingest API will eventually provide: end-user
// language demand observed by the web beacon (Accept-Language counts), the app
// telemetry ingest (device locales), and store territories — joined against the
// project's current locale coverage and priced with the plan estimator.
//
// Everything is served through the `DemandDataSource` interface so the real
// ingest-backed implementation can replace `sampleDemandDataSource` without
// touching any component: swap the object handed to the route/panels and the
// UI keeps working. Keep the interface transport-agnostic (sync today, but a
// real source would return promises / query options).
// ---------------------------------------------------------------------------

export type TimeRange = "7d" | "30d" | "90d";

export const TIME_RANGES: { value: TimeRange; label: string }[] = [
  { value: "7d", label: "Last 7 days" },
  { value: "30d", label: "Last 30 days" },
  { value: "90d", label: "Last 90 days" },
];

/** Coverage of a demanded language in the current project. */
export type Coverage =
  | { status: "covered" }
  | { status: "partial"; percent: number }
  | { status: "not-covered" };

/** Mocked plan estimate for closing a language gap (real one comes from the plan machinery). */
export interface PlanEstimate {
  /** Translatable units the plan would touch. */
  units: number;
  /** Approximate token volume for the run. */
  tokens: number;
  /** Expected TM leverage, percent. */
  tmCoveragePercent: number;
}

export interface CountryLanguageSplit {
  /** BCP-47 language code. */
  code: string;
  /** Share of this country's sessions requesting the language (0..1). */
  share: number;
}

export interface CountryDemand {
  /** ISO 3166-1 alpha-2. */
  code: string;
  /** ISO 3166-1 numeric id as used by world-atlas TopoJSON features (zero-padded). */
  numericId: string;
  name: string;
  flag: string;
  sessions: number;
  /** Share of all sessions in the snapshot (0..1). */
  share: number;
  /** Share of sessions that received a locale they asked for (0..1). */
  servedRate: number;
  /** Language mix within the country, sorted by share desc. */
  languages: CountryLanguageSplit[];
  /** 12 weekly session counts, oldest first. */
  trend: number[];
}

export interface LanguageCountrySplit {
  /** ISO 3166-1 alpha-2. */
  code: string;
  /** Share of this language's sessions originating in the country (0..1). */
  share: number;
}

export interface LanguageDemand {
  /** BCP-47 language code. */
  code: string;
  sessions: number;
  /** Share of all sessions in the snapshot (0..1). */
  share: number;
  /** 12 weekly session counts, oldest first. */
  trend: number[];
  coverage: Coverage;
  /** Share of sessions in this language that were served the language (0..1). */
  servedRate: number;
  /** Countries the demand comes from, sorted by share desc. */
  countries: LanguageCountrySplit[];
  /** Present when the language is not fully covered. */
  planEstimate?: PlanEstimate;
}

export interface DemandSnapshot {
  range: TimeRange;
  generatedAt: string;
  totalSessions: number;
  /** Locales currently configured on the project (mocked). */
  projectLocales: string[];
  countries: CountryDemand[];
  languages: LanguageDemand[];
}

/**
 * Seam for the real ingest API. The sample implementation below is synchronous
 * and deterministic; a production implementation would wrap the demand-ingest
 * REST endpoints (web beacon + app telemetry + store territories) and the plan
 * estimator, most likely as React Query options keyed by workspace/project.
 */
export interface DemandDataSource {
  getSnapshot(range: TimeRange): DemandSnapshot;
}

// ---------------------------------------------------------------------------
// Sample dataset
// ---------------------------------------------------------------------------

/** Locales the mocked project already ships. */
const PROJECT_LOCALES = ["en", "de", "fr", "ja"];

const COVERAGE: Record<string, Coverage> = {
  en: { status: "covered" },
  de: { status: "covered" },
  fr: { status: "covered" },
  ja: { status: "partial", percent: 62 },
};

function coverageFor(code: string): Coverage {
  return COVERAGE[code] ?? { status: "not-covered" };
}

/** Mocked plan estimates per uncovered/partial language (units · TM leverage). */
const PLAN_ESTIMATES: Record<string, PlanEstimate> = {
  ja: { units: 1566, tokens: 720_000, tmCoveragePercent: 62 },
  "pt-BR": { units: 4120, tokens: 2_100_000, tmCoveragePercent: 14 },
  ko: { units: 4120, tokens: 2_460_000, tmCoveragePercent: 6 },
  "es-419": { units: 4120, tokens: 1_980_000, tmCoveragePercent: 21 },
  pl: { units: 4120, tokens: 2_240_000, tmCoveragePercent: 9 },
  tr: { units: 4120, tokens: 2_190_000, tmCoveragePercent: 7 },
  ar: { units: 4120, tokens: 2_620_000, tmCoveragePercent: 4 },
  it: { units: 4120, tokens: 2_050_000, tmCoveragePercent: 18 },
  nl: { units: 4120, tokens: 2_020_000, tmCoveragePercent: 16 },
  "zh-Hans": { units: 4120, tokens: 2_380_000, tmCoveragePercent: 5 },
  hi: { units: 4120, tokens: 2_550_000, tmCoveragePercent: 3 },
  vi: { units: 4120, tokens: 2_340_000, tmCoveragePercent: 4 },
  id: { units: 4120, tokens: 2_150_000, tmCoveragePercent: 8 },
};

interface CountrySeed {
  code: string;
  numericId: string;
  name: string;
  flag: string;
  /** Weekly sessions in the base (7d) window. */
  sessions: number;
  /** Language splits, must sum to ~1. */
  languages: [string, number][];
  /** Weekly trend slope, fraction per week (0.02 → +2%/week). */
  growth: number;
}

// ~28 countries with plausible language mixes. Sessions are weekly web+app
// sessions in the 7d window; longer ranges scale them below.
const COUNTRY_SEEDS: CountrySeed[] = [
  {
    code: "US",
    numericId: "840",
    name: "United States",
    flag: "🇺🇸",
    sessions: 48210,
    growth: 0.004,
    languages: [
      ["en", 0.86],
      ["es-419", 0.1],
      ["zh-Hans", 0.02],
      ["vi", 0.02],
    ],
  },
  {
    code: "BR",
    numericId: "076",
    name: "Brazil",
    flag: "🇧🇷",
    sessions: 30440,
    growth: 0.055,
    languages: [
      ["pt-BR", 0.94],
      ["en", 0.05],
      ["es-419", 0.01],
    ],
  },
  {
    code: "DE",
    numericId: "276",
    name: "Germany",
    flag: "🇩🇪",
    sessions: 21470,
    growth: 0.008,
    languages: [
      ["de", 0.83],
      ["en", 0.11],
      ["tr", 0.04],
      ["pl", 0.02],
    ],
  },
  {
    code: "JP",
    numericId: "392",
    name: "Japan",
    flag: "🇯🇵",
    sessions: 19860,
    growth: 0.021,
    languages: [
      ["ja", 0.96],
      ["en", 0.04],
    ],
  },
  {
    code: "KR",
    numericId: "410",
    name: "South Korea",
    flag: "🇰🇷",
    sessions: 16750,
    growth: 0.048,
    languages: [
      ["ko", 0.95],
      ["en", 0.05],
    ],
  },
  {
    code: "MX",
    numericId: "484",
    name: "Mexico",
    flag: "🇲🇽",
    sessions: 14980,
    growth: 0.037,
    languages: [
      ["es-419", 0.93],
      ["en", 0.07],
    ],
  },
  {
    code: "FR",
    numericId: "250",
    name: "France",
    flag: "🇫🇷",
    sessions: 14110,
    growth: 0.006,
    languages: [
      ["fr", 0.88],
      ["en", 0.08],
      ["ar", 0.04],
    ],
  },
  {
    code: "GB",
    numericId: "826",
    name: "United Kingdom",
    flag: "🇬🇧",
    sessions: 13580,
    growth: 0.003,
    languages: [
      ["en", 0.92],
      ["pl", 0.04],
      ["fr", 0.02],
      ["ar", 0.02],
    ],
  },
  {
    code: "PL",
    numericId: "616",
    name: "Poland",
    flag: "🇵🇱",
    sessions: 11930,
    growth: 0.041,
    languages: [
      ["pl", 0.93],
      ["en", 0.07],
    ],
  },
  {
    code: "TR",
    numericId: "792",
    name: "Türkiye",
    flag: "🇹🇷",
    sessions: 11060,
    growth: 0.046,
    languages: [
      ["tr", 0.95],
      ["en", 0.05],
    ],
  },
  {
    code: "IN",
    numericId: "356",
    name: "India",
    flag: "🇮🇳",
    sessions: 10420,
    growth: 0.052,
    languages: [
      ["en", 0.55],
      ["hi", 0.45],
    ],
  },
  {
    code: "ES",
    numericId: "724",
    name: "Spain",
    flag: "🇪🇸",
    sessions: 9640,
    growth: 0.018,
    languages: [
      ["es-419", 0.9],
      ["en", 0.1],
    ],
  },
  {
    code: "IT",
    numericId: "380",
    name: "Italy",
    flag: "🇮🇹",
    sessions: 8890,
    growth: 0.014,
    languages: [
      ["it", 0.91],
      ["en", 0.09],
    ],
  },
  {
    code: "CA",
    numericId: "124",
    name: "Canada",
    flag: "🇨🇦",
    sessions: 8130,
    growth: 0.006,
    languages: [
      ["en", 0.74],
      ["fr", 0.24],
      ["zh-Hans", 0.02],
    ],
  },
  {
    code: "SA",
    numericId: "682",
    name: "Saudi Arabia",
    flag: "🇸🇦",
    sessions: 7660,
    growth: 0.058,
    languages: [
      ["ar", 0.9],
      ["en", 0.1],
    ],
  },
  {
    code: "NL",
    numericId: "528",
    name: "Netherlands",
    flag: "🇳🇱",
    sessions: 7020,
    growth: 0.011,
    languages: [
      ["nl", 0.82],
      ["en", 0.18],
    ],
  },
  {
    code: "AR",
    numericId: "032",
    name: "Argentina",
    flag: "🇦🇷",
    sessions: 6540,
    growth: 0.033,
    languages: [
      ["es-419", 0.95],
      ["en", 0.05],
    ],
  },
  {
    code: "ID",
    numericId: "360",
    name: "Indonesia",
    flag: "🇮🇩",
    sessions: 6110,
    growth: 0.049,
    languages: [
      ["id", 0.9],
      ["en", 0.1],
    ],
  },
  {
    code: "VN",
    numericId: "704",
    name: "Vietnam",
    flag: "🇻🇳",
    sessions: 5480,
    growth: 0.056,
    languages: [
      ["vi", 0.93],
      ["en", 0.07],
    ],
  },
  {
    code: "AU",
    numericId: "036",
    name: "Australia",
    flag: "🇦🇺",
    sessions: 5230,
    growth: 0.005,
    languages: [
      ["en", 0.93],
      ["zh-Hans", 0.05],
      ["vi", 0.02],
    ],
  },
  {
    code: "CO",
    numericId: "170",
    name: "Colombia",
    flag: "🇨🇴",
    sessions: 4870,
    growth: 0.036,
    languages: [
      ["es-419", 0.96],
      ["en", 0.04],
    ],
  },
  {
    code: "EG",
    numericId: "818",
    name: "Egypt",
    flag: "🇪🇬",
    sessions: 4390,
    growth: 0.044,
    languages: [
      ["ar", 0.94],
      ["en", 0.06],
    ],
  },
  {
    code: "AE",
    numericId: "784",
    name: "United Arab Emirates",
    flag: "🇦🇪",
    sessions: 4160,
    growth: 0.031,
    languages: [
      ["ar", 0.55],
      ["en", 0.38],
      ["hi", 0.07],
    ],
  },
  {
    code: "CH",
    numericId: "756",
    name: "Switzerland",
    flag: "🇨🇭",
    sessions: 3720,
    growth: 0.007,
    languages: [
      ["de", 0.6],
      ["fr", 0.22],
      ["it", 0.08],
      ["en", 0.1],
    ],
  },
  {
    code: "AT",
    numericId: "040",
    name: "Austria",
    flag: "🇦🇹",
    sessions: 3350,
    growth: 0.009,
    languages: [
      ["de", 0.88],
      ["en", 0.09],
      ["tr", 0.03],
    ],
  },
  {
    code: "BE",
    numericId: "056",
    name: "Belgium",
    flag: "🇧🇪",
    sessions: 3090,
    growth: 0.008,
    languages: [
      ["nl", 0.55],
      ["fr", 0.36],
      ["en", 0.09],
    ],
  },
  {
    code: "PT",
    numericId: "620",
    name: "Portugal",
    flag: "🇵🇹",
    sessions: 2840,
    growth: 0.024,
    languages: [
      ["pt-BR", 0.9],
      ["en", 0.1],
    ],
  },
  {
    code: "CN",
    numericId: "156",
    name: "China",
    flag: "🇨🇳",
    sessions: 2610,
    growth: 0.017,
    languages: [
      ["zh-Hans", 0.97],
      ["en", 0.03],
    ],
  },
];

const TREND_WEEKS = 12;

/** Deterministic pseudo-random in [0, 1) — keeps the fixture stable across runs. */
function seededNoise(seed: string, week: number): number {
  let h = 2166136261;
  const s = `${seed}:${week}`;
  for (let i = 0; i < s.length; i++) {
    h ^= s.charCodeAt(i);
    h = Math.imul(h, 16777619);
  }
  return ((h >>> 0) % 1000) / 1000;
}

/** 12-week trend ending at `latest`, following the seed's growth slope with mild noise. */
function buildTrend(seed: string, latest: number, growth: number): number[] {
  const trend: number[] = [];
  for (let week = 0; week < TREND_WEEKS; week++) {
    const weeksBack = TREND_WEEKS - 1 - week;
    const base = latest / Math.pow(1 + growth, weeksBack);
    const noise = 1 + (seededNoise(seed, week) - 0.5) * 0.12;
    trend.push(Math.max(0, Math.round(base * noise)));
  }
  return trend;
}

/** How well a language request is served given current project coverage. */
function servedRateForLanguage(code: string): number {
  const coverage = coverageFor(code);
  if (coverage.status === "covered") return 0.97;
  if (coverage.status === "partial") return coverage.percent / 100;
  return 0;
}

function round4(n: number): number {
  return Math.round(n * 10000) / 10000;
}

function buildSnapshot(range: TimeRange): DemandSnapshot {
  // Longer windows scale volume; shares stay comparable (good enough for a mock).
  const scale = range === "7d" ? 1 : range === "30d" ? 4.2 : 12.4;

  const totalSessions = COUNTRY_SEEDS.reduce((sum, c) => sum + c.sessions, 0);

  const countries: CountryDemand[] = COUNTRY_SEEDS.map((seed) => {
    const sessions = Math.round(seed.sessions * scale);
    const servedRate = seed.languages.reduce(
      (sum, [lang, share]) => sum + share * servedRateForLanguage(lang),
      0,
    );
    return {
      code: seed.code,
      numericId: seed.numericId,
      name: seed.name,
      flag: seed.flag,
      sessions,
      share: round4(seed.sessions / totalSessions),
      servedRate: round4(servedRate),
      languages: seed.languages
        .map(([code, share]) => ({ code, share }))
        .sort((a, b) => b.share - a.share),
      // Trends stay weekly regardless of range — the chart is about shape.
      trend: buildTrend(`${range}:${seed.code}`, seed.sessions, seed.growth),
    };
  });

  // Aggregate language demand from the per-country splits so the two views
  // always tell the same story.
  const perLanguage = new Map<
    string,
    { sessions: number; countries: Map<string, number>; growth: number }
  >();
  for (const seed of COUNTRY_SEEDS) {
    for (const [lang, share] of seed.languages) {
      const langSessions = seed.sessions * share;
      const entry = perLanguage.get(lang) ?? { sessions: 0, countries: new Map(), growth: 0 };
      entry.sessions += langSessions;
      entry.countries.set(seed.code, (entry.countries.get(seed.code) ?? 0) + langSessions);
      perLanguage.set(lang, entry);
    }
  }
  for (const [lang, entry] of perLanguage) {
    // Demand-weighted growth so language trends match their countries.
    let weighted = 0;
    for (const seed of COUNTRY_SEEDS) {
      const share = seed.languages.find(([l]) => l === lang)?.[1] ?? 0;
      weighted += seed.sessions * share * seed.growth;
    }
    entry.growth = entry.sessions > 0 ? weighted / entry.sessions : 0;
  }

  const languages: LanguageDemand[] = [...perLanguage.entries()]
    .map(([code, entry]) => {
      const sessions = Math.round(entry.sessions * scale);
      const coverage = coverageFor(code);
      return {
        code,
        sessions,
        share: round4(entry.sessions / totalSessions),
        trend: buildTrend(`${range}:${code}`, entry.sessions, entry.growth),
        coverage,
        servedRate: round4(servedRateForLanguage(code)),
        countries: [...entry.countries.entries()]
          .map(([country, s]) => ({ code: country, share: round4(s / entry.sessions) }))
          .sort((a, b) => b.share - a.share),
        planEstimate: coverage.status === "covered" ? undefined : PLAN_ESTIMATES[code],
      };
    })
    .sort((a, b) => b.sessions - a.sessions);

  return {
    range,
    generatedAt: "2026-06-29T06:00:00Z",
    totalSessions: Math.round(totalSessions * scale),
    projectLocales: PROJECT_LOCALES,
    countries: [...countries].sort((a, b) => b.sessions - a.sessions),
    languages,
  };
}

const snapshots: Record<TimeRange, DemandSnapshot> = {
  "7d": buildSnapshot("7d"),
  "30d": buildSnapshot("30d"),
  "90d": buildSnapshot("90d"),
};

/** Sample-data implementation of the seam. Replace with the ingest-backed source. */
export const sampleDemandDataSource: DemandDataSource = {
  getSnapshot(range: TimeRange): DemandSnapshot {
    return snapshots[range];
  },
};

// ---------------------------------------------------------------------------
// Shared helpers
// ---------------------------------------------------------------------------

export function formatSessions(n: number): string {
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`;
  if (n >= 10_000) return `${Math.round(n / 1000)}K`;
  if (n >= 1000) return `${(n / 1000).toFixed(1)}K`;
  return `${n}`;
}

export function formatShare(share: number): string {
  const pct = share * 100;
  return `${pct >= 10 ? Math.round(pct) : pct.toFixed(1)}%`;
}

export function formatTokens(tokens: number): string {
  if (tokens >= 1_000_000) return `~${(tokens / 1_000_000).toFixed(1)}M tokens`;
  return `~${Math.round(tokens / 1000)}K tokens`;
}

/** One-line gap estimate, e.g. "~4,120 units · ~2.1M tokens". */
export function formatGapEstimate(estimate: PlanEstimate): string {
  return `~${estimate.units.toLocaleString("en-US")} units · ${formatTokens(estimate.tokens)}`;
}

export function countryByCode(snapshot: DemandSnapshot, code: string): CountryDemand | undefined {
  return snapshot.countries.find((c) => c.code === code);
}

export function languageByCode(snapshot: DemandSnapshot, code: string): LanguageDemand | undefined {
  return snapshot.languages.find((l) => l.code === code);
}
