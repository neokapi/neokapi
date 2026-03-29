import { Handle, Position, type NodeProps } from "@xyflow/react";

const STYLE = {
  bg: "rgba(34, 197, 94, 0.12)",
  border: "#22c55e",
  sub: "#86efac",
  text: "#e4e4e7",
};

export function ReaderNode({ data }: NodeProps) {
  return (
    <div
      className="px-4 py-2.5 rounded-lg min-w-[140px] text-center text-[13px]"
      style={{ border: `2px solid ${STYLE.border}`, background: STYLE.bg, color: STYLE.text }}
    >
      <div className="text-[10px] font-semibold mb-0.5" style={{ color: STYLE.border }}>
        INPUT
      </div>
      <div className="font-semibold">{(data.label as string) || "Reader"}</div>
      {data.formatName && (
        <div className="text-[11px] mt-0.5" style={{ color: STYLE.sub }}>
          {data.formatName as string}
        </div>
      )}
      <Handle type="source" position={Position.Right} style={{ background: STYLE.border }} />
    </div>
  );
}
