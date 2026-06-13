// @neokapi/concept-ui — framework-level, data-source-agnostic UI for browsing a
// concept knowledge base and reading one concept's story (Apache-2.0). Drive it
// against any backend via the ConceptDataSource adapter: kapi-desktop against a
// local SQLite termbase (core only), bowrain against its REST API (core + rich
// markets/observations/comments/timeline).

// ── Views ──
export { ConceptList } from "./ConceptList";
export type { ConceptListProps } from "./ConceptList";
export { ConceptView } from "./ConceptView";
export type {
  ConceptViewProps,
  ConceptViewSlots,
  ConceptSectionProps,
  ConceptSectionRenderer,
} from "./ConceptView";
// The fully composed dashboard — wires the production panels from a data source.
export { ConceptDashboard, defaultConceptSlots } from "./ConceptDashboard";
export type { ConceptDashboardProps } from "./ConceptDashboard";

// ── Concept-view panels (section-slot renderers) ──
export { RelationsPanel } from "./RelationsPanel";
export type { RelationsPanelProps } from "./RelationsPanel";
export { MarketsPanel } from "./MarketsPanel";
export { ConceptTimeline } from "./ConceptTimeline";
export { ConstraintsPanel } from "./ConstraintsPanel";
export { ObservationsPanel } from "./ObservationsPanel";
export { CommentsPanel } from "./CommentsPanel";

// ── Adapter + capabilities ──
export { deriveCapabilities, resolveCapabilities } from "./adapter";
export type {
  ConceptDataSource,
  ConceptCapabilities,
  ConceptQuery,
  ConceptListResult,
  Awaitable,
} from "./adapter";

// ── Types ──
export { TERM_STATUSES, RELATION_TYPES, OBSERVATION_KINDS } from "./types";
export type {
  Concept,
  ConceptSummary,
  Term,
  TermStatus,
  TermSource,
  Relation,
  RelationType,
  RelationInput,
  Validity,
  Market,
  Observation,
  ObservationKind,
  Comment,
  TimelineEvent,
  TimelineKind,
  WhereUsed,
  TermRef,
  TermPatch,
} from "./types";

// ── Shared vocabulary (labels, status colours, naming) ──
export {
  TERM_STATUS_CLASS,
  TERM_STATUS_LABEL,
  RELATION_LABEL,
  relationLabel,
  primaryName,
  isBannedStatus,
  isPreferredStatus,
  isGovernedRelation,
} from "./concept-meta";

// ── Pure grouping logic (relations, geography) ──
export {
  groupRelations,
  shouldCollapse,
  RELATION_COLLAPSE_THRESHOLD,
  termsByLocale,
  termsByMarket,
  deriveMarketsFromTerms,
} from "./grouping";
export type { RelationGroup, RelationItem, LocaleTerms, MarketTermGroup } from "./grouping";

// ── Relations widget view-model (ordering + collapse) ──
export {
  buildRelationView,
  neighbourIds,
  RELATION_DISPLAY_ORDER,
  DEFAULT_RELATIONS_COLLAPSE,
} from "./relations-group";
export type { RelationView } from "./relations-group";

// ── Geography (markets) view-model ──
export { buildMarketView, orderLocaleTerms } from "./markets-view";
export type { MarketView, MarketLocaleView } from "./markets-view";

// ── Pure timeline logic ──
export { buildTimeline, sortTimeline, synthesizeTimeline, dayKey } from "./timeline";
export type { TimelineDay } from "./timeline";

// ── Concept-timeline view-model (rich/core merge + display building) ──
export {
  TIMELINE_KIND_META,
  timelineKindMeta,
  synthesizeCoreTimeline,
  resolveTimelineEvents,
  buildDisplayTimeline,
} from "./timeline-build";
export type {
  TimelineTone,
  TimelineKindMeta,
  TimelineDisplayEvent,
  TimelineDisplayDay,
  CoreTimelineOptions,
  ResolveTimelineOptions,
} from "./timeline-build";

// ── Constraints view-model (validity → lanes + banned/preferred summary) ──
export {
  buildConstraintModel,
  constraintSummary,
  marketLabelFor,
  windowPhrase,
} from "./constraints";
export type {
  ConstraintModel,
  ConstraintLane,
  ConstraintScale,
  ConstraintTick,
  ConstraintSummary,
  ConstraintPlacement,
  BuildConstraintsOptions,
  SummaryOptions,
} from "./constraints";

// ── Presentational atoms (for the section panels) ──
export {
  ConceptSection,
  StatusChip,
  RelationChip,
  LocalePill,
  EmptyHint,
  ErrorHint,
  formatDate,
  formatRelative,
} from "./atoms";

// ── Loader hook (for panels with their own async reads) ──
export { useResource } from "./useResource";
export type { ResourceState } from "./useResource";
