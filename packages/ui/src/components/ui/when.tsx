import * as React from "react";
import { useNeokapi } from "@neokapi/i18n-react/runtime";

import { cn } from "../../lib/utils";
import { formatWhen, type WhenFieldStyle } from "../../lib/when";

/**
 * When: the one way an instant is shown.
 *
 * A `<time>` element carrying the machine-readable instant in `dateTime`, the
 * date and time in the reader's own language as its text, and the exact instant
 * with its zone in the tooltip. A reader who wants the precise moment hovers;
 * everyone else reads a date.
 *
 * `relative` draws the distance from now ("3 minutes ago", "yesterday") for a
 * feed or a listing where recency is the question. Both forms come from the
 * same formatter, so a surface showing one beside the other reads as one voice.
 */
export interface WhenProps extends Omit<
  React.ComponentProps<"time">,
  "children" | "dateTime" | "title"
> {
  /** The instant, as an ISO 8601 string or anything `Date` reads. */
  iso: string | number | Date | undefined | null;
  /** How much of the date to spell out. `none` leaves the time alone. */
  dateStyle?: WhenFieldStyle | "none";
  /** How much of the time to spell out. `none` leaves the date alone. */
  timeStyle?: WhenFieldStyle | "none";
  /** Draw the distance from now instead of the instant itself. */
  relative?: boolean;
  /** Language to render the instant in. Defaults to the active UI language. */
  uiLocale?: string;
  /** Replace the tooltip, which otherwise carries the exact instant. */
  title?: string;
}

/** An instant shown in the reader's language, exact in its tooltip. */
export function When({
  iso,
  dateStyle,
  timeStyle,
  relative = false,
  uiLocale,
  title,
  className,
  ...props
}: WhenProps) {
  // Subscribing to the dictionary is what makes a locale switch re-render every
  // date on the page, not only the strings around them.
  const { locale: activeLocale } = useNeokapi();
  const when = formatWhen(iso, {
    uiLocale: uiLocale ?? activeLocale,
    dateStyle,
    timeStyle,
    relative,
  });
  if (!when.text) return null;
  return (
    <time
      data-slot="when"
      dateTime={when.valid ? when.iso : undefined}
      title={title ?? when.title}
      className={cn("tabular-nums", className)}
      {...props}
    >
      {when.text}
    </time>
  );
}
