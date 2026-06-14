// Small shared presentational atoms for the concept UI (Apache-2.0): a term
// status chip, a relation label badge, a locale pill, a titled section frame,
// an empty hint, and date helpers. Kept here so every section — list, shell,
// and the panels the section agents fill — renders one consistent vocabulary in
// @neokapi/ui design tokens.
import type { ReactNode } from "react";
import { Badge, cn } from "@neokapi/ui-primitives";
import { TriangleAlert } from "lucide-react";
import {
  RELATION_LABEL,
  TERM_STATUS_CLASS,
  TERM_STATUS_LABEL,
  isGovernedRelation,
} from "./concept-meta";
import type { RelationType, TermStatus } from "./types";

// ── Term status ──────────────────────────────────────────────────────────────

export function StatusChip({ status, className }: { status: TermStatus; className?: string }) {
  return (
    <Badge className={cn("font-medium", TERM_STATUS_CLASS[status], className)}>
      {TERM_STATUS_LABEL[status]}
    </Badge>
  );
}

// ── Relations ────────────────────────────────────────────────────────────────

export function RelationChip({ type, className }: { type: RelationType; className?: string }) {
  return (
    <Badge
      variant="outline"
      className={cn(
        "font-medium",
        isGovernedRelation(type) && "border-primary/40 text-primary",
        className,
      )}
    >
      {RELATION_LABEL[type]}
    </Badge>
  );
}

// ── Locale ───────────────────────────────────────────────────────────────────

export function LocalePill({ locale, className }: { locale: string; className?: string }) {
  return (
    <span
      className={cn(
        "inline-flex items-center rounded-sm bg-muted px-1.5 py-0.5 font-mono text-[11px] leading-none text-muted-foreground",
        className,
      )}
    >
      {locale}
    </span>
  );
}

// ── Section frame ────────────────────────────────────────────────────────────

/**
 * The titled card frame each concept-view section sits in. Panels render their
 * body as children; `icon`, `description`, and `actions` are optional. Kept
 * presentational so the shell and every panel agree on spacing and chrome.
 */
export function ConceptSection({
  title,
  icon,
  description,
  actions,
  children,
  className,
}: {
  title: ReactNode;
  icon?: ReactNode;
  description?: ReactNode;
  actions?: ReactNode;
  children: ReactNode;
  className?: string;
}) {
  return (
    <section className={cn("rounded-xl border bg-card text-card-foreground shadow-sm", className)}>
      <header className="flex items-start gap-3 border-b px-4 py-3">
        {icon && <div className="mt-0.5 text-muted-foreground [&_svg]:size-4">{icon}</div>}
        <div className="min-w-0 flex-1">
          <h3 className="text-sm font-semibold leading-tight text-foreground">{title}</h3>
          {description && <p className="mt-0.5 text-xs text-muted-foreground">{description}</p>}
        </div>
        {actions && <div className="flex shrink-0 items-center gap-1">{actions}</div>}
      </header>
      <div className="p-4">{children}</div>
    </section>
  );
}

// ── Empty hint ───────────────────────────────────────────────────────────────

export function EmptyHint({
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
        "flex flex-col items-center justify-center gap-2 rounded-lg border border-dashed bg-muted/20 px-6 py-10 text-center",
        className,
      )}
    >
      {icon && <div className="text-muted-foreground [&_svg]:size-6">{icon}</div>}
      <div className="space-y-0.5">
        <p className="text-sm font-medium text-foreground">{title}</p>
        {description && (
          <p className="mx-auto max-w-xs text-xs text-muted-foreground">{description}</p>
        )}
      </div>
      {action}
    </div>
  );
}

// ── Error hint ───────────────────────────────────────────────────────────────

/**
 * The destructive-toned sibling of EmptyHint, shown when a data-source read
 * FAILS (a rejected fetch). It is visually distinct from the empty state so a
 * fetch error never masquerades as "no data" — the reader can tell a genuine
 * absence from a load that did not complete.
 */
export function ErrorHint({
  title,
  description,
  className,
}: {
  title: string;
  description?: string;
  className?: string;
}) {
  return (
    <div
      role="alert"
      className={cn(
        "flex flex-col items-center justify-center gap-2 rounded-lg border border-dashed border-destructive/40 bg-destructive/5 px-6 py-10 text-center",
        className,
      )}
    >
      <div className="text-destructive [&_svg]:size-6">
        <TriangleAlert aria-hidden />
      </div>
      <div className="space-y-0.5">
        <p className="text-sm font-medium text-foreground">{title}</p>
        {description && (
          <p className="mx-auto max-w-xs text-xs text-muted-foreground">{description}</p>
        )}
      </div>
    </div>
  );
}

// ── Date helpers ─────────────────────────────────────────────────────────────

/** Compact absolute date, e.g. "14 Jun 2026". Empty/invalid input renders "—". */
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
