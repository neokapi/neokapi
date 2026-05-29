import type { BlockInfo } from "../../types/api";

/**
 * Single source of truth for per-block translation status, its derived CSS
 * classes, and the TM/term score colourings. Previously this logic was
 * duplicated across TranslationEditor, VisualEditorCard, and TermSidebar; the
 * Translate / Review / Pre-process surfaces all import it from here.
 */

export type BlockStatus = "not-started" | "draft" | "translated" | "reviewed";

/** Derive a block's translation status for a given target locale. */
export function getBlockStatus(block: BlockInfo, locale: string): BlockStatus {
  if (block.properties["translation-status"] === "reviewed") return "reviewed";
  if (block.properties["translation-status"] === "draft") return "draft";
  if (!block.targets[locale]) return "not-started";
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
