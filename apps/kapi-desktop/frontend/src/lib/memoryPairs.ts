// The rows a content memory is listed as, and the conversions between a
// variant's runs and the shapes the editor and the store take.

import { flattenRuns, type Run } from "@neokapi/kapi-format";
import {
  runsToCoded,
  type MemoryEntryDTO,
  type SpanInfo,
  type VariantInputDTO,
} from "@neokapi/ui-primitives";

/**
 * One row of the list: a source wording and every language it is held in.
 *
 * Entries that share their source language and wording are one row, so a
 * sentence kept in five languages reads as one line with five language pills
 * rather than five lines repeating it.
 */
export interface MemoryRow {
  /** The first entry's id, unique in the list. */
  id: string;
  /** The source language every entry in the row is read from. */
  source: string;
  /** The entries in the row, in the order the store listed them. */
  entries: MemoryEntryDTO[];
  /** The other languages the row holds, in the order first seen. */
  targets: string[];
}

/** The language an entry is read from: its hint, else the first it holds. */
export function sourceLocaleOf(entry: MemoryEntryDTO): string {
  if (entry.hint_src_lang && entry.variants[entry.hint_src_lang]) return entry.hint_src_lang;
  return Object.keys(entry.variants)[0] ?? "";
}

/** The wording a row groups on: the source variant's text, or its runs flattened. */
function sourceTextOf(entry: MemoryEntryDTO, source: string): string {
  const variant = entry.variants[source];
  if (!variant) return "";
  return variant.text || flattenRuns(variant.runs ?? []);
}

/**
 * The rows a list of entries is shown as: one per source language and
 * wording. With `targets`, an entry holding none of those languages is left
 * out, and a row lists only the languages asked for.
 */
export function rowsOf(entries: MemoryEntryDTO[], targets?: ReadonlySet<string>): MemoryRow[] {
  const rows: MemoryRow[] = [];
  const byKey = new Map<string, MemoryRow>();
  for (const entry of entries) {
    const source = sourceLocaleOf(entry);
    const others = Object.keys(entry.variants).filter(
      (l) => l !== source && (!targets || targets.has(l)),
    );
    if (targets && others.length === 0) continue;
    const key = `${source}\u0000${sourceTextOf(entry, source)}`;
    let row = byKey.get(key);
    if (!row) {
      row = { id: entry.id, source, entries: [], targets: [] };
      byKey.set(key, row);
      rows.push(row);
    }
    if (!row.entries.some((e) => e.id === entry.id)) row.entries.push(entry);
    for (const locale of others) if (!row.targets.includes(locale)) row.targets.push(locale);
  }
  return rows;
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
