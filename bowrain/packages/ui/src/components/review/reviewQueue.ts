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
 * pass, and `failing` means a bar it applies turned the block down.
 * `not_checked` is neither: no bar turned it down, but a bar that governs the
 * block's language has no result for it, so the server holds no evidence to
 * approve it on and leaves it for a person. A bar that governs nothing in the
 * language is no bar at all. This mirrors shipstate.blockApproveBlocker over the
 * evidence the queue payload carries.
 */
export type ReviewQueueVerdict = "failing" | "not_checked" | "passing";

/**
 * Which bar a failing entry misses. Ordered as the server applies them
 * (blockApproveBlocker), so a reviewer reads the same reasons the bulk pass
 * reports back.
 */
export type ReviewBlocker = "checks" | "terms" | "voice";

/**
 * Which governing bar an entry has no result for: terminology with no verdict,
 * or a voice bar nothing has scored the entry against. Every pending target is
 * run through the rule-based checks, so they are never among them.
 */
export type ReviewUnchecked = "terms" | "voice";

/**
 * Which bar governs nothing in an entry's language: no terms or voice profile
 * rules apply to it, or no voice profile does. Such a bar neither passes nor
 * blocks the entry.
 */
export type ReviewUngoverned = "terms" | "voice";

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
  /** Check findings for this block+locale; empty until the checks have been loaded. */
  issues: CheckIssue[];
  /**
   * The server's terminology verdict for this target: `"not_governed"` where no
   * terms or voice profile rules apply to the locale, and `""` where they do and
   * the target had nothing to check.
   */
  termCompliance: TermCompliance;
  /**
   * The voice bar this block is held to, present wherever a voice profile
   * governs the locale, and its latest persisted score, present once something
   * has scored it. A bar with no score is a governed block with no result;
   * neither means no voice profile governs the locale.
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
  notChecked: number;
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
 * Whether an entry's voice score is below the bar it is held to. False for an
 * unscored block, which is below nothing: where a voice profile governs, it has
 * no result instead (see entryUnchecked).
 */
export function isBelowVoiceBar(entry: ReviewEntry): boolean {
  return (
    entry.voiceScore !== undefined &&
    entry.voiceBar !== undefined &&
    entry.voiceScore < entry.voiceBar
  );
}

/**
 * The bars a pending entry misses, in the order the server applies them. Empty
 * for an entry no bar turned down, which is not the same as one approve-passing
 * will take: see entryUnchecked.
 */
export function entryBlockers(entry: ReviewEntry): ReviewBlocker[] {
  const blockers: ReviewBlocker[] = [];
  if (entryHasErrors(entry)) blockers.push("checks");
  if (entry.termCompliance === "violation") blockers.push("terms");
  if (isBelowVoiceBar(entry)) blockers.push("voice");
  return blockers;
}

/**
 * The governing bars an entry has no result for, in the order the server applies
 * them. A missing result is neither a pass nor a failure, and the server
 * approves no entry that has one.
 */
export function entryUnchecked(entry: ReviewEntry): ReviewUnchecked[] {
  const unchecked: ReviewUnchecked[] = [];
  if (entry.termCompliance === "") unchecked.push("terms");
  if (entry.voiceBar !== undefined && entry.voiceScore === undefined) unchecked.push("voice");
  return unchecked;
}

/**
 * The bars that govern nothing in an entry's language. They are named so a
 * reviewer can see what was never a bar here, and they decide nothing.
 */
export function entryNotGoverned(entry: ReviewEntry): ReviewUngoverned[] {
  const ungoverned: ReviewUngoverned[] = [];
  if (entry.termCompliance === "not_governed") ungoverned.push("terms");
  if (entry.voiceBar === undefined) ungoverned.push("voice");
  return ungoverned;
}

/**
 * The bucket for an entry: `failing` when a bar the server applies turns it
 * down, `not_checked` when none does but a bar was not checked, and `passing`
 * when it clears every bar. The mirror of shipstate.blockApproveBlocker over
 * the evidence the queue payload carries.
 */
export function entryVerdict(entry: ReviewEntry): ReviewQueueVerdict {
  if (entryBlockers(entry).length > 0) return "failing";
  if (entryUnchecked(entry).length > 0) return "not_checked";
  return "passing";
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
  not_checked: "Not checked",
  passing: "Clears every bar",
};

/** How each blocker is named where a failing entry's reasons are listed. */
export const BLOCKER_LABELS: Record<ReviewBlocker, string> = {
  checks: "Failing checks",
  terms: "Terminology",
  voice: "Below the voice bar",
};

/** How each unchecked bar is named where an entry's reasons are listed. */
export const UNCHECKED_LABELS: Record<ReviewUnchecked, string> = {
  terms: "Terminology not checked",
  voice: "Voice not checked",
};

/** How each bar that governs nothing is named where an entry's reasons are listed. */
export const NOT_GOVERNED_LABELS: Record<ReviewUngoverned, string> = {
  terms: "No terms apply",
  voice: "No voice profile applies",
};

/** The sentence each bar that governs nothing adds to the verdict's tooltip. */
const NOT_GOVERNED_DETAIL: Record<ReviewUngoverned, string> = {
  terms: "No terms apply to this language, so terminology is no bar here.",
  voice: "No voice profile applies to this language, so there is no voice bar.",
};

/**
 * The one-word name each blocker takes inside the verdict, where the sentence
 * already says a bar was missed and only the bar is still in question.
 */
export const BLOCKER_SHORT_LABELS: Record<ReviewBlocker | ReviewUnchecked, string> = {
  checks: "checks",
  terms: "terminology",
  voice: "voice",
};

/**
 * The verdict as one phrase: what happened, and which bars it happened to.
 *
 * A reviewer's next act depends on which bar was missed or not checked, so the
 * bars are read inside the verdict rather than beside it as separate chips.
 * `verdictDetail` spells each one out for the tooltip.
 */
export function verdictLabel(entry: ReviewEntry): string {
  const blockers = entryBlockers(entry);
  if (blockers.length > 0) {
    return `${VERDICT_LABELS.failing}: ${blockers.map((b) => BLOCKER_SHORT_LABELS[b]).join(", ")}`;
  }
  const unchecked = entryUnchecked(entry);
  if (unchecked.length > 0) {
    return `${VERDICT_LABELS.not_checked}: ${unchecked.map((u) => BLOCKER_SHORT_LABELS[u]).join(", ")}`;
  }
  return VERDICT_LABELS.passing;
}

/**
 * The bars an entry misses or has no result for, named in full, for the
 * verdict's tooltip, followed by the bars that govern nothing in its language.
 */
export function verdictDetail(entry: ReviewEntry): string {
  const missed = entryBlockers(entry).map((b) => BLOCKER_LABELS[b]);
  const unchecked = entryUnchecked(entry).map((u) => UNCHECKED_LABELS[u]);
  const ungoverned = entryNotGoverned(entry).map((g) => NOT_GOVERNED_DETAIL[g]);
  let detail: string;
  if (missed.length === 0 && unchecked.length === 0) {
    detail = "Every bar the server applies on approve is clear.";
  } else if (missed.length === 0) {
    detail = `${unchecked.join(" · ")}: a bar that applies to this language has no result, so approving all passing leaves this block for a person.`;
  } else {
    detail = [...missed, ...unchecked].join(" · ");
  }
  return ungoverned.length > 0 ? `${detail} ${ungoverned.join(" ")}` : detail;
}

/** Order groups deterministically: failing first, then not checked, then passing. */
const VERDICT_ORDER: ReviewQueueVerdict[] = ["failing", "not_checked", "passing"];

/**
 * Group entries for the list panel. Insertion order is preserved within a
 * group; group order is first-appearance for item/locale, and severity order
 * (failing → not checked → passing) for the verdict.
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
    notChecked: 0,
    passing: 0,
    byLocale: {},
    byItem: {},
  };
  for (const entry of entries) {
    const verdict = entryVerdict(entry);
    if (verdict === "failing") counts.failing++;
    else if (verdict === "not_checked") counts.notChecked++;
    else counts.passing++;
    counts.byLocale[entry.locale] = (counts.byLocale[entry.locale] ?? 0) + 1;
    counts.byItem[entry.itemId] = (counts.byItem[entry.itemId] ?? 0) + 1;
  }
  return counts;
}

/**
 * Entries that clear every bar the server applies — what "Approve all passing"
 * will approve, over the same evidence the server judges on. An entry with no
 * result for a bar that governs it is not among them. The response still reports
 * the split that actually happened, because the queue is a snapshot and a
 * re-check between preview and pass can move a block.
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
