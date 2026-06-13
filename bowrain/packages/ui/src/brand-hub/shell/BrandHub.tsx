import type { ReactNode } from "react";
import { cn } from "@neokapi/ui-primitives";

export interface BrandHubProps {
  /** The section title, rendered as the page heading (sentence case). */
  title: string;
  /** Optional one-line description under the title. */
  description?: string;
  /** Optional header-right slot — typically a primary action button. */
  actions?: ReactNode;
  /** Optional element rendered below the header, above the content (e.g. filters). */
  toolbar?: ReactNode;
  /** Constrain the content column width. Defaults to a comfortable reading width. */
  width?: "default" | "wide" | "full";
  children: ReactNode;
  className?: string;
}

const widthClass: Record<NonNullable<BrandHubProps["width"]>, string> = {
  default: "max-w-5xl",
  wide: "max-w-7xl",
  full: "max-w-none",
};

/**
 * The shared page-shell for every section of the Brand hub (AD-021). Cross-section
 * navigation (Concepts · Voice · Experiments · Activity · Dashboard) lives in the
 * app shell's secondary panel; this component frames a single section's content
 * with a consistent heading, description, action slot, and content column.
 */
export function BrandHub({
  title,
  description,
  actions,
  toolbar,
  width = "default",
  children,
  className,
}: BrandHubProps) {
  return (
    <div className={cn("mx-auto w-full px-1 py-1", widthClass[width], className)}>
      <header className="flex flex-wrap items-start justify-between gap-3">
        <div className="min-w-0 space-y-1">
          <h1 className="text-xl font-semibold tracking-tight text-foreground">{title}</h1>
          {description && <p className="max-w-2xl text-sm text-muted-foreground">{description}</p>}
        </div>
        {actions && <div className="flex shrink-0 items-center gap-2">{actions}</div>}
      </header>
      {toolbar && <div className="mt-4">{toolbar}</div>}
      <div className="mt-6">{children}</div>
    </div>
  );
}
