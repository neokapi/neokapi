// Framework concept UI (R4) — the Concepts section now reuses @neokapi/concept-ui
// (the same components kapi-desktop runs), driven by a RestConceptDataSource over
// the workspace ApiAdapter with bowrain's full feature set + governed editing.
export { ConceptsSection } from "./ConceptsSection";
export type { ConceptsSectionProps } from "./ConceptsSection";
export { ConceptStorySection } from "./ConceptStorySection";
export type { ConceptStorySectionProps } from "./ConceptStorySection";
export { ConceptEditDialog } from "./ConceptEditDialog";
export type { ConceptEditDialogProps } from "./ConceptEditDialog";
export {
  createRestConceptSource,
  GovernedEditError,
  isGovernedEditError,
  asGovernedEditError,
} from "./restConceptSource";
export type { RestConceptSourceOptions } from "./restConceptSource";
