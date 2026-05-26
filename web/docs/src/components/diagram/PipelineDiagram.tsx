import React from "react";
import "./diagram.css";

/*
  PipelineDiagram — the reusable primitive behind the uniform diagram style.

  Declare a flow as a list of stages and it lays out role-colored boxes joined by
  `chan Part` connectors, in exactly the visual language of the Architecture hero.
  This is what the inline ASCII flow diagrams across the docs (pipeline, tools,
  brand-voice, the architecture-decision pages, …) convert to.

      <PipelineDiagram
        stages={[
          { label: "Reader", sub: "DataFormat", role: "io" },
          { label: "segmentation", role: "annotate" },
          { label: "ai-translate", role: "translate" },
          { label: "qa-check", role: "qa" },
          { label: "Writer", sub: "DataFormat", role: "io" },
        ]}
      />

  Static by default (calm on a page full of them); pass `animated` for flowing
  Parts. Pure SVG + CSS — themes for light/dark with no JS, SSR-safe.
*/

export type StageRole = "io" | "annotate" | "translate" | "qa" | "tool";

export interface PipelineStage {
  label: string;
  sub?: string;
  role?: StageRole;
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
const STAGE_Y = 30;
const STAGE_H = 58;
const CY = STAGE_Y + STAGE_H / 2;
const GAP = 40; // channel length between stages
const CHAR = 7.2; // approx px per char for width estimate

const stageWidth = (s: PipelineStage): number => {
  const longest = Math.max(s.label.length, (s.sub ?? "").length);
  return Math.max(78, Math.round(longest * CHAR) + 26);
};

const roleBox = (role?: StageRole) => (role && role !== "tool" ? ` kdx-box--${role}` : "");
const roleSub = (role?: StageRole) => (role && role !== "tool" ? ` kdx-sub--${role}` : "");

export function PipelineDiagram({
  stages,
  animated = false,
  caption,
  channelLabel = "chan",
}: PipelineDiagramProps): React.ReactElement {
  // Lay stages out left-to-right, accumulating x.
  let x = PAD;
  const placed = stages.map((s) => {
    const w = stageWidth(s);
    const box = { ...s, x, w };
    x += w + GAP;
    return box;
  });
  const totalW = x - GAP + PAD;
  const totalH = STAGE_H + STAGE_Y + (caption ? 8 : 16);

  return (
    <div className={`kdx${animated ? " kdx--animated" : ""}`}>
      <div className="kdx-scroll">
        <div className="kdx-canvas" style={{ minWidth: Math.min(totalW, 760) }}>
          <svg
            viewBox={`0 0 ${totalW} ${totalH}`}
            xmlns="http://www.w3.org/2000/svg"
            role="img"
            aria-label={`Pipeline: ${stages.map((s) => s.label).join(" then ")}`}
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
              const cx = s.x + s.w / 2;
              const next = placed[i + 1];
              return (
                <g key={`${s.label}-${i}`}>
                  <rect
                    x={s.x}
                    y={STAGE_Y}
                    width={s.w}
                    height={STAGE_H}
                    rx={9}
                    className={`kdx-box${roleBox(s.role)}`}
                  />
                  <text
                    x={cx}
                    y={s.sub ? CY - 1 : CY + 4}
                    textAnchor="middle"
                    fontSize={12.5}
                    className="kdx-label"
                  >
                    {s.label}
                  </text>
                  {s.sub && (
                    <text
                      x={cx}
                      y={CY + 15}
                      textAnchor="middle"
                      fontSize={9}
                      className={`kdx-sub${roleSub(s.role)}`}
                    >
                      {s.sub}
                    </text>
                  )}

                  {next && (
                    <>
                      <line
                        x1={s.x + s.w}
                        y1={CY}
                        x2={next.x - 4}
                        y2={CY}
                        className="kdx-channel"
                        markerEnd="url(#kdx-arrow)"
                      />
                      {channelLabel && (
                        <text
                          x={(s.x + s.w + next.x) / 2}
                          y={CY - 7}
                          textAnchor="middle"
                          fontSize={7.5}
                          className="kdx-chan"
                        >
                          {channelLabel}
                        </text>
                      )}
                      {animated && (
                        <FlowDot
                          path={`M${s.x + s.w},${CY} L${next.x - 4},${CY}`}
                          dur={1.6}
                          begin={i * 0.3}
                        />
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
