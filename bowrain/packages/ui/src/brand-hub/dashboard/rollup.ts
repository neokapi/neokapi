// Client-side brand-compliance rollup aggregation (AD-021). The web app reads
// the server's rollup endpoint directly, but the desktop app has no endpoint to
// proxy — it composes the same per-project brand reads it already uses and folds
// them here, so both surfaces show the same all-projects board. Pure and
// unit-tested; mirrors the server's aggregation in bowrain/server/handlers_brand_rollup.go.
import type { BrandRollup, BrandRollupEntry, DriftResult, StoredScore } from "../../brand/types";
import { aggregateDimensions, averageScore, latestChecked, trendDirection } from "./metrics";

/** Per-project inputs a client composes from the per-project brand reads. */
export interface RollupProjectInput {
  id: string;
  name: string;
  scores: StoredScore[];
  drift?: DriftResult;
}

/**
 * rollupEntry folds one project's stored scores + drift into a rollup row: the
 * rounded-mean overall score, per-dimension breakdown, block count, last-check
 * timestamp, and trend direction — the same fields the server rollup returns
 * (minus the effective profile, which is a server-side ladder resolution).
 */
export function rollupEntry(input: RollupProjectInput): BrandRollupEntry {
  return {
    project_id: input.id,
    project_name: input.name,
    overall: averageScore(input.scores),
    dimensions: aggregateDimensions(input.scores),
    scored_blocks: input.scores.length,
    last_scored_at: latestChecked(input.scores),
    trend: trendDirection(input.drift),
    drift: input.drift,
  };
}

/**
 * aggregateBrandRollup builds a paginated workspace rollup from per-project
 * inputs. Projects are ordered by name (id breaks ties) so the page is stable,
 * then the [offset, offset+limit) window is materialised.
 */
export function aggregateBrandRollup(
  inputs: RollupProjectInput[],
  offset: number,
  limit: number,
): BrandRollup {
  const sorted = [...inputs].sort(
    (a, b) => a.name.localeCompare(b.name) || a.id.localeCompare(b.id),
  );
  return {
    projects: sorted.slice(offset, offset + limit).map(rollupEntry),
    total: sorted.length,
    limit,
    offset,
  };
}
