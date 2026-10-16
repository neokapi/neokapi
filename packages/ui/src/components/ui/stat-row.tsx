import * as React from "react";
import { t } from "@neokapi/i18n-react/runtime";

import { cn } from "../../lib/utils";
import { uiLocaleTag } from "../../lib/locale-name";
import { SimpleTooltip } from "./tooltip";

/**
 * StatRow: a row of labelled counts, the second region of an overview page.
 *
 * A number on a page is a count with a label, never a clause in a generated
 * sentence: projects 4, rules 61, suggestions 11, needs you 1. The value sits
 * at the page-title step in tabular figures, so counts in a row line up; the
 * label sits under it at the caption step in the muted ink.
 *
 * `warning` is for the one count that waits for a person. `muted` is for a
 * count that sets context. A zero is drawn as `0`: a count that drops out of
 * the row when it reaches zero reads as a row that changed shape, so a caller
 * that means to hide one omits the item.
 *
 * Each count is read as one phrase ("4 projects"), in the order the active
 * language puts a number and its noun.
 */

/** How a count is drawn. */
export type StatTone = "default" | "warning" | "muted";

/** One labelled count. */
export interface StatItem {
  /** The count. Formatted for the UI language. */
  value: number;
  /**
   * What it counts, in the UI language and in the form that agrees with the
   * value: the caller passes it through `t()` or a plural.
   */
  label: string;
  /** `warning` for the one count that needs a person. */
  tone?: StatTone;
  /** Makes the count a button that leads to what it counts. */
  onClick?: () => void;
  /** A tooltip that says more than the label. */
  hint?: string;
  /** Stable key, when two items could share a label. */
  id?: string;
}

export interface StatRowProps extends Omit<React.ComponentProps<"ul">, "children"> {
  items: readonly StatItem[];
}

// `--warning` is a fill colour that holds dark ink, and as ink on a light
// ground it reads at about 2.3:1. Mixed with its own ink it clears 4.9:1, and
// on a dark ground the fill colour is legible as it is.
const TONE_CLASS: Record<StatTone, string> = {
  default: "text-foreground",
  warning:
    "text-[color-mix(in_oklch,var(--warning)_60%,var(--warning-foreground))] dark:text-warning",
  muted: "text-muted-foreground",
};

function formatCount(value: number): string {
  try {
    return new Intl.NumberFormat(uiLocaleTag()).format(value);
  } catch {
    return String(value);
  }
}

/** A row of labelled counts that wraps on a narrow pane. */
export function StatRow({ items, className, ...props }: StatRowProps) {
  return (
    // An unstyled list loses its role in Safari, so it is spelt out.
    <ul
      role="list"
      data-slot="stat-row"
      className={cn("flex flex-wrap items-start gap-x-7 gap-y-3", className)}
      {...props}
    >
      {items.map((item) => (
        <Stat key={item.id ?? item.label} item={item} />
      ))}
    </ul>
  );
}

function Stat({ item }: { item: StatItem }) {
  const { label, hint } = item;
  const tone = item.tone ?? "default";
  const value = formatCount(item.value);
  const phrase = t("{value} {label}", "a count and what it counts, e.g. 4 projects", {
    value,
    label,
  });

  // The phrase is what assistive technology reads; the drawn value and label
  // are hidden from it, so the count is read once and in the language's order.
  const body = (
    <>
      <span className="sr-only">{phrase}</span>
      <span aria-hidden className="flex flex-col items-start">
        <span className={cn("text-xl leading-tight font-semibold tabular-nums", TONE_CLASS[tone])}>
          {value}
        </span>
        <span className="text-[11px] text-muted-foreground">{label}</span>
      </span>
    </>
  );

  const focusable = item.onClick ? (
    <button
      type="button"
      onClick={item.onClick}
      className="-mx-1.5 -my-1 cursor-pointer rounded-md px-1.5 py-1 text-left transition-colors hover:bg-muted focus-visible:outline-2 focus-visible:outline-offset-1 focus-visible:outline-ring"
    >
      {body}
    </button>
  ) : hint ? (
    // A count with a hint takes focus, so the hint is reachable from the
    // keyboard as well as on hover.
    <span
      tabIndex={0}
      className="-mx-1.5 -my-1 block rounded-md px-1.5 py-1 focus-visible:outline-2 focus-visible:outline-offset-1 focus-visible:outline-ring"
    >
      {body}
    </span>
  ) : null;

  return (
    <li data-slot="stat" data-tone={tone}>
      {focusable && hint ? (
        <SimpleTooltip content={hint}>{focusable}</SimpleTooltip>
      ) : (
        (focusable ?? body)
      )}
    </li>
  );
}
