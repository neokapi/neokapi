/**
 * Adapter from bowrain's `BlockInfo` (REST/Wails API shape) to the
 * minimal `@neokapi/kapi-format` Block fields that editor primitives
 * need (the source edition + `placeholders` table for pivot candidates).
 *
 * Intentionally narrow: bowrain doesn't carry typed Placeholder data on
 * every block, so the table is read from the source runs. A variable
 * surfaces as a plural-pivot candidate, a paired code as an element, and a
 * source that is a plural names its pivot, matching `pluralPivotCandidates`
 * in `@neokapi/kapi-format/target-plural`.
 *
 * Lives outside `PluralTargetCell` / `UnifiedTargetEditor` so both
 * (current and future) editor surfaces share the same adaptation.
 */

import type { Block, Placeholder, Run } from "@neokapi/kapi-format";
import { projectRuns, sourceEditions, type ModelRunSpec } from "@neokapi/kapi-format";

import { sourceRuns } from "./editor/blockContent";
import type { BlockInfo } from "../types/api";

export function toKapiBlock(block: BlockInfo): Pick<Block, "editions" | "placeholders"> {
  const runs = sourceRuns(block);
  return {
    editions: sourceEditions(runs),
    placeholders: placeholdersOf(runs),
  };
}

/**
 * The placeholder each run declares. A closing code declares nothing: its
 * opening half already did. A plural or select declares its pivot and
 * whatever its branches hold.
 */
const PLACEHOLDERS: ModelRunSpec<Placeholder[]> = {
  text: () => [],
  ph: (r) => [{ name: r.ph.equiv, kind: "variable", sourceExpr: r.ph.data }],
  pcOpen: (r) => [{ name: r.pcOpen.equiv, kind: "element", sourceExpr: r.pcOpen.data }],
  pcClose: { dropped: "the opening half of the pair declares the placeholder" },
  sub: (r) => [{ name: r.sub.equiv, kind: "variable", sourceExpr: `[${r.sub.equiv}]` }],
  plural: (r) => [
    { name: r.plural.pivot, kind: "icu-pivot", jsType: "number", sourceExpr: r.plural.pivot },
    ...Object.values(r.plural.forms).flatMap((form) => placeholdersOf(form)),
  ],
  select: (r) => [
    { name: r.select.pivot, kind: "icu-pivot", sourceExpr: r.select.pivot },
    ...Object.values(r.select.cases).flatMap((c) => placeholdersOf(c)),
  ],
  fallback: () => [],
};

/** The placeholders a run sequence references, one entry per name. */
function placeholdersOf(runs: readonly Run[]): Placeholder[] {
  const out: Placeholder[] = [];
  const seen = new Set<string>();
  for (const placeholder of projectRuns(runs, PLACEHOLDERS).flat()) {
    const name = placeholder.name.trim();
    if (!name || seen.has(name)) continue;
    seen.add(name);
    out.push({ ...placeholder, name });
  }
  return out;
}
