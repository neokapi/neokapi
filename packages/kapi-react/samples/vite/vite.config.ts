import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
import neokapi from '@neokapi/kapi-react/vite';

export default defineConfig({
  plugins: [
    // neokapi runs first (enforce: 'pre') — processes JSX before React compiles it.
    //
    // Locale resolution:
    //   LOCALE not set → dev mode, no-op, source text renders as-is
    //   LOCALE=de      → inline mode, German text baked into the build
    //   LOCALE=de-AT   → inline mode, Austrian German with fallback to standard German
    //   LOCALE=qps     → inline mode, pseudo-translated text for QA
    //
    // For runtime/OTA mode (dynamic locale switching in a SPA):
    //   Set mode: 'runtime' instead of locale.
    //   Your app detects the user's locale and calls loadTranslations() at runtime.
    neokapi({
      locale: process.env.LOCALE,
      translationsDir: './translations',
      fallbackLocales: ['en'],              // Fall back to English for missing strings
      strict: 'warn',                       // Warn about missing translations in build output
      // mode: 'runtime',                   // Uncomment for OTA dynamic loading
    }),
    react(),
  ],
});
