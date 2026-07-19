import { plugin } from "../index.ts";

/**
 * Strict config — everything on as `error`, including the higher-FP
 * `prefer-t-for-label-props`. Use in CI once the team has vetted the
 * codebase; expect some file-level disables for data arrays that are
 * genuinely not user-facing.
 */
export const recommendedStrict = {
  plugins: {
    "neokapi-i18n": plugin,
  },
  rules: {
    "neokapi-i18n/t-literal-first-arg": "error",
    "neokapi-i18n/t-no-concat": "error",
    "neokapi-i18n/no-concat-in-translatable-attr": "error",
    "neokapi-i18n/no-ternary-in-translatable-attr": "error",
    "neokapi-i18n/no-ternary-literals-in-jsx-child": "error",
    "neokapi-i18n/no-string-literal-jsx-expr": "error",
    "neokapi-i18n/prefer-t-for-label-props": "error",
    "neokapi-i18n/prefer-t-for-label-expr": "error",
  },
} as const;
