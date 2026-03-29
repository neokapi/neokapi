import { Handle, Position, type NodeProps } from "@xyflow/react";

const STYLE = {
  bg: "rgba(148, 163, 184, 0.08)",
  border: "#64748b",
  sub: "#94a3b8",
  text: "#e4e4e7",
  selectedBorder: "var(--accent, #6366f1)",
  selectedBg: "rgba(99, 102, 241, 0.15)",
};

export function ToolNode({ data, selected }: NodeProps) {
  return (
    <div
      className="px-4 py-2.5 rounded-lg min-w-[140px] text-center text-[13px]"
      style={{
        border: `2px solid ${selected ? STYLE.selectedBorder : STYLE.border}`,
        background: selected ? STYLE.selectedBg : STYLE.bg,
        color: STYLE.text,
        boxShadow: selected ? "0 0 0 2px rgba(99, 102, 241, 0.2)" : undefined,
      }}
    >
      <Handle type="target" position={Position.Left} style={{ background: STYLE.border }} />
      <div className="text-[10px] font-semibold mb-0.5" style={{ color: STYLE.sub }}>
        TOOL
      </div>
      <div className="font-semibold">{(data.label as string) || (data.toolName as string)}</div>
      {data.description && (
        <div className="text-[11px] mt-0.5" style={{ color: STYLE.sub }}>
          {data.description as string}
        </div>
      )}
      <Handle type="source" position={Position.Right} style={{ background: STYLE.border }} />
    </div>
  );
}
