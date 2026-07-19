import type { BlockInfo, QAIssue } from "../../types/api";
import { getBlockStatus, getTargetText } from "../editor/blockStatus";

/**
 * Pure model + helpers for the governed review session's queue.
 *
 * A review session gathers every block that is *pending review* across all of a
 * project's items and target locales into one flat, ordered queue, then lets a
 * reviewer clear it in a single focused flow (keyboard-first, approve → advance).
 * This module owns the queue's data shape and all the grouping / filtering /
 * navigation logic, kept free of React so it is exhaustively unit-testable and
 * shared by the list panel, the focused reviewer, and the session orchestrator.
 */

/**
 * The check bucket a pending entry falls in — the third axis a reviewer filters
 * by (alongside item and locale): a block with an error-severity finding is
 * `failing`; one flagged off-brand (a brand-dimension finding, or an explicit
 * per-block on-brand:false signal) is `off_brand`; everything else is `clean`
 * (passes checks and the on-brand bar — the set "Approve all passing" clears).
 */
export type ReviewCheckStatus = "failing" | "off_brand" | "clean";

/** How the queue list groups its rows. */
export type ReviewGroupBy = "item" | "locale" | "check";

/** Active filter over the queue. An unset field imposes no constraint. */
export interface ReviewQueueFilter {
  itemId?: string;
  locale?: string;
  check?: ReviewCheckStatus;
}

/**
 * One pending block awaiting review in one locale, carrying everything the
 * reviewer needs to render and act faithfully: the full (runs-normalised)
 * block for source/target rendering + edit, the block's check findings for
 * that locale, and an optional per-block on-brand signal.
 */
export interface ReviewEntry {
  /** Stable identity: `${itemId}::${blockId}::${locale}`. */
  id: string;
  itemId: string;
  itemName: string;
  format?: string;
  collectionId?: string;
  locale: string;
  /** Full block (source_runs/targets_runs already normalised to coded+spans). */
  block: BlockInfo;
  /** Check findings for this block+locale; empty until QA has been loaded. */
  issues: QAIssue[];
  /** Per-block on-brand signal; `undefined` when not yet determined. */
  onBrand?: boolean;
}

/** One grouped section of the queue list. */
export interface ReviewGroup {
  /** Group discriminator (itemId, locale, or check-status). */
  key: string;
  /** Human label for the section header. */
  label: string;
  entries: ReviewEntry[];
}

/** Live counts over a set of entries, for the header and group chips. */
export interface ReviewQueueCounts {
  total: number;
  failing: number;
  offBrand: number;
  clean: number;
  /** Pending entries per locale (locale → count). */
  byLocale: Record<string, number>;
  /** Pending entries per item (itemId → count). */
  byItem: Record<string, number>;
}

/** Stable entry id from its coordinates. */
export function entryKey(itemId: string, blockId: string, locale: string): string {
  return `${itemId}::${blockId}::${locale}`;
}

/** Whether an entry carries an error-severity finding. */
export function entryHasErrors(entry: ReviewEntry): boolean {
  return entry.issues.some((i) => i.severity === "error");
}

/**
 * Whether a block is pending review for a locale: it is translatable, has a
 * non-empty committed target, and that target is not yet reviewed/signed-off.
 * Mirrors the ship model's "translated but not approved" per-block, so the
 * queue and the dashboard's `translated − approved` counts agree.
 */
export function isPendingReview(block: BlockInfo, locale: string): boolean {
  if (!block.translatable) return false;
  if (!getTargetText(block, locale).trim()) return false;
  const status = getBlockStatus(block, locale);
  // draft + translated are pending; reviewed maps from reviewed/signed-off.
  return status === "draft" || status === "translated";
}

/** The check bucket for an entry (see ReviewCheckStatus). */
export function entryCheckStatus(entry: ReviewEntry): ReviewCheckStatus {
  if (entryHasErrors(entry)) return "failing";
  if (entry.onBrand === false) return "off_brand";
  return "clean";
}

/** Whether an entry passes checks + the on-brand bar (a "passing" block). */
export function isEntryPassing(entry: ReviewEntry): boolean {
  return entryCheckStatus(entry) === "clean";
}

/** Whether an entry satisfies every set field of a filter. */
export function matchesFilter(entry: ReviewEntry, filter: ReviewQueueFilter): boolean {
  if (filter.itemId && entry.itemId !== filter.itemId) return false;
  if (filter.locale && entry.locale !== filter.locale) return false;
  if (filter.check && entryCheckStatus(entry) !== filter.check) return false;
  return true;
}

/** Entries matching the filter, order preserved. */
export function filterEntries(
  entries: readonly ReviewEntry[],
  filter: ReviewQueueFilter,
): ReviewEntry[] {
  return entries.filter((e) => matchesFilter(e, filter));
}

const CHECK_LABELS: Record<ReviewCheckStatus, string> = {
  failing: "Failing checks",
  off_brand: "Off-brand",
  clean: "Clean",
};

/** Order groups deterministically: failing first, then off-brand, then clean. */
const CHECK_ORDER: ReviewCheckStatus[] = ["failing", "off_brand", "clean"];

/**
 * Group entries for the list panel. Insertion order is preserved within a
 * group; group order is first-appearance for item/locale, and severity order
 * (failing → off-brand → clean) for check-status.
 */
export function groupEntries(
  entries: readonly ReviewEntry[],
  groupBy: ReviewGroupBy,
): ReviewGroup[] {
  const map = new Map<string, ReviewGroup>();
  for (const entry of entries) {
    let key: string;
    let label: string;
    if (groupBy === "item") {
      key = entry.itemId;
      label = entry.itemName;
    } else if (groupBy === "locale") {
      key = entry.locale;
      label = entry.locale;
    } else {
      const status = entryCheckStatus(entry);
      key = status;
      label = CHECK_LABELS[status];
    }
    const existing = map.get(key);
    if (existing) existing.entries.push(entry);
    else map.set(key, { key, label, entries: [entry] });
  }
  const groups = [...map.values()];
  if (groupBy === "check") {
    groups.sort(
      (a, b) =>
        CHECK_ORDER.indexOf(a.key as ReviewCheckStatus) -
        CHECK_ORDER.indexOf(b.key as ReviewCheckStatus),
    );
  }
  return groups;
}

/** Live counts over a set of entries. */
export function queueCounts(entries: readonly ReviewEntry[]): ReviewQueueCounts {
  const counts: ReviewQueueCounts = {
    total: entries.length,
    failing: 0,
    offBrand: 0,
    clean: 0,
    byLocale: {},
    byItem: {},
  };
  for (const entry of entries) {
    const status = entryCheckStatus(entry);
    if (status === "failing") counts.failing++;
    else if (status === "off_brand") counts.offBrand++;
    else counts.clean++;
    counts.byLocale[entry.locale] = (counts.byLocale[entry.locale] ?? 0) + 1;
    counts.byItem[entry.itemId] = (counts.byItem[entry.itemId] ?? 0) + 1;
  }
  return counts;
}

/** Number of passing (clean) entries — the "Approve all passing" preview. */
export function passingCount(entries: readonly ReviewEntry[]): number {
  return entries.reduce((n, e) => (isEntryPassing(e) ? n + 1 : n), 0);
}

/**
 * Clamp a keyboard move (j/k, ↑/↓) to a valid index. Returns `current` when
 * the list is empty. Movement does not wrap — the ends are hard stops, so a
 * held key rests on the first/last row rather than looping.
 */
export function nextIndex(length: number, current: number, delta: number): number {
  if (length <= 0) return current;
  return Math.min(Math.max(current + delta, 0), length - 1);
}

/**
 * The index to focus after the entry at `removedIndex` is cleared (approved or
 * rejected) from a list of the given pre-removal length. The next pending entry
 * slides into the freed slot, so focus stays put — clamped to the new last
 * index, and `0` once the queue empties (callers should show completion).
 */
export function indexAfterRemoval(length: number, removedIndex: number): number {
  const newLength = length - 1;
  if (newLength <= 0) return 0;
  return Math.min(removedIndex, newLength - 1);
}
