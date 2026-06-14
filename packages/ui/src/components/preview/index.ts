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
export { useTextTransition } from "./useTextTransition";
export type {
  TransitionEffect,
  TypewriterGranularity,
  UseTextTransitionOptions,
  TextTransitionState,
} from "./useTextTransition";

export * from "./types";
