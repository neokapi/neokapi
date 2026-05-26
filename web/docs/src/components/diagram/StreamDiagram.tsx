import React from "react";
import "./diagram.css";

/*
  StreamDiagram — a vertical sequence of emitted items down a single spine, the
  uniform-style replacement for the "Part stream" ASCII listings (a reader
  emitting PartLayerStart / PartBlock / PartLayerEnd …). The spine is stream
  order top-to-bottom; `depth` indents nested items (an embedded child layer and
  its contents), and `role` colors the node.

      <StreamDiagram
        title="Read(ctx)"
        items={[
          { kind: "PartLayerStart", detail: 'format = "json"', role: "layer" },
          { kind: "PartBlock", detail: '"title"', depth: 1, role: "block" },
          …
        ]}
      />

  Pure SVG + CSS: themes for light/dark with no JS, SSR-safe. Optional animated
  Part travelling down the spine; suppressed under prefers-reduced-motion.
*/

export type StreamRole = "layer" | "block" | "end" | "meta";

export interface StreamItem {
  /** The Part/event type, shown in the chip (e.g. "PartBlock"). */
  kind: string;
  /** Parenthetical detail (e.g. 'format = "json"', '"title"'). */
  detail?: string;
  /** Nesting depth — indents the chip to show embedded layers. */
  depth?: number;
  /** Colors the node + chip border. */
  role?: StreamRole;
  /** Muted trailing note (e.g. "embedded child layer"). */
  note?: string;
}

export interface StreamDiagramProps {
  /** Small label above the spine (the call that produces the stream). */
  title?: string;
  items: StreamItem[];
  caption?: string;
  /** Send a Part travelling down the spine. Default false. */
  animated?: boolean;
}

const ROW_H = 30;
const SPINE_X = 24;
const CHIP_X0 = 50;
const INDENT = 26;
const TOP = 18; // first row baseline offset for the spine start

const nodeClass = (role?: StreamRole): string => {
  if (role === "layer") return "kdx-node kdx-node--io";
  if (role === "block") return "kdx-node kdx-node--translate";
  return "kdx-node"; // end / meta — muted
};
const boxClass = (role?: StreamRole): string => {
  if (role === "layer") return "kdx-box kdx-box--io";
  if (role === "block") return "kdx-box kdx-box--translate";
  return "kdx-box";
};

export function StreamDiagram({
  title,
  items,
  caption,
  animated = false,
}: StreamDiagramProps): React.ReactElement {
  const titleH = title ? 24 : 4;
  const rowY = (i: number) => titleH + TOP + i * ROW_H;
  const firstY = rowY(0);
  const lastY = rowY(items.length - 1);
  const totalH = lastY + 18;
  const totalW = 560;

  return (
    <div className={`kdx${animated ? " kdx--animated" : ""}`}>
      <div className="kdx-scroll">
        <div className="kdx-canvas" style={{ minWidth: 360, maxWidth: 600 }}>
          <svg
            viewBox={`0 0 ${totalW} ${totalH}`}
            xmlns="http://www.w3.org/2000/svg"
            role="img"
            aria-label={`Stream: ${items.map((it) => it.kind).join(", ")}`}
          >
            {title && (
              <text x={SPINE_X - 6} y={16} fontSize={11} className="kdx-mono">
                {title}
              </text>
            )}

            {/* spine */}
            <line x1={SPINE_X} y1={firstY - 8} x2={SPINE_X} y2={lastY} className="kdx-channel" />
            {animated && (
              <circle r={3} className="kdx-dot kdx-dot--io">
                <animateMotion
                  dur="3.4s"
                  repeatCount="indefinite"
                  path={`M${SPINE_X},${firstY - 8} L${SPINE_X},${lastY}`}
                />
                <animate
                  attributeName="opacity"
                  values="0;1;1;0"
                  keyTimes="0;0.06;0.92;1"
                  dur="3.4s"
                  repeatCount="indefinite"
                />
              </circle>
            )}

            {items.map((it, i) => {
              const y = rowY(i);
              const cy = y - 4;
              const chipX = CHIP_X0 + (it.depth ?? 0) * INDENT;
              const chipW = Math.max(96, it.kind.length * 7 + 18);
              return (
                <g key={`${it.kind}-${i}`}>
                  {/* connector from spine to chip (elbow shows nesting) */}
                  <path
                    d={`M${SPINE_X},${cy} L${chipX},${cy}`}
                    className="kdx-channel"
                    opacity={0.5}
                  />
                  <circle cx={SPINE_X} cy={cy} r={3.2} className={nodeClass(it.role)} />
                  <rect
                    x={chipX}
                    y={y - 15}
                    width={chipW}
                    height={22}
                    rx={6}
                    className={boxClass(it.role)}
                  />
                  <text x={chipX + chipW / 2} y={y} textAnchor="middle" fontSize={10.5} className="kdx-label">
                    {it.kind}
                  </text>
                  {it.detail && (
                    <text x={chipX + chipW + 10} y={y} fontSize={10} className="kdx-mono">
                      {it.detail}
                    </text>
                  )}
                  {it.note && (
                    <text x={totalW - 12} y={y} textAnchor="end" fontSize={9} className="kdx-note">
                      ← {it.note}
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

export default StreamDiagram;
