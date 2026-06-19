import React from "react";
import "./diagram.css";

/*
  CycleDiagram — a closed loop of steps laid around a ring, with directional
  arrows that flow clockwise and close back to the start. The uniform-style way
  to show a self-sustaining process — the format-ops runbook loop, where each run
  reconciles, computes what is due, executes with evidence, records the ledger,
  and feeds the next run.

      <CycleDiagram
        steps={[
          { label: "Reconcile", sub: "ledger vs reality" },
          { label: "Compute due", sub: "signals + watermarks" },
          { label: "Rank & budget" },
          { label: "Execute", sub: "with evidence" },
          { label: "Record", sub: "ledger commit" },
          { label: "Reflect", sub: "learnings" },
        ]}
        caption="Each run feeds the next — watermarks make any deferral safe."
      />

  Pure SVG + CSS — light/dark with no JS, SSR-safe.
*/

export interface CycleStep {
  label: string;
  /** Optional second line under the label. */
  sub?: string;
}

export interface CycleDiagramProps {
  steps: CycleStep[];
  caption?: string;
}

const PAD = 16;
const LABEL_CHAR = 7; // px per char, label
const SUB_CHAR = 6; // px per char, sub

export function CycleDiagram({ steps, caption }: CycleDiagramProps): React.ReactElement {
  const n = steps.length;
  const anySub = steps.some((s) => s.sub);
  const boxH = anySub ? 42 : 32;
  const boxWidth = (s: CycleStep) =>
    Math.max(
      72,
      Math.round(Math.max(s.label.length * LABEL_CHAR, (s.sub ?? "").length * SUB_CHAR)) + 24,
    );
  const maxBoxW = Math.max(...steps.map(boxWidth));

  // Radius so the arcs between boxes stay legible (circumference fits N boxes + gaps).
  const R = Math.max(120, ((maxBoxW + 40) * n) / (2 * Math.PI));
  const center = R + maxBoxW / 2 + PAD;
  const totalW = 2 * center;
  const totalH = 2 * center;

  const step = (2 * Math.PI) / n;
  const angle = (i: number) => -Math.PI / 2 + i * step; // start at top, go clockwise
  const pt = (a: number): [number, number] => [center + R * Math.cos(a), center + R * Math.sin(a)];
  const halfAng = (maxBoxW / 2 + 6) / R; // angular clearance for a box on the ring

  return (
    <div className="kdx">
      <div className="kdx-scroll">
        <div
          className="kdx-canvas"
          style={{ minWidth: Math.min(totalW, 320), maxWidth: Math.min(totalW, 440) }}
        >
          <svg
            viewBox={`0 0 ${totalW} ${totalH}`}
            xmlns="http://www.w3.org/2000/svg"
            role="img"
            aria-label={`Cycle: ${steps.map((s) => s.label).join(" then ")}, then back to start`}
          >
            <defs>
              <marker
                id="kdx-arrow-cyc"
                markerWidth="7"
                markerHeight="7"
                refX="5.5"
                refY="3"
                orient="auto"
              >
                <path d="M0,0 L6,3 L0,6 Z" className="kdx-arrow" />
              </marker>
            </defs>

            {/* clockwise arcs between consecutive steps, closing the loop */}
            {steps.map((s, i) => {
              const [sx, sy] = pt(angle(i) + halfAng);
              const [ex, ey] = pt(angle(i) + step - halfAng);
              return (
                <path
                  key={`arc-${i}`}
                  d={`M${sx},${sy} A ${R} ${R} 0 0 1 ${ex},${ey}`}
                  className="kdx-channel"
                  markerEnd="url(#kdx-arrow-cyc)"
                />
              );
            })}

            {/* step nodes */}
            {steps.map((s, i) => {
              const [px, py] = pt(angle(i));
              const w = boxWidth(s);
              return (
                <g key={`${s.label}-${i}`}>
                  <rect
                    x={px - w / 2}
                    y={py - boxH / 2}
                    width={w}
                    height={boxH}
                    rx={9}
                    className="kdx-box"
                  />
                  <text
                    x={px}
                    y={s.sub ? py - 1 : py + 4}
                    textAnchor="middle"
                    fontSize={11.5}
                    className="kdx-label"
                  >
                    {s.label}
                  </text>
                  {s.sub && (
                    <text
                      x={px}
                      y={py + 11}
                      textAnchor="middle"
                      fontSize={8.5}
                      className="kdx-note"
                    >
                      {s.sub}
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

export default CycleDiagram;
