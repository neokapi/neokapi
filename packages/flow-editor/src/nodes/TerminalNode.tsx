import { Handle, Position, type NodeProps } from "@xyflow/react";
import type { LucideIcon } from "lucide-react";

interface TerminalNodeConfig {
  accent: string;
  icon: LucideIcon;
  typeLabel: string;
  defaultLabel: string;
  handleType: "source" | "target";
  handlePosition: Position;
}

export function createTerminalNode(config: TerminalNodeConfig) {
  const { accent, icon: Icon, typeLabel, defaultLabel, handleType, handlePosition } = config;
  const handleSide = handlePosition === Position.Right ? { right: -9 } : { left: -9 };

  return function TerminalNode({ data }: NodeProps) {
    return (
      <div
        style={{
          display: "flex",
          minWidth: 160,
          borderRadius: 8,
          overflow: "hidden",
          border: "2px solid oklch(0.3 0.01 260)",
          background: "oklch(0.18 0.012 260)",
          boxShadow: "0 2px 8px oklch(0 0 0 / 0.2)",
        }}
      >
        <div style={{ width: 4, background: accent, flexShrink: 0 }} />
        <div style={{ flex: 1, padding: "8px 12px" }}>
          <Handle
            type={handleType}
            position={handlePosition}
            style={{
              width: 10,
              height: 10,
              background: accent,
              border: "2px solid oklch(0.18 0.012 260)",
              ...handleSide,
            }}
          />
          <div style={{ display: "flex", alignItems: "center", gap: 4, marginBottom: 3 }}>
            <Icon size={11} style={{ color: accent }} />
            <span
              style={{
                fontSize: 9,
                fontWeight: 700,
                letterSpacing: "0.06em",
                textTransform: "uppercase",
                color: accent,
              }}
            >
              {typeLabel}
            </span>
          </div>
          <div style={{ fontSize: 13, fontWeight: 600, color: "oklch(0.92 0.005 265)" }}>
            {String(data.label || defaultLabel)}
          </div>
          {!!data.formatName && (
            <div style={{ fontSize: 10, color: "oklch(0.55 0.01 260)", marginTop: 2 }}>
              {String(data.formatName)}
            </div>
          )}
        </div>
      </div>
    );
  };
}
