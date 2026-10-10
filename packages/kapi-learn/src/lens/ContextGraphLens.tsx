import React, { useMemo, useState } from "react";
import { layoutGraph, type Placed } from "./layout.ts";
import type { EdgeKind, Focus, GraphEdge, GraphModel, GraphNode } from "./types.ts";

// The context graph, drawn. Three columns and a row (see layout.ts); a chapter
// brings one part forward and the rest dims; a node that appeared since the
// last snapshot pulses once; pressing a node lists what the engine said about
// it in the panel beside the graph.

export interface ContextGraphLensProps {
  model: GraphModel | null;
  /** The snapshot before this one, so what changed can be shown. */
  previous?: GraphModel | null;
  focus?: Focus;
  loading?: boolean;
}

const FOCUS_KINDS: Record<Focus, GraphNode["kind"][]> = {
  all: [
    "project",
    "collection",
    "source",
    "edition",
    "point",
    "voice",
    "terms",
    "concept",
    "memory",
    "record",
  ],
  content: ["project", "collection", "source", "edition"],
  point: ["source", "point", "voice", "terms", "memory"],
  context: ["point", "voice", "terms", "concept", "memory"],
  terms: ["terms", "concept", "source"],
  memory: ["memory", "edition", "point"],
  editions: ["source", "edition"],
  record: ["record"],
};

function focusSet(model: GraphModel, focus: Focus): Set<string> {
  const kinds = new Set(FOCUS_KINDS[focus] ?? FOCUS_KINDS.all);
  const ids = new Set(model.nodes.filter((n) => kinds.has(n.kind)).map((n) => n.id));
  if (focus === "record") {
    for (const e of model.edges) if (e.kind === "records" && ids.has(e.from)) ids.add(e.to);
  }
  return ids;
}

/** Cut a label to what fits a node, with an ellipsis. */
function fit(text: string, maxChars: number): string {
  return text.length <= maxChars ? text : `${text.slice(0, Math.max(1, maxChars - 1))}…`;
}

function kindLabel(kind: GraphNode["kind"]): string {
  switch (kind) {
    case "source":
      return "source file";
    case "edition":
      return "edition";
    case "point":
      return "point";
    case "voice":
      return "voice profile";
    case "terms":
      return "terms";
    case "concept":
      return "concept";
    case "memory":
      return "content memory";
    case "record":
      return "record";
    default:
      return kind;
  }
}

const RIGHTWARD: EdgeKind[] = ["in", "edition", "at", "governs", "applies", "has"];

function edgePath(e: GraphEdge, a: Placed, b: Placed): string {
  if (e.kind === "records") {
    // Up from the record row to the bottom of its subject.
    const x1 = a.x + a.w / 2;
    const y1 = a.y;
    const x2 = b.x + b.w / 2;
    const y2 = b.y + b.h;
    const my = (y1 + y2) / 2;
    return `M ${x1} ${y1} C ${x1} ${my}, ${x2} ${my}, ${x2} ${y2}`;
  }
  if (e.kind === "in" || e.kind === "edition" || e.kind === "has") {
    // Down the column: from the left margin of the parent to the child's left edge.
    const x1 = a.x + 10;
    const y1 = a.y + a.h;
    const x2 = b.x;
    const y2 = b.y + b.h / 2;
    return `M ${x1} ${y1} C ${x1} ${y2}, ${x1} ${y2}, ${x2} ${y2}`;
  }
  const leftToRight = RIGHTWARD.includes(e.kind) || a.x < b.x;
  const x1 = leftToRight ? a.x + a.w : a.x;
  const y1 = a.y + a.h / 2;
  const x2 = leftToRight ? b.x : b.x + b.w;
  const y2 = b.y + b.h / 2;
  const dx = (x2 - x1) / 2;
  return `M ${x1} ${y1} C ${x1 + dx} ${y1}, ${x2 - dx} ${y2}, ${x2} ${y2}`;
}

function edgeLabelPoint(e: GraphEdge, a: Placed, b: Placed): [number, number] {
  if (e.kind === "records") return [(a.x + a.w / 2 + b.x + b.w / 2) / 2, (a.y + b.y + b.h) / 2];
  const leftToRight = RIGHTWARD.includes(e.kind) || a.x < b.x;
  const x1 = leftToRight ? a.x + a.w : a.x;
  const x2 = leftToRight ? b.x : b.x + b.w;
  return [(x1 + x2) / 2, (a.y + a.h / 2 + b.y + b.h / 2) / 2 - 4];
}

export default function ContextGraphLens({
  model,
  previous,
  focus = "all",
  loading = false,
}: ContextGraphLensProps): React.ReactElement {
  const [selected, setSelected] = useState<string | null>(null);
  const layout = useMemo(() => (model ? layoutGraph(model) : null), [model]);
  const inFocus = useMemo(
    () => (model ? focusSet(model, focus) : new Set<string>()),
    [model, focus],
  );
  const fresh = useMemo(() => {
    if (!model || !previous) return new Set<string>();
    const had = new Set(previous.nodes.map((n) => n.id));
    return new Set(model.nodes.filter((n) => !had.has(n.id)).map((n) => n.id));
  }, [model, previous]);
  const changed = useMemo(() => {
    if (!model || !previous) return new Set<string>();
    const before = new Map(previous.nodes.map((n) => [n.id, n]));
    return new Set(
      model.nodes
        .filter((n) => {
          const b = before.get(n.id);
          return b && (b.count !== n.count || b.status !== n.status || b.sub !== n.sub);
        })
        .map((n) => n.id),
    );
  }, [model, previous]);

  const selectedNode = model?.nodes.find((n) => n.id === selected) ?? null;

  if (!model || !layout) {
    return (
      <div className="kl-graph kl-graph--empty" role="status">
        {loading
          ? "Reading the project's context…"
          : "The graph appears once the engine has read the project."}
      </div>
    );
  }

  return (
    <div className={`kl-graph${loading ? " kl-graph--loading" : ""}`} data-focus={focus}>
      <svg
        className="kl-graph__svg"
        viewBox={`0 0 ${layout.width} ${layout.height}`}
        role="img"
        aria-label={`The context graph: ${model.summary}`}
      >
        <defs>
          <marker
            id="kl-arrow"
            viewBox="0 0 8 8"
            refX="7"
            refY="4"
            markerWidth="7"
            markerHeight="7"
            orient="auto-start-reverse"
          >
            <path d="M 0 0 L 8 4 L 0 8 z" className="kl-graph__arrow" />
          </marker>
        </defs>

        {layout.columns.map((c) => (
          <text key={c.label} x={c.x} y={18} className="kl-graph__column">
            {c.label}
          </text>
        ))}
        {model.nodes.some((n) => n.kind === "record") && (
          <>
            <line
              x1={16}
              x2={layout.width - 16}
              y1={layout.recordY}
              y2={layout.recordY}
              className="kl-graph__rule"
            />
            <text x={16} y={layout.recordY + 14} className="kl-graph__column">
              The record, in order
            </text>
          </>
        )}

        <g className="kl-graph__edges">
          {model.edges.map((e) => {
            const a = layout.placed.get(e.from);
            const b = layout.placed.get(e.to);
            if (!a || !b) return null;
            const dim = !(inFocus.has(e.from) && inFocus.has(e.to));
            const width = e.weight ? Math.min(5, 1 + Math.log2(e.weight + 1)) : 1.2;
            const [lx, ly] = edgeLabelPoint(e, a, b);
            return (
              <g key={e.id} className={`kl-edge kl-edge--${e.kind}${dim ? " kl-edge--dim" : ""}`}>
                <path
                  d={edgePath(e, a, b)}
                  strokeWidth={width}
                  markerEnd={
                    e.kind === "records" || e.kind === "in" || e.kind === "has"
                      ? undefined
                      : "url(#kl-arrow)"
                  }
                />
                {e.label && (
                  <text x={lx} y={ly} className="kl-edge__label" textAnchor="middle">
                    {e.label}
                  </text>
                )}
              </g>
            );
          })}
        </g>

        <g className="kl-graph__nodes">
          {[...layout.placed.values()].map((p) => {
            const n = p.node;
            const dim = !inFocus.has(n.id);
            const cls = [
              "kl-node",
              `kl-node--${n.kind}`,
              n.status ? `kl-node--status-${n.status}` : "",
              dim ? "kl-node--dim" : "",
              fresh.has(n.id) ? "kl-node--new" : "",
              changed.has(n.id) ? "kl-node--changed" : "",
              selected === n.id ? "kl-node--selected" : "",
            ]
              .filter(Boolean)
              .join(" ");
            const maxChars = Math.floor((p.w - 24) / 7.2);
            const small = n.kind === "concept";
            return (
              <g
                key={n.id}
                className={cls}
                transform={`translate(${p.x}, ${p.y})`}
                role="button"
                tabIndex={0}
                aria-label={`${kindLabel(n.kind)} ${n.label}`}
                onClick={() => setSelected((s) => (s === n.id ? null : n.id))}
                onKeyDown={(ev) => {
                  if (ev.key === "Enter" || ev.key === " ") {
                    ev.preventDefault();
                    setSelected((s) => (s === n.id ? null : n.id));
                  }
                }}
              >
                <rect
                  width={p.w}
                  height={p.h}
                  rx={n.kind === "record" ? p.h / 2 : 8}
                  ry={n.kind === "record" ? p.h / 2 : 8}
                />
                {n.kind === "concept" && (
                  <circle cx={13} cy={p.h / 2} r={4.5} className="kl-node__dot" />
                )}
                <text
                  x={n.kind === "concept" ? 24 : 12}
                  y={small ? p.h / 2 + 4 : n.sub ? 18 : p.h / 2 + 4}
                  className="kl-node__label"
                >
                  {fit(n.label, small ? maxChars - 2 : maxChars)}
                  {small && n.sub ? (
                    <tspan className="kl-node__inline">
                      {" "}
                      {fit(n.sub, Math.max(0, maxChars - n.label.length - 3))}
                    </tspan>
                  ) : null}
                </text>
                {!small && n.sub && (
                  <text x={12} y={n.kind === "edition" ? 30 : 33} className="kl-node__sub">
                    {fit(n.sub, maxChars + 6)}
                  </text>
                )}
                {n.kind === "edition" && n.pct && (
                  <g className="kl-node__bar" transform={`translate(12, ${p.h - 8})`}>
                    <rect width={p.w - 24} height={4} rx={2} className="kl-node__bar-track" />
                    <rect
                      width={((p.w - 24) * n.pct.translated) / 100}
                      height={4}
                      rx={2}
                      className="kl-node__bar-translated"
                    />
                    <rect
                      width={((p.w - 24) * n.pct.established) / 100}
                      height={4}
                      rx={2}
                      className="kl-node__bar-established"
                    />
                  </g>
                )}
                {n.kind === "edition" && n.status && (
                  <text x={p.w - 10} y={18} textAnchor="end" className="kl-node__state">
                    {n.status}
                  </text>
                )}
              </g>
            );
          })}
        </g>
      </svg>

      <aside className="kl-graph__panel" aria-live="polite">
        {selectedNode ? (
          <>
            <p className="kl-graph__panel-kind">{kindLabel(selectedNode.kind)}</p>
            <h3 className="kl-graph__panel-title">{selectedNode.label}</h3>
            {selectedNode.sub && !selectedNode.detail?.length && (
              <p className="kl-graph__panel-sub">{selectedNode.sub}</p>
            )}
            <dl className="kl-graph__detail">
              {(selectedNode.detail ?? []).map(([k, v], i) =>
                v ? (
                  <div key={`${k}-${i}`}>
                    <dt>{k}</dt>
                    <dd>{v}</dd>
                  </div>
                ) : null,
              )}
            </dl>
            <button type="button" className="kl-linkbtn" onClick={() => setSelected(null)}>
              Back to the summary
            </button>
          </>
        ) : (
          <>
            <p className="kl-graph__panel-kind">
              Context graph{model.revision !== undefined ? `, revision ${model.revision}` : ""}
            </p>
            <p className="kl-graph__panel-sub">{model.summary}.</p>
            {model.stale && (
              <p className="kl-graph__panel-note">
                The engine has read no content yet, so no term is placed in a block.
              </p>
            )}
            <ul className="kl-graph__legend" aria-label="Legend">
              <li className="kl-legend--content">content and its editions</li>
              <li className="kl-legend--point">the point the content sits at</li>
              <li className="kl-legend--voice">voice profile</li>
              <li className="kl-legend--terms">terms: preferred, deprecated, forbidden</li>
              <li className="kl-legend--memory">content memory</li>
              <li className="kl-legend--record">the record of what was decided</li>
            </ul>
            <p className="kl-graph__panel-hint">Press a node for what the engine says about it.</p>
          </>
        )}
      </aside>
    </div>
  );
}
