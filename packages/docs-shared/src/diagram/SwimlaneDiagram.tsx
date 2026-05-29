import React from "react";
import type { StageRole } from "./PipelineDiagram";
import "./diagram.css";

/*
  SwimlaneDiagram — a sequence/message-passing diagram across a few actor
  columns, the uniform-style replacement for the ASCII swimlanes that thread a
  developer, the Bowrain server, and translators with `kapi push` / `kapi pull`
  messages between them:

      Developer                    Bowrain Server                 Translator
          |                              |                             |
          |  kapi push ───────────────► |                             |
          |                             | translate / QA              |
          |  kapi pull ◄─────────────── |                             |
          |                             | ◄──────────── review, approve|

  Each actor is a titled header over a vertical lifeline; each message is an
  arrow from one lifeline to another (or a self-note on a single lifeline),
  flowing top-to-bottom in declaration order. Self-messages (`from === to`)
  render as a muted note beside that lifeline.

  Pure SVG + CSS — light/dark with no JS, SSR-safe.
*/

export interface SwimlaneActor {
  label: string;
  /** Mono sub-line under the actor header (e.g. "web / desktop"). */
  sub?: string;
  role?: StageRole;
}

export interface SwimlaneMessage {
  /** Index of the source actor. */
  from: number;
  /** Index of the target actor. `from === to` renders a self-note. */
  to: number;
  /** Message text shown on (or beside) the arrow. */
  label: string;
  /** Optional second line of message text. */
  detail?: string;
}

export interface SwimlaneDiagramProps {
  actors: SwimlaneActor[];
  messages: SwimlaneMessage[];
  caption?: string;
}

const PAD = 16;
const HEADER_H = 40; // actor header band
const TOP = HEADER_H + 16; // first message baseline
const ROW_H = 38; // vertical step per message
const CHAR = 7.2;
const MIN_COL = 150;

const roleBox = (role?: StageRole) => (role && role !== "tool" ? ` kdx-box--${role}` : "");
const roleSub = (role?: StageRole) => (role && role !== "tool" ? ` kdx-sub--${role}` : "");

const actorWidth = (a: SwimlaneActor): number =>
  Math.max(MIN_COL, Math.round(Math.max(a.label.length, (a.sub ?? "").length) * CHAR) + 28);

interface PlacedActor extends SwimlaneActor {
  /** Lifeline x (column center). */
  cx: number;
}

export function SwimlaneDiagram({
  actors,
  messages,
  caption,
}: SwimlaneDiagramProps): React.ReactElement {
  // Lay actors out left-to-right; each owns a column of equal-ish width.
  let x = PAD;
  const placed: PlacedActor[] = actors.map((a) => {
    const w = actorWidth(a);
    const cx = x + w / 2;
    x += w;
    return { ...a, cx };
  });
  const totalW = x + PAD;

  const msgY = (i: number) => TOP + i * ROW_H;
  const lastY = messages.length ? msgY(messages.length - 1) : TOP;
  const lifelineBottom = lastY + 18;
  const totalH = lifelineBottom + (caption ? 6 : 14);

  return (
    <div className="kdx">
      <div className="kdx-scroll">
        <div className="kdx-canvas" style={{ minWidth: Math.min(totalW, 520), maxWidth: totalW }}>
          <svg
            viewBox={`0 0 ${totalW} ${totalH}`}
            xmlns="http://www.w3.org/2000/svg"
            role="img"
            aria-label={`Sequence across ${actors.map((a) => a.label).join(", ")}`}
          >
            <defs>
              <marker
                id="kdx-sw-r"
                markerWidth="7"
                markerHeight="7"
                refX="5.5"
                refY="3"
                orient="auto"
              >
                <path d="M0,0 L6,3 L0,6 Z" className="kdx-arrow" />
              </marker>
              <marker
                id="kdx-sw-l"
                markerWidth="7"
                markerHeight="7"
                refX="0.5"
                refY="3"
                orient="auto"
              >
                <path d="M6,0 L0,3 L6,6 Z" className="kdx-arrow" />
              </marker>
            </defs>

            {/* actor headers + lifelines */}
            {placed.map((a, i) => {
              const w = actorWidth(a);
              const hx = a.cx - w / 2 + 6;
              const hw = w - 12;
              return (
                <g key={`actor-${i}`}>
                  <rect
                    x={hx}
                    y={PAD}
                    width={hw}
                    height={HEADER_H}
                    rx={9}
                    className={`kdx-box${roleBox(a.role)}`}
                  />
                  <text
                    x={a.cx}
                    y={a.sub ? PAD + 18 : PAD + 25}
                    textAnchor="middle"
                    fontSize={12}
                    className="kdx-label"
                  >
                    {a.label}
                  </text>
                  {a.sub && (
                    <text
                      x={a.cx}
                      y={PAD + 31}
                      textAnchor="middle"
                      fontSize={9}
                      className={`kdx-sub${roleSub(a.role)}`}
                    >
                      {a.sub}
                    </text>
                  )}
                  <line
                    x1={a.cx}
                    y1={PAD + HEADER_H}
                    x2={a.cx}
                    y2={lifelineBottom}
                    className="kdx-channel"
                    opacity={0.55}
                  />
                </g>
              );
            })}

            {/* messages */}
            {messages.map((m, i) => {
              const y = msgY(i);
              const a = placed[Math.min(m.from, placed.length - 1)];
              const b = placed[Math.min(m.to, placed.length - 1)];

              // self-message → muted note beside the lifeline
              if (m.from === m.to) {
                return (
                  <g key={`msg-${i}`}>
                    <circle cx={a.cx} cy={y} r={2.6} className="kdx-node kdx-node--io" />
                    <text x={a.cx + 12} y={y - 3} fontSize={9.5} className="kdx-note">
                      {m.label}
                    </text>
                    {m.detail && (
                      <text x={a.cx + 12} y={y + 9} fontSize={9} className="kdx-chan">
                        {m.detail}
                      </text>
                    )}
                  </g>
                );
              }

              const right = b.cx > a.cx;
              const x1 = right ? a.cx + 3 : a.cx - 3;
              const x2 = right ? b.cx - 3 : b.cx + 3;
              const midX = (a.cx + b.cx) / 2;
              return (
                <g key={`msg-${i}`}>
                  <line
                    x1={x1}
                    y1={y}
                    x2={x2}
                    y2={y}
                    className="kdx-channel"
                    markerEnd={right ? "url(#kdx-sw-r)" : "url(#kdx-sw-l)"}
                  />
                  <text x={midX} y={y - 5} textAnchor="middle" fontSize={9.5} className="kdx-chan">
                    {m.label}
                  </text>
                  {m.detail && (
                    <text x={midX} y={y + 11} textAnchor="middle" fontSize={9} className="kdx-note">
                      {m.detail}
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

export default SwimlaneDiagram;
