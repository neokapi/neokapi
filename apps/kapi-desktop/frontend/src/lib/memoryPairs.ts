// The pairs a content memory entry is listed as, and the conversions between a
// variant's runs and the shapes the editor and the store take.

import { flattenRuns, type Run } from "@neokapi/kapi-format";
import {
  runsToCoded,
  type MemoryEntryDTO,
  type SpanInfo,
  type VariantInputDTO,
} from "@neokapi/ui-primitives";

/** One row of the list: an entry's source and one of its other languages. */
export interface MemoryPair {
  /** `<entry id>::<target locale>`, unique in the list. */
  id: string;
  entry: MemoryEntryDTO;
  source: string;
  /** Empty for an entry that holds its source language alone. */
  target: string;
}

/** The language an entry is read from: its hint, else the first it holds. */
export function sourceLocaleOf(entry: MemoryEntryDTO): string {
  if (entry.hint_src_lang && entry.variants[entry.hint_src_lang]) return entry.hint_src_lang;
  return Object.keys(entry.variants)[0] ?? "";
}

/** The rows an entry contributes: one per language other than its source. */
export function pairsOf(entry: MemoryEntryDTO, targets?: ReadonlySet<string>): MemoryPair[] {
  const source = sourceLocaleOf(entry);
  const others = Object.keys(entry.variants).filter((l) => l !== source);
  if (others.length === 0) {
    return targets ? [] : [{ id: `${entry.id}::`, entry, source, target: "" }];
  }
  return others
    .filter((l) => !targets || targets.has(l))
    .map((target) => ({ id: `${entry.id}::${target}`, entry, source, target }));
}

/** A variant as the store takes it back: the runs, and the text they flatten to. */
export function variantInput(runs: Run[]): VariantInputDTO {
  const input: VariantInputDTO = { text: flattenRuns(runs) };
  if (runs.length > 0) input.runs = runs;
  return input;
}

/** The editor's shape for a variant. A plural or a select opens as plain text. */
export function editable(runs: Run[]): { codedText: string; spans: SpanInfo[] } {
  try {
    return runsToCoded(runs);
  } catch {
    return { codedText: "", spans: [] };
  }
}
