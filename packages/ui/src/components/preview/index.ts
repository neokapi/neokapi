// The preview kit — the shared, runtime-free content previewer. It lives here in
// @neokapi/ui-primitives so every consumer (kapi-lab, kapi-playground,
// kapi-desktop, the docs site) shares one editor without a dependency cycle.
// DocumentViewer is the entry point: a tabbed Preview / Blocks / Raw / Stats /
// Download editor. The rest are its building blocks.

export { default as DocumentViewer } from "./DocumentViewer";
export type { DocumentViewerProps } from "./DocumentViewer";
export { default as FormatPreview } from "./FormatPreview";
export type { FormatPreviewProps, PreviewSide } from "./FormatPreview";
export { default as FileBrowser } from "./FileBrowser";
export type { FileBrowserProps, BrowserFile } from "./FileBrowser";
export { default as BlockInspector } from "./BlockInspector";
export type { BlockInspectorProps } from "./BlockInspector";
export { default as StructureView } from "./StructureView";
export type { StructureViewProps } from "./StructureView";
export { default as LayoutView } from "./LayoutView";
export type { LayoutViewProps } from "./LayoutView";
export { roleStyle } from "./roleStyle";
export type { RoleStyle } from "./roleStyle";
export { default as ContentTreeView, Stats as ContentStatsBar } from "./ContentTreeView";
export type { ContentTreeViewProps } from "./ContentTreeView";
export { default as DocumentRender } from "./DocumentRender";
export type { DocumentRenderProps } from "./DocumentRender";
export { default as RunSequence } from "./RunSequence";
export { default as CodeView } from "./CodeView";
export type { CodeViewProps } from "./CodeView";

export { detectLang, tokenize } from "./highlight";
export type { Lang, Token, TokenType } from "./highlight";
export { FileIcon, fileType, extOf } from "./fileTypes";
export type { FileTypeInfo, FileGroup, FileIconProps } from "./fileTypes";
export { downloadBytes, downloadText, formatBytes } from "./download";
export { treeToRenderDoc, runsText, parseCellRef, colLabel, STRUCTURE_RULES } from "./renderDoc";
export {
  inlineSegments,
  tableFromNode,
  nodeText,
  headingLevel,
} from "./projectionRender";
export type { InlineSeg, RenderTable, RenderTableCell } from "./projectionRender";
export type {
  RenderDoc,
  RenderKind,
  RenderLine,
  RenderSlide,
  RenderSheet,
  RenderCell,
  RenderPage,
  RenderSection,
  StructureRule,
} from "./renderDoc";
export { overlayStyle, resolveOverlaySpans, segmentText, overlayTypes } from "./overlayHighlight";
export type { OverlayStyle, ResolvedSpan, TextSegment } from "./overlayHighlight";
// Multimodal viewers (AD-030): timed media (subtitles/audio/video) + raster OCR.
export { default as SubtitleTimeline } from "./SubtitleTimeline";
export type { SubtitleTimelineProps } from "./SubtitleTimeline";
export { default as MediaCanvas } from "./MediaCanvas";
export type { MediaCanvasProps } from "./MediaCanvas";
export { default as OCROverlay } from "./OCROverlay";
export type { OCROverlayProps } from "./OCROverlay";
export { default as AudioPlayer } from "./AudioPlayer";
export type { AudioPlayerProps } from "./AudioPlayer";
export { default as VideoPlayer } from "./VideoPlayer";
export type { VideoPlayerProps } from "./VideoPlayer";
export { collectCues, activeCueIndex, formatTimecode, formatDuration } from "./timeline";
export type { Cue } from "./timeline";
export { flattenGeometry, topUnits, boxPercent, boxStyle, extentOf } from "./geometry";
export type { PlacedBlock, Box, BoxPercent } from "./geometry";
export { useMediaTime } from "./useMediaTime";

export { useTextTransition } from "./useTextTransition";
export type {
  TransitionEffect,
  TypewriterGranularity,
  UseTextTransitionOptions,
  TextTransitionState,
} from "./useTextTransition";

export * from "./types";
