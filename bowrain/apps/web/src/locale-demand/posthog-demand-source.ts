// ---------------------------------------------------------------------------
// Locale demand — live PostHog source (phase 0, read-only).
//
// `PostHogDemandSource` implements the `DemandDataSource` seam by calling the
// bowrain server's PostHog connector endpoints
// (/:ws/:id/connectors/posthog/demand) and mapping the wire snapshot into the
// page's `DemandSnapshot` shape. The server owns the PostHog personal API key
// (sealed at rest, masked on read) and caches snapshots per (project, range)
// for an hour — this module never talks to PostHog directly.
//
// Mapping honesty rules:
// - What the source cannot derive is *absent*, not zero: per-market/-language
//   served rates are null ("—" in the UI), missing trends are empty arrays
//   ("no data", muted), and countries PostHog reported without a resolvable
//   ISO code fall off the map but stay in the totals.
// - Coverage is a plain locale match against the project's configured
//   locales (covered / not-covered). Partial coverage and plan estimates
//   need the plan machinery — phase 1.
// ---------------------------------------------------------------------------

import type { ApiAdapter, PostHogDemandResponse } from "@neokapi/ui";
import { countryDisplayName, countryFlagEmoji, countryNumericId } from "./country-codes";
import type {
  CountryDemand,
  DemandDataSource,
  DemandSnapshot,
  LanguageDemand,
  TimeRange,
} from "./locale-demand-fixtures";

/** Classified error from the PostHog connector API. */
export type PostHogApiErrorCode =
  | "bad_host"
  | "bad_key"
  | "bad_project"
  | "bad_pattern"
  | "bad_range"
  | "not_configured"
  | "query_error"
  | "unknown";

export interface PostHogApiError {
  code: PostHogApiErrorCode;
  message: string;
}

/**
 * Parse an error thrown by the RestApiAdapter (`Error("<status>: <json>")`)
 * into the server's classified PostHog error ({error, code}). Anything that
 * doesn't carry a known code comes back as "unknown" with the best message
 * we can extract.
 */
export function parsePostHogApiError(err: unknown): PostHogApiError {
  const raw = err instanceof Error ? err.message : String(err);
  const body = raw.replace(/^\d{3}:\s*/, "");
  try {
    const parsed = JSON.parse(body) as { error?: string; code?: string };
    const code = (
      [
        "bad_host",
        "bad_key",
        "bad_project",
        "bad_pattern",
        "bad_range",
        "not_configured",
        "query_error",
      ] as const
    ).find((c) => c === parsed.code);
    return { code: code ?? "unknown", message: parsed.error || raw };
  } catch {
    return { code: "unknown", message: raw };
  }
}

/** Primary-language subtag of a normalized BCP-47 tag ("pt-BR" → "pt"). */
function langBase(tag: string): string {
  return tag.split("-")[0]?.toLowerCase() ?? "";
}

/**
 * Coverage from a plain locale match: covered when the project ships the
 * exact tag or the same base language. Partial coverage needs per-locale
 * completion stats the connector doesn't have (phase 1).
 */
function coverageFromProjectLocales(
  tag: string,
  projectLocales: string[],
): LanguageDemand["coverage"] {
  const wanted = tag.toLowerCase();
  const wantedBase = langBase(tag);
  for (const locale of projectLocales) {
    if (locale.toLowerCase() === wanted || langBase(locale) === wantedBase) {
      return { status: "covered" };
    }
  }
  return { status: "not-covered" };
}

function round4(n: number): number {
  return Math.round(n * 10000) / 10000;
}

/**
 * Pure mapping from the server's wire snapshot to the page's DemandSnapshot.
 * Exported for tests.
 */
export function mapPostHogDemand(
  resp: PostHogDemandResponse,
  projectLocales: string[],
): DemandSnapshot {
  const totalSessions = resp.total_sessions;
  const wireCountries = resp.countries ?? [];
  const wireLanguages = resp.languages ?? [];

  const countries: CountryDemand[] = wireCountries.map((c) => {
    const code = c.country_code.toUpperCase();
    const languages = (c.languages ?? [])
      .filter((l) => l.tag !== "unknown")
      .map((l) => ({
        code: l.tag,
        share: c.sessions > 0 ? round4(l.sessions / c.sessions) : 0,
      }))
      .sort((a, b) => b.share - a.share);
    return {
      code,
      numericId: countryNumericId(code),
      name: countryDisplayName(code),
      flag: countryFlagEmoji(code),
      sessions: c.sessions,
      share: totalSessions > 0 ? round4(c.sessions / totalSessions) : 0,
      // Per-market served rate is not derivable from PostHog in phase 0.
      servedRate: null,
      languages,
      // PostHog trends are per language only; no per-country series.
      trend: [],
    };
  });

  // Invert the per-country language mix into per-language country origins.
  const languageCountrySessions = new Map<string, { code: string; sessions: number }[]>();
  for (const c of wireCountries) {
    for (const l of c.languages ?? []) {
      const list = languageCountrySessions.get(l.tag) ?? [];
      list.push({ code: c.country_code.toUpperCase(), sessions: l.sessions });
      languageCountrySessions.set(l.tag, list);
    }
  }

  const languages: LanguageDemand[] = wireLanguages
    .filter((l) => l.tag !== "unknown")
    .map((l) => {
      const origins = (languageCountrySessions.get(l.tag) ?? []).sort(
        (a, b) => b.sessions - a.sessions,
      );
      return {
        code: l.tag,
        sessions: l.sessions,
        share: totalSessions > 0 ? round4(l.sessions / totalSessions) : 0,
        trend: (l.trend ?? []).map((p) => p.sessions),
        coverage: coverageFromProjectLocales(l.tag, projectLocales),
        // Per-language served rate is not derivable from PostHog in phase 0
        // (the server computes only a site-wide hit rate).
        servedRate: null,
        countries: origins.map((s) => ({
          code: s.code,
          share: l.sessions > 0 ? round4(s.sessions / l.sessions) : 0,
        })),
        // Plan estimates come from the plan machinery — phase 1.
        planEstimate: undefined,
      };
    });

  return {
    range: resp.range as TimeRange,
    generatedAt: resp.generated_at,
    totalSessions,
    projectLocales,
    countries,
    languages,
    provenance: {
      kind: "posthog",
      hostLabel: resp.source.host_label,
      posthogProjectId: resp.source.posthog_project_id,
      cachedAt: resp.cached_at,
    },
    warnings: resp.warnings ?? undefined,
  };
}

/** The slice of the ApiAdapter the live source needs — easy to mock. */
export type PostHogDemandApi = Pick<ApiAdapter, "getPostHogDemand">;

/**
 * Live PostHog implementation of the DemandDataSource seam. Throws the
 * adapter's error on failure — callers classify it with
 * `parsePostHogApiError` (a `not_configured` code means "fall back to the
 * sample dataset").
 */
export class PostHogDemandSource implements DemandDataSource {
  constructor(
    private readonly api: PostHogDemandApi,
    private readonly workspaceSlug: string,
    private readonly projectId: string,
    private readonly projectLocales: string[],
  ) {}

  async getSnapshot(range: TimeRange): Promise<DemandSnapshot> {
    const resp = await this.api.getPostHogDemand(this.workspaceSlug, this.projectId, range);
    return mapPostHogDemand(resp, this.projectLocales);
  }
}
