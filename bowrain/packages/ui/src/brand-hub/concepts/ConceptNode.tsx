// The concept node rendered on the React Flow canvas (AD-021): a compact card
// accented by the concept's dominant term status, with its label, domain, term
// count, and a status badge. Hidden centre handles let the floating edges
// attach anywhere on the border.
import { memo } from "react";
import { Handle, Position, type NodeProps, type Node } from "@xyflow/react";
import { cn } from "@neokapi/ui-primitives";
import type { TermStatus } from "../../types/brand-graph";
import { TermStatusBadge } from "../shell/atoms";
import { statusColorVar, isRetiredStatus } from "./graph-style";

export interface ConceptNodeData extends Record<string, unknown> {
  label: string;
  domain?: string;
  status?: TermStatus | "";
  term_count: number;
  /** Part of the focused neighbourhood (kept bright). */
  focused?: boolean;
  /** Outside the focused neighbourhood (faded back). */
  dimmed?: boolean;
}

export type ConceptFlowNode = Node<ConceptNodeData, "concept">;

/** Footprint of a concept node — kept in sync with graph-layout defaults. */
export const CONCEPT_NODE_WIDTH = 184;
export const CONCEPT_NODE_HEIGHT = 76;

function ConceptNodeImpl({ data, selected }: NodeProps<ConceptFlowNode>) {
  const accent = statusColorVar(data.status);
  const retired = isRetiredStatus(data.status);
  return (
    <div
      className={cn(
        "relative flex h-full w-full overflow-hidden rounded-lg border bg-card text-card-foreground shadow-sm transition-all",
        selected && "border-primary ring-2 ring-primary/40",
        data.focused && !selected && "border-primary/50",
        data.dimmed && "opacity-35",
      )}
    >
      <span aria-hidden className="w-1.5 shrink-0" style={{ backgroundColor: accent }} />
      <div className="flex min-w-0 flex-1 flex-col justify-center gap-1 px-3 py-2">
        <div
          className={cn(
            "truncate text-sm font-medium leading-tight text-foreground",
            retired && "line-through decoration-from-font",
          )}
          title={data.label}
        >
          {data.label}
        </div>
        <div className="flex items-center gap-1.5">
          {data.status ? (
            <TermStatusBadge status={data.status} className="px-1.5 py-0 text-[10px]" />
          ) : (
            <span className="text-[10px] text-muted-foreground">no terms</span>
          )}
          {data.domain && (
            <span className="truncate text-[10px] text-muted-foreground">{data.domain}</span>
          )}
          {data.term_count > 0 && (
            <span className="ml-auto shrink-0 text-[10px] tabular-nums text-muted-foreground">
              {data.term_count}
            </span>
          )}
        </div>
      </div>
      {/* Hidden handles — floating edges compute their own attachment points. */}
      <Handle
        type="target"
        position={Position.Top}
        isConnectable={false}
        className="!h-px !w-px !min-w-0 !border-0 !bg-transparent"
        style={{ left: "50%", top: "50%" }}
      />
      <Handle
        type="source"
        position={Position.Bottom}
        isConnectable={false}
        className="!h-px !w-px !min-w-0 !border-0 !bg-transparent"
        style={{ left: "50%", top: "50%" }}
      />
    </div>
  );
}

export const ConceptNode = memo(ConceptNodeImpl);
