import { plugin } from "../index.ts";

/**
 * Strict config — everything on as `error`, including the higher-FP
 * `prefer-t-for-label-props`. Use in CI once the team has vetted the
 * codebase; expect some file-level disables for data arrays that are
 * genuinely not user-facing.
 */
export const recommendedStrict = {
  plugins: {
    "kapi-react": plugin,
  },
  rules: {
    "kapi-react/t-literal-first-arg": "error",
    "kapi-react/t-no-concat": "error",
    "kapi-react/no-concat-in-translatable-attr": "error",
    "kapi-react/no-ternary-in-translatable-attr": "error",
    "kapi-react/no-string-literal-jsx-expr": "error",
    "kapi-react/prefer-t-for-label-props": "error",
    "kapi-react/prefer-t-for-label-expr": "error",
  },
} as const;
