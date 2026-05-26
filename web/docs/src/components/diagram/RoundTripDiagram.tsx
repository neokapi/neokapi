import React from "react";
import type { PipelineStage } from "./PipelineDiagram";
import "./diagram.css";

/*
  RoundTripDiagram — a two-row round-trip with a shared hub, the uniform-style
  replacement for the bilingual extract/merge ASCII diagram:

      authored source ─► extract ─► XLIFF/PO ─► translator        (forward, →)
                            │
                       project TM   (hub: pre-fill on extract, absorb on merge)
                            ▲
      kapi merge  ◄─ translated XLIFF/PO ◄─ returned by translator (return, ←)

  `forward` renders left→right with right-pointing arrows; `back` renders
  left→right with left-pointing arrows (the return flow). The hub sits between
  the rows and links to one stage in each (by index), with labeled dashed edges.

  Pure SVG + CSS — light/dark with no JS, SSR-safe.
*/

const GAP = 44;
const CHAR = 7.2;
const ROW_H = 50;
const FWD_Y = 54;
const BACK_Y = 214;
const HUB_Y = 126;
const HUB_W = 150;
const HUB_H = 54;
const PAD = 36;

const stageWidth = (s: PipelineStage): number =>
  Math.max(84, Math.round(Math.max(s.label.length, (s.sub ?? "").length) * CHAR) + 26);

interface Placed extends PipelineStage {
  x: number;
  w: number;
}

const layoutRow = (stages: PipelineStage[], startX: number): { placed: Placed[]; end: number } => {
  let x = startX;
  const placed = stages.map((s) => {
    const w = stageWidth(s);
    const p: Placed = { ...s, x, w };
    x += w + GAP;
    return p;
  });
  return { placed, end: x - GAP };
};

const roleBox = (role?: PipelineStage["role"]) => (role && role !== "tool" ? ` kdx-box--${role}` : "");
const roleSub = (role?: PipelineStage["role"]) => (role && role !== "tool" ? ` kdx-sub--${role}` : "");

export interface RoundTripDiagramProps {
  /** Top row, left→right (right-pointing arrows). */
  forward: PipelineStage[];
  /** Bottom row, displayed left→right with left-pointing (return) arrows. */
  back: PipelineStage[];
  hub: { label: string; sub?: string };
  /** Index in `forward` whose stage links down to the hub. Default 1. */
  forwardIndex?: number;
  /** Index in `back` whose stage links up from the hub. Default 0. */
  backIndex?: number;
  /** Label on the forward→hub edge (e.g. "pre-fill"). */
  forwardLabel?: string;
  /** Label on the hub→back edge (e.g. "absorb"). */
  backLabel?: string;
  caption?: string;
  animated?: boolean;
}

/** A short label on a connector, with a surface pill so it stays legible. */
function EdgeLabel({ x, y, text }: { x: number; y: number; text: string }): React.ReactElement {
  const w = text.length * 5.6 + 12;
  return (
    <g>
      <rect x={x - w / 2} y={y - 9} width={w} height={15} rx={7} className="kdx-pill" />
      <text x={x} y={y + 2} textAnchor="middle" fontSize={8.5} className="kdx-chan">
        {text}
      </text>
    </g>
  );
}

function Row({ placed, arrow }: { placed: Placed[]; arrow: "right" | "left" }): React.ReactElement {
  const y = placed.length ? (arrow === "right" ? FWD_Y : BACK_Y) : 0;
  const cy = y + ROW_H / 2;
  return (
    <>
      {placed.map((s, i) => {
        const cx = s.x + s.w / 2;
        const next = placed[i + 1];
        return (
          <g key={`${s.label}-${i}`}>
            <rect x={s.x} y={y} width={s.w} height={ROW_H} rx={9} className={`kdx-box${roleBox(s.role)}`} />
            <text x={cx} y={s.sub ? cy - 1 : cy + 4} textAnchor="middle" fontSize={12} className="kdx-label">
              {s.label}
            </text>
            {s.sub && (
              <text x={cx} y={cy + 13} textAnchor="middle" fontSize={9} className={`kdx-sub${roleSub(s.role)}`}>
                {s.sub}
              </text>
            )}
            {next &&
              (arrow === "right" ? (
                <line
                  x1={s.x + s.w}
                  y1={cy}
                  x2={next.x - 4}
                  y2={cy}
                  className="kdx-channel"
                  markerEnd="url(#kdx-arr-r)"
                />
              ) : (
                <line
                  x1={s.x + s.w - 4}
                  y1={cy}
                  x2={next.x}
                  y2={cy}
                  className="kdx-channel"
                  markerStart="url(#kdx-arr-l)"
                />
              ))}
          </g>
        );
      })}
    </>
  );
}

export function RoundTripDiagram({
  forward,
  back,
  hub,
  forwardIndex = 1,
  backIndex = 0,
  forwardLabel,
  backLabel,
  caption,
  animated = false,
}: RoundTripDiagramProps): React.ReactElement {
  const fwd = layoutRow(forward, PAD);
  const bck = layoutRow(back, PAD);
  const totalW = Math.max(fwd.end, bck.end) + PAD;
  const totalH = BACK_Y + ROW_H + 26;
  const hubX = totalW / 2 - HUB_W / 2;
  const hubCx = totalW / 2;

  const fStage = fwd.placed[Math.min(forwardIndex, fwd.placed.length - 1)];
  const bStage = bck.placed[Math.min(backIndex, bck.placed.length - 1)];
  const fCx = fStage ? fStage.x + fStage.w / 2 : hubCx;
  const bCx = bStage ? bStage.x + bStage.w / 2 : hubCx;

  return (
    <div className={`kdx${animated ? " kdx--animated" : ""}`}>
      <div className="kdx-scroll">
        <div className="kdx-canvas" style={{ minWidth: Math.min(totalW, 520), maxWidth: totalW }}>
          <svg
            viewBox={`0 0 ${totalW} ${totalH}`}
            xmlns="http://www.w3.org/2000/svg"
            role="img"
            aria-label="Round-trip: extract forward, merge back, sharing a translation memory"
          >
            <defs>
              <marker id="kdx-arr-r" markerWidth="7" markerHeight="7" refX="5.5" refY="3" orient="auto">
                <path d="M0,0 L6,3 L0,6 Z" className="kdx-arrow" />
              </marker>
              <marker id="kdx-arr-l" markerWidth="7" markerHeight="7" refX="0.5" refY="3" orient="auto">
                <path d="M6,0 L0,3 L6,6 Z" className="kdx-arrow" />
              </marker>
            </defs>

            {/* hub links */}
            <path d={`M${fCx},${FWD_Y + ROW_H} L${hubCx},${HUB_Y}`} className="kdx-link kdx-link--resource" />
            {forwardLabel && (
              <EdgeLabel x={(fCx + hubCx) / 2} y={(FWD_Y + ROW_H + HUB_Y) / 2} text={forwardLabel} />
            )}
            <path d={`M${hubCx},${HUB_Y + HUB_H} L${bCx},${BACK_Y}`} className="kdx-link kdx-link--resource" />
            {backLabel && (
              <EdgeLabel x={(bCx + hubCx) / 2} y={(HUB_Y + HUB_H + BACK_Y) / 2} text={backLabel} />
            )}

            <Row placed={fwd.placed} arrow="right" />
            <Row placed={bck.placed} arrow="left" />

            {/* hub */}
            <rect x={hubX} y={HUB_Y} width={HUB_W} height={HUB_H} rx={10} className="kdx-hub" />
            <text x={hubCx} y={hub.sub ? HUB_Y + 24 : HUB_Y + 31} textAnchor="middle" fontSize={12.5} className="kdx-label">
              {hub.label}
            </text>
            {hub.sub && (
              <text x={hubCx} y={HUB_Y + 40} textAnchor="middle" fontSize={9} className="kdx-sub" style={{ fill: "var(--kdx-resource)" }}>
                {hub.sub}
              </text>
            )}
          </svg>
        </div>
      </div>
      {caption && <p className="kdx-caption">{caption}</p>}
    </div>
  );
}

export default RoundTripDiagram;
