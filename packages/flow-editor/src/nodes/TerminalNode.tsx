import { Handle, Position, type NodeProps } from "@xyflow/react";
import type { LucideIcon } from "lucide-react";

interface TerminalNodeConfig {
  accent: string;
  icon: LucideIcon;
  typeLabel: string;
  defaultLabel: string;
  handleType: "source" | "target";
  /** Handle position for horizontal layout. */
  handlePosition: Position;
}

const HANDLE_OFFSET: Record<Position, React.CSSProperties> = {
  [Position.Right]: { right: -9 },
  [Position.Left]: { left: -9 },
  [Position.Bottom]: { bottom: -9 },
  [Position.Top]: { top: -9 },
};

/** Map a horizontal handle position to its vertical equivalent. */
function toVertical(pos: Position): Position {
  if (pos === Position.Right) return Position.Bottom;
  if (pos === Position.Left) return Position.Top;
  return pos;
}

export function createTerminalNode(config: TerminalNodeConfig) {
  const { accent, icon: Icon, typeLabel, defaultLabel, handleType } = config;

  return function TerminalNode({ data }: NodeProps) {
    const vertical = data.layoutDirection === "vertical";
    const pos = vertical ? toVertical(config.handlePosition) : config.handlePosition;

    return (
      <div className="flex min-w-[160px] overflow-hidden rounded-lg border-2 border-border bg-card shadow-[0_2px_8px_oklch(0_0_0/0.2)]">
        <div className="w-1 shrink-0" style={{ background: accent }} />
        <div className="flex-1 px-3 py-2">
          <Handle
            type={handleType}
            position={pos}
            style={{
              width: 10,
              height: 10,
              background: accent,
              border: "2px solid var(--card)",
              ...HANDLE_OFFSET[pos],
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
