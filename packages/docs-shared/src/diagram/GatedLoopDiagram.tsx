import React from "react";
import "./diagram.css";

/*
  GatedLoopDiagram — the source-first convergence loop with visual ship-gates.

  The loop has one shape: settle the SOURCE, hold at a green ship-gate until the
  source is ready, then translate the approved source per locale, hold at a
  second ship-gate until each locale clears its bar — converged. A gate that is
  not met does not ship: it HOLDS the work and routes it to a person (a source
  review, or the target review queue), drawn as an amber dashed branch.

      <GatedLoopDiagram
        phases={[
          {
            kind: "phase", role: "source", label: "settle source",
            steps: ["term-check + protect", "voice-check", "source checks"],
          },
          {
            kind: "gate", label: "source ship-gate", sub: "source_gate",
            hold: "hold — settle your source first",
          },
          {
            kind: "phase", role: "translate", label: "translate approved source",
            steps: ["recycle (content memory-first)", "AI remainder", "target checks"],
          },
          {
            kind: "gate", label: "target ship-gate", sub: "ship_gate",
            hold: "park — needs review",
          },
          { kind: "done", label: "converged" },
        ]}
      />

  Pure SVG + CSS — light/dark with no JS, SSR-safe.
*/

export type LoopRole = "source" | "translate";

export interface LoopPhase {
  kind: "phase";
  label: string;
  role?: LoopRole;
  /** Mono step lines listed inside the phase band. */
  steps?: string[];
}

export interface LoopGate {
  kind: "gate";
  label: string;
  /** Mono sub-line (the recipe key that decides the bar, e.g. "source_gate"). */
  sub?: string;
  /** Label on the amber "held" branch when the gate is not met. */
  hold?: string;
}

export interface LoopDone {
  kind: "done";
  label: string;
  sub?: string;
}

export type LoopNode = LoopPhase | LoopGate | LoopDone;

export interface GatedLoopDiagramProps {
  nodes: LoopNode[];
  caption?: string;
}

const PAD = 16;
const TOP = 20; // room above the spine for the "not ready" branch labels
const SPINE_H = 68; // height of a phase / gate box
const GAP = 40; // horizontal channel length between nodes
const CHAR = 6.6; // px per char, phase title
const STEP_CHAR = 6.2; // px per mono step char
const HOLD_DROP = 46; // how far below the spine the held branch sits

const titleW = (s: string) => s.length * CHAR;
const stepW = (s: string) => s.length * STEP_CHAR;

const nodeWidth = (n: LoopNode): number => {
  if (n.kind === "phase") {
    const longest = Math.max(titleW(n.label), ...(n.steps ?? []).map(stepW));
    return Math.max(150, longest + 28);
  }
  if (n.kind === "gate") {
    return Math.max(120, titleW(n.label) + 40, stepW(n.sub ?? "") + 40);
  }
  return Math.max(96, titleW(n.label) + 30);
};

interface Placed {
  n: LoopNode;
  x: number;
  w: number;
}

export function GatedLoopDiagram({ nodes, caption }: GatedLoopDiagramProps): React.ReactElement {
  let x = PAD;
  const placed: Placed[] = nodes.map((n) => {
    const w = nodeWidth(n);
    const p: Placed = { n, x, w };
    x += w + GAP;
    return p;
  });
  const spineW = x - GAP + PAD;

  const anyHold = nodes.some((n) => n.kind === "gate" && n.hold);
  const holdLabelW = Math.max(
    0,
    ...nodes.map((n) => (n.kind === "gate" && n.hold ? n.hold.length * 5.6 + 20 : 0)),
  );
  const totalW = Math.max(spineW, holdLabelW + 2 * PAD);
  const spineTop = TOP;
  const cy = spineTop + SPINE_H / 2;
  const totalH = cy + SPINE_H / 2 + (anyHold ? HOLD_DROP + 26 : 0) + (caption ? 4 : 14);

  return (
    <div className="kdx">
      <div className="kdx-scroll">
        <div className="kdx-canvas" style={{ minWidth: Math.min(totalW, 520), maxWidth: totalW }}>
          <svg
            viewBox={`0 0 ${totalW} ${totalH}`}
            xmlns="http://www.w3.org/2000/svg"
            role="img"
            aria-label={`Source-first loop: ${nodes.map((n) => n.label).join(" then ")}`}
          >
            <defs>
              <marker
                id="kdx-gl-arrow"
                markerWidth="7"
                markerHeight="7"
                refX="5.5"
                refY="3"
                orient="auto"
              >
                <path d="M0,0 L6,3 L0,6 Z" className="kdx-arrow" />
              </marker>
              <marker
                id="kdx-gl-hold"
                markerWidth="8"
                markerHeight="8"
                refX="5"
                refY="3"
                orient="auto"
              >
                <path d="M0,0 L6,3 L0,6 Z" className="kdx-hold-t" />
              </marker>
            </defs>

            {placed.map((p, i) => {
              const next = placed[i + 1];
              const channel = next && (
                <line
                  x1={p.x + p.w}
                  y1={cy}
                  x2={next.x - 4}
                  y2={cy}
                  className="kdx-channel"
                  markerEnd="url(#kdx-gl-arrow)"
                />
              );
              return (
                <g key={`${p.n.label}-${i}`}>
                  <Node placed={p} cy={cy} />
                  {channel}
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

function Node({ placed, cy }: { placed: Placed; cy: number }): React.ReactElement {
  const { n, x, w } = placed;
  const cx = x + w / 2;
  const top = cy - SPINE_H / 2;

  if (n.kind === "gate") return <Gate n={n} x={x} w={w} cx={cx} cy={cy} top={top} />;
  if (n.kind === "done") {
    return (
      <g>
        <rect x={x} y={top} width={w} height={SPINE_H} rx={10} className="kdx-box kdx-box--io" />
        <text x={cx} y={cy + 5} textAnchor="middle" fontSize={13} className="kdx-gate-t">
          {n.label}
        </text>
      </g>
    );
  }

  // phase band
  const steps = n.steps ?? [];
  const cls = n.role ? ` kdx-phase--${n.role}` : "";
  const titleY = steps.length ? top + 18 : cy + 4;
  return (
    <g>
      <rect x={x} y={top} width={w} height={SPINE_H} rx={10} className={`kdx-phase${cls}`} />
      <text x={cx} y={titleY} textAnchor="middle" fontSize={12} className="kdx-phase-t">
        {n.label}
      </text>
      {steps.map((s, i) => (
        <text
          key={i}
          x={cx}
          y={top + 32 + i * 12}
          textAnchor="middle"
          fontSize={8.5}
          className="kdx-phase-step"
        >
          {s}
        </text>
      ))}
    </g>
  );
}

/** A gate: a green shield with a check, plus the amber "held" branch below. */
function Gate({
  n,
  x,
  w,
  cx,
  cy,
  top,
}: {
  n: LoopGate;
  x: number;
  w: number;
  cx: number;
  cy: number;
  top: number;
}): React.ReactElement {
  const bottom = top + SPINE_H;
  // A shield outline: rounded top, pointed-ish bottom — reads as a gate/seal.
  const r = 10;
  const shield = [
    `M${x + r},${top}`,
    `H${x + w - r}`,
    `Q${x + w},${top} ${x + w},${top + r}`,
    `V${top + SPINE_H * 0.62}`,
    `L${cx},${bottom}`,
    `L${x},${top + SPINE_H * 0.62}`,
    `V${top + r}`,
    `Q${x},${top} ${x + r},${top}`,
    "Z",
  ].join(" ");
  const subY = n.sub ? cy - 2 : cy + 3;
  return (
    <g>
      <path d={shield} className="kdx-gate-fill" />
      <path d={shield} className="kdx-gate" />
      {/* small check mark above the label */}
      <path
        d={`M${cx - 16},${top + 15} l6,6 l12,-13`}
        className="kdx-gate-check"
        transform={`translate(0, ${n.sub ? -2 : 2})`}
      />
      <text x={cx} y={subY + 8} textAnchor="middle" fontSize={11} className="kdx-gate-t">
        {n.label}
      </text>
      {n.sub && (
        <text x={cx} y={subY + 20} textAnchor="middle" fontSize={8.5} className="kdx-gate-sub">
          {n.sub}
        </text>
      )}

      {/* the "held" branch: not met → drop to a review node (amber, dashed) */}
      {n.hold && (
        <>
          <path
            d={`M${cx},${bottom} V${bottom + HOLD_DROP}`}
            className="kdx-hold"
            markerEnd="url(#kdx-gl-hold)"
          />
          <text
            x={cx}
            y={bottom + HOLD_DROP + 14}
            textAnchor="middle"
            fontSize={9}
            className="kdx-hold-t"
          >
            {n.hold}
          </text>
        </>
      )}
    </g>
  );
}

export default GatedLoopDiagram;
