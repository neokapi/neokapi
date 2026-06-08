import { Handle, Position, type NodeProps } from "@xyflow/react";
import {
  Settings2,
  GitBranch,
  CheckCircle2,
  AlertCircle,
  Loader2,
  RefreshCw,
  X,
  Layers,
} from "lucide-react";
import { cn } from "@neokapi/ui-primitives";
import { getCategoryStyle } from "../category";
import type { IOPort } from "../types";
import { PortChip } from "./PortChip";
import { getSystemEffects } from "../sideEffects";

/** Accent color for the source-transform stage. */
const SOURCE_TRANSFORM_COLOR = "oklch(0.68 0.16 250)";
const SOURCE_TRANSFORM_BG = "oklch(0.68 0.16 250 / 0.10)";

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

/**
 * BoundaryPorts renders a node's IO as ports straddling the relevant edge: the
 * consumed ports sit on the input edge (top, or left when horizontal), the
 * produced ports on the output edge (bottom/right). The React Flow Handle is
 * the connection anchor; the chips are the visible port. The connecting edge
 * therefore runs port → port, so the data type is read at the boundary, not
 * floating mid-edge.
 */
function BoundaryPorts({
  handleType,
  position,
  ports,
  verb,
  railColor,
}: {
  handleType: "source" | "target";
  position: Position;
  ports: IOPort[];
  verb: "consumes" | "produces";
  railColor: string;
}) {
  const isVertical = position === Position.Top || position === Position.Bottom;
  const edge =
    position === Position.Top
      ? { top: -11, left: "50%", transform: "translateX(-50%)" }
      : position === Position.Bottom
        ? { bottom: -11, left: "50%", transform: "translateX(-50%)" }
        : position === Position.Left
          ? { left: -11, top: "50%", transform: "translateY(-50%)" }
          : { right: -11, top: "50%", transform: "translateY(-50%)" };
  const shown = ports.slice(0, 4);
  const hidden = ports.length - shown.length;

  return (
    <>
      <Handle
        type={handleType}
        position={position}
        style={{
          width: 8,
          height: 8,
          background: railColor,
          border: "2px solid var(--card)",
          ...(position === Position.Top
            ? { top: -5 }
            : position === Position.Bottom
              ? { bottom: -5 }
              : position === Position.Left
                ? { left: -5 }
                : { right: -5 }),
        }}
      />
      {ports.length > 0 && (
        <div
          className={cn(
            "nodrag absolute z-[2] flex items-center gap-0.5 rounded-md border border-border bg-card px-1 py-0.5 shadow-sm",
            isVertical ? "flex-row" : "flex-col",
          )}
          style={edge}
        >
          {shown.map((f, i) => (
            <PortChip
              key={`${verb}-${f.type}-${f.side ?? ""}-${i}`}
              type={f.type}
              side={f.side}
              optional={f.optional}
              verb={verb}
            />
          ))}
          {hidden > 0 && <span className="text-[8px] text-muted-foreground">+{hidden}</span>}
        </div>
      )}
    </>
  );
}

export function ToolNode({ data, selected }: NodeProps) {
  const category = (data.category as string) || "pipeline";
  const style = getCategoryStyle(category);
  const Icon = style.icon;
  const hasConfig = !!data.config && Object.keys(data.config as object).length > 0;
  const isParallel = !!data.parallel;
  const execState = data.execState as string | undefined;
  const partCount = data.partCount as number | undefined;
  const consumes = data.consumes as IOPort[] | undefined;
  const produces = data.produces as IOPort[] | undefined;
  const cardinality = data.cardinality as string | undefined;
  const defaultLocale = data.defaultLocale as string | undefined;
  const sideEffects = data.sideEffects as string[] | undefined;
  const systems = getSystemEffects(sideEffects, produces);
  const isValid = data.valid !== false;
  const toolName = (data.toolName as string) || "";
  const label = (data.label as string) || toolName || "";
  // Port types this tool requires that nothing upstream produces (set by the
  // requirement analysis in Phase 2; absent until then).
  const unmet = data.unmet as string[] | undefined;
  const vertical = data.layoutDirection === "vertical";
  // Explicit handle sides (serpentine flips them per row); fall back to the
  // linear vertical/horizontal convention.
  const inPosition = (data.inPosition as Position) ?? (vertical ? Position.Top : Position.Left);
  const outPosition =
    (data.outPosition as Position) ?? (vertical ? Position.Bottom : Position.Right);
  const retryConfig = data.retryConfig as Record<string, unknown> | undefined;
  const onRemove = data.onRemove as (() => void) | undefined;
  const isSourceTransformStage = data.stage === "source-transform";

  // Rail color: source-transform stage overrides the category color.
  const railColor = isSourceTransformStage ? SOURCE_TRANSFORM_COLOR : style.color;

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
                ? `2px solid ${railColor}`
                : isSourceTransformStage
                  ? `2px solid ${SOURCE_TRANSFORM_COLOR}`
                  : "2px solid var(--border)",
        opacity: isValid ? 1 : 0.7,
        background: isSourceTransformStage ? SOURCE_TRANSFORM_BG : undefined,
        boxShadow: selected
          ? `0 0 0 3px ${railColor}33, 0 4px 12px oklch(0 0 0 / 0.3)`
          : "0 2px 8px oklch(0 0 0 / 0.2)",
        animation: execState === "active" ? "nodePulse 1.5s ease-in-out infinite" : undefined,
      }}
    >
      {/* Category / stage rail */}
      <div className="w-1 shrink-0 rounded-l-[6px]" style={{ background: railColor }} />

      <div className="flex-1 px-3 py-2 relative">
        <BoundaryPorts
          handleType="target"
          position={inPosition}
          ports={consumes ?? []}
          verb="consumes"
          railColor={railColor}
        />

        {/* Header row */}
        <div className="flex items-center gap-1 mb-0.5">
          <Icon
            size={11}
            style={{ color: isSourceTransformStage ? SOURCE_TRANSFORM_COLOR : style.text }}
          />
          <span
            className="text-[9px] font-bold tracking-wider uppercase"
            style={{ color: isSourceTransformStage ? SOURCE_TRANSFORM_COLOR : style.text }}
          >
            {style.label}
          </span>
          {isSourceTransformStage && (
            <span
              className="ml-1 inline-flex items-center gap-0.5 rounded px-1 py-px text-[8px] font-semibold"
              style={{
                background: SOURCE_TRANSFORM_BG,
                color: SOURCE_TRANSFORM_COLOR,
                border: `1px solid ${SOURCE_TRANSFORM_COLOR}`,
              }}
              title="Runs in the source-transform stage — settles the model before main tools"
            >
              <Layers size={7} />
              pre
            </span>
          )}
          {isParallel && !isSourceTransformStage && (
            <GitBranch size={10} className="text-accent ml-auto" aria-label="Runs in parallel" />
          )}
          {hasConfig && !isParallel && !isSourceTransformStage && (
            <Settings2 size={10} className="text-muted-foreground ml-auto" />
          )}
          {hasConfig && isSourceTransformStage && (
            <Settings2
              size={10}
              className="ml-auto"
              style={{ color: SOURCE_TRANSFORM_COLOR, opacity: 0.7 }}
            />
          )}
        </div>

        {/* Tool name (centered) + tool id in code font */}
        <div className="flex flex-col items-center text-center">
          <div className="flex items-center justify-center gap-1">
            <span
              className={`text-[13px] font-semibold leading-tight ${isValid ? "text-foreground" : "text-foreground/50 line-through"}`}
            >
              {label}
            </span>
            {!isValid && (
              <AlertCircle
                size={12}
                style={{ color: "oklch(0.7 0.15 85)" }}
                aria-label="Unknown tool"
              />
            )}
          </div>
          {toolName && toolName !== label && (
            <span className="font-mono text-[9px] leading-tight text-muted-foreground">
              {toolName}
            </span>
          )}
          {!isValid && (
            <div className="text-[9px] font-medium" style={{ color: "oklch(0.65 0.15 85)" }}>
              Tool not found
            </div>
          )}
        </div>

        {/* Capability/locale meta + side effects */}
        <div className="flex items-center gap-1 mt-1">
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
          {defaultLocale && (
            <span
              className="rounded px-1 py-px text-[8px] font-mono font-medium"
              style={{ background: "oklch(0.6 0.12 290 / 0.12)", color: "oklch(0.55 0.12 290)" }}
              title={`Default locale: ${defaultLocale}`}
            >
              {defaultLocale}
            </span>
          )}
        </div>

        {/* Unmet-requirement warning (a required input nothing upstream produces) */}
        {unmet && unmet.length > 0 && (
          <div
            className="flex items-center gap-1 mt-1 text-[8px] font-medium"
            style={{ color: "oklch(0.62 0.17 45)" }}
            title={`Needs upstream: ${unmet.join(", ")} — add a tool that produces ${unmet.length > 1 ? "these" : "this"} earlier in the flow.`}
          >
            <AlertCircle size={9} />
            <span>needs {unmet.join(", ")}</span>
          </div>
        )}

        {/* External-system satellites: TM / termbase / API / analytics / vault
            the tool reads from or writes to, hanging off the right edge with a
            dashed connector. Collision-safe across layouts (part of the node). */}
        {systems.length > 0 && (
          <div className="absolute right-0 top-1/2 z-[1] flex -translate-y-1/2 translate-x-[calc(100%+2px)] flex-col gap-1">
            {systems.map((s) => {
              const SysIcon = s.icon;
              const arrow = s.direction === "read" ? "←" : s.direction === "write" ? "→" : "↔";
              return (
                <div
                  key={s.key}
                  className="flex items-center"
                  title={`${s.label}: ${s.description}`}
                >
                  <span
                    className="mr-0.5 inline-block w-3 border-t border-dashed"
                    style={{ borderColor: s.color }}
                    aria-hidden
                  />
                  <span className="mr-0.5 text-[8px] leading-none" style={{ color: s.color }}>
                    {arrow}
                  </span>
                  <span
                    className="flex items-center gap-0.5 rounded-md border bg-card px-1 py-0.5 shadow-sm"
                    style={{ borderColor: s.color }}
                  >
                    <SysIcon size={10} style={{ color: s.color }} aria-hidden />
                    <span className="text-[8px] font-medium" style={{ color: s.color }}>
                      {s.label}
                    </span>
                  </span>
                </div>
              );
            })}
          </div>
        )}

        <BoundaryPorts
          handleType="source"
          position={outPosition}
          ports={produces ?? []}
          verb="produces"
          railColor={railColor}
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

      {/* Part count badge (right corner, clear of the bottom output ports) */}
      {partCount !== undefined && partCount > 0 && (
        <div className="absolute -bottom-1.5 right-1 text-[9px] font-bold px-1.5 py-px rounded-full bg-secondary text-muted-foreground z-[1]">
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
