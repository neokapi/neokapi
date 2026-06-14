import React, { useMemo } from "react";
import { cn } from "../../lib/utils";
import { Badge } from "../ui/badge";
import { runsText } from "./renderDoc";
import { roleStyle } from "./roleStyle";
import type { ContentNode, ContentTree } from "./types";

// StructureView — the editor's structure/outline view (WS5). It renders the
// document's logical structure from the WS1 role layer: each block's semantic
// role (heading + level, list item, table cell, caption, …), its reading order,
// its layout layer (body vs furniture), and its container nesting. It is a pure
// projection of the ContentTree's `structure` field — reflowable, format-shaped
// independent (a DOCX, a DocLang doc, or a Docling PDF all outline the same way).

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
  const isFurniture = s?.layer === "furniture";
  const text = blockText(node, side).trim();

  return (
    <div
      className={cn("flex items-start gap-2 py-1", isFurniture && "opacity-70")}
      style={{ paddingLeft: `${depth * 1.25}rem` }}
      data-role={s?.role ?? node.type ?? ""}
      data-testid="structure-row"
    >
      <span className="w-6 shrink-0 pt-0.5 text-right text-[10px] tabular-nums text-muted-foreground">
        {order}
      </span>
      <Badge variant="outline" className={cn("shrink-0 border-current/25", rs.className)}>
        {rs.label}
      </Badge>
      {isFurniture && (
        <Badge variant="ghost" className="shrink-0 text-[10px] text-muted-foreground">
          furniture
        </Badge>
      )}
      <span className="min-w-0 flex-1 truncate text-sm" title={text}>
        {text || <span className="text-muted-foreground italic">(empty)</span>}
      </span>
    </div>
  );
}

export default function StructureView({
  tree,
  side = "source",
  className,
}: StructureViewProps): React.ReactElement {
  const rows = useMemo(() => flatten(tree), [tree]);

  if (rows.length === 0) {
    return <p className="py-3 text-sm text-muted-foreground">No content blocks to outline.</p>;
  }

  return (
    <div className={cn("flex flex-col divide-y divide-border/40", className)}>
      {rows.map((r) => (
        <StructureRow key={r.node.id} row={r} side={side} />
      ))}
    </div>
  );
}
