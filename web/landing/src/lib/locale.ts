// Locale selection for the static landing page. The page defaults to
// English; a visitor picks Norwegian Bokmål via the footer switcher or a
// `?lang=nb` link. The choice persists to localStorage so it survives
// navigation and reloads.
export const LOCALES = ["en", "nb"] as const;
export type Locale = (typeof LOCALES)[number];

const STORAGE_KEY = "neokapi.landing.lang";

function isLocale(value: string | null): value is Locale {
  return value !== null && (LOCALES as readonly string[]).includes(value);
}

/** Resolve the active locale: ?lang= wins, then localStorage, then "en". */
export function resolveLocale(): Locale {
  const fromQuery = new URLSearchParams(window.location.search).get("lang");
  if (isLocale(fromQuery)) {
    localStorage.setItem(STORAGE_KEY, fromQuery);
    return fromQuery;
  }
  const stored = localStorage.getItem(STORAGE_KEY);
  if (isLocale(stored)) return stored;
  return "en";
}

/**
 * Persist the chosen locale and reload. A full reload keeps the page simple:
 * the translation dictionary is loaded once, before the app module is
 * imported, so module-scope `t()` data resolves in the active locale.
 */
export function setLocale(locale: Locale): void {
  localStorage.setItem(STORAGE_KEY, locale);
  const url = new URL(window.location.href);
  url.searchParams.delete("lang");
  window.history.replaceState(null, "", url);
  window.location.reload();
}
