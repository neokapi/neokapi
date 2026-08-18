/**
 * Pure Run encoders. No Node-specific deps so the neokapi-i18n runtime
 * (browser) can pull these in without dragging any Node-only
 * toolchain into the bundle.
 */

import type { Run } from "./block.ts";
import { projectRunsText, type ModelRunSpec } from "./run-projection.ts";

/**
 * The runtime's string form: every kind of run is representable, so this
 * projection drops nothing. It is the reference for what a lossless run
 * projection looks like — `t()` / `tx()` reconstruct the typed sequence from
 * the string, which is only possible because nothing left it.
 */
const RUNTIME_TEXT: ModelRunSpec<string> = {
  text: (r) => r.text,
  ph: (r) => `{${r.ph.equiv || r.ph.id}}`,
  // Paired codes keep their content; the markers let tx() re-attach elements.
  pcOpen: (r) => `{=m${r.pcOpen.id}}`,
  pcClose: (r) => `{/=m${r.pcClose.id}}`,
  sub: (r) => `[${r.sub.equiv || r.sub.id}]`,
  // ICU syntax, so the runtime's resolveICU picks the form at render time.
  plural: (r) => `{${r.plural.pivot}, plural, ${branches(r.plural.forms)}}`,
  select: (r) => `{${r.select.pivot}, select, ${branches(r.select.cases)}}`,
  fallback: (kind) => `{${kind}}`,
};

function branches(byKey: Record<string, Run[]>): string {
  return Object.entries(byKey)
    .map(([key, runs]) => `${key} {${flattenRuns(runs)}}`)
    .join(" ");
}

/**
 * Flatten a Run sequence to the string shape the neokapi-i18n runtime
 * consumes via `t()` and `tx()`: placeholders become `{equiv}`
 * tokens, paired codes keep their content with `{=<id>...}` markers
 * for `tx()` to re-attach elements, subblocks become `[equiv]`.
 * Plural / select emit ICU syntax so the runtime's resolveICU picks
 * the right form at render time.
 */
export function flattenRuns(runs: Run[]): string {
  return projectRunsText(runs, RUNTIME_TEXT);
}
