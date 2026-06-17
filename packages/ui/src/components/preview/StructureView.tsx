import React, { useMemo, useState } from "react";
import { cn } from "../../lib/utils";
import { Badge } from "../ui/badge";
import { runsText } from "./renderDoc";
import { planeStyle, roleStyle, visibilityStyle } from "./roleStyle";
import type { ContentNode, ContentTree } from "./types";

// StructureView — the editor's structure/outline view (WS5). It renders the
// document's logical structure from the WS1 role layer: each block's semantic
// role (heading + level, list item, table cell, caption, …), its reading order,
// its plane (body / furniture / overlay / metadata) and visibility (hidden,
// conditional, screen/print-only), and its container nesting. A "layers" panel
// toggles facet values on and off — the same machinery as overlay-type toggles,
// applied to the structure facets (structure-geometry-landscape.md §8).

export interface StructureViewProps {
  tree: ContentTree;
  /** "source" or a target variant key — selects which text the rows show. */
  side?: string;
  className?: string;
}

function blockText(node: ContentNode, side: string): string {
  if (side && side !== "source") {
    const t = node.targets?.[side];
    if (t && t.length > 0) return runsText(t);
  }
  return runsText(node.source);
}

/** Normalize the implicit defaults so body/visible are first-class facet keys. */
function planeOf(node: ContentNode): string {
  const p = node.structure?.layer;
  return p && p !== "" ? p : "body";
}
function visibilityOf(node: ContentNode): string {
  const v = node.structure?.visibility;
  return v && v !== "" ? v : "visible";
}

/** A flattened outline row carrying display + structural info for one block. */
interface Row {
  node: ContentNode;
  depth: number;
  order: number;
}

// flatten walks the tree in document order, producing one Row per block. Group
// containers add a level of indentation; layers are transparent (they carry the
// part structure, not logical nesting) so the outline reads as content, not
// plumbing.
function flatten(tree: ContentTree): Row[] {
  const rows: Row[] = [];
  let order = 0;
  const walk = (n: ContentNode, depth: number) => {
    if (n.kind === "block") {
      order += 1;
      rows.push({ node: n, depth, order });
      return;
    }
    const childDepth = n.kind === "group" ? depth + 1 : depth;
    n.children?.forEach((c) => walk(c, childDepth));
  };
  tree.root.forEach((n) => walk(n, 0));
  return rows;
}

function StructureRow({ row, side }: { row: Row; side: string }): React.ReactElement {
  const { node, depth, order } = row;
  const s = node.structure;
  const rs = roleStyle(s?.role ?? node.type, s?.level);
  const plane = planeOf(node);
  const visibility = visibilityOf(node);
  const dimmed = plane !== "body" || visibility !== "visible";
  const text = blockText(node, side).trim();

  return (
    <div
      className={cn("flex items-start gap-2 py-1", dimmed && "opacity-70")}
      style={{ paddingLeft: `${depth * 1.25}rem` }}
      data-role={s?.role ?? node.type ?? ""}
      data-plane={plane}
      data-visibility={visibility}
      data-testid="structure-row"
    >
      <span className="w-6 shrink-0 pt-0.5 text-right text-[10px] tabular-nums text-muted-foreground">
        {order}
      </span>
      <Badge variant="outline" className={cn("shrink-0 border-current/25", rs.className)}>
        {rs.label}
      </Badge>
      {plane !== "body" && (
        <Badge
          variant="outline"
          className={cn("shrink-0 border-current/25", planeStyle(plane).className)}
        >
          {planeStyle(plane).label}
        </Badge>
      )}
      {visibility !== "visible" && (
        <Badge
          variant="outline"
          className={cn("shrink-0 border-current/25", visibilityStyle(visibility).className)}
        >
          {visibilityStyle(visibility).label}
        </Badge>
      )}
      <span className="min-w-0 flex-1 truncate text-sm" title={text}>
        {text || <span className="text-muted-foreground italic">(empty)</span>}
      </span>
    </div>
  );
}

/** A facet toggle key, namespaced so plane and visibility values never collide. */
type FacetKey = `plane:${string}` | `vis:${string}`;

function FacetToggle({
  label,
  className,
  active,
  onToggle,
}: {
  label: string;
  className: string;
  active: boolean;
  onToggle: () => void;
}): React.ReactElement {
  return (
    <button
      type="button"
      onClick={onToggle}
      aria-pressed={active}
      data-testid="facet-toggle"
      className={cn(
        "rounded-full border px-2 py-0.5 text-xs transition-opacity",
        active ? className : "border-border text-muted-foreground line-through opacity-50",
      )}
    >
      {label}
    </button>
  );
}

export default function StructureView({
  tree,
  side = "source",
  className,
}: StructureViewProps): React.ReactElement {
  const rows = useMemo(() => flatten(tree), [tree]);

  // The distinct facet values actually present, in a stable display order.
  const { planes, visibilities } = useMemo(() => {
    const planeSet = new Set<string>();
    const visSet = new Set<string>();
    rows.forEach((r) => {
      planeSet.add(planeOf(r.node));
      visSet.add(visibilityOf(r.node));
    });
    const order = (all: string[], present: Set<string>) => all.filter((v) => present.has(v));
    return {
      planes: order(["body", "furniture", "background", "overlay", "metadata"], planeSet),
      visibilities: order(
        ["visible", "conditional", "hidden", "print-only", "screen-only"],
        visSet,
      ),
    };
  }, [rows]);

  // Facets toggled OFF (everything visible by default).
  const [off, setOff] = useState<Set<FacetKey>>(() => new Set());
  const toggle = (key: FacetKey) =>
    setOff((prev) => {
      const next = new Set(prev);
      if (next.has(key)) next.delete(key);
      else next.add(key);
      return next;
    });

  const visibleRows = useMemo(
    () =>
      rows.filter(
        (r) => !off.has(`plane:${planeOf(r.node)}`) && !off.has(`vis:${visibilityOf(r.node)}`),
      ),
    [rows, off],
  );

  if (rows.length === 0) {
    return <p className="py-3 text-sm text-muted-foreground">No content blocks to outline.</p>;
  }

  // Only show the layers panel when there's more than one facet value to choose
  // between (a plain body-only document needs no controls).
  const hasFacets = planes.length > 1 || visibilities.length > 1;

  return (
    <div className={cn("flex flex-col gap-2", className)}>
      {hasFacets && (
        <div
          className="flex flex-wrap items-center gap-x-3 gap-y-1.5 border-b border-border/40 pb-2"
          data-testid="layers-panel"
        >
          {planes.length > 1 && (
            <div className="flex flex-wrap items-center gap-1.5">
              <span className="text-[10px] font-medium uppercase tracking-wide text-muted-foreground">
                Plane
              </span>
              {planes.map((p) => (
                <FacetToggle
                  key={p}
                  label={planeStyle(p).label}
                  className={planeStyle(p).className}
                  active={!off.has(`plane:${p}`)}
                  onToggle={() => toggle(`plane:${p}`)}
                />
              ))}
            </div>
          )}
          {visibilities.length > 1 && (
            <div className="flex flex-wrap items-center gap-1.5">
              <span className="text-[10px] font-medium uppercase tracking-wide text-muted-foreground">
                Visibility
              </span>
              {visibilities.map((v) => (
                <FacetToggle
                  key={v}
                  label={visibilityStyle(v).label}
                  className={visibilityStyle(v).className}
                  active={!off.has(`vis:${v}`)}
                  onToggle={() => toggle(`vis:${v}`)}
                />
              ))}
            </div>
          )}
        </div>
      )}

      <div className="flex flex-col divide-y divide-border/40">
        {visibleRows.length === 0 ? (
          <p className="py-3 text-sm text-muted-foreground">
            All blocks hidden by the active layer filters.
          </p>
        ) : (
          visibleRows.map((r) => <StructureRow key={r.node.id} row={r} side={side} />)
        )}
      </div>
    </div>
  );
}
