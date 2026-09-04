// A compact view switch: a pill group of small toggle buttons for choosing
// which reading of one thing to show (a keyed table or the file, the step list
// or the diagram). It sits inside a panel, beside its content, where a full
// Tabs bar would be too heavy.

import * as React from "react";
import { cn } from "../../lib/utils";

export interface ViewTabGroupProps extends React.HTMLAttributes<HTMLDivElement> {
  children: React.ReactNode;
}

/** The pill that holds a row of ViewTabs. Give it an `aria-label` naming the choice. */
export function ViewTabGroup({ className, children, ...rest }: ViewTabGroupProps) {
  return (
    <div
      role="group"
      className={cn("flex items-center gap-1 self-start rounded-md bg-muted p-0.5", className)}
      {...rest}
    >
      {children}
    </div>
  );
}

export interface ViewTabProps {
  active: boolean;
  onClick: () => void;
  children: React.ReactNode;
  className?: string;
  "data-testid"?: string;
}

/** One choice in a ViewTabGroup; `active` marks the reading currently shown. */
export function ViewTab({ active, onClick, children, className, ...rest }: ViewTabProps) {
  return (
    <button
      type="button"
      onClick={onClick}
      aria-pressed={active}
      className={cn(
        "rounded px-2 py-0.5 text-[11px] font-medium transition-colors",
        active
          ? "bg-background text-foreground shadow-sm"
          : "text-muted-foreground hover:text-foreground",
        className,
      )}
      {...rest}
    >
      {children}
    </button>
  );
}
