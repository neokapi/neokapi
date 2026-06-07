// @neokapi/kapi-lab — interactive, WASM-powered explorers for learning the
// neokapi architecture by running it. Framework-agnostic: no @docusaurus/*
// imports. Hosts boot the kapi WASM via @neokapi/kapi-playground and feed the
// resulting ContentTree / FlowTrace data into these components.
//
// The shared preview kit (DocumentViewer, FormatPreview, FileBrowser,
// BlockInspector, CodeView, renderDoc, the ContentTree / FlowTrace types, …)
// lives in @neokapi/ui-primitives/preview — import it from there directly. This
// module exports only kapi-lab's own runtime + explorer surface.

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
export { default as FileSource } from "./FileSource";
export type { FileSourceValue } from "./FileSource";

// File management + selection.
export { default as FileExplorer } from "./FileExplorer";
export type { FileExplorerProps } from "./FileExplorer";
export { default as FileSelectorField } from "./FileSelectorField";
export type { FileSelectorFieldProps } from "./FileSelectorField";
export { default as ActiveFileSwitcher } from "./ActiveFileSwitcher";
export type { ActiveFileSwitcherProps } from "./ActiveFileSwitcher";
export { useFileLibrary, resolveSelection, selectionSummary, EMPTY_SELECTION } from "./fileLibrary";
export type {
  FileLibrary,
  LibFile,
  FileOrigin,
  FileSelection,
  SelectionMode,
  UseFileLibraryOptions,
} from "./fileLibrary";
export { isGlob, globToRegExp, matchGlob, globMatches } from "./glob";

// The engine-output adapter (reads bytes from the in-memory FS, diffs runs) —
// renders via the shared DocumentViewer from @neokapi/ui-primitives/preview.
export { default as OutputView } from "./OutputView";
export type { OutputViewProps } from "./OutputView";

// Drop-a-file widget kit: a generic no-terminal "drop a file → see the result"
// surface plus per-tool wrappers.
export { default as ToolDropWidget } from "./ToolDropWidget";
export type {
  ToolDropWidgetProps,
  ToolDropRender,
  ToolDropStat,
  DropInput,
} from "./ToolDropWidget";
export { default as PseudoTranslateWidget } from "./PseudoTranslateWidget";
export type { PseudoTranslateWidgetProps } from "./PseudoTranslateWidget";
export { default as StatsWidget } from "./StatsWidget";
export type { StatsWidgetProps } from "./StatsWidget";
export { default as SearchReplaceWidget, buildSearchReplaceRecipe } from "./SearchReplaceWidget";
export type { SearchReplaceWidgetProps } from "./SearchReplaceWidget";
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
export type { AnnotateOptions } from "@neokapi/kapi-playground/runtime";

export { SAMPLES, sampleById } from "./samples";
export type { LabSample } from "./samples";

export { useFlowPlayback } from "./useFlowPlayback";
export type {
  PlaybackMode,
  FlowPlaybackState,
  FlowPlaybackReturn,
  FrameDelta,
} from "./useFlowPlayback";
