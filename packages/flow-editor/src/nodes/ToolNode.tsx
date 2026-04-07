import { Handle, Position, type NodeProps } from "@xyflow/react";
import {
  Settings2,
  GitBranch,
  CheckCircle2,
  AlertCircle,
  Loader2,
  RefreshCw,
  X,
} from "lucide-react";
import { cn } from "@neokapi/ui-primitives";
import { getCategoryStyle } from "../category";

/** Status badge shown at top-right of a node (complete/error/active). */
function NodeStatusBadge({ execState }: { execState: string }) {
  const base =
    "absolute -top-1 -right-1 size-3.5 rounded-full flex items-center justify-center z-[1]";
  if (execState === "complete") {
    return (
      <div className={cn(base, "bg-[oklch(0.65_0.15_145)]")}>
        <CheckCircle2 size={10} className="text-white" />
      </div>
    );
  }
  if (execState === "error") {
    return (
      <div className={cn(base, "bg-destructive")}>
        <AlertCircle size={10} className="text-white" />
      </div>
    );
  }
  if (execState === "active") {
    return (
      <div className={cn(base, "bg-accent")}>
        <Loader2 size={10} className="text-white animate-spin" />
      </div>
    );
  }
  return null;
}

const PORT_COLORS: Record<string, string> = {
  block: "oklch(0.75 0.12 85)",
  data: "oklch(0.6 0.02 260)",
  media: "oklch(0.7 0.12 180)",
  layer: "oklch(0.65 0.14 300)",
};

function portColor(type: string): string {
  return PORT_COLORS[type] ?? "var(--muted-foreground)";
}

export function ToolNode({ data, selected }: NodeProps) {
  const category = (data.category as string) || "pipeline";
  const style = getCategoryStyle(category);
  const Icon = style.icon;
  const hasConfig = !!data.config && Object.keys(data.config as object).length > 0;
  const isParallel = !!data.parallel;
  const execState = data.execState as string | undefined;
  const partCount = data.partCount as number | undefined;
  const inputs = data.inputs as string[] | undefined;
  const outputs = data.outputs as string[] | undefined;
  const cardinality = data.cardinality as string | undefined;
  const defaultLocale = data.defaultLocale as string | undefined;
  const sideEffects = data.sideEffects as string[] | undefined;
  const isValid = data.valid !== false;
  const vertical = data.layoutDirection === "vertical";
  const retryConfig = data.retryConfig as Record<string, unknown> | undefined;
  const onRemove = data.onRemove as (() => void) | undefined;

  return (
    <div
      className="relative flex min-w-[180px] rounded-lg overflow-visible bg-card transition-[border-color,box-shadow] duration-150"
      style={{
        border: !isValid
          ? "2px solid oklch(0.7 0.15 85)"
          : execState === "error"
            ? "2px solid var(--destructive)"
            : execState === "complete"
              ? "2px solid oklch(0.65 0.15 145)"
              : selected
                ? `2px solid ${style.color}`
                : "2px solid var(--border)",
        opacity: isValid ? 1 : 0.7,
        boxShadow: selected
          ? `0 0 0 3px ${style.color}33, 0 4px 12px oklch(0 0 0 / 0.3)`
          : "0 2px 8px oklch(0 0 0 / 0.2)",
        animation: execState === "active" ? "nodePulse 1.5s ease-in-out infinite" : undefined,
      }}
    >
      {/* Category rail */}
      <div className="w-1 shrink-0 rounded-l-[6px]" style={{ background: style.color }} />

      <div className="flex-1 px-3 py-2 relative">
        <Handle
          type="target"
          position={vertical ? Position.Top : Position.Left}
          style={{
            width: 10,
            height: 10,
            background: style.color,
            border: "2px solid var(--card)",
            ...(vertical ? { top: -9 } : { left: -9 }),
          }}
        />

        {/* Header row */}
        <div className="flex items-center gap-1 mb-0.5">
          <Icon size={11} style={{ color: style.text }} />
          <span
            className="text-[9px] font-bold tracking-wider uppercase"
            style={{ color: style.text }}
          >
            {style.label}
          </span>
          {isParallel && (
            <GitBranch size={10} className="text-accent ml-auto" aria-label="Runs in parallel" />
          )}
          {hasConfig && !isParallel && (
            <Settings2 size={10} className="text-muted-foreground ml-auto" />
          )}
        </div>

        {/* Tool name */}
        <div className="flex items-center gap-1">
          <span
            className={`text-[13px] font-semibold leading-tight ${isValid ? "text-foreground" : "text-foreground/50 line-through"}`}
          >
            {String(data.label || data.toolName || "")}
          </span>
          {!isValid && (
            <AlertCircle
              size={12}
              style={{ color: "oklch(0.7 0.15 85)" }}
              aria-label="Unknown tool"
            />
          )}
        </div>
        {!isValid && (
          <div className="text-[9px] font-medium" style={{ color: "oklch(0.65 0.15 85)" }}>
            Tool not found
          </div>
        )}

        {/* IO indicators */}
        <div className="flex items-center gap-1 mt-1">
          {/* Locale cardinality */}
          {cardinality && cardinality !== "monolingual" && (
            <span
              className="rounded px-1 py-px text-[8px] font-mono font-semibold uppercase tracking-wider"
              style={{
                background:
                  cardinality === "bilingual"
                    ? "oklch(0.55 0.15 250 / 0.12)"
                    : "oklch(0.55 0.15 320 / 0.12)",
                color:
                  cardinality === "bilingual" ? "oklch(0.55 0.15 250)" : "oklch(0.55 0.15 320)",
              }}
              title={
                cardinality === "bilingual" ? "Operates on two locales" : "Operates on all locales"
              }
            >
              {cardinality === "bilingual" ? "BI" : "ML"}
            </span>
          )}
          {/* Default locale */}
          {defaultLocale && (
            <span
              className="rounded px-1 py-px text-[8px] font-mono font-medium"
              style={{
                background: "oklch(0.6 0.12 290 / 0.12)",
                color: "oklch(0.55 0.12 290)",
              }}
              title={`Default locale: ${defaultLocale}`}
            >
              {defaultLocale}
            </span>
          )}
          {/* Port type dots */}
          {inputs?.map((t) => (
            <span
              key={`in-${t}`}
              title={`Input: ${t}`}
              className="size-1.5 rounded-full opacity-70"
              style={{ background: portColor(t) }}
            />
          ))}
          {inputs?.length && outputs?.length ? (
            <span className="text-[8px] text-muted-foreground">&rarr;</span>
          ) : null}
          {outputs?.map((t) => (
            <span
              key={`out-${t}`}
              title={`Output: ${t}`}
              className="size-1.5 rounded-full opacity-70"
              style={{ background: portColor(t) }}
            />
          ))}
          {/* Side effect indicator */}
          {sideEffects && sideEffects.length > 0 && (
            <span
              className="ml-auto text-[8px] font-mono opacity-60"
              title={`Side effects: ${sideEffects.join(", ")}`}
            >
              ⚡
            </span>
          )}
        </div>

        <Handle
          type="source"
          position={vertical ? Position.Bottom : Position.Right}
          style={{
            width: 10,
            height: 10,
            background: style.color,
            border: "2px solid var(--card)",
            ...(vertical ? { bottom: -9 } : { right: -9 }),
          }}
        />
      </div>

      {/* Remove button */}
      {onRemove && (
        <button
          onClick={(e) => {
            e.stopPropagation();
            onRemove();
          }}
          className={cn(
            "nopan absolute -top-1.5 -left-1.5 size-4 rounded-full",
            "bg-secondary border border-border",
            "flex items-center justify-center cursor-pointer z-[2]",
            "transition-opacity duration-150",
            selected ? "opacity-100" : "opacity-0",
          )}
          title="Remove tool (Delete)"
          aria-label="Remove tool"
        >
          <X size={10} className="text-muted-foreground" />
        </button>
      )}

      {/* Status badge */}
      {execState && <NodeStatusBadge execState={execState} />}

      {/* Part count badge */}
      {partCount !== undefined && partCount > 0 && (
        <div className="absolute -bottom-1.5 left-1/2 -translate-x-1/2 text-[9px] font-bold px-1.5 py-px rounded-full bg-secondary text-muted-foreground z-[1]">
          {partCount} pts
        </div>
      )}

      {/* Retry badge */}
      {retryConfig && (
        <div
          className="absolute -bottom-1 -left-0.5 size-3.5 rounded-full bg-secondary flex items-center justify-center z-[1]"
          title="Has retry policy"
        >
          <RefreshCw size={10} className="text-muted-foreground" />
        </div>
      )}
    </div>
  );
}
