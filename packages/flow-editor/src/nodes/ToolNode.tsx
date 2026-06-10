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
import type { PlacementDiagnostic } from "../placement";
import { formatUs } from "../traceSelectors";
import { PortChip } from "./PortChip";
import { getSystemEffects } from "../sideEffects";

/** Accent color for transformer (rewrites-source) affordances. */
const TRANSFORMER_COLOR = "oklch(0.68 0.16 250)";

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
  const spanUs = data.spanUs as number | undefined;
  const consumes = data.consumes as IOPort[] | undefined;
  const produces = data.produces as IOPort[] | undefined;
  const cardinality = data.cardinality as string | undefined;
  const defaultLocale = data.defaultLocale as string | undefined;
  const sideEffects = data.sideEffects as string[] | undefined;
  const systems = getSystemEffects(sideEffects, produces, consumes);
  const isValid = data.valid !== false;
  const toolName = (data.toolName as string) || "";
  const label = (data.label as string) || toolName || "";
  // Port types this tool requires that nothing upstream produces (set by the
  // requirement analysis in Phase 2; absent until then).
  const unmet = data.unmet as string[] | undefined;
  // Transformer placement diagnostics for this step (AD-006 placement pass).
  const placement = data.placement as PlacementDiagnostic[] | undefined;
  const placementError = placement?.find((d) => d.severity === "error");
  const placementWarning = placement?.find((d) => d.severity === "warning");
  // Handle sides come from the serpentine layout (it flips them per wrapped row);
  // fall back to a left→right flow.
  const inPosition = (data.inPosition as Position) ?? Position.Left;
  const outPosition = (data.outPosition as Position) ?? Position.Right;
  // Side-effect satellites sit on a free side (one not used by the in/out ports)
  // so they never overlap the ports or the edge to the next tool.
  const usedSides = new Set<Position>([inPosition, outPosition]);
  const satelliteSide: Position = !usedSides.has(Position.Top)
    ? Position.Top
    : !usedSides.has(Position.Right)
      ? Position.Right
      : Position.Bottom;
  const retryConfig = data.retryConfig as Record<string, unknown> | undefined;
  const onRemove = data.onRemove as (() => void) | undefined;
  // Transformer (AD-006): this tool rewrites the source. Ordinary ordered step;
  // the badge is informational and the placement pass flags an unsafe slot.
  const isTransformer = !!data.isSourceTransform;

  const railColor = style.color;

  return (
    <div
      className="relative flex h-[84px] min-w-[170px] max-w-[200px] rounded-lg overflow-visible bg-card transition-[border-color,box-shadow] duration-150"
      style={{
        border: !isValid
          ? "2px solid oklch(0.7 0.15 85)"
          : placementError
            ? "2px solid var(--destructive)"
            : execState === "error"
              ? "2px solid var(--destructive)"
              : execState === "complete"
                ? "2px solid oklch(0.65 0.15 145)"
                : selected
                  ? `2px solid ${railColor}`
                  : "2px solid var(--border)",
        opacity: isValid ? 1 : 0.7,
        boxShadow: selected
          ? `0 0 0 3px ${railColor}33, 0 4px 12px oklch(0 0 0 / 0.3)`
          : "0 2px 8px oklch(0 0 0 / 0.2)",
        animation: execState === "active" ? "nodePulse 1.5s ease-in-out infinite" : undefined,
      }}
    >
      {/* Category rail */}
      <div className="w-1 shrink-0 rounded-l-[6px]" style={{ background: railColor }} />

      <div className="flex-1 px-3 relative flex flex-col justify-center">
        <BoundaryPorts
          handleType="target"
          position={inPosition}
          ports={consumes ?? []}
          verb="consumes"
          railColor={railColor}
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
          {/* Transformer badge: this tool rewrites the source (AD-006). */}
          {isTransformer && (
            <span
              className="ml-1 inline-flex items-center gap-0.5 rounded px-1 py-px text-[8px] font-semibold"
              style={{
                color: TRANSFORMER_COLOR,
                border: `1px solid ${TRANSFORMER_COLOR}`,
              }}
              title="Transformer: rewrites the source. The framework applier rebases overlays across its rewrite; the placement pass validates its position."
            >
              <Layers size={7} />
              rewrites source
            </span>
          )}
          {/* Project preset chip: this tool inherits defaults.tools config. */}
          {!!data.hasPreset && (
            <span
              className="ml-1 rounded px-1 py-px text-[8px] font-semibold"
              style={{
                color: "oklch(0.55 0.12 290)",
                border: "1px solid oklch(0.55 0.12 290 / 0.55)",
                background: "oklch(0.55 0.12 290 / 0.08)",
              }}
              title="Inherits a project preset (defaults.tools); the step's own config overrides it per key."
            >
              preset
            </span>
          )}
          {isParallel && (
            <GitBranch size={10} className="text-accent ml-auto" aria-label="Runs in parallel" />
          )}
          {hasConfig && !isParallel && (
            <Settings2 size={10} className="text-muted-foreground ml-auto" />
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

        {/* Unmet-requirement warning (a required input nothing upstream
            produces). Rendered as an absolute overlay BELOW the node so it never
            changes the node's fixed height — keeping handle centers aligned
            across a row for straight connectors. */}
        {unmet && unmet.length > 0 && (
          <div
            className="absolute left-1/2 top-full z-[1] mt-1 flex -translate-x-1/2 items-center gap-1 whitespace-nowrap text-[8px] font-medium"
            style={{ color: "oklch(0.62 0.17 45)" }}
            title={`Needs upstream: ${unmet.join(", ")} — add a tool that produces ${unmet.length > 1 ? "these" : "this"} earlier in the flow.`}
          >
            <AlertCircle size={9} />
            <span>needs {unmet.join(", ")}</span>
          </div>
        )}

        {/* Placement diagnostic (AD-006): the transformer sits in an unsafe or
            wasteful slot. Same overlay treatment as the unmet warning so the
            node keeps its fixed height. Errors win over warnings. */}
        {!unmet && (placementError || placementWarning) && (
          <div
            className="absolute left-1/2 top-full z-[1] mt-1 flex -translate-x-1/2 items-center gap-1 whitespace-nowrap text-[8px] font-medium"
            style={{
              color: placementError ? "var(--destructive)" : "oklch(0.62 0.17 45)",
            }}
            title={(placementError ?? placementWarning)!.message}
          >
            <AlertCircle size={9} />
            <span>{placementError ? "unsafe placement" : "late placement"}</span>
          </div>
        )}

        {/* External-system satellites: TM / termbase / API / analytics / vault
            the tool reads from or writes to. Dashed chips on a free side —
            deliberately distinct from the solid ports and the main edge to the
            next tool. Part of the node, so collision-safe across layouts. */}
        {systems.length > 0 &&
          (() => {
            const onTop = satelliteSide === Position.Top;
            const onBottom = satelliteSide === Position.Bottom;
            const stacked = onTop || onBottom; // chips laid in a row, stub vertical
            const containerCls = onTop
              ? "bottom-full left-1/2 -translate-x-1/2 -translate-y-1 flex-row items-end gap-1.5"
              : onBottom
                ? "top-full left-1/2 -translate-x-1/2 translate-y-1 flex-row items-start gap-1.5"
                : "left-full top-1/2 -translate-y-1/2 translate-x-1 flex-col gap-1";
            // Arrow points toward the chip for writes, toward the node for reads.
            const arrowFor = (dir: string) => {
              if (onTop) return dir === "read" ? "↓" : dir === "write" ? "↑" : "↕";
              if (onBottom) return dir === "read" ? "↑" : dir === "write" ? "↓" : "↕";
              return dir === "read" ? "←" : dir === "write" ? "→" : "↔";
            };
            return (
              <div className={cn("absolute z-[1] flex", containerCls)}>
                {systems.map((s) => {
                  const SysIcon = s.icon;
                  const chip = (
                    <span
                      className="flex items-center gap-0.5 rounded-md border border-dashed bg-card px-1 py-0.5 shadow-sm"
                      style={{ borderColor: s.color }}
                    >
                      <SysIcon size={10} style={{ color: s.color }} aria-hidden />
                      <span className="text-[8px] font-medium" style={{ color: s.color }}>
                        {s.label}
                      </span>
                    </span>
                  );
                  const arrow = (
                    <span className="text-[8px] leading-none" style={{ color: s.color }}>
                      {arrowFor(s.direction)}
                    </span>
                  );
                  const vStub = (
                    <span
                      className="inline-block h-1.5 border-l border-dashed"
                      style={{ borderColor: s.color }}
                      aria-hidden
                    />
                  );
                  const hStub = (
                    <span
                      className="inline-block w-3 border-t border-dashed"
                      style={{ borderColor: s.color }}
                      aria-hidden
                    />
                  );
                  return (
                    <div
                      key={s.key}
                      className={cn("flex", stacked ? "flex-col items-center" : "items-center")}
                      title={`${s.label}: ${s.description}`}
                    >
                      {onTop && (
                        <>
                          {chip}
                          {arrow}
                          {vStub}
                        </>
                      )}
                      {onBottom && (
                        <>
                          {vStub}
                          {arrow}
                          {chip}
                        </>
                      )}
                      {!stacked && (
                        <>
                          {hStub}
                          {arrow}
                          {chip}
                        </>
                      )}
                    </div>
                  );
                })}
              </div>
            );
          })()}

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

      {/* Run badge (right corner, clear of the bottom output ports): parts
          processed and this node's wall-clock span — the trace data lives on
          the node, not in a separate timeline. */}
      {partCount !== undefined && partCount > 0 && (
        <div
          className="absolute -bottom-1.5 right-1 text-[9px] font-bold px-1.5 py-px rounded-full bg-secondary text-muted-foreground z-[1]"
          title={`${partCount} part(s) processed${spanUs !== undefined ? ` · active for ${formatUs(spanUs)} (first enter → last exit)` : ""}`}
        >
          {partCount} parts
          {spanUs !== undefined && (
            <span className="ml-1 font-mono font-medium">{formatUs(spanUs)}</span>
          )}
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
