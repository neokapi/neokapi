/**
 * Adapter from bowrain's `BlockInfo` (REST/Wails API shape) to the
 * minimal `@neokapi/kapi-format` Block fields that editor primitives
 * need (`source` runs + `placeholders` table for pivot candidates).
 *
 * Intentionally narrow — bowrain doesn't carry typed Placeholder
 * data on every block, so we synthesise the table from
 * `source_spans`. Numeric-equiv spans naturally surface as the
 * first plural-pivot candidates, matching `pluralPivotCandidates`
 * in `@neokapi/kapi-format/target-plural`.
 *
 * Lives outside `PluralTargetCell` / `UnifiedTargetEditor` so both
 * (current and future) editor surfaces share the same adaptation.
 */

import type { Block, Placeholder } from "@neokapi/kapi-format";

import type { BlockInfo, SpanInfo } from "../types/api";

export function toKapiBlock(block: BlockInfo): Pick<Block, "source" | "placeholders"> {
  return {
    source: [{ text: block.source }],
    placeholders: spansToPlaceholders(block.source_spans),
  };
}

function spansToPlaceholders(spans: readonly SpanInfo[] | undefined): Placeholder[] {
  if (!spans) return [];
  const out: Placeholder[] = [];
  const seen = new Set<string>();
  for (const span of spans) {
    // Paired codes contribute opening + closing with the same equiv;
    // dedupe by name.
    const name = span.equiv_text?.trim();
    if (!name || seen.has(name)) continue;
    seen.add(name);
    out.push({
      name,
      kind: span.span_type === "placeholder" ? "variable" : "element",
      sourceExpr: span.data,
    });
  }
  return out;
}
