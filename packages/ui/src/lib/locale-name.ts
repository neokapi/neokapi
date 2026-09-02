/**
 * Locale display names.
 *
 * A locale code is an identifier, and a reader needs a label. `fr` names the
 * language to the machine and to a reviewer who already knows the tag; everyone
 * else reads a two-letter abbreviation and guesses. So a surface with room for a
 * name shows one, and a surface without room (a dense coverage grid, a table
 * cell) shows the code and keeps the name reachable, which is what `LocaleLabel`
 * does in its compact form and `LocalePill` does with its title.
 *
 * The name is resolved in the UI language, so a Norwegian reader reads
 * "fransk (Frankrike)" and an English reader reads "French (France)". The UI
 * language comes from the caller, else from `document.documentElement.lang`
 * (which `syncDocumentLocale` keeps current), else English.
 *
 * CLDR names every well-formed tag, so this needs no per-locale table and
 * handles a real project target (`fr-FR`, `pt-BR`, `zh-Hant`) as readily as a
 * major tag. It falls back to the code when the runtime has no `Intl.DisplayNames`
 * or has no name for the tag (`qps`, our pseudo locale, echoes its own code back).
 */

/** How much of a tag the name spells out. */
export type LocaleNameVariant = "full" | "short";

/** English is the last resort, and the source language of every catalog. */
const FALLBACK_UI_LOCALE = "en";

/**
 * The language the UI is being read in.
 *
 * `syncDocumentLocale` writes the active locale onto `<html lang>` on every
 * switch, so the document is the one place a non-React caller can read it.
 */
export function uiLocaleTag(): string {
  if (typeof document === "undefined") return FALLBACK_UI_LOCALE;
  const lang = document.documentElement?.lang;
  return lang && lang.trim() ? lang : FALLBACK_UI_LOCALE;
}

// One formatter per UI language. `null` records a runtime that rejected the
// language, so a broken environment is asked once rather than on every render.
const formatters = new Map<string, Intl.DisplayNames | null>();

function formatterFor(uiLocale: string): Intl.DisplayNames | null {
  const cached = formatters.get(uiLocale);
  if (cached !== undefined) return cached;
  let made: Intl.DisplayNames | null = null;
  try {
    made = new Intl.DisplayNames(uiLocale, { type: "language" });
  } catch {
    made = null;
  }
  formatters.set(uiLocale, made);
  return made;
}

/**
 * Resolve a locale code to a display name, falling back to the code.
 *
 * `variant: "short"` names the language alone ("French" for `fr-FR`), for a
 * surface where the region is noise. The default names the whole tag.
 */
export function resolveLocaleName(
  code: string,
  uiLocale?: string,
  variant: LocaleNameVariant = "full",
): string {
  if (!code) return code;
  const names = formatterFor(uiLocale?.trim() || uiLocaleTag());
  if (!names) return code;
  const subject = variant === "short" ? (code.split("-")[0] ?? code) : code;
  try {
    return names.of(subject) ?? code;
  } catch {
    return code;
  }
}

/**
 * Render a locale as "Name (code)" for a single-string context (a tooltip, an
 * aria-label, a title) where a name and a code cannot sit side by side. When the
 * code has no name it is returned alone rather than doubled.
 */
export function localeLabel(code: string, uiLocale?: string): string {
  const name = resolveLocaleName(code, uiLocale);
  return name === code ? code : `${name} (${code})`;
}

/** How a locale should be shown. */
export interface FormatLocaleOptions {
  /** Language to name the locale in. Defaults to the UI language. */
  uiLocale?: string;
  /** Dense context: the code carries the label and the name goes to the title. */
  compact?: boolean;
  /** `short` drops the region from the name. */
  variant?: LocaleNameVariant;
}

/** A locale resolved for display. */
export interface FormattedLocale {
  /** The display name, or the code when CLDR has no name for the tag. */
  name: string;
  /** The BCP 47 tag exactly as it was given. */
  code: string;
  /** What to draw: the name normally, the code in a compact context. */
  text: string;
  /** What to put in the tooltip, the title or the accessible name. */
  title: string;
}

/**
 * Resolve a locale for display, outside React.
 *
 * The component form is `LocaleLabel`; this is the same resolution for a chart
 * axis, a document title, a CSV column header or an `aria-label` built by hand.
 * The code is returned verbatim, because a BCP 47 tag carries meaning in its
 * casing (`zh-Hant`, `sr-Latn-RS`) and reshaping it makes it a different tag.
 */
export function formatLocale(tag: string, options: FormatLocaleOptions = {}): FormattedLocale {
  const { uiLocale, compact = false, variant = "full" } = options;
  const code = tag;
  const name = resolveLocaleName(code, uiLocale, variant);
  const named = name !== code;
  return {
    name,
    code,
    text: compact ? code : name,
    title: named ? `${name} (${code})` : code,
  };
}
