/**
 * @neokapi/kapi-react runtime — thin translation layer for OTA mode.
 *
 * ~2KB total. Only loaded when mode='runtime'. Inline mode needs no runtime at all.
 *
 * Features:
 *   - t(text, params?) — mark a string for translation outside JSX
 *     (plugin rewrites each call into a hash-based lookup)
 *   - setTranslations() / loadTranslations() — update dictionary, trigger re-renders
 *   - useNeokapi() — React hook for reactive translation updates
 *
 * Internal exports `__t` / `__tx` power plugin-injected call sites
 * and should not be used directly from application code.
 */

import type { ReactNode } from "react";
import {
  createElement,
  cloneElement,
  isValidElement,
  Fragment,
  useSyncExternalStore,
  useCallback,
} from "react";
import { resolveICU } from "./icu.ts";

// ─── Translation store ───────────────────────────────────────

let currentLocale = "";
let dict: Record<string, string> = {};
let version = 0;
const listeners = new Set<() => void>();

function notify() {
  version++;
  listeners.forEach((fn) => fn());
}

export interface SetTranslationsOptions {
  /**
   * Sync `<html lang="…" dir="…">` to match the new locale.
   * Defaults to `true` in a browser environment, `false` under SSR
   * (where `document` isn't defined). Pass `false` when you render
   * multiple locales on one page (e.g. a demo switcher that only
   * localises a subtree) or when your app manages these attributes
   * itself.
   */
  syncDocumentLocale?: boolean;
}

/**
 * Set the active translation dictionary. Triggers re-render of all
 * components using useNeokapi(), and — by default — pushes the
 * locale onto `<html lang="…">` plus a matching `dir="ltr|rtl"`
 * attribute.
 */
export function setTranslations(
  locale: string,
  translations: Record<string, string>,
  options: SetTranslationsOptions = {},
) {
  currentLocale = locale;
  dict = translations;
  const sync = options.syncDocumentLocale ?? typeof document !== "undefined";
  if (sync) syncDocumentLocale(locale);
  notify();
}

/**
 * Fetch a translation file from a URL and activate it. Forwards
 * `syncDocumentLocale` to `setTranslations`.
 */
export async function loadTranslations(
  locale: string,
  url: string,
  options: SetTranslationsOptions = {},
): Promise<void> {
  const response = await fetch(url);
  const translations = await response.json();
  setTranslations(locale, translations, options);
}

// Writing-direction defaults per primary language subtag. Covers
// the common RTL scripts — Arabic, Hebrew, Farsi, Urdu, Yiddish,
// Pashto, Sindhi, Divehi, Kurdish (Sorani), Aramaic, Samaritan.
// Add more via the second arg to `setTranslations` / `loadTranslations`
// if you need to override.
const RTL_LANGS = new Set(["ar", "dv", "fa", "he", "ku", "ps", "sd", "ur", "yi", "arc", "sam"]);

function isRTL(locale: string): boolean {
  const primary = locale.split(/[-_]/)[0].toLowerCase();
  return RTL_LANGS.has(primary);
}

/**
 * Push the locale onto `document.documentElement` — `lang` for
 * assistive tech + browser defaults (fonts, hyphenation, spelling),
 * `dir` for writing direction. Exposed so advanced callers can
 * invoke it without also swapping the dict (unusual).
 */
export function syncDocumentLocale(locale: string): void {
  if (typeof document === "undefined") return;
  const html = document.documentElement;
  if (!html) return;
  html.setAttribute("lang", locale);
  html.setAttribute("dir", isRTL(locale) ? "rtl" : "ltr");
}

// ─── Runtime string transform hook ───────────────────────────

/**
 * Optional post-lookup transform applied to every translated string
 * before parameter substitution. Set to non-null to install a
 * transform — the runtime `@neokapi/kapi-react/runtime/pseudo`
 * subpath uses this to wire on-the-fly pseudo-translation.
 *
 * Runs AFTER dict lookup + ICU resolution but BEFORE `{param}`
 * substitution, so the transform sees `{foo}` / `{=m0}` tokens
 * still in place and can choose to preserve them.
 */
let stringTransform: ((text: string) => string) | null = null;

export function setStringTransform(fn: ((text: string) => string) | null): void {
  stringTransform = fn;
  notify();
}

// ─── String translation ──────────────────────────────────────

/**
 * Internal hash-based lookup — the plugin rewrites every JSX text
 * node and every user `t("…")` call into `__t(hash, fallback, …)` at
 * build time. Do not call directly from application code.
 */
export function __t(
  hash: string,
  fallback: string,
  params?: Record<string, string | number>,
): string {
  let text = dict[hash] ?? fallback;

  // Resolve ICU plural/select if present
  if (text.includes(", plural,") || text.includes(", select,")) {
    text = resolveICU(text, params, currentLocale);
  }

  // Post-lookup runtime transform (pseudo-translation, debug
  // markers, etc.). Runs before {param} substitution so the
  // transform can choose to preserve placeholder names.
  if (stringTransform) text = stringTransform(text);

  // Substitute {param} tokens
  if (params) {
    for (const [key, value] of Object.entries(params)) {
      text = text.replaceAll(`{${key}}`, String(value));
    }
  }

  return text;
}

// ─── Rich JSX translation ────────────────────────────────────

/**
 * Internal hash-based JSX lookup — plugin rewrites JSX text with
 * inline elements into `__tx(hash, fallback, elements, params)`.
 * Do not call directly from application code.
 */
export function __tx(
  hash: string,
  fallback: string,
  elements: Record<string, ReactNode>,
  params?: Record<string, string | number>,
): ReactNode {
  let text = dict[hash] ?? fallback;

  // Resolve ICU
  if (text.includes(", plural,") || text.includes(", select,")) {
    text = resolveICU(text, params, currentLocale);
  }

  // Post-lookup runtime transform — same hook used by __t so a
  // pseudo mode applies uniformly across plain-text and
  // element-bearing translations. Runs before placeholder
  // substitution; transforms that want to protect {=m0}-style
  // element tokens need to look for them.
  if (stringTransform) text = stringTransform(text);

  // Substitute string params first (not element tokens)
  if (params) {
    for (const [key, value] of Object.entries(params)) {
      text = text.replaceAll(`{${key}}`, String(value));
    }
  }

  // Split on element tokens {=m0}, {=m1}, etc. and interleave with elements
  const parts: ReactNode[] = [];
  const tokenRegex = /\{(=[^}]+)\}/g;
  let lastIndex = 0;
  let match;
  let hasElements = false;

  while ((match = tokenRegex.exec(text)) !== null) {
    if (match.index > lastIndex) {
      parts.push(text.slice(lastIndex, match.index));
    }
    const tokenName = match[1];
    if (elements[tokenName] !== undefined) {
      parts.push(elements[tokenName]);
      hasElements = true;
    } else {
      parts.push(match[0]);
    }
    lastIndex = match.index + match[0].length;
  }

  if (lastIndex < text.length) {
    parts.push(text.slice(lastIndex));
  }

  if (!hasElements) {
    return parts.join("");
  }

  // Return a React Fragment — NOT a wrapping <span>. A wrapper
  // collapses multi-child content (`<Play /> Run`) into a single
  // flex item of the enclosing inline-flex container, which breaks
  // shadcn-style buttons that rely on `items-center gap-N` between
  // direct children. Fragment is transparent to layout.
  //
  // Embedded elements get a cloned `key` for stable reconciliation
  // across re-renders; strings don't need keys because they flow as
  // individual children arguments.
  return createElement(
    Fragment,
    null,
    ...parts.map((part, i) =>
      typeof part === "string"
        ? part
        : isValidElement(part)
          ? cloneElement(part, { key: i })
          : createElement(Fragment, { key: i }, part),
    ),
  );
}

// ─── React hook ──────────────────────────────────────────────

/**
 * React hook for reactive translations. Re-renders when translations change.
 */
export function useNeokapi() {
  const subscribe = useCallback((callback: () => void) => {
    listeners.add(callback);
    return () => {
      listeners.delete(callback);
    };
  }, []);

  const getSnapshot = useCallback(() => version, []);

  useSyncExternalStore(subscribe, getSnapshot, getSnapshot);

  return {
    locale: currentLocale,
    setTranslations,
    loadTranslations,
  };
}

// ─── JS-context escape hatch ─────────────────────────────────

/**
 * Mark a standalone string for extraction + translation outside JSX.
 *
 * Usage:
 *
 *     t("English")
 *     t("English", "UI Language")                  // with context
 *     t("Hello, {name}!", { name })                // with params
 *     t("State", "US state", { stateCode: "CA" })  // context + params
 *
 * Context disambiguates identically-worded strings with different
 * meanings (gettext's msgctxt). It's hashed into the Block's
 * descriptor so the English source "State" under `"US state"`
 * and the same source under `"workflow status"` get different
 * translations.
 *
 * The kapi-react plugin rewrites every `t(...)` call bound to
 * `@neokapi/kapi-react/runtime` into a hash-based lookup at build
 * time, so runtime lookups resolve through the same dict loaded
 * by `loadTranslations()`.
 *
 * Without the plugin (e.g. tests, dev-mode builds) `t` just
 * returns the source text with `{name}` substitutions applied —
 * so you can use it unconditionally.
 */
export function t(text: string, context?: string, params?: Record<string, string | number>): string;
export function t(text: string, params: Record<string, string | number>): string;
export function t(
  text: string,
  contextOrParams?: string | Record<string, string | number>,
  params?: Record<string, string | number>,
): string {
  const actualParams = typeof contextOrParams === "object" ? contextOrParams : params;
  if (!actualParams) return text;
  let out = text;
  for (const [k, v] of Object.entries(actualParams)) {
    out = out.replaceAll(`{${k}}`, String(v));
  }
  return out;
}

// ─── Plural / Select authoring components ────────────────────

export { Plural, Select, Case, Zero, One, Two, Few, Many, Other, pluralKeyFor } from "./plural.tsx";

export type { PluralProps, PluralFormKey, SelectProps, CaseProps } from "./plural.tsx";
