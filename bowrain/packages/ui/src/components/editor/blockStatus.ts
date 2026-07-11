import type { BlockInfo, TargetEntry, TargetStatus } from "../../types/api";

/**
 * Single source of truth for per-block translation status, its derived CSS
 * classes, and the TM/term score colourings. Previously this logic was
 * duplicated across TranslationEditor, VisualEditorCard, and TermSidebar; the
 * Translate / Review / Pre-process surfaces all import it from here.
 *
 * A block's `targets` entry is either a bare string (legacy payloads) or a
 * `TargetInfo` object carrying the per-locale `Target.Status` ladder value.
 * All reads must go through `getTargetText` / `getTargetStatus` /
 * `getBlockStatus` so both shapes work.
 */

export type BlockStatus = "not-started" | "draft" | "translated" | "reviewed";

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

/**
 * The per-locale status an edited target carries after saving `newText` for
 * `locale`. A review decision judges ONE specific translation, so changing the
 * text invalidates a stale reviewed/signed-off status — the server demotes it
 * to translated on content change (editorUpdateBlockTarget[Runs]); the
 * optimistic write mirrors that so the chip matches what a reload fetches.
 * Re-saving identical content, and every rung at or below translated, keeps
 * the current status.
 */
export function statusAfterEdit(block: BlockInfo, locale: string, newText: string): TargetStatus {
  const prev = getTargetStatus(block, locale);
  if ((prev === "reviewed" || prev === "signed-off") && newText !== getTargetText(block, locale)) {
    return "translated";
  }
  return prev;
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

/** Human-readable label for a status. */
export const statusLabel: Record<BlockStatus, string> = {
  "not-started": "Not Started",
  draft: "Draft",
  translated: "Translated",
  reviewed: "Reviewed",
};

/** Status dot fill (used in compact rows). */
export const statusDotClass: Record<BlockStatus, string> = {
  "not-started": "bg-transparent",
  draft: "bg-warning",
  translated: "bg-info",
  reviewed: "bg-success",
};

/** Left-border accent for a row keyed by status. */
export const statusBorderClass: Record<BlockStatus, string> = {
  "not-started": "border-l-transparent",
  draft: "border-l-warning",
  translated: "border-l-info",
  reviewed: "border-l-success",
};

/** Solid pill badge for a status. */
export const statusBadgeClass: Record<BlockStatus, string> = {
  "not-started": "bg-muted-foreground text-white",
  draft: "bg-warning text-white",
  translated: "bg-info text-white",
  reviewed: "bg-success text-white",
};

/** Tinted badge config (subtler than statusBadgeClass) for cards. */
export const statusConfig: Record<BlockStatus, { label: string; className: string }> = {
  "not-started": { label: "Not Started", className: "bg-muted text-muted-foreground" },
  draft: { label: "Draft", className: "bg-warning/15 text-warning dark:text-warning" },
  translated: { label: "Translated", className: "bg-info/15 text-info dark:text-info" },
  reviewed: { label: "Reviewed", className: "bg-success/15 text-success dark:text-success" },
};

/** Colour class for a TM match score badge. */
export function tmScoreClass(score: number): string {
  if (score >= 1) return "text-success bg-success/15 dark:text-success";
  if (score >= 0.85) return "text-info bg-info/15 dark:text-info";
  if (score >= 0.7) return "text-warning bg-warning/15 dark:text-warning";
  return "text-muted-foreground bg-muted";
}

/** Colour class for a terminology status badge. */
export function termStatusClass(status: string): string {
  const colors: Record<string, string> = {
    preferred: "text-success bg-success/[0.08]",
    approved: "text-info bg-info/[0.08]",
    admitted: "text-warning bg-warning/[0.08]",
    deprecated: "text-destructive bg-destructive/[0.08]",
  };
  return colors[status] || "text-muted-foreground bg-muted";
}
