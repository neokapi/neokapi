// ParallelGroupNode — a parallel step rendered as ONE composite node with its
// branch tools listed inside, a single input and a single output. This keeps the
// graph readable: a parallel group occupies a single slot in the layout instead
// of fanning out into branch nodes with crossing merge edges. Clicking a branch
// row selects that branch for configuration.

import { Handle, Position, type NodeProps } from "@xyflow/react";
import { GitBranch, X, AlertCircle } from "lucide-react";
import { cn } from "@neokapi/ui-primitives";
import { getCategoryStyle } from "../category";
import { PortChip } from "./PortChip";
import type { ParallelBranch } from "../conversion";
import type { IOPort } from "../types";

const PARALLEL_COLOR = "oklch(0.62 0.15 300)";

export function ParallelGroupNode({ data, selected }: NodeProps) {
  const branches = (data.branches as ParallelBranch[] | undefined) ?? [];
  const vertical = data.layoutDirection === "vertical";
  const inPosition = (data.inPosition as Position) ?? (vertical ? Position.Top : Position.Left);
  const outPosition =
    (data.outPosition as Position) ?? (vertical ? Position.Bottom : Position.Right);
  const onSelectBranch = data.onSelectBranch as ((index: number) => void) | undefined;
  const onRemove = data.onRemove as (() => void) | undefined;
  const selectedBranch = data.selectedBranch as number | undefined;
  const unmet = data.unmet as string[] | undefined;

  const handleStyle = {
    width: 8,
    height: 8,
    background: PARALLEL_COLOR,
    border: "2px solid var(--card)",
  } as const;

  return (
    <div
      className="relative flex w-[220px] flex-col rounded-lg bg-card overflow-visible"
      style={{
        border: selected ? `2px solid ${PARALLEL_COLOR}` : "2px solid var(--border)",
        boxShadow: selected
          ? `0 0 0 3px ${PARALLEL_COLOR}33, 0 4px 12px oklch(0 0 0 / 0.3)`
          : "0 2px 8px oklch(0 0 0 / 0.2)",
      }}
    >
      <Handle type="target" position={inPosition} style={handleStyle} />

      {/* Header */}
      <div className="flex items-center gap-1 px-3 pt-2">
        <GitBranch size={11} style={{ color: PARALLEL_COLOR }} />
        <span
          className="text-[9px] font-bold uppercase tracking-wider"
          style={{ color: PARALLEL_COLOR }}
        >
          Parallel
        </span>
        <span className="ml-auto text-[8px] font-medium text-muted-foreground">
          {branches.length} branches
        </span>
      </div>

      {/* Branch rows */}
      <div className="flex flex-col gap-1 px-2 pb-2 pt-1.5">
        {branches.map((b, i) => {
          const style = getCategoryStyle(b.category);
          const isSel = selectedBranch === i;
          return (
            <button
              key={`${b.toolName}-${i}`}
              type="button"
              onClick={(e) => {
                e.stopPropagation();
                onSelectBranch?.(i);
              }}
              className={cn(
                "nodrag flex w-full items-center gap-1.5 rounded-md border px-1.5 py-1 text-left transition-colors",
                isSel ? "bg-secondary" : "bg-card hover:bg-secondary/60",
              )}
              style={{ borderColor: isSel ? PARALLEL_COLOR : "var(--border)" }}
            >
              <span
                className="size-2 shrink-0 rounded-full"
                style={{ background: style.color }}
                aria-hidden
              />
              <span className="min-w-0 flex-1 leading-tight">
                <span
                  className={cn(
                    "block truncate text-[11px] font-semibold",
                    b.valid ? "text-foreground" : "text-foreground/50 line-through",
                  )}
                >
                  {b.label}
                </span>
                {b.toolName !== b.label && (
                  <span className="block truncate font-mono text-[8px] text-muted-foreground">
                    {b.toolName}
                  </span>
                )}
              </span>
              {!b.valid && <AlertCircle size={11} style={{ color: "oklch(0.7 0.15 85)" }} />}
              {(b.produces ?? []).slice(0, 2).map((p: IOPort, pi) => (
                <PortChip key={`${p.type}-${pi}`} type={p.type} side={p.side} verb="produces" />
              ))}
            </button>
          );
        })}
      </div>

      {unmet && unmet.length > 0 && (
        <div
          className="flex items-center gap-1 px-3 pb-1.5 text-[8px] font-medium"
          style={{ color: "oklch(0.62 0.17 45)" }}
          title={`Needs upstream: ${unmet.join(", ")}`}
        >
          <AlertCircle size={9} />
          <span>needs {unmet.join(", ")}</span>
        </div>
      )}

      <Handle type="source" position={outPosition} style={handleStyle} />

      {onRemove && (
        <button
          onClick={(e) => {
            e.stopPropagation();
            onRemove();
          }}
          className={cn(
            "nopan absolute -top-1.5 -left-1.5 size-4 rounded-full bg-secondary border border-border",
            "flex items-center justify-center cursor-pointer z-[2] transition-opacity duration-150",
            selected ? "opacity-100" : "opacity-0",
          )}
          title="Remove parallel group (Delete)"
          aria-label="Remove parallel group"
        >
          <X size={10} className="text-muted-foreground" />
        </button>
      )}
    </div>
  );
}
