import React from "react";
import type { StageRole } from "./PipelineDiagram";
import "./diagram.css";

/*
  LanesDiagram — stacked "lane" cards (a titled box with monospace step lines)
  joined by a labeled handoff arrow. The uniform-style replacement for the
  plugin-bridge two-thread model, where a Reader thread hands events to a Writer
  thread over a queue.

      <LanesDiagram
        handoff="eventQueue"
        lanes={[
          { title: "Reader Thread", sub: "filterPool, bounded", role: "io", steps: [...] },
          { title: "Writer Thread", sub: "writerPool, unbounded", role: "translate", steps: [...] },
        ]}
      />

  Pure SVG + CSS — light/dark with no JS, SSR-safe.
*/

export interface LaneThread {
  title: string;
  sub?: string;
  steps: string[];
  role?: StageRole;
}

export interface LanesDiagramProps {
  lanes: LaneThread[];
  /** Label on the arrow handing off from one lane to the next. */
  handoff?: string;
  caption?: string;
}

const PAD = 14;
const HEADER_H = 26;
const ROW_H = 18;
const CARD_PAD_B = 12;
const V_GAP = 40; // gap (handoff arrow) between cards
const MONO = 5.55; // px per mono char at 9px
const roleBox = (role?: StageRole) => (role && role !== "tool" ? ` kdx-box--${role}` : "");

const cardHeight = (l: LaneThread) => HEADER_H + l.steps.length * ROW_H + CARD_PAD_B;

export function LanesDiagram({ lanes, handoff, caption }: LanesDiagramProps): React.ReactElement {
  const cardW = Math.max(
    260,
    ...lanes.map((l) =>
      Math.max(
        l.title.length * 6.8 + (l.sub ? l.sub.length * MONO + 24 : 0) + 28,
        ...l.steps.map((s) => s.length * MONO + 28),
      ),
    ),
  );
  const x = PAD;
  const cx = x + cardW / 2;

  // place cards top→bottom
  let y = PAD;
  const placed = lanes.map((l) => {
    const h = cardHeight(l);
    const p = { l, y, h };
    y += h + V_GAP;
    return p;
  });
  const totalH = y - V_GAP + PAD;
  const totalW = cardW + 2 * PAD;

  return (
    <div className="kdx">
      <div className="kdx-scroll">
        <div className="kdx-canvas" style={{ minWidth: Math.min(totalW, 460), maxWidth: totalW }}>
          <svg
            viewBox={`0 0 ${totalW} ${totalH}`}
            xmlns="http://www.w3.org/2000/svg"
            role="img"
            aria-label={`Concurrent lanes: ${lanes.map((l) => l.title).join(", ")}`}
          >
            <defs>
              <marker id="kdx-arrow-dn" markerWidth="9" markerHeight="9" refX="3" refY="6" orient="auto">
                <path d="M0,0 L6,0 L3,6 Z" className="kdx-arrow" />
              </marker>
            </defs>

            {placed.map(({ l, y: cardY, h }, i) => {
              const next = placed[i + 1];
              return (
                <g key={`${l.title}-${i}`}>
                  <rect x={x} y={cardY} width={cardW} height={h} rx={10} className={`kdx-box${roleBox(l.role)}`} />
                  <text x={x + 14} y={cardY + 17} fontSize={11.5} className="kdx-label">
                    {l.title}
                  </text>
                  {l.sub && (
                    <text x={x + cardW - 12} y={cardY + 17} textAnchor="end" fontSize={9} className="kdx-chip-sub">
                      {l.sub}
                    </text>
                  )}
                  <line x1={x + 10} y1={cardY + HEADER_H - 4} x2={x + cardW - 10} y2={cardY + HEADER_H - 4} className="kdx-divider" />
                  {l.steps.map((s, si) => (
                    <text
                      key={si}
                      x={x + 14}
                      y={cardY + HEADER_H + si * ROW_H + 13}
                      fontSize={9}
                      className="kdx-mono"
                      xmlSpace="preserve"
                    >
                      {s}
                    </text>
                  ))}

                  {next && (
                    <>
                      <line
                        x1={cx}
                        y1={cardY + h}
                        x2={cx}
                        y2={next.y - 2}
                        className="kdx-channel"
                        markerEnd="url(#kdx-arrow-dn)"
                      />
                      {handoff && (
                        <g>
                          <rect
                            x={cx + 8}
                            y={(cardY + h + next.y) / 2 - 9}
                            width={handoff.length * 5.8 + 14}
                            height={16}
                            rx={7}
                            className="kdx-pill"
                          />
                          <text x={cx + 15} y={(cardY + h + next.y) / 2 + 2.5} fontSize={9} className="kdx-chan">
                            {handoff}
                          </text>
                        </g>
                      )}
                    </>
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

export default LanesDiagram;
