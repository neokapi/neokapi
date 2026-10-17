import React from "react";
import "./diagram.css";

/*
  PlaceGraphDiagram: how places contain one another. A chain of places runs
  from the widest at the top to the narrowest at the bottom, each sitting
  within the one above it. Places drawn beside a chain place sit within the
  same parent, and one bus joins the row to that parent. A place made of
  places is a dashed outline around its members, named to the right of the
  row. A place the recipe declares has an accent outline.

      <PlaceGraphDiagram
        chain={[
          { id: "world", name: "World", type: "world" },
          { id: "europe", name: "Europe", type: "region" },
          { id: "no", name: "Norway", type: "country" },
          { id: "oslo", name: "Oslo", type: "locality", declared: true },
        ]}
        siblings={{ no: [{ id: "se", name: "Sweden", type: "country" }] }}
        madeOf={[{ id: "nordics", name: "Nordics", type: "territory", of: ["no", "se"] }]}
      />

  The members of a place made of places are drawn in one row, side by side:
  a chain place and the places beside it.

  Pure SVG + CSS: light and dark with no JS, SSR-safe.
*/

export interface PlaceGraphNode {
  /** The place id, as a geo axis takes it ("no-03"). */
  id: string;
  /** The display name ("Oslo"). */
  name: string;
  /** The place type ("country", "locality"). */
  type: string;
  /** A place the recipe declares, drawn with an accent outline. */
  declared?: boolean;
}

export interface PlaceGraphMadeOf extends PlaceGraphNode {
  /** The ids of the places it is made of, drawn side by side in one row. */
  of: string[];
}

export interface PlaceGraphDiagramProps {
  /** Places from the widest to the narrowest; each sits within the one before. */
  chain: PlaceGraphNode[];
  /** Places beside a chain place, keyed by its id, sitting within the same parent. */
  siblings?: Record<string, PlaceGraphNode[]>;
  /** Places made of places, each outlined around its members. */
  madeOf?: PlaceGraphMadeOf[];
  caption?: string;
}

const PAD = 16;
const BOX_H = 40;
const ROW_GAP = 36;
const H_GAP = 12;
const OUTLINE = 6; // outline inset around a made-of place's members
const NEST = 5; // extra inset for each further outline on a row
const NAME_CHAR = 7;
const SUB_CHAR = 5.8;
const LEGEND_H = 28;

const subOf = (n: PlaceGraphNode) => `${n.id} · ${n.type}`;
const widthOf = (n: PlaceGraphNode) =>
  Math.round(Math.max(n.name.length * NAME_CHAR, subOf(n).length * SUB_CHAR)) + 22;

interface Placed {
  node: PlaceGraphNode;
  row: number;
  x: number;
  w: number;
}

export function PlaceGraphDiagram({
  chain,
  siblings = {},
  madeOf = [],
  caption,
}: PlaceGraphDiagramProps): React.ReactElement {
  const rowTop = (row: number) => PAD + row * (BOX_H + ROW_GAP);
  const chainW = Math.max(...chain.map(widthOf));

  const placed = new Map<string, Placed>();
  const rows: Placed[][] = [];
  chain.forEach((node, row) => {
    const own: Placed = { node, row, x: PAD, w: chainW };
    placed.set(node.id, own);
    rows[row] = [own];
    let right = PAD + chainW;
    for (const sib of siblings[node.id] ?? []) {
      const p: Placed = { node: sib, row, x: right + H_GAP, w: widthOf(sib) };
      placed.set(sib.id, p);
      rows[row].push(p);
      right = p.x + p.w;
    }
  });

  // A place made of places outlines its members and is named right of the row.
  const outlines = madeOf.flatMap((m) => {
    const members = m.of.map((id) => placed.get(id)).filter((p): p is Placed => !!p);
    if (members.length === 0) return [];
    return [{ m, row: members[0].row, members }];
  });
  const rowRight = rows.map((r) => Math.max(...r.map((p) => p.x + p.w)));
  const labelX = new Map<number, number>();
  const shapes = outlines.map((o, i) => {
    const depth = outlines.slice(0, i).filter((p) => p.row === o.row).length;
    const inset = OUTLINE + depth * NEST;
    const left = Math.min(...o.members.map((p) => p.x)) - inset;
    const right = Math.max(...o.members.map((p) => p.x + p.w)) + inset;
    const lx = labelX.get(o.row) ?? rowRight[o.row] + OUTLINE + 14;
    labelX.set(o.row, lx + widthOf(o.m) + 8);
    return { ...o, inset, left, right, lx };
  });

  const contentRight = Math.max(
    ...rowRight,
    ...shapes.map((s) => s.lx + widthOf(s.m)),
    ...shapes.map((s) => s.right),
  );
  const totalW = contentRight + PAD;
  const legendY = rowTop(chain.length - 1) + BOX_H + ROW_GAP - 6;
  const totalH = legendY + LEGEND_H + PAD / 2;

  const label = [
    chain.map((n) => n.name).join(" within "),
    ...madeOf.map((m) => `${m.name} made of ${m.of.join(", ")}`),
  ].join("; ");

  return (
    <div className="kdx">
      <div className="kdx-scroll">
        <div className="kdx-canvas" style={{ minWidth: Math.min(totalW, 460), maxWidth: totalW }}>
          <svg
            viewBox={`0 0 ${totalW} ${totalH}`}
            xmlns="http://www.w3.org/2000/svg"
            role="img"
            aria-label={`Place graph: ${label}`}
          >
            <defs>
              <marker
                id="kdx-arrow-place"
                markerWidth="7"
                markerHeight="7"
                refX="5.5"
                refY="3"
                orient="auto"
              >
                <path d="M0,0 L6,3 L0,6 Z" className="kdx-arrow" />
              </marker>
            </defs>

            {/* Each row hangs from its parent: one bus, a drop to every place. */}
            {rows.map((row, r) => {
              if (r === 0) return null;
              const parent = placed.get(chain[r - 1].id)!;
              const px = parent.x + parent.w / 2;
              const busY = rowTop(r) - ROW_GAP / 2;
              const xs = row.map((p) => p.x + p.w / 2);
              return (
                <g key={`bus-${r}`}>
                  <line
                    x1={px}
                    y1={busY}
                    x2={px}
                    y2={rowTop(r - 1) + BOX_H + 4}
                    className="kdx-channel"
                    markerEnd="url(#kdx-arrow-place)"
                  />
                  {xs.length > 1 && (
                    <line
                      x1={Math.min(...xs)}
                      y1={busY}
                      x2={Math.max(...xs)}
                      y2={busY}
                      className="kdx-channel"
                    />
                  )}
                  {xs.map((x) => (
                    <line
                      key={x}
                      x1={x}
                      y1={rowTop(r) - (row.length > 1 ? OUTLINE + 2 : 0)}
                      x2={x}
                      y2={busY}
                      className="kdx-channel"
                    />
                  ))}
                </g>
              );
            })}

            {shapes.map((s) => (
              <g key={`of-${s.m.id}`}>
                <rect
                  x={s.left}
                  y={rowTop(s.row) - s.inset}
                  width={s.right - s.left}
                  height={BOX_H + 2 * s.inset}
                  rx={10}
                  className="kdx-loop"
                />
                <text x={s.lx} y={rowTop(s.row) + 17} fontSize={12} className="kdx-label">
                  {s.m.name}
                </text>
                <text
                  x={s.lx}
                  y={rowTop(s.row) + 31}
                  fontSize={9.5}
                  className="kdx-sub kdx-mono kdx-sub--annotate"
                >
                  {subOf(s.m)}
                </text>
              </g>
            ))}

            {[...placed.values()].map((p) => {
              const declared = !!p.node.declared;
              const y = rowTop(p.row);
              return (
                <g key={p.node.id}>
                  <rect
                    x={p.x}
                    y={y}
                    width={p.w}
                    height={BOX_H}
                    rx={8}
                    className={declared ? "kdx-box kdx-box--io" : "kdx-box"}
                  />
                  <text x={p.x + 11} y={y + 17} fontSize={12} className="kdx-label">
                    {p.node.name}
                  </text>
                  <text
                    x={p.x + 11}
                    y={y + 31}
                    fontSize={9.5}
                    className={declared ? "kdx-sub kdx-mono kdx-sub--io" : "kdx-note kdx-mono"}
                  >
                    {subOf(p.node)}
                  </text>
                </g>
              );
            })}

            <g transform={`translate(${PAD}, ${legendY})`}>
              <line x1={0} y1={10} x2={26} y2={10} className="kdx-channel" />
              <text x={34} y={14} fontSize={10} className="kdx-note">
                sits within
              </text>
              <rect x={108} y={2} width={26} height={16} rx={5} className="kdx-loop" />
              <text x={142} y={14} fontSize={10} className="kdx-note">
                made of
              </text>
              <rect x={204} y={2} width={22} height={16} rx={4} className="kdx-box kdx-box--io" />
              <text x={234} y={14} fontSize={10} className="kdx-note">
                declared in the recipe
              </text>
            </g>
          </svg>
        </div>
      </div>
      {caption && <p className="kdx-caption">{caption}</p>}
    </div>
  );
}

export default PlaceGraphDiagram;
