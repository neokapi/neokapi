import * as React from "react";
import { t, useNeokapi } from "@neokapi/i18n-react/runtime";

import { cn } from "../../lib/utils";
import { formatLocale, type LocaleNameVariant } from "../../lib/locale-name";

/**
 * LocaleLabel: the one way a language is shown.
 *
 * The convention: where there is room, the display name in the reader's own
 * language followed by the tag in muted monospace, so "French (France) fr-FR"
 * answers both "which language" and "which tag" at once. Where there is no room
 * (a table cell, a coverage grid, a chip rail), `compact` shows the tag alone
 * and puts the name in the tooltip, so the code is never a dead end.
 *
 * The tag is drawn exactly as it was given and left to read left to right. Case
 * carries meaning in BCP 47 (`zh-Hant`, `sr-Latn-RS`), so no `uppercase` class
 * belongs anywhere near it, and a tag inside a right-to-left sentence would
 * otherwise reorder its own subtags.
 *
 * `LocalePill` remains the hue-coded chip for a dense grid that colour-codes its
 * languages; this is the label for everything that reads as prose.
 */
export interface LocaleLabelProps extends Omit<React.ComponentProps<"span">, "children"> {
  /** BCP 47 tag, e.g. `fr-FR`. */
  locale: string;
  /** Dense context: draw the tag alone and carry the name in the tooltip. */
  compact?: boolean;
  /** `short` names the language without its region ("French" for `fr-FR`). */
  variant?: LocaleNameVariant;
  /** Name this locale instead of the one CLDR resolves (a workspace's own). */
  displayName?: string;
  /** Drop the code suffix, leaving the name alone. */
  hideCode?: boolean;
  /** Mark this as the project's source language. */
  source?: boolean;
  /** Language to name the locale in. Defaults to the active UI language. */
  uiLocale?: string;
  /** Override the source marker's wording. */
  sourceLabel?: string;
}

/** A language shown by name, with its tag beside it. */
export function LocaleLabel({
  locale,
  compact = false,
  variant = "full",
  displayName,
  hideCode = false,
  source = false,
  uiLocale,
  sourceLabel,
  className,
  ...props
}: LocaleLabelProps) {
  // Subscribing to the dictionary is what makes a locale switch rename every
  // language on the page, not only the strings around them.
  const { locale: activeLocale } = useNeokapi();
  const resolved = formatLocale(locale, {
    uiLocale: uiLocale ?? activeLocale,
    compact,
    variant,
  });
  const name = displayName ?? resolved.name;
  const named = name !== resolved.code;
  const title = named ? `${name} (${resolved.code})` : resolved.code;
  const marker = sourceLabel ?? t("source", "the project's source language");

  const code = (
    <span dir="ltr" className="font-mono text-[0.85em] text-muted-foreground">
      {resolved.code}
    </span>
  );

  if (compact || !named) {
    return (
      <span
        data-slot="locale-label"
        data-locale={locale}
        title={title}
        className={cn("inline-flex items-center gap-1.5", className)}
        {...props}
      >
        {code}
        {source && <span className="text-[0.85em] text-muted-foreground">· {marker}</span>}
      </span>
    );
  }

  return (
    <span
      data-slot="locale-label"
      data-locale={locale}
      title={title}
      className={cn("inline-flex min-w-0 items-center gap-1.5", className)}
      {...props}
    >
      <span className="truncate">{name}</span>
      {!hideCode && code}
      {source && <span className="text-[0.85em] text-muted-foreground">· {marker}</span>}
    </span>
  );
}
