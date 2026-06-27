// Frontend consumption of the Go generative-projection render AST
// (core/projection, shipped on ContentTree.render). This module is the
// TypeScript counterpart of core/projection's WalkInline + table topology: it
// decodes a run sequence into structural inline segments and reads a RenderNode
// tree's tables/lists, so the preview renders inline formatting as semantic
// markup and tables as a real grid — from the one Go-built tree, not a
// re-derivation. Pure (no React) so it is unit-testable in isolation; the
// presentational layer consumes these.

import type { RenderNode, Run } from "./types";

/** Projection roles shipped by Go (core/projection.Role*). */
export const ROLE_DOCUMENT = "document";
export const ROLE_TABLE = "table";
export const ROLE_TABLE_ROW = "table-row";
export const ROLE_TABLE_CELL = "table-cell";
export const ROLE_TABLE_HEADER = "table-header";
export const ROLE_LIST = "list";
export const ROLE_LIST_ITEM = "list-item";
export const ROLE_HEADING = "heading";
export const ROLE_TITLE = "title";
export const ROLE_CODE = "code";

/**
 * InlineSeg is one structural inline event decoded from a run sequence — the
 * mirror of core/projection.InlineSink. `open`/`close` are balanced and nested
 * for well-formed runs; `close` carries the closing run's own type.
 */
export type InlineSeg =
  | { kind: "text"; text: string }
  | { kind: "open"; type: string; attrs?: Record<string, string> }
  | { kind: "close"; type: string }
  | { kind: "placeholder"; type: string; equiv?: string; attrs?: Record<string, string> };

/**
 * inlineSegments decodes a run sequence into inline events, mirroring Go's
 * projection.WalkInline: text runs pass through; paired codes (pcOpen/pcClose)
 * become open/close; placeholders become self-closing events; plural/select
 * recurse into their "other" branch (or the first branch present). Sub runs are
 * skipped (sub-document content projects as its own node).
 */
export function inlineSegments(runs: Run[] | undefined): InlineSeg[] {
  const out: InlineSeg[] = [];
  walk(runs ?? [], out);
  return out;
}

function walk(runs: Run[], out: InlineSeg[]): void {
  for (const r of runs) {
    if (r.text !== undefined) {
      out.push({ kind: "text", text: r.text });
    } else if (r.pcOpen) {
      out.push({ kind: "open", type: r.pcOpen.type ?? "", attrs: r.pcOpen.attrs });
    } else if (r.pcClose) {
      out.push({ kind: "close", type: r.pcClose.type ?? "" });
    } else if (r.ph) {
      out.push({
        kind: "placeholder",
        type: r.ph.type ?? "",
        equiv: r.ph.equiv,
        attrs: r.ph.attrs,
      });
    } else if (r.plural) {
      walk(pluralBranch(r.plural.forms), out);
    } else if (r.select) {
      walk(pluralBranch(r.select.cases), out);
    }
  }
}

function pluralBranch(branches: Record<string, Run[]>): Run[] {
  if (branches.other) return branches.other;
  for (const k of Object.keys(branches)) return branches[k];
  return [];
}

/** A reconstructed table: rows of cells, each cell carrying span + header info. */
export interface RenderTable {
  rows: RenderTableCell[][];
}
export interface RenderTableCell {
  node: RenderNode;
  header: boolean;
  colSpan: number;
  rowSpan: number;
}

/**
 * tableFromNode reads a RoleTable RenderNode into a rows-of-cells grid, honoring
 * colSpan/rowSpan and the header flag, so a serializer/component renders a real
 * `<table>`. Returns null when the node is not a table.
 */
export function tableFromNode(node: RenderNode): RenderTable | null {
  if (node.role !== ROLE_TABLE) return null;
  const rows: RenderTableCell[][] = [];
  for (const row of node.children ?? []) {
    if (row.role !== ROLE_TABLE_ROW) continue;
    const cells: RenderTableCell[] = [];
    for (const cell of row.children ?? []) {
      cells.push({
        node: cell,
        header: cell.header === true || cell.role === ROLE_TABLE_HEADER,
        colSpan: cell.colSpan && cell.colSpan > 0 ? cell.colSpan : 1,
        rowSpan: cell.rowSpan && cell.rowSpan > 0 ? cell.rowSpan : 1,
      });
    }
    rows.push(cells);
  }
  return { rows };
}

/** Plain-text flattening of a leaf node's runs (text runs only) — for labels/tests. */
export function nodeText(node: RenderNode): string {
  let out = "";
  for (const seg of inlineSegments(node.runs)) {
    if (seg.kind === "text") out += seg.text;
  }
  return out;
}

/** True when a node renders as a heading (heading/title), with its level. */
export function headingLevel(node: RenderNode): number | null {
  if (node.role === ROLE_HEADING || node.role === ROLE_TITLE) {
    return node.level && node.level > 0 ? node.level : node.role === ROLE_TITLE ? 1 : 2;
  }
  return null;
}
