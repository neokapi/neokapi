import React from "react";
import type { StageRole } from "./PipelineDiagram";
import "./diagram.css";

/*
  AxisFamiliesDiagram — the maturity axes grouped into their families, each a
  labelled card listing its axes with their grade range. The uniform-style way to
  show that the rubric's axes group by the question they answer (Comprehension /
  Assurance / Enablement), each family distinguished by its own accent.

      <AxisFamiliesDiagram
        families={[
          {
            name: "Comprehension",
            tagline: "how deeply we read it",
            axes: [
              { label: "Engine", range: "L0–L4" },
              { label: "Vocabulary", range: "V0–V3" },
              { label: "Structure & Geometry", range: "G0–G4" },
            ],
          },
          {
            name: "Assurance",
            tagline: "how we prove it",
            axes: [
              { label: "Corpus", range: "C0–C3" },
              { label: "Security", range: "S0–S4" },
            ],
          },
          {
            name: "Enablement",
            tagline: "how we work with it",
            axes: [
              { label: "Knowledge", range: "K0–K3" },
              { label: "Editor", range: "E0–E4" },
            ],
          },
        ]}
      />

  Pure SVG + CSS — light/dark with no JS, SSR-safe.
*/

export interface AxisFamily {
  name: string;
  /** Short phrase under the family name (the question it answers). */
  tagline: string;
  axes: { label: string; range: string }[];
}

export interface AxisFamiliesDiagramProps {
  families: AxisFamily[];
  caption?: string;
}

const PAD = 14;
const COL_GAP = 18;
const HEADER_H = 40;
const ROW_H = 26;
const CARD_PAD_B = 12;
const NAME_CHAR = 7.6;
const TAG_CHAR = 6; // tagline, small
const LABEL_CHAR = 7; // axis label
const MONO = 6.6; // grade range, mono

// Each family gets a distinct accent, cycled from the kit's box roles.
const ROLES: StageRole[] = ["io", "qa", "annotate", "translate"];
const roleBox = (role: StageRole) => ` kdx-box--${role}`;
const roleSub = (role: StageRole) => ` kdx-sub--${role}`;

const rangeChipW = (range: string) => Math.round(range.length * MONO) + 14;

export function AxisFamiliesDiagram({
  families,
  caption,
}: AxisFamiliesDiagramProps): React.ReactElement {
  // Per-family column width fits the header and its widest axis row.
  const colW = families.map((f) =>
    Math.max(
      120,
      Math.round(f.name.length * NAME_CHAR) + 24,
      Math.round(f.tagline.length * TAG_CHAR) + 24,
      ...f.axes.map((a) => Math.round(a.label.length * LABEL_CHAR) + 14 + rangeChipW(a.range) + 24),
    ),
  );

  // Lay families out left→right.
  let x = PAD;
  const placed = families.map((f, i) => {
    const p = { f, x, w: colW[i], role: ROLES[i % ROLES.length] };
    x += colW[i] + COL_GAP;
    return p;
  });
  const totalW = x - COL_GAP + PAD;
  const cardH = (f: AxisFamily) => HEADER_H + f.axes.length * ROW_H + CARD_PAD_B;
  const totalH = Math.max(...families.map(cardH)) + 2 * PAD;

  return (
    <div className="kdx">
      <div className="kdx-scroll">
        <div className="kdx-canvas" style={{ minWidth: Math.min(totalW, 520), maxWidth: totalW }}>
          <svg
            viewBox={`0 0 ${totalW} ${totalH}`}
            xmlns="http://www.w3.org/2000/svg"
            role="img"
            aria-label={`Axis families: ${families.map((f) => f.name).join(", ")}`}
          >
            {placed.map(({ f, x: cx0, w, role }, i) => (
              <g key={`${f.name}-${i}`}>
                <rect
                  x={cx0}
                  y={PAD}
                  width={w}
                  height={cardH(f)}
                  rx={10}
                  className={`kdx-box${roleBox(role)}`}
                />
                <text
                  x={cx0 + 12}
                  y={PAD + 20}
                  fontSize={13.5}
                  className="kdx-label"
                  style={{ fill: `var(--kdx-${role})` }}
                >
                  {f.name}
                </text>
                <text x={cx0 + 12} y={PAD + 33} fontSize={9.5} className="kdx-note">
                  {f.tagline}
                </text>
                <line
                  x1={cx0 + 10}
                  y1={PAD + HEADER_H - 2}
                  x2={cx0 + w - 10}
                  y2={PAD + HEADER_H - 2}
                  className="kdx-divider"
                />

                {f.axes.map((a, j) => {
                  const rowCenter = PAD + HEADER_H + j * ROW_H + ROW_H / 2;
                  const chipW = rangeChipW(a.range);
                  const chipX = cx0 + w - 12 - chipW;
                  return (
                    <g key={`${a.label}-${j}`}>
                      <text x={cx0 + 14} y={rowCenter + 3.5} fontSize={11} className="kdx-label">
                        {a.label}
                      </text>
                      <rect
                        x={chipX}
                        y={rowCenter - 10}
                        width={chipW}
                        height={20}
                        rx={6}
                        className={`kdx-box${roleBox(role)}`}
                      />
                      <text
                        x={chipX + chipW / 2}
                        y={rowCenter + 3}
                        textAnchor="middle"
                        fontSize={9.5}
                        className={`kdx-sub${roleSub(role)}`}
                      >
                        {a.range}
                      </text>
                    </g>
                  );
                })}
              </g>
            ))}
          </svg>
        </div>
      </div>
      {caption && <p className="kdx-caption">{caption}</p>}
    </div>
  );
}

export default AxisFamiliesDiagram;
