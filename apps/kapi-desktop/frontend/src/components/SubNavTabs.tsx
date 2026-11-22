import type { ReactNode } from "react";
import { cn } from "@neokapi/ui-primitives";

export interface SubNavTab<T extends string> {
  id: T;
  label: ReactNode;
}

export interface SubNavTabsProps<T extends string> {
  /** The accessible name of the tab list, e.g. "Context sections". */
  label: string;
  tabs: ReadonlyArray<SubNavTab<T>>;
  /** The marked tab, or null when none is. */
  active: T | null;
  onSelect: (id: T) => void;
  /** A control drawn at the right end of the row, outside the tab list. */
  trailing?: ReactNode;
  className?: string;
}

/**
 * The one sub-navigation of a page: underlined tabs in a row under the top
 * bar, inset to the page's content padding. Context, Project Settings and
 * App Settings all draw their sections with it.
 */
export function SubNavTabs<T extends string>({
  label,
  tabs,
  active,
  onSelect,
  trailing,
  className,
}: SubNavTabsProps<T>) {
  return (
    <div
      data-slot="sub-nav"
      className={cn(
        "sticky top-0 z-10 flex shrink-0 items-center border-b border-border bg-background/95 px-6 backdrop-blur",
        className,
      )}
    >
      <div role="tablist" aria-label={label} className="flex items-center gap-1">
        {tabs.map((tab) => (
          <button
            key={tab.id}
            type="button"
            role="tab"
            // The label arrives already translated by the caller, so the
            // button is not extracted as a block of its own.
            translate="no"
            aria-selected={active === tab.id}
            onClick={() => onSelect(tab.id)}
            className={cn(
              "-mb-px border-b-2 px-3 py-2.5 text-sm transition-colors",
              active === tab.id
                ? "border-primary font-medium text-foreground"
                : "border-transparent text-muted-foreground hover:text-foreground",
            )}
          >
            {tab.label}
          </button>
        ))}
      </div>
      {trailing && <div className="ml-auto flex items-center">{trailing}</div>}
    </div>
  );
}
