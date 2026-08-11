import { cn } from "@neokapi/ui-primitives";
import { Fragment } from "react";
import type { BreadcrumbItem } from "../context/BreadcrumbContext";
import { ChevronRight } from "./icons";

export interface BreadcrumbBarProps {
  items: BreadcrumbItem[];
  className?: string;
}

/**
 * The trail across the top of every page: where this page sits, and one click
 * to each place above it.
 *
 * It is the only way up, which is why it replaced the rail's unlabelled back
 * arrow. An arrow can express "up one" and nothing else — not how deep you are,
 * not what is above you, and not how to reach a level two steps up without
 * guessing how many times to press it.
 *
 * Long trails give ground from the left. The last step is the page you are on,
 * so it is the one that must survive a narrow window; the first is the
 * workspace, which the switcher also names, so it is the first to go.
 */
export function BreadcrumbBar({ items, className }: BreadcrumbBarProps) {
  if (items.length === 0) return null;

  return (
    <nav aria-label="Breadcrumb" className={cn("min-w-0", className)}>
      <ol className="flex min-w-0 items-center gap-1 text-sm">
        {items.map((item, i) => {
          const last = i === items.length - 1;
          return (
            <Fragment key={`${item.label}-${i}`}>
              {i > 0 && (
                <ChevronRight aria-hidden className="size-3.5 shrink-0 text-muted-foreground/60" />
              )}
              <li
                className={cn(
                  "min-w-0 truncate",
                  // Everything but the final two steps folds away as the window
                  // narrows, rather than the trail pushing the top bar's own
                  // controls off the end.
                  i < items.length - 2 && "hidden lg:block",
                  last ? "font-medium text-foreground" : "shrink text-muted-foreground",
                )}
              >
                {last || !item.onClick ? (
                  // The hook rides the step, not the step's shape: the project
                  // is a link from a page inside it and plain text from the
                  // project's own overview, and a test asking "which project am
                  // I in" should not have to know which of those it is looking
                  // at.
                  <span
                    aria-current={last ? "page" : undefined}
                    data-testid={item.testId}
                    className="block truncate"
                  >
                    {item.label}
                  </span>
                ) : (
                  <button
                    type="button"
                    onClick={item.onClick}
                    data-testid={item.testId}
                    className="block max-w-full cursor-pointer truncate rounded-sm border-none bg-transparent p-0 text-left text-inherit transition-colors hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                  >
                    {item.label}
                  </button>
                )}
              </li>
            </Fragment>
          );
        })}
      </ol>
    </nav>
  );
}
