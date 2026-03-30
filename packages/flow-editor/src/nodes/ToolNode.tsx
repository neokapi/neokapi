import { Handle, Position, type NodeProps } from "@xyflow/react";
import { Settings2, GitBranch, CheckCircle2, AlertCircle, Loader2, RefreshCw } from "lucide-react";
import { getCategoryStyle } from "../category";
import { theme } from "../theme";

export function ToolNode({ data, selected }: NodeProps) {
  const category = (data.category as string) || "pipeline";
  const style = getCategoryStyle(category);
  const Icon = style.icon;
  const hasConfig =
    !!data.config && Object.keys(data.config as object).length > 0;
  const isParallel = !!data.parallel;
  const execState = data.execState as string | undefined; // "active" | "complete" | "error"
  const partCount = data.partCount as number | undefined;
  const inputs = data.inputs as string[] | undefined;
  const outputs = data.outputs as string[] | undefined;
  const retryConfig = data.retryConfig as Record<string, unknown> | undefined;

  return (
    <div
      style={{
        position: "relative",
        display: "flex",
        minWidth: 180,
        borderRadius: 8,
        overflow: "visible",
        border: execState === "error"
          ? `2px solid ${theme.destructive}`
          : execState === "complete"
            ? `2px solid oklch(0.65 0.15 145)`
            : selected
              ? `2px solid ${style.color}`
              : `2px solid ${theme.border}`,
        background: theme.bgCard,
        boxShadow: selected
            ? `0 0 0 3px ${style.color}33, 0 4px 12px oklch(0 0 0 / 0.3)`
            : "0 2px 8px oklch(0 0 0 / 0.2)",
        animation: execState === "active" ? "nodePulse 1.5s ease-in-out infinite" : undefined,
        transition: "border-color 150ms, box-shadow 150ms",
      }}
    >
      {/* Category rail */}
      <div
        style={{
          width: 4,
          background: style.color,
          flexShrink: 0,
          borderRadius: "6px 0 0 6px",
        }}
      />

      <div style={{ flex: 1, padding: "8px 12px", position: "relative" }}>
        <Handle
          type="target"
          position={Position.Left}
          style={{
            width: 10,
            height: 10,
            background: style.color,
            border: `2px solid ${theme.bgCard}`,
            left: -9,
          }}
        />

        {/* Header row: icon + category label */}
        <div
          style={{
            display: "flex",
            alignItems: "center",
            gap: 4,
            marginBottom: 3,
          }}
        >
          <Icon size={11} style={{ color: style.text }} />
          <span
            style={{
              fontSize: 9,
              fontWeight: 700,
              letterSpacing: "0.06em",
              textTransform: "uppercase",
              color: style.text,
            }}
          >
            {style.label}
          </span>
          {isParallel && (
            <GitBranch
              size={10}
              style={{ color: theme.accent, marginLeft: "auto" }}
              title="Runs in parallel"
            />
          )}
          {hasConfig && !isParallel && (
            <Settings2
              size={10}
              style={{ color: theme.fgMuted, marginLeft: "auto" }}
            />
          )}
        </div>

        {/* Tool name */}
        <div
          style={{
            fontSize: 13,
            fontWeight: 600,
            color: theme.fg,
            lineHeight: 1.25,
          }}
        >
          {String(data.label || data.toolName || "")}
        </div>

        {/* Description (truncated) */}
        {!!data.description && (
          <div
            style={{
              fontSize: 10,
              color: theme.fgMuted,
              marginTop: 2,
              lineHeight: 1.3,
              overflow: "hidden",
              textOverflow: "ellipsis",
              whiteSpace: "nowrap",
              maxWidth: 160,
            }}
          >
            {String(data.description)}
          </div>
        )}

        {/* Port type indicators */}
        {(inputs?.length || outputs?.length) && (
          <div style={{ display: "flex", alignItems: "center", gap: 3, marginTop: 4 }}>
            {inputs?.map((t) => (
              <span
                key={`in-${t}`}
                title={`Input: ${t}`}
                style={{
                  width: 6,
                  height: 6,
                  borderRadius: 3,
                  background: portColor(t),
                  opacity: 0.7,
                }}
              />
            ))}
            {inputs?.length && outputs?.length ? (
              <span style={{ fontSize: 8, color: theme.fgMuted }}>&rarr;</span>
            ) : null}
            {outputs?.map((t) => (
              <span
                key={`out-${t}`}
                title={`Output: ${t}`}
                style={{
                  width: 6,
                  height: 6,
                  borderRadius: 3,
                  background: portColor(t),
                  opacity: 0.7,
                }}
              />
            ))}
          </div>
        )}

        <Handle
          type="source"
          position={Position.Right}
          style={{
            width: 10,
            height: 10,
            background: style.color,
            border: `2px solid ${theme.bgCard}`,
            right: -9,
          }}
        />
      </div>

      {/* Complete badge (top-right) */}
      {execState === "complete" && (
        <div style={{ position: "absolute", top: -4, right: -4, width: 14, height: 14, borderRadius: 7, background: "oklch(0.65 0.15 145)", display: "flex", alignItems: "center", justifyContent: "center", zIndex: 1 }}>
          <CheckCircle2 size={10} style={{ color: "white" }} />
        </div>
      )}

      {/* Error badge (top-right) */}
      {execState === "error" && (
        <div style={{ position: "absolute", top: -4, right: -4, width: 14, height: 14, borderRadius: 7, background: theme.destructive, display: "flex", alignItems: "center", justifyContent: "center", zIndex: 1 }}>
          <AlertCircle size={10} style={{ color: "white" }} />
        </div>
      )}

      {/* Active spinner (top-right) */}
      {execState === "active" && (
        <div style={{ position: "absolute", top: -4, right: -4, width: 14, height: 14, borderRadius: 7, background: theme.accent, display: "flex", alignItems: "center", justifyContent: "center", zIndex: 1 }}>
          <Loader2 size={10} style={{ color: "white", animation: "spin 1s linear infinite" }} />
        </div>
      )}

      {/* Part count badge (bottom-center) */}
      {partCount !== undefined && partCount > 0 && (
        <div style={{ position: "absolute", bottom: -6, left: "50%", transform: "translateX(-50%)", fontSize: 9, fontWeight: 700, padding: "1px 5px", borderRadius: 8, background: theme.bgSecondary, color: theme.fgMuted, zIndex: 1 }}>
          {partCount} pts
        </div>
      )}

      {/* Retry badge (bottom-left, in category rail area) */}
      {retryConfig && (
        <div style={{ position: "absolute", bottom: -4, left: -2, width: 14, height: 14, borderRadius: 7, background: theme.bgSecondary, display: "flex", alignItems: "center", justifyContent: "center", zIndex: 1 }} title="Has retry policy">
          <RefreshCw size={10} style={{ color: theme.fgMuted }} />
        </div>
      )}
    </div>
  );
}

const PORT_COLORS: Record<string, string> = {
  block: "oklch(0.75 0.12 85)",   // amber
  data: "oklch(0.6 0.02 260)",    // slate
  media: "oklch(0.7 0.12 180)",   // teal
  layer: "oklch(0.65 0.14 300)",  // violet
};

function portColor(type: string): string {
  return PORT_COLORS[type] ?? theme.fgMuted;
}
