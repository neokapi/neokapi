// @neokapi/kapi-lab — interactive, WASM-powered explorers for learning the
// neokapi architecture by running it. Framework-agnostic: no @docusaurus/*
// imports. Hosts boot the kapi WASM via @neokapi/kapi-playground and feed the
// resulting ContentTree / FlowTrace data into these components.

export { default as FlowTracePlayer } from "./FlowTracePlayer";
export type { FlowTracePlayerProps } from "./FlowTracePlayer";

export { default as FlowGraph } from "./FlowGraph";
export { default as StepControls } from "./StepControls";
export { default as PartInspector } from "./PartInspector";
export { default as PartDetailsModal } from "./PartDetailsModal";
export type { PartDetailsModalProps } from "./PartDetailsModal";

// Explorers + shared harness.
export { default as AnatomyExplorer } from "./AnatomyExplorer";
export type { AnatomyExplorerProps } from "./AnatomyExplorer";
export { default as PipelineExplorer } from "./PipelineExplorer";
export type { PipelineExplorerProps } from "./PipelineExplorer";
export { default as ToolLab } from "./ToolLab";
export { default as RoundTripExplorer } from "./RoundTripExplorer";
export { default as FlowBuilderRunner } from "./FlowBuilderRunner";
export { default as ScriptLab } from "./ScriptLab";
export type { ScriptLabProps } from "./ScriptLab";
export { SCRIPT_EXAMPLES, SCRIPT_API_DTS } from "./scriptApi";
export type { ScriptExample } from "./scriptApi";
export { default as BlockResults } from "./BlockResults";
export type { BlockResultsProps } from "./BlockResults";
export { default as RunSequence } from "./RunSequence";
export { default as FileSource } from "./FileSource";
export type { FileSourceValue } from "./FileSource";

export { useLabRuntime } from "./useLabRuntime";
export type {
  LabRuntime,
  LabRuntimeAssets,
  LabStatus,
  InspectOutcome,
  TraceOutcome,
} from "./useLabRuntime";

export { SAMPLES, sampleById } from "./samples";
export type { LabSample } from "./samples";

export { useFlowPlayback } from "./useFlowPlayback";
export type {
  PlaybackMode,
  FlowPlaybackState,
  FlowPlaybackReturn,
  FrameDelta,
} from "./useFlowPlayback";

export * from "./types";
