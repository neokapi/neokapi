// Curated result-view components (R8, issue #677).
//
// Framework-first docs widgets: instead of "run a terminal command," these show
// what the *framework* produced — the content model, before/after transforms,
// and a dual CLI ⇄ result view. All three are lazy + client-only (BrowserOnly +
// dynamic import of the heavy @neokapi/kapi-playground kit) so a docs page ships
// zero wasm until a curated view first mounts. See README.md for props + when to
// use which.

export { default as BlockPreview } from "./BlockPreview";
export type { BlockPreviewProps } from "./BlockPreview";

export { default as BeforeAfter } from "./BeforeAfter";
export type { BeforeAfterProps } from "./BeforeAfter";

export { default as DualExample } from "./DualExample";
export type { DualExampleProps, DualResult } from "./DualExample";

// Re-export the inline-sample type for authors who pass inline content rather
// than a bundled fixture name.
export type { InlineSample } from "./seed";
