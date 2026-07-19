/**
 * Oxlint entry point. Oxlint's `jsPlugins` accepts a path or package
 * that default-exports an ESLint-v9-compatible plugin object.
 *
 * Usage in `.oxlintrc.json`:
 *   {
 *     "jsPlugins": ["@neokapi/i18n-react-lint/oxlint"],
 *     "rules": {
 *       "neokapi-i18n/t-literal-first-arg": "error",
 *       "neokapi-i18n/t-no-concat": "error",
 *       "neokapi-i18n/no-concat-in-translatable-attr": "error",
 *       "neokapi-i18n/no-string-literal-jsx-expr": "warn"
 *     }
 *   }
 */
export { plugin as default, plugin } from "./index.ts";
