// @neokapi/context-atlas: an explorable atlas of a project's context
// (Apache-2.0). It reads the `kapi context atlas --json` contract and draws
// one query across linked views: a time brush, a dimension flow, a matrix, a
// raster over time, a tile map and the records themselves, with a detail
// pane for one record and an A/B comparison.
//
// The model under `./model` is pure and React-free, so a host, a test or an
// agent can resolve an investigation's query against the same data.

// ── The shell ──
export { ContextAtlas } from "./ContextAtlas";
export type { ContextAtlasProps } from "./ContextAtlas";

// ── Parts, for surfaces composing their own layout ──
export { TimeBrush } from "./components/TimeBrush";
export type { TimeBrushProps } from "./components/TimeBrush";
export { ScopeChips } from "./components/ScopeChips";
export type { ScopeChipsProps } from "./components/ScopeChips";
export { DimensionFlow } from "./components/DimensionFlow";
export type { DimensionFlowProps } from "./components/DimensionFlow";
export { MatrixView } from "./components/MatrixView";
export type { MatrixViewProps } from "./components/MatrixView";
export { RasterView } from "./components/RasterView";
export type { RasterViewProps } from "./components/RasterView";
export { MapView } from "./components/MapView";
export type { MapViewProps } from "./components/MapView";
export { RecordsTable } from "./components/RecordsTable";
export type { RecordsTableProps } from "./components/RecordsTable";
export { RecordInspector } from "./components/RecordInspector";
export type { RecordInspectorProps } from "./components/RecordInspector";
export { SelectionPane } from "./components/SelectionPane";
export type { SelectionPaneProps } from "./components/SelectionPane";
export { CompareView } from "./components/CompareView";
export type { CompareViewProps } from "./components/CompareView";
export { StateBadge } from "./components/StateBadge";
export type { StateBadgeProps } from "./components/StateBadge";

// ── Names and colours ──
export { axisLabel, valueLabel, stateLabel, questionLabel } from "./labels";
export { ATLAS_STYLE, valueFill, stateFill } from "./colors";

// ── The contract ──
export { ATLAS_FORMAT, ATLAS_VERSION } from "./types";
export type {
  AtlasActor,
  AtlasAxis,
  AtlasAxisKind,
  AtlasAxisSource,
  AtlasData,
  AtlasEvent,
  AtlasEvidence,
  AtlasGeometry,
  AtlasPlace,
  AtlasPlaceType,
  AtlasRecord,
  AtlasSpan,
  AtlasState,
  AtlasValue,
} from "./types";

// ── The model ──
export * from "./model";
