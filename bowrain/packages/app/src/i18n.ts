import { loadTranslations, setTranslations } from "@neokapi/i18n-react/runtime";

/**
 * UI-locale plumbing for the shared Bowrain app (neokapi-i18n runtime mode).
 *
 * The compiled per-locale dictionaries ship as static assets at
 * `public/translations/<locale>.json` in every shell (web, desktop webview,
 * ctrl, pulse) — the same layout kapi-desktop uses. The active locale is a
 * per-host preference stored in localStorage; both shells read it at startup
 * and the user-settings language picker writes it. A missing or partial
 * catalog is never an error: untranslated strings render the English source
 * (see CLAUDE.md "Target-language drift must never block the build").
 *
 * Server-side profile persistence of the preference is a follow-up — the
 * storage key keeps the choice per browser/app install for now.
 */

export const UI_LOCALE_STORAGE_KEY = "bowrain.ui-locale";

export type UILocale = "en" | "nb" | "qps";

/** Locales offered by the language picker. `qps` (pseudo) is for development and verification only. */
export const UI_LOCALES: ReadonlyArray<{ value: UILocale; label: string }> = [
  { value: "en", label: "English" },
  { value: "nb", label: "Norsk bokmål" },
];

function isUILocale(value: string): value is UILocale {
  return value === "en" || value === "nb" || value === "qps";
}

/** The persisted UI locale, defaulting to English. Safe without localStorage. */
export function getStoredUILocale(): UILocale {
  try {
    const raw = window.localStorage.getItem(UI_LOCALE_STORAGE_KEY);
    return raw && isUILocale(raw) ? raw : "en";
  } catch {
    return "en";
  }
}

/**
 * Activate a locale: English resets to the source dictionary; any other
 * locale loads its compiled catalog from the shell's static assets. Load
 * failures fall back to English source text silently — translations are
 * pending work, never a blocker.
 */
export async function applyUILocale(locale: UILocale): Promise<void> {
  if (locale === "en") {
    setTranslations("en", {});
    return;
  }
  try {
    await loadTranslations(locale, `/translations/${locale}.json`);
  } catch {
    // Catalog missing — keep rendering English source.
  }
}

/** Persist and activate a locale (the language picker's onChange). */
export async function setUILocale(locale: UILocale): Promise<void> {
  try {
    window.localStorage.setItem(UI_LOCALE_STORAGE_KEY, locale);
  } catch {
    // Persistence is best-effort (e.g. storage disabled); still apply.
  }
  await applyUILocale(locale);
}
