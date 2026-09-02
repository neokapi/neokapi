import type { BlockInfo, CheckIssue, TermCompliance } from "../../types/api";
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
 * The bucket a pending entry falls in — the third axis a reviewer filters by
 * (alongside item and locale).
 *
 * `passing` means the server will take this block in a bulk approve-passing
 * pass, and `failing` means it will not. The queue used to bucket on check
 * findings alone, because they were the only per-block evidence the payload
 * carried; the server applies two further bars (terminology and the voice
 * score), so a "no failing checks" bucket over-counted by exactly the blocks
 * approve-passing then refused. The entries now carry all three, and this
 * mirrors shipstate.blockCompliantAndPassing over them.
 */
export type ReviewQueueVerdict = "failing" | "passing";

/**
 * Which bar a failing entry misses. Ordered as the server applies them
 * (blockApproveBlocker), so a reviewer reads the same reasons the bulk pass
 * reports back.
 */
export type ReviewBlocker = "checks" | "terms" | "voice";

/** How the queue list groups its rows. */
export type ReviewGroupBy = "item" | "locale" | "verdict";

/** Active filter over the queue. An unset field imposes no constraint. */
export interface ReviewQueueFilter {
  itemId?: string;
  locale?: string;
  verdict?: ReviewQueueVerdict;
  /**
   * The collection whose items the queue is narrowed to. `""` selects the
   * items belonging to no collection — the same bucket the dashboard's rollups
   * mark as ungrouped — which is why the field is matched on presence rather
   * than truthiness: an empty string is a scope, not an absent filter.
   *
   * The server applies this one (`GET …/pending-review?collection=`), so a
   * collection larger than the session's slice is paged as its own queue rather
   * than sieved out of the project's. Matching it again here is a no-op that
   * keeps the pure model complete for callers holding a mixed set.
   */
  collectionId?: string;
}

/**
 * One pending block awaiting review in one locale, carrying everything the
 * reviewer needs to render and act faithfully: the full (runs-normalised)
 * block for source/target rendering + edit, and the block's check findings for
 * that locale.
 */
export interface ReviewEntry {
  /** Stable identity: `${itemId}::${blockId}::${locale}`. */
  id: string;
  itemId: string;
  itemName: string;
  format?: string;
  /**
   * The collection of this entry's item, `""` for an item in no collection.
   * The server names it on the queue payload, from the same join its collection
   * filter tests — so the entry a scope selected and the entry a group files
   * always agree.
   */
  collectionId: string;
  locale: string;
  /** Full block (source_runs/targets_runs already normalised to coded+spans). */
  block: BlockInfo;
  /** Check findings for this block+locale; empty until QA has been loaded. */
  issues: CheckIssue[];
  /**
   * The server's terminology verdict for this target, `""` when no terminology
   * governance was active for the locale.
   */
  termCompliance: TermCompliance;
  /**
   * The latest persisted voice score for this block+locale, and the bar
   * of the profile that produced it. Absent together for a block that has never
   * been scored — the server applies no voice bar to it either.
   */
  voiceScore?: number;
  voiceBar?: number;
}

/** One grouped section of the queue list. */
export interface ReviewGroup {
  /** Group discriminator (itemId, locale, or verdict). */
  key: string;
  /** Human label for the section header. */
  label: string;
  entries: ReviewEntry[];
}

/** Live counts over a set of entries, for the header and group chips. */
export interface ReviewQueueCounts {
  total: number;
  failing: number;
  passing: number;
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

/**
 * Whether an entry's voice score is below the bar of the profile that produced
 * it. False for an unscored block: the server applies no voice bar to one
 * either, so an unscored block is not "below" anything.
 */
export function isBelowVoiceBar(entry: ReviewEntry): boolean {
  return entry.voiceScore !== undefined && entry.voiceScore < (entry.voiceBar ?? 0);
}

/**
 * The bars a pending entry misses, in the order the server applies them. Empty
 * for an entry approve-passing will take.
 */
export function entryBlockers(entry: ReviewEntry): ReviewBlocker[] {
  const blockers: ReviewBlocker[] = [];
  if (entryHasErrors(entry)) blockers.push("checks");
  if (entry.termCompliance === "violation") blockers.push("terms");
  if (isBelowVoiceBar(entry)) blockers.push("voice");
  return blockers;
}

/**
 * The bucket for an entry: `passing` when it clears every bar the server
 * applies on approve, `failing` otherwise. The mirror of
 * shipstate.blockCompliantAndPassing over the evidence the queue payload carries.
 */
export function entryVerdict(entry: ReviewEntry): ReviewQueueVerdict {
  return entryBlockers(entry).length === 0 ? "passing" : "failing";
}

/** Whether the server's bulk approve-passing pass will take this entry. */
export function isEntryPassing(entry: ReviewEntry): boolean {
  return entryVerdict(entry) === "passing";
}

/** Whether an entry satisfies every set field of a filter. */
export function matchesFilter(entry: ReviewEntry, filter: ReviewQueueFilter): boolean {
  if (filter.itemId && entry.itemId !== filter.itemId) return false;
  if (filter.locale && entry.locale !== filter.locale) return false;
  if (filter.verdict && entryVerdict(entry) !== filter.verdict) return false;
  if (filter.collectionId !== undefined && entry.collectionId !== filter.collectionId) {
    return false;
  }
  return true;
}

/** Entries matching the filter, order preserved. */
export function filterEntries(
  entries: readonly ReviewEntry[],
  filter: ReviewQueueFilter,
): ReviewEntry[] {
  return entries.filter((e) => matchesFilter(e, filter));
}

/** How each verdict is named on a group header, a chip, and a filter pill. */
export const VERDICT_LABELS: Record<ReviewQueueVerdict, string> = {
  failing: "Misses a bar",
  passing: "Clears every bar",
};

/** How each blocker is named where a failing entry's reasons are listed. */
export const BLOCKER_LABELS: Record<ReviewBlocker, string> = {
  checks: "Failing checks",
  terms: "Terminology",
  voice: "Below the voice bar",
};

/** Order groups deterministically: failing first, then passing. */
const VERDICT_ORDER: ReviewQueueVerdict[] = ["failing", "passing"];

/**
 * Group entries for the list panel. Insertion order is preserved within a
 * group; group order is first-appearance for item/locale, and severity order
 * (failing → passing) for the verdict.
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
      const verdict = entryVerdict(entry);
      key = verdict;
      label = VERDICT_LABELS[verdict];
    }
    const existing = map.get(key);
    if (existing) existing.entries.push(entry);
    else map.set(key, { key, label, entries: [entry] });
  }
  const groups = [...map.values()];
  if (groupBy === "verdict") {
    groups.sort(
      (a, b) =>
        VERDICT_ORDER.indexOf(a.key as ReviewQueueVerdict) -
        VERDICT_ORDER.indexOf(b.key as ReviewQueueVerdict),
    );
  }
  return groups;
}

/** Live counts over a set of entries. */
export function queueCounts(entries: readonly ReviewEntry[]): ReviewQueueCounts {
  const counts: ReviewQueueCounts = {
    total: entries.length,
    failing: 0,
    passing: 0,
    byLocale: {},
    byItem: {},
  };
  for (const entry of entries) {
    if (entryVerdict(entry) === "failing") counts.failing++;
    else counts.passing++;
    counts.byLocale[entry.locale] = (counts.byLocale[entry.locale] ?? 0) + 1;
    counts.byItem[entry.itemId] = (counts.byItem[entry.itemId] ?? 0) + 1;
  }
  return counts;
}

/**
 * Entries that clear every bar the server applies — what "Approve all passing"
 * will approve, over the same three-way evidence the server judges on. It was
 * an upper bound while the payload carried only check findings; the response
 * still reports the split that actually happened, because the queue is a
 * snapshot and a re-check between preview and pass can move a block.
 */
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
