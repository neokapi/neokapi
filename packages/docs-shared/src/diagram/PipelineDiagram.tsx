import React from "react";
import "./diagram.css";

/*
  PipelineDiagram — the reusable primitive behind the uniform diagram style.

  Declare a flow as a list of stages and it lays out role-colored boxes joined by
  `chan Part` connectors, in exactly the visual language of the Architecture hero.

      <PipelineDiagram
        stages={[
          { label: "Reader", sub: "DataFormat", role: "io" },
          { label: "segmentation", role: "annotate" },
          { label: "translate", role: "translate" },
          { label: "qa", role: "qa" },
          { label: "Writer", sub: "DataFormat", role: "io" },
        ]}
      />

  A stage may instead fan out into parallel `lanes` (workers / branches), which
  render as a stacked group with fan-out/fan-in lines and an optional label:

      { lanes: [{ label: "Worker 1" }, { label: "Worker 2" }], parallelLabel: "fan-out" }

  Static by default (calm on a page full of them); pass `animated` for flowing
  Parts. Pure SVG + CSS — themes for light/dark with no JS, SSR-safe.
*/

export type StageRole = "io" | "annotate" | "translate" | "qa" | "tool";

export interface PipelineStage {
  label?: string;
  /** Mono sub-line, role-tinted (e.g. "DataFormat", "Tool"). */
  sub?: string;
  /** Extra muted note line under the box (e.g. "handles Block · passes Layer*"). */
  note?: string;
  role?: StageRole;
  /** Render this stage as a parallel fan-out of these lanes instead of one box. */
  lanes?: { label: string; sub?: string }[];
  /** Label shown above a `lanes` group (e.g. "fan-out · N goroutines"). */
  parallelLabel?: string;
}

export interface PipelineDiagramProps {
  stages: PipelineStage[];
  /** Flow animated Parts along the channels. Default false. */
  animated?: boolean;
  /** Caption shown under the diagram. */
  caption?: string;
  /** Label on each channel hop. Default "chan". Pass "" to hide. */
  channelLabel?: string;
}

const PAD = 14;
const STAGE_H = 58;
const GAP = 42; // channel length between stages
const CHAR = 7.2; // approx px per char for width estimate
const LANE_H = 26;
const LANE_GAP = 9;
const FAN = 26; // gutter on each side of a lanes group for the fan lines
const TOP = 10; // top padding above the content band

/** Vertical offsets (from the stage center) for 1–3 stacked text lines. */
const LINE_DELTAS: Record<number, number[]> = {
  1: [4],
  2: [-1, 13],
  3: [-6, 8, 20],
};

const textWidth = (s: string) => Math.round(s.length * CHAR);
const laneWidth = (l: { label: string; sub?: string }) =>
  Math.max(textWidth(l.label), textWidth(l.sub ?? ""));
const stackHeight = (n: number) => n * LANE_H + (n - 1) * LANE_GAP;

const stageWidth = (s: PipelineStage): number => {
  if (s.lanes && s.lanes.length) {
    const inner = Math.max(70, Math.max(...s.lanes.map(laneWidth)) + 24);
    return inner + 2 * FAN;
  }
  const longest = Math.max(
    textWidth(s.label ?? ""),
    textWidth(s.sub ?? ""),
    textWidth(s.note ?? ""),
  );
  return Math.max(78, longest + 26);
};

const roleBox = (role?: StageRole) => (role && role !== "tool" ? ` kdx-box--${role}` : "");
const roleSub = (role?: StageRole) => (role && role !== "tool" ? ` kdx-sub${`--${role}`}` : "");

interface Placed extends PipelineStage {
  x: number;
  w: number;
}

export function PipelineDiagram({
  stages,
  animated = false,
  caption,
  channelLabel = "chan",
}: PipelineDiagramProps): React.ReactElement {
  // Lay stages out left-to-right, accumulating x.
  let x = PAD;
  const placed: Placed[] = stages.map((s) => {
    const w = stageWidth(s);
    const box = { ...s, x, w };
    x += w + GAP;
    return box;
  });
  const totalW = x - GAP + PAD;

  // Dynamic vertical sizing: a lanes group can be taller than a normal box.
  const maxStack = Math.max(
    0,
    ...placed.filter((s) => s.lanes?.length).map((s) => stackHeight(s.lanes!.length)),
  );
  const anyLanes = maxStack > 0;
  const labelPad = anyLanes ? 16 : 0; // room for the parallelLabel above the stack
  const contentH = Math.max(STAGE_H, maxStack);
  const cy = TOP + labelPad + contentH / 2;
  const totalH = cy + contentH / 2 + labelPad + (caption ? 6 : 12);
  const boxTop = cy - STAGE_H / 2;

  return (
    <div className={`kdx${animated ? " kdx--animated" : ""}`}>
      <div className="kdx-scroll">
        <div className="kdx-canvas" style={{ minWidth: Math.min(totalW, 460), maxWidth: totalW }}>
          <svg
            viewBox={`0 0 ${totalW} ${totalH}`}
            xmlns="http://www.w3.org/2000/svg"
            role="img"
            aria-label={`Pipeline: ${stages
              .map((s) => s.label ?? (s.lanes ? s.lanes.map((l) => l.label).join(" / ") : "stage"))
              .join(" then ")}`}
          >
            <defs>
              <marker
                id="kdx-arrow"
                markerWidth="7"
                markerHeight="7"
                refX="5.5"
                refY="3"
                orient="auto"
              >
                <path d="M0,0 L6,3 L0,6 Z" className="kdx-arrow" />
              </marker>
            </defs>

            {placed.map((s, i) => {
              const next = placed[i + 1];
              const channel = next && (
                <>
                  <line
                    x1={s.x + s.w}
                    y1={cy}
                    x2={next.x - 4}
                    y2={cy}
                    className="kdx-channel"
                    markerEnd="url(#kdx-arrow)"
                  />
                  {channelLabel && (
                    <text
                      x={(s.x + s.w + next.x) / 2}
                      y={cy - 7}
                      textAnchor="middle"
                      fontSize={7.5}
                      className="kdx-chan"
                    >
                      {channelLabel}
                    </text>
                  )}
                  {animated && (
                    <FlowDot
                      path={`M${s.x + s.w},${cy} L${next.x - 4},${cy}`}
                      dur={1.6}
                      begin={i * 0.3}
                    />
                  )}
                </>
              );

              if (s.lanes && s.lanes.length) {
                return (
                  <g key={`lanes-${i}`}>
                    <LaneGroup stage={s} cy={cy} />
                    {channel}
                  </g>
                );
              }

              const cx = s.x + s.w / 2;
              const lines: { t: string; size: number; cls: string }[] = [
                { t: s.label ?? "", size: 12.5, cls: "kdx-label" },
              ];
              if (s.sub) lines.push({ t: s.sub, size: 9, cls: `kdx-sub${roleSub(s.role)}` });
              if (s.note) lines.push({ t: s.note, size: 8, cls: "kdx-chan" });
              const deltas = LINE_DELTAS[lines.length] ?? LINE_DELTAS[3];
              return (
                <g key={`${s.label}-${i}`}>
                  <rect
                    x={s.x}
                    y={boxTop}
                    width={s.w}
                    height={STAGE_H}
                    rx={9}
                    className={`kdx-box${roleBox(s.role)}`}
                  />
                  {lines.map((ln, li) => (
                    <text
                      key={li}
                      x={cx}
                      y={cy + deltas[li]}
                      textAnchor="middle"
                      fontSize={ln.size}
                      className={ln.cls}
                    >
                      {ln.t}
                    </text>
                  ))}
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

/** A fan-out group: stacked lane boxes with fan-in/fan-out lines + optional label. */
function LaneGroup({ stage, cy }: { stage: Placed; cy: number }): React.ReactElement {
  const lanes = stage.lanes!;
  const laneX = stage.x + FAN;
  const laneW = stage.w - 2 * FAN;
  const stack = stackHeight(lanes.length);
  const firstTop = cy - stack / 2;
  const fanInX = stage.x;
  const fanOutX = stage.x + stage.w;
  const role = stage.role;
  return (
    <g>
      {stage.parallelLabel && (
        <text
          x={stage.x + stage.w / 2}
          y={firstTop - 7}
          textAnchor="middle"
          fontSize={8.5}
          className="kdx-note"
        >
          {stage.parallelLabel}
        </text>
      )}
      <rect
        x={laneX - 7}
        y={firstTop - 6}
        width={laneW + 14}
        height={stack + 12}
        rx={10}
        className="kdx-bracket"
      />
      {lanes.map((ln, i) => {
        const ly = firstTop + i * (LANE_H + LANE_GAP);
        const lcy = ly + LANE_H / 2;
        const lcx = laneX + laneW / 2;
        return (
          <g key={`${ln.label}-${i}`}>
            <path d={`M${fanInX},${cy} L${laneX},${lcy}`} className="kdx-thin kdx-thin--t" />
            <path
              d={`M${laneX + laneW},${lcy} L${fanOutX},${cy}`}
              className="kdx-thin kdx-thin--t"
            />
            <rect
              x={laneX}
              y={ly}
              width={laneW}
              height={LANE_H}
              rx={7}
              className={`kdx-box${roleBox(role)}`}
            />
            <text
              x={lcx}
              y={ln.sub ? lcy - 1 : lcy + 4}
              textAnchor="middle"
              fontSize={10.5}
              className="kdx-label"
            >
              {ln.label}
            </text>
            {ln.sub && (
              <text
                x={lcx}
                y={lcy + 10}
                textAnchor="middle"
                fontSize={8}
                className={`kdx-sub${roleSub(role)}`}
              >
                {ln.sub}
              </text>
            )}
          </g>
        );
      })}
    </g>
  );
}

function FlowDot({
  path,
  dur,
  begin,
}: {
  path: string;
  dur: number;
  begin: number;
}): React.ReactElement {
  return (
    <circle r={2.8} className="kdx-dot kdx-dot--io">
      <animateMotion dur={`${dur}s`} begin={`${begin}s`} repeatCount="indefinite" path={path} />
      <animate
        attributeName="opacity"
        values="0;1;1;0"
        keyTimes="0;0.12;0.85;1"
        dur={`${dur}s`}
        begin={`${begin}s`}
        repeatCount="indefinite"
      />
    </circle>
  );
}

export default PipelineDiagram;
