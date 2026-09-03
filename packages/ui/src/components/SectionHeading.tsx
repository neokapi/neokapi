import * as React from "react";

interface SectionHeadingProps {
  /** A small leading icon, e.g. `<Globe size={14} />`. */
  icon?: React.ReactNode;
  /** An optional trailing count, shown in a lighter weight. */
  count?: number | string;
  className?: string;
  children: React.ReactNode;
}

/**
 * The one in-page section eyebrow: a small, uppercase, muted heading that
 * labels a group of settings or a block of a document. It pairs with
 * `PageHeader`, which owns the page title (`h1`); a `SectionHeading` is the
 * `h2` beneath it. Using it keeps every section label the same size and weight
 * instead of each page inventing its own.
 */
export function SectionHeading({ icon, count, className, children }: SectionHeadingProps) {
  return (
    <h2
      className={`flex items-center gap-2 text-xs font-semibold uppercase tracking-wide text-muted-foreground ${
        className ?? ""
      }`}
    >
      {icon}
      {children}
      {count !== undefined && <span className="font-normal normal-case">{count}</span>}
    </h2>
  );
}
