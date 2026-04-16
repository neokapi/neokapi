const neokapi = require('@neokapi/kapi-react/webpack');

/** @type {import('next').NextConfig} */
module.exports = {
  // Next.js handles locale routing — locale comes from the URL path.
  // e.g., /de/about → locale is "de", /de-AT/about → locale is "de-AT"
  i18n: {
    locales: ['en', 'de', 'de-AT', 'ja'],
    defaultLocale: 'en',
  },

  webpack: (config) => {
    config.plugins.push(
      neokapi({
        locale: process.env.LOCALE,
        translationsDir: './translations',
        // Austrian German falls back to standard German, then English:
        fallbackLocales: ['de', 'en'],
        // Fail the CI build if translations are incomplete:
        strict: process.env.CI ? 'error' : 'warn',
      })
    );
    return config;
  },
};
