import React, { useState } from "react";
import { ChevronDown, ChevronRight } from "lucide-react";
import { Badge, cn } from "@neokapi/ui-primitives";
import BlockInspector from "./BlockInspector";
import type { ContentNode, ContentStats, ContentTree } from "./types";

export interface ContentTreeViewProps {
  tree: ContentTree;
  /** Block ids changed by a run, to flag in the inspector. */
  changedIds?: ReadonlySet<string>;
  /** Expand block detail by default (default false). */
  expandBlocks?: boolean;
  /** Show the document stats header (default true). */
  showStats?: boolean;
  className?: string;
}

const KIND_BG: Record<string, string> = {
  layer: "#22c55e",
  group: "#a855f7",
  data: "#94a3b8",
  media: "#f59e0b",
};

// ContentTreeView renders the hierarchical content model: Layers and Groups
// nest, Blocks expand into the full BlockInspector, and Data/Media show as
// leaves. Shared by the Anatomy explorer and the output viewer's Structure tab.
export default function ContentTreeView({
  tree,
  changedIds,
  expandBlocks = false,
  showStats = true,
  className,
}: ContentTreeViewProps): React.ReactElement {
  return (
    <div className={cn("kapi-reference flex flex-col gap-2 text-foreground", className)}>
      {showStats && <Stats stats={tree.stats} format={tree.format} />}
      <div className="rounded-lg border bg-card p-1.5">
        {tree.root.map((node) => (
          <NodeView
            key={`${node.kind}:${node.id}`}
            node={node}
            changedIds={changedIds}
            expandBlocks={expandBlocks}
          />
        ))}
      </div>
    </div>
  );
}

export function Stats({
  stats,
  format,
}: {
  stats: ContentStats;
  format: string;
}): React.ReactElement {
  const items: [string, number][] = [
    ["layers", stats.layers],
    ["groups", stats.groups],
    ["blocks", stats.blocks],
    ["data", stats.data],
    ["media", stats.media],
    ["runs", stats.runs],
  ];
  return (
    <div className="flex flex-wrap gap-1.5">
      <Badge variant="secondary" className="font-mono">
        {format} reader
      </Badge>
      {items
        .filter(([, n]) => n > 0)
        .map(([label, n]) => (
          <Badge key={label} variant="outline" className="gap-1">
            <span className="font-mono font-bold tabular-nums">{n}</span> {label}
          </Badge>
        ))}
    </div>
  );
}

function NodeView({
  node,
  changedIds,
  expandBlocks,
}: {
  node: ContentNode;
  changedIds?: ReadonlySet<string>;
  expandBlocks?: boolean;
}): React.ReactElement {
  const isContainer = node.kind === "layer" || node.kind === "group";
  const [open, setOpen] = useState(true);

  if (node.kind === "block") {
    return (
      <div className="py-0.5">
        <BlockInspector node={node} defaultOpen={expandBlocks} changed={changedIds?.has(node.id)} />
      </div>
    );
  }

  const badge = (
    <Badge style={{ backgroundColor: KIND_BG[node.kind] }} className="text-white">
      {node.kind}
    </Badge>
  );

  if (isContainer) {
    const meta = [node.format, node.locale].filter(Boolean).join(" · ");
    return (
      <div className="py-0.5">
        <button
          className="flex w-full items-center gap-2 rounded-md px-1.5 py-1 text-left hover:bg-muted"
          onClick={() => setOpen((o) => !o)}
        >
          {open ? (
            <ChevronDown className="size-3.5 text-muted-foreground" />
          ) : (
            <ChevronRight className="size-3.5 text-muted-foreground" />
          )}
          {badge}
          <span className="font-mono text-xs text-foreground/80">{node.name || node.id}</span>
          {meta && <span className="text-xs text-muted-foreground">{meta}</span>}
        </button>
        {open && node.children && node.children.length > 0 && (
          <div className="ml-3 border-l border-dashed border-border pl-2.5">
            {node.children.map((child) => (
              <NodeView
                key={`${child.kind}:${child.id}`}
                node={child}
                changedIds={changedIds}
                expandBlocks={expandBlocks}
              />
            ))}
          </div>
        )}
      </div>
    );
  }

  // data / media leaf
  return (
    <div className="flex items-center gap-2 px-1.5 py-1 text-sm">
      {badge}
      <span className="font-mono text-xs text-foreground/80">{node.id}</span>
      {node.summary && <span className="text-xs text-muted-foreground">{node.summary}</span>}
      {node.hasSkeleton && <span className="text-xs text-muted-foreground">· skeleton</span>}
    </div>
  );
}
