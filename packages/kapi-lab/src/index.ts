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
export { default as KlfExplorer } from "./KlfExplorer";
export type { KlfExplorerProps } from "./KlfExplorer";
export { default as KlfConformance } from "./KlfConformance";
export type { KlfConformanceProps } from "./KlfConformance";
export { default as WorkspaceExplorer } from "./WorkspaceExplorer";
export type { WorkspaceExplorerProps } from "./WorkspaceExplorer";
export { default as ProjectExplorer } from "./ProjectExplorer";
export type { ProjectExplorerProps } from "./ProjectExplorer";
export { WORKSPACE_SAMPLES, workspaceSampleById } from "./workspaceSamples";
export type { WorkspaceSample } from "./workspaceSamples";
export { KLF_SAMPLES, klfSampleById, klfText, ANNOTATIONS_KLFL } from "./klfFixtures";
export type { KlfSample } from "./klfFixtures";
export { SCRIPT_EXAMPLES, SCRIPT_API_DTS } from "./scriptApi";
export type { ScriptExample } from "./scriptApi";
export { default as BlockResults } from "./BlockResults";
export type { BlockResultsProps } from "./BlockResults";
export { default as RunSequence } from "./RunSequence";
export { default as FileSource } from "./FileSource";
export type { FileSourceValue } from "./FileSource";

// File management + selection.
export { default as FileExplorer } from "./FileExplorer";
export type { FileExplorerProps } from "./FileExplorer";
export { default as FileSelectorField } from "./FileSelectorField";
export type { FileSelectorFieldProps } from "./FileSelectorField";
export { useFileLibrary, resolveSelection, selectionSummary, EMPTY_SELECTION } from "./fileLibrary";
export type {
  FileLibrary,
  LibFile,
  FileOrigin,
  FileSelection,
  SelectionMode,
  UseFileLibraryOptions,
} from "./fileLibrary";
export { FileIcon, fileType, extOf } from "./fileTypes";
export type { FileTypeInfo, FileGroup, FileIconProps } from "./fileTypes";
export { isGlob, globToRegExp, matchGlob, globMatches } from "./glob";
export { downloadBytes, downloadText, formatBytes } from "./download";

// Code + content viewers.
export { default as CodeView } from "./CodeView";
export type { CodeViewProps } from "./CodeView";
export { detectLang, tokenize } from "./highlight";
export type { Lang, Token, TokenType } from "./highlight";
export { default as BlockInspector } from "./BlockInspector";
export type { BlockInspectorProps } from "./BlockInspector";
export { default as ContentTreeView, Stats as ContentStatsBar } from "./ContentTreeView";
export type { ContentTreeViewProps } from "./ContentTreeView";
export { default as OutputView } from "./OutputView";
export type { OutputViewProps } from "./OutputView";
export { default as BatchExplorer } from "./BatchExplorer";
export type { BatchExplorerProps } from "./BatchExplorer";

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
