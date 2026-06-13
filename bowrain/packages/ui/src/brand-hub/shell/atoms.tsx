// Small shared presentational atoms for the Brand hub (AD-021): status badges,
// relation labels, an empty-state, and date helpers. Kept in the shell so every
// section renders the same vocabulary consistently.
import type { ReactNode } from "react";
import { Badge, cn } from "@neokapi/ui-primitives";
import type { TermStatus, RelationType, ChangeSetStatus } from "../../types/brand-graph";

// ── Term status ─────────────────────────────────────────────────────────────

const TERM_STATUS_CLASS: Record<TermStatus, string> = {
  preferred: "border-transparent bg-success/15 text-success [a&]:hover:bg-success/25",
  approved: "border-transparent bg-primary/15 text-primary [a&]:hover:bg-primary/25",
  admitted: "border-transparent bg-warning/15 text-warning [a&]:hover:bg-warning/25",
  proposed: "border-border bg-muted text-muted-foreground",
  deprecated: "border-border bg-muted text-muted-foreground line-through",
  forbidden: "border-transparent bg-destructive/15 text-destructive [a&]:hover:bg-destructive/25",
};

export function TermStatusBadge({ status, className }: { status: TermStatus; className?: string }) {
  return (
    <Badge className={cn("font-medium capitalize", TERM_STATUS_CLASS[status], className)}>
      {status}
    </Badge>
  );
}

// ── Change-set status ───────────────────────────────────────────────────────

const CHANGESET_STATUS_CLASS: Record<ChangeSetStatus, string> = {
  draft: "border-border bg-muted text-muted-foreground",
  in_review: "border-transparent bg-warning/15 text-warning",
  approved: "border-transparent bg-primary/15 text-primary",
  merged: "border-transparent bg-success/15 text-success",
  abandoned: "border-border bg-muted text-muted-foreground/70 line-through",
};

const CHANGESET_STATUS_LABEL: Record<ChangeSetStatus, string> = {
  draft: "Draft",
  in_review: "In review",
  approved: "Approved",
  merged: "Merged",
  abandoned: "Abandoned",
};

export function ChangeSetStatusBadge({
  status,
  className,
}: {
  status: ChangeSetStatus;
  className?: string;
}) {
  return (
    <Badge className={cn("font-medium", CHANGESET_STATUS_CLASS[status], className)}>
      {CHANGESET_STATUS_LABEL[status]}
    </Badge>
  );
}

export const changeSetStatusLabel = (status: ChangeSetStatus): string =>
  CHANGESET_STATUS_LABEL[status];

// ── Relations ───────────────────────────────────────────────────────────────

const RELATION_LABEL: Record<RelationType, string> = {
  BROADER: "broader than",
  NARROWER: "narrower than",
  PART_OF: "part of",
  HAS_PART: "has part",
  RELATED: "related to",
  REPLACED_BY: "replaced by",
  USE_INSTEAD: "use instead",
  EXACT_MATCH: "exact match",
  CLOSE_MATCH: "close match",
  COMPETITOR: "competitor",
};

export const relationLabel = (type: RelationType): string => RELATION_LABEL[type];

export function RelationBadge({ type, className }: { type: RelationType; className?: string }) {
  const governed = type === "REPLACED_BY";
  return (
    <Badge
      variant="outline"
      className={cn("font-medium", governed && "border-primary/40 text-primary", className)}
    >
      {RELATION_LABEL[type]}
    </Badge>
  );
}

// ── Empty state ─────────────────────────────────────────────────────────────

export function EmptyState({
  icon,
  title,
  description,
  action,
  className,
}: {
  icon?: ReactNode;
  title: string;
  description?: string;
  action?: ReactNode;
  className?: string;
}) {
  return (
    <div
      className={cn(
        "flex flex-col items-center justify-center gap-3 rounded-lg border border-dashed bg-muted/20 px-6 py-14 text-center",
        className,
      )}
    >
      {icon && <div className="text-muted-foreground [&_svg]:size-7">{icon}</div>}
      <div className="space-y-1">
        <p className="text-sm font-medium text-foreground">{title}</p>
        {description && (
          <p className="mx-auto max-w-sm text-sm text-muted-foreground">{description}</p>
        )}
      </div>
      {action}
    </div>
  );
}

// ── Date helpers ────────────────────────────────────────────────────────────

/** Compact absolute date, e.g. "13 Jun 2026". Empty/invalid input renders "—". */
export function formatDate(iso?: string): string {
  if (!iso) return "—";
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return "—";
  return d.toLocaleDateString(undefined, { day: "numeric", month: "short", year: "numeric" });
}

/** Relative time ("3h ago", "just now"); falls back to absolute date past a week. */
export function formatRelative(iso?: string): string {
  if (!iso) return "—";
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return "—";
  const secs = Math.round((Date.now() - d.getTime()) / 1000);
  if (secs < 45) return "just now";
  const mins = Math.round(secs / 60);
  if (mins < 60) return `${mins}m ago`;
  const hours = Math.round(mins / 60);
  if (hours < 24) return `${hours}h ago`;
  const days = Math.round(hours / 24);
  if (days < 7) return `${days}d ago`;
  return formatDate(iso);
}
