import { plugin } from "../index.ts";

/**
 * Recommended config — safe defaults for everyday use. No type info
 * required, minimal false positives. `prefer-t-for-label-props` is
 * off here because it has the highest FP risk (see the rule doc);
 * opt in via `recommended-strict` or enable it explicitly.
 *
 * ESLint flat-config example:
 *
 *   import { recommended } from '@neokapi/kapi-react-lint/eslint';
 *   export default [recommended];
 */
export const recommended = {
  plugins: {
    "kapi-react": plugin,
  },
  rules: {
    "kapi-react/t-literal-first-arg": "error",
    "kapi-react/t-no-concat": "error",
    "kapi-react/no-concat-in-translatable-attr": "error",
    "kapi-react/no-ternary-in-translatable-attr": "error",
    "kapi-react/no-ternary-literals-in-jsx-child": "error",
    "kapi-react/no-string-literal-jsx-expr": "warn",
    "kapi-react/prefer-t-for-label-expr": "warn",
  },
} as const;
