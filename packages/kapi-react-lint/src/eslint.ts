/**
 * ESLint entry point. Re-exports the plugin and the shareable
 * configs. Kept as a separate file from `./index.ts` so users who
 * mix oxlint + eslint can pin imports clearly.
 */
export { plugin as default, plugin } from "./index.ts";
export { recommended } from "./configs/recommended.ts";
export { recommendedStrict } from "./configs/recommended-strict.ts";
