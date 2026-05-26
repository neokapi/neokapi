import React from "react";
import type { StageRole } from "./PipelineDiagram";
import "./diagram.css";

/*
  PhaseFlow — a vertical node flow whose edges carry a command/transform label,
  with an optional self-loop on a node. The uniform-style replacement for the
  kapi-react "extract → translate (loop) → compile → split" diagram, where the
  i18n/ stage loops while you accumulate locales in place.

      <PhaseFlow nodes={[
        { label: "Your source code" },
        { label: "i18n/", sub: "KLF archive", edge: "kapi-react extract",
          loop: ["kapi ai-translate / pseudo / qa", "accumulate locales in place"] },
        { label: "public/translations/{locale}.json", edge: "kapi-react compile" },
      ]} />

  Pure SVG + CSS — light/dark with no JS, SSR-safe.
*/

export interface PhaseNode {
  label: string;
  sub?: string;
  role?: StageRole;
  /** Label on the edge entering this node from the one above. */
  edge?: string;
  /** Self-loop label (1–2 lines) shown beside a looping node. */
  loop?: string | string[];
}

export interface PhaseFlowProps {
  nodes: PhaseNode[];
  caption?: string;
}

const NODE_H = 44;
const V_GAP = 52; // vertical gap (edge + label) between nodes
const TOP = 12;
const NODE_X = 16;
const CHAR = 7;
const LOOP_OUT = 30; // how far the self-loop bulges right of the node

const roleBox = (role?: StageRole) => (role && role !== "tool" ? ` kdx-box--${role}` : "");

export function PhaseFlow({ nodes, caption }: PhaseFlowProps): React.ReactElement {
  const nodeW = Math.max(
    180,
    ...nodes.map((n) => Math.max(n.label.length, (n.sub ?? "").length) * CHAR + 28),
  );
  const cx = NODE_X + nodeW / 2;
  const nodeY = (i: number) => TOP + i * (NODE_H + V_GAP);

  // Right-side room for the widest self-loop label.
  const loopLines = (n: PhaseNode) => (Array.isArray(n.loop) ? n.loop : n.loop ? [n.loop] : []);
  const loopLabelW = Math.max(
    0,
    ...nodes.map((n) => Math.max(0, ...loopLines(n).map((l) => l.length * 6.2))),
  );
  const totalW = NODE_X + nodeW + LOOP_OUT + 14 + loopLabelW + 14;
  const totalH = nodeY(nodes.length - 1) + NODE_H + (caption ? 6 : 14);

  return (
    <div className="kdx">
      <div className="kdx-scroll">
        <div className="kdx-canvas" style={{ minWidth: Math.min(totalW, 420), maxWidth: totalW }}>
          <svg
            viewBox={`0 0 ${totalW} ${totalH}`}
            xmlns="http://www.w3.org/2000/svg"
            role="img"
            aria-label={`Phases: ${nodes.map((n) => n.label).join(" then ")}`}
          >
            <defs>
              <marker id="kdx-arrow-d" markerWidth="8" markerHeight="8" refX="3" refY="5.5" orient="auto">
                <path d="M0,0 L6,0 L3,6 Z" className="kdx-arrow" />
              </marker>
              <marker id="kdx-arrow-v" markerWidth="8" markerHeight="8" refX="3" refY="5.5" orient="auto">
                <path d="M0,0 L6,0 L3,6 Z" className="kdx-arrow" />
              </marker>
            </defs>

            {nodes.map((n, i) => {
              const y = nodeY(i);
              const top = y;
              const prevBottom = nodeY(i - 1) + NODE_H;
              const ll = loopLines(n);
              return (
                <g key={`${n.label}-${i}`}>
                  {/* edge from the node above */}
                  {i > 0 && (
                    <>
                      <line x1={cx} y1={prevBottom} x2={cx} y2={top - 2} className="kdx-channel" markerEnd="url(#kdx-arrow-d)" />
                      {n.edge && <EdgeLabel x={cx + 10} y={(prevBottom + top) / 2} text={n.edge} />}
                    </>
                  )}

                  {/* the node */}
                  <rect x={NODE_X} y={top} width={nodeW} height={NODE_H} rx={9} className={`kdx-box${roleBox(n.role)}`} />
                  <text x={cx} y={n.sub ? top + 20 : top + 27} textAnchor="middle" fontSize={12} className="kdx-label">
                    {n.label}
                  </text>
                  {n.sub && (
                    <text x={cx} y={top + 34} textAnchor="middle" fontSize={9} className="kdx-sub">
                      {n.sub}
                    </text>
                  )}

                  {/* optional self-loop on the right */}
                  {ll.length > 0 && (
                    <>
                      <path
                        d={`M${NODE_X + nodeW},${top + 9} h${LOOP_OUT} v${NODE_H - 18} h${-LOOP_OUT}`}
                        className="kdx-loop"
                        markerEnd="url(#kdx-arrow-back)"
                      />
                      {ll.map((line, li) => (
                        <text
                          key={li}
                          x={NODE_X + nodeW + LOOP_OUT + 10}
                          y={top + NODE_H / 2 - (ll.length - 1) * 6 + li * 12 + 3}
                          fontSize={9}
                          className="kdx-note"
                        >
                          {line}
                        </text>
                      ))}
                    </>
                  )}
                </g>
              );
            })}

            <defs>
              <marker id="kdx-arrow-back" markerWidth="8" markerHeight="8" refX="2" refY="3" orient="auto">
                <path d="M6,0 L0,3 L6,6 Z" className="kdx-arrow" />
              </marker>
            </defs>
          </svg>
        </div>
      </div>
      {caption && <p className="kdx-caption">{caption}</p>}
    </div>
  );
}

/** A short label on an edge, with a surface pill so it stays legible over a line. */
function EdgeLabel({ x, y, text }: { x: number; y: number; text: string }): React.ReactElement {
  const w = text.length * 5.8 + 12;
  return (
    <g>
      <rect x={x} y={y - 9} width={w} height={16} rx={7} className="kdx-pill" />
      <text x={x + 7} y={y + 2.5} fontSize={9} className="kdx-chan">
        {text}
      </text>
    </g>
  );
}

export default PhaseFlow;
