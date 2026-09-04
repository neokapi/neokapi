import * as React from "react";
import { cn } from "../lib/utils";

interface PageHeaderProps {
  title: string;
  /** A line under the title. Takes an element so a count can be pluralized. */
  subtitle?: React.ReactNode;
  /** A small uppercase kicker above the title, naming what the page sits in. */
  eyebrow?: React.ReactNode;
  actions?: React.ReactNode;
  backButton?: React.ReactNode;
  /**
   * "default" is the page title row, with its actions on the right. "hero" is
   * the large centred title an empty state opens with, above a lead paragraph.
   */
  variant?: "default" | "hero";
  className?: string;
}

/**
 * The one page title. It owns the `h1`; `SectionHeading` is the `h2` beneath
 * it. Using it keeps every page title the same size and weight across kapi
 * desktop and the platform instead of each page inventing its own.
 */
export function PageHeader({
  title,
  subtitle,
  eyebrow,
  actions,
  backButton,
  variant = "default",
  className,
}: PageHeaderProps) {
  if (variant === "hero") {
    return (
      <div className={cn("mx-auto mb-10 max-w-2xl text-center", className)}>
        {eyebrow && (
          <div className="mb-3 text-xs font-medium uppercase tracking-[0.2em] text-muted-foreground">
            {eyebrow}
          </div>
        )}
        <h1 className="mb-3 text-3xl font-semibold tracking-tight md:text-4xl">{title}</h1>
        {subtitle && <p className="text-base leading-relaxed text-muted-foreground">{subtitle}</p>}
        {actions && <div className="mt-5 flex items-center justify-center gap-2">{actions}</div>}
      </div>
    );
  }

  return (
    <div className={cn("mb-6 flex items-center justify-between", className)}>
      <div className="flex items-center gap-3">
        {backButton}
        <div>
          {eyebrow && (
            <div className="mb-0.5 text-xs font-medium uppercase tracking-[0.2em] text-muted-foreground">
              {eyebrow}
            </div>
          )}
          <h1 className="text-xl font-semibold">{title}</h1>
          {subtitle && <p className="mt-0.5 text-xs text-muted-foreground">{subtitle}</p>}
        </div>
      </div>
      {actions && <div className="flex items-center gap-2">{actions}</div>}
    </div>
  );
}
