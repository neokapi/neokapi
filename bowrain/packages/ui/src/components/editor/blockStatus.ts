import { statusMeta, type StatusTone } from "@neokapi/ui-primitives";
import type { BlockInfo, BlockStatusBucket, TargetEntry, TargetStatus } from "../../types/api";

/**
 * Single source of truth for per-block translation status, the row paint it
 * derives, and the content memory and term score colourings. TranslationEditor,
 * VisualEditorCard, TermSidebar and the Translate, Review and Pre-process
 * surfaces all import it from here.
 *
 * A status is *named* by `StatusBadge` from the shared primitives, not here: the
 * ladder, its wording and its colours are the vocabulary Kapi Desktop and the
 * platform share. What stays here is the reading of a block's payload and the
 * dot and margin rule a dense row paints from the resulting rung.
 *
 * A block's `targets` entry is either a bare string (legacy payloads) or a
 * `TargetInfo` object carrying the per-locale `Target.Status` ladder value.
 * All reads must go through `getTargetText` / `getTargetStatus` /
 * `getBlockStatus` so both shapes work.
 */

/** The bucket the server counts a block under, for one locale. */
export type BlockStatus = BlockStatusBucket;

/** Plain target text for a locale, whatever shape the entry has. */
export function getTargetText(block: BlockInfo, locale: string): string {
  const entry = block.targets?.[locale];
  if (entry == null) return "";
  if (typeof entry === "string") return entry;
  return entry.text ?? "";
}

/**
 * Per-locale `Target.Status` for the given locale ("" when the payload carries
 * none). This is the canonical review-ladder read; the legacy block-global
 * `properties["translation-status"]` is only consulted by `getBlockStatus` as
 * a read fallback for blocks written before per-locale status existed.
 */
export function getTargetStatus(block: BlockInfo, locale: string): TargetStatus {
  const entry = block.targets?.[locale];
  if (entry != null && typeof entry === "object" && entry.status) return entry.status;
  return "";
}

/**
 * Return a copy of the block with the target entry for `locale` replaced.
 * Passing `undefined` removes the entry (used to roll back an optimistic
 * review update when the server call fails).
 */
export function withTargetEntry(
  block: BlockInfo,
  locale: string,
  entry: TargetEntry | undefined,
): BlockInfo {
  const targets = { ...block.targets };
  if (entry === undefined) delete targets[locale];
  else targets[locale] = entry;
  return { ...block, targets };
}

/**
 * Return a copy of the block with the per-locale target status set in the
 * canonical object shape (text preserved) — the same shape a reload would
 * fetch from the server after a review call. Nothing writes the legacy
 * block-global property anymore.
 */
export function withTargetStatus(
  block: BlockInfo,
  locale: string,
  status: TargetStatus,
): BlockInfo {
  return withTargetEntry(block, locale, { text: getTargetText(block, locale), status });
}

/**
 * Snapshot of a target entry's review-relevant state. Capture it INSIDE the
 * `setBlocks` updater at the moment of the optimistic status write (not from a
 * component-scope block that may be stale), so a rollback restores the state
 * the entry actually had when the write happened.
 */
export interface TargetStatusSnapshot {
  existed: boolean;
  status: TargetStatus;
}

/** Capture the review-status snapshot for a locale (see TargetStatusSnapshot). */
export function captureTargetStatus(block: BlockInfo, locale: string): TargetStatusSnapshot {
  return { existed: block.targets?.[locale] != null, status: getTargetStatus(block, locale) };
}

/**
 * Roll back an optimistic review-status write: restore ONLY the status field,
 * preserving whatever target text is current — a target save may have landed
 * while the review call was in flight, and clobbering it would show text the
 * server no longer holds. If the entry didn't exist at capture time and no
 * text has appeared since, the entry is removed instead.
 */
export function rollbackTargetStatus(
  block: BlockInfo,
  locale: string,
  snapshot: TargetStatusSnapshot,
): BlockInfo {
  if (!snapshot.existed && !getTargetText(block, locale)) {
    return withTargetEntry(block, locale, undefined);
  }
  return withTargetStatus(block, locale, snapshot.status);
}

/** Coded target text for a locale ("" when the payload carries none). */
export function getTargetCoded(block: BlockInfo, locale: string): string {
  return block.targets_coded?.[locale] ?? "";
}

/**
 * The per-locale status an edited target carries after saving for `locale`. A
 * review decision judges ONE specific translation, so changing the content
 * invalidates a stale reviewed/signed-off status — the server demotes it to
 * translated whenever the runs differ (demoteStaleReviewOnEdit); the optimistic
 * write mirrors that so the chip matches what a reload fetches. Re-saving
 * identical content, and every rung at or below translated, keeps the status.
 *
 * `newCoded` is the coded text of the saved runs. Pass it whenever the save
 * carries runs: coded text encodes the inline codes, so an edit that only moves
 * or retypes a code — identical once flattened, a different run sequence to the
 * server — is seen as the change it is. Without it the comparison falls back to
 * plain text and can miss such an edit.
 */
export function statusAfterEdit(
  block: BlockInfo,
  locale: string,
  newText: string,
  newCoded?: string,
): TargetStatus {
  const prev = getTargetStatus(block, locale);
  if (prev !== "reviewed" && prev !== "signed-off") return prev;
  const prevCoded = getTargetCoded(block, locale);
  const changed =
    newCoded !== undefined && prevCoded !== ""
      ? newCoded !== prevCoded
      : newText !== getTargetText(block, locale);
  return changed ? "translated" : prev;
}

/** Derive a block's translation status for a given target locale. */
export function getBlockStatus(block: BlockInfo, locale: string): BlockStatus {
  // Canonical: per-locale Target.Status on the block's target for this locale.
  // Reviewed/signed-off are trusted as-is — the server refuses to approve an
  // empty translation, so a reviewed entry always carries text.
  const status = getTargetStatus(block, locale);
  if (status === "reviewed" || status === "signed-off") return "reviewed";
  // Every lower rung only means something when the locale actually has target
  // text: a status entry without text (a phantom optimistic write, or the
  // legacy block-global flag on a locale that was never translated) is still
  // not-started — mirroring convergence.TargetState, which reports no state
  // when no non-empty target exists, so the UI counts agree with coverage.
  if (!getTargetText(block, locale)) return "not-started";
  if (status === "translated") return "translated";
  if (status === "draft") return "draft";
  // Legacy block-global property — read fallback only (pre-per-locale blocks).
  if (block.properties["translation-status"] === "reviewed") return "reviewed";
  if (block.properties["translation-status"] === "draft") return "draft";
  if (
    block.properties["translation-origin"] === "machine" ||
    block.properties["translation-origin"] === "pseudo"
  ) {
    return "draft";
  }
  return "translated";
}

/**
 * The ladder value to badge for a locale: the wire `Target.Status` the payload
 * carries, and the derived bucket for one that carries none. Rendering the wire
 * value is what lets `signed-off` reach a badge at all, since the bucket
 * collapses it into `reviewed`; a locale with no target text stays
 * `not-started`, which is what the server counts it as.
 *
 * Hand the result to `StatusBadge ladder="content"`.
 */
export function targetLadderStatus(block: BlockInfo, locale: string): string {
  const bucket = getBlockStatus(block, locale);
  if (bucket === "not-started") return bucket;
  return getTargetStatus(block, locale) || bucket;
}

/** Where a bucket sits on the shared content ladder. */
export function blockStatusTone(status: BlockStatus): StatusTone {
  return statusMeta("content", status).tone;
}

// A row's dot and its margin rule are the badge's colour without the badge, so
// they read the shared ladder's tone rather than a second table of their own:
// muted at the bottom rung, neutral in the middle, green once a rung has been
// earned. `not-started` is the exception, and stays blank — a locale with no
// target text has nothing to paint, and an empty rule is how the pane has
// always said so.
const TONE_DOT: Record<StatusTone, string> = {
  start: "bg-muted-foreground/60",
  middle: "bg-foreground/40",
  earned: "bg-success/60",
  settled: "bg-success",
  attention: "bg-warning",
};

const TONE_BORDER: Record<StatusTone, string> = {
  start: "border-l-muted-foreground/60",
  middle: "border-l-foreground/40",
  earned: "border-l-success/60",
  settled: "border-l-success",
  attention: "border-l-warning",
};

const TONE_RULE: Record<StatusTone, string> = {
  start: "[--kapi-block-rule:var(--color-muted-foreground)]",
  middle: "[--kapi-block-rule:var(--color-foreground)]",
  earned: "[--kapi-block-rule:var(--color-success)]",
  settled: "[--kapi-block-rule:var(--color-success)]",
  attention: "[--kapi-block-rule:var(--color-warning)]",
};

/** Status dot fill for a compact row. */
export function statusDotClass(status: BlockStatus): string {
  return status === "not-started" ? "bg-transparent" : TONE_DOT[blockStatusTone(status)];
}

/** Left-border accent for a dense table row. */
export function statusBorderClass(status: BlockStatus): string {
  return status === "not-started" ? "border-l-transparent" : TONE_BORDER[blockStatusTone(status)];
}

/** The margin rule the reading pane paints beside a block. */
export function statusRuleClass(status: BlockStatus): string {
  return status === "not-started"
    ? "[--kapi-block-rule:var(--color-muted-foreground)] opacity-80"
    : TONE_RULE[blockStatusTone(status)];
}

/** Colour class for a content-memory match score badge. */
export function memoryScoreClass(score: number): string {
  if (score >= 1) return "text-success bg-success/15 dark:text-success";
  if (score >= 0.85) return "text-info bg-info/15 dark:text-info";
  if (score >= 0.7) return "text-warning bg-warning/15 dark:text-warning";
  return "text-muted-foreground bg-muted";
}

/**
 * Colour class for a terminology status badge — the full model.TermStatus
 * ladder. A forbidden term badges destructive, matching the brand hub's
 * TermStatusBadge.
 */
export function termStatusClass(status: string): string {
  const colors: Record<string, string> = {
    preferred: "text-success bg-success/[0.08]",
    approved: "text-info bg-info/[0.08]",
    admitted: "text-warning bg-warning/[0.08]",
    proposed: "text-muted-foreground bg-muted",
    deprecated: "text-muted-foreground bg-muted line-through",
    forbidden: "text-destructive bg-destructive/[0.08]",
  };
  return colors[status] || "text-muted-foreground bg-muted";
}
