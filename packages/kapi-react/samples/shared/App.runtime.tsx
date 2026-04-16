/**
 * Sample app entry with OTA runtime language switching.
 *
 * This is the ONLY file that needs i18n-related code.
 * All other components (like App.tsx) stay vanilla JSX.
 *
 * In runtime mode, the plugin emits:
 *   - t() calls for plain text (string return)
 *   - tx() calls for text with inline elements like <a>, <strong> (ReactNode return)
 * Both are auto-generated — the developer doesn't choose which to use.
 *
 * The user's locale is determined by YOUR app — the plugin doesn't detect
 * it automatically.
 */

import { useEffect, useState } from 'react';
import { loadTranslations, useNeokapi } from '@neokapi/kapi-react/runtime';
import App from './App';

const SUPPORTED_LOCALES = ['en', 'de', 'de-AT', 'ja'];

/**
 * Detect the user's preferred locale.
 *
 * This is YOUR logic — the plugin doesn't do this for you.
 * Common strategies:
 *   1. Saved user preference (localStorage, cookie, user profile)
 *   2. URL path (/de/about → "de")
 *   3. Browser language (navigator.language)
 *   4. Server-side detection (Accept-Language header, GeoIP)
 */
function detectLocale(): string {
  // 1. Check saved preference
  const saved = localStorage.getItem('locale');
  if (saved && SUPPORTED_LOCALES.includes(saved)) return saved;

  // 2. Check browser language (includes regional variant, e.g. "de-AT")
  const browserLang = navigator.language;
  if (SUPPORTED_LOCALES.includes(browserLang)) return browserLang;

  // 3. Try base language
  const baseLang = browserLang.split('-')[0];
  if (SUPPORTED_LOCALES.includes(baseLang)) return baseLang;

  // 4. Default
  return 'en';
}

export default function Root() {
  const [locale, setLocaleState] = useState(detectLocale);
  useNeokapi(); // subscribe to translation changes for re-rendering

  // Load translations on mount and when locale changes
  useEffect(() => {
    if (locale !== 'en') {
      loadTranslations(locale, `/translations/${locale}.json`);
    }
    localStorage.setItem('locale', locale);
  }, [locale]);

  return (
    <div>
      <nav>
        <label>
          Language:{' '}
          <select value={locale} onChange={e => setLocaleState(e.target.value)}>
            <option value="en">English</option>
            <option value="de">Deutsch</option>
            <option value="de-AT">Deutsch (Osterreich)</option>
            <option value="ja">Japanese</option>
          </select>
        </label>
      </nav>

      {/* App component is vanilla JSX — no i18n imports, no changes.
          The plugin auto-generates t() for plain text and tx() for
          text with inline elements like <a> and <strong>. */}
      <App />
    </div>
  );
}
