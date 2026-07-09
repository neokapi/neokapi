// @neokapi/editor-grid — framework-level, data-source-agnostic building blocks
// for the localization editor/review surfaces both desktops and the web app
// rebuild across the Apache/AGPL boundary (Apache-2.0). The virtualized list
// body is owned here once; callers keep their own row markup, cells, and data
// wiring via the `renderRow` render prop (kapi-desktop feeds it from Wails,
// bowrain from its REST adapter).

export { VirtualList } from "./VirtualList";
export type {
  VirtualListProps,
  VirtualListHandle,
  VirtualRowProps,
  VirtualRowRenderArgs,
  ScrollAlign,
} from "./VirtualList";
