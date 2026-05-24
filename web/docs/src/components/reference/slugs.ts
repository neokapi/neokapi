// Stable slug helpers for the static per-entry reference pages (R4, #673).
//
// Every command / format / tool gets a crawlable URL under
// /reference/{commands,formats,tools}/<slug>. The slug must be:
//   • deterministic — the generator and the grid links derive it the same way,
//   • collision-free — two entries never map to the same page.
//
// These helpers are the single source of truth, imported by BOTH the build-time
// generator (scripts/gen-reference-pages.ts) and the runtime grids/cards so the
// `?id=` modal's canonical "open the static page" links always match the files
// on disk.

import type { CommandEntry, ReferenceEntry } from "@neokapi/reference-data";

/** Lower-case, hyphenate, and strip anything unsafe for a URL path segment. */
function sanitize(s: string): string {
  return s
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-+|-+$/g, "");
}

/**
 * Command slug. Command ids are dot-joined paths (e.g. "formats.info") and are
 * already globally unique, so we just swap dots for hyphens to keep the URL
 * clean (e.g. /reference/commands/formats-info).
 */
export function commandSlug(cmd: CommandEntry): string {
  return cmd.path.map(sanitize).join("-");
}

/**
 * Format slug. Format ids are globally unique already (built-in ids are bare,
 * Okapi-bridge ids carry the `okf_` prefix), so the sanitized id is enough —
 * e.g. "json" → "json", "okf_html5" → "okf-html5".
 */
export function formatSlug(entry: ReferenceEntry): string {
  return sanitize(entry.id);
}

/**
 * Tool slug. Tool ids collide across sources — `pseudo-translate`,
 * `word-count`, `create-target`, … exist as BOTH a built-in and an Okapi-bridge
 * tool. `source:id` is unique, so a built-in tool keeps its bare id and an
 * Okapi-bridge tool is suffixed with `-okapi` only when its id also exists as a
 * built-in. Non-colliding Okapi tools keep their bare id so most URLs stay
 * clean.
 *
 * `builtinIds` is the set of built-in tool ids in the dataset; pass it so the
 * decision is made over the whole list (both the generator and the grid build
 * it from the same `tools.entries`).
 */
export function toolSlug(entry: ReferenceEntry, builtinIds: ReadonlySet<string>): string {
  const base = sanitize(entry.id);
  if (entry.source === "okapi" && builtinIds.has(entry.id)) {
    return `${base}-okapi`;
  }
  return base;
}

/** Build the set of built-in tool ids from a tool entry list. */
export function builtinToolIds(entries: readonly ReferenceEntry[]): Set<string> {
  const set = new Set<string>();
  for (const e of entries) if (e.source === "built-in") set.add(e.id);
  return set;
}

/** Public route path for an entry's static page (without the docs baseUrl). */
export function commandHref(cmd: CommandEntry): string {
  return `/reference/commands/${commandSlug(cmd)}`;
}
export function formatHref(entry: ReferenceEntry): string {
  return `/reference/formats/${formatSlug(entry)}`;
}
export function toolHref(entry: ReferenceEntry, builtinIds: ReadonlySet<string>): string {
  return `/reference/tools/${toolSlug(entry, builtinIds)}`;
}
