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
      <div className="flex min-w-[160px] overflow-hidden rounded-lg border-2 border-border bg-card shadow-[0_2px_8px_oklch(0_0_0/0.2)]">
        <div className="w-1 shrink-0" style={{ background: accent }} />
        <div className="flex-1 px-3 py-2">
          <Handle
            type={handleType}
            position={handlePosition}
            style={{
              width: 10,
              height: 10,
              background: accent,
              border: "2px solid var(--card)",
              ...handleSide,
            }}
          />
          <div className="mb-0.5 flex items-center gap-1">
            <Icon size={11} style={{ color: accent }} />
            <span
              className="text-[9px] font-bold uppercase tracking-wider"
              style={{ color: accent }}
            >
              {typeLabel}
            </span>
          </div>
          <div className="text-[13px] font-semibold text-foreground">
            {String(data.label || defaultLabel)}
          </div>
          {!!data.formatName && (
            <div className="mt-0.5 text-[10px] text-muted-foreground">
              {String(data.formatName)}
            </div>
          )}
        </div>
      </div>
    );
  };
}
