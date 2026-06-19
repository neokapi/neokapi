import React from "react";
import "./diagram.css";

/*
  AxisLadderDiagram — an ascending staircase of rungs for a single maturity axis.

  Each rung is a graded step: a level chip (e.g. G0…G4), a name, and a short
  gloss. The rungs climb left→right so the ladder reads as deepening capability —
  the uniform-style way to show one axis of the format-maturity rubric (the
  Structure & Geometry depth ladder, or any of L / V / E / K / C / S / G).

      <AxisLadderDiagram
        rungs={[
          { grade: "G0", name: "opaque", gloss: "bytes only" },
          { grade: "G1", name: "metadata", gloss: "title, author, page count" },
          { grade: "G2", name: "linear text", gloss: "reading-order characters" },
          { grade: "G3", name: "roles", gloss: "headings, tables, reading order" },
          { grade: "G4", name: "geometry", gloss: "page coords, bounding boxes" },
        ]}
        caption="Structure & Geometry — how much document structure we recover."
      />

  Pure SVG + CSS — light/dark with no JS, SSR-safe.
*/

export interface AxisRung {
  /** Short level code shown in the chip (e.g. "G0", "L4"). */
  grade: string;
  /** Rung name (e.g. "linear text"). */
  name: string;
  /** Optional one-line gloss under the name. */
  gloss?: string;
}

export interface AxisLadderDiagramProps {
  rungs: AxisRung[];
  caption?: string;
}

const PAD = 14;
const BOX_H = 46;
const RISE = 28; // vertical step up per rung
const H_GAP = 24; // horizontal gap between rungs
const NAME_CHAR = 7; // px per char, name
const GLOSS_CHAR = 6; // px per char, gloss (smaller font)
const GRADE_CHAR = 7.4; // px per char, grade chip
const CHIP_PAD = 10; // inner left/right padding around the grade chip

export function AxisLadderDiagram({ rungs, caption }: AxisLadderDiagramProps): React.ReactElement {
  const n = rungs.length;
  const chipW = Math.max(34, ...rungs.map((r) => Math.round(r.grade.length * GRADE_CHAR) + 14));
  const contentW = Math.max(
    60,
    ...rungs.map((r) =>
      Math.round(Math.max(r.name.length * NAME_CHAR, (r.gloss ?? "").length * GLOSS_CHAR)),
    ),
  );
  const boxW = CHIP_PAD + chipW + 10 + contentW + 12;

  const boxTop = (i: number) => PAD + (n - 1 - i) * RISE; // climb to the right
  const boxX = (i: number) => PAD + i * (boxW + H_GAP);
  const centerY = (i: number) => boxTop(i) + BOX_H / 2;

  const totalW = PAD + n * boxW + (n - 1) * H_GAP + PAD;
  const totalH = PAD + (n - 1) * RISE + BOX_H + PAD;

  return (
    <div className="kdx">
      <div className="kdx-scroll">
        <div className="kdx-canvas" style={{ minWidth: Math.min(totalW, 460), maxWidth: totalW }}>
          <svg
            viewBox={`0 0 ${totalW} ${totalH}`}
            xmlns="http://www.w3.org/2000/svg"
            role="img"
            aria-label={`Axis ladder: ${rungs.map((r) => `${r.grade} ${r.name}`).join(" then ")}`}
          >
            <defs>
              <marker
                id="kdx-arrow-lad"
                markerWidth="7"
                markerHeight="7"
                refX="5.5"
                refY="3"
                orient="auto"
              >
                <path d="M0,0 L6,3 L0,6 Z" className="kdx-arrow" />
              </marker>
            </defs>

            {rungs.map((r, i) => {
              const x = boxX(i);
              const top = boxTop(i);
              const cy = centerY(i);
              const next = rungs[i + 1];
              const cx = x + CHIP_PAD + chipW / 2;
              const contentX = x + CHIP_PAD + chipW + 10;
              return (
                <g key={`${r.grade}-${i}`}>
                  {/* riser to the next (higher) rung */}
                  {next && (
                    <line
                      x1={x + boxW}
                      y1={cy}
                      x2={boxX(i + 1) - 4}
                      y2={centerY(i + 1)}
                      className="kdx-channel"
                      markerEnd="url(#kdx-arrow-lad)"
                    />
                  )}

                  {/* rung box */}
                  <rect x={x} y={top} width={boxW} height={BOX_H} rx={9} className="kdx-box" />

                  {/* grade chip */}
                  <rect
                    x={x + CHIP_PAD}
                    y={top + (BOX_H - 20) / 2}
                    width={chipW}
                    height={20}
                    rx={6}
                    className="kdx-box kdx-box--io"
                  />
                  <text
                    x={cx}
                    y={cy + 3.5}
                    textAnchor="middle"
                    fontSize={11}
                    className="kdx-sub kdx-sub--io"
                  >
                    {r.grade}
                  </text>

                  {/* name + gloss */}
                  <text
                    x={contentX}
                    y={r.gloss ? top + 20 : cy + 4}
                    fontSize={12}
                    className="kdx-label"
                  >
                    {r.name}
                  </text>
                  {r.gloss && (
                    <text x={contentX} y={top + 34} fontSize={9.5} className="kdx-note">
                      {r.gloss}
                    </text>
                  )}
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

export default AxisLadderDiagram;
