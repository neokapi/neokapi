// A fixed, readable layout for the context graph: three columns and a row.
//
//   content          the point          what applies there
//   project          ┌────────┐         voice
//   collection  ───▶ │ point  │ ──────▶ terms: concepts…
//   source file      └────────┘         content memory
//   editions ◀───────────── memory recalls
//   ─────────────────────────────────────────────────────
//   the record: imports, decisions, notes, in time order
//
// Positions are a pure function of the model, so two snapshots lay out alike
// and a node that persists between them slides rather than jumps.

import type { GraphModel, GraphNode } from "./types.ts";

export interface Placed {
  node: GraphNode;
  x: number;
  y: number;
  w: number;
  h: number;
}

export interface Layout {
  width: number;
  height: number;
  placed: Map<string, Placed>;
  /** Column headings, with the x they start at. */
  columns: { x: number; label: string }[];
  recordY: number;
}

export const CONTENT_W = 256;
export const POINT_W = 184;
export const CONTEXT_W = 288;
/** A record pill narrower than this loses its label, so the row wraps first. */
const RECORD_MIN_W = 150;
const RECORD_H = 40;
const RECORD_GAP = 10;
const CONCEPT_H = 30;
const NODE_H = 46;
const GAP = 14;

export function layoutGraph(model: GraphModel, width = 1040): Layout {
  const placed = new Map<string, Placed>();
  const byKind = (kind: GraphNode["kind"]) => model.nodes.filter((n) => n.kind === kind);
  const put = (node: GraphNode, x: number, y: number, w: number, h: number) =>
    placed.set(node.id, { node, x, y, w, h });

  const x0 = 16;
  const x1 = x0 + CONTENT_W + 96;
  const x2 = x1 + POINT_W + 96;
  const top = 34;

  // Content column.
  let y = top;
  for (const n of byKind("project")) {
    put(n, x0, y, CONTENT_W, NODE_H);
    y += NODE_H + GAP;
  }
  for (const n of byKind("collection")) {
    put(n, x0 + 12, y, CONTENT_W - 12, 36);
    y += 36 + GAP;
  }
  for (const n of byKind("source")) {
    put(n, x0 + 24, y, CONTENT_W - 24, NODE_H);
    y += NODE_H + GAP;
  }
  const editions = byKind("edition");
  if (editions.length > 0) y += 6;
  for (const n of editions) {
    put(n, x0 + 24, y, CONTENT_W - 24, 44);
    y += 44 + 10;
  }
  const contentBottom = y;

  // Context column.
  let cy = top;
  for (const n of byKind("voice")) {
    put(n, x2, cy, CONTEXT_W, NODE_H);
    cy += NODE_H + GAP;
  }
  const termsGroup = byKind("terms");
  const concepts = byKind("concept");
  if (termsGroup.length > 0) {
    put(termsGroup[0], x2, cy, CONTEXT_W, 36);
    cy += 36 + 6;
    for (const n of concepts) {
      put(n, x2 + 16, cy, CONTEXT_W - 16, CONCEPT_H - 4);
      cy += CONCEPT_H;
    }
    cy += GAP;
  }
  for (const n of byKind("memory")) {
    put(n, x2, cy, CONTEXT_W, NODE_H);
    cy += NODE_H + GAP;
  }
  const contextBottom = cy;

  // The point sits level with the source file, or mid-column.
  const source = byKind("source")[0];
  const sourceP = source ? placed.get(source.id) : undefined;
  const pointY = sourceP ? sourceP.y - 2 : top + 60;
  for (const n of byKind("point")) put(n, x1, pointY, POINT_W, 50);

  // The record row, below everything.
  const recordY = Math.max(contentBottom, contextBottom, pointY + 70) + 28;
  // In time order, left to right, wrapping before a pill gets too narrow to
  // read its label.
  const records = byKind("record");
  const span = width - x0 * 2;
  const perRow = Math.max(
    1,
    Math.min(records.length, Math.floor((span + GAP) / (RECORD_MIN_W + GAP))),
  );
  const rw =
    records.length > 0 ? Math.min(220, Math.floor((span - GAP * (perRow - 1)) / perRow)) : 0;
  records.forEach((n, i) => {
    const row = Math.floor(i / perRow);
    const col = i % perRow;
    put(n, x0 + col * (rw + GAP), recordY + 22 + row * (RECORD_H + RECORD_GAP), rw, RECORD_H);
  });

  const rows = Math.ceil(records.length / Math.max(1, perRow));
  const height =
    records.length > 0 ? recordY + 22 + rows * (RECORD_H + RECORD_GAP) - RECORD_GAP + 16 : recordY;
  return {
    width,
    height,
    placed,
    columns: [
      { x: x0, label: "Content" },
      { x: x1, label: "Where it sits" },
      { x: x2, label: "What applies there" },
    ],
    recordY,
  };
}
