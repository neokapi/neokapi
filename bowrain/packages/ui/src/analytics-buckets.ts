/**
 * Bucketing helpers for client-side analytics properties — the TS mirror of
 * `bowrain/analytics`' `CountBucket` / `CreditBucket` / `PercentBucket`.
 *
 * A magnitude never rides on an event verbatim: sizes, credit amounts, and
 * shares are coarsened to a band first, so no event can carry an exact spend,
 * token count, or corpus size. The labels are byte-identical to the Go ones on
 * purpose — the same property on the server (`surface: "server"`) and web
 * (`surface: "web-app"`) surface must be comparable in one PostHog breakdown.
 * Change one side and you must change the other (and its test).
 */

/** Coarsens a count of units (blocks, segments, items) — mirrors CountBucket. */
export function countBucket(n: number): string {
  if (!Number.isFinite(n) || n <= 0) return "0";
  if (n <= 10) return "1_10";
  if (n <= 100) return "11_100";
  if (n <= 1000) return "101_1000";
  return "gt_1000";
}

/**
 * Coarsens a credit amount — mirrors CreditBucket. The edges are anchored on
 * the amounts the pricing model already names: the one-time new-workspace grant
 * and one purchased top-up pack are both 200_000 credits, so "does this run fit
 * inside the grant?" is read straight off the boundary at 200k.
 */
export function creditBucket(credits: number): string {
  if (!Number.isFinite(credits) || credits <= 0) return "0";
  if (credits <= 1_000) return "1_1k";
  if (credits <= 10_000) return "1k_10k";
  if (credits <= 50_000) return "10k_50k";
  if (credits <= 100_000) return "50k_100k";
  if (credits <= 200_000) return "100k_200k"; // upper edge == the grant / pack size
  if (credits <= 500_000) return "200k_500k";
  if (credits <= 1_000_000) return "500k_1m";
  return "gt_1m";
}

/** Bands a percentage (0–100), clamping out-of-range input — mirrors PercentBucket. */
export function percentBucket(pct: number): string {
  if (!Number.isFinite(pct) || pct <= 0) return "0";
  if (pct <= 25) return "1_25";
  if (pct <= 50) return "26_50";
  if (pct <= 75) return "51_75";
  if (pct < 100) return "76_99";
  return "100";
}

/**
 * Bands part/whole as a percentage — mirrors SharePercentBucket. A non-zero
 * part never collapses into "0" and a part short of the whole never reaches
 * "100", so the saturated bands keep meaning exactly none / exactly all.
 */
export function sharePercentBucket(part: number, whole: number): string {
  if (!Number.isFinite(part) || !Number.isFinite(whole) || whole <= 0 || part <= 0) return "0";
  if (part >= whole) return "100";
  let pct = Math.floor((part * 100) / whole);
  if (pct <= 0) pct = 1; // a real but tiny share belongs in the lowest non-zero band
  if (pct >= 100) pct = 99; // part < whole must not read as "everything"
  return percentBucket(pct);
}
