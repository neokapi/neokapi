// text-direction — derive writing direction from a BCP-47 locale tag, and the
// bidi-isolation helpers every surface that renders target text needs.
//
// Rendering Arabic/Hebrew/Persian/Urdu target text inside an LTR container is
// not merely cosmetic: without `dir="rtl"` the Unicode Bidirectional Algorithm
// resolves the paragraph direction as LTR, so trailing punctuation, digits, and
// any embedded Latin run (a placeholder, a URL, a code identifier) land in the
// wrong visual order. The fix is two-part:
//
//   1. the block element carries `dir` + `lang` for the text's own locale, and
//   2. embedded opposite-direction fragments are *isolated* (`<bdi>`), so their
//      internal ordering cannot leak out and reorder the surrounding text.
//
// Direction is derived from the tag, never from the content: a locale is the
// authoritative signal, and content-sniffing ("first strong character") mislabels
// an Arabic string that happens to start with a Latin product name.
//
// {@link DirectionalText} is how part 1 is applied: the one element a
// neokapi Block/Run's own source or target text is ever rendered through.
// `directionAttrs`/`localeDirection` below are its internals, exported for the
// few sites (a `useMemo`d attrs object, a non-JSX consumer) that need the raw
// pair — a new call site reaches for the component, not these.
//
// The element `dir`/`lang` land on matters as much as computing them
// correctly: CSS `text-align` resolves against the nearest block or
// (blockified) flex-item box, not an inline descendant, so a `<span>{text}</span>`
// with the attributes one layer away — an inner span, a sibling, a wrapper the
// text isn't actually inside — compiles and renders identically to correct
// code until RTL content reaches it. Nothing short of never hand-writing that
// element closes the gap; `DirectionalText` is that closure.

import type React from "react";

/** Writing direction of a text run. */
export type TextDirection = "ltr" | "rtl";

/**
 * Right-to-left script subtags (ISO 15924). When a tag carries an explicit
 * script, it decides — `az-Arab` is RTL, `az-Latn` is LTR — regardless of what
 * the language subtag's default script would be.
 */
const RTL_SCRIPTS = new Set([
  "adlm",
  "arab",
  "aran",
  "armi",
  "avst",
  "chrs",
  "cprt",
  "elym",
  "gara",
  "hatr",
  "hebr",
  "hung",
  "khar",
  "lydi",
  "mand",
  "mani",
  "marc",
  "mend",
  "merc",
  "mero",
  "narb",
  "nbat",
  "nkoo",
  "orkh",
  "ougr",
  "palm",
  "phli",
  "phlp",
  "phnx",
  "prti",
  "rohg",
  "samr",
  "sarb",
  "sogd",
  "sogo",
  "syrc",
  "thaa",
  "todr",
  "yezi",
]);

/**
 * Language subtags whose default script is right-to-left (CLDR
 * `characterOrder = right-to-left`). Only consulted when the tag carries no
 * explicit script subtag.
 */
const RTL_LANGUAGES = new Set([
  "aeb", // Tunisian Arabic
  "ar", // Arabic
  "arc", // Aramaic
  "arq", // Algerian Arabic
  "ary", // Moroccan Arabic
  "arz", // Egyptian Arabic
  "azb", // South Azerbaijani
  "bal", // Baluchi
  "bcc", // Southern Balochi
  "bqi", // Bakhtiari
  "brh", // Brahui
  "ckb", // Central Kurdish (Sorani)
  "dv", // Divehi
  "fa", // Persian
  "glk", // Gilaki
  "he", // Hebrew
  "iw", // Hebrew (legacy code)
  "khw", // Khowar
  "ks", // Kashmiri
  "ku", // Kurdish (Sorani-script default in CLDR's ku-Arab; see note below)
  "lrc", // Northern Luri
  "mzn", // Mazanderani
  "nqo", // N'Ko
  "pnb", // Western Punjabi
  "prs", // Dari
  "ps", // Pashto
  "sd", // Sindhi
  "sdh", // Southern Kurdish
  "skr", // Saraiki
  "syr", // Syriac
  "ug", // Uyghur
  "ur", // Urdu
  "uzs", // Southern Uzbek
  "yi", // Yiddish
  "ji", // Yiddish (legacy code)
]);

/**
 * Language subtags that CLDR defaults to a left-to-right script even though a
 * right-to-left variant exists. An explicit script subtag still wins, so this
 * only guards the bare tag (`ku` alone is Kurmanji/Latin in practice).
 */
const LTR_OVERRIDES = new Set(["ku", "kmr"]);

/** Split a BCP-47 tag into its lowercased language and script subtags. */
function subtags(locale: string): { language: string; script?: string } {
  // Accept POSIX-ish separators too — locale ids reach the UI from file names
  // (`messages_ar_EG.properties`) as often as from a recipe.
  const parts = locale
    .trim()
    .replace(/[_@.]/g, "-")
    .split("-")
    .filter((p) => p !== "");
  if (parts.length === 0) return { language: "" };
  const language = parts[0].toLowerCase();
  // BCP-47 is positional: language[-script][-region][-variant…][-extension…],
  // so the script — four alphabetic characters — can only be the subtag
  // immediately after the language. Searching the whole tag instead would let a
  // Unicode extension value masquerade as a script: `en-US-u-nu-arab` asks for
  // Arabic *numerals* in English, and would otherwise resolve as right-to-left.
  const next = parts[1];
  const script = next !== undefined && /^[A-Za-z]{4}$/.test(next) ? next.toLowerCase() : undefined;
  return script ? { language, script } : { language };
}

/**
 * The writing direction for a locale tag. Unknown, empty, and undefined tags
 * resolve to `"ltr"` — the safe default, and what the DOM would do anyway.
 *
 * ```ts
 * localeDirection("ar")        // "rtl"
 * localeDirection("ar-EG")     // "rtl"
 * localeDirection("az-Arab")   // "rtl"  (explicit script wins)
 * localeDirection("he-IL")     // "rtl"
 * localeDirection("pt-BR")     // "ltr"
 * ```
 */
export function localeDirection(locale: string | undefined | null): TextDirection {
  if (!locale) return "ltr";
  const { language, script } = subtags(locale);
  if (script) return RTL_SCRIPTS.has(script) ? "rtl" : "ltr";
  if (LTR_OVERRIDES.has(language)) return "ltr";
  return RTL_LANGUAGES.has(language) ? "rtl" : "ltr";
}

/** True when the locale is written right-to-left. */
export function isRTLLocale(locale: string | undefined | null): boolean {
  return localeDirection(locale) === "rtl";
}

/**
 * The `dir`/`lang` attribute pair for a block of text in `locale`.
 *
 * Always returns `dir` — spelling out `dir="ltr"` matters as much as `dir="rtl"`
 * once a surface can nest one direction inside the other (an English source
 * panel beside an Arabic target, an LTR key beside an RTL value). `lang` is
 * omitted for an unknown locale so we never assert a bogus tag to a screen
 * reader.
 */
export function directionAttrs(locale: string | undefined | null): {
  dir: TextDirection;
  lang?: string;
} {
  const dir = localeDirection(locale);
  const lang = locale?.trim();
  return lang ? { dir, lang } : { dir };
}

type DirectionalTextOwnProps = {
  /**
   * The BCP-47 locale this text is written in — direction and `lang` are
   * derived from it. Omit for text of unknown locale (resolves to `ltr`,
   * the safe default); never pass `dir`/`lang` yourselves instead, that is
   * exactly the mistake this component exists to make unavailable.
   */
  locale?: string | null;
  /**
   * Force a direction regardless of `locale` — for content that is never
   * prose in the surrounding language no matter what locale it's tagged
   * with, such as source code or a raw identifier (always `"ltr"`).
   */
  dir?: TextDirection;
};

/** {@link DirectionalText}'s props: its own two, plus whatever `as`'s element accepts. */
export type DirectionalTextProps<E extends React.ElementType> = DirectionalTextOwnProps & {
  /** The element to render as — whichever tag would otherwise have carried the text. */
  as?: E;
} & Omit<React.ComponentPropsWithoutRef<E>, keyof DirectionalTextOwnProps | "as">;

/**
 * The element a neokapi Block's or Run's own source or target text is
 * rendered through — see the module comment for why this exists rather than
 * a `<span {...directionAttrs(locale)}>` written at the call site. Renders as
 * `as` (default `"span"`; pass `"div"`/`"li"`/`"td"`/`"p"`/… to match
 * whatever element the surrounding layout needs) with every other prop
 * (`className`, `title`, `children`, event handlers, …) passed through
 * unchanged — it is a direction-aware substitute for that element, not a
 * wrapper around it.
 *
 * ```tsx
 * <DirectionalText as="li" locale={line.sourceLocale} className={styles.bullet}>
 *   {line.text}
 * </DirectionalText>
 * ```
 */
export function DirectionalText<E extends React.ElementType = "span">({
  as,
  locale,
  dir,
  ...rest
}: DirectionalTextProps<E>): React.ReactElement {
  const Tag = (as ?? "span") as React.ElementType;
  const attrs = dir ? { dir } : directionAttrs(locale);
  return <Tag {...attrs} {...rest} />;
}

/**
 * True when `inner` must be bidi-isolated from a surrounding run in `outer`.
 *
 * Isolation is only *required* across a direction boundary, but `<bdi>` is
 * harmless when the directions agree, so callers may isolate unconditionally.
 */
export function needsIsolation(
  outer: string | undefined | null,
  inner: string | undefined | null,
): boolean {
  return localeDirection(outer) !== localeDirection(inner);
}

/**
 * The locale part of a target variant key ("ar-EG", "ar-EG#formal",
 * "ar-EG|social" → "ar-EG"): a variant key is the locale plus an optional
 * `#tone` and/or `|channel` suffix (model.VariantKey), and only the locale
 * part decides writing direction.
 */
export function localeOfVariant(variant: string): string {
  const cut = variant.search(/[#|]/);
  return cut === -1 ? variant : variant.slice(0, cut);
}
