import { plugin } from "../index.ts";

/**
 * Recommended config — safe defaults for everyday use. No type info
 * required, minimal false positives. `prefer-t-for-label-props` is
 * off here because it has the highest FP risk (see the rule doc);
 * opt in via `recommended-strict` or enable it explicitly.
 *
 * ESLint flat-config example:
 *
 *   import { recommended } from '@neokapi/i18n-react-lint/eslint';
 *   export default [recommended];
 */
export const recommended = {
  plugins: {
    "neokapi-i18n": plugin,
  },
  rules: {
    "neokapi-i18n/t-literal-first-arg": "error",
    "neokapi-i18n/t-no-concat": "error",
    "neokapi-i18n/no-concat-in-translatable-attr": "error",
    "neokapi-i18n/no-ternary-in-translatable-attr": "error",
    "neokapi-i18n/no-ternary-literals-in-jsx-child": "error",
    "neokapi-i18n/no-string-literal-jsx-expr": "warn",
    "neokapi-i18n/prefer-t-for-label-expr": "warn",
  },
} as const;
