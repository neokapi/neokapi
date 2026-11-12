// UI text through react-i18next. i18n/en.json is the source catalog; the
// dogfood recipe's bowrain-mobile collection writes the other languages beside
// it (i18n/<lang>.json), and every catalog present at build time is bundled.
// A language without a catalog, or a key a catalog lacks, falls back to English.
import i18n from "i18next";
import { getLocales } from "expo-localization";
import { initReactI18next } from "react-i18next";

import en from "../../i18n/en.json";

type Catalog = typeof en;

function bundledCatalogs(): Record<string, { translation: Catalog }> {
  const resources: Record<string, { translation: Catalog }> = { en: { translation: en } };
  // Metro's require.context lists the catalogs at bundle time; the call has to
  // be spelled out for Metro to see it. Jest has no require.context, and tests
  // read English only.
  if (typeof require.context !== "function") return resources;
  const files = require.context("../../i18n", false, /\.json$/);
  for (const key of files.keys()) {
    const lang = key.replace(/^\.\//, "").replace(/\.json$/, "");
    if (lang !== "en") resources[lang] = { translation: files(key) as Catalog };
  }
  return resources;
}

export function deviceLanguage(): string {
  return getLocales()[0]?.languageCode ?? "en";
}

void i18n.use(initReactI18next).init({
  resources: bundledCatalogs(),
  lng: deviceLanguage(),
  fallbackLng: "en",
  interpolation: { escapeValue: false },
  returnNull: false,
});

export default i18n;
