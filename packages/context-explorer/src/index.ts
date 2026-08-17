// @neokapi/context-explorer — framework-level, data-source-agnostic UI for the
// context explorer (Apache-2.0). One component set answers the same three
// questions at any point in the context space; a mount decides which dimensions
// are pinned and which are free.
//
//   • kapi (desktop) drives it against the local project — workspace, project
//     and stream pinned, answers resolved from the recipe and the project store.
//   • Bowrain drives it against its REST API — the dimensions free, so the same
//     views slice a workspace by project, stream, collection, coordinate and
//     language.
//
// Retrieval-first by design: by-location and by-content need no persisted edges
// and answer today. Traversal views widen as the graph does, through the same
// components and the same `reach` field.

// ── The shell ──
export { ContextExplorer } from "./ContextExplorer";
export type { ContextExplorerProps } from "./ContextExplorer";

// ── Scope: the provider, the ladder, the filter bar ──
export { ScopeProvider, useScope } from "./ScopeProvider";
export type { ScopeProviderProps, ScopeContextValue } from "./ScopeProvider";
export { LadderBreadcrumbs } from "./LadderBreadcrumbs";
export type { LadderBreadcrumbsProps } from "./LadderBreadcrumbs";
export { ScopeFilterBar, dimensionLabel } from "./ScopeFilterBar";
export type { ScopeFilterBarProps } from "./ScopeFilterBar";

// ── The three panes and by-content search ──
export { GovernsPane } from "./GovernsPane";
export type { GovernsPaneProps } from "./GovernsPane";
export { LivesPane } from "./LivesPane";
export type { LivesPaneProps } from "./LivesPane";
export { RelatesPane } from "./RelatesPane";
export type { RelatesPaneProps } from "./RelatesPane";
export { ContextSearchPanel } from "./ContextSearchPanel";
export type { ContextSearchPanelProps } from "./ContextSearchPanel";

// ── Data source + capabilities ──
export { deriveCapabilities, MINIMAL_CAPABILITIES } from "./source";
export type { ContextDataSource, ExplorerCapabilities, Awaitable } from "./source";

// ── The scope tuple and its `context://` addresses ──
export {
  LADDER,
  FILTERS,
  clearDimension,
  contextAddress,
  dimensionValue,
  isRoot,
  ladderOf,
  profileAddress,
  withDimension,
} from "./scope";
export type { Dimension, LadderRung, ScopeTuple } from "./scope";

// ── Answer shapes ──
export { SEARCH_GROUP_ORDER } from "./types";
export type {
  AnswerScope,
  BlessingRow,
  CollectionAtPoint,
  ContentAnswer,
  ContentItem,
  ContextPoint,
  ContextVoice,
  GovernanceAnswer,
  LocaleCoverage,
  OccurrenceRow,
  PrecedentHit,
  ProfileWindow,
  ProjectTermUse,
  ProjectUse,
  RelationSubject,
  RelationsAnswer,
  RuleBinding,
  RungOption,
  ScopedQuery,
  SearchAnswer,
  SearchGroup,
  TermHit,
  TermUse,
} from "./types";

// ── Presentational atoms (for surfaces composing their own layout) ──
export {
  AddressChip,
  NoteStrip,
  Pane,
  PaneError,
  PaneSkeleton,
  ReachNote,
  ValidityChip,
} from "./atoms";
