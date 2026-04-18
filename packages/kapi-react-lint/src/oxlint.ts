/**
 * Oxlint entry point. Oxlint's `jsPlugins` accepts a path or package
 * that default-exports an ESLint-v9-compatible plugin object.
 *
 * Usage in `.oxlintrc.json`:
 *   {
 *     "jsPlugins": ["@neokapi/kapi-react-lint/oxlint"],
 *     "rules": {
 *       "kapi-react/t-literal-first-arg": "error",
 *       "kapi-react/t-no-concat": "error",
 *       "kapi-react/no-concat-in-translatable-attr": "error",
 *       "kapi-react/no-string-literal-jsx-expr": "warn"
 *     }
 *   }
 */
export { plugin as default, plugin } from "./index.ts";
