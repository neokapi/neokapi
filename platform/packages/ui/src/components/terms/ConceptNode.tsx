import { memo } from "react";
import { Handle, Position } from "@xyflow/react";
import type { NodeProps } from "@xyflow/react";
import { Badge } from "../ui/badge";
import { cn } from "../../lib/utils";

export interface ConceptNodeData {
  conceptId: string;
  preferredTerm: string;
  domain: string;
  definition: string;
  localeCount: number;
  termCount: number;
  childCount: number;
  parentCount: number;
  isSelected: boolean;
  source?: string;
}

export function ConceptNodeComponent({ data }: NodeProps) {
  const d = data as unknown as ConceptNodeData;

  return (
    <div
      className={cn(
        "rounded-lg border bg-card text-card-foreground shadow-sm transition-all w-[220px]",
        "hover:shadow-md hover:border-primary/50 cursor-pointer",
        d.isSelected && "ring-2 ring-primary border-primary shadow-md",
      )}
    >
      <Handle type="target" position={Position.Top} className="!w-2 !h-2 !bg-muted-foreground/40" />

      <div className="px-3 py-2.5">
        {/* Domain tag */}
        {d.domain && (
          <span className="text-[10px] uppercase tracking-wider text-muted-foreground font-medium">
            {d.domain}
          </span>
        )}

        {/* Preferred term — the concept's main label */}
        <div
          className="text-sm font-semibold mt-0.5 leading-tight truncate"
          title={d.preferredTerm}
        >
          {d.preferredTerm}
        </div>

        {/* Definition preview */}
        {d.definition && (
          <p className="text-[11px] text-muted-foreground mt-1 line-clamp-2 leading-snug">
            {d.definition}
          </p>
        )}

        {/* Stats row */}
        <div className="flex items-center gap-1.5 mt-2">
          {d.localeCount > 0 && (
            <Badge variant="secondary" className="text-[9px] px-1 py-0 h-4">
              {d.localeCount} {d.localeCount === 1 ? "lang" : "langs"}
            </Badge>
          )}
          {d.termCount > 1 && (
            <Badge variant="secondary" className="text-[9px] px-1 py-0 h-4">
              {d.termCount} terms
            </Badge>
          )}
          {(d.childCount > 0 || d.parentCount > 0) && (
            <Badge variant="outline" className="text-[9px] px-1 py-0 h-4">
              {d.childCount + d.parentCount} links
            </Badge>
          )}
        </div>
      </div>

      <Handle
        type="source"
        position={Position.Bottom}
        className="!w-2 !h-2 !bg-muted-foreground/40"
      />
    </div>
  );
}

export const ConceptNode = memo(ConceptNodeComponent);
