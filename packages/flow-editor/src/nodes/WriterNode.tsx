import { Handle, Position, type NodeProps } from "@xyflow/react";

const STYLE = {
  bg: "rgba(96, 165, 250, 0.12)",
  border: "#60a5fa",
  sub: "#93c5fd",
  text: "#e4e4e7",
};

export function WriterNode({ data }: NodeProps) {
  return (
    <div
      className="px-4 py-2.5 rounded-lg min-w-[140px] text-center text-[13px]"
      style={{ border: `2px solid ${STYLE.border}`, background: STYLE.bg, color: STYLE.text }}
    >
      <Handle type="target" position={Position.Left} style={{ background: STYLE.border }} />
      <div className="text-[10px] font-semibold mb-0.5" style={{ color: STYLE.border }}>
        OUTPUT
      </div>
      <div className="font-semibold">{(data.label as string) || "Writer"}</div>
      {data.formatName && (
        <div className="text-[11px] mt-0.5" style={{ color: STYLE.sub }}>
          {data.formatName as string}
        </div>
      )}
    </div>
  );
}
