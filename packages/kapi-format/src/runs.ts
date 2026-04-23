/**
 * Pure Run encoders. No Node-specific deps so the kapi-react runtime
 * (browser) can pull these in without dragging any Node-only
 * toolchain into the bundle.
 */

import type { Run } from "./block.ts";

/**
 * Flatten a Run sequence to the string shape the kapi-react runtime
 * consumes via `t()` and `tx()`: placeholders become `{equiv}`
 * tokens, paired codes keep their content with `{=<id>...}` markers
 * for `tx()` to re-attach elements, subblocks become `[equiv]`.
 * Plural / select emit ICU syntax so the runtime's resolveICU picks
 * the right form at render time.
 */
export function flattenRuns(runs: Run[]): string {
  let out = "";
  for (const r of runs) {
    if ("text" in r) {
      out += r.text;
    } else if ("ph" in r) {
      out += `{${r.ph.equiv || r.ph.id}}`;
    } else if ("pcOpen" in r) {
      out += `{=m${r.pcOpen.id}}`;
    } else if ("pcClose" in r) {
      out += `{/=m${r.pcClose.id}}`;
    } else if ("sub" in r) {
      out += `[${r.sub.equiv || r.sub.id}]`;
    } else if ("plural" in r) {
      const forms = Object.entries(r.plural.forms)
        .map(([k, v]) => `${k} {${flattenRuns(v)}}`)
        .join(" ");
      out += `{${r.plural.pivot}, plural, ${forms}}`;
    } else if ("select" in r) {
      const cases = Object.entries(r.select.cases)
        .map(([k, v]) => `${k} {${flattenRuns(v)}}`)
        .join(" ");
      out += `{${r.select.pivot}, select, ${cases}}`;
    }
  }
  return out;
}
