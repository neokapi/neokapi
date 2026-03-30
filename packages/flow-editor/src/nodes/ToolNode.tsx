import { Handle, Position, type NodeProps } from "@xyflow/react";
import { Settings2, GitBranch } from "lucide-react";
import { getCategoryStyle } from "../category";
import { theme } from "../theme";

export function ToolNode({ data, selected }: NodeProps) {
  const category = (data.category as string) || "pipeline";
  const style = getCategoryStyle(category);
  const Icon = style.icon;
  const hasConfig =
    !!data.config && Object.keys(data.config as object).length > 0;
  const isParallel = !!data.parallel;

  return (
    <div
      style={{
        display: "flex",
        minWidth: 180,
        borderRadius: 8,
        overflow: "hidden",
        border: selected
          ? `2px solid ${style.color}`
          : `2px solid ${theme.border}`,
        background: theme.bgCard,
        boxShadow: selected
          ? `0 0 0 3px ${style.color}33, 0 4px 12px oklch(0 0 0 / 0.3)`
          : "0 2px 8px oklch(0 0 0 / 0.2)",
        transition: "border-color 150ms, box-shadow 150ms",
      }}
    >
      {/* Category rail */}
      <div
        style={{
          width: 4,
          background: style.color,
          flexShrink: 0,
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
    </div>
  );
}
