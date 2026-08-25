/**
 * Reading and writing a coordinate filter — the `axis=value,axis=value` region
 * a grant names.
 *
 * One spelling, everywhere: the same form the token scope grammar takes after
 * its "@", the same form an audit line records, the same form this UI accepts.
 * A second spelling would be a second thing to keep in step with the server's
 * ParseCoordinateFilter.
 */

/** Renders a filter, axes sorted, so the text is stable whatever order it arrives in. */
export function formatCoordinateFilter(coordinates?: Record<string, string>): string {
  const axes = Object.keys(coordinates ?? {}).sort();
  return axes.map((axis) => `${axis}=${coordinates?.[axis] ?? ""}`).join(",");
}

export interface ParsedCoordinateFilter {
  /** The region, or undefined for the unconstrained filter. */
  coordinates?: Record<string, string>;
  /** Set when the text could not be read; the caller shows it and saves nothing. */
  error?: string;
}

/**
 * Reads the `axis=value` form. Empty text is the unconstrained filter — the
 * whole space — which is what a membership carries when it names no region.
 *
 * Mirrors the server's ParseCoordinateFilter, including its refusals: a half
 * written axis is rejected rather than dropped, because an axis a filter does
 * not name is an axis it does not constrain, so silently discarding one would
 * widen the grant instead of failing it.
 */
export function parseCoordinateFilter(text: string): ParsedCoordinateFilter {
  const trimmed = text.trim();
  if (trimmed === "") return {};

  const out: Record<string, string> = {};
  for (const rawPair of trimmed.split(",")) {
    const pair = rawPair.trim();
    if (pair === "") continue;
    const eq = pair.indexOf("=");
    const axis = eq === -1 ? "" : pair.slice(0, eq).trim();
    const value = eq === -1 ? "" : pair.slice(eq + 1).trim();
    if (axis === "" || value === "") {
      return { error: `"${pair}" is not axis=value` };
    }
    if (out[axis] !== undefined && out[axis] !== value) {
      return { error: `axis "${axis}" is given twice with different values` };
    }
    out[axis] = value;
  }
  return Object.keys(out).length === 0 ? {} : { coordinates: out };
}
