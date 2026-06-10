import { Languages } from "lucide-react";
import { t } from "@neokapi/kapi-react/runtime";
import { LOCALES, resolveLocale, setLocale, type Locale } from "@/lib/locale";

// Injected at build time by vite.config.ts `define`.
declare const __BUILD_STAMP__: string;

// Native-language endonyms — never translated.
const LOCALE_LABELS: Record<Locale, string> = {
  en: "English",
  nb: "Norsk (bokmål)",
};

function LanguageSwitcher() {
  const active = resolveLocale();
  return (
    <div translate="no" className="flex items-center gap-1.5 text-xs text-neutral-600">
      <Languages aria-hidden="true" className="h-3.5 w-3.5" />
      {LOCALES.map((locale, i) => (
        <span key={locale} className="flex items-center gap-1.5">
          {i > 0 && <span className="text-neutral-700">·</span>}
          <button
            onClick={() => setLocale(locale)}
            lang={locale}
            aria-pressed={locale === active}
            className={locale === active ? "text-neutral-400" : "transition hover:text-brand-400"}
          >
            {LOCALE_LABELS[locale]}
          </button>
        </span>
      ))}
    </div>
  );
}

export function Footer() {
  return (
    <footer className="border-t border-surface-700/50 px-6 py-10">
      <div className="mx-auto flex max-w-6xl flex-col items-center justify-between gap-6 sm:flex-row">
        <div className="flex items-center gap-2.5">
          <img
            src={`${import.meta.env.BASE_URL}hero-logo.png`}
            alt="Neokapi"
            className="h-6 w-6 rounded"
          />
          <span translate="no" className="font-display text-sm font-semibold text-neutral-500">
            neokapi
          </span>
        </div>

        <div className="flex flex-wrap items-center justify-center gap-6 text-xs text-neutral-600">
          <a
            href="https://github.com/neokapi/neokapi"
            target="_blank"
            rel="noopener"
            className="transition hover:text-brand-400"
          >
            GitHub
          </a>
          <a
            href="https://neokapi.github.io/web/neokapi/docs/"
            target="_blank"
            rel="noopener"
            className="transition hover:text-brand-400"
          >
            Documentation
          </a>
          <a
            href="https://github.com/neokapi/neokapi/releases"
            target="_blank"
            rel="noopener"
            className="transition hover:text-brand-400"
          >
            Releases
          </a>
          <LanguageSwitcher />
        </div>

        <div translate="no" className="text-xs text-neutral-700">
          Apache 2.0
        </div>
      </div>
      <div className="mx-auto mt-6 max-w-6xl text-center text-[11px] tabular-nums text-neutral-700/80">
        {t("built {stamp}", { stamp: __BUILD_STAMP__ })}
      </div>
    </footer>
  );
}
