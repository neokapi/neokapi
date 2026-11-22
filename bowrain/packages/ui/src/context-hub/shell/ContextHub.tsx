import type { ReactNode } from "react";
import { PageContainer, cn, type PageWidth } from "@neokapi/ui-primitives";

export interface ContextHubProps {
  /** The section title, rendered as the page heading (sentence case). */
  title: string;
  /** Optional one-line description under the title. */
  description?: string;
  /** Optional header-right slot — typically a primary action button. */
  actions?: ReactNode;
  /** Optional element rendered below the header, above the content (e.g. filters). */
  toolbar?: ReactNode;
  /**
   * The content column width. `default` and `wide` both take the shared `wide`
   * page width, since every hub section is a list, a table or an overview;
   * `full` runs edge to edge.
   */
  width?: "default" | "wide" | "full";
  children: ReactNode;
  className?: string;
}

const pageWidth: Record<NonNullable<ContextHubProps["width"]>, PageWidth> = {
  default: "wide",
  wide: "wide",
  full: "full",
};

/**
 * The shared page-shell for every section of the Context hub (AD-021). Cross-section
 * navigation (Profiles · Explorer · Concepts · Voice · Content memory · Changes ·
 * Activity) lives in the app shell's secondary panel; this component frames one
 * section's content with a consistent heading, description, action slot and
 * content column.
 */
export function ContextHub({
  title,
  description,
  actions,
  toolbar,
  width = "default",
  children,
  className,
}: ContextHubProps) {
  return (
    <PageContainer
      width={pageWidth[width]}
      className={width === "full" ? cn("px-6 py-5", className) : className}
    >
      <header className="flex flex-wrap items-start justify-between gap-3">
        <div className="min-w-0 space-y-1">
          {/* The brand's prism ramp as a quiet identity accent on the hub. */}
          <div
            aria-hidden
            className="mb-2 h-0.5 w-10 rounded-full"
            style={{
              background:
                "linear-gradient(90deg, var(--prism-1), var(--prism-2), var(--prism-3), var(--prism-4), var(--prism-5))",
            }}
          />
          <h1 className="text-xl font-semibold tracking-tight text-foreground">{title}</h1>
          {description && <p className="max-w-2xl text-sm text-muted-foreground">{description}</p>}
        </div>
        {actions && <div className="flex shrink-0 items-center gap-2">{actions}</div>}
      </header>
      {toolbar && <div className="mt-4">{toolbar}</div>}
      <div className="mt-6">{children}</div>
    </PageContainer>
  );
}
