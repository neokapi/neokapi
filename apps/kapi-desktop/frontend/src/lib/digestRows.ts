// The rows of a project's digest in the order What's new lists them, and where
// the selection goes as a person works down the list.
//
// The list has five groups: Needs you (one row per disagreement), Suggested
// (every suggestion, theme by theme and collection by collection, as the
// digest orders them), Established (rules that came into force since the
// person last looked), Earlier (rules in force from before that) and Drift
// (rules the content is moving away from). A row's key names its group and
// its item, because one rule can sit in Established and in Drift at once.

import type { ContextDigest, DigestConflict, DigestDrift, DigestItem } from "../types/api";

/** The list's groups, in the order they are shown. */
export const DIGEST_ROW_GROUPS = [
  "conflict",
  "suggested",
  "established",
  "earlier",
  "drift",
] as const;

export type DigestRowGroup = (typeof DIGEST_ROW_GROUPS)[number];

/** One row of the list. */
export interface DigestRow {
  /** Unique within the list: the group and the item's id. */
  key: string;
  group: DigestRowGroup;
  /** The item the row stands for. A disagreement's row stands for its first side. */
  item: DigestItem;
  /** The disagreement, for a row in Needs you. */
  conflict?: DigestConflict;
  /** What the content is doing, for a row in Drift. */
  drift?: DigestDrift;
  /**
   * The suggestions that share a suggestion's theme and collection: the group
   * `g` keeps in one go.
   */
  siblings?: DigestItem[];
}

const rowKey = (group: DigestRowGroup, id: string) => `${group}:${id}`;

/** Every row of a digest, grouped and in the order the list shows them. */
export function digestRows(digest: ContextDigest): DigestRow[] {
  const byGroup: Record<DigestRowGroup, DigestRow[]> = {
    conflict: [],
    suggested: [],
    established: [],
    earlier: [],
    drift: [],
  };
  for (const conflict of digest.conflicts) {
    const first = conflict.sides[0];
    if (first) {
      byGroup.conflict.push({
        key: rowKey("conflict", first.id),
        group: "conflict",
        item: first,
        conflict,
      });
    }
  }
  for (const theme of digest.suggested) {
    for (const g of theme.groups) {
      for (const item of g.items) {
        byGroup.suggested.push({
          key: rowKey("suggested", item.id),
          group: "suggested",
          item,
          siblings: g.items,
        });
      }
    }
  }
  for (const item of digest.established) {
    const group = item.new ? "established" : "earlier";
    byGroup[group].push({ key: rowKey(group, item.id), group, item });
  }
  for (const drift of digest.drift) {
    byGroup.drift.push({
      key: rowKey("drift", drift.rule.id),
      group: "drift",
      item: drift.rule,
      drift,
    });
  }
  return DIGEST_ROW_GROUPS.flatMap((g) => byGroup[g]);
}

/** A row a person decides about: a disagreement or a suggestion. */
export function awaitsDecision(row: DigestRow): boolean {
  return row.group === "conflict" || row.group === "suggested";
}

/**
 * The row the list opens on: the first that needs a person, else the first
 * suggestion, else none. Rows in `skip` (decided, and still on screen until the
 * digest is read again) are passed over.
 */
export function defaultRowKey(
  rows: readonly DigestRow[],
  skip: ReadonlySet<string> = new Set(),
): string | null {
  const open = rows.filter((r) => !skip.has(r.key));
  const first =
    open.find((r) => r.group === "conflict") ?? open.find((r) => r.group === "suggested");
  return first?.key ?? null;
}

/**
 * Where the selection goes once the row at `key` is decided, so a person can
 * work down the list with one hand. After a disagreement or a suggestion it is
 * the next row that awaits a decision, else the nearest one above; after a rule
 * in force it is the next row of any group, else the one above. None is left
 * when nothing else qualifies. Rows in `skip` are passed over.
 */
export function nextRowKey(
  rows: readonly DigestRow[],
  key: string,
  skip: ReadonlySet<string> = new Set(),
): string | null {
  const at = rows.findIndex((r) => r.key === key);
  if (at === -1) return defaultRowKey(rows, skip);
  const decided = rows[at];
  const qualifies = (r: DigestRow) =>
    r.key !== key && !skip.has(r.key) && (!awaitsDecision(decided) || awaitsDecision(r));
  for (let i = at + 1; i < rows.length; i++) if (qualifies(rows[i])) return rows[i].key;
  for (let i = at - 1; i >= 0; i--) if (qualifies(rows[i])) return rows[i].key;
  return null;
}
