import React from "react";
import type { StageRole } from "./PipelineDiagram";
import "./diagram.css";

/*
  TypeDiagram — record types as titled cards listing their fields, joined by
  labelled edges. The uniform-style way to show a small data model: which type
  holds which field, and what the link between two of them means.

      <TypeDiagram
        boxes={[
          { name: "Layer", col: 0, role: "io", self: "nests — embedded content",
            fields: [{ name: "Format", type: "string" }] },
          { name: "Block", col: 1, role: "translate",
            fields: [{ name: "Source", type: "[]Run" }] },
          { name: "Run", col: 2 },
        ]}
        edges={[
          { from: 0, to: 1, label: "contains" },
          { from: 1, to: 2, label: "flat sequence" },
        ]}
      />

  Boxes sharing a `col` stack top-to-bottom, and each column is centred against
  the tallest one, so a parent fans out evenly to its children. An edge runs left
  to right, from a box in a lower column to one in a higher column; `self` draws
  a labelled arc above a box that holds its own kind.

  Pure SVG + CSS — light/dark with no JS, SSR-safe.
*/

export interface TypeField {
  name: string;
  /** Rendered mono and muted beside the field name (e.g. "[]Run"). */
  type: string;
}

export interface TypeBox {
  name: string;
  /** Mono sub-line under the type name. */
  sub?: string;
  role?: StageRole;
  fields?: TypeField[];
  /** Column index; boxes sharing a column stack top-to-bottom. */
  col: number;
  /** Label on a self-edge, for a type that holds its own kind. */
  self?: string;
}

export interface TypeEdge {
  /** Index into `boxes`; must sit in a lower column than `to`. */
  from: number;
  to: number;
  label?: string;
}

export interface TypeDiagramProps {
  boxes: TypeBox[];
  edges?: TypeEdge[];
  caption?: string;
}

const PAD = 14;
const COL_GAP = 86; // room between columns for the edge labels
const V_GAP = 18; // gap between stacked cards in one column
const HEADER_H = 28;
const SUB_H = 12;
const ROW_H = 18;
const CARD_PAD_B = 10;
const SELF_H = 26; // headroom above a card carrying a self-edge
const NAME_CHAR = 7.6;
const MONO = 6.6;

const roleBox = (role?: StageRole) => (role && role !== "tool" ? ` kdx-box--${role}` : "");
const roleSub = (role?: StageRole) => (role && role !== "tool" ? ` kdx-sub--${role}` : "");

const headerHeight = (b: TypeBox) => HEADER_H + (b.sub ? SUB_H : 0);
const cardHeight = (b: TypeBox) => headerHeight(b) + (b.fields?.length ?? 0) * ROW_H + CARD_PAD_B;
const fieldWidth = (f: TypeField) => Math.round((f.name.length + f.type.length) * MONO) + 44;
const cardWidth = (b: TypeBox) =>
  Math.max(
    130,
    Math.round(b.name.length * NAME_CHAR) + 28,
    Math.round((b.sub ?? "").length * MONO) + 28,
    ...(b.fields ?? []).map(fieldWidth),
  );

export function TypeDiagram({ boxes, edges = [], caption }: TypeDiagramProps): React.ReactElement {
  const colCount = Math.max(...boxes.map((b) => b.col)) + 1;
  const columns = Array.from({ length: colCount }, (_, c) =>
    boxes.map((b, i) => ({ b, i })).filter((entry) => entry.b.col === c),
  );
  const colW = columns.map((col) => Math.max(0, ...col.map(({ b }) => cardWidth(b))));

  const colX: number[] = [];
  let x = PAD;
  for (let c = 0; c < colCount; c++) {
    colX.push(x);
    x += colW[c] + COL_GAP;
  }
  const totalW = x - COL_GAP + PAD;

  const colH = columns.map(
    (col) => col.reduce((h, { b }) => h + cardHeight(b), 0) + Math.max(0, col.length - 1) * V_GAP,
  );
  const bandH = Math.max(...colH);
  const top = PAD + (boxes.some((b) => b.self) ? SELF_H : 0);
  const totalH = top + bandH + PAD;

  // Place each card: columns left→right, each stack centred against the band.
  const placed: { x: number; y: number; w: number; h: number }[] = [];
  columns.forEach((col, c) => {
    let y = top + (bandH - colH[c]) / 2;
    col.forEach(({ b, i }) => {
      const h = cardHeight(b);
      placed[i] = { x: colX[c], y, w: colW[c], h };
      y += h + V_GAP;
    });
  });

  // Fan several edges out of one box at staggered x, so their vertical runs
  // don't sit on top of each other.
  const fanned = new Map<number, number>();

  return (
    <div className="kdx">
      <div className="kdx-scroll">
        <div className="kdx-canvas" style={{ minWidth: Math.min(totalW, 520), maxWidth: totalW }}>
          <svg
            viewBox={`0 0 ${totalW} ${totalH}`}
            xmlns="http://www.w3.org/2000/svg"
            role="img"
            aria-label={`Types: ${boxes.map((b) => b.name).join(", ")}`}
          >
            <defs>
              <marker
                id="kdx-arrow-type-r"
                markerWidth="8"
                markerHeight="8"
                refX="5"
                refY="3"
                orient="auto"
              >
                <path d="M0,0 L6,3 L0,6 Z" className="kdx-arrow" />
              </marker>
              <marker
                id="kdx-arrow-type-d"
                markerWidth="8"
                markerHeight="8"
                refX="3"
                refY="5.5"
                orient="auto"
              >
                <path d="M0,0 L6,0 L3,6 Z" className="kdx-arrow" />
              </marker>
            </defs>

            {edges.map((e, i) => {
              const a = placed[Math.min(e.from, placed.length - 1)];
              const b = placed[Math.min(e.to, placed.length - 1)];
              const y1 = a.y + a.h / 2;
              const y2 = b.y + b.h / 2;
              const x1 = a.x + a.w;
              const x2 = b.x;
              const n = fanned.get(e.from) ?? 0;
              fanned.set(e.from, n + 1);
              const mid = x1 + (x2 - x1) * (0.34 + 0.16 * (n % 3));
              const labelW = (e.label ?? "").length * 5.8 + 12;
              return (
                <g key={`edge-${i}`}>
                  <path
                    d={`M${x1},${y1} H${mid} V${y2} H${x2 - 3}`}
                    className="kdx-channel"
                    markerEnd="url(#kdx-arrow-type-r)"
                  />
                  {e.label && (
                    <>
                      <rect
                        x={x2 - 8 - labelW}
                        y={y2 - 18}
                        width={labelW}
                        height={16}
                        rx={7}
                        className="kdx-pill"
                      />
                      <text
                        x={x2 - 15}
                        y={y2 - 7}
                        textAnchor="end"
                        fontSize={9}
                        className="kdx-chan"
                      >
                        {e.label}
                      </text>
                    </>
                  )}
                </g>
              );
            })}

            {boxes.map((b, i) => {
              const { x: bx, y, w, h } = placed[i];
              const fields = b.fields ?? [];
              const divY = y + headerHeight(b) - 3;
              return (
                <g key={`${b.name}-${i}`}>
                  {b.self && (
                    <>
                      <path
                        d={`M${bx + w * 0.3},${y} V${y - 14} H${bx + w * 0.7} V${y - 3}`}
                        className="kdx-loop"
                        markerEnd="url(#kdx-arrow-type-d)"
                      />
                      <text
                        x={bx + w / 2}
                        y={y - 19}
                        textAnchor="middle"
                        fontSize={9}
                        className="kdx-note"
                      >
                        {b.self}
                      </text>
                    </>
                  )}

                  <rect
                    x={bx}
                    y={y}
                    width={w}
                    height={h}
                    rx={10}
                    className={`kdx-box${roleBox(b.role)}`}
                  />
                  <text x={bx + 12} y={y + 19} fontSize={13} className="kdx-label">
                    {b.name}
                  </text>
                  {b.sub && (
                    <text
                      x={bx + 12}
                      y={y + 30}
                      fontSize={9.5}
                      className={`kdx-sub${roleSub(b.role)}`}
                    >
                      {b.sub}
                    </text>
                  )}
                  {fields.length > 0 && (
                    <line
                      x1={bx + 10}
                      y1={divY}
                      x2={bx + w - 10}
                      y2={divY}
                      className="kdx-divider"
                    />
                  )}
                  {fields.map((f, j) => {
                    const fy = y + headerHeight(b) + j * ROW_H + ROW_H / 2 + 3;
                    return (
                      <g key={`${f.name}-${j}`}>
                        <text x={bx + 14} y={fy} fontSize={10.5} className="kdx-mono">
                          {f.name}
                        </text>
                        <text
                          x={bx + w - 14}
                          y={fy}
                          textAnchor="end"
                          fontSize={10.5}
                          className="kdx-sub"
                        >
                          {f.type}
                        </text>
                      </g>
                    );
                  })}
                </g>
              );
            })}
          </svg>
        </div>
      </div>
      {caption && <p className="kdx-caption">{caption}</p>}
    </div>
  );
}

export default TypeDiagram;
