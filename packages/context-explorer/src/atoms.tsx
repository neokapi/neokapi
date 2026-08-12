// Presentational atoms shared by the three panes (Apache-2.0). The vocabulary
// is deliberately small: a pane frame that states its own reach, a note strip, a
// validity chip and an address chip. Everything else comes from
// @neokapi/ui-primitives so the explorer renders in the same tokens as the rest
// of the product.

import type { ReactNode } from "react";
import { Badge, Skeleton, cn } from "@neokapi/ui-primitives";
import { ErrorHint } from "@neokapi/concept-ui";
import { t } from "@neokapi/i18n-react/runtime";
import { Info, Link2, TriangleAlert } from "lucide-react";

/**
 * The frame every pane sits in.
 *
 * `reach` is not decoration: a pane that can only see one project inside a
 * workspace says so in its header, because the alternative — an answer that
 * looks complete and is not — is the failure this surface exists to prevent.
 */
export function Pane({
  title,
  icon,
  reach,
  description,
  actions,
  children,
  className,
  ...rest
}: {
  title: ReactNode;
  icon?: ReactNode;
  reach?: string;
  description?: ReactNode;
  actions?: ReactNode;
  children: ReactNode;
  className?: string;
} & Omit<React.HTMLAttributes<HTMLElement>, "title">) {
  return (
    <section
      className={cn("rounded-xl border bg-card text-card-foreground shadow-sm", className)}
      {...rest}
    >
      <header className="flex items-start gap-3 border-b px-4 py-3">
        {icon && <div className="mt-0.5 text-muted-foreground [&_svg]:size-4">{icon}</div>}
        <div className="min-w-0 flex-1">
          <h3 className="text-sm leading-tight font-semibold text-foreground">{title}</h3>
          {description && <p className="mt-0.5 text-xs text-muted-foreground">{description}</p>}
        </div>
        {reach && (
          <Badge variant="outline" className="shrink-0 font-normal text-muted-foreground">
            {reach}
          </Badge>
        )}
        {actions && <div className="flex shrink-0 items-center gap-1">{actions}</div>}
      </header>
      <div className="p-4">{children}</div>
    </section>
  );
}

/**
 * The notes an answer carries.
 *
 * Rendered at the top of the pane and never collapsed: the note that says the
 * graph moved under the reader is the one thing on the surface that invalidates
 * everything below it.
 */
export function NoteStrip({ notes, className }: { notes: string[]; className?: string }) {
  if (!notes.length) return null;
  return (
    <ul
      className={cn(
        "mb-3 space-y-1 rounded-lg border border-amber-500/40 bg-amber-500/10 px-3 py-2",
        className,
      )}
      data-testid="context-notes"
    >
      {notes.map((note) => (
        <li key={note} className="flex items-start gap-2 text-xs text-foreground">
          <TriangleAlert className="mt-0.5 size-3.5 shrink-0 text-amber-600 dark:text-amber-500" />
          <span>{note}</span>
        </li>
      ))}
    </ul>
  );
}

/** A validity window, and where it stands right now. */
export function ValidityChip({
  state,
  from,
  to,
  className,
}: {
  state?: string;
  from?: string;
  to?: string;
  className?: string;
}) {
  if (!state && !from && !to) return null;
  const tone =
    state === "expired"
      ? "border-destructive/40 text-destructive"
      : state === "upcoming"
        ? "border-amber-500/40 text-amber-700 dark:text-amber-500"
        : "border-emerald-500/40 text-emerald-700 dark:text-emerald-500";
  const window = [from, to].filter(Boolean).join(" → ");
  return (
    <Badge variant="outline" className={cn("font-normal", tone, className)}>
      {state ?? t("bounded")}
      {window && <span className="ml-1 font-mono text-[11px] opacity-70">{window}</span>}
    </Badge>
  );
}

/** A `context://` address, rendered as the copyable thing it is. */
export function AddressChip({ address, className }: { address: string; className?: string }) {
  return (
    <code
      className={cn(
        "inline-flex items-center gap-1.5 rounded-md bg-muted px-2 py-1 font-mono text-[11px] leading-none text-muted-foreground",
        className,
      )}
      data-testid="context-address"
    >
      <Link2 className="size-3" />
      {address}
    </code>
  );
}

/** The placeholder a pane shows while its answer is in flight. */
export function PaneSkeleton({ rows = 3 }: { rows?: number }) {
  return (
    <div className="space-y-2" data-testid="pane-loading">
      {Array.from({ length: rows }, (_, i) => (
        <Skeleton key={i} className="h-8 w-full" />
      ))}
    </div>
  );
}

/**
 * A read that did not complete.
 *
 * Distinct from an empty answer on purpose: "the source is unreachable" and
 * "nothing governs here" are opposite facts, and a reader who cannot tell them
 * apart will act on the wrong one.
 */
export function PaneError({ error }: { error: Error }) {
  return <ErrorHint title={t("This answer did not load")} description={error.message} />;
}

/** A one-line statement of what a pane could see, for the empty case. */
export function ReachNote({ children }: { children: ReactNode }) {
  return (
    <p className="flex items-start gap-2 text-xs text-muted-foreground">
      <Info className="mt-0.5 size-3.5 shrink-0" />
      <span>{children}</span>
    </p>
  );
}
