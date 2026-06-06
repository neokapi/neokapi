// Workspace demo fixtures for the .klz WorkspaceExplorer / ProjectExplorer.
//
// The sample data (JSON + Office OOXML packages + their per-sample en→fr TMX)
// now lives in the single source of truth at
// `@neokapi/kapi-playground/samples`, so the bytes are defined exactly once and
// shared with the docs CLI playground picker. This module re-exports the
// kapi-lab-facing `WorkspaceSample` API unchanged, so the explorers, their
// tests, and stories keep importing from here.

export { WORKSPACE_SAMPLES, workspaceSampleById, JSON_SAMPLE } from "@neokapi/kapi-playground";
export type { WorkspaceSample } from "@neokapi/kapi-playground";
